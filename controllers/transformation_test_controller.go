package controllers

import (
	"context"
	"database/sql"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/parsers"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
)

type TransformationTestController struct {
	db               *sql.DB
	executorRegistry *services.ExecutorRegistry
	pipelineService  *services.TransformationPipelineService
	parserRegistry   *parsers.ParserRegistry
}

func NewTransformationTestController(db *sql.DB, credStore *services.CredentialStore) *TransformationTestController {
	return &TransformationTestController{
		db:               db,
		executorRegistry: services.NewExecutorRegistry(db, credStore),
		pipelineService:  services.NewTransformationPipelineService(db, credStore),
		parserRegistry:   parsers.NewParserRegistry(),
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

	log.Printf("🧪 Testing pipeline with TransformationPipelineService (supports routing)")
	log.Printf("   pipeline_id: %s", req.PipelineID)
	if req.Pipeline != nil {
		log.Printf("   pipeline.interfaceId: %v", req.Pipeline["interfaceId"])
		log.Printf("   pipeline.messageType: %v", req.Pipeline["messageType"])
		log.Printf("   pipeline.id: %v", req.Pipeline["id"])
	}

	testMessage := req.TestMessage
	if testMessage == "" {
		testMessage = req.SampleMessage
	}

	if testMessage == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "test_message is required",
		})
		return
	}

	// Get interface_id and message_type - try from pipeline_id first, then from pipeline object
	var interfaceID, messageType string
	var err error

	if req.PipelineID != "" {
		// Load pipeline metadata from database by ID
		query := `SELECT interface_id, message_type FROM transformation_pipelines WHERE id = $1`
		err = c.db.QueryRow(query, req.PipelineID).Scan(&interfaceID, &messageType)
		if err != nil {
			log.Printf("⚠️  Pipeline not found by ID, trying pipeline object...")
		} else {
			log.Printf("✅ Found pipeline by ID: interface=%s, message_type=%s", interfaceID, messageType)
		}
	}

	// Fallback: extract from pipeline object if not found by ID
	if interfaceID == "" && req.Pipeline != nil {
		if iid, ok := req.Pipeline["interfaceId"].(string); ok {
			interfaceID = iid
		}
		if mt, ok := req.Pipeline["messageType"].(string); ok {
			messageType = mt
		}
		log.Printf("📋 Using pipeline object: interface=%s, message_type=%s", interfaceID, messageType)

		// If the inline pipeline has execution_groups, it is self-contained — no DB lookup needed
		_, hasExecutionGroups := req.Pipeline["execution_groups"]
		if hasExecutionGroups {
			log.Printf("✅ Inline pipeline with execution_groups — skipping DB lookup for pipeline ID")
			req.PipelineID = "inline-test"
		} else {
			// Look up the pipeline ID from interface + message type
			query := `SELECT id FROM transformation_pipelines WHERE interface_id = $1 AND message_type = $2`
			err = c.db.QueryRow(query, interfaceID, messageType).Scan(&req.PipelineID)
			if err != nil {
				ctx.JSON(http.StatusNotFound, gin.H{
					"success": false,
					"error":   fmt.Sprintf("Pipeline not found for interface %s and message type %s", interfaceID, messageType),
				})
				return
			}
			log.Printf("✅ Found pipeline ID from interface+messageType: %s", req.PipelineID)
		}
	}

	if interfaceID == "" || messageType == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Could not determine interface_id and message_type from request",
		})
		return
	}

	// IMPORTANT: Use the pipeline from the frontend request (current UI state)
	// This allows testing unsaved changes before committing them
	var pipeline *models.TransformationPipeline

	if req.Pipeline != nil {
		// Convert frontend pipeline object to model
		log.Printf("📋 Using pipeline from frontend request (current UI state, may include unsaved changes)")
		pipeline, err = c.convertFrontendPipeline(req.Pipeline, interfaceID, messageType)
		if err != nil {
			log.Printf("⚠️ Failed to convert frontend pipeline: %v, falling back to database", err)
			pipeline = nil
		}
	}

	// Fallback: Load from database if frontend pipeline not available or conversion failed
	if pipeline == nil {
		log.Printf("📋 Loading pipeline from database (saved state)")
		pipeline, err = c.pipelineService.GetPipeline(ctx, interfaceID, messageType)
		if err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to load pipeline: %v", err),
			})
			return
		}
	}

	log.Printf("🚀 Executing pipeline via TransformationPipelineService (supports conditional routing)")
	fmt.Printf("🚀🚀🚀 EXECUTING PIPELINE: %d steps loaded\n", len(pipeline.Steps))
	for i, s := range pipeline.Steps {
		fmt.Printf("   Step %d: %s (%s) - ID=%s, enabled=%v\n", i, s.StepName, s.StepType, s.ID, s.Enabled)
	}

	// Parse test message - production uses parseResult.ParsedJSON directly
	parsedJSON := c.parseTestMessage(testMessage)

	// Execute pipeline with real service (supports routing)
	// Set test mode so connector steps are skipped/dry-run (not real connections)
	testCtx := models.WithTestMode(ctx.Request.Context())
	result, err := c.pipelineService.ExecutePipeline(testCtx, pipeline, parsedJSON)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Pipeline execution failed: %v", err),
		})
		return
	}

	log.Printf("✅ Pipeline execution completed successfully, starting response transformation")
	log.Printf("   Status: %s", result.Status)
	log.Printf("   Execution log entries: %d", len(result.ExecutionLog))
	fmt.Printf("✅✅✅ EXECUTION COMPLETE: Status=%s, ExecutionLog=%d entries\n", result.Status, len(result.ExecutionLog))

	// Initialize OOP normalizer for consistent output format
	normalizer := models.NewOutputNormalizer()
	log.Printf("🔧 Normalizer initialized: %s", normalizer)

	// Build simplified step outputs keyed by step name (user-friendly)
	// STANDARDIZED STRUCTURE: All steps use step_output and step_metadata
	steps := make(map[string]interface{})
	stepNameCounts := make(map[string]int)                // Track duplicate step names
	pipelineErrors := make([]map[string]interface{}, 0)  // Top-level error aggregation

	for _, stepLog := range result.ExecutionLog {
		log.Printf("🔍 Processing step: '%s'", stepLog.StepName)

		// Standardized metadata for ALL step types
		// Base metadata: duration_ms, success (always present)
		stepMetadata := map[string]interface{}{
			"duration_ms": stepLog.DurationMs,
			"success":     stepLog.Success,
		}

		// Merge executor-specific execution details into step_metadata
		// This provides a SINGLE step_metadata object with all info
		if stepLog.StepOutput != nil && stepLog.StepOutput.ExecutionDetails != nil {
			for key, value := range stepLog.StepOutput.ExecutionDetails {
				stepMetadata[key] = value
			}
			log.Printf("  📊 Merged execution details: %v", getMapKeys(stepLog.StepOutput.ExecutionDetails))
		}

		// Process step output - normalize and flatten for ALL step types
		// OutputData now contains ONLY user-created variables (no step_metadata inside)
		var stepOutput map[string]interface{}
		if stepLog.StepOutput != nil && stepLog.StepOutput.OutputData != nil {
			fmt.Printf("  📊 [%s] Original output keys: %v\n", stepLog.StepName, getMapKeys(stepLog.StepOutput.OutputData))
			fmt.Printf("  📊 [%s] Original output data: %+v\n", stepLog.StepName, stepLog.StepOutput.OutputData)

			// Use OOP normalizer: flattens nested structures + normalizes keys to snake_case
			normalizedOutput := normalizer.NormalizeStepOutput(stepLog.StepOutput.OutputData)
			fmt.Printf("  ✅ [%s] Normalized output keys: %v\n", stepLog.StepName, getMapKeys(normalizedOutput))
			fmt.Printf("  ✅ [%s] Normalized output data: %+v\n", stepLog.StepName, normalizedOutput)

			stepOutput = normalizedOutput
		} else {
			stepOutput = map[string]interface{}{}
		}

		// Break circular references in step output via JSON round-trip
		// Some executors (e.g., Loop) store references to parent context that create cycles
		safeStepOutput := breakCycles(stepOutput)

		// STANDARDIZED STRUCTURE: step_output + step_metadata (2 keys only)
		// Errors are aggregated into a top-level "errors" array, NOT per-step
		stepData := map[string]interface{}{
			"step_output":   safeStepOutput,
			"step_metadata": stepMetadata,
		}

		// Use normalized step name as key (consistent snake_case)
		// Handle duplicate step names by appending a counter (_2, _3, etc.)
		normalizedStepName := normalizer.NormalizeKey(stepLog.StepName)
		stepNameCounts[normalizedStepName]++

		// If this is not the first occurrence, append counter
		stepKey := normalizedStepName
		if stepNameCounts[normalizedStepName] > 1 {
			stepKey = fmt.Sprintf("%s_%d", normalizedStepName, stepNameCounts[normalizedStepName])
		}

		log.Printf("  🏷️ Step name: '%s' -> '%s' (key: '%s')", stepLog.StepName, normalizedStepName, stepKey)

		steps[stepKey] = stepData

		// Collect errors into top-level array
		stepSequence := 0
		if stepLog.StepOutput != nil {
			stepSequence = stepLog.StepOutput.Sequence
		}

		if !stepLog.Success && stepLog.Error != "" {
			// Uncaught error — step failed
			pipelineErrors = append(pipelineErrors, map[string]interface{}{
				"step":     stepKey,
				"sequence": stepSequence,
				"error":    stepLog.Error,
				"caught":   false,
			})
		} else if stepLog.Success && stepLog.StepOutput != nil && strings.HasPrefix(stepLog.StepOutput.Error, "caught:") {
			// Caught error — step succeeded via error handler
			errorEntry := map[string]interface{}{
				"step":     stepKey,
				"sequence": stepSequence,
				"error":    strings.TrimPrefix(stepLog.StepOutput.Error, "caught: "),
				"caught":   true,
				"handler":  stepLog.StepOutput.OutputData["error_handler"],
			}
			// Include default value applied info if present
			if df, ok := stepLog.StepOutput.OutputData["default_applied_field"]; ok {
				errorEntry["default_applied"] = map[string]interface{}{
					"field": df,
					"value": stepLog.StepOutput.OutputData["default_applied_value"],
				}
			}
			pipelineErrors = append(pipelineErrors, errorEntry)
			stepMetadata["error_caught"] = true
		}
	}

	// Build lightweight response (avoid sending huge input/output payloads)
	response := gin.H{
		"success": result.Status == "completed",
		"status":  result.Status,
		"steps":   steps, // Keyed by step name for easy lookup
	}

	// Add top-level errors array (all step errors aggregated)
	if len(pipelineErrors) > 0 {
		response["errors"] = pipelineErrors
	}

	// Include lightweight input/output metadata (without full payload to avoid huge responses)
	if result.Input != nil {
		response["input"] = gin.H{
			"format":       result.Input.Format,
			"message_type": result.Input.MessageType,
			"version":      result.Input.Version,
			"size_bytes":   result.Input.SizeBytes,
		}
	}
	if result.Output != nil {
		response["output"] = gin.H{
			"format":       result.Output.Format,
			"message_type": result.Output.MessageType,
			"version":      result.Output.Version,
			"size_bytes":   result.Output.SizeBytes,
			"payload":      result.Output.Payload, // Include output payload for FHIR bundle display
		}
	}

	if result.Status == "failed" && len(result.Errors) > 0 {
		response["error"] = result.Errors[0].Message
		// Merge pipeline-level errors into the top-level errors array
		for _, e := range result.Errors {
			pipelineErrors = append(pipelineErrors, map[string]interface{}{
				"step":   "_pipeline",
				"error":  e.Message,
				"caught": false,
			})
		}
		response["errors"] = pipelineErrors
	}

	log.Printf("✅ Test completed: %d steps executed, status=%s", len(result.ExecutionLog), result.Status)

	// Pre-serialize to JSON to catch any serialization errors from typed Go structs
	// (Gin's ctx.JSON writes 200 header first, then serializes - if serialization fails,
	// the client sees 200 with empty body instead of an error)
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("❌ JSON serialization failed: %v", err)
		// Return a safe fallback response without step outputs that may contain unserializable types
		fallback := gin.H{
			"success": result.Status == "completed",
			"status":  result.Status,
			"error":   fmt.Sprintf("Response serialization error: %v", err),
			"steps":   gin.H{},
		}
		ctx.JSON(http.StatusOK, fallback)
		return
	}

	log.Printf("✅ Response serialized: %d bytes", len(jsonBytes))
	ctx.Data(http.StatusOK, "application/json; charset=utf-8", jsonBytes)
}

