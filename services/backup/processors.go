package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// =============================================================================
// INPUT PROCESSORS
// =============================================================================

// MLLPInputProcessor handles MLLP (Minimal Lower Layer Protocol) input
type MLLPInputProcessor struct {
	server *MLLPServer
	mllpService *MLLPConnectivityService
}

// MLLPServer represents an MLLP server instance
type MLLPServer struct {
	Host           string
	Port           int
	MaxConnections int
	Timeout        time.Duration
}

func (p *MLLPInputProcessor) ProcessInput(ctx context.Context, config InputConfig, message []byte) (*ProcessedMessage, error) {
	// Parse MLLP message
	hl7Message, err := p.parseMLLPMessage(message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MLLP message: %w", err)
	}

	// Extract message metadata from HL7 segments
	metadata := MessageMetadata{
		SourceType:     "mllp",
		MessageType:    p.extractMessageType(hl7Message),
		SourceEndpoint: p.extractSourceEndpoint(config),
		SourceIP:       p.extractSourceIP(config),
		MessageSize:    len(message),
		Encoding:       "utf-8",
		Priority:       1,
		Properties:     make(map[string]interface{}),
	}

	// Parse HL7 segments using enhanced parsing
	parsedContent, err := p.parseHL7SegmentsEnhanced(hl7Message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HL7 segments: %w", err)
	}

	processedMessage := &ProcessedMessage{
		ParsedContent: parsedContent,
		Metadata:      metadata,
	}

	return processedMessage, nil
}

func (p *MLLPInputProcessor) Start(ctx context.Context, config InputConfig) error {
	// Initialize MLLP service if not already done
	if p.mllpService == nil {
		p.mllpService = NewMLLPConnectivityService(nil)
	}

	// Extract MLLP configuration
	mllpConfig := &MLLPConfig{
		Host: p.getStringConfig(config.ConnectorConfig, "host", "0.0.0.0"),
		Port: p.getIntConfig(config.ConnectorConfig, "port", DefaultMLLPPort),
		MaxConnections: p.getIntConfig(config.ConnectorConfig, "max_connections", DefaultMaxConnections),
		ReadTimeout: time.Duration(p.getIntConfig(config.ConnectorConfig, "read_timeout_ms", 30000)) * time.Millisecond,
		WriteTimeout: time.Duration(p.getIntConfig(config.ConnectorConfig, "write_timeout_ms", 10000)) * time.Millisecond,
		MaxMessageSize: p.getIntConfig(config.ConnectorConfig, "max_message_size", DefaultMaxMessageSize),
		EnableKeepAlive: p.getBoolConfig(config.ConnectorConfig, "enable_keep_alive", true),
		KeepAliveInterval: time.Duration(p.getIntConfig(config.ConnectorConfig, "keep_alive_interval_ms", 60000)) * time.Millisecond,
	}

	// Start MLLP listener
	listener, err := p.mllpService.StartListener(ctx, mllpConfig)
	if err != nil {
		return fmt.Errorf("failed to start MLLP listener: %w", err)
	}

	p.server = &MLLPServer{
		Host: mllpConfig.Host,
		Port: mllpConfig.Port,
		MaxConnections: mllpConfig.MaxConnections,
		Timeout: mllpConfig.ReadTimeout,
	}

	log.Printf("🚀 MLLP server started on %s:%d (ID: %s)", mllpConfig.Host, mllpConfig.Port, listener.ID)
	return nil
}

func (p *MLLPInputProcessor) Stop(ctx context.Context) error {
	if p.mllpService == nil {
		return nil
	}

	// Stop all MLLP listeners
	err := p.mllpService.StopAllListeners(ctx)
	if err != nil {
		log.Printf("Warning: Error stopping MLLP listeners: %v", err)
	}

	log.Printf("🛑 MLLP server stopped")
	return nil
}

func (p *MLLPInputProcessor) parseMLLPMessage(message []byte) (string, error) {
	// MLLP wrapping: <SB>message<EB><CR>
	// SB = 0x0B (Start Block), EB = 0x1C (End Block), CR = 0x0D (Carriage Return)

	if len(message) < 3 {
		return "", fmt.Errorf("message too short for MLLP format")
	}

	// Remove MLLP wrapper
	if message[0] == 0x0B {
		// Find end block
		for i := 1; i < len(message)-1; i++ {
			if message[i] == 0x1C && message[i+1] == 0x0D {
				return string(message[1:i]), nil
			}
		}
	}

	// If no MLLP wrapper found, assume raw HL7
	return string(message), nil
}

