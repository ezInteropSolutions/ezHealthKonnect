package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/executors/enrichment"
	"ezhealthkonnect/services/executors/validation"
)

// StepExecutor interface - all executors must implement this
type StepExecutor interface {
	// Execute runs the transformation step
	Execute(
		ctx context.Context,
		step *models.TransformationStep,
		inputData map[string]interface{},
	) (map[string]interface{}, error)

	// GetStepType returns the step type this executor handles
	GetStepType() string

	// Validate validates step configuration
	Validate(step *models.TransformationStep) error
}

// ExecutorRegistry manages all available step executors (Factory Pattern + OOB)
type ExecutorRegistry struct {
	executors map[string]StepExecutor
	db        *sql.DB
}

// NewExecutorRegistry creates a new executor registry with auto-registration (OOB)
func NewExecutorRegistry(db *sql.DB) *ExecutorRegistry {
	registry := &ExecutorRegistry{
		executors: make(map[string]StepExecutor),
		db:        db,
	}

	// OOB: Auto-register all built-in executors
	registry.autoRegisterExecutors()

	log.Printf("✅ Executor Registry initialized with %d executors", len(registry.executors))

	return registry
}

// autoRegisterExecutors registers all built-in executors (OOB pattern)
func (er *ExecutorRegistry) autoRegisterExecutors() {
	// Essential OOB executor
	er.Register(NewPassthroughExecutor())

	// Pre-processing executors - Validation
	er.Register(validation.NewFieldValidationExecutor())

	// Pre-processing executors - Control Flow
	er.Register(control.NewIfThenElseExecutor())  // Conditional logic
	er.Register(control.NewSwitchCaseExecutor())  // Multi-way branching

	// Pre-processing executors - Enrichment (Strategy Pattern)
	er.Register(enrichment.NewAPIEnrichmentExecutor())       // API enrichment
	er.Register(enrichment.NewDatabaseEnrichmentExecutor(er.db)) // Database enrichment (includes Redis)
	er.Register(enrichment.NewScriptEnrichmentExecutor())    // Script-based enrichment

	// Core executors
	hl7FhirExecutor := NewHL7FHIRMappingExecutor(er.db)
	er.Register(hl7FhirExecutor)
	// Also register under legacy type for backward compatibility
	er.executors["core.mapping"] = hl7FhirExecutor

	// Field Mapping executor (core.transformation)
	er.Register(enrichment.NewFieldMappingExecutor())

	// Post-processing executors
	er.Register(NewFHIRValidationExecutor())

	// Custom executors
	er.Register(NewGenericExecutor())

	log.Println("  ✓ Registered: Passthrough, Field Validation, If-Then-Else, Switch/Case, API Enrichment, Database Enrichment, Script Enrichment, HL7→FHIR Mapping, Field Mapping, FHIR Validation, Generic")
}

// Register adds an executor to the registry
func (er *ExecutorRegistry) Register(executor StepExecutor) {
	stepType := executor.GetStepType()
	er.executors[stepType] = executor
	log.Printf("  📝 Registered executor: %s", stepType)
}

// GetExecutor returns the executor for a given step type
func (er *ExecutorRegistry) GetExecutor(stepType string) StepExecutor {
	if executor, exists := er.executors[stepType]; exists {
		return executor
	}

	// Fallback to generic executor
	log.Printf("⚠️  No specific executor for '%s', using generic executor", stepType)
	return er.executors["generic"]
}

// ExecuteStep executes a transformation step (MVC Controller pattern)
func (er *ExecutorRegistry) ExecuteStep(
	ctx context.Context,
	step models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	// Get appropriate executor for step type
	executor := er.GetExecutor(step.StepType)
	if executor == nil {
		return nil, fmt.Errorf("no executor available for step type: %s", step.StepType)
	}

	// Validate step configuration
	if err := executor.Validate(&step); err != nil {
		return nil, fmt.Errorf("step validation failed: %w", err)
	}

	// Execute the step
	return executor.Execute(ctx, &step, inputData)
}

// ListExecutors returns all registered executor types
func (er *ExecutorRegistry) ListExecutors() []string {
	types := make([]string, 0, len(er.executors))
	for stepType := range er.executors {
		types = append(types, stepType)
	}
	return types
}

