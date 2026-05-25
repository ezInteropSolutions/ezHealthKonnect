// fhir/transformation_engine.go
// Schema-driven FHIR transformation engine leveraging existing FHIR schemas
package fhir

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// =====================================
// TRANSFORMATION ENGINE CORE
// =====================================

type TransformationEngine struct {
	db           *sql.DB
	schemaLoader *FHIRSchemaLoader
	config       *TransformationConfig
}

type TransformationConfig struct {
	DefaultProfile    string
	ValidateOutput    bool
	CreateBundle      bool
	MaxProcessingTime time.Duration
	VerboseLogging    bool
}

// =====================================
// TRANSFORMATION REQUEST/RESPONSE
// =====================================

type TransformationRequest struct {
	HL7Message     map[string]interface{} `json:"hl7Message"`     // Enhanced HL7 JSON
	TargetProfile  string                 `json:"targetProfile"`  // us-core, base, etc.
	FHIRVersion    string                 `json:"fhirVersion"`    // R4, R5
	CreateBundle   bool                   `json:"createBundle"`   // Bundle resources
	ValidationMode string                 `json:"validationMode"` // strict, lenient, none
	CustomMappings map[string]interface{} `json:"customMappings"` // Override rules
	RequestID      string                 `json:"requestId"`
	SourceSystem   string                 `json:"sourceSystem"`
}

type TransformationResponse struct {
	Success          bool                     `json:"success"`
	RequestID        string                   `json:"requestId"`
	FHIRResources    []map[string]interface{} `json:"fhirResources"`
	Bundle           map[string]interface{}   `json:"bundle,omitempty"`
	ValidationErrors []ValidationError        `json:"validationErrors,omitempty"`
	Warnings         []TransformWarning       `json:"warnings,omitempty"`
	Metadata         TransformationMetadata   `json:"metadata"`
	Performance      PerformanceMetrics       `json:"performance"`
}

type ValidationError struct {
	ResourceType string `json:"resourceType"`
	Path         string `json:"path"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	Severity     string `json:"severity"`
}

type TransformWarning struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HL7Path    string `json:"hl7Path,omitempty"`
	FHIRPath   string `json:"fhirPath,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type TransformationMetadata struct {
	ProcessedAt      time.Time `json:"processedAt"`
	HL7MessageType   string    `json:"hl7MessageType"`
	FHIRVersion      string    `json:"fhirVersion"`
	Profile          string    `json:"profile"`
	ResourcesCreated int       `json:"resourcesCreated"`
	RulesApplied     int       `json:"rulesApplied"`
	SchemaUsed       bool      `json:"schemaUsed"`
}

type PerformanceMetrics struct {
	TotalTime      time.Duration `json:"totalTime"`
	SchemaLoadTime time.Duration `json:"schemaLoadTime"`
	TransformTime  time.Duration `json:"transformTime"`
	ValidationTime time.Duration `json:"validationTime"`
	RulesProcessed int           `json:"rulesProcessed"`
	MemoryUsed     int64         `json:"memoryUsed"`
}

// =====================================
// MAPPING RULE STRUCTURES
// =====================================

type MappingRule struct {
	ID              int                    `json:"id"`
	HL7MessageType  string                 `json:"hl7MessageType"`
	HL7Segment      string                 `json:"hl7Segment"`
	HL7Field        string                 `json:"hl7Field"`
	HL7Component    string                 `json:"hl7Component,omitempty"`
	FHIRResource    string                 `json:"fhirResource"`
	FHIRProfile     string                 `json:"fhirProfile"`
	FHIRPath        string                 `json:"fhirPath"`
	TransformLogic  map[string]interface{} `json:"transformLogic"`
	Condition       string                 `json:"condition,omitempty"`
	Priority        int                    `json:"priority"`
	Required        bool                   `json:"required"`
	SchemaValidated bool                   `json:"schemaValidated"`
}

// =====================================
// ENGINE INITIALIZATION
// =====================================