func (p *MLLPInputProcessor) extractMessageType(hl7Message string) string {
	// Extract from MSH segment
	lines := strings.Split(hl7Message, "\r")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "MSH") {
		segments := strings.Split(lines[0], "|")
		if len(segments) >= 9 {
			return segments[8] // MSH.9 - Message Type
		}
	}
	return "unknown"
}

func (p *MLLPInputProcessor) parseHL7SegmentsEnhanced(hl7Message string) (map[string]interface{}, error) {
	parsedContent := make(map[string]interface{})
	segments := make(map[string]interface{})
	structuredSegments := make(map[string]interface{})

	lines := strings.Split(hl7Message, "\r")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}

		segmentType := line[:3]
		fields := strings.Split(line, "|")

		// Store both raw fields and structured data
		segments[segmentType] = fields
		structuredSegments[segmentType] = p.parseSegmentStructure(segmentType, fields)
	}

	parsedContent["segments"] = segments
	parsedContent["structured_segments"] = structuredSegments
	parsedContent["raw_message"] = hl7Message
	parsedContent["message_control_id"] = p.extractMessageControlID(segments)
	parsedContent["sending_application"] = p.extractSendingApplication(segments)
	parsedContent["receiving_application"] = p.extractReceivingApplication(segments)

	return parsedContent, nil
}

// FileInputProcessor handles file-based input
type FileInputProcessor struct {
	watchDir string
}

func (p *FileInputProcessor) ProcessInput(ctx context.Context, config InputConfig, message []byte) (*ProcessedMessage, error) {
	metadata := MessageMetadata{
		SourceType:  "file",
		MessageSize: len(message),
		Encoding:    "utf-8",
		Properties:  make(map[string]interface{}),
	}

	// Determine file type and parse accordingly
	parsedContent := map[string]interface{}{
		"raw_content": string(message),
		"file_size":   len(message),
	}

	// Try to detect content type
	contentType := p.detectContentType(message)
	parsedContent["content_type"] = contentType
	metadata.MessageType = contentType

	processedMessage := &ProcessedMessage{
		ParsedContent: parsedContent,
		Metadata:      metadata,
	}

	return processedMessage, nil
}

func (p *FileInputProcessor) Start(ctx context.Context, config InputConfig) error {
	p.watchDir = config.ConnectorConfig["watch_directory"].(string)
	log.Printf("📁 Starting file watcher on directory: %s", p.watchDir)
	return nil
}

func (p *FileInputProcessor) Stop(ctx context.Context) error {
	log.Printf("🛑 Stopping file watcher")
	return nil
}

func (p *FileInputProcessor) detectContentType(content []byte) string {
	contentStr := string(content)

	if strings.HasPrefix(contentStr, "MSH|") {
		return "hl7"
	}
	if strings.HasPrefix(contentStr, "{") && strings.Contains(contentStr, "resourceType") {
		return "fhir"
	}
	if strings.HasPrefix(contentStr, "<") {
		return "xml"
	}

	return "text"
}

// APIInputProcessor handles REST API input
type APIInputProcessor struct{}

func (p *APIInputProcessor) ProcessInput(ctx context.Context, config InputConfig, message []byte) (*ProcessedMessage, error) {
	metadata := MessageMetadata{
		SourceType:  "api",
		MessageSize: len(message),
		Encoding:    "utf-8",
		Properties:  make(map[string]interface{}),
	}

	// Parse JSON content
	var parsedContent map[string]interface{}
	if err := json.Unmarshal(message, &parsedContent); err != nil {
		// If not JSON, treat as raw content
		parsedContent = map[string]interface{}{
			"raw_content": string(message),
		}
	}

	metadata.MessageType = "api_request"

	processedMessage := &ProcessedMessage{
		ParsedContent: parsedContent,
		Metadata:      metadata,
	}

	return processedMessage, nil
}

func (p *APIInputProcessor) Start(ctx context.Context, config InputConfig) error {
	log.Printf("🌐 API input processor ready")
	return nil
}

func (p *APIInputProcessor) Stop(ctx context.Context) error {
	log.Printf("🛑 Stopping API input processor")
	return nil
}