// ===============================================================
// LEGACY EXECUTORS REMOVED
// ===============================================================
// The following executors have been removed during consolidation:
// - ValidationExecutor (replaced by FieldValidationExecutor)
// - MetadataEnrichmentExecutor (merged into FieldMappingExecutor)
// - JavaScriptExecutor (replaced by ScriptEnrichmentExecutor)
// - EnrichmentExecutor (placeholder with no functionality)
// See EXECUTOR_CONSOLIDATION_PLAN.md for details

// ===============================================================
// HL7→FHIR MAPPING EXECUTOR
// ===============================================================

// HL7FHIRMappingExecutor handles HL7 to FHIR transformation
type HL7FHIRMappingExecutor struct {
	db                *sql.DB
	transformService  *HL7FHIRTransformServiceV3 // Reuse existing service
}

func NewHL7FHIRMappingExecutor(db *sql.DB) *HL7FHIRMappingExecutor {
	return &HL7FHIRMappingExecutor{
		db:               db,
		transformService: NewHL7FHIRTransformServiceV3(db),
	}
}

func (hme *HL7FHIRMappingExecutor) GetStepType() string {
	return "hl7_to_fhir_mapping"
}

func (hme *HL7FHIRMappingExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  🔄 HL7→FHIR transformation starting...")

	// Get interface ID from config or input
	interfaceID, _ := step.Config["interface_id"].(string)
	if interfaceID == "" {
		interfaceID, _ = inputData["interface_id"].(string)
	}

	messageType, _ := inputData["messageType"].(string)
	messageID, _ := inputData["message_id"].(string)
	correlationID, _ := inputData["correlation_id"].(string)

	// Call existing transformation service using Transform method
	req := &TransformRequest{
		ParsedHL7Data: inputData,
		MessageType:   messageType,
		FHIRVersion:   "R4",
		CreateBundle:  true,
		InterfaceID:   interfaceID,
	}

	resp, err := hme.transformService.Transform(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("HL7→FHIR transformation failed: %w", err)
	}

	// Debug: Check what we got from transformation
	log.Printf("  🔍 [DEBUG] Transform response - Bundle: %v, Success: %v", resp.Bundle != nil, resp.Success)
	if resp.Bundle != nil {
		bundleJSON, _ := json.Marshal(resp.Bundle)
		preview := string(bundleJSON)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		log.Printf("  🔍 [DEBUG] Bundle size: %d bytes, preview: %s...", len(bundleJSON), preview)
	} else {
		log.Printf("  ⚠️  [DEBUG] Bundle is NIL!")
	}

	// Create DeliveryPayload for FHIR over HTTP transmission
	deliveryPayload := hme.createFHIRDeliveryPayload(
		ctx,
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		resp.Bundle,
		step.Config,
	)

	// Add to output data
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Store delivery payload (this will be extracted by engine for transmission)
	outputData["_deliveryPayload"] = deliveryPayload

	// Keep transformed content for storage
	outputData["fhirBundle"] = resp.Bundle

	log.Printf("  ✅ HL7→FHIR transformation complete with delivery payload")

	return outputData, nil
}

