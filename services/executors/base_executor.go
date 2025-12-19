package executors

import (
	"context"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// ===============================================================
// BASE EXECUTOR - Template Method Pattern
// ===============================================================

// BaseExecutor provides common functionality for all executors
// Implements Template Method pattern for common pre/post execution logic
type BaseExecutor struct {
	stepType string
	metadata models.ExecutorMetadata
}

// NewBaseExecutor creates a new base executor
func NewBaseExecutor(stepType string, metadata models.ExecutorMetadata) *BaseExecutor {
	return &BaseExecutor{
		stepType: stepType,
		metadata: metadata,
	}
}

// GetStepType returns the executor's step type identifier
func (b *BaseExecutor) GetStepType() string {
	return b.stepType
}

// GetMetadata returns the executor's metadata
func (b *BaseExecutor) GetMetadata() models.ExecutorMetadata {
	return b.metadata
}

// PreExecute performs common pre-execution checks (Template Method)
func (b *BaseExecutor) PreExecute(ctx context.Context, step *models.TransformationStep) error {
	// Check context timeout
	if ctx.Err() != nil {
		return fmt.Errorf("context error: %w", ctx.Err())
	}

	// Validate step is enabled
	if !step.Enabled {
		return fmt.Errorf("step is disabled")
	}

	// Validate step type matches
	if step.StepType != b.stepType {
		return fmt.Errorf("step type mismatch: expected %s, got %s", b.stepType, step.StepType)
	}

	log.Printf("🔄 [%s] Starting execution: %s", b.stepType, step.StepName)
	return nil
}

// PostExecute performs common post-execution logging (Template Method)
func (b *BaseExecutor) PostExecute(ctx context.Context, step *models.TransformationStep, err error, duration time.Duration) {
	if err != nil {
		log.Printf("❌ [%s] Failed after %v: %v", step.StepName, duration, err)
	} else {
		log.Printf("✅ [%s] Completed in %v", step.StepName, duration)
	}
}

// ValidateConfig is a helper for validating required config fields
func (b *BaseExecutor) ValidateConfig(step *models.TransformationStep, requiredFields []string) error {
	if step.Config == nil {
		return fmt.Errorf("config is required")
	}

	for _, field := range requiredFields {
		if _, exists := step.Config[field]; !exists {
			return fmt.Errorf("required config field missing: %s", field)
		}
	}

	return nil
}

// ===============================================================
// STEP OUTPUT MANAGEMENT
// ===============================================================

// SetStepOutput stores step-specific output data in the execution context
func (b *BaseExecutor) SetStepOutput(
	execContext *models.PipelineExecutionContext,
	step *models.TransformationStep,
	outputData map[string]interface{},
) {
	namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)

	alias := ""
	if step.StepAlias != nil {
		alias = *step.StepAlias
	} else {
		alias = models.GenerateDefaultAlias(step.StepName)
	}

	execContext.StepOutputs[namespace] = models.StepOutput{
		StepID:     step.ID,
		StepName:   step.StepName,
		StepAlias:  alias,
		StepType:   step.StepType,
		Namespace:  namespace,
		Sequence:   step.Sequence,
		OutputData: outputData,
		Success:    true, // Will be updated by pipeline service if error occurs
	}
}

// GetStepOutput retrieves step output by namespace
func (b *BaseExecutor) GetStepOutput(
	execContext *models.PipelineExecutionContext,
	namespace string,
) (map[string]interface{}, bool) {
	if output, exists := execContext.StepOutputs[namespace]; exists {
		return output.OutputData, true
	}
	return nil, false
}

