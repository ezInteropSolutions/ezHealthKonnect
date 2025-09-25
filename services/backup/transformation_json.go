// services/transformation_json.go
// JSON Transformation Service for Universal Interface Engine
//
// 🎯 PURPOSE: Comprehensive JSON message transformation, validation, and mapping
// Supports JSON-to-JSON, JSON-to-FHIR, JSON-to-HL7, and custom mapping rules
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// JSON TRANSFORMATION SERVICE
// =====================================

// JSONTransformationService handles JSON message processing
type JSONTransformationService struct {
	db             *sql.DB
	schemaRegistry *JSONSchemaRegistry
	mappingEngine  *JSONMappingEngine
	validator      *JSONValidator
	templateEngine *JSONTemplateEngine
	performanceMetrics JSONPerformanceMetrics
}

// JSONPerformanceMetrics tracks service performance
type JSONPerformanceMetrics struct {
	MessagesProcessed    int64         `json:"messagesProcessed"`
	AverageTransformTime time.Duration `json:"averageTransformTime"`
	AverageValidateTime  time.Duration `json:"averageValidateTime"`
	SchemaValidations    int64         `json:"schemaValidations"`
	MappingOperations    int64         `json:"mappingOperations"`
	ErrorRate            float64       `json:"errorRate"`
	ThroughputPerSecond  float64       `json:"throughputPerSecond"`
}

// JSONTransformRequest defines JSON transformation request
type JSONTransformRequest struct {
	MessageID         string                 `json:"messageId"`
	SourceJSON        map[string]interface{} `json:"sourceJson"`
	TargetFormat      MessageType            `json:"targetFormat"`         // JSON, FHIR, HL7, XML
	TargetSchema      string                 `json:"targetSchema,omitempty"` // Schema ID or URL
	MappingRules      []JSONMappingRule      `json:"mappingRules,omitempty"`
	ValidationLevel   string                 `json:"validationLevel,omitempty"` // STRICT, MODERATE, LENIENT, NONE
	PreserveStructure bool                   `json:"preserveStructure"`
	FlattenArrays     bool                   `json:"flattenArrays"`
	IncludeMetadata   bool                   `json:"includeMetadata"`
	CustomTransforms  map[string]interface{} `json:"customTransforms,omitempty"`
	TransformationOptions map[string]interface{} `json:"transformationOptions,omitempty"`
}

// JSONTransformResponse defines JSON transformation response
type JSONTransformResponse struct {
	Success              bool                   `json:"success"`
	MessageID            string                 `json:"messageId"`
	SourceFormat         MessageType            `json:"sourceFormat"`
	TargetFormat         MessageType            `json:"targetFormat"`
	TransformedData      map[string]interface{} `json:"transformedData"`
	ValidationResults    []JSONValidationResult `json:"validationResults"`
	TransformationSteps  []TransformationStep   `json:"transformationSteps"`
	MappingStatistics    JSONMappingStatistics  `json:"mappingStatistics"`
	ProcessingMetrics    ProcessingMetrics      `json:"processingMetrics"`
	Warnings             []string               `json:"warnings"`
	Errors               []string               `json:"errors"`
	QualityScore         float64                `json:"qualityScore"`
	RecommendedActions   []string               `json:"recommendedActions"`
}

// JSONMappingRule defines transformation mapping rules
type JSONMappingRule struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	SourcePath       string                 `json:"sourcePath"`       // JSONPath expression
	TargetPath       string                 `json:"targetPath"`       // Target path
	DataType         string                 `json:"dataType"`         // string, number, boolean, array, object
	Transform        string                 `json:"transform,omitempty"` // Transformation function
	DefaultValue     interface{}            `json:"defaultValue,omitempty"`
	Condition        string                 `json:"condition,omitempty"`        // Condition for applying rule
	ValidationRules  []JSONValidationRule   `json:"validationRules,omitempty"`
	Priority         int                    `json:"priority"`
	Enabled          bool                   `json:"enabled"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// JSONValidationRule defines validation criteria
type JSONValidationRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`        // required, format, range, pattern, custom
	Severity    string                 `json:"severity"`    // error, warning, info
	Rule        string                 `json:"rule"`        // Rule expression
	Message     string                 `json:"message"`     // Validation message
	Enabled     bool                   `json:"enabled"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// JSONValidationResult contains validation outcome
type JSONValidationResult struct {
	RuleID      string                 `json:"ruleId"`
	RuleName    string                 `json:"ruleName"`
	Path        string                 `json:"path"`
	Severity    string                 `json:"severity"`
	Passed      bool                   `json:"passed"`
	Message     string                 `json:"message"`
	Value       interface{}            `json:"value,omitempty"`
	Expected    interface{}            `json:"expected,omitempty"`
	Suggestion  string                 `json:"suggestion,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// JSONMappingStatistics contains mapping performance data