// getDestinationConfig retrieves the target endpoint and auth headers from interface configuration
func (hme *HL7FHIRMappingExecutor) getDestinationConfig(
	ctx context.Context,
	interfaceID string,
	stepConfig map[string]interface{},
) (string, map[string]string) {
	authHeaders := make(map[string]string)
	// First check step config for override
	if endpoint, ok := stepConfig["destination_endpoint"].(string); ok && endpoint != "" {
		return endpoint, authHeaders
	}

	// Query interface target_config from database
	query := `
		SELECT target_config
		FROM interfaces
		WHERE id = $1 AND deleted_at IS NULL
	`

	var targetConfigJSON []byte
	err := hme.db.QueryRowContext(ctx, query, interfaceID).Scan(&targetConfigJSON)
	if err != nil {
		log.Printf("⚠️  Failed to get target_config for interface %s: %v, using default", interfaceID, err)
		return "http://localhost:8080/fhir", authHeaders // Fallback default
	}

	// Parse target_config
	var targetConfig map[string]interface{}
	if err := json.Unmarshal(targetConfigJSON, &targetConfig); err != nil {
		log.Printf("⚠️  Failed to parse target_config JSON: %v, using default", err)
		return "http://localhost:8080/fhir", authHeaders
	}

	var endpoint string

	// NEW FORMAT: Check for direct "endpoint" field first (wizard/edit modal unified format)
	if ep, ok := targetConfig["endpoint"].(string); ok && ep != "" {
		endpoint = ep
		log.Printf("📍 Using endpoint from unified format: %s", endpoint)
	} else {
		// LEGACY FORMAT: Build from components (protocol, host, port, path)
		protocol, _ := targetConfig["protocol"].(string)
		host, _ := targetConfig["host"].(string)
		path, _ := targetConfig["path"].(string)

		// Port can be float64 (from JSON) or string
		var port string
		switch v := targetConfig["port"].(type) {
		case float64:
			port = fmt.Sprintf("%.0f", v)
		case string:
			port = v
		}

		if protocol == "" || host == "" {
			log.Printf("⚠️  Incomplete target_config for interface %s, using default", interfaceID)
			return "http://localhost:8080/fhir", authHeaders
		}

		// Build full endpoint URL
		endpoint = fmt.Sprintf("%s://%s", protocol, host)
		if port != "" && port != "80" && port != "443" {
			endpoint += ":" + port
		}
		if path != "" {
			endpoint += path
		}
		log.Printf("📍 Built endpoint from legacy format: %s", endpoint)
	}

	// Extract authentication configuration
	if authType, ok := targetConfig["authType"].(string); ok {
		switch authType {
		case "basic":
			username, _ := targetConfig["username"].(string)
			password, _ := targetConfig["password"].(string)
			if username != "" && password != "" {
				// Base64 encode username:password for Basic Auth
				auth := username + ":" + password
				authHeaders["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
				log.Printf("📍 Added Basic Auth for user: %s", username)
			}
		case "bearer":
			if token, ok := targetConfig["bearerToken"].(string); ok && token != "" {
				authHeaders["Authorization"] = "Bearer " + token
				log.Printf("📍 Added Bearer token authentication")
			}
		case "apikey":
			if apiKey, ok := targetConfig["apiKey"].(string); ok && apiKey != "" {
				authHeaders["X-API-Key"] = apiKey
				log.Printf("📍 Added API Key authentication")
			}
		}
	}

	return endpoint, authHeaders
}

// createFHIRDeliveryPayload creates a delivery payload for FHIR over HTTP
func (hme *HL7FHIRMappingExecutor) createFHIRDeliveryPayload(
	ctx context.Context,
	messageID string,
	correlationID string,
	interfaceID string,
	pipelineID string,
	fhirBundle map[string]interface{},
	stepConfig map[string]interface{},
) *models.DeliveryPayload {

	// Serialize FHIR bundle to JSON bytes
	fhirJSON, err := json.Marshal(fhirBundle)
	if err != nil {
		log.Printf("⚠️  Failed to marshal FHIR bundle: %v", err)
		fhirJSON = []byte("{}")
	}

	// Get destination endpoint and auth headers from interface target_config
	destinationEndpoint, authHeaders := hme.getDestinationConfig(ctx, interfaceID, stepConfig)

	// Create HTTP headers for FHIR
	httpHeaders := map[string]string{
		"Content-Type": "application/fhir+json",
		"Accept":       "application/fhir+json",
	}

	// Merge authentication headers from target_config
	for k, v := range authHeaders {
		httpHeaders[k] = v
	}

	// Add authentication overrides from step config if present
	if authToken, ok := stepConfig["auth_token"].(string); ok && authToken != "" {
		httpHeaders["Authorization"] = "Bearer " + authToken
	}
	if apiKey, ok := stepConfig["api_key"].(string); ok && apiKey != "" {
		httpHeaders["X-API-Key"] = apiKey
	}

	// Create transmission payload (ONLY what gets sent over wire)
	transmission := models.NewFHIRTransmissionPayload(fhirJSON, httpHeaders)

	// Create full delivery payload
	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		pipelineID,
		transmission,
		"http_rest",                // Destination type
		destinationEndpoint,        // Destination endpoint
	)

	// Add format metadata
	payload.Format = "fhir"
	payload.FormatVersion = "R4"
	payload.MessageType = "Bundle"

	resourceType, _ := fhirBundle["resourceType"].(string)
	payload.ResourceType = resourceType

	payload.FormatMetadata = map[string]interface{}{
		"fhir_version":  "R4",
		"bundle_type":   fhirBundle["type"],
		"resource_type": resourceType,
	}

	// Add connectivity metadata
	payload.ConnectivityMetadata = map[string]interface{}{
		"method":           "POST",
		"timeout_seconds":  30,
		"retry_on_5xx":     true,
		"expected_status":  []int{200, 201},
	}

	// Set destination name
	payload.DestinationName = "FHIR Server"
	if name, ok := stepConfig["destination_name"].(string); ok {
		payload.DestinationName = name
	}

	return payload
}

