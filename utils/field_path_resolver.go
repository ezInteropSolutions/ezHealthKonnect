// FILE: utils/field_path_resolver.go
// User-friendly field path resolver for HL7 enhancedSegments
// Converts user-friendly paths (PID.5.1) to actual values without array indices

package utils

import (
	"fmt"
	"strings"
)

// ===============================================================
// FIELD PATH RESOLVER
// ===============================================================

// ResolveFieldPath resolves a user-friendly HL7 field path to its value
// Supports paths like:
//   - "PID.5" → returns field value
//   - "PID.5.1" → returns subfield value
//   - "MSH.9" → returns field value
//
// This searches the enhancedSegments structure by matching the "key" property
// instead of requiring users to know array indices.
//
// Example:
//   Input: data = parsed HL7 message, path = "PID.5.1"
//   Output: "MOUSE" (Family Name value)
func ResolveFieldPath(data map[string]interface{}, userPath string) interface{} {
	if userPath == "" {
		return nil
	}

	// Parse the user path: "PID.5.1" → ["PID", "5", "1"]
	parts := strings.Split(userPath, ".")
	if len(parts) < 2 {
		return nil // Invalid path (need at least segment.field)
	}

	segmentName := parts[0] // "PID"
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1]) // "PID.5"

	// Navigate to enhancedSegments
	enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Get the segment
	segment, ok := enhancedSegments[segmentName].(map[string]interface{})
	if !ok {
		return nil
	}

	// Get the fields array
	fieldsInterface, ok := segment["fields"]
	if !ok {
		return nil
	}

	fields, ok := fieldsInterface.([]interface{})
	if !ok {
		return nil
	}

	// Search for the field by matching its "key" property
	for _, fieldInterface := range fields {
		field, ok := fieldInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is the field we're looking for
		if field["key"] == fieldKey {
			// Found the field!

			if len(parts) == 2 {
				// Field-level access: "PID.5"
				// Return the field value
				if value, exists := field["value"]; exists {
					return value
				}
				return nil
			}

			// Subfield-level access: "PID.5.1"
			subfieldKey := userPath // Full key: "PID.5.1"

			// Get the subfields array
			subfieldsInterface, ok := field["subfields"]
			if !ok {
				return nil
			}

			subfields, ok := subfieldsInterface.([]interface{})
			if !ok {
				return nil
			}

			// Search for the subfield by matching its "key" property
			for _, subfieldInterface := range subfields {
				subfield, ok := subfieldInterface.(map[string]interface{})
				if !ok {
					continue
				}

				if subfield["key"] == subfieldKey {
					// Found the subfield!
					if value, exists := subfield["value"]; exists {
						return value
					}
					return nil
				}
			}

			return nil // Subfield not found
		}
	}

	return nil // Field not found
}

// IsUserFriendlyPath checks if a path is user-friendly format (e.g., "PID.5.1")
// vs array-based format (e.g., "enhancedSegments.PID.fields[4].value")
func IsUserFriendlyPath(path string) bool {
	// User-friendly paths:
	// - Start with segment name (3 uppercase letters)
	// - Don't contain "enhancedSegments"
	// - Don't contain array notation "[...]"

	if strings.Contains(path, "enhancedSegments") {
		return false
	}

	if strings.Contains(path, "[") {
		return false
	}

	// Check if it matches pattern: SEGMENT.FIELD or SEGMENT.FIELD.COMPONENT
	parts := strings.Split(path, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// First part should be 2-4 uppercase letters (segment name)
	segmentName := parts[0]
	if len(segmentName) < 2 || len(segmentName) > 4 {
		return false
	}

	for _, c := range segmentName {
		if c < 'A' || c > 'Z' {
			return false
		}
	}

	return true
}

// GetFieldMetadata returns metadata for a field path (name, description, dataType)
// Used by autocomplete and validation to show field information
func GetFieldMetadata(data map[string]interface{}, userPath string) map[string]interface{} {
	if userPath == "" {
		return nil
	}

	parts := strings.Split(userPath, ".")
	if len(parts) < 2 {
		return nil
	}

	segmentName := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])

	enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{})
	if !ok {
		return nil
	}

	segment, ok := enhancedSegments[segmentName].(map[string]interface{})
	if !ok {
		return nil
	}

	fieldsInterface, ok := segment["fields"]
	if !ok {
		return nil
	}

	fields, ok := fieldsInterface.([]interface{})
	if !ok {
		return nil
	}

	for _, fieldInterface := range fields {
		field, ok := fieldInterface.(map[string]interface{})
		if !ok {
			continue
		}

		if field["key"] == fieldKey {
			if len(parts) == 2 {
				// Return field metadata
				return map[string]interface{}{
					"key":         field["key"],
					"name":        field["name"],
					"description": field["description"],
					"dataType":    field["dataType"],
					"position":    field["position"],
				}
			}

			// Subfield metadata
			subfieldKey := userPath
			subfieldsInterface, ok := field["subfields"]
			if !ok {
				return nil
			}

			subfields, ok := subfieldsInterface.([]interface{})
			if !ok {
				return nil
			}

			for _, subfieldInterface := range subfields {
				subfield, ok := subfieldInterface.(map[string]interface{})
				if !ok {
					continue
				}

				if subfield["key"] == subfieldKey {
					return map[string]interface{}{
						"key":         subfield["key"],
						"name":        subfield["name"],
						"description": subfield["description"],
						"dataType":    subfield["dataType"],
						"position":    subfield["position"],
					}
				}
			}
		}
	}

	return nil
}
