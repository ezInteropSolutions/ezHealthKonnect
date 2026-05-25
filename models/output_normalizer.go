package models

import (
	"fmt"
	"reflect"
	"strings"
)

// OutputNormalizer provides centralized output normalization following OOP principles
// Single Responsibility: Normalize all output data to consistent snake_case convention
// Open/Closed: Extensible through configuration without modifying core logic
type OutputNormalizer struct {
	// Convention to use: "snake_case", "camelCase", or "kebab-case"
	Convention string

	// Custom mappings for special cases (e.g., "ID" -> "id" not "i_d")
	CustomMappings map[string]string
}

// NewOutputNormalizer creates a new normalizer with default snake_case convention
func NewOutputNormalizer() *OutputNormalizer {
	return &OutputNormalizer{
		Convention: "snake_case",
		CustomMappings: map[string]string{
			// Common acronyms and special cases
			"ID":          "id",
			"URL":         "url",
			"URI":         "uri",
			"HTTP":        "http",
			"HTTPS":       "https",
			"API":         "api",
			"JSON":        "json",
			"XML":         "xml",
			"HTML":        "html",
			"SQL":         "sql",
			"HL7":         "hl7",
			"FHIR":        "fhir",
			"UUID":        "uuid",
			"SSN":         "ssn",
			"MRN":         "mrn",
			"NPI":         "npi",
			"PHI":         "phi",
			"HIPAA":       "hipaa",
			"insuranceId": "insurance_id", // Database column name
		},
	}
}

// NormalizeMap normalizes all keys in a map to the configured convention.
// Recursively processes nested maps and arrays with depth protection.
func (n *OutputNormalizer) NormalizeMap(input map[string]interface{}) map[string]interface{} {
	return n.normalizeMapDepth(input, 0)
}

// NormalizeKey converts a single key to the configured convention
// PRESERVES HL7-style keys (e.g., PID.13.4) for user-friendly output display
func (n *OutputNormalizer) NormalizeKey(key string) string {
	// Check custom mappings first
	if mapped, exists := n.CustomMappings[key]; exists {
		return mapped
	}

	// PRESERVE HL7-style keys (e.g., PID.13.4, MSH.9, OBX.5.1)
	// These are user-provided field references that should remain readable
	if n.isHL7FieldKey(key) {
		return key
	}

	// Already in snake_case? Return as-is
	if n.isSnakeCase(key) {
		return key
	}

	// Convert to snake_case
	return n.toSnakeCase(key)
}

// isHL7FieldKey checks if a key looks like an HL7 field reference
// Matches patterns like: PID.3, MSH.9, PID.5.1, OBX.5.2, IN1.2
// Format: SEGMENT.FIELD or SEGMENT.FIELD.COMPONENT
func (n *OutputNormalizer) isHL7FieldKey(key string) bool {
	parts := strings.Split(key, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// First part: 2-4 uppercase letters (segment name)
	// Common: MSH, PID, PV1, OBX, ORC, IN1, NK1, GT1, DG1, PR1, AL1, ZDS
	segment := parts[0]
	if len(segment) < 2 || len(segment) > 4 {
		return false
	}
	for _, c := range segment {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}

	// Second part: must be a number (field position)
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if len(parts[1]) == 0 {
		return false
	}

	// Third part (if exists): must be a number (component position)
	if len(parts) == 3 {
		for _, c := range parts[2] {
			if c < '0' || c > '9' {
				return false
			}
		}
		if len(parts[2]) == 0 {
			return false
		}
	}

	return true
}

// NormalizeValue recursively normalizes nested structures.
// A depth guard (max 30 levels) prevents stack overflow on deeply nested or
// circular structures — e.g., from user-provided JavaScript script output.
func (n *OutputNormalizer) NormalizeValue(value interface{}) interface{} {
	return n.normalizeValueDepth(value, 0)
}

const maxNormalizeDepth = 30

func (n *OutputNormalizer) normalizeValueDepth(value interface{}, depth int) interface{} {
	if value == nil {
		return nil
	}
	if depth > maxNormalizeDepth {
		return value // Stop recursing; return the raw value rather than panic
	}

	// Use reflection to handle different types
	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Map:
		if m, ok := value.(map[string]interface{}); ok {
			return n.normalizeMapDepth(m, depth+1)
		}
		return value

	case reflect.Slice, reflect.Array:
		if arr, ok := value.([]interface{}); ok {
			result := make([]interface{}, len(arr))
			for i, item := range arr {
				result[i] = n.normalizeValueDepth(item, depth+1)
			}
			return result
		}
		// Handle any Go-native typed slice via reflection (covers []map[string]interface{},
		// []string, []int, etc. — whatever the FHIR/HL7 service returns as concrete types).
		// v is already reflect.ValueOf(value) from the outer switch.
		length := v.Len()
		result := make([]interface{}, length)
		for i := 0; i < length; i++ {
			result[i] = n.normalizeValueDepth(v.Index(i).Interface(), depth+1)
		}
		return result

	default:
		return value
	}
}