func NewTransformationEngine(db *sql.DB, config *TransformationConfig) *TransformationEngine {
	if config == nil {
		config = &TransformationConfig{
			DefaultProfile:    "base",
			ValidateOutput:    true,
			CreateBundle:      false,
			MaxProcessingTime: 30 * time.Second,
			VerboseLogging:    false,
		}
	}

	return &TransformationEngine{
		db:           db,
		schemaLoader: GetFHIRSchemaLoader(),
		config:       config,
	}
}

// =====================================
// MAIN TRANSFORMATION PIPELINE
// =====================================

func (te *TransformationEngine) Transform(ctx context.Context, request *TransformationRequest) (*TransformationResponse, error) {
	startTime := time.Now()

	if te.config.VerboseLogging {
		log.Printf("🔄 Starting FHIR transformation: %s", request.RequestID)
	}

	response := &TransformationResponse{
		RequestID:        request.RequestID,
		FHIRResources:    []map[string]interface{}{},
		ValidationErrors: []ValidationError{},
		Warnings:         []TransformWarning{},
		Metadata: TransformationMetadata{
			ProcessedAt: time.Now(),
			FHIRVersion: getVersionOrDefault(request.FHIRVersion, "R4"),
			Profile:     getProfileOrDefault(request.TargetProfile, te.config.DefaultProfile),
			SchemaUsed:  false,
		},
		Performance: PerformanceMetrics{},
	}

	// Extract HL7 message info
	messageType, err := te.extractMessageType(request.HL7Message)
	if err != nil {
		return te.errorResponse(response, "INVALID_HL7_MESSAGE", err.Error()), nil
	}
	response.Metadata.HL7MessageType = messageType

	// Step 1: Load mapping rules from database
	rules, err := te.loadMappingRules(ctx, messageType, response.Metadata.Profile)
	if err != nil {
		return te.errorResponse(response, "RULE_LOADING_FAILED", err.Error()), nil
	}
	response.Performance.RulesProcessed = len(rules)

	if te.config.VerboseLogging {
		log.Printf("📋 Loaded %d mapping rules for %s -> %s", len(rules), messageType, response.Metadata.Profile)
	}

	// Step 2: Load FHIR schemas for validation and structure
	schemaStartTime := time.Now()
	resourceSchemas, err := te.loadRequiredSchemas(ctx, rules, response.Metadata.FHIRVersion, response.Metadata.Profile)
	response.Performance.SchemaLoadTime = time.Since(schemaStartTime)

	if err != nil {
		response.Warnings = append(response.Warnings, TransformWarning{
			Code:    "SCHEMA_LOAD_PARTIAL",
			Message: fmt.Sprintf("Some FHIR schemas could not be loaded: %v", err),
		})
	} else {
		response.Metadata.SchemaUsed = true
		if te.config.VerboseLogging {
			log.Printf("📚 Loaded %d FHIR schemas", len(resourceSchemas))
		}
	}

	// Step 3: Apply transformation rules with schema guidance
	transformStartTime := time.Now()
	resources, warnings, err := te.applyTransformationRules(request.HL7Message, rules, resourceSchemas)
	response.Performance.TransformTime = time.Since(transformStartTime)

	if err != nil {
		return te.errorResponse(response, "TRANSFORMATION_FAILED", err.Error()), nil
	}

	response.FHIRResources = resources
	response.Warnings = append(response.Warnings, warnings...)
	response.Metadata.ResourcesCreated = len(resources)
	response.Metadata.RulesApplied = len(rules)

	// Step 4: Schema-based validation (if enabled)
	if te.config.ValidateOutput && response.Metadata.SchemaUsed {
		validationStartTime := time.Now()
		validationErrors := te.validateResourcesAgainstSchemas(resources, resourceSchemas)
		response.Performance.ValidationTime = time.Since(validationStartTime)
		response.ValidationErrors = validationErrors
	}

	// Step 5: Create FHIR Bundle (if requested)
	if request.CreateBundle || te.config.CreateBundle {
		bundle, err := te.createFHIRBundle(resources, request.RequestID)
		if err != nil {
			response.Warnings = append(response.Warnings, TransformWarning{
				Code:    "BUNDLE_CREATION_FAILED",
				Message: fmt.Sprintf("Failed to create bundle: %v", err),
			})
		} else {
			response.Bundle = bundle
		}
	}

	// Step 6: Audit logging
	go te.logTransformation(request, response) // Async logging

	// Finalize response
	response.Performance.TotalTime = time.Since(startTime)
	response.Success = len(response.ValidationErrors) == 0

	if te.config.VerboseLogging {
		log.Printf("✅ FHIR transformation completed: %s (%v)", request.RequestID, response.Performance.TotalTime)
	}

	return response, nil
}

