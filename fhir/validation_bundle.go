// fhir/validation_bundle.go
// Schema-based validation and bundle creation for FHIR resources
package fhir

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// =====================================
// SCHEMA-BASED RESOURCE VALIDATION
// =====================================

func (te *TransformationEngine) validateResourcesAgainstSchemas(resources []map[string]interface{}, schemas map[string]*FHIRSchema) []ValidationError {
	var errors []ValidationError

	for _, resource := range resources {
		resourceType, ok := resource["resourceType"].(string)
		if !ok {
			errors = append(errors, ValidationError{
				ResourceType: "Unknown",
				Path:         "resourceType",
				Code:         "MISSING_RESOURCE_TYPE",
				Message:      "Resource missing required resourceType field",
				Severity:     "error",
			})
			continue
		}

		schema, schemaExists := schemas[resourceType]
		if !schemaExists {
			errors = append(errors, ValidationError{
				ResourceType: resourceType,
				Path:         resourceType,
				Code:         "SCHEMA_NOT_FOUND",
				Message:      fmt.Sprintf("No schema found for resource type %s", resourceType),
				Severity:     "warning",
			})
			continue
		}

		// Validate against schema
		resourceErrors := te.validateResourceAgainstSchema(resource, schema)
		errors = append(errors, resourceErrors...)
	}

	return errors
}

func (te *TransformationEngine) validateResourceAgainstSchema(resource map[string]interface{}, schema *FHIRSchema) []ValidationError {
	var errors []ValidationError
	resourceType := schema.ResourceType

	// 1. Check required elements
	for _, requiredPath := range schema.Required {
		if err := te.validateRequiredElement(resource, requiredPath, resourceType); err != nil {
			errors = append(errors, *err)
		}
	}

	// 2. Validate each field in the resource against schema elements
	for fieldName, fieldValue := range resource {
		if fieldName == "resourceType" {
			continue // Skip resourceType validation
		}

		elementPath := fmt.Sprintf("%s.%s", resourceType, fieldName)
		element, elementExists := schema.Elements[elementPath]

		if !elementExists {
			// Field not in schema - could be an extension or error
			errors = append(errors, ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "UNKNOWN_ELEMENT",
				Message:      fmt.Sprintf("Element %s not found in schema", fieldName),
				Severity:     "warning",
			})
			continue
		}

		// Validate field value against element definition
		fieldErrors := te.validateFieldAgainstElement(fieldValue, element, elementPath, resourceType)
		errors = append(errors, fieldErrors...)
	}

	// 3. Validate must-support elements (if profile requires them)
	for _, mustSupportPath := range schema.MustSupport {
		if err := te.validateMustSupportElement(resource, mustSupportPath, resourceType); err != nil {
			errors = append(errors, *err)
		}
	}

	return errors
}

func (te *TransformationEngine) validateRequiredElement(resource map[string]interface{}, requiredPath, resourceType string) *ValidationError {
	parts := strings.Split(requiredPath, ".")
	if len(parts) < 2 {
		return nil
	}

	fieldName := parts[1]
	value, exists := resource[fieldName]

	if !exists || te.isEmpty(value) {
		return &ValidationError{
			ResourceType: resourceType,
			Path:         requiredPath,
			Code:         "REQUIRED_ELEMENT_MISSING",
			Message:      fmt.Sprintf("Required element %s is missing or empty", requiredPath),
			Severity:     "error",
		}
	}

	return nil
}

func (te *TransformationEngine) validateMustSupportElement(resource map[string]interface{}, mustSupportPath, resourceType string) *ValidationError {
	parts := strings.Split(mustSupportPath, ".")
	if len(parts) < 2 {
		return nil
	}

	fieldName := parts[1]
	value, exists := resource[fieldName]

	if !exists || te.isEmpty(value) {
		return &ValidationError{
			ResourceType: resourceType,
			Path:         mustSupportPath,
			Code:         "MUST_SUPPORT_MISSING",
			Message:      fmt.Sprintf("Must-support element %s should be present", mustSupportPath),
			Severity:     "warning",
		}
	}

	return nil
}

