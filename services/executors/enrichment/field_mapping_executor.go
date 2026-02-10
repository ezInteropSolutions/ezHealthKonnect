package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ===============================================================
// FIELD MAPPING EXECUTOR
// ===============================================================

// FieldMappingExecutor performs generic field-to-field mappings with transformations
// Implements Strategy Pattern - concrete strategy for field mapping
type FieldMappingExecutor struct {
	*executors.BaseExecutor
}

// NewFieldMappingExecutor creates a new field mapping executor
func NewFieldMappingExecutor() *FieldMappingExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Field Mapping",
		Description: "Map source fields to target variables with optional transformations (trim, upper, lower, regex, substring, replace)",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "transformation",
	}

	base := executors.NewBaseExecutor("field_mapping", metadata)

	return &FieldMappingExecutor{
		BaseExecutor: base,
	}
}

// Execute performs field mapping with transformations
func (e *FieldMappingExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Ensure enriched map exists
	enriched := executors.EnsureMapExists(inputData, "enriched")

	// Create field mapping results
	mappedFields := make(map[string]interface{})

	// Process each mapping
	for i, mapping := range config.Mappings {
		_ = i // used for iteration

		// Resolve source value
		sourceValue, err := e.resolveSourceValue(mapping.RHS, inputData)
		if err != nil {
			log.Printf("   ⚠️  Failed to resolve source: %v", err)
			continue
		}

		// Apply transformations using shared utility (OOP - DRY principle)
		transformedValue := fmt.Sprintf("%v", executors.ApplyTransformations(sourceValue, mapping.Transforms))

		// Try to parse as JSON if it looks like JSON
		var finalValue interface{} = transformedValue
		if e.isJSONString(transformedValue) {
			log.Printf("   🔍 [JSON Detection] Value looks like JSON: %s", transformedValue)
			var jsonValue interface{}
			if err := json.Unmarshal([]byte(transformedValue), &jsonValue); err == nil {
				finalValue = jsonValue
				log.Printf("   📦 Parsed JSON object for %s", mapping.LHS)
			} else {
				log.Printf("   ⚠️  Failed to parse JSON for %s: %v", mapping.LHS, err)
			}
		}

		// Store result
		mappedFields[mapping.LHS] = finalValue
		log.Printf("   ✅ %s = %v", mapping.LHS, finalValue)
	}

	// Store results in enriched.field_mapping
	enriched["field_mapping"] = mappedFields

	log.Printf("✅ [FieldMapping] Mapped %d fields", len(mappedFields))

	// Add metadata if configured
	if config.Metadata != nil && len(config.Metadata) > 0 {
		metadata := executors.EnsureMapExists(inputData, "metadata")
		for key, value := range config.Metadata {
			metadata[key] = value
			log.Printf("   ✅ Added metadata: %s", key)
		}
		log.Printf("✅ [FieldMapping] Added %d metadata entries", len(config.Metadata))
	}

	// STANDARDIZED: Variables (the mapped fields) + execution details
	e.SetStepOutputWithDetails(inputData, map[string]interface{}{
		"mapped_fields": mappedFields,
	}, map[string]interface{}{
		"field_count":    len(mappedFields),
		"transformation": "field_mapping",
	})

	// Post-execution logging
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// resolveSourceValue resolves the source value from RHS
func (e *FieldMappingExecutor) resolveSourceValue(rhs string, inputData map[string]interface{}) (string, error) {
	// Handle system variables
	if strings.HasPrefix(rhs, "${") && strings.HasSuffix(rhs, "}") {
		varName := strings.TrimSuffix(strings.TrimPrefix(rhs, "${"), "}")
		return e.resolveSystemVariable(varName, inputData), nil
	}

	// Handle reference variables (from previous steps) - format: ["step_name"].path.to.field
	if strings.HasPrefix(rhs, "[\"") && strings.Contains(rhs, "\"].") {
		value := executors.GetNestedValue(inputData, rhs)
		if value != nil {
			return fmt.Sprintf("%v", value), nil
		}
		return "", nil
	}

	// Handle HL7 field paths - format: PID.5.1, MSH.9, IN1.2, etc.
	if e.isHL7FieldPath(rhs) {
		// LOOP-AWARE: When running inside a loop, resolve from the current loop item first.
		// The loop executor puts the current item in inputData["item"].
		// The item is a single HL7 segment (generic map with "fields" array).
		if item, hasLoopItem := inputData["item"]; hasLoopItem {
			value := resolveFieldFromLoopItem(item, rhs)
			if value != nil {
				return fmt.Sprintf("%v", value), nil
			}
			// Fall through to resolve from the full message
		}

		// Fallback: resolve from the full message
		value := executors.GetNestedValue(inputData, rhs)
		if value != nil {
			return fmt.Sprintf("%v", value), nil
		}
		return "", nil
	}

	// Handle dot-notation paths (enriched.api.response, metadata.correlationId, etc.)
	if strings.Contains(rhs, ".") {
		value := executors.GetNestedValue(inputData, rhs)
		if value != nil {
			return fmt.Sprintf("%v", value), nil
		}
	}

	// Literal value
	return rhs, nil
}

// resolveFieldFromLoopItem resolves an HL7 field path from a single loop item (segment).
// The loop item is a generic map (from JSON round-trip of hl7.EnhancedSegment) with structure:
//
//	{ "key": "IN1", "name": "Insurance", "fields": [ { "key": "IN1.2", "value": "PPO123", "subfields": [...] }, ... ] }
//
// Supports both field-level (IN1.2) and subfield-level (IN1.2.1) paths.
func resolveFieldFromLoopItem(item interface{}, fieldKey string) interface{} {
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}

	// Get the fields array from the loop item
	fieldsRaw, ok := itemMap["fields"]
	if !ok {
		return nil
	}

	fields, ok := fieldsRaw.([]interface{})
	if !ok {
		return nil
	}

	// Parse the field key: "IN1.2" -> field key "IN1.2", or "IN1.2.1" -> parent "IN1.2", subfield "IN1.2.1"
	parts := strings.Split(fieldKey, ".")
	if len(parts) < 2 {
		return nil
	}

	// Build the target field key (segment.field, e.g., "IN1.2")
	targetFieldKey := parts[0] + "." + parts[1]

	// Search fields for the matching key
	for _, fieldRaw := range fields {
		field, ok := fieldRaw.(map[string]interface{})
		if !ok {
			continue
		}

		key, _ := field["key"].(string)
		if key != targetFieldKey {
			continue
		}

		// Field-level access (e.g., IN1.2)
		if len(parts) == 2 {
			return field["value"]
		}

		// Subfield-level access (e.g., IN1.2.1) - search subfields
		subfieldsRaw, ok := field["subfields"]
		if !ok {
			return field["value"] // No subfields, return field value
		}

		subfields, ok := subfieldsRaw.([]interface{})
		if !ok {
			return field["value"]
		}

		for _, subfieldRaw := range subfields {
			subfield, ok := subfieldRaw.(map[string]interface{})
			if !ok {
				continue
			}
			subKey, _ := subfield["key"].(string)
			if subKey == fieldKey {
				return subfield["value"]
			}
		}

		return field["value"] // Subfield not found, return field value
	}

	return nil
}