// =====================================
// SCHEMA INTEGRATION FUNCTIONS
// =====================================

func (te *TransformationEngine) loadRequiredSchemas(ctx context.Context, rules []MappingRule, version, profile string) (map[string]*FHIRSchema, error) {
	if te.schemaLoader == nil {
		return nil, fmt.Errorf("FHIR schema loader not initialized")
	}

	schemas := make(map[string]*FHIRSchema)
	uniqueResources := make(map[string]bool)

	// Collect unique resource types from rules
	for _, rule := range rules {
		uniqueResources[rule.FHIRResource] = true
	}

	if te.config.VerboseLogging {
		log.Printf("🔍 Loading FHIR schemas for resources: %v", getKeys(uniqueResources))
	}

	// Load schema for each resource type
	for resourceType := range uniqueResources {
		schema, err := te.schemaLoader.LoadFHIRSchema(resourceType, profile, version)
		if err != nil {
			// Try fallback to base profile
			if profile != "base" {
				schema, err = te.schemaLoader.LoadFHIRSchema(resourceType, "base", version)
			}
			if err != nil {
				log.Printf("⚠️ Could not load FHIR schema for %s/%s/%s: %v", resourceType, profile, version, err)
				continue
			}
		}
		schemas[resourceType] = schema
	}

	return schemas, nil
}

// =====================================
// RULE APPLICATION WITH SCHEMA GUIDANCE
// =====================================

func (te *TransformationEngine) applyTransformationRules(hl7Message map[string]interface{}, rules []MappingRule, schemas map[string]*FHIRSchema) ([]map[string]interface{}, []TransformWarning, error) {
	resources := make(map[string]map[string]interface{}) // resourceType -> resource
	warnings := []TransformWarning{}

	// Initialize resources based on schemas
	for resourceType, schema := range schemas {
		resources[resourceType] = te.initializeResourceFromSchema(resourceType, schema)
	}

	// Apply rules in priority order
	for _, rule := range rules {
		// Extract HL7 value
		hl7Value, found := te.extractHL7Value(hl7Message, rule.HL7Segment, rule.HL7Field, rule.HL7Component)
		if !found {
			if rule.Required {
				warnings = append(warnings, TransformWarning{
					Code:    "REQUIRED_FIELD_MISSING",
					Message: fmt.Sprintf("Required HL7 field %s.%s not found", rule.HL7Segment, rule.HL7Field),
					HL7Path: fmt.Sprintf("%s.%s", rule.HL7Segment, rule.HL7Field),
				})
			}
			continue
		}

		// Check condition if specified
		if rule.Condition != "" && !te.evaluateCondition(rule.Condition, hl7Message) {
			continue
		}

		// Transform value based on rule logic and schema
		transformedValue, err := te.transformValue(hl7Value, rule.TransformLogic, schemas[rule.FHIRResource])
		if err != nil {
			warnings = append(warnings, TransformWarning{
				Code:     "TRANSFORMATION_ERROR",
				Message:  fmt.Sprintf("Failed to transform %s.%s: %v", rule.HL7Segment, rule.HL7Field, err),
				HL7Path:  fmt.Sprintf("%s.%s", rule.HL7Segment, rule.HL7Field),
				FHIRPath: rule.FHIRPath,
			})
			continue
		}

		// Apply to FHIR resource with schema validation
		err = te.setFHIRValue(resources[rule.FHIRResource], rule.FHIRPath, transformedValue, schemas[rule.FHIRResource])
		if err != nil {
			warnings = append(warnings, TransformWarning{
				Code:     "FHIR_PATH_ERROR",
				Message:  fmt.Sprintf("Failed to set FHIR path %s: %v", rule.FHIRPath, err),
				FHIRPath: rule.FHIRPath,
			})
			continue
		}
	}

	// Convert to array and clean up empty resources
	result := []map[string]interface{}{}
	for resourceType, resource := range resources {
		if te.isResourcePopulated(resource) {
			// Ensure required fields from schema
			if schema, exists := schemas[resourceType]; exists {
				resource = te.ensureRequiredFields(resource, schema)
			}
			result = append(result, resource)
		}
	}

	return result, warnings, nil
}