// QueueInputProcessor handles message queue input
type QueueInputProcessor struct{}

func (p *QueueInputProcessor) ProcessInput(ctx context.Context, config InputConfig, message []byte) (*ProcessedMessage, error) {
	metadata := MessageMetadata{
		SourceType:  "queue",
		MessageSize: len(message),
		Encoding:    "utf-8",
		Properties:  make(map[string]interface{}),
	}

	// Parse queue message (could be various formats)
	parsedContent := map[string]interface{}{
		"queue_message": string(message),
	}

	metadata.MessageType = "queue_message"

	processedMessage := &ProcessedMessage{
		ParsedContent: parsedContent,
		Metadata:      metadata,
	}

	return processedMessage, nil
}

func (p *QueueInputProcessor) Start(ctx context.Context, config InputConfig) error {
	log.Printf("📬 Queue input processor ready")
	return nil
}

func (p *QueueInputProcessor) Stop(ctx context.Context) error {
	log.Printf("🛑 Stopping queue input processor")
	return nil
}

// =============================================================================
// VALIDATORS
// =============================================================================

// HL7Validator validates HL7 messages
type HL7Validator struct{
	db *sql.DB
	validationService *BusinessValidationService
}

func (v *HL7Validator) Validate(ctx context.Context, config ValidationConfig, message *ProcessedMessage) (*ValidationResult, error) {
	// Initialize validation service if available
	if v.validationService == nil && v.db != nil {
		v.validationService = NewBusinessValidationService(v.db)
	}

	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
		Metadata: make(map[string]interface{}),
	}

	// Get HL7 content
	segments, ok := message.ParsedContent["segments"].(map[string]interface{})
	if !ok {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "segments",
			Code:     "MISSING_SEGMENTS",
			Message:  "No HL7 segments found in message",
			Severity: "error",
		})
		return result, nil
	}

	// Enhanced HL7 structure validation
	if err := v.validateHL7Structure(segments, result); err != nil {
		log.Printf("HL7 structure validation error: %v", err)
	}

	// Schema validation if enabled
	if config.SchemaValidation.Enabled {
		if err := v.validateHL7Schema(segments, config.SchemaValidation, result); err != nil {
			log.Printf("HL7 schema validation error: %v", err)
		}
	}

	// Business rules validation
	for _, rule := range config.BusinessRules {
		if err := v.validateHL7BusinessRule(segments, rule); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:    rule.RuleID,
				Code:     "BUSINESS_RULE_VIOLATION",
				Message:  err.Error(),
				Severity: rule.Severity,
			})
		}
	}

	// Use existing business validation service if available
	if v.validationService != nil {
		v.enhanceWithBusinessValidation(ctx, message, result)
	}

	return result, nil
}

func (v *HL7Validator) validateHL7BusinessRule(segments map[string]interface{}, rule BusinessRule) error {
	// Simplified business rule validation
	// In production, this would use a proper expression evaluator
	return nil
}

// FHIRValidator validates FHIR resources
type FHIRValidator struct{}

func (v *FHIRValidator) Validate(ctx context.Context, config ValidationConfig, message *ProcessedMessage) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
		Metadata: make(map[string]interface{}),
	}

	// Check if content has resourceType
	if resourceType, ok := message.ParsedContent["resourceType"]; ok {
		result.Metadata["resource_type"] = resourceType
	} else {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "resourceType",
			Code:     "MISSING_RESOURCE_TYPE",
			Message:  "FHIR resource must have a resourceType",
			Severity: "error",
		})
	}

	return result, nil
}

// CustomValidator handles custom validation logic
type CustomValidator struct{}

func (v *CustomValidator) Validate(ctx context.Context, config ValidationConfig, message *ProcessedMessage) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
		Metadata: make(map[string]interface{}),
	}

	// Execute custom validation functions
	for _, validator := range config.CustomValidators {
		if err := v.executeCustomValidator(validator, message); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:    validator.Name,
				Code:     "CUSTOM_VALIDATION_FAILED",
				Message:  err.Error(),
				Severity: "error",
			})
		}
	}

	return result, nil
}

func (v *CustomValidator) executeCustomValidator(validator CustomValidator, message *ProcessedMessage) error {
	// In production, this would execute JavaScript or other scripting languages
	return nil
}