func (hme *HL7FHIRMappingExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// FHIR VALIDATION EXECUTOR
// ===============================================================

// FHIRValidationExecutor validates FHIR bundles
type FHIRValidationExecutor struct{}

func NewFHIRValidationExecutor() *FHIRValidationExecutor {
	return &FHIRValidationExecutor{}
}

func (fve *FHIRValidationExecutor) GetStepType() string {
	return "post.validation"
}

func (fve *FHIRValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// Extract FHIR bundle
	fhirBundle, ok := inputData["fhirBundle"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no fhirBundle found for validation")
	}

	// Basic validation
	resourceType, _ := fhirBundle["resourceType"].(string)
	if resourceType != "Bundle" {
		return nil, fmt.Errorf("invalid FHIR resource type: %s (expected Bundle)", resourceType)
	}

	// TODO: Add comprehensive FHIR validation
	// - Validate against R4 schema
	// - Check required fields
	// - Validate references

	log.Printf("  ✅ FHIR validation passed")

	return inputData, nil
}

func (fve *FHIRValidationExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// GENERIC EXECUTOR (Fallback)
// ===============================================================

// GenericExecutor is a fallback executor for unknown step types
type GenericExecutor struct{}

func NewGenericExecutor() *GenericExecutor {
	return &GenericExecutor{}
}

func (ge *GenericExecutor) GetStepType() string {
	return "generic"
}

func (ge *GenericExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  ⚙️  Generic executor: %s (pass-through)", step.StepName)

	// Generic executor just passes through the input
	return inputData, nil
}

func (ge *GenericExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// getNestedValue retrieves a value from nested map using dot notation
// Example: getNestedValue(data, "patient.name.family") => data["patient"]["name"]["family"]
func getNestedValue(data map[string]interface{}, path string) interface{} {
	log.Printf("[getNestedValue] Path: %s", path)
	keys := strings.Split(path, ".")
	var current interface{} = data

	for _, key := range keys {
		log.Printf("[getNestedValue]   Processing key: %s, current type: %T", key, current)

		// Handle array access like "fields[1]"
		if strings.Contains(key, "[") {
			parts := strings.Split(key, "[")
			mapKey := parts[0]
			indexStr := strings.TrimSuffix(parts[1], "]")
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				log.Printf("[getNestedValue]   Failed to parse index: %v", err)
				return nil
			}

			// Get map value
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				log.Printf("[getNestedValue]   Current is not a map")
				return nil
			}

			arrayValue, exists := currentMap[mapKey]
			if !exists {
				log.Printf("[getNestedValue]   Key %s not found in map", mapKey)
				return nil
			}

			// Convert to array
			array, ok := arrayValue.([]interface{})
			if !ok {
				log.Printf("[getNestedValue]   %s is not an array, type: %T", mapKey, arrayValue)
				return nil
			}

			// Check bounds
			if index < 0 || index >= len(array) {
				log.Printf("[getNestedValue]   Index %d out of bounds (len=%d)", index, len(array))
				return nil
			}

			current = array[index]
		} else {
			// Normal map access
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				log.Printf("[getNestedValue]   Current is not a map for key: %s", key)
				return nil
			}

			value, exists := currentMap[key]
			if !exists {
				log.Printf("[getNestedValue]   Key %s not found", key)
				return nil
			}

			current = value
		}
	}

	log.Printf("[getNestedValue] Final value: %v (type: %T)", current, current)
	return current
}



// ===============================================================
// PASSTHROUGH EXECUTOR (OOB - Most Common Use Case)
// ===============================================================

// PassthroughExecutor simply passes data through without transformation
// Used for raw HL7 delivery, JSON passthrough, etc.
type PassthroughExecutor struct{}

func NewPassthroughExecutor() *PassthroughExecutor {
	return &PassthroughExecutor{}
}

func (e *PassthroughExecutor) GetStepType() string {
	return "passthrough"
}

func (e *PassthroughExecutor) Validate(step *models.TransformationStep) error {
	// No validation needed for passthrough
	return nil
}