func (te *TransformationEngine) validateFieldAgainstElement(fieldValue interface{}, element *FHIRElement, elementPath, resourceType string) []ValidationError {
	var errors []ValidationError

	// Validate cardinality
	if cardinalityError := te.validateCardinality(fieldValue, element, elementPath, resourceType); cardinalityError != nil {
		errors = append(errors, *cardinalityError)
	}

	// Validate data type
	if dataTypeError := te.validateDataType(fieldValue, element, elementPath, resourceType); dataTypeError != nil {
		errors = append(errors, *dataTypeError)
	}

	// Validate constraints
	constraintErrors := te.validateConstraints(fieldValue, element, elementPath, resourceType)
	errors = append(errors, constraintErrors...)

	// Validate value set binding
	if element.ValueSet != "" {
		if valueSetError := te.validateValueSetBinding(fieldValue, element, elementPath, resourceType); valueSetError != nil {
			errors = append(errors, *valueSetError)
		}
	}

	return errors
}

func (te *TransformationEngine) validateCardinality(fieldValue interface{}, element *FHIRElement, elementPath, resourceType string) *ValidationError {
	cardinality := element.Cardinality

	// Parse cardinality (e.g., "0..1", "1..*", "0..*")
	parts := strings.Split(cardinality, "..")
	if len(parts) != 2 {
		return nil // Invalid cardinality format, skip validation
	}

	minStr := parts[0]
	maxStr := parts[1]

	// Check if field is array
	if arr, ok := fieldValue.([]interface{}); ok {
		count := len(arr)

		// Check minimum
		if minStr != "0" && count == 0 {
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "CARDINALITY_MIN_VIOLATION",
				Message:      fmt.Sprintf("Element %s requires at least %s items, found %d", elementPath, minStr, count),
				Severity:     "error",
			}
		}

		// Check maximum (if not *)
		if maxStr != "*" && maxStr != "1" {
			// maxStr is a number, would need proper parsing
		} else if maxStr == "1" && count > 1 {
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "CARDINALITY_MAX_VIOLATION",
				Message:      fmt.Sprintf("Element %s allows maximum 1 item, found %d", elementPath, count),
				Severity:     "error",
			}
		}
	} else {
		// Single value
		if strings.Contains(cardinality, "*") && fieldValue != nil {
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "CARDINALITY_TYPE_MISMATCH",
				Message:      fmt.Sprintf("Element %s expects array but found single value", elementPath),
				Severity:     "warning",
			}
		}
	}

	return nil
}

func (te *TransformationEngine) validateDataType(fieldValue interface{}, element *FHIRElement, elementPath, resourceType string) *ValidationError {
	expectedType := element.DataType

	switch strings.ToLower(expectedType) {
	case "string", "code", "id", "uri", "url":
		if _, ok := fieldValue.(string); !ok {
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "DATA_TYPE_MISMATCH",
				Message:      fmt.Sprintf("Element %s expects %s, got %T", elementPath, expectedType, fieldValue),
				Severity:     "error",
			}
		}
	case "boolean":
		if _, ok := fieldValue.(bool); !ok {
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "DATA_TYPE_MISMATCH",
				Message:      fmt.Sprintf("Element %s expects boolean, got %T", elementPath, fieldValue),
				Severity:     "error",
			}
		}
	case "integer":
		// Accept both int and float64 (from JSON parsing)
		switch fieldValue.(type) {
		case int, int64, float64:
			// Valid
		default:
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "DATA_TYPE_MISMATCH",
				Message:      fmt.Sprintf("Element %s expects integer, got %T", elementPath, fieldValue),
				Severity:     "error",
			}
		}
	case "identifier", "humanname", "address", "contactpoint", "coding", "codeableconcept":
		// Complex types should be objects or arrays of objects
		switch v := fieldValue.(type) {
		case map[string]interface{}:
			// Single complex object - valid
		case []interface{}:
			// Array of complex objects - check each
			for i, item := range v {
				if _, ok := item.(map[string]interface{}); !ok {
					return &ValidationError{
						ResourceType: resourceType,
						Path:         fmt.Sprintf("%s[%d]", elementPath, i),
						Code:         "DATA_TYPE_MISMATCH",
						Message:      fmt.Sprintf("Element %s[%d] expects object, got %T", elementPath, i, item),
						Severity:     "error",
					}
				}
			}
		default:
			return &ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "DATA_TYPE_MISMATCH",
				Message:      fmt.Sprintf("Element %s expects %s, got %T", elementPath, expectedType, fieldValue),
				Severity:     "error",
			}
		}
	}

	return nil
}