type JSONMappingStatistics struct {
	TotalMappings         int     `json:"totalMappings"`
	SuccessfulMappings    int     `json:"successfulMappings"`
	FailedMappings        int     `json:"failedMappings"`
	ConditionalMappings   int     `json:"conditionalMappings"`
	DefaultValuesApplied  int     `json:"defaultValuesApplied"`
	TransformationsApplied int    `json:"transformationsApplied"`
	DataTypeConversions   int     `json:"dataTypeConversions"`
	MappingSuccessRate    float64 `json:"mappingSuccessRate"`
}

// =====================================
// JSON SCHEMA REGISTRY
// =====================================

// JSONSchemaRegistry manages JSON schemas for validation
type JSONSchemaRegistry struct {
	schemas         map[string]*JSONSchema
	schemaCache     map[string]*CompiledSchema
	defaultSchemas  map[MessageType]*JSONSchema
}

// JSONSchema represents a JSON schema definition
type JSONSchema struct {
	ID          string                 `json:"id"`
	Schema      string                 `json:"$schema"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Properties  map[string]*JSONProperty `json:"properties"`
	Required    []string               `json:"required"`
	Definitions map[string]*JSONSchema `json:"definitions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	LoadedAt    time.Time              `json:"loadedAt"`
}

// JSONProperty represents a property in JSON schema
type JSONProperty struct {
	Type        interface{}            `json:"type"`         // string, array, or []string for multiple types
	Format      string                 `json:"format,omitempty"`
	Pattern     string                 `json:"pattern,omitempty"`
	MinLength   *int                   `json:"minLength,omitempty"`
	MaxLength   *int                   `json:"maxLength,omitempty"`
	Minimum     *float64               `json:"minimum,omitempty"`
	Maximum     *float64               `json:"maximum,omitempty"`
	Enum        []interface{}          `json:"enum,omitempty"`
	Items       *JSONProperty          `json:"items,omitempty"`
	Properties  map[string]*JSONProperty `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Description string                 `json:"description,omitempty"`
	Default     interface{}            `json:"default,omitempty"`
	Examples    []interface{}          `json:"examples,omitempty"`
}

// CompiledSchema represents a compiled JSON schema for fast validation
type CompiledSchema struct {
	ID            string
	Schema        *JSONSchema
	RequiredPaths []string
	PatternCache  map[string]*regexp.Regexp
	CompiledAt    time.Time
}

// =====================================
// JSON MAPPING ENGINE
// =====================================

// JSONMappingEngine handles JSON transformation logic
type JSONMappingEngine struct {
	mappingRules     map[string][]JSONMappingRule
	transformFunctions map[string]TransformFunction
	conditionEvaluator *ConditionEvaluator
	pathResolver      *JSONPathResolver
}

// TransformFunction defines a transformation function
type TransformFunction func(interface{}, map[string]interface{}) (interface{}, error)

// ConditionEvaluator evaluates conditional mapping rules
type ConditionEvaluator struct {
	operators map[string]OperatorFunction
}

// OperatorFunction defines condition evaluation operators
type OperatorFunction func(interface{}, interface{}) bool

// JSONPathResolver resolves JSONPath expressions
type JSONPathResolver struct {
	pathCache map[string][]string
}

// =====================================
// JSON VALIDATOR
// =====================================

// JSONValidator handles JSON validation
type JSONValidator struct {
	schemaRegistry *JSONSchemaRegistry
	customValidators map[string]CustomValidator
}

// CustomValidator defines custom validation functions
type CustomValidator func(interface{}, JSONValidationRule) (bool, string)

// =====================================
// JSON TEMPLATE ENGINE
// =====================================

// JSONTemplateEngine handles template-based transformations
type JSONTemplateEngine struct {
	templates     map[string]*JSONTemplate
	templateCache map[string]*CompiledTemplate
	functions     map[string]TemplateFunction
}

// JSONTemplate represents a transformation template
type JSONTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	SourceType  MessageType            `json:"sourceType"`
	TargetType  MessageType            `json:"targetType"`
	Template    map[string]interface{} `json:"template"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	Functions   []string               `json:"functions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}

// CompiledTemplate represents a compiled transformation template
type CompiledTemplate struct {
	ID           string
	Template     *JSONTemplate
	Instructions []TemplateInstruction
	CompiledAt   time.Time
}

// TemplateInstruction represents a single template operation
type TemplateInstruction struct {
	Operation string                 `json:"operation"` // copy, transform, condition, loop
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Function  string                 `json:"function,omitempty"`
	Condition string                 `json:"condition,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TemplateFunction defines template functions
type TemplateFunction func(interface{}, map[string]interface{}) (interface{}, error)

// =====================================
// SERVICE CONSTRUCTOR AND INITIALIZATION
// =====================================

// NewJSONTransformationService creates a new JSON transformation service
func NewJSONTransformationService(database *sql.DB) *JSONTransformationService {
	service := &JSONTransformationService{
		db:             database,
		schemaRegistry: NewJSONSchemaRegistry(),
		mappingEngine:  NewJSONMappingEngine(),
		validator:      NewJSONValidator(),
		templateEngine: NewJSONTemplateEngine(),
		performanceMetrics: JSONPerformanceMetrics{},
	}

	// Initialize default schemas and mappings
	if err := service.initializeDefaultConfiguration(); err != nil {
		log.Printf("⚠️ Warning: Failed to initialize JSON transformation defaults: %v", err)
	}

	log.Printf("✅ JSONTransformationService initialized")
	return service
}

// NewJSONSchemaRegistry creates a new schema registry
func NewJSONSchemaRegistry() *JSONSchemaRegistry {
	return &JSONSchemaRegistry{
		schemas:        make(map[string]*JSONSchema),
		schemaCache:    make(map[string]*CompiledSchema),
		defaultSchemas: make(map[MessageType]*JSONSchema),
	}
}

// NewJSONMappingEngine creates a new mapping engine
func NewJSONMappingEngine() *JSONMappingEngine {
	engine := &JSONMappingEngine{
		mappingRules:       make(map[string][]JSONMappingRule),
		transformFunctions: make(map[string]TransformFunction),
		conditionEvaluator: &ConditionEvaluator{
			operators: make(map[string]OperatorFunction),
		},
		pathResolver: &JSONPathResolver{
			pathCache: make(map[string][]string),
		},
	}

	engine.initializeTransformFunctions()
	engine.initializeOperators()
	return engine
}

// NewJSONValidator creates a new JSON validator
func NewJSONValidator() *JSONValidator {
	validator := &JSONValidator{
		customValidators: make(map[string]CustomValidator),
	}

	validator.initializeCustomValidators()
	return validator
}

// NewJSONTemplateEngine creates a new template engine
func NewJSONTemplateEngine() *JSONTemplateEngine {
	engine := &JSONTemplateEngine{
		templates:     make(map[string]*JSONTemplate),
		templateCache: make(map[string]*CompiledTemplate),
		functions:     make(map[string]TemplateFunction),
	}

	engine.initializeTemplateFunctions()
	return engine
}

// initializeDefaultConfiguration sets up default schemas and mappings
func (s *JSONTransformationService) initializeDefaultConfiguration() error {
	// Initialize default schemas
	if err := s.initializeDefaultSchemas(); err != nil {
		return fmt.Errorf("failed to initialize default schemas: %v", err)
	}

	// Initialize default mapping rules
	if err := s.initializeDefaultMappingRules(); err != nil {
		return fmt.Errorf("failed to initialize default mapping rules: %v", err)
	}

	// Initialize default templates
	if err := s.initializeDefaultTemplates(); err != nil {
		return fmt.Errorf("failed to initialize default templates: %v", err)
	}

	log.Printf("✅ Initialized JSON transformation defaults")
	return nil
}

// initializeDefaultSchemas sets up default JSON schemas
func (s *JSONTransformationService) initializeDefaultSchemas() error {
	// Generic JSON schema
	genericSchema := &JSONSchema{
		ID:          "generic-json-v1",
		Schema:      "http://json-schema.org/draft-07/schema#",
		Title:       "Generic JSON Schema",
		Description: "Generic schema for JSON message validation",
		Type:        "object",
		Properties:  make(map[string]*JSONProperty),
		Required:    []string{},
		LoadedAt:    time.Now(),
	}

	// FHIR-compatible JSON schema
	fhirSchema := &JSONSchema{
		ID:          "fhir-json-v1",
		Schema:      "http://json-schema.org/draft-07/schema#",
		Title:       "FHIR JSON Schema",
		Description: "Schema for FHIR-compatible JSON messages",
		Type:        "object",
		Properties: map[string]*JSONProperty{
			"resourceType": {
				Type:        "string",
				Description: "FHIR resource type",
				Pattern:     "^[A-Z][a-zA-Z0-9]*$",
			},
			"id": {
				Type:        "string",
				Description: "Resource identifier",
				Pattern:     "^[A-Za-z0-9\\-\\.]{1,64}$",
			},
			"meta": {
				Type:        "object",
				Description: "Resource metadata",
				Properties: map[string]*JSONProperty{
					"versionId": {Type: "string"},
					"lastUpdated": {Type: "string", Format: "date-time"},
					"profile": {Type: "array", Items: &JSONProperty{Type: "string"}},
				},
			},
		},
		Required: []string{"resourceType"},
		LoadedAt: time.Now(),
	}

	// HL7-compatible JSON schema
	hl7Schema := &JSONSchema{
		ID:          "hl7-json-v1",
		Schema:      "http://json-schema.org/draft-07/schema#",
		Title:       "HL7 JSON Schema",
		Description: "Schema for HL7-compatible JSON messages",
		Type:        "object",
		Properties: map[string]*JSONProperty{
			"messageHeader": {
				Type:        "object",
				Description: "HL7 message header information",
				Required:    []string{"messageType", "sendingApplication"},
			},
			"segments": {
				Type:        "array",
				Description: "HL7 message segments",
				Items: &JSONProperty{
					Type: "object",
					Properties: map[string]*JSONProperty{
						"name": {Type: "string"},
						"fields": {Type: "array"},
					},
				},
			},
		},
		Required: []string{"messageHeader"},
		LoadedAt: time.Now(),
	}

	// Register schemas
	s.schemaRegistry.schemas[genericSchema.ID] = genericSchema
	s.schemaRegistry.schemas[fhirSchema.ID] = fhirSchema
	s.schemaRegistry.schemas[hl7Schema.ID] = hl7Schema

	// Set default schemas
	s.schemaRegistry.defaultSchemas[MessageTypeJSON] = genericSchema
	s.schemaRegistry.defaultSchemas[MessageTypeFHIR] = fhirSchema
	s.schemaRegistry.defaultSchemas[MessageTypeHL7] = hl7Schema

	return nil
}

// initializeDefaultMappingRules sets up default mapping rules
func (s *JSONTransformationService) initializeDefaultMappingRules() error {
	// JSON to FHIR mapping rules
	jsonToFhirRules := []JSONMappingRule{
		{
			ID:         "json-to-fhir-resourcetype",
			Name:       "Set FHIR ResourceType",
			SourcePath: "$.type",
			TargetPath: "$.resourceType",
			DataType:   "string",
			Transform:  "capitalize",
			Priority:   1,
			Enabled:    true,
		},
		{
			ID:         "json-to-fhir-id",
			Name:       "Map ID Field",
			SourcePath: "$.id",
			TargetPath: "$.id",
			DataType:   "string",
			Transform:  "sanitize_fhir_id",
			Priority:   2,
			Enabled:    true,
		},
		{
			ID:         "json-to-fhir-timestamp",
			Name:       "Map Timestamp",
			SourcePath: "$.timestamp",
			TargetPath: "$.meta.lastUpdated",
			DataType:   "string",
			Transform:  "iso_datetime",
			Priority:   3,
			Enabled:    true,
		},
	}

	// JSON to HL7 mapping rules
	jsonToHl7Rules := []JSONMappingRule{
		{
			ID:         "json-to-hl7-message-type",
			Name:       "Map Message Type",
			SourcePath: "$.messageType",
			TargetPath: "$.messageHeader.messageType.messageCode",
			DataType:   "string",
			Transform:  "uppercase",
			Priority:   1,
			Enabled:    true,
		},
		{
			ID:         "json-to-hl7-source",
			Name:       "Map Source Application",
			SourcePath: "$.source",
			TargetPath: "$.messageHeader.sendingApplication",
			DataType:   "string",
			Priority:   2,
			Enabled:    true,
		},
	}

	// Store mapping rules
	s.mappingEngine.mappingRules["JSON_TO_FHIR"] = jsonToFhirRules
	s.mappingEngine.mappingRules["JSON_TO_HL7"] = jsonToHl7Rules

	return nil
}

// initializeDefaultTemplates sets up default transformation templates
func (s *JSONTransformationService) initializeDefaultTemplates() error {
	// Generic JSON to FHIR template
	fhirTemplate := &JSONTemplate{
		ID:          "json-to-fhir-generic",
		Name:        "Generic JSON to FHIR",
		Description: "Generic transformation from JSON to FHIR format",
		SourceType:  MessageTypeJSON,
		TargetType:  MessageTypeFHIR,
		Template: map[string]interface{}{
			"resourceType": "{{.type | capitalize}}",
			"id":          "{{.id | sanitize_fhir_id}}",
			"meta": map[string]interface{}{
				"lastUpdated": "{{.timestamp | iso_datetime}}",
				"source":      "{{.source | default \"Unknown\"}}",
			},
			"text": map[string]interface{}{
				"status": "generated",
				"div":    "<div xmlns=\"http://www.w3.org/1999/xhtml\">{{.description | default \"Generated from JSON\"}}</div>",
			},
		},
		Variables: map[string]interface{}{
			"defaultSource": "JSON Transformation Service",
		},
		Functions: []string{"capitalize", "sanitize_fhir_id", "iso_datetime", "default"},
		CreatedAt: time.Now(),
	}

	// Generic JSON to HL7 template
	hl7Template := &JSONTemplate{
		ID:          "json-to-hl7-generic",
		Name:        "Generic JSON to HL7",
		Description: "Generic transformation from JSON to HL7 format",
		SourceType:  MessageTypeJSON,
		TargetType:  MessageTypeHL7,
		Template: map[string]interface{}{
			"messageHeader": map[string]interface{}{
				"sendingApplication":   "{{.source | default \"JSON_APP\"}}",
				"sendingFacility":     "{{.facility | default \"JSON_FACILITY\"}}",
				"receivingApplication": "{{.target | default \"TARGET_APP\"}}",
				"messageType": map[string]interface{}{
					"messageCode":      "{{.messageType | uppercase}}",
					"triggerEvent":     "{{.triggerEvent | default \"Z01\"}}",
					"messageStructure": "{{.messageType | uppercase}}_{{.triggerEvent | default \"Z01\"}}",
				},
				"messageControlID": "{{.id | default (uuid)}}",
				"messageDateTime":  "{{.timestamp | hl7_timestamp}}",
				"versionID":        "2.5",
			},
		},
		Functions: []string{"uppercase", "default", "uuid", "hl7_timestamp"},
		CreatedAt: time.Now(),
	}

	// Store templates
	s.templateEngine.templates[fhirTemplate.ID] = fhirTemplate
	s.templateEngine.templates[hl7Template.ID] = hl7Template

	return nil
}

// =====================================
// MAIN TRANSFORMATION INTERFACE
// =====================================

// Transform transforms a UniversalMessage containing JSON content
func (s *JSONTransformationService) Transform(ctx context.Context, message *UniversalMessage) error {
	// Start transformation tracking
	transformRecord := message.StartTransformation("JSONTransformationService", MessageTypeJSON, MessageTypeJSON)

	startTime := time.Now()
	var outputSize int64 = 0
	var transformError error

	defer func() {
		message.CompleteTransformation(transformError == nil, outputSize, func() string {
			if transformError != nil {
				return transformError.Error()
			}
			return ""
		}())
	}()

	// Update message status
	message.UpdateStatus(StatusTransforming, "JSONTransformationService", "Starting JSON transformation")

	// Parse source JSON if not already parsed
	var sourceJSON map[string]interface{}
	if message.ParsedContent != nil {
		sourceJSON = message.ParsedContent
	} else {
		if err := json.Unmarshal(message.RawContent, &sourceJSON); err != nil {
			transformError = fmt.Errorf("failed to parse JSON content: %v", err)
			message.AddError("PARSING", "JSONTransformationService", "JSON_PARSE_ERROR",
				"Failed to parse JSON content", transformError.Error(), true)
			return transformError
		}
		message.ParsedContent = sourceJSON
	}

	// Create transformation request
	request := &JSONTransformRequest{
		MessageID:         message.ID,
		SourceJSON:        sourceJSON,
		TargetFormat:      MessageTypeJSON, // Default to JSON-to-JSON
		ValidationLevel:   "MODERATE",
		PreserveStructure: true,
		IncludeMetadata:   true,
		TransformationOptions: map[string]interface{}{
			"validateInput":   true,
			"validateOutput":  true,
			"preserveTypes":   true,
			"generateMeta":    true,
		},
	}

	// Detect target format from metadata or source structure
	if targetFormat := s.detectTargetFormat(sourceJSON, message); targetFormat != "" {
		request.TargetFormat = MessageType(targetFormat)
	}

	// Perform transformation
	response, err := s.TransformJSON(ctx, request)
	if err != nil {
		transformError = err
		message.AddError("TRANSFORMATION", "JSONTransformationService", "JSON_TRANSFORM_FAILED",
			"Failed to transform JSON message", err.Error(), true)
		return err
	}

	if !response.Success {
		transformError = fmt.Errorf("JSON transformation failed: %v", response.Errors)
		for _, error := range response.Errors {
			message.AddError("TRANSFORMATION", "JSONTransformationService", "JSON_TRANSFORMATION_ERROR",
				"JSON transformation error", error, false)
		}
		return transformError
	}

	// Store transformed content
	message.ParsedContent = response.TransformedData

	// Add transformed content
	outputBytes, _ := json.Marshal(response.TransformedData)
	outputSize = int64(len(outputBytes))
	message.AddTransformedContent(response.TargetFormat, outputBytes, transformRecord.ID)

	// Add validation warnings
	for _, warning := range response.Warnings {
		message.AddWarning("VALIDATION", "JSONTransformationService", "JSON_VALIDATION_WARNING",
			warning, "Data quality impact")
	}

	// Update status to transformed
	message.UpdateStatus(StatusTransformed, "JSONTransformationService",
		fmt.Sprintf("JSON transformation completed (Format: %s, Quality: %.2f)",
			response.TargetFormat, response.QualityScore))

	// Update transformation metadata
	if transformRecord != nil {
		transformRecord.Metadata["targetFormat"] = response.TargetFormat
		transformRecord.Metadata["mappingStatistics"] = response.MappingStatistics
		transformRecord.Metadata["qualityScore"] = response.QualityScore
		transformRecord.Metadata["validationResults"] = len(response.ValidationResults)
	}

	log.Printf("✅ JSON transformation completed for message %s (Format: %s, Duration: %v)",
		message.ID, response.TargetFormat, time.Since(startTime))

	return nil
}

// =====================================
// CORE JSON TRANSFORMATION LOGIC
// =====================================

// TransformJSON performs complete JSON transformation
func (s *JSONTransformationService) TransformJSON(ctx context.Context, request *JSONTransformRequest) (*JSONTransformResponse, error) {
	startTime := time.Now()

	response := &JSONTransformResponse{
		Success:              false,
		MessageID:            request.MessageID,
		SourceFormat:         MessageTypeJSON,
		TargetFormat:         request.TargetFormat,
		ValidationResults:    []JSONValidationResult{},
		TransformationSteps:  []TransformationStep{},
		Warnings:             []string{},
		Errors:               []string{},
		ProcessingMetrics:    ProcessingMetrics{},
	}

	// Step 1: Validate source JSON (if requested)
	if request.ValidationLevel != "NONE" {
		validateStep := s.startTransformationStep("VALIDATE_SOURCE", "JSON", "VALIDATED")
		validationResults, validateErr := s.validateSourceJSON(request.SourceJSON, request.ValidationLevel)
		s.completeTransformationStep(&validateStep, validateErr, 1, len(validationResults))
		response.TransformationSteps = append(response.TransformationSteps, validateStep)

		response.ValidationResults = append(response.ValidationResults, validationResults...)

		// Check for validation errors
		errorCount := s.countValidationErrors(validationResults)
		if request.ValidationLevel == "STRICT" && errorCount > 0 {
			response.Errors = append(response.Errors, fmt.Sprintf("Source validation failed with %d errors", errorCount))
			return response, fmt.Errorf("strict validation failed")
		}
	}

	// Step 2: Apply transformation based on target format
	transformStep := s.startTransformationStep("TRANSFORM_JSON", "JSON", string(request.TargetFormat))
	transformedData, mappingStats, transformErr := s.transformJSONData(request)
	outputSize := 0
	if transformedData != nil {
		if outputBytes, err := json.Marshal(transformedData); err == nil {
			outputSize = len(outputBytes)
		}
	}
	s.completeTransformationStep(&transformStep, transformErr, 1, outputSize)
	response.TransformationSteps = append(response.TransformationSteps, transformStep)

	if transformErr != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Transformation error: %v", transformErr))
		return response, transformErr
	}

	response.TransformedData = transformedData
	response.MappingStatistics = mappingStats

	// Step 3: Validate transformed data (if requested)
	if request.ValidationLevel != "NONE" && request.TargetSchema != "" {
		validateTargetStep := s.startTransformationStep("VALIDATE_TARGET", string(request.TargetFormat), "VALIDATED")
		targetValidationResults, validateTargetErr := s.validateTransformedData(transformedData, request.TargetSchema, request.ValidationLevel)
		s.completeTransformationStep(&validateTargetStep, validateTargetErr, 1, len(targetValidationResults))
		response.TransformationSteps = append(response.TransformationSteps, validateTargetStep)

		response.ValidationResults = append(response.ValidationResults, targetValidationResults...)
	}

	// Calculate quality score
	response.QualityScore = s.calculateQualityScore(response.ValidationResults, mappingStats)

	// Add warnings for validation issues
	for _, result := range response.ValidationResults {
		if !result.Passed {
			if result.Severity == "warning" {
				response.Warnings = append(response.Warnings, result.Message)
			} else if result.Severity == "error" {
				response.Errors = append(response.Errors, result.Message)
			}
		}
	}

	response.Success = len(response.Errors) == 0

	// Calculate processing metrics
	response.ProcessingMetrics = ProcessingMetrics{
		TotalTime:     time.Since(startTime),
		TransformTime: transformStep.Duration,
		ValidationTime: func() time.Duration {
			var total time.Duration
			for _, step := range response.TransformationSteps {
				if strings.Contains(step.StepName, "VALIDATE") {
					total += step.Duration
				}
			}
			return total
		}(),
		MemoryUsage: int64(outputSize),
	}

	// Update service metrics
	s.updatePerformanceMetrics(response.ProcessingMetrics)

	log.Printf("✅ JSON transformation completed (Message: %s, Target: %s, Quality: %.2f%%)",
		response.MessageID, response.TargetFormat, response.QualityScore)

	return response, nil
}

// Continue with remaining JSON transformation methods in next part...