// =====================================
// SCHEMA-BASED RESOURCE INITIALIZATION
// =====================================

func (te *TransformationEngine) initializeResourceFromSchema(resourceType string, schema *FHIRSchema) map[string]interface{} {
	resource := map[string]interface{}{
		"resourceType": resourceType,
	}

	// Add required fields with defaults based on schema
	for path, element := range schema.Elements {
		if element.Required {
			fieldPath := strings.TrimPrefix(path, resourceType+".")
			if fieldPath != "resourceType" {
				resource[fieldPath] = te.getDefaultValueForDataType(element.DataType)
			}
		}
	}

	return resource
}

func (te *TransformationEngine) getDefaultValueForDataType(dataType string) interface{} {
	switch strings.ToLower(dataType) {
	case "id", "string", "code", "uri", "url":
		return ""
	case "boolean":
		return false
	case "integer", "decimal":
		return nil
	case "datetime", "date", "instant":
		return nil
	case "identifier":
		return []interface{}{}
	case "humanname":
		return []interface{}{}
	case "address":
		return []interface{}{}
	case "contactpoint":
		return []interface{}{}
	default:
		return nil
	}
}

// =====================================
// HELPER FUNCTIONS
// =====================================

func (te *TransformationEngine) extractMessageType(hl7Message map[string]interface{}) (string, error) {
	messageType, ok := hl7Message["messageType"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("messageType not found in HL7 message")
	}

	code, codeOk := messageType["code"].(string)
	event, eventOk := messageType["event"].(string)

	if !codeOk || !eventOk {
		return "", fmt.Errorf("invalid messageType structure")
	}

	return fmt.Sprintf("%s^%s", code, event), nil
}

func (te *TransformationEngine) loadMappingRules(ctx context.Context, messageType, profile string) ([]MappingRule, error) {
	query := `
		SELECT id, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1 AND (fhir_profile = $2 OR fhir_profile = 'base')
		  AND is_active = true
		ORDER BY priority ASC, id ASC
	`

	rows, err := te.db.QueryContext(ctx, query, messageType, profile)
	if err != nil {
		return nil, fmt.Errorf("failed to query mapping rules: %v", err)
	}
	defer rows.Close()

	var rules []MappingRule
	for rows.Next() {
		var rule MappingRule
		var transformLogicJSON []byte
		var component, condition sql.NullString

		err := rows.Scan(
			&rule.ID, &rule.HL7MessageType, &rule.HL7Segment, &rule.HL7Field, &component,
			&rule.FHIRResource, &rule.FHIRProfile, &rule.FHIRPath, &transformLogicJSON,
			&condition, &rule.Required, &rule.Priority,
		)
		if err != nil {
			continue
		}

		// Parse optional fields
		if component.Valid {
			rule.HL7Component = component.String
		}
		if condition.Valid {
			rule.Condition = condition.String
		}

		// Parse transformation logic JSON
		if len(transformLogicJSON) > 0 {
			json.Unmarshal(transformLogicJSON, &rule.TransformLogic)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

func (te *TransformationEngine) errorResponse(response *TransformationResponse, code, message string) *TransformationResponse {
	response.Success = false
	response.ValidationErrors = append(response.ValidationErrors, ValidationError{
		Code:     code,
		Message:  message,
		Severity: "error",
	})
	return response
}

// Additional helper functions would continue here...
func getVersionOrDefault(version, defaultVersion string) string {
	if version == "" {
		return defaultVersion
	}
	return version
}

func getProfileOrDefault(profile, defaultProfile string) string {
	if profile == "" {
		return defaultProfile
	}
	return profile
}

func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