func (te *TransformationEngine) validateConstraints(fieldValue interface{}, element *FHIRElement, elementPath, resourceType string) []ValidationError {
	var errors []ValidationError

	// Implement constraint validation based on element.Constraints
	// This would be enhanced with a proper constraint parser
	for _, constraint := range element.Constraints {
		// Simple constraint validation - could be enhanced
		if strings.Contains(constraint, "required") && te.isEmpty(fieldValue) {
			errors = append(errors, ValidationError{
				ResourceType: resourceType,
				Path:         elementPath,
				Code:         "CONSTRAINT_VIOLATION",
				Message:      fmt.Sprintf("Constraint violation on %s: %s", elementPath, constraint),
				Severity:     "error",
			})
		}
	}

	return errors
}

func (te *TransformationEngine) validateValueSetBinding(fieldValue interface{}, element *FHIRElement, elementPath, resourceType string) *ValidationError {
	// Value set validation would require loading the value set and checking codes
	// For now, just log that validation should occur
	if te.config.VerboseLogging {
		log.Printf("🔍 Value set validation needed for %s with value set %s", elementPath, element.ValueSet)
	}
	return nil
}

func (te *TransformationEngine) isEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	default:
		return false
	}
}

// =====================================
// FHIR BUNDLE CREATION
// =====================================

func (te *TransformationEngine) createFHIRBundle(resources []map[string]interface{}, requestID string) (map[string]interface{}, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources to bundle")
	}

	// Create bundle entries
	entries := make([]map[string]interface{}, len(resources))
	for i, resource := range resources {
		resourceType, ok := resource["resourceType"].(string)
		if !ok {
			return nil, fmt.Errorf("resource %d missing resourceType", i)
		}

		// Generate resource ID if not present
		resourceID, hasID := resource["id"].(string)
		if !hasID || resourceID == "" {
			resourceID = fmt.Sprintf("%s-%s-%d", strings.ToLower(resourceType), requestID, i+1)
			resource["id"] = resourceID
		}

		entry := map[string]interface{}{
			"fullUrl":  fmt.Sprintf("urn:uuid:%s", resourceID),
			"resource": resource,
			"request": map[string]interface{}{
				"method": "POST",
				"url":    resourceType,
			},
		}

		entries[i] = entry
	}

	// Create the bundle
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"id":           fmt.Sprintf("bundle-%s", requestID),
		"type":         "transaction",
		"timestamp":    time.Now().Format(time.RFC3339),
		"entry":        entries,
		"meta": map[string]interface{}{
			"lastUpdated": time.Now().Format(time.RFC3339),
			"tag": []map[string]interface{}{
				{
					"system":  "http://terminology.hl7.org/CodeSystem/v3-ActReason",
					"code":    "HTEST",
					"display": "test health data",
				},
			},
		},
	}

	// Add bundle metadata
	bundle["total"] = len(entries)

	return bundle, nil
}

// =====================================
// AUDIT LOGGING
// =====================================