func (e *PassthroughExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	log.Printf("▶️  Passthrough: Creating delivery payload for original format")

	// Get metadata
	messageID, _ := inputData["message_id"].(string)
	correlationID, _ := inputData["correlation_id"].(string)
	interfaceID, _ := inputData["interface_id"].(string)
	format, _ := inputData["format"].(string)

	// Create delivery payload based on format
	var deliveryPayload *models.DeliveryPayload

	switch format {
	case "hl7v2":
		deliveryPayload = e.createHL7DeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	case "json":
		deliveryPayload = e.createJSONDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	case "xml":
		deliveryPayload = e.createXMLDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	default:
		deliveryPayload = e.createGenericDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	}

	// Add delivery payload to output
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["_deliveryPayload"] = deliveryPayload

	return outputData, nil
}

// createHL7DeliveryPayload creates delivery payload for HL7 over MLLP
func (e *PassthroughExecutor) createHL7DeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Get raw HL7 content from parsed data
	var hl7Content string
	if rawMsg, ok := inputData["raw_message"].(string); ok {
		hl7Content = rawMsg
	} else {
		// If no raw message, serialize parsed content back to HL7 (fallback)
		hl7Content = "MSH|..." // Would need HL7 serializer
	}

	// Create transmission payload for HL7 MLLP
	transmission := models.NewHL7TransmissionPayload(hl7Content)

	// Get destination from step config
	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "localhost:6661" // Default MLLP port
	}

	// Create delivery payload
	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"tcp_mllp",
		destinationEndpoint,
	)

	// Add format metadata
	payload.Format = "hl7v2"
	if version, ok := inputData["version"].(string); ok {
		payload.FormatVersion = version
	}
	if msgType, ok := inputData["messageType"].(string); ok {
		payload.MessageType = msgType
	}

	payload.FormatMetadata = map[string]interface{}{
		"hl7_version":        payload.FormatVersion,
		"message_type":       payload.MessageType,
		"sending_application": inputData["sendingApplication"],
		"sending_facility":   inputData["sendingFacility"],
	}

	// Add connectivity metadata for MLLP
	payload.ConnectivityMetadata = map[string]interface{}{
		"protocol":          "MLLP",
		"timeout_seconds":   30,
		"expect_ack":        true,
		"ack_timeout":       5,
		"retry_on_nack":     true,
	}

	payload.DestinationName = "HL7 MLLP Receiver"
	if name, ok := step.Config["destination_name"].(string); ok {
		payload.DestinationName = name
	}

	return payload
}

// createJSONDeliveryPayload creates delivery payload for JSON over HTTP
func (e *PassthroughExecutor) createJSONDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Serialize JSON content
	jsonBytes, err := json.Marshal(inputData)
	if err != nil {
		log.Printf("⚠️  Failed to marshal JSON: %v", err)
		jsonBytes = []byte("{}")
	}

	// Create transmission payload
	httpHeaders := map[string]string{
		"Content-Type": "application/json",
	}
	transmission := models.NewJSONTransmissionPayload(jsonBytes, httpHeaders)

	// Get destination
	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "http://localhost:8080/json"
	}

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "json"
	payload.ConnectivityMetadata = map[string]interface{}{
		"method":          "POST",
		"timeout_seconds": 30,
	}

	payload.DestinationName = "JSON HTTP Endpoint"

	return payload
}

// createXMLDeliveryPayload creates delivery payload for XML over HTTP
func (e *PassthroughExecutor) createXMLDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Get XML content (would need XML serializer)
	xmlContent := []byte("<?xml version=\"1.0\"?>")

	httpHeaders := map[string]string{
		"Content-Type": "application/xml",
	}

	transmission := &models.TransmissionPayload{
		Content:     xmlContent,
		ContentType: "application/xml",
		Encoding:    "utf-8",
		Headers:     httpHeaders,
	}

	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "http://localhost:8080/xml"
	}

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "xml"
	payload.DestinationName = "XML HTTP Endpoint"

	return payload
}

// createGenericDeliveryPayload creates delivery payload for unknown formats
func (e *PassthroughExecutor) createGenericDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Serialize as JSON as fallback
	jsonBytes, _ := json.Marshal(inputData)

	transmission := &models.TransmissionPayload{
		Content:     jsonBytes,
		ContentType: "application/octet-stream",
		Encoding:    "utf-8",
		Headers:     make(map[string]string),
	}

	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "unknown"
	payload.DestinationName = "Generic Endpoint"

	return payload
}