// normalizeMapDepth is the depth-aware version of NormalizeMap.
func (n *OutputNormalizer) normalizeMapDepth(input map[string]interface{}, depth int) map[string]interface{} {
	if input == nil {
		return nil
	}
	result := make(map[string]interface{}, len(input))
	for key, value := range input {
		result[n.NormalizeKey(key)] = n.normalizeValueDepth(value, depth)
	}
	return result
}

// toSnakeCase converts camelCase or PascalCase to snake_case
// Also handles spaces, hyphens, and special characters
func (n *OutputNormalizer) toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	var result strings.Builder
	result.Grow(len(s) + 5) // Pre-allocate with some buffer

	prevWasUnderscore := false

	for i, r := range s {
		// Replace spaces, hyphens, and arrows with underscores
		if r == ' ' || r == '-' || r == '→' {
			// Only add underscore if we haven't just added one
			if !prevWasUnderscore && i > 0 {
				result.WriteRune('_')
				prevWasUnderscore = true
			}
			continue
		}

		// Skip special characters
		if !isUpper(r) && !isLower(r) && r != '_' && (r < '0' || r > '9') {
			continue
		}

		if i > 0 && isUpper(r) {
			// Check if previous char is lowercase or next char is lowercase
			// This handles acronyms like "URLPath" -> "url_path" not "u_r_l_path"
			prevChar := rune(s[i-1])
			prevIsLower := isLower(prevChar)
			prevIsSpace := prevChar == ' ' || prevChar == '-'
			nextIsLower := i < len(s)-1 && isLower(rune(s[i+1]))

			// Add underscore before capital if needed (but not after space/hyphen)
			if (prevIsLower || nextIsLower) && !prevIsSpace && !prevWasUnderscore {
				result.WriteRune('_')
				prevWasUnderscore = true
			}
		}

		result.WriteRune(toSnakeCaseLower(r))
		prevWasUnderscore = (r == '_')
	}

	return result.String()
}

// isSnakeCase checks if a string is already in snake_case
func (n *OutputNormalizer) isSnakeCase(s string) bool {
	for _, r := range s {
		if isUpper(r) {
			return false
		}
	}
	return true
}

// Helper functions
func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func toSnakeCaseLower(r rune) rune {
	if isUpper(r) {
		return r + 32
	}
	return r
}

// NormalizeStepOutput is a convenience method for normalizing step outputs
// Flattens ALL nested structures AND normalizes keys.
// The "result" key is treated specially: its value is preserved without
// recursive key normalization because it contains user-controlled data
// (e.g. pivot column names like "GLUC", ICD codes like "E11.9",
// caseTransform=upper output like "PATIENT_ID"). Normalizing these would
// silently corrupt the user's intended output.
func (n *OutputNormalizer) NormalizeStepOutput(output map[string]interface{}) map[string]interface{} {
	// First flatten enrichment structures (enriched.xxx pattern)
	flattened := n.flattenEnrichment(output)

	// Then flatten any other wrapper keys (mapped_fields, field_results, etc.)
	unwrapped := n.flattenWrapperKeys(flattened)

	// Extract keys whose values must NOT have their internal keys snake_cased.
	// - "result": user-controlled pivot/transform output (column names like "GLUC", ICD codes)
	// - "fhirBundle": FHIR R4 resource tree — all keys MUST remain camelCase per the FHIR spec
	//   (e.g. resourceType, birthDate, effectiveDateTime). Normalizing them breaks FHIR validators.
	preserved := map[string]interface{}{}
	for _, key := range []string{"result", "fhirBundle"} {
		if v, ok := unwrapped[key]; ok {
			preserved[key] = v
			delete(unwrapped, key)
		}
	}

	// Normalize all metadata keys (result_count, operation, duration_ms, etc.)
	normalized := n.NormalizeMap(unwrapped)

	// Restore preserved keys without key normalization
	for key, val := range preserved {
		normalized[key] = val
	}

	return normalized
}

