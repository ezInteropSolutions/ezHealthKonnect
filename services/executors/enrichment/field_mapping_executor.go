package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"regexp"
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

	base := executors.NewBaseExecutor("core.transformation", metadata)

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

	log.Printf("🔍 [FieldMapping] Processing %d mappings...", len(config.Mappings))

	// Process each mapping
	for i, mapping := range config.Mappings {
		log.Printf("   [%d] %s = %s (transforms: %s)", i+1, mapping.LHS, mapping.RHS, mapping.Transforms)

		// Resolve source value
		sourceValue, err := e.resolveSourceValue(mapping.RHS, inputData)
		if err != nil {
			log.Printf("   ⚠️  Failed to resolve source: %v", err)
			continue
		}

		// Apply transformations
		transformedValue := e.applyTransformations(sourceValue, mapping.Transforms)

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

	// Handle HL7 field paths using BaseExecutor utility - format: PID.5.1, MSH.9, etc.
	if e.isHL7FieldPath(rhs) {
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

// isHL7FieldPath checks if a path looks like an HL7 field (e.g., PID.5.1, MSH.9)
func (e *FieldMappingExecutor) isHL7FieldPath(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// First part should be 3-letter uppercase segment name
	segment := parts[0]
	if len(segment) != 3 {
		return false
	}
	for _, c := range segment {
		if c < 'A' || c > 'Z' {
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

// applyTransformations applies transformation functions to a value
func (e *FieldMappingExecutor) applyTransformations(value string, transforms string) string {
	if transforms == "" {
		return value
	}

	// Split multiple transformations (comma-separated)
	transformList := strings.Split(transforms, ",")
	result := value

	for _, transform := range transformList {
		transform = strings.TrimSpace(transform)

		// Simple transformations
		if transform == "trim" {
			result = strings.TrimSpace(result)
		} else if transform == "upper" {
			result = strings.ToUpper(result)
		} else if transform == "lower" {
			result = strings.ToLower(result)
		} else if strings.HasPrefix(transform, "regex:") {
			// Regex extraction: regex:pattern
			pattern := strings.TrimPrefix(transform, "regex:")
			re, err := regexp.Compile(pattern)
			if err == nil {
				matches := re.FindStringSubmatch(result)
				if len(matches) > 0 {
					result = matches[0]
				}
			}
		} else if strings.HasPrefix(transform, "substring:") {
			// Substring: substring:start:end
			params := strings.TrimPrefix(transform, "substring:")
			parts := strings.Split(params, ":")
			if len(parts) == 2 {
				var start, end int
				fmt.Sscanf(parts[0], "%d", &start)
				fmt.Sscanf(parts[1], "%d", &end)
				if start >= 0 && end <= len(result) && start < end {
					result = result[start:end]
				}
			}
		} else if strings.HasPrefix(transform, "replace:") {
			// Replace: replace:old:new
			params := strings.TrimPrefix(transform, "replace:")
			parts := strings.Split(params, ":")
			if len(parts) == 2 {
				result = strings.ReplaceAll(result, parts[0], parts[1])
			}
		}
	}

	return result
}

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