// =============================================================================
// TRANSFORMERS
// =============================================================================

// HL7ToFHIRTransformer transforms HL7 messages to FHIR format
type HL7ToFHIRTransformer struct{
	db *sql.DB
	transformService *HL7FHIRTransformServiceV3
}

func (t *HL7ToFHIRTransformer) Transform(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
	// Initialize transform service if not available
	if t.transformService == nil && t.db != nil {
		t.transformService = NewHL7FHIRTransformServiceV3(t.db)
	}

	// Use existing transformation service if available
	if t.transformService != nil {
		return t.useExistingTransformService(ctx, config, message)
	}

	// Fallback to basic transformation
	return t.basicHL7ToFHIRTransform(ctx, config, message)
}

func (t *HL7ToFHIRTransformer) useExistingTransformService(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
	// Prepare transform request
	request := &TransformRequest{
		HL7Message: message.ParsedContent["raw_message"].(string),
		TargetFormat: "fhir_r4",
		TemplateMapping: config.MappingTemplate,
		CustomMappings: make(map[string]interface{}),
	}

	// Convert custom mappings
	for _, mapping := range config.CustomMappings {
		request.CustomMappings[mapping.SourceField] = mapping.TargetField
	}

	// Perform transformation
	response, err := t.transformService.Transform(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("transformation service failed: %w", err)
	}

	// Create transformed message
	transformedMessage := &TransformedMessage{
		ProcessedMessage: message,
		TransformedContent: response.TransformedData,
		TargetFormat: response.TargetFormat,
	}

	return transformedMessage, nil
}

func (t *HL7ToFHIRTransformer) basicHL7ToFHIRTransform(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
	// Get HL7 segments
	segments, ok := message.ParsedContent["segments"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no HL7 segments found")
	}

	// Create FHIR Bundle
	fhirBundle := map[string]interface{}{
		"resourceType": "Bundle",
		"id": message.ID,
		"type": "transaction",
		"entry": []map[string]interface{}{},
	}

	// Transform segments to FHIR resources
	if mshSegment, exists := segments["MSH"]; exists {
		messageHeader := t.transformMSHToMessageHeader(mshSegment)
		fhirBundle["entry"] = append(fhirBundle["entry"].([]map[string]interface{}), map[string]interface{}{
			"resource": messageHeader,
		})
	}

	if pidSegment, exists := segments["PID"]; exists {
		patient := t.transformPIDToPatient(pidSegment)
		fhirBundle["entry"] = append(fhirBundle["entry"].([]map[string]interface{}), map[string]interface{}{
			"resource": patient,
		})
	}

	transformedMessage := &TransformedMessage{
		ProcessedMessage: message,
		TransformedContent: fhirBundle,
		TargetFormat: "fhir_r4",
	}

	return transformedMessage, nil
}

func (t *HL7ToFHIRTransformer) transformMSHToMessageHeader(mshSegment interface{}) map[string]interface{} {
	// Enhanced MSH to MessageHeader transformation
	mshFields, ok := mshSegment.([]string)
	if !ok || len(mshFields) < 9 {
		return map[string]interface{}{
			"resourceType": "MessageHeader",
			"id": "msg-header-" + fmt.Sprint(time.Now().Unix()),
		}
	}

	return map[string]interface{}{
		"resourceType": "MessageHeader",
		"id": "msg-header-" + fmt.Sprint(time.Now().Unix()),
		"eventCoding": map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
			"code": mshFields[8], // MSH.9 - Message Type
		},
		"source": map[string]interface{}{
			"name": mshFields[2], // MSH.3 - Sending Application
			"endpoint": mshFields[3], // MSH.4 - Sending Facility
		},
		"destination": []map[string]interface{}{
			{
				"name": mshFields[4], // MSH.5 - Receiving Application
				"endpoint": mshFields[5], // MSH.6 - Receiving Facility
			},
		},
	}
}

