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
	"strings"

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

	// Extract warnings from results
	warnings := []string{}
	if allWarnings, ok := parsedJSON["_all_warnings"].([]string); ok {
		warnings = allWarnings
		log.Printf("⚠️ [Test] Found %d warnings during execution", len(warnings))
	}

	if execErr != nil {
		log.Printf("❌ [Test] Execution failed: %v", execErr)
	} else {
		log.Printf("✅ [Test] Execution completed, %d results", len(results))
		if len(warnings) > 0 {
			log.Printf("⚠️ [Test] Execution succeeded with %d warnings", len(warnings))
		}
	}

	// Clean parsed message - remove internal metadata and step-added fields
	cleanedMessage := make(map[string]interface{})
	for k, v := range parsedJSON {
		// Skip internal/temporary fields added by executors
		if strings.HasPrefix(k, "_") {
			continue
		}
		// Skip metadata field - it's shown in the enrichment step output
		if k == "metadata" {
			continue
		}
		cleanedMessage[k] = v
	}

	response := gin.H{
		"success":           execErr == nil,
		"error":             formatError(execErr),
		"execution_results": results,
		"parsed_message":    cleanedMessage, // Cleaned message without internal metadata
		"steps_count":       len(steps),
	}

	// Add warnings if any
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	ctx.JSON(http.StatusOK, response)
}