func (c *TransformationTestController) parseTestMessage(message string) map[string]interface{} {
	// Auto-detect format then delegate to the format-agnostic parser registry.
	// HL7: parsedJSON carries enhancedSegments/segmentGroups for backward compat.
	// FHIR: parsedJSON carries resourceType/entry/etc. at root.
	// Other formats: passthrough with raw preserved.
	fd := services.NewFormatDetector()
	detection := fd.DetectFormat(message)
	detectedFormat := string(detection.DetectedFormat)
	log.Printf("🔍 [Test] Auto-detected format: %s (confidence: %.2f)", detectedFormat, detection.Confidence)

	parseResult := c.parserRegistry.Get(detectedFormat).Parse(message)

	result := parseResult.ParsedJSON
	if result == nil {
		result = map[string]interface{}{"raw": message}
	}

	// _format drives enrichMessageEnvelope (semantic index, sensitivity map).
	result["_format"] = detectedFormat

	// Format-agnostic enhanced fields for new consumers (schema-annotated view).
	if len(parseResult.EnhancedFields) > 0 {
		result["enhancedFields"] = parseResult.EnhancedFields
		result["fieldOrder"] = parseResult.FieldOrder
	}
	if parseResult.TypeName != "" {
		result["typeName"] = parseResult.TypeName
		result["typeDescription"] = parseResult.TypeDescription
	}

	log.Printf("✅ [Test] Parsed %s message: type=%s fields=%d schemaLoaded=%v",
		detectedFormat, parseResult.Metadata.MessageType,
		len(parseResult.EnhancedFields), parseResult.TypeName != "")
	return result
}