func (te *TransformationEngine) logTransformation(request *TransformationRequest, response *TransformationResponse) {
	if te.db == nil {
		return
	}

	// Prepare audit data
	inputJSON, _ := json.Marshal(request.HL7Message)
	outputJSON, _ := json.Marshal(response.FHIRResources)
	errorsJSON, _ := json.Marshal(response.ValidationErrors)
	warningsJSON, _ := json.Marshal(response.Warnings)

	query := `
		INSERT INTO fhir_transformation_logs 
		(request_id, hl7_message_type, fhir_target_profile, rules_applied, 
		 resources_created, warnings_count, errors_count, processing_time_ms,
		 transformation_status, input_hl7_message, output_fhir_bundle,
		 error_details, warnings, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	status := "success"
	if !response.Success {
		status = "error"
	} else if len(response.Warnings) > 0 {
		status = "warning"
	}

	_, err := te.db.Exec(query,
		response.RequestID,
		response.Metadata.HL7MessageType,
		response.Metadata.Profile,
		response.Metadata.RulesApplied,
		response.Metadata.ResourcesCreated,
		len(response.Warnings),
		len(response.ValidationErrors),
		int(response.Performance.TotalTime.Milliseconds()),
		status,
		string(inputJSON),
		string(outputJSON),
		string(errorsJSON),
		string(warningsJSON),
		response.Metadata.ProcessedAt,
		response.Metadata.ProcessedAt.Add(response.Performance.TotalTime),
	)

	if err != nil {
		log.Printf("⚠️ Failed to log transformation: %v", err)
	}
}

// =====================================
// TRANSFORMATION ENGINE STATUS
// =====================================

func (te *TransformationEngine) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"status":  "operational",
		"version": "1.0.0",
		"uptime":  time.Since(time.Now()).String(), // Would track actual uptime
	}

	// Database status
	if te.db != nil {
		var ruleCount int
		err := te.db.QueryRow("SELECT COUNT(*) FROM hl7_fhir_mappings WHERE is_active = true").Scan(&ruleCount)
		if err == nil {
			status["database"] = map[string]interface{}{
				"connected": true,
				"rules":     ruleCount,
			}
		} else {
			status["database"] = map[string]interface{}{
				"connected": false,
				"error":     err.Error(),
			}
		}
	}

	// Schema loader status
	if te.schemaLoader != nil {
		schemaStats := te.schemaLoader.GetStats()
		status["schemas"] = map[string]interface{}{
			"loaded":      schemaStats.CacheSize,
			"totalLoads":  schemaStats.TotalLoads,
			"cacheHits":   schemaStats.CacheHits,
			"cacheMisses": schemaStats.CacheMisses,
			"errors":      schemaStats.LoadErrors,
		}

		// List available schemas
		if available, err := te.schemaLoader.ListAvailableSchemas(); err == nil {
			status["availableSchemas"] = available
		}
	}

	// Configuration
	status["configuration"] = map[string]interface{}{
		"defaultProfile":    te.config.DefaultProfile,
		"validateOutput":    te.config.ValidateOutput,
		"createBundle":      te.config.CreateBundle,
		"verboseLogging":    te.config.VerboseLogging,
		"maxProcessingTime": te.config.MaxProcessingTime.String(),
	}

	// Capabilities
	status["capabilities"] = []string{
		"HL7 to FHIR transformation",
		"Schema-based validation",
		"Bundle creation",
		"Rule management",
		"Audit logging",
		"Multi-profile support",
	}

	return status
}

// =====================================
// CONFIGURATION UPDATES
// =====================================

func (te *TransformationEngine) UpdateConfiguration(updates map[string]interface{}) error {
	if defaultProfile, ok := updates["defaultProfile"].(string); ok {
		te.config.DefaultProfile = defaultProfile
	}

	if validateOutput, ok := updates["validateOutput"].(bool); ok {
		te.config.ValidateOutput = validateOutput
	}

	if createBundle, ok := updates["createBundle"].(bool); ok {
		te.config.CreateBundle = createBundle
	}

	if verboseLogging, ok := updates["verboseLogging"].(bool); ok {
		te.config.VerboseLogging = verboseLogging
	}

	return nil
}