// GetStepOutputByAlias retrieves step output by user-friendly alias
func (b *BaseExecutor) GetStepOutputByAlias(
	execContext *models.PipelineExecutionContext,
	alias string,
) (map[string]interface{}, error) {
	output, err := execContext.GetStepOutputByAlias(alias)
	if err != nil {
		return nil, err
	}
	return output.OutputData, nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// GetNestedValue retrieves a value from a nested map using dot notation and array indices
// Supports paths like:
//   - "enhancedSegments.PID.fields[4].subfields[1].value"
//   - "patient.name"
//   - "metadata.timestamp"
func GetNestedValue(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	// Try direct key first (for simple paths)
	if val, ok := data[path]; ok {
		return val
	}

	// Auto-lookup HL7 field keys directly from enhanced schema
	// Example: "PID.3" looks up enhancedSegments["PID"].Fields for Key="PID.3"
	// Example: "PID.5.1" looks up field PID.5 then searches its Subfields for Key="PID.5.1"
	if isHL7FieldKey(path) {
		return getHL7FieldValue(data, path)
	}

	// LEGACY SUPPORT: Convert old format to new format automatically
	// "enhancedSegments.MSH.fields[2].value" -> "MSH.2"
	// "enhancedSegments.PID.fields[3].subfields[0].value" -> "PID.3.1"
	if strings.HasPrefix(path, "enhancedSegments.") {
		convertedKey := convertLegacyPathToHL7Key(path)
		if convertedKey != "" && isHL7FieldKey(convertedKey) {
			log.Printf("🔄 [LEGACY] Auto-converting path '%s' to '%s'", path, convertedKey)
			return getHL7FieldValue(data, convertedKey)
		}
	}

	// Parse path with dot notation and array indices
	// Example: "enhancedSegments.PID.fields[4].subfields[1].value"
	// Special handling: Don't split on dots inside brackets
	current := interface{}(data)
	parts := splitPathRespectingBrackets(path)

	for _, part := range parts {
		// Check if part has bracket notation like "fields[4]" or "fields[PID.3]"
		if strings.Contains(part, "[") {
			// Extract key and index/key: "fields[4]" -> "fields", "4"
			openBracket := strings.Index(part, "[")
			closeBracket := strings.Index(part, "]")

			if openBracket == -1 || closeBracket == -1 {
				return nil // Invalid syntax
			}

			key := part[:openBracket]
			indexOrKey := part[openBracket+1 : closeBracket]

			// Navigate to the key first
			if currentMap, ok := current.(map[string]interface{}); ok {
				val, exists := currentMap[key]
				if !exists {
					return nil
				}

				// Try as array index first (integer)
				if index, err := strconv.Atoi(indexOrKey); err == nil {
					// It's an integer - treat as array index
					if arraySlice, ok := val.([]interface{}); ok {
						if index < 0 || index >= len(arraySlice) {
							return nil // Index out of bounds
						}
						current = arraySlice[index]
					} else {
						return nil // Not an array
					}
				} else {
					// It's a string - treat as map key
					if mapVal, ok := val.(map[string]interface{}); ok {
						subVal, exists := mapVal[indexOrKey]
						if !exists {
							return nil
						}
						current = subVal
					} else {
						return nil // Not a map
					}
				}
			} else {
				return nil
			}
		} else {
			// Simple key navigation
			if currentMap, ok := current.(map[string]interface{}); ok {
				val, exists := currentMap[part]
				if !exists {
					return nil
				}
				current = val
			} else {
				return nil
			}
		}
	}

	return current
}

// EnsureMapExists ensures a nested map path exists
func EnsureMapExists(data map[string]interface{}, path string) map[string]interface{} {
	if existing, ok := data[path]; ok {
		if existingMap, isMap := existing.(map[string]interface{}); isMap {
			return existingMap
		}
	}

	newMap := make(map[string]interface{})
	data[path] = newMap
	return newMap
}

// isHL7FieldKey checks if a path looks like an HL7 field key (e.g., "PID.3", "MSH.9", "PID.5.1")
func isHL7FieldKey(path string) bool {
	// HL7 field keys have format: SEGMENT.FIELD or SEGMENT.FIELD.SUBFIELD
	// Examples: PID.3, MSH.9, PID.5.1, PID.13.4
	parts := strings.Split(path, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	
	// First part should be 3-letter segment name (uppercase)
	segment := parts[0]
	if len(segment) != 3 {
		return false
	}
	for _, c := range segment {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	
	// Second part should be a number (field position)
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return false
	}
	
	// Third part (if exists) should be a number (subfield position)
	if len(parts) == 3 {
		if _, err := strconv.Atoi(parts[2]); err != nil {
			return false
		}
	}
	
	return true
}

// getHL7FieldValue retrieves a field value directly from the enhanced schema structure
// This function works with the actual Go struct types (map[string]hl7.EnhancedSegment)
// instead of trying to use generic path resolution
//
// Examples:
//   - "PID.3" -> searches enhancedSegments["PID"].Fields for Key="PID.3" and returns Value
//   - "PID.5.1" -> searches enhancedSegments["PID"].Fields for Key="PID.5", then Subfields for Key="PID.5.1"
//   - "MSH.9" -> searches enhancedSegments["MSH"].Fields for Key="MSH.9" and returns Value
func getHL7FieldValue(data map[string]interface{}, fieldKey string) interface{} {
	// 1. Type-assert enhancedSegments as map[string]hl7.EnhancedSegment
	enhancedSegsRaw, ok := data["enhancedSegments"]
	if !ok {
		return nil
	}

	enhancedSegs, ok := enhancedSegsRaw.(map[string]hl7.EnhancedSegment)
	if !ok {
		return nil
	}

	// 2. Parse field key to extract segment name
	parts := strings.Split(fieldKey, ".")
	if len(parts) < 2 {
		return nil
	}

	segmentName := parts[0] // e.g., "PID"

	// 3. Get the segment
	segment, ok := enhancedSegs[segmentName]
	if !ok {
		return nil
	}

	// 4. Search Fields array for matching Key
	// Handle both field-level (PID.3) and subfield-level (PID.5.1) access
	if len(parts) == 2 {
		// Field-level access: PID.3
		for _, field := range segment.Fields {
			if field.Key == fieldKey {
				return field.Value
			}
		}
		return nil
	}

	// Subfield-level access: PID.5.1
	// First find the parent field (PID.5)
	parentFieldKey := parts[0] + "." + parts[1] // e.g., "PID.5"
	for _, field := range segment.Fields {
		if field.Key == parentFieldKey {
			// Found parent field, now search its subfields
			for _, subfield := range field.Subfields {
				if subfield.Key == fieldKey {
					return subfield.Value
				}
			}
			return nil
		}
	}

	return nil
}

// splitPathRespectingBrackets splits a path on dots, but ignores dots inside brackets
// Example: "fields[PID.3].value" -> ["fields[PID.3]", "value"]
func splitPathRespectingBrackets(path string) []string {
	var parts []string
	var current strings.Builder
	insideBrackets := 0

	for _, char := range path {
		if char == '[' {
			insideBrackets++
			current.WriteRune(char)
		} else if char == ']' {
			insideBrackets--
			current.WriteRune(char)
		} else if char == '.' && insideBrackets == 0 {
			// Only split on dots outside of brackets
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(char)
		}
	}

	// Add the last part
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// convertLegacyPathToHL7Key converts old path format to new HL7 field key format
// Examples:
//   - "enhancedSegments.MSH.fields[2].value" -> "MSH.2"
//   - "enhancedSegments.PID.fields[3].value" -> "PID.3"
//   - "enhancedSegments.PID.fields[5].subfields[0].value" -> "PID.5.1"
//
// LIMITATION: Cannot reliably convert array indices to field numbers without data access
// because fields array is indexed by position, not field number.
// The actual field key (PID.3, MSH.9) is stored in Field.Key property.
// This function exists for documentation purposes but returns empty string.
// Users should use simple HL7 field keys (PID.3, MSH.9) directly in validation rules.
func convertLegacyPathToHL7Key(path string) string {
	// Cannot convert without accessing the actual data structure
	// UI has been updated to generate simple keys directly
	return ""
}

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// SetNestedValue sets a value in a nested map using dot notation
// Creates intermediate maps as needed
// Example: SetNestedValue(data, "enriched.api.patient", value) creates enriched -> api -> patient
func SetNestedValue(data map[string]interface{}, path string, value interface{}) {
	if path == "" {
		return
	}

	parts := strings.Split(path, ".")
	current := data

	// Navigate to the parent of the target field, creating maps as needed
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		// Check if this part exists
		if existing, ok := current[part]; ok {
			// If it exists and is a map, continue navigating
			if existingMap, ok := existing.(map[string]interface{}); ok {
				current = existingMap
			} else {
				// Overwrite non-map values with a new map
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			}
		} else {
			// Create new map for this part
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}

	// Set the final value
	finalKey := parts[len(parts)-1]
	current[finalKey] = value
}