func (c *TransformationTestController) parseTestMessage(message string) map[string]interface{} {
	// TODO: Auto-detect message format (HL7, FHIR, CDA, EDI, CSV, etc.)
	// For now, assume HL7 v2.x format

	log.Printf("🔍 [Test] Parsing HL7 message with real parser...")
	enhancedResult := hl7.ParseWithRealSchema(message)

	// CRITICAL: PRESERVE typed structures - DO NOT marshal/unmarshal to JSON
	// Converting to JSON loses Go type information (map[string]hl7.EnhancedSegment becomes map[string]interface{})
	// This breaks field lookups in validators and other executors
	//
	// This applies to ALL message formats:
	// - HL7 v2.x: Keep enhancedSegments as map[string]hl7.EnhancedSegment
	// - FHIR: Keep resource structures as typed FHIR models
	// - CDA: Keep document structures as typed CDA models
	// - EDI: Keep segment structures as typed EDI models
	// - CSV: Keep parsed structures with appropriate types
	result := map[string]interface{}{
		"raw":              message,
		"enhancedSegments": enhancedResult.EnhancedSegments, // Keep typed structure
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
	warnings := []string{}

	for i, step := range steps {
		log.Printf("   Step %d: %s (type: %s)", i+1, step.StepName, step.StepType)

		executor := c.executorRegistry.GetExecutor(step.StepType)
		if executor == nil {
			log.Printf("   ⚠️ No executor found for type: %s", step.StepType)
			results = append(results, map[string]interface{}{
				"step_name":  step.StepName,
				"step_type":  step.StepType,
				"success":    false,
				"error":      fmt.Sprintf("No executor for type: %s", step.StepType),
				"output":     map[string]interface{}{},
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
				"step_name":  step.StepName,
				"step_type":  step.StepType,
				"success":    false,
				"error":      err.Error(),
				"output":     output,
			})
			return results, err
		}

		log.Printf("   ✅ Step completed successfully")

		// Check for validation warnings in output
		if validationStatus, ok := output["_validation_status"].(string); ok && validationStatus == "warning" {
			if validationWarnings, ok := output["_validation_warnings"].([]models.FieldValidationError); ok {
				for _, warning := range validationWarnings {
					warningMsg := fmt.Sprintf("[%s] %s: %s", step.StepName, warning.Field, warning.Message)
					warnings = append(warnings, warningMsg)
					log.Printf("   ⚠️ Validation warning: %s", warningMsg)
				}
			}
		}

		// Extract only step-specific metadata for the result
		// Don't include the entire message with enhancedSegments in each step output
		stepOutput := map[string]interface{}{}

		// Extract step-specific output based on step type
		switch step.StepType {
		case "pre.validation", "pre.validation.field":
			// Validation steps: Extract validation results only
			if validationStatus, ok := output["_validation_status"].(string); ok {
				stepOutput["validation_status"] = validationStatus
			}
			if validationErrors, ok := output["_validation_errors"].([]models.FieldValidationError); ok {
				stepOutput["validation_errors"] = validationErrors
				stepOutput["error_count"] = len(validationErrors)
			}
			if validationWarnings, ok := output["_validation_warnings"].([]models.FieldValidationError); ok {
				stepOutput["validation_warnings"] = validationWarnings
				stepOutput["warning_count"] = len(validationWarnings)
			}

			// Check if detailed output is enabled in step config
			showDetailedOutput := false
			if config, ok := step.Config["detailedOutput"].(bool); ok {
				showDetailedOutput = config
			}

			// Extract field-level results only if detailed output is enabled
			if showDetailedOutput {
				if fieldResults, ok := output["_field_validation_results"].([]map[string]interface{}); ok {
					stepOutput["field_results"] = fieldResults
				}
			} else {
				// Summary only - just show total fields validated
				if fieldResults, ok := output["_field_validation_results"].([]map[string]interface{}); ok {
					stepOutput["fields_validated"] = len(fieldResults)
				}
			}

		case "pre.enrichment.api":
			// API enrichment: Extract API call metadata AND response data
			if endpoint, ok := step.Config["endpoint"].(string); ok {
				stepOutput["api_endpoint"] = endpoint
			}
			if method, ok := step.Config["method"].(string); ok {
				stepOutput["http_method"] = method
			}
			if targetPath, ok := step.Config["targetPath"].(string); ok {
				stepOutput["enriched_path"] = targetPath

				// Extract the actual API response data
				apiResponse := getNestedValue(output, targetPath)
				if apiResponse != nil {
					stepOutput["api_response"] = apiResponse
				}
			}
			stepOutput["message"] = "API enrichment completed"

		case "pre.enrichment.metadata":
			// Metadata enrichment: Show the actual metadata that was added
			if metadata, ok := output["metadata"].(map[string]interface{}); ok {
				stepOutput["metadata"] = metadata
				stepOutput["fields_added"] = len(metadata)

				// List field names for quick reference
				fieldNames := make([]string, 0, len(metadata))
				for k := range metadata {
					fieldNames = append(fieldNames, k)
				}
				stepOutput["field_names"] = fieldNames
				stepOutput["message"] = fmt.Sprintf("Added %d metadata fields", len(metadata))
			} else {
				stepOutput["message"] = "No metadata added"
			}

		default:
			// For other steps, copy non-message fields
			for k, v := range output {
				// Skip full message structure fields and internal metadata
				if k == "enhancedSegments" || k == "raw" || k == "segmentOrder" ||
				   k == "messageType" || k == "version" || k == "dictionaryUsed" ||
				   k == "schemaLoaded" || strings.HasPrefix(k, "_") {
					continue
				}
				stepOutput[k] = v
			}
		}

		results = append(results, map[string]interface{}{
			"step_name":  step.StepName,
			"step_type":  step.StepType,
			"success":    true,
			"output":     stepOutput, // Only step-specific metadata
		})

		// Update currentData for next step (full message with modifications)
		currentData = output
	}

	// Add warnings to the last result context if any were found
	if len(warnings) > 0 {
		// Store warnings in a way the caller can access them
		currentData["_all_warnings"] = warnings
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

// getNestedValue retrieves a value from nested maps using dot notation
// e.g., "enriched.empi" or "metadata.correlationId"
func getNestedValue(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if currentMap, ok := current.(map[string]interface{}); ok {
			current = currentMap[part]
		} else {
			return nil
		}
	}

	return current
}

func (c *TransformationTestController) GetPipeline(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "GetPipeline endpoint not yet implemented"})
}

func (c *TransformationTestController) TransformMessage(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "Transform endpoint not yet implemented"})
}