// isHL7FieldPath checks if a path looks like an HL7 field (e.g., PID.5.1, MSH.9)
func (e *FieldMappingExecutor) isHL7FieldPath(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// First part should be 2-3 char uppercase segment name (e.g., MSH, PID, IN1, NK1, PV1)
	segment := parts[0]
	if len(segment) < 2 || len(segment) > 3 {
		return false
	}
	for _, c := range segment {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}

	// Remaining parts should be numeric
	for i := 1; i < len(parts); i++ {
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				return false
			}
		}
	}

	return true
}

// resolveSystemVariable resolves system-generated values
func (e *FieldMappingExecutor) resolveSystemVariable(varName string, inputData map[string]interface{}) string {
	switch varName {
	case "CURRENT_TIMESTAMP":
		return time.Now().UTC().Format(time.RFC3339)
	case "GUID", "UUID":
		return uuid.New().String()
	case "INTERFACE_ID":
		if id, ok := inputData["interfaceId"]; ok {
			return fmt.Sprintf("%v", id)
		}
		return ""
	case "INTERFACE_NAME":
		if name, ok := inputData["interfaceName"]; ok {
			return fmt.Sprintf("%v", name)
		}
		return ""
	default:
		return ""
	}
}

// NOTE: applyTransformations has been moved to shared utility: executors.ApplyTransformations()
// This enables code reuse across FieldMapping, SwitchCase, IfThenElse executors