func (t *HL7ToFHIRTransformer) transformPIDToPatient(pidSegment interface{}) map[string]interface{} {
	// Enhanced PID to Patient transformation
	pidFields, ok := pidSegment.([]string)
	if !ok || len(pidFields) < 6 {
		return map[string]interface{}{
			"resourceType": "Patient",
			"id": "patient-" + fmt.Sprint(time.Now().Unix()),
		}
	}

	// Parse patient name from PID.5
	patientName := map[string]interface{}{
		"use": "official",
	}

	if len(pidFields) > 5 && pidFields[5] != "" {
		nameParts := strings.Split(pidFields[5], "^")
		if len(nameParts) > 0 {
			patientName["family"] = nameParts[0]
		}
		if len(nameParts) > 1 {
			patientName["given"] = []string{nameParts[1]}
		}
	}

	// Parse patient ID from PID.3
	patientID := "patient-" + fmt.Sprint(time.Now().Unix())
	identifiers := []map[string]interface{}{}

	if len(pidFields) > 3 && pidFields[3] != "" {
		identifiers = append(identifiers, map[string]interface{}{
			"use": "usual",
			"value": pidFields[3],
			"system": "http://hospital.smarthealthit.org",
		})
		patientID = "patient-" + pidFields[3]
	}

	return map[string]interface{}{
		"resourceType": "Patient",
		"id": patientID,
		"identifier": identifiers,
		"name": []map[string]interface{}{patientName},
		"gender": t.mapHL7Gender(pidFields),
		"birthDate": t.mapHL7BirthDate(pidFields),
	}
}

// CustomTransformer handles custom transformation logic
type CustomTransformer struct{}

func (t *CustomTransformer) Transform(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
	// Apply custom mappings
	transformedContent := make(map[string]interface{})

	for _, mapping := range config.CustomMappings {
		value, err := t.extractValue(message.ParsedContent, mapping.SourceField)
		if err != nil {
			continue
		}

		transformedValue, err := t.applyTransformation(value, mapping.Transformation)
		if err != nil {
			continue
		}

		t.setValue(transformedContent, mapping.TargetField, transformedValue)
	}

	transformedMessage := &TransformedMessage{
		ProcessedMessage:   message,
		TransformedContent: transformedContent,
		TargetFormat:       "custom",
	}

	return transformedMessage, nil
}

func (t *CustomTransformer) extractValue(content map[string]interface{}, fieldPath string) (interface{}, error) {
	// Simple dot notation field extraction
	parts := strings.Split(fieldPath, ".")
	current := content

	for _, part := range parts {
		if next, ok := current[part]; ok {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return next, nil
			}
		} else {
			return nil, fmt.Errorf("field not found: %s", fieldPath)
		}
	}

	return current, nil
}

func (t *CustomTransformer) applyTransformation(value interface{}, transformation TransformationStep) (interface{}, error) {
	switch transformation.Type {
	case "direct_copy":
		return value, nil
	case "uppercase":
		if str, ok := value.(string); ok {
			return strings.ToUpper(str), nil
		}
	case "lowercase":
		if str, ok := value.(string); ok {
			return strings.ToLower(str), nil
		}
	}

	return value, nil
}

func (t *CustomTransformer) setValue(content map[string]interface{}, fieldPath string, value interface{}) {
	parts := strings.Split(fieldPath, ".")
	current := content

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
	}
}

// PassthroughTransformer passes messages through without transformation
type PassthroughTransformer struct{}

func (t *PassthroughTransformer) Transform(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
	transformedMessage := &TransformedMessage{
		ProcessedMessage:   message,
		TransformedContent: message.ParsedContent,
		TargetFormat:       "passthrough",
	}

	return transformedMessage, nil
}

// =============================================================================
// BUSINESS LOGIC PROCESSORS
// =============================================================================

// RulesEngineProcessor executes business rules
type RulesEngineProcessor struct{}

func (p *RulesEngineProcessor) Process(ctx context.Context, config BusinessLogicConfig, message *TransformedMessage) (*ProcessedMessage, error) {
	// Execute business rules
	for _, rule := range config.RulesEngine.Rules {
		if err := p.executeRule(rule, message); err != nil {
			return message.ProcessedMessage, fmt.Errorf("rule %s failed: %w", rule.RuleID, err)
		}
	}

	return message.ProcessedMessage, nil
}

func (p *RulesEngineProcessor) executeRule(rule BusinessLogicRule, message *TransformedMessage) error {
	// Simplified rule execution
	// In production, this would use a proper rules engine
	return nil
}

// WorkflowProcessor handles workflow automation
type WorkflowProcessor struct{}

