package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/parsers"

	"github.com/gin-gonic/gin"
)

// PipelineTestController handles pipeline testing operations
type PipelineTestController struct {
	db               *sql.DB
	transformService *services.HL7FHIRTransformServiceV3
	pipelineService  *services.TransformationPipelineService
	parserRegistry   *parsers.ParserRegistry
}

// NewPipelineTestController creates a new pipeline test controller
func NewPipelineTestController(db *sql.DB) *PipelineTestController {
	transformService := services.NewHL7FHIRTransformServiceV3(db)
	pipelineService := services.NewTransformationPipelineService(db, nil)
	return &PipelineTestController{
		db:               db,
		transformService: transformService,
		pipelineService:  pipelineService,
		parserRegistry:   parsers.NewParserRegistry(),
	}
}

// TestPipelineRequest represents the request body for testing a pipeline
type TestPipelineRequest struct {
	PipelineID    string                 `json:"pipeline_id"`
	Pipeline      map[string]interface{} `json:"pipeline"`
	SampleMessage string                 `json:"sample_message"`
	TestMessage   string                 `json:"test_message"`   // legacy alias
	MessageFormat string                 `json:"message_format"` // "auto","hl7v2","fhir","json","xml","ccda","edi"
	InputData     map[string]interface{} `json:"input_data"`
}