// flattenEnrichment extracts data from nested enrichment structures
// Converts: { "enriched": { "database": [{"insuranceId": "..."}] } }
// To:       { "insuranceId": "..." }
// Also handles: { "enriched": { "database": {"insuranceId": "..."} } }
// Returns empty map if enrichment has no data (empty array or null)
func (n *OutputNormalizer) flattenEnrichment(output map[string]interface{}) map[string]interface{} {
	// Check for enriched.database pattern (database enrichment)
	if enriched, ok := output["enriched"].(map[string]interface{}); ok {
		fmt.Printf("🔍 flattenEnrichment: found 'enriched' key with %d subkeys: %v\n", len(enriched), getEnrichedKeys(enriched))

		// Check what type database actually is
		if dbValue, exists := enriched["database"]; exists {
			fmt.Printf("  🔍 database type: %T, value: %v\n", dbValue, dbValue)
		}

		// Try database as []interface{} (standard JSON array)
		if database, ok := enriched["database"].([]interface{}); ok {
			fmt.Printf("  ✅ Found database as []interface{} with %d items\n", len(database))
			if len(database) > 0 {
				// Flatten first record from database array
				if record, ok := database[0].(map[string]interface{}); ok {
					return record
				}
			}
			// Empty array - return empty map instead of keeping "enriched" key
			return make(map[string]interface{})
		}

		// Try database as []map[string]interface{} (Go native type - postgres enrichment)
		if database, ok := enriched["database"].([]map[string]interface{}); ok {
			fmt.Printf("  ✅ Found database as []map[string]interface{} with %d items\n", len(database))
			if len(database) > 0 {
				// Flatten first record from database array
				return database[0]
			}
			// Empty array - return empty map
			return make(map[string]interface{})
		}

		// Try database as object (some enrichments return direct object)
		if database, ok := enriched["database"].(map[string]interface{}); ok {
			if len(database) > 0 {
				return database
			}
			// Empty object - return empty map
			return make(map[string]interface{})
		}

		// Try api as array
		if api, ok := enriched["api"].([]interface{}); ok {
			if len(api) > 0 {
				if record, ok := api[0].(map[string]interface{}); ok {
					return record
				}
			}
			// Empty array - return empty map
			return make(map[string]interface{})
		}

		// Try api as object
		if api, ok := enriched["api"].(map[string]interface{}); ok {
			if len(api) > 0 {
				return api
			}
			// Empty object - return empty map
			return make(map[string]interface{})
		}

		// Generic fallback: check for any enrichment type
		for key, enrichData := range enriched {
			// Skip null values - return empty map
			if enrichData == nil {
				return make(map[string]interface{})
			}

			// Try as array
			if dataArray, ok := enrichData.([]interface{}); ok {
				if len(dataArray) > 0 {
					if record, ok := dataArray[0].(map[string]interface{}); ok {
						return record
					}
				}
				// Empty array - return empty map
				return make(map[string]interface{})
			}

			// Try as object
			if dataObject, ok := enrichData.(map[string]interface{}); ok {
				if len(dataObject) > 0 {
					return dataObject
				}
				// Empty object - return empty map
				return make(map[string]interface{})
			}

			// Log unhandled enrichment type for debugging
			fmt.Printf("⚠️  Unhandled enrichment type '%s': %T\n", key, enrichData)
		}
	}

	// No flattening needed
	return output
}

// String implements Stringer interface for debugging
func (n *OutputNormalizer) String() string {
	return fmt.Sprintf("OutputNormalizer{convention=%s, custom_mappings=%d}",
		n.Convention, len(n.CustomMappings))
}

// Helper function to get enriched keys for debugging
func getEnrichedKeys(enriched map[string]interface{}) []string {
	keys := make([]string, 0, len(enriched))
	for k := range enriched {
		keys = append(keys, k)
	}
	return keys
}

// flattenWrapperKeys unwraps nested data wrapper keys
// For field_mapping: { field_count: 5, mapped_fields: {a:1, b:2}, transformation: "..." }
//   → Extract mapped_fields contents and merge with other keys
// For field_validation: { valid: true, field_results: [...], error_count: 0 }
//   → Keep field_results as-is (array needs context)
func (n *OutputNormalizer) flattenWrapperKeys(output map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Keys that should have their contents extracted and merged
	extractKeys := map[string]bool{
		"mapped_fields": true, // Extract object contents and merge
	}

	for key, value := range output {
		// Should we extract the contents of this key?
		if extractKeys[key] {
			// If it's an object, extract its contents and merge into result
			if objValue, isObject := value.(map[string]interface{}); isObject {
				fmt.Printf("🔓 Extracting contents of '%s' (%d fields)\n", key, len(objValue))
				for k, v := range objValue {
					result[k] = v
				}
				continue
			}
		}

		// Keep this key as-is
		result[key] = value
	}

	return result
}