// convertFrontendPipeline converts the frontend pipeline JSON to a TransformationPipeline model
// This allows testing unsaved changes directly from the UI without saving first
func (c *TransformationTestController) convertFrontendPipeline(pipelineData map[string]interface{}, interfaceID, messageType string) (*models.TransformationPipeline, error) {
	fmt.Printf("🔄🔄🔄 Converting frontend pipeline to model...\n")
	fmt.Printf("   pipelineData keys: %v\n", getMapKeys(pipelineData))

	// Create pipeline with basic info
	pipeline := &models.TransformationPipeline{
		InterfaceID: interfaceID,
		MessageType: messageType,
		Steps:       make([]models.TransformationStep, 0),
	}

	// Get pipeline ID if available
	if id, ok := pipelineData["id"].(string); ok {
		pipeline.ID = id
	}

	// Get pipeline name
	if name, ok := pipelineData["name"].(string); ok {
		pipeline.PipelineName = name
	}

	// Get pipeline config (error handling & retry defaults)
	if pc, ok := pipelineData["pipeline_config"].(map[string]interface{}); ok {
		pipeline.PipelineConfig = pc
		fmt.Printf("   ✅ Pipeline config loaded: %v\n", getMapKeys(pc))
	}

	// Collect all steps — support both formats:
	// 1. Top-level execution_groups (V50+, layers removed)
	// 2. layers map with pre/core/post structure (legacy)
	var allRawSteps []interface{}

	if execGroups, ok := pipelineData["execution_groups"].([]interface{}); ok {
		fmt.Printf("   ✅ Found top-level execution_groups (%d groups)\n", len(execGroups))
		for _, group := range execGroups {
			if groupMap, ok := group.(map[string]interface{}); ok {
				if groupSteps, ok := groupMap["steps"].([]interface{}); ok {
					allRawSteps = append(allRawSteps, groupSteps...)
				}
			}
		}
		fmt.Printf("   📦 Extracted %d steps from execution_groups\n", len(allRawSteps))
	} else if layers, ok := pipelineData["layers"].(map[string]interface{}); ok {
		fmt.Printf("   ✅ Found layers (legacy format): %v\n", getMapKeys(layers))
		for _, layerName := range []string{"pre", "core", "post"} {
			layerData, exists := layers[layerName]
			if !exists {
				continue
			}
			layerMap, ok := layerData.(map[string]interface{})
			if !ok {
				continue
			}
			if directSteps, ok := layerMap["steps"].([]interface{}); ok {
				allRawSteps = append(allRawSteps, directSteps...)
			} else if execGroups, ok := layerMap["execution_groups"].([]interface{}); ok {
				for _, group := range execGroups {
					if groupMap, ok := group.(map[string]interface{}); ok {
						if groupSteps, ok := groupMap["steps"].([]interface{}); ok {
							allRawSteps = append(allRawSteps, groupSteps...)
						}
					}
				}
			}
		}
		fmt.Printf("   📦 Extracted %d steps from layers\n", len(allRawSteps))
	} else {
		fmt.Printf("   ❌ pipeline missing both 'execution_groups' and 'layers' fields!\n")
		return nil, fmt.Errorf("pipeline missing 'execution_groups' or 'layers' field")
	}

	sequence := 10

	for _, stepData := range allRawSteps {
		stepMap, ok := stepData.(map[string]interface{})
		if !ok {
			continue
		}

		// Convert step (value type, not pointer)
		step := models.TransformationStep{
			Sequence: sequence,
			Enabled:  true, // Default to enabled
		}

		// CRITICAL: Extract step ID - needed for conditional routing (skipSteps, nextStep)
		if id, ok := stepMap["id"].(string); ok {
			step.ID = id
		} else if id, ok := stepMap["stepId"].(string); ok {
			step.ID = id
		} else if id, ok := stepMap["step_id"].(string); ok {
			step.ID = id
		}

		// Extract step fields - support both camelCase (frontend model) and snake_case (database model)
		if name, ok := stepMap["stepName"].(string); ok {
			step.StepName = name
		} else if name, ok := stepMap["step_name"].(string); ok {
			step.StepName = name
		}
		if stepType, ok := stepMap["stepType"].(string); ok {
			step.StepType = stepType
		} else if stepType, ok := stepMap["step_type"].(string); ok {
			step.StepType = stepType
		}
		if seq, ok := stepMap["sequence"].(float64); ok {
			step.Sequence = int(seq)
		}
		if enabled, ok := stepMap["enabled"].(bool); ok {
			step.Enabled = enabled
		}
		if config, ok := stepMap["config"].(map[string]interface{}); ok {
			step.Config = config
		}
		if timeout, ok := stepMap["timeoutMs"].(float64); ok {
			step.TimeoutMs = int(timeout)
		} else if timeout, ok := stepMap["timeout_ms"].(float64); ok {
			step.TimeoutMs = int(timeout)
		}
		if step.TimeoutMs <= 0 {
			step.TimeoutMs = 30000 // Default 30s timeout
		}

		// Debug logging to understand what's being processed
		log.Printf("   🔍 Processing step: id=%v, stepName=%v, stepType=%v",
			step.ID, step.StepName, step.StepType)
		fmt.Printf("   🔍 Raw stepMap keys: %v, id=%v\n", getConfigKeys(stepMap), stepMap["id"])

		// Only add enabled steps
		if step.Enabled && step.StepName != "" {
			pipeline.Steps = append(pipeline.Steps, step)
			log.Printf("   ✅ Added step: %s (%s) - sequence %d", step.StepName, step.StepType, step.Sequence)
		}

		sequence += 10
	}

	log.Printf("✅ Converted frontend pipeline: %d steps", len(pipeline.Steps))
	return pipeline, nil
}

