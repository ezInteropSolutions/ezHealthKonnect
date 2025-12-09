package controllers

import (
	"context"
	"database/sql"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/hl7"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type TransformationTestController struct {
	db               *sql.DB
	executorRegistry *services.ExecutorRegistry
}

func NewTransformationTestController(db *sql.DB) *TransformationTestController {
	return &TransformationTestController{
		db:               db,
		executorRegistry: services.NewExecutorRegistry(db),
	}
}

func (c *TransformationTestController) TestPipeline(ctx *gin.Context) {
	var req struct {
		PipelineID    string                 `json:"pipeline_id"`
		Pipeline      map[string]interface{} `json:"pipeline"`
		TestMessage   string                 `json:"test_message"`
		SampleMessage string                 `json:"sample_message"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	testMessage := req.TestMessage
	if testMessage == "" {
		testMessage = req.SampleMessage
	}

	if testMessage == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "test_message is required"})
		return
	}

	// Step 1: Parse HL7 message to JSON (simplified for now)
	parsedJSON := c.parseTestMessage(testMessage)
	log.Printf("🧪 [Test] Parsed test message (simplified)")
	log.Printf("🔍 [Test] Data structure: %+v", parsedJSON)

	// Step 2: Load pipeline steps
	steps, err := c.loadPipelineSteps(req.PipelineID)
	if err != nil {
		log.Printf("❌ [Test] Failed to load pipeline: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to load pipeline: %v", err),
		})
		return
	}
	log.Printf("✅ [Test] Loaded %d steps from pipeline %s", len(steps), req.PipelineID)

	// Step 3: Execute steps
	log.Printf("🔄 [Test] Executing %d steps...", len(steps))
	results, execErr := c.executeSteps(steps, parsedJSON)

	if execErr != nil {
		log.Printf("❌ [Test] Execution failed: %v", execErr)
	} else {
		log.Printf("✅ [Test] Execution completed, %d results", len(results))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":           execErr == nil,
		"error":             formatError(execErr),
		"execution_results": results,
		"parsed_message":    parsedJSON,
		"steps_count":       len(steps),
	})
}

func (c *TransformationTestController) parseTestMessage(message string) map[string]interface{} {
	// Use real HL7 parser with schema
	log.Printf("🔍 [Test] Parsing HL7 message with real parser...")
	enhancedResult := hl7.ParseWithRealSchema(message)

	// Convert enhancedSegments to JSON-serializable format
	segmentsJSON, err := json.Marshal(enhancedResult.EnhancedSegments)
	if err != nil {
		log.Printf("⚠️ [Test] Failed to marshal segments: %v", err)
	}

	var enhancedSegmentsMap map[string]interface{}
	if err := json.Unmarshal(segmentsJSON, &enhancedSegmentsMap); err != nil {
		log.Printf("⚠️ [Test] Failed to unmarshal segments: %v", err)
		enhancedSegmentsMap = make(map[string]interface{})
	}

	// Convert to map for processing
	result := map[string]interface{}{
		"raw":              message,
		"enhancedSegments": enhancedSegmentsMap,
		"messageType":      enhancedResult.MessageType,
		"version":          enhancedResult.Version,
		"dictionaryUsed":   enhancedResult.DictionaryUsed,
		"schemaLoaded":     enhancedResult.SchemaLoaded,
	}

	log.Printf("✅ [Test] Parsed message type: %v", enhancedResult.MessageType)
	return result
}

func (c *TransformationTestController) loadPipelineSteps(pipelineID string) ([]*models.TransformationStep, error) {
	query := `
		SELECT id, step_name, step_type, sequence, layer, config, enabled
		FROM transformation_steps
		WHERE pipeline_id = $1 AND enabled = true
		ORDER BY sequence ASC
	`

	rows, err := c.db.Query(query, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []*models.TransformationStep
	for rows.Next() {
		var step models.TransformationStep
		var configJSON []byte

		err := rows.Scan(
			&step.ID,
			&step.StepName,
			&step.StepType,
			&step.Sequence,
			&step.Layer,
			&configJSON,
			&step.Enabled,
		)
		if err != nil {
			return nil, err
		}

		if len(configJSON) > 0 {
			json.Unmarshal(configJSON, &step.Config)
		}

		steps = append(steps, &step)
	}

	return steps, nil
}

func (c *TransformationTestController) executeSteps(steps []*models.TransformationStep, data map[string]interface{}) ([]map[string]interface{}, error) {
	results := []map[string]interface{}{}
	currentData := data

	for i, step := range steps {
		log.Printf("   Step %d: %s (type: %s)", i+1, step.StepName, step.StepType)

		executor := c.executorRegistry.GetExecutor(step.StepType)
		if executor == nil {
			log.Printf("   ⚠️ No executor found for type: %s", step.StepType)
			results = append(results, map[string]interface{}{
				"step":    step.StepName,
				"type":    step.StepType,
				"status":  "skipped",
				"message": fmt.Sprintf("No executor for type: %s", step.StepType),
			})
			continue
		}

		log.Printf("   ▶️ Executing step: %s (type: %s)", step.StepName, step.StepType)
		log.Printf("   📋 Step config: %+v", step.Config)
		log.Printf("   📦 Config keys: %v", getConfigKeys(step.Config))
		output, err := executor.Execute(context.Background(), step, currentData)
		if err != nil {
			log.Printf("   ❌ Step failed: %v", err)
			results = append(results, map[string]interface{}{
				"step":   step.StepName,
				"type":   step.StepType,
				"status": "failed",
				"error":  err.Error(),
			})
			return results, err
		}

		log.Printf("   ✅ Step completed successfully")
		results = append(results, map[string]interface{}{
			"step":   step.StepName,
			"type":   step.StepType,
			"status": "success",
		})

		currentData = output
	}

	return results, nil
}

func formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func getConfigKeys(config map[string]interface{}) []string {
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	return keys
}

func (c *TransformationTestController) GetPipeline(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "GetPipeline endpoint not yet implemented"})
}

func (c *TransformationTestController) TransformMessage(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "Transform endpoint not yet implemented"})
}