// isJSONString checks if a string looks like JSON (starts with { or [)
func (e *FieldMappingExecutor) isJSONString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return false
	}

	// Check if it starts with { (object) or [ (array)
	firstChar := trimmed[0]
	return firstChar == '{' || firstChar == '['
}

// Validate checks if the step configuration is valid
func (e *FieldMappingExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses and validates the step configuration
func (e *FieldMappingExecutor) parseConfig(step *models.TransformationStep) (*models.FieldMappingConfig, error) {
	if step.Config == nil {
		return &models.FieldMappingConfig{Mappings: []models.FieldMapping{}}, nil
	}

	// Marshal to JSON then unmarshal to struct for type safety
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.FieldMappingConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// GetConfigSchema returns the JSON schema for configuration
func (e *FieldMappingExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mappings": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"lhs": map[string]interface{}{
							"type":        "string",
							"description": "Target variable name",
						},
						"rhs": map[string]interface{}{
							"type":        "string",
							"description": "Source field path or system variable",
						},
						"transforms": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of transformations (trim, upper, lower, regex:pattern, substring:start:end, replace:old:new)",
						},
					},
					"required": []string{"lhs", "rhs"},
				},
			},
		},
	}
}

// GetConfigExample returns an example configuration
func (e *FieldMappingExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"mappings": []map[string]string{
			{
				"lhs":        "patientName",
				"rhs":        "enhancedSegments.PID.fields[5].value",
				"transforms": "trim, upper",
			},
			{
				"lhs":        "timestamp",
				"rhs":        "${CURRENT_TIMESTAMP}",
				"transforms": "",
			},
			{
				"lhs":        "correlationId",
				"rhs":        "${GUID}",
				"transforms": "",
			},
		},
	}
}

// ===============================================================
// VARIABLE PROVIDER INTERFACE IMPLEMENTATION
// ===============================================================

// GetOutputVariables returns the list of variables this executor will produce
// Implements the VariableProvider interface for automatic variable discovery
func (e *FieldMappingExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	// Parse config to extract field mappings
	config, err := e.parseConfig(step)
	if err != nil {
		log.Printf("⚠️  [FieldMapping] Failed to parse config for variable discovery: %v", err)
		return variables
	}

	// Extract output variables from each mapping's LHS (left-hand side)
	basePath := "enriched.field_mapping"

	for _, mapping := range config.Mappings {
		if mapping.LHS == "" {
			continue
		}

		// Determine description based on source
		description := fmt.Sprintf("Mapped from %s", mapping.RHS)
		if mapping.Transforms != "" {
			description += fmt.Sprintf(" with transformations: %s", mapping.Transforms)
		}

		variables = append(variables, models.VariableDefinition{
			Name:         mapping.LHS,
			Path:         fmt.Sprintf("%s.%s", basePath, mapping.LHS),
			DataType:     "string", // Field mappings typically produce strings after transformations
			Description:  description,
			SourceField:  mapping.RHS,
			UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.%s")`, basePath, mapping.LHS),
			Category:     "Field Mapping",
		})
	}

	return variables
}