func (c *TransformationTestController) loadPipelineSteps(pipelineID string) ([]*models.TransformationStep, error) {
	query := `
		SELECT id, step_name, step_type, sequence, config, enabled
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

			// Apply error handling config (same logic as production pipeline service)
			ehConfig := executors.ResolveErrorHandlingConfig(step.Config, nil)
			if ehConfig != nil {
				output, err = executors.ApplyErrorHandling(ehConfig, err, output, currentData, step.StepName)
			}

			if err != nil {
				// Error not caught — fail the pipeline
				results = append(results, map[string]interface{}{
					"step_name": step.StepName,
					"step_type": step.StepType,
					"success":   false,
					"error":     err.Error(),
					"output":    output,
				})
				return results, err
			}

			// Error was caught/suppressed — record the caught error and continue
			log.Printf("   🛡️ Step error caught by handler, continuing: %s", step.StepName)
			results = append(results, map[string]interface{}{
				"step_name":     step.StepName,
				"step_type":     step.StepType,
				"success":       false,
				"error_handled": true,
				"output":        output,
			})
			if output != nil {
				currentData = output
			}
			continue
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
			// API enrichment: Extract comprehensive request/response details

			// Extract request details (method, URL, headers, body)
			if apiRequest, ok := output["_api_request"].(map[string]interface{}); ok {
				stepOutput["request"] = apiRequest
			}

			// Extract response details (status, headers, body, timing)
			if apiResponse, ok := output["_api_response"].(map[string]interface{}); ok {
				stepOutput["response"] = apiResponse
			}

			// Extract enriched data and show reference path
			targetPath := "enriched.api"
			if tp, ok := step.Config["targetPath"].(string); ok && tp != "" {
				targetPath = tp
			}

			enrichedData := getNestedValue(output, targetPath)
			if enrichedData != nil {
				stepOutput["data"] = enrichedData

				// USER VISIBILITY: Show how to reference this data in subsequent steps
				stepOutput["reference_path"] = targetPath
				stepOutput["usage_example"] = fmt.Sprintf("getNestedValue(input, \"%s\")", targetPath)

				if dataMap, ok := enrichedData.(map[string]interface{}); ok {
					stepOutput["fields_count"] = len(dataMap)
				}
			}

			stepOutput["message"] = "API enrichment completed"

		case "pre.enrichment.metadata":
			// Metadata enrichment: Show the actual metadata that was added
			if metadata, ok := output["metadata"].(map[string]interface{}); ok {
				stepOutput["data"] = metadata
				stepOutput["fields_added"] = len(metadata)

				// USER VISIBILITY: Show how to reference this data in subsequent steps
				stepOutput["reference_path"] = "metadata"
				stepOutput["usage_example"] = "getNestedValue(input, \"metadata.yourKey\")"

				stepOutput["message"] = fmt.Sprintf("Added %d metadata fields", len(metadata))
			} else {
				stepOutput["message"] = "No metadata added"
				stepOutput["reference_path"] = "metadata"
			}

		case "pre.enrichment.database":
			// Database enrichment: Extract enriched data from the configured target path
			targetPath := "enriched.database" // default
			if tp, ok := step.Config["targetPath"].(string); ok && tp != "" {
				targetPath = tp
			}

			// DEBUG: Log what keys exist in output
			log.Printf("   🔍 DEBUG: Output keys: %v", getMapKeys(output))
			log.Printf("   🔍 DEBUG: Looking for targetPath: %s", targetPath)

			// Extract the enriched data
			enrichedData := getNestedValue(output, targetPath)

			// DEBUG: Log what we found
			log.Printf("   🔍 DEBUG: enrichedData type: %T, value: %+v", enrichedData, enrichedData)

			if enrichedData != nil {
				stepOutput["data"] = enrichedData

				// USER VISIBILITY: Show how to reference this data in subsequent steps
				stepOutput["reference_path"] = targetPath
				stepOutput["usage_example"] = fmt.Sprintf("getNestedValue(input, \"%s\")", targetPath)

				// Count fields/rows
				if dataMap, ok := enrichedData.(map[string]interface{}); ok {
					stepOutput["fields_count"] = len(dataMap)
				} else if dataArray, ok := enrichedData.([]interface{}); ok {
					stepOutput["rows_count"] = len(dataArray)
				} else if dataArrayMap, ok := enrichedData.([]map[string]interface{}); ok {
					stepOutput["rows_count"] = len(dataArrayMap)
				}
			} else {
				// DEBUG: Show what's in enriched if it exists
				if enriched, ok := output["enriched"].(map[string]interface{}); ok {
					log.Printf("   🔍 DEBUG: enriched object exists with keys: %v", getMapKeys(enriched))
					stepOutput["debug_enriched_keys"] = getMapKeys(enriched)
					stepOutput["debug_enriched_content"] = enriched
				} else {
					log.Printf("   🔍 DEBUG: enriched is not a map[string]interface{}, type: %T", output["enriched"])
				}
				stepOutput["message"] = "No database enrichment data found"
				stepOutput["reference_path"] = targetPath
				stepOutput["debug_output_keys"] = getMapKeys(output)
			}

		case "pre.enrichment.script":
			// Script enrichment: Extract enriched data from the configured target path
			targetPath := "enriched.script" // default
			if tp, ok := step.Config["targetPath"].(string); ok && tp != "" {
				targetPath = tp
			}

			log.Printf("   🔍 DEBUG [Script]: Output keys: %v", getMapKeys(output))
			log.Printf("   🔍 DEBUG [Script]: Looking for targetPath: %s", targetPath)

			// Extract the enriched data
			enrichedData := getNestedValue(output, targetPath)

			log.Printf("   🔍 DEBUG [Script]: enrichedData type: %T, value: %+v", enrichedData, enrichedData)

			if enrichedData != nil {
				stepOutput["data"] = enrichedData

				// USER VISIBILITY: Show how to reference this data in subsequent steps
				stepOutput["reference_path"] = targetPath
				stepOutput["usage_example"] = fmt.Sprintf("getNestedValue(input, \"%s\")", targetPath)

				// Count fields if it's a map
				if dataMap, ok := enrichedData.(map[string]interface{}); ok {
					stepOutput["fields_count"] = len(dataMap)
				}
			} else {
				log.Printf("   ⚠️  DEBUG [Script]: enrichedData is nil at path %s", targetPath)
				stepOutput["message"] = "No script enrichment data found"
				stepOutput["reference_path"] = targetPath
				stepOutput["debug_output_keys"] = getMapKeys(output)

				// DEBUG: Show what's in enriched if it exists
				if enriched, ok := output["enriched"].(map[string]interface{}); ok {
					log.Printf("   🔍 DEBUG [Script]: enriched object exists with keys: %v", getMapKeys(enriched))
					stepOutput["debug_enriched_keys"] = getMapKeys(enriched)

					// Check if script key exists but is empty/null
					if scriptData, ok := enriched["script"]; ok {
						log.Printf("   🔍 DEBUG [Script]: enriched.script exists but is: %T = %+v", scriptData, scriptData)
						stepOutput["debug_script_value"] = scriptData
					}
				}

				// Check for errors in the step execution
				if errMsg, ok := output["_script_error"].(string); ok && errMsg != "" {
					stepOutput["script_error"] = errMsg
					log.Printf("   ❌ DEBUG [Script]: Script execution error: %s", errMsg)
				}
			}

		case "core.transformation":
			// Field Mapping: Extract mapped fields from enriched.field_mapping
			targetPath := "enriched.field_mapping"

			log.Printf("   🔍 DEBUG [Field Mapping]: Output keys: %v", getMapKeys(output))
			log.Printf("   🔍 DEBUG [Field Mapping]: Looking for targetPath: %s", targetPath)

			// Extract the mapped fields
			mappedFields := getNestedValue(output, targetPath)

			log.Printf("   🔍 DEBUG [Field Mapping]: mappedFields type: %T, value: %+v", mappedFields, mappedFields)

			if mappedFields != nil {
				stepOutput["data"] = mappedFields

				// USER VISIBILITY: Show how to reference this data in subsequent steps
				stepOutput["reference_path"] = targetPath
				stepOutput["usage_example"] = fmt.Sprintf("getNestedValue(input, \"%s.yourVariable\")", targetPath)

				// Count mapped fields (consistent with database/script enrichment - no message)
				if dataMap, ok := mappedFields.(map[string]interface{}); ok {
					stepOutput["fields_count"] = len(dataMap)
				}
			} else {
				log.Printf("   ⚠️  DEBUG [Field Mapping]: mappedFields is nil at path %s", targetPath)
				stepOutput["message"] = "No field mapping data found"
				stepOutput["reference_path"] = targetPath
				stepOutput["debug_output_keys"] = getMapKeys(output)

				// DEBUG: Show what's in enriched if it exists
				if enriched, ok := output["enriched"].(map[string]interface{}); ok {
					log.Printf("   🔍 DEBUG [Field Mapping]: enriched object exists with keys: %v", getMapKeys(enriched))
					stepOutput["debug_enriched_keys"] = getMapKeys(enriched)
				}
			}

		default:
			// For other steps, copy non-message fields
			for k, v := range output {
				// Skip full message structure fields, internal metadata, AND global metadata
				if k == "enhancedSegments" || k == "raw" || k == "segmentOrder" ||
				   k == "messageType" || k == "version" || k == "dictionaryUsed" ||
				   k == "schemaLoaded" || k == "metadata" || k == "enriched" ||
				   strings.HasPrefix(k, "_") {
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

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
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

// ================================================================
// TEST API ENDPOINT
// ================================================================

// TestAPIEndpoint tests an API enrichment step configuration
// POST /api/transformation/test-api-endpoint
func (c *TransformationTestController) TestAPIEndpoint(ctx *gin.Context) {
	var req struct {
		StepConfig map[string]interface{} `json:"stepConfig"` // API enrichment step configuration
		TestData   map[string]interface{} `json:"testData"`   // Sample message data for field mappings
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ Invalid test API endpoint request: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	log.Printf("🧪 Testing API endpoint configuration")

	// Create temporary step for testing
	stepID := "test-step-temp"
	step := &models.TransformationStep{
		ID:       stepID,
		StepName: "API Endpoint Test",
		StepType: "pre.enrichment.api",
		Config:   req.StepConfig,
		Enabled:  true,
	}

	// Get API enrichment executor
	executor := c.executorRegistry.GetExecutor("pre.enrichment.api")
	if executor == nil {
		log.Printf("❌ Failed to get API enrichment executor")
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "API enrichment executor not available",
		})
		return
	}

	// Execute API call with test data
	testContext := context.Background()
	result, execErr := executor.Execute(testContext, step, req.TestData)

	// Extract request/response details
	var requestDetails map[string]interface{}
	var responseDetails map[string]interface{}

	if apiRequest, ok := result["_api_request"].(map[string]interface{}); ok {
		requestDetails = apiRequest
	}

	if apiResponse, ok := result["_api_response"].(map[string]interface{}); ok {
		responseDetails = apiResponse
	}

	// Check if API call failed
	if execErr != nil {
		log.Printf("❌ API endpoint test failed: %v", execErr)
		ctx.JSON(http.StatusOK, gin.H{
			"success":  false,
			"error":    execErr.Error(),
			"request":  requestDetails,
			"response": responseDetails,
			"message":  "API call failed - check endpoint configuration and authentication",
		})
		return
	}

	// Check if response has error (API call failed but not executor error)
	if responseDetails != nil {
		if respErr, hasErr := responseDetails["error"]; hasErr {
			log.Printf("⚠️  API returned error: %v", respErr)
			ctx.JSON(http.StatusOK, gin.H{
				"success":  false,
				"error":    fmt.Sprintf("%v", respErr),
				"request":  requestDetails,
				"response": responseDetails,
				"message":  "API call failed - see error details",
			})
			return
		}
	}

	log.Printf("✅ API endpoint test successful")

	// Return success with full request/response details
	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"request":  requestDetails,
		"response": responseDetails,
		"message":  "API call successful - inspect response to configure field mapping",
		"help":     "Click on fields below to add them to your response mapping configuration",
	})
}

// GetAvailableReferenceVariables analyzes pipeline config and returns available reference paths for each step
func (c *TransformationTestController) GetAvailableReferenceVariables(ctx *gin.Context) {
	var req struct {
		Pipeline     map[string]interface{} `json:"pipeline"`
		CurrentLayer string                 `json:"current_layer"` // e.g., "pre", "core", "post"
		CurrentStep  int                    `json:"current_step"`  // Step index in current layer
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("🔍 [GetAvailableReferenceVariables] Called for layer=%s, step=%d", req.CurrentLayer, req.CurrentStep)

	// DEBUG: Log the entire pipeline to see if resultMapping is present
	if req.Pipeline != nil && req.Pipeline["layers"] != nil {
		pipelineJSON, _ := json.MarshalIndent(req.Pipeline, "", "  ")
		log.Printf("📦 [DEBUG] Full Pipeline JSON:\n%s", string(pipelineJSON))
	}

	// Build available variables based on execution order
	stepVariables := c.buildStepVariables(req.Pipeline, req.CurrentLayer, req.CurrentStep)

	log.Printf("📊 [GetAvailableReferenceVariables] Returning %d variable categories", len(stepVariables))

	ctx.JSON(http.StatusOK, gin.H{
		"success":   true,
		"variables": stepVariables,
		"message":   "Available reference variables for this step (from previous steps only)",
	})
}

// buildStepVariables builds available variables up to a specific step
func (c *TransformationTestController) buildStepVariables(pipeline map[string]interface{}, currentLayer string, currentStep int) []map[string]interface{} {
	variables := make([]map[string]interface{}, 0)

	// Always available: HL7 enhanced segments
	variables = append(variables, map[string]interface{}{
		"category":    "HL7 Message Fields",
		"description": "Parsed HL7 message segments and fields (always available)",
		"variables": []map[string]interface{}{
			{
				"name":          "Message Header",
				"path":          "enhancedSegments.MSH",
				"usage_example": `getNestedValue(input, "enhancedSegments.MSH.fields.3.value")`,
				"description":   "Access MSH segment fields (Sending Application, Facility, etc.)",
			},
			{
				"name":          "Patient Identification",
				"path":          "enhancedSegments.PID",
				"usage_example": `getNestedValue(input, "enhancedSegments.PID.fields.5.value")`,
				"description":   "Access PID segment fields (Name, DOB, Gender, etc.)",
			},
			{
				"name":          "Patient Visit",
				"path":          "enhancedSegments.PV1",
				"usage_example": `getNestedValue(input, "enhancedSegments.PV1.fields.2.value")`,
				"description":   "Access PV1 segment fields (Patient Class, Location, etc.)",
			},
			{
				"name":          "All Segments",
				"path":          "enhancedSegments",
				"usage_example": `getNestedValue(input, "enhancedSegments.YOUR_SEGMENT.fields.N.value")`,
				"description":   "Access any HL7 segment in the message",
			},
		},
	})

	// Define layer execution order
	layerOrder := []string{"pre", "core", "post"}

	// Parse pipeline layers in execution order
	// Collect all enrichment variables first, then group by step name
	allEnrichmentVars := make([]map[string]interface{}, 0)

	if layers, ok := pipeline["layers"].(map[string]interface{}); ok {
		for _, layerName := range layerOrder {
			// Stop if we've reached the current layer
			if layerName == currentLayer {
				// Include steps BEFORE current step in this layer
				if layerData, ok := layers[layerName].(map[string]interface{}); ok {
					if steps, ok := layerData["steps"].([]interface{}); ok {
						enrichmentVars := c.extractEnrichmentVariablesUpTo(steps, currentStep)
						allEnrichmentVars = append(allEnrichmentVars, enrichmentVars...)
					}
				}
				break
			}

			// Include all steps from previous layers
			if layerData, ok := layers[layerName].(map[string]interface{}); ok {
				if steps, ok := layerData["steps"].([]interface{}); ok {
					enrichmentVars := c.extractEnrichmentVariablesUpTo(steps, -1) // -1 means all steps
					allEnrichmentVars = append(allEnrichmentVars, enrichmentVars...)
				}
			}
		}
	}

	// Group variables by step name
	stepGroups := make(map[string][]map[string]interface{})
	for _, variable := range allEnrichmentVars {
		stepName, _ := variable["step_name"].(string)
		if stepName == "" {
			stepName = "Unknown_Step"
		}
		stepGroups[stepName] = append(stepGroups[stepName], variable)
	}

	// Convert groups to category format
	for stepName, vars := range stepGroups {
		variables = append(variables, map[string]interface{}{
			"category":    stepName,
			"description": fmt.Sprintf("Variables from %s", stepName),
			"variables":   vars,
		})
	}

	return variables
}

// extractEnrichmentVariablesUpTo extracts reference variables up to a specific step index
// NEW IMPLEMENTATION: Uses VariableProvider interface for systematic variable discovery
func (c *TransformationTestController) extractEnrichmentVariablesUpTo(steps []interface{}, upToStep int) []map[string]interface{} {
	variables := make([]map[string]interface{}, 0)

	maxSteps := len(steps)
	if upToStep >= 0 && upToStep < maxSteps {
		maxSteps = upToStep
	}

	for i := 0; i < maxSteps; i++ {
		stepMap, ok := steps[i].(map[string]interface{})
		if !ok {
			continue
		}

		stepType, _ := stepMap["step_type"].(string)
		stepName, _ := stepMap["step_name"].(string)
		config, _ := stepMap["config"].(map[string]interface{})

		if stepName == "" {
			stepName = fmt.Sprintf("Step_%d", i+1)
		}

		// Sanitize step name - replace spaces with underscores
		sanitizedStepName := strings.ReplaceAll(stepName, " ", "_")

		// Create a TransformationStep from the step map
		step := &models.TransformationStep{
			StepName: stepName,
			StepType: stepType,
			Sequence: i,
			Config:   config,
		}

		// Get executor for this step type
		executor := c.executorRegistry.GetExecutor(stepType)
		if executor == nil {
			log.Printf("⚠️  [extractEnrichmentVariables] No executor found for step type: %s", stepType)
			continue
		}

		// Check if executor implements VariableProvider interface
		if variableProvider, ok := executor.(interface {
			GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition
		}); ok {
			// Use VariableProvider interface - SYSTEMATIC APPROACH
			log.Printf("✅ [extractEnrichmentVariables] Using VariableProvider for %s (step: %s)", stepType, sanitizedStepName)

			outputVars := variableProvider.GetOutputVariables(step)
			log.Printf("   📦 Got %d output variables from VariableProvider", len(outputVars))
			for _, varDef := range outputVars {
				log.Printf("      • %s: %s (path: %s)", varDef.Name, varDef.Description, varDef.Path)
				variables = append(variables, map[string]interface{}{
					"name":          varDef.Name,
					"path":          varDef.Path,
					"usage_example": varDef.UsageExample,
					"description":   varDef.Description,
					"data_type":     varDef.DataType,
					"source_field":  varDef.SourceField,
					"examples":      varDef.Examples,
					"category":      varDef.Category,
					"required":      varDef.Required,
					"step_index":    i,
					"step_name":     sanitizedStepName,
				})
			}
		} else {
			// LEGACY FALLBACK: For executors that don't implement VariableProvider yet
			log.Printf("⚠️  [extractEnrichmentVariables] Executor %s doesn't implement VariableProvider, using legacy logic", stepType)

			switch stepType {
			case "pre.enrichment.metadata":
				// Metadata enrichment adds custom metadata fields
				customMetadata, _ := config["customMetadata"].(map[string]interface{})
				if len(customMetadata) > 0 {
					for key := range customMetadata {
						variables = append(variables, map[string]interface{}{
							"name":          key,
							"path":          fmt.Sprintf("metadata.%s", key),
							"usage_example": fmt.Sprintf(`getNestedValue(input, "metadata.%s")`, key),
							"description":   fmt.Sprintf("Metadata field from %s", sanitizedStepName),
							"step_index":    i,
							"step_name":     sanitizedStepName,
						})
					}
				}

			case "pre.enrichment.api", "pre.enrichment.script":
				// API and Script enrichment: Generic variables
				basePath := fmt.Sprintf("[\"%s\"].enriched_data", stepName)

				var description string
				var examples []string
				switch stepType {
				case "pre.enrichment.api":
					description = "API enrichment results"
					examples = []string{
						fmt.Sprintf(`%s.responseData`, basePath),
						fmt.Sprintf(`%s.status`, basePath),
						fmt.Sprintf(`%s.timestamp`, basePath),
					}
				case "pre.enrichment.script":
					description = "Script enrichment results"
					examples = []string{
						fmt.Sprintf(`%s.riskScore`, basePath),
						fmt.Sprintf(`%s.riskLevel`, basePath),
						fmt.Sprintf(`%s.calculatedAt`, basePath),
					}
				}

				descriptionWithExamples := fmt.Sprintf("%s | Examples: %s", description, strings.Join(examples, " • "))

				variables = append(variables, map[string]interface{}{
					"name":          "enriched_data",
					"path":          basePath,
					"usage_example": fmt.Sprintf(`getNestedValue(input, "%s.fieldName")`, basePath),
					"description":   descriptionWithExamples,
					"examples":      examples,
					"step_index":    i,
					"step_name":     sanitizedStepName,
				})
			}
		}
	}

	return variables
}

// extractColumnsFromQuery parses a SQL query and extracts column names from SELECT
func (c *TransformationTestController) extractColumnsFromQuery(query string) []string {
	if query == "" {
		return nil
	}

	// Convert to lowercase for easier parsing
	lowerQuery := strings.ToLower(strings.TrimSpace(query))

	// Find SELECT and FROM positions
	selectPos := strings.Index(lowerQuery, "select")
	fromPos := strings.Index(lowerQuery, "from")

	if selectPos == -1 || fromPos == -1 || fromPos <= selectPos {
		return nil
	}

	// Extract column list between SELECT and FROM
	columnsPart := query[selectPos+6 : fromPos] // +6 for "select"
	columnsPart = strings.TrimSpace(columnsPart)

	// Handle SELECT *
	if strings.TrimSpace(columnsPart) == "*" {
		return nil // Can't determine columns from SELECT *
	}

	// Split by comma and extract column names/aliases
	columns := []string{}
	parts := strings.Split(columnsPart, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for AS alias
		asPos := strings.LastIndex(strings.ToLower(part), " as ")
		if asPos != -1 {
			// Use the alias
			alias := strings.TrimSpace(part[asPos+4:])
			columns = append(columns, alias)
			continue
		}

		// Check for space-separated alias (e.g., "column_name alias")
		spaceParts := strings.Fields(part)
		if len(spaceParts) > 1 {
			// Last part is likely the alias
			columns = append(columns, spaceParts[len(spaceParts)-1])
		} else {
			// Extract column name after last dot (for table.column)
			dotPos := strings.LastIndex(part, ".")
			if dotPos != -1 {
				columns = append(columns, part[dotPos+1:])
			} else {
				columns = append(columns, part)
			}
		}
	}

	return columns
}

// ValidateScript validates a JavaScript script for syntax and dependency errors
func (c *TransformationTestController) ValidateScript(ctx *gin.Context) {
	var request struct {
		Script     string `json:"script"`
		PipelineID string `json:"pipelineId"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	if request.Script == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Script cannot be empty",
		})
		return
	}

	// Get the script enrichment executor
	executor := c.executorRegistry.GetExecutor("pre.enrichment.script")
	if executor == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Script executor not available",
		})
		return
	}

	// Create a dummy transformation step for validation.
	// Use the executor's canonical type (not the alias) so BaseExecutor.PreExecute passes.
	step := &models.TransformationStep{
		StepName: "Script Validation",
		StepType: executor.GetStepType(),
		Sequence: 1,
		Enabled:  true, // CRITICAL: Step must be enabled for validation to work
		Config: map[string]interface{}{
			"script":      request.Script,
			"timeout_ms":  5000,
			"failOnError": false,
		},
	}

	// Create dummy input data with common structures that scripts might reference
	dummyInput := map[string]interface{}{
		"enhancedSegments": map[string]interface{}{
			"MSH": map[string]interface{}{
				"fields": []interface{}{
					map[string]interface{}{"value": "test"},
				},
			},
			"PID": map[string]interface{}{
				"fields": []interface{}{
					map[string]interface{}{"value": "test"},
				},
			},
		},
		"enriched": map[string]interface{}{
			"api":      map[string]interface{}{},
			"database": map[string]interface{}{},
		},
		"metadata": map[string]interface{}{},
	}

	// FIRST: Do a syntax-only check by compiling without executing
	// Wrap in function to allow return statements, just like the executor does
	wrappedScript := fmt.Sprintf("(function() { %s })()", request.Script)
	_, compileErr := goja.Compile("syntax_check", wrappedScript, false)
	if compileErr != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Syntax error: %s", compileErr.Error()),
			"details": map[string]interface{}{
				"raw_error": compileErr.Error(),
				"suggestions": []string{
					"Check for missing brackets, parentheses, or semicolons",
					"Ensure all variables are properly declared",
					"Verify JavaScript syntax is correct",
				},
			},
		})
		return
	}

	// SECOND: Try to execute the script with dummy data to catch runtime errors
	_, err := executor.Execute(context.Background(), step, dummyInput)
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   err.Error(),
			"details": map[string]interface{}{
				"raw_error": err.Error(),
			},
		})
		return
	}

	// Script is valid (syntax OK, no runtime errors)
	// Note: We don't check return values - validation is ONLY for syntax
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Script is syntactically valid",
	})
}

// breakCycles converts a map through JSON round-trip to eliminate circular references.
// Some executors (e.g., Loop) pass context maps that reference parent data, creating
// cycles that cause json.Marshal to fail with "encountered a cycle via []interface{}".
// The round-trip produces a clean copy with all cycles broken.
func breakCycles(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("⚠️ breakCycles: marshal failed (%v), stripping step output", err)
		// If even the round-trip fails, return just the keys for debugging
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		return map[string]interface{}{
			"_error":         "circular reference in step output",
			"_original_keys": keys,
		}
	}
	var clean map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &clean); err != nil {
		return map[string]interface{}{
			"_error": "failed to unmarshal step output",
		}
	}
	return clean
}