// TestPipelineResponse represents the response from testing a pipeline
type TestPipelineResponse struct {
	Success          bool                     `json:"success"`
	ExecutionResults []StepExecutionResult    `json:"execution_results"`
	FinalOutput      map[string]interface{}   `json:"final_output"`
	ExecutionTime    int64                    `json:"execution_time_ms"`
	Errors           []string                 `json:"errors,omitempty"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

// StepExecutionResult represents the result of executing a single step
type StepExecutionResult struct {
	StepID      string                 `json:"step_id"`
	StepName    string                 `json:"step_name"`
	StepType    string                 `json:"step_type"`
	Success     bool                   `json:"success"`
	Output      map[string]interface{} `json:"output"`
	Error       string                 `json:"error,omitempty"`
	ExecutionMs int64                  `json:"execution_ms"`
}

// TestPipeline tests a pipeline with sample data
func (c *PipelineTestController) TestPipeline(ctx *gin.Context) {
	var req TestPipelineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Accept sample_message (frontend) or test_message (legacy)
	rawMessage := req.SampleMessage
	if rawMessage == "" {
		rawMessage = req.TestMessage
	}

	log.Printf("🧪 Testing pipeline: pipeline_id=%s, message_length=%d, format=%s",
		req.PipelineID, len(rawMessage), req.MessageFormat)

	// Validate pipeline_id
	if req.PipelineID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "pipeline_id is required",
		})
		return
	}

	// Validate message
	if rawMessage == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "sample_message is required",
		})
		return
	}

	// FIXED: Load pipeline metadata from database by ID
	var interfaceID, messageType string
	query := `SELECT interface_id, message_type FROM transformation_pipelines WHERE id = $1`
	err := c.db.QueryRow(query, req.PipelineID).Scan(&interfaceID, &messageType)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Pipeline not found: %v", err),
		})
		return
	}

	// Load full pipeline with steps
	pipeline, err := c.pipelineService.GetPipeline(ctx, interfaceID, messageType)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to load pipeline: %v", err),
		})
		return
	}

	log.Printf("🚀 Executing pipeline via TransformationPipelineService (supports routing)")

	// Auto-detect or use specified format
	detectedFormat := req.MessageFormat
	if detectedFormat == "" || detectedFormat == "auto" {
		fd := services.NewFormatDetector()
		detection := fd.DetectFormat(rawMessage)
		detectedFormat = string(detection.DetectedFormat)
		log.Printf("🔍 [Test] Auto-detected format: %s (confidence: %.2f)", detectedFormat, detection.Confidence)
	} else {
		log.Printf("📋 [Test] Using specified format: %s", detectedFormat)
	}

	// All formats share the same root envelope approach:
	//   - format-specific data is merged at the ROOT of inputData
	//   - _format is set so enrichMessageEnvelope() builds _semantic_index / _sensitivity_map
	//   - raw content is preserved under message.raw_content (not replaced)
	inputData := map[string]interface{}{
		"_format": detectedFormat,
		"message": map[string]interface{}{
			"raw_content":     rawMessage,
			"detected_format": detectedFormat,
		},
	}

	// Parse and enrich the message using the format-agnostic registry.
	// ParsedJSON carries format-specific structures (enhancedSegments for HL7,
	// raw resource fields for FHIR) so existing executors keep working.
	// EnhancedFields provides the uniform schema-annotated view for new consumers.
	parseResult := c.parserRegistry.Get(detectedFormat).Parse(rawMessage)

	// Merge ParsedJSON at root (format-specific backward-compat data)
	for k, v := range parseResult.ParsedJSON {
		inputData[k] = v
	}

	// Add format-agnostic enriched fields
	if len(parseResult.EnhancedFields) > 0 {
		inputData["enhancedFields"] = parseResult.EnhancedFields
		inputData["fieldOrder"] = parseResult.FieldOrder
	}
	if parseResult.TypeName != "" {
		inputData["typeName"] = parseResult.TypeName
		inputData["typeDescription"] = parseResult.TypeDescription
	}

	// XML-based formats: signal content type for downstream steps
	if detectedFormat == "xml" || detectedFormat == "ccda" || detectedFormat == "hl7v3" {
		if msgMap, ok := inputData["message"].(map[string]interface{}); ok {
			msgMap["content_type"] = "text/xml"
		}
	}

	if parseResult.Success {
		log.Printf("✅ [Test] %s parsed: type=%s fields=%d schemaLoaded=%v",
			detectedFormat, parseResult.Metadata.MessageType,
			len(parseResult.EnhancedFields), parseResult.TypeName != "")
	} else {
		log.Printf("⚠️  [Test] %s parse error: %s — raw content preserved", detectedFormat, parseResult.Error)
	}

	// Execute pipeline with real service (supports routing)
	result, err := c.pipelineService.ExecutePipeline(ctx, pipeline, inputData)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Pipeline execution failed: %v", err),
		})
		return
	}

	// Build simplified step outputs keyed by step name (user-friendly)
	steps := make(map[string]interface{})
	for _, stepLog := range result.ExecutionLog {
		stepData := map[string]interface{}{
			"duration_ms": stepLog.DurationMs,
			"success":     stepLog.Success,
		}

		// Include step output if available (flattened, not nested)
		if stepLog.StepOutput != nil && stepLog.StepOutput.OutputData != nil {
			stepData["output"] = stepLog.StepOutput.OutputData
		} else {
			stepData["output"] = map[string]interface{}{}
		}

		// Include error if failed
		if !stepLog.Success && stepLog.Error != "" {
			stepData["error"] = stepLog.Error
		}

		// Use step name as key (user-friendly lookup)
		steps[stepLog.StepName] = stepData
	}

	// Simplified response format: input, output, steps
	response := gin.H{
		"success":         result.Status == "completed",
		"detected_format": detectedFormat,
		"input":           result.Input,
		"output":          result.Output,
		"steps":           steps,
	}

	if result.Status == "failed" && len(result.Errors) > 0 {
		response["error"] = result.Errors[0].Message
	}

	log.Printf("✅ Test completed: %d steps executed, status=%s", len(result.ExecutionLog), result.Status)
	ctx.JSON(http.StatusOK, response)
}

// loadPipelineFromDB loads a pipeline configuration from the database
func (c *PipelineTestController) loadPipelineFromDB(pipelineID string) (map[string]interface{}, error) {
	query := `
		SELECT pipeline_config
		FROM transformation_pipelines
		WHERE id = $1 AND enabled = true;
	`

	var configJSON []byte
	err := c.db.QueryRow(query, pipelineID).Scan(&configJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("pipeline not found: %s", pipelineID)
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	var pipeline map[string]interface{}
	if err := json.Unmarshal(configJSON, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline config: %v", err)
	}

	return pipeline, nil
}

// executePipeline executes a pipeline with the given input
func (c *PipelineTestController) executePipeline(pipeline map[string]interface{}, testMessage string, inputData map[string]interface{}) TestPipelineResponse {
	response := TestPipelineResponse{
		Success:          true,
		ExecutionResults: []StepExecutionResult{},
		FinalOutput:      make(map[string]interface{}),
		Errors:           []string{},
		Warnings:         []string{},
	}

	// Parse test message to structured data (for now, pass as-is)
	currentData := map[string]interface{}{
		"raw_message": testMessage,
		"parsed":      inputData,
	}

	// Get layers from pipeline
	layers, ok := pipeline["layers"].(map[string]interface{})
	if !ok {
		response.Success = false
		response.Errors = append(response.Errors, "Pipeline has no layers defined")
		return response
	}

	// Execute layers in order: pre -> core -> post
	layerOrder := []string{"pre", "core", "post"}

	for _, layerName := range layerOrder {
		layerData, exists := layers[layerName]
		if !exists {
			continue
		}

		layer, ok := layerData.(map[string]interface{})
		if !ok {
			continue
		}

		// Get execution groups
		groups, ok := layer["execution_groups"].([]interface{})
		if !ok || len(groups) == 0 {
			log.Printf("⏭️  Skipping %s layer - no execution groups", layerName)
			continue
		}

		log.Printf("▶️  Executing %s layer with %d execution group(s)", layerName, len(groups))

		// Execute each execution group
		for groupIdx, groupData := range groups {
			group, ok := groupData.(map[string]interface{})
			if !ok {
				continue
			}

			steps, ok := group["steps"].([]interface{})
			if !ok || len(steps) == 0 {
				continue
			}

			log.Printf("  📦 Execution Group %d: %d step(s)", groupIdx+1, len(steps))

			// Execute each step
			for stepIdx, stepData := range steps {
				step, ok := stepData.(map[string]interface{})
				if !ok {
					continue
				}

				stepName, _ := step["step_name"].(string)
				stepType, _ := step["step_type"].(string)

				log.Printf("    🔹 Step %d: %s (%s)", stepIdx+1, stepName, stepType)

				// Execute step - it receives currentData but returns only step-specific output
				stepResult := c.executeStep(step, currentData)
				response.ExecutionResults = append(response.ExecutionResults, stepResult)

				if !stepResult.Success {
					response.Success = false
					response.Errors = append(response.Errors,
						fmt.Sprintf("Step '%s' failed: %s", stepName, stepResult.Error))

					// For now, continue execution even on error (soft fail)
					// In production, check step config for fail_on_error flag
				} else {
					// Steps may modify currentData directly (e.g., add parsed_hl7, fhir_bundle)
					// But stepResult.Output should ONLY contain step-specific metadata, not the entire message

					// Extract validation warnings if present
					if validationStatus, ok := stepResult.Output["_validation_status"].(string); ok && validationStatus == "warning" {
						if warnings, ok := stepResult.Output["_validation_warnings"].([]interface{}); ok {
							for _, warning := range warnings {
								if warningMap, ok := warning.(map[string]interface{}); ok {
									field := warningMap["field"]
									errorMsg := warningMap["error"]
									response.Warnings = append(response.Warnings,
										fmt.Sprintf("[%s] %s: %s", stepName, field, errorMsg))
								}
							}
						}
					}
				}
			}
		}
	}

	response.FinalOutput = currentData

	log.Printf("✅ Pipeline execution completed: success=%v, errors=%d, warnings=%d",
		response.Success, len(response.Errors), len(response.Warnings))

	return response
}

// executeStep executes a single pipeline step with REAL services
func (c *PipelineTestController) executeStep(step map[string]interface{}, inputData map[string]interface{}) StepExecutionResult {
	startTime := time.Now()

	stepID, _ := step["id"].(string)
	stepName, _ := step["step_name"].(string)
	stepType, _ := step["step_type"].(string)
	config, _ := step["config"].(map[string]interface{})

	result := StepExecutionResult{
		StepID:   stepID,
		StepName: stepName,
		StepType: stepType,
		Success:  true,
		Output:   make(map[string]interface{}),
	}

	// Execute actual step logic
	switch stepType {
	case "pre.validation":
		result = c.executeHL7Validation(inputData, config)

	case "pre.enrichment", "pre.enrichment.metadata", "pre.enrichment.database", "pre.enrichment.api", "pre.enrichment.script":
		result = c.executeEnrichment(inputData, step)

	case "core.mapping":
		// REAL HL7→FHIR transformation using wizard mappings
		result = c.executeHL7ToFHIRTransform(inputData, config)

	case "post.validation":
		result = c.executeFHIRValidation(inputData, config)

	case "post.delivery":
		endpoint, _ := config["endpoint"].(string)
		result.Output["delivered"] = true
		result.Output["endpoint"] = endpoint
		result.Output["message"] = fmt.Sprintf("Delivery (placeholder - would send to %s)", endpoint)

	default:
		result.Output["message"] = fmt.Sprintf("Unknown step type: %s", stepType)
		result.Success = false
	}

	result.ExecutionMs = time.Since(startTime).Milliseconds()
	return result
}

// executeHL7Validation validates HL7 message structure using REAL validation executor
func (c *PipelineTestController) executeHL7Validation(inputData map[string]interface{}, config map[string]interface{}) StepExecutionResult {
	startTime := time.Now()
	result := StepExecutionResult{
		StepName: "HL7 Validation",
		StepType: "pre.validation",
		Success:  true,
		Output:   make(map[string]interface{}),
	}

	rawMessage, _ := inputData["raw_message"].(string)
	if rawMessage == "" {
		result.Success = false
		result.Error = "No raw_message in input data"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Parse HL7 with real schema parser first
	parsed := hl7.ParseWithRealSchema(rawMessage)
	if parsed == nil || !parsed.SchemaLoaded {
		result.Success = false
		result.Error = "HL7 parsing failed - schema not loaded"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Store parsed message for validation executor
	inputData["parsed_hl7"] = parsed

	// Convert parsed HL7 to proper format for validation
	parsedJSON, _ := json.Marshal(parsed)
	var parsedData map[string]interface{}
	json.Unmarshal(parsedJSON, &parsedData)

	// Create transformation step with validation config
	step := &models.TransformationStep{
		StepName:        "Field Validation",
		StepType:        "pre.validation",
		Required:        false, // Will be set from actual step config
		OnErrorStrategy: "continue", // Will be set from actual step config
		Config:          config,
	}

	// Execute REAL validation using ExecutorRegistry
	ctx := context.Background()
	_, err := c.pipelineService.GetExecutorRegistry().ExecuteStep(ctx, *step, parsedData)

	// Return ONLY validation-specific metadata - NOT the entire parsed message
	if err != nil {
		// Validation failed with Required=true or OnErrorStrategy=fail
		result.Success = false
		result.Error = err.Error()
		result.Output["validation_status"] = "rejected"
		result.Output["message"] = fmt.Sprintf("Validation failed: %s", err.Error())
	} else {
		// Check if validation added warnings to parsedData
		validationStatus, _ := parsedData["_validation_status"].(string)

		if validationStatus == "warning" {
			// Validation found issues but continuing
			result.Success = true
			result.Output["validation_status"] = "warning"
			if warnings, ok := parsedData["_validation_warnings"]; ok {
				result.Output["validation_warnings"] = warnings
				result.Output["message"] = "Validation passed with warnings"
			}
		} else {
			// Validation passed completely
			result.Success = true
			result.Output["validation_status"] = "passed"
			result.Output["message"] = "All validation rules satisfied"
		}
	}

	// Include ONLY basic metadata - NOT the entire parsed message
	result.Output["message_type"] = parsed.MessageType
	result.Output["version"] = parsed.Version
	result.Output["segments_count"] = len(parsed.EnhancedSegments)

	// Validation rules applied
	if rulesConfig, ok := config["rules"].([]interface{}); ok {
		result.Output["rules_applied"] = len(rulesConfig)
	}

	result.ExecutionMs = time.Since(startTime).Milliseconds()
	return result
}

// executeEnrichment executes enrichment steps using REAL enrichment executors
func (c *PipelineTestController) executeEnrichment(inputData map[string]interface{}, step map[string]interface{}) StepExecutionResult {
	startTime := time.Now()

	stepName, _ := step["step_name"].(string)
	stepType, _ := step["step_type"].(string)
	config, _ := step["config"].(map[string]interface{})

	result := StepExecutionResult{
		StepName: stepName,
		StepType: stepType,
		Success:  true,
		Output:   make(map[string]interface{}),
	}

	// Parse HL7 first if not already done
	if _, exists := inputData["enhancedSegments"]; !exists {
		rawMessage, _ := inputData["raw_message"].(string)
		if rawMessage != "" {
			parsed := hl7.ParseWithRealSchema(rawMessage)
			if parsed != nil {
				parsedJSON, _ := json.Marshal(parsed)
				var parsedData map[string]interface{}
				json.Unmarshal(parsedJSON, &parsedData)

				// Merge parsed data into inputData
				for k, v := range parsedData {
					inputData[k] = v
				}
			}
		}
	}

	// Create transformation step
	transformStep := &models.TransformationStep{
		StepName: stepName,
		StepType: stepType,
		Config:   config,
	}

	// Execute using ExecutorRegistry
	ctx := context.Background()
	enrichedData, err := c.pipelineService.GetExecutorRegistry().ExecuteStep(ctx, *transformStep, inputData)

	// DEBUG: Log what ExecuteStep returned
	log.Printf("🐛 [DEBUG] executeEnrichment for '%s':", stepName)
	log.Printf("🐛 [DEBUG]   Error: %v", err)
	log.Printf("🐛 [DEBUG]   enrichedData keys: %v", getKeys(enrichedData))
	if enriched, ok := enrichedData["enriched"].(map[string]interface{}); ok {
		log.Printf("🐛 [DEBUG]   enriched keys: %v", getKeys(enriched))
		if script, ok := enriched["script"].(map[string]interface{}); ok {
			log.Printf("🐛 [DEBUG]   enriched.script keys: %v", getKeys(script))
		}
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Output["message"] = fmt.Sprintf("Enrichment failed: %s", err.Error())
	} else {
		// Extract enrichment metadata for step output
		result.Success = true

		// Check what was enriched
		if enriched, ok := enrichedData["enriched"].(map[string]interface{}); ok {
			// Count enriched fields
			enrichedCount := 0
			var enrichedPath string

			if metadata, ok := enriched["metadata"].(map[string]interface{}); ok {
				enrichedCount += len(metadata)
				enrichedPath = "enriched.metadata"
			}
			if database, ok := enriched["database"].(map[string]interface{}); ok {
				enrichedCount += len(database)
				enrichedPath = "enriched.database"
			}
			if api, ok := enriched["api"].(map[string]interface{}); ok {
				enrichedCount += len(api)
				enrichedPath = "enriched.api"
			}
			if script, ok := enriched["script"].(map[string]interface{}); ok {
				// Script enrichment stores result at configurable path
				for key, value := range script {
					enrichedPath = fmt.Sprintf("enriched.script.%s", key)
					if valueMap, ok := value.(map[string]interface{}); ok {
						enrichedCount = len(valueMap)
						result.Output["enriched_data"] = value
					}
				}
			}

			if enrichedCount > 0 {
				result.Output["enriched_path"] = enrichedPath
				result.Output["fields_count"] = enrichedCount
				result.Output["message"] = fmt.Sprintf("Enriched %d fields", enrichedCount)
			}
		}

		// For metadata enrichment, show what was added
		if metadata, ok := enrichedData["metadata"].(map[string]interface{}); ok {
			metadataCount := 0
			for range metadata {
				metadataCount++
			}
			result.Output["metadata_fields"] = metadataCount
		}
	}

	result.ExecutionMs = time.Since(startTime).Milliseconds()

	// DEBUG: Log final result output
	log.Printf("🐛 [DEBUG] executeEnrichment result.Output: %v", result.Output)

	return result
}

// getKeys helper function for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// executeHL7ToFHIRTransform performs REAL HL7→FHIR transformation
func (c *PipelineTestController) executeHL7ToFHIRTransform(inputData map[string]interface{}, config map[string]interface{}) StepExecutionResult {
	startTime := time.Now()
	result := StepExecutionResult{
		StepName: "HL7 to FHIR Transform",
		StepType: "core.mapping",
		Success:  true,
		Output:   make(map[string]interface{}),
	}

	rawMessage, _ := inputData["raw_message"].(string)
	if rawMessage == "" {
		result.Success = false
		result.Error = "No raw_message in input data"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Extract mappings from config
	mappings, _ := config["mappings"].([]interface{})
	if len(mappings) == 0 {
		result.Success = false
		result.Error = "No mappings configured"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Convert mappings to format expected by transform service
	atomicMappings := make([]services.AtomicMapping, 0, len(mappings))
	for _, m := range mappings {
		mapping, ok := m.(map[string]interface{})
		if !ok {
			continue
		}

		atomicMappings = append(atomicMappings, services.AtomicMapping{
			SourcePath:    getString(mapping, "sourcePath"),
			TargetPath:    getString(mapping, "targetPath"),
			TransformType: getString(mapping, "transformType"),
			IsRequired:    getBool(mapping, "isRequired"),
		})
	}

	// Parse HL7 message first
	parsedHL7 := hl7.ParseWithRealSchema(rawMessage)
	if parsedHL7 == nil {
		result.Success = false
		result.Error = "Failed to parse HL7 message"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// DEBUG: Check if parsing succeeded
	log.Printf("🔍 Controller: Parsed HL7 - Success: %v, SchemaLoaded: %v, Segments: %d",
		parsedHL7.Success, parsedHL7.SchemaLoaded, len(parsedHL7.EnhancedSegments))

	// Convert parsed HL7 to JSON and back to map[string]interface{}
	// This ensures proper type conversion for the transformation service
	parsedHL7JSON, err := json.Marshal(parsedHL7)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to serialize parsed HL7: %v", err)
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	log.Printf("🔍 Controller: Serialized to JSON - %d bytes", len(parsedHL7JSON))

	var parsedHL7Data map[string]interface{}
	if err := json.Unmarshal(parsedHL7JSON, &parsedHL7Data); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to deserialize parsed HL7: %v", err)
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// DEBUG: Check what's in the deserialized data
	if enhancedSeg, exists := parsedHL7Data["enhancedSegments"]; exists {
		log.Printf("🔍 Controller: enhancedSegments exists in parsedHL7Data, type: %T", enhancedSeg)
		if segMap, ok := enhancedSeg.(map[string]interface{}); ok {
			log.Printf("🔍 Controller: enhancedSegments is a map with %d keys", len(segMap))
		} else {
			log.Printf("⚠️ Controller: enhancedSegments is NOT map[string]interface{}")
		}
	} else {
		log.Printf("❌ Controller: enhancedSegments NOT in parsedHL7Data")
	}

	// Create transform request with correct structure
	transformRequest := &services.TransformRequest{
		ParsedHL7Data:  parsedHL7Data,
		MessageType:    parsedHL7.MessageType.Code + "^" + parsedHL7.MessageType.Event,
		FHIRVersion:    "R4",
		CreateBundle:   true,
		ValidationMode: "STANDARD",
	}

	// Call REAL transformation service
	ctx := context.Background()
	transformResponse, err := c.transformService.Transform(ctx, transformRequest)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Transformation failed: %v", err)
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Build result - include resources even if validation failed
	result.Output["fhir_bundle"] = transformResponse.Bundle
	result.Output["resources_created"] = transformResponse.FHIRResources
	result.Output["resource_counts"] = transformResponse.ResourceCounts
	result.Output["mappings_applied"] = len(atomicMappings)
	result.Output["validation_errors"] = transformResponse.Errors

	// Store FHIR bundle for validation step
	inputData["fhir_bundle"] = transformResponse.Bundle

	// Set success/error based on validation results
	if !transformResponse.Success {
		result.Success = false
		result.Error = fmt.Sprintf("Transformation completed with %d validation errors", len(transformResponse.Errors))
		if len(transformResponse.Errors) > 0 {
			result.Error += ": " + transformResponse.Errors[0]
		}
		result.Output["message"] = fmt.Sprintf("Transformed to FHIR with warnings - created %d resources but %d validation errors",
			len(transformResponse.FHIRResources), len(transformResponse.Errors))
	} else {
		result.Success = true
		result.Output["message"] = fmt.Sprintf("Successfully transformed to FHIR - created %d resources", len(transformResponse.FHIRResources))
	}

	result.ExecutionMs = time.Since(startTime).Milliseconds()
	return result
}

// executeFHIRValidation validates FHIR bundle against schema
func (c *PipelineTestController) executeFHIRValidation(inputData map[string]interface{}, config map[string]interface{}) StepExecutionResult {
	startTime := time.Now()
	result := StepExecutionResult{
		StepName: "FHIR Validation",
		StepType: "post.validation",
		Success:  true,
		Output:   make(map[string]interface{}),
	}

	fhirBundle, ok := inputData["fhir_bundle"].(map[string]interface{})
	if !ok {
		result.Success = false
		result.Error = "No FHIR bundle in input data"
		result.ExecutionMs = time.Since(startTime).Milliseconds()
		return result
	}

	// Basic validation checks
	validationIssues := []string{}

	// Check bundle structure
	if fhirBundle["resourceType"] != "Bundle" {
		validationIssues = append(validationIssues, "Invalid resourceType - must be Bundle")
	}

	// Check entries
	entries, _ := fhirBundle["entry"].([]interface{})
	resourceCounts := make(map[string]int)

	for _, entry := range entries {
		entryMap, _ := entry.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		resourceType, _ := resource["resourceType"].(string)
		if resourceType != "" {
			resourceCounts[resourceType]++
		}
	}

	// Validate required resources from config
	requiredResources, _ := config["required_resources"].(map[string]interface{})
	for resourceType, requirement := range requiredResources {
		reqMap, _ := requirement.(map[string]interface{})
		minCount := int(getFloat(reqMap, "min"))
		maxCount := int(getFloat(reqMap, "max"))
		actualCount := resourceCounts[resourceType]

		if actualCount < minCount {
			validationIssues = append(validationIssues,
				fmt.Sprintf("%s: required min %d, found %d", resourceType, minCount, actualCount))
		}
		if actualCount > maxCount {
			validationIssues = append(validationIssues,
				fmt.Sprintf("%s: max allowed %d, found %d", resourceType, maxCount, actualCount))
		}
	}

	result.Output["validation_passed"] = len(validationIssues) == 0
	result.Output["issues"] = validationIssues
	result.Output["resources_validated"] = resourceCounts

	if len(validationIssues) > 0 {
		result.Success = false
		result.Error = fmt.Sprintf("Validation failed: %d issues found", len(validationIssues))
		result.Output["message"] = "FHIR validation failed"
	} else {
		result.Output["message"] = fmt.Sprintf("FHIR validation passed - validated %d resources", len(entries))
	}

	result.ExecutionMs = time.Since(startTime).Milliseconds()
	return result
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