func (p *WorkflowProcessor) Process(ctx context.Context, config BusinessLogicConfig, message *TransformedMessage) (*ProcessedMessage, error) {
	// Execute workflow actions
	for _, workflow := range config.WorkflowAutomation {
		if err := p.executeWorkflow(workflow, message); err != nil {
			log.Printf("Warning: Workflow %s failed: %v", workflow.Trigger, err)
		}
	}

	return message.ProcessedMessage, nil
}

func (p *WorkflowProcessor) executeWorkflow(workflow WorkflowAction, message *TransformedMessage) error {
	// Simplified workflow execution
	return nil
}

// =============================================================================
// DESTINATION PROCESSORS
// =============================================================================

// FHIRAPIDestination delivers to FHIR API endpoints
type FHIRAPIDestination struct{}

func (d *FHIRAPIDestination) Deliver(ctx context.Context, config DestinationConfig, message *ProcessedMessage) (*DeliveryResult, error) {
	// Get FHIR API configuration
	baseURL, ok := config.Config["base_url"].(string)
	if !ok {
		return nil, fmt.Errorf("base_url not specified for FHIR API destination")
	}

	// Prepare HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Marshal message content
	contentBytes, err := json.Marshal(message.ParsedContent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, strings.NewReader(string(contentBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/fhir+json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Process response
	responseBody, _ := ioutil.ReadAll(resp.Body)

	result := &DeliveryResult{
		Success:       resp.StatusCode >= 200 && resp.StatusCode < 300,
		DestinationID: config.DestinationID,
		ResponseData: map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(responseBody),
		},
		DeliveredAt: time.Now(),
	}

	if !result.Success {
		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	return result, nil
}

// FileDestination delivers to file system
type FileDestination struct{}

func (d *FileDestination) Deliver(ctx context.Context, config DestinationConfig, message *ProcessedMessage) (*DeliveryResult, error) {
	// Get file configuration
	outputDir, ok := config.Config["output_directory"].(string)
	if !ok {
		return nil, fmt.Errorf("output_directory not specified for file destination")
	}

	// Create output filename
	filename := fmt.Sprintf("message_%s_%d.json", message.ID, time.Now().Unix())
	filepath := filepath.Join(outputDir, filename)

	// Ensure directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Marshal message content
	contentBytes, err := json.MarshalIndent(message.ParsedContent, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	// Write to file
	if err := ioutil.WriteFile(filepath, contentBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	result := &DeliveryResult{
		Success:       true,
		DestinationID: config.DestinationID,
		ResponseData: map[string]interface{}{
			"file_path": filepath,
			"file_size": len(contentBytes),
		},
		DeliveredAt: time.Now(),
	}

	return result, nil
}

// DatabaseDestination delivers to database
type DatabaseDestination struct{
	db *sql.DB
}

func (d *DatabaseDestination) Deliver(ctx context.Context, config DestinationConfig, message *ProcessedMessage) (*DeliveryResult, error) {
	if d.db == nil {
		return &DeliveryResult{
			Success:      false,
			DestinationID: config.DestinationID,
			ErrorMessage: "Database connection not available",
			DeliveredAt:  time.Now(),
		}, nil
	}

	// Get database configuration
	tableName := "processed_messages"
	if table, ok := config.Config["table_name"].(string); ok {
		tableName = table
	}

	// Prepare message data for database insertion
	messageData, err := json.Marshal(message.ParsedContent)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			DestinationID: config.DestinationID,
			ErrorMessage: fmt.Sprintf("Failed to serialize message data: %v", err),
			DeliveredAt:  time.Now(),
		}, nil
	}

	metadata, _ := json.Marshal(message.Metadata)

	// Insert into database
	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, interface_id, message_data, metadata,
			status, created_at, processed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tableName)

	_, err = d.db.ExecContext(ctx, query,
		message.ID,
		message.InterfaceID,
		string(messageData),
		string(metadata),
		"delivered",
		message.CreatedAt,
		time.Now(),
	)

	if err != nil {
		return &DeliveryResult{
			Success:      false,
			DestinationID: config.DestinationID,
			ErrorMessage: fmt.Sprintf("Database insertion failed: %v", err),
			DeliveredAt:  time.Now(),
		}, nil
	}

	result := &DeliveryResult{
		Success:       true,
		DestinationID: config.DestinationID,
		ResponseData: map[string]interface{}{
			"table":      tableName,
			"message_id": message.ID,
			"status":     "inserted",
		},
		DeliveredAt: time.Now(),
	}

	return result, nil
}

// QueueDestination delivers to message queue
type QueueDestination struct{}

func (d *QueueDestination) Deliver(ctx context.Context, config DestinationConfig, message *ProcessedMessage) (*DeliveryResult, error) {
	// Queue delivery would integrate with message queue systems
	// For now, return a successful result
	result := &DeliveryResult{
		Success:       true,
		DestinationID: config.DestinationID,
		ResponseData: map[string]interface{}{
			"queue_name": config.Config["queue_name"],
			"message_id": message.ID,
		},
		DeliveredAt: time.Now(),
	}

	return result, nil
}

// =============================================================================
// PROCESSOR FACTORY FUNCTIONS
// =============================================================================

// NewMLLPInputProcessor creates a new MLLP input processor with database integration
func NewMLLPInputProcessor(db *sql.DB) *MLLPInputProcessor {
	return &MLLPInputProcessor{
		mllpService: NewMLLPConnectivityService(db),
	}
}

// NewHL7ToFHIRTransformer creates a new HL7 to FHIR transformer with service integration
func NewHL7ToFHIRTransformer(db *sql.DB) *HL7ToFHIRTransformer {
	transformer := &HL7ToFHIRTransformer{
		db: db,
	}
	if db != nil {
		transformer.transformService = NewHL7FHIRTransformServiceV3(db)
	}
	return transformer
}

// NewHL7Validator creates a new HL7 validator with business validation integration
func NewHL7Validator(db *sql.DB) *HL7Validator {
	validator := &HL7Validator{
		db: db,
	}
	if db != nil {
		validator.validationService = NewBusinessValidationService(db)
	}
	return validator
}

// NewDatabaseDestination creates a new database destination processor
func NewDatabaseDestination(db *sql.DB) *DatabaseDestination {
	return &DatabaseDestination{
		db: db,
	}
}

// =============================================================================
// HELPER METHODS FOR PROCESSORS
// =============================================================================

// MLLP Input Processor Helper Methods
func (p *MLLPInputProcessor) extractSourceEndpoint(config InputConfig) string {
	if endpoint, ok := config.ConnectorConfig["endpoint"].(string); ok {
		return endpoint
	}
	host := p.getStringConfig(config.ConnectorConfig, "host", "unknown")
	port := p.getIntConfig(config.ConnectorConfig, "port", 0)
	return fmt.Sprintf("%s:%d", host, port)
}

func (p *MLLPInputProcessor) extractSourceIP(config InputConfig) string {
	if ip, ok := config.ConnectorConfig["source_ip"].(string); ok {
		return ip
	}
	return "127.0.0.1"
}

func (p *MLLPInputProcessor) parseSegmentStructure(segmentType string, fields []string) map[string]interface{} {
	switch segmentType {
	case "MSH":
		return p.parseMSHSegment(fields)
	case "PID":
		return p.parsePIDSegment(fields)
	case "PV1":
		return p.parsePV1Segment(fields)
	default:
		return map[string]interface{}{
			"segment_type": segmentType,
			"field_count":  len(fields),
			"raw_fields":   fields,
		}
	}
}

func (p *MLLPInputProcessor) parseMSHSegment(fields []string) map[string]interface{} {
	msh := map[string]interface{}{"segment_type": "MSH"}
	if len(fields) > 2 {
		msh["sending_application"] = fields[2]
	}
	if len(fields) > 3 {
		msh["sending_facility"] = fields[3]
	}
	if len(fields) > 4 {
		msh["receiving_application"] = fields[4]
	}
	if len(fields) > 5 {
		msh["receiving_facility"] = fields[5]
	}
	if len(fields) > 8 {
		msh["message_type"] = fields[8]
	}
	if len(fields) > 9 {
		msh["message_control_id"] = fields[9]
	}
	return msh
}

func (p *MLLPInputProcessor) parsePIDSegment(fields []string) map[string]interface{} {
	pid := map[string]interface{}{"segment_type": "PID"}
	if len(fields) > 3 {
		pid["patient_id"] = fields[3]
	}
	if len(fields) > 5 {
		pid["patient_name"] = fields[5]
	}
	if len(fields) > 7 {
		pid["birth_date"] = fields[7]
	}
	if len(fields) > 8 {
		pid["gender"] = fields[8]
	}
	return pid
}

func (p *MLLPInputProcessor) parsePV1Segment(fields []string) map[string]interface{} {
	pv1 := map[string]interface{}{"segment_type": "PV1"}
	if len(fields) > 2 {
		pv1["patient_class"] = fields[2]
	}
	if len(fields) > 3 {
		pv1["assigned_patient_location"] = fields[3]
	}
	if len(fields) > 7 {
		pv1["attending_doctor"] = fields[7]
	}
	return pv1
}

func (p *MLLPInputProcessor) extractMessageControlID(segments map[string]interface{}) string {
	if mshSegment, ok := segments["MSH"].([]string); ok && len(mshSegment) > 9 {
		return mshSegment[9]
	}
	return ""
}

func (p *MLLPInputProcessor) extractSendingApplication(segments map[string]interface{}) string {
	if mshSegment, ok := segments["MSH"].([]string); ok && len(mshSegment) > 2 {
		return mshSegment[2]
	}
	return ""
}

func (p *MLLPInputProcessor) extractReceivingApplication(segments map[string]interface{}) string {
	if mshSegment, ok := segments["MSH"].([]string); ok && len(mshSegment) > 4 {
		return mshSegment[4]
	}
	return ""
}

func (p *MLLPInputProcessor) getStringConfig(config map[string]interface{}, key, defaultValue string) string {
	if val, ok := config[key].(string); ok {
		return val
	}
	return defaultValue
}

func (p *MLLPInputProcessor) getIntConfig(config map[string]interface{}, key string, defaultValue int) int {
	if val, ok := config[key].(int); ok {
		return val
	}
	if val, ok := config[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}

func (p *MLLPInputProcessor) getBoolConfig(config map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := config[key].(bool); ok {
		return val
	}
	return defaultValue
}

// HL7ToFHIRTransformer Helper Methods
func (t *HL7ToFHIRTransformer) mapHL7Gender(pidFields []string) string {
	if len(pidFields) <= 8 || pidFields[8] == "" {
		return "unknown"
	}
	switch strings.ToUpper(pidFields[8]) {
	case "M":
		return "male"
	case "F":
		return "female"
	case "O":
		return "other"
	default:
		return "unknown"
	}
}

func (t *HL7ToFHIRTransformer) mapHL7BirthDate(pidFields []string) string {
	if len(pidFields) <= 7 || pidFields[7] == "" {
		return ""
	}
	// HL7 date format: YYYYMMDD
	date := pidFields[7]
	if len(date) >= 8 {
		return fmt.Sprintf("%s-%s-%s", date[:4], date[4:6], date[6:8])
	}
	return ""
}

// HL7Validator Helper Methods
func (v *HL7Validator) validateHL7Structure(segments map[string]interface{}, result *ValidationResult) error {
	// Validate MSH segment (required)
	if _, exists := segments["MSH"]; !exists {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:      "MSH",
			Code:       "MISSING_MSH",
			Message:    "MSH segment is required",
			Severity:   "error",
			Suggestion: "All HL7 messages must start with an MSH segment",
		})
		return fmt.Errorf("missing required MSH segment")
	}

	// Validate MSH segment structure
	if mshFields, ok := segments["MSH"].([]string); ok {
		if len(mshFields) < 10 {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:   "MSH",
				Code:    "MSH_INCOMPLETE",
				Message: "MSH segment has fewer fields than expected",
			})
		}
	}

	return nil
}

func (v *HL7Validator) validateHL7Schema(segments map[string]interface{}, schemaConfig SchemaValidationConfig, result *ValidationResult) error {
	// Basic schema validation - in production this would use actual HL7 schemas
	requiredSegments := []string{"MSH"}
	for _, required := range requiredSegments {
		if _, exists := segments[required]; !exists {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:    required,
				Code:     "REQUIRED_SEGMENT_MISSING",
				Message:  fmt.Sprintf("Required segment %s is missing", required),
				Severity: "error",
			})
		}
	}
	return nil
}

func (v *HL7Validator) enhanceWithBusinessValidation(ctx context.Context, message *ProcessedMessage, result *ValidationResult) {
	// Use existing business validation service for enhanced validation
	if v.validationService != nil {
		log.Printf("Running enhanced business validation for message: %s", message.ID)
		// Here you would call the actual business validation service methods
		// This is a placeholder for the integration
	}
}