// services/executors/field_utils.go
// Format-agnostic field utilities for reading and updating message fields
// Supports: HL7v2, FHIR, generic JSON paths
//
// OOP Pattern: Strategy Pattern - different resolvers for different formats
// DRY Principle: Single implementation used by all executors

package executors

import (
	"ezhealthkonnect/hl7"
	"fmt"
	"strconv"
	"strings"
)

// ===============================================================
// FORMAT-AGNOSTIC FIELD PATH UTILITIES
// ===============================================================

// FieldPathType represents the type of field path notation
type FieldPathType string

const (
	PathTypeHL7     FieldPathType = "hl7"     // e.g., PID.3, MSH.9.1
	PathTypeFHIR    FieldPathType = "fhir"    // e.g., Patient.name[0].given
	PathTypeJSON    FieldPathType = "json"    // e.g., data.patient.name
	PathTypeUnknown FieldPathType = "unknown"
)

// DetectPathType determines the type of field path notation
func DetectPathType(path string) FieldPathType {
	if IsHL7FieldPath(path) {
		return PathTypeHL7
	}
	if IsFHIRPath(path) {
		return PathTypeFHIR
	}
	// Default to JSON path
	return PathTypeJSON
}

// IsHL7FieldPath checks if path is HL7 field notation (e.g., PID.3, MSH.9.1)
func IsHL7FieldPath(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// First part: 2-4 uppercase letters/numbers (segment name)
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
	if len(parts[1]) == 0 {
		return false
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Third part (if exists): must be a number (component position)
	if len(parts) == 3 {
		if len(parts[2]) == 0 {
			return false
		}
		for _, c := range parts[2] {
			if c < '0' || c > '9' {
				return false
			}
		}
	}

	return true
}

// IsFHIRPath checks if path looks like FHIR resource path
func IsFHIRPath(path string) bool {
	// FHIR paths typically start with resource type (Patient, Observation, etc.)
	fhirResources := []string{
		"Patient", "Observation", "Encounter", "Condition", "Procedure",
		"MedicationRequest", "DiagnosticReport", "AllergyIntolerance",
		"Immunization", "CarePlan", "Goal", "ServiceRequest",
	}

	for _, resource := range fhirResources {
		if strings.HasPrefix(path, resource+".") {
			return true
		}
	}
	return false
}

// ===============================================================
// FORMAT-AGNOSTIC FIELD VALUE GETTER
// ===============================================================

// GetFieldValue retrieves a field value from message data using format-agnostic path
// Automatically detects path type and uses appropriate resolver
func GetFieldValue(data map[string]interface{}, path string) interface{} {
	pathType := DetectPathType(path)

	switch pathType {
	case PathTypeHL7:
		return resolveHL7FieldValue(data, path)
	case PathTypeFHIR:
		return resolveFHIRFieldValue(data, path)
	default:
		return resolveJSONPathValue(data, path)
	}
}

// resolveHL7FieldValue retrieves value from HL7 enhanced segments (format-agnostic version)
// Supports both typed Go structs and map[string]interface{} formats
func resolveHL7FieldValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return nil
	}

	segmentKey := parts[0]
	fieldPosition := 0
	fmt.Sscanf(parts[1], "%d", &fieldPosition)

	// Find enhancedSegments
	var enhancedSegments interface{}
	if es, ok := data["enhancedSegments"]; ok {
		enhancedSegments = es
	} else if msg, ok := data["message"].(map[string]interface{}); ok {
		enhancedSegments = msg["enhancedSegments"]
	}

	if enhancedSegments == nil {
		return nil
	}

	// Handle typed Go struct
	if typedSegments, ok := enhancedSegments.(map[string]hl7.EnhancedSegment); ok {
		segment, exists := typedSegments[segmentKey]
		if !exists {
			return nil
		}

		fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])
		for _, field := range segment.Fields {
			if field.Key == fieldKey {
				if len(parts) == 2 {
					return field.Value
				}
				// Subfield access
				subfieldKey := path
				for _, sf := range field.Subfields {
					if sf.Key == subfieldKey {
						return sf.Value
					}
				}
			}
		}
		return nil
	}

	// Handle map[string]interface{}
	if segmentsMap, ok := enhancedSegments.(map[string]interface{}); ok {
		return resolveHL7FieldFromMap(segmentsMap, parts)
	}

	return nil
}

// resolveHL7FieldFromMap retrieves HL7 field from map structure
func resolveHL7FieldFromMap(segments map[string]interface{}, parts []string) interface{} {
	segmentKey := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])

	segmentData, exists := segments[segmentKey]
	if !exists {
		return nil
	}

	segmentMap, ok := segmentData.(map[string]interface{})
	if !ok {
		return nil
	}

	fields, ok := segmentMap["fields"].([]interface{})
	if !ok {
		return nil
	}

	for _, f := range fields {
		fieldMap, ok := f.(map[string]interface{})
		if !ok {
			continue
		}

		if fieldMap["key"] == fieldKey {
			if len(parts) == 2 {
				return fieldMap["value"]
			}
			// Subfield access
			subfieldKey := strings.Join(parts, ".")
			subfields, ok := fieldMap["subfields"].([]interface{})
			if !ok {
				return fieldMap["value"]
			}

			for _, sf := range subfields {
				subfieldMap, ok := sf.(map[string]interface{})
				if !ok {
					continue
				}
				if subfieldMap["key"] == subfieldKey {
					return subfieldMap["value"]
				}
			}
		}
	}

	return nil
}

// resolveFHIRFieldValue retrieves value from FHIR resource structure
func resolveFHIRFieldValue(data map[string]interface{}, path string) interface{} {
	// FHIR resources can be at root or under "fhirBundle" or "resource"
	var fhirData map[string]interface{}

	if bundle, ok := data["fhirBundle"].(map[string]interface{}); ok {
		fhirData = bundle
	} else if resource, ok := data["resource"].(map[string]interface{}); ok {
		fhirData = resource
	} else {
		fhirData = data
	}

	return resolveJSONPathValue(fhirData, path)
}

// resolveJSONPathValue retrieves value using dot notation with array support
func resolveJSONPathValue(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	// Try direct key first
	if val, ok := data[path]; ok {
		return val
	}

	// Parse path with array indices support
	parts := parseJSONPath(path)
	var current interface{} = data

	for _, part := range parts {
		if part.isArray {
			// Array access
			if arr, ok := current.([]interface{}); ok {
				if part.index >= 0 && part.index < len(arr) {
					current = arr[part.index]
				} else {
					return nil
				}
			} else {
				return nil
			}
		} else {
			// Map access
			if currentMap, ok := current.(map[string]interface{}); ok {
				current = currentMap[part.key]
				if current == nil {
					return nil
				}
			} else {
				return nil
			}
		}
	}

	return current
}

// pathPart represents a part of a JSON path
type pathPart struct {
	key     string
	isArray bool
	index   int
}

// parseJSONPath parses a JSON path into parts
// Supports: "data.items[0].name" -> [{key:"data"}, {key:"items"}, {isArray:true, index:0}, {key:"name"}]
func parseJSONPath(path string) []pathPart {
	var parts []pathPart
	var current strings.Builder

	for i := 0; i < len(path); i++ {
		c := path[i]

		if c == '.' {
			if current.Len() > 0 {
				parts = append(parts, pathPart{key: current.String()})
				current.Reset()
			}
		} else if c == '[' {
			if current.Len() > 0 {
				parts = append(parts, pathPart{key: current.String()})
				current.Reset()
			}
			// Parse array index
			i++
			var indexStr strings.Builder
			for i < len(path) && path[i] != ']' {
				indexStr.WriteByte(path[i])
				i++
			}
			if idx, err := strconv.Atoi(indexStr.String()); err == nil {
				parts = append(parts, pathPart{isArray: true, index: idx})
			}
		} else {
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, pathPart{key: current.String()})
	}

	return parts
}

// ===============================================================
// FORMAT-AGNOSTIC LEAF PATH EXPANSION
// ===============================================================

// GetLeafPaths returns all terminal (primitive-value) paths reachable from path.
//
// Design intent — called by DataMaskingExecutor before masking so that a single
// rule targeting a composite/container path automatically covers every nested
// field, regardless of the message format:
//
//   HL7v2  PID.5      → ["PID.5", "PID.5.1", "PID.5.2", "PID.5.3", ...]
//   HL7v2  PID.5.1    → ["PID.5.1"]          (already a leaf, no expansion)
//   FHIR   Patient.name[0]  → ["Patient.name[0].family", "Patient.name[0].given"]
//   JSON   patient.address  → ["patient.address.street", "patient.address.city", ...]
//   JSON   patient.lastName → ["patient.lastName"]   (scalar → no expansion)
//
// Returns nil when the path cannot be resolved in data (field absent).
func GetLeafPaths(data map[string]interface{}, path string) []string {
	switch DetectPathType(path) {
	case PathTypeHL7:
		return getHL7LeafPaths(data, path)
	default:
		// FHIR paths resolve through the same JSON walk after the resource root
		// is located; generic JSON paths are walked directly.
		return getJSONLeafPaths(data, path)
	}
}

// getHL7LeafPaths expands an HL7 path to all reachable leaf paths.
// A 3-part path (PID.5.1) is already a leaf — returned as-is.
// A 2-part path (PID.5)   is a composite field — expanded to include its subfields.
func getHL7LeafPaths(data map[string]interface{}, path string) []string {
	parts := strings.Split(path, ".")

	// Already a subfield path (PID.5.1) — terminal leaf.
	if len(parts) == 3 {
		if GetFieldValue(data, path) != nil {
			return []string{path}
		}
		return nil
	}

	if len(parts) != 2 {
		return nil
	}

	segmentKey := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])

	// Locate enhancedSegments — same discovery logic as resolveHL7FieldValue.
	var enhancedSegments interface{}
	if es, ok := data["enhancedSegments"]; ok {
		enhancedSegments = es
	} else if msg, ok := data["message"].(map[string]interface{}); ok {
		enhancedSegments = msg["enhancedSegments"]
	}
	if enhancedSegments == nil {
		return nil
	}

	// ── Typed Go struct ──────────────────────────────────────────────────────
	if typedSegments, ok := enhancedSegments.(map[string]hl7.EnhancedSegment); ok {
		segment, exists := typedSegments[segmentKey]
		if !exists {
			return nil
		}
		for _, field := range segment.Fields {
			if field.Key != fieldKey {
				continue
			}
			paths := []string{path} // composite path always included
			for _, sf := range field.Subfields {
				paths = append(paths, sf.Key)
			}
			return paths
		}
		return nil
	}

	// ── map[string]interface{} ───────────────────────────────────────────────
	if segmentsMap, ok := enhancedSegments.(map[string]interface{}); ok {
		segmentData, exists := segmentsMap[segmentKey]
		if !exists {
			return nil
		}
		segmentMap, ok := segmentData.(map[string]interface{})
		if !ok {
			return nil
		}
		fields, ok := segmentMap["fields"].([]interface{})
		if !ok {
			return nil
		}
		for _, f := range fields {
			fieldMap, ok := f.(map[string]interface{})
			if !ok {
				continue
			}
			if fieldMap["key"] != fieldKey {
				continue
			}
			paths := []string{path} // composite path always included
			if subfields, ok := fieldMap["subfields"].([]interface{}); ok {
				for _, sf := range subfields {
					if sfMap, ok := sf.(map[string]interface{}); ok {
						if key, ok := sfMap["key"].(string); ok && key != "" {
							paths = append(paths, key)
						}
					}
				}
			}
			return paths
		}
	}

	return nil
}

// getJSONLeafPaths resolves path in data and, if the result is a container
// (map or slice), recursively enumerates every leaf path within it.
// For primitive results the original path is returned unchanged.
func getJSONLeafPaths(data map[string]interface{}, path string) []string {
	value := GetFieldValue(data, path)
	if value == nil {
		return nil
	}
	var leaves []string
	collectLeafPaths(path, value, &leaves)
	return leaves
}

// collectLeafPaths recursively walks value, appending fully-qualified leaf
// paths to out.  Primitive types (string, number, bool) are the leaves.
func collectLeafPaths(prefix string, value interface{}, out *[]string) {
	switch v := value.(type) {
	case map[string]interface{}:
		for k, child := range v {
			collectLeafPaths(prefix+"."+k, child, out)
		}
	case []interface{}:
		for i, child := range v {
			collectLeafPaths(fmt.Sprintf("%s[%d]", prefix, i), child, out)
		}
	default:
		// Primitive (string, number, bool, nil already filtered above)
		*out = append(*out, prefix)
	}
}

// ===============================================================
// FORMAT-AGNOSTIC FIELD VALUE SETTER
// ===============================================================

// UpdateFieldValue updates a field value in message data using format-agnostic path
// Returns true if update was successful
func UpdateFieldValue(data map[string]interface{}, path string, newValue interface{}) bool {
	pathType := DetectPathType(path)

	switch pathType {
	case PathTypeHL7:
		return modifyHL7FieldValue(data, path, newValue)
	case PathTypeFHIR:
		return modifyFHIRFieldValue(data, path, newValue)
	default:
		return modifyJSONPathValue(data, path, newValue)
	}
}

// modifyHL7FieldValue updates value in HL7 enhanced segments (format-agnostic version)
func modifyHL7FieldValue(data map[string]interface{}, path string, newValue interface{}) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return false
	}

	segmentKey := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])

	// Find enhancedSegments
	var enhancedSegments interface{}
	if es, ok := data["enhancedSegments"]; ok {
		enhancedSegments = es
	} else if msg, ok := data["message"].(map[string]interface{}); ok {
		enhancedSegments = msg["enhancedSegments"]
	}

	if enhancedSegments == nil {
		return false
	}

	// Handle typed Go struct
	if typedSegments, ok := enhancedSegments.(map[string]hl7.EnhancedSegment); ok {
		segment, exists := typedSegments[segmentKey]
		if !exists {
			return false
		}

		for i := range segment.Fields {
			if segment.Fields[i].Key == fieldKey {
				if len(parts) == 2 {
					segment.Fields[i].Value = fmt.Sprintf("%v", newValue)
					typedSegments[segmentKey] = segment
					return true
				}
				// Subfield update
				subfieldKey := path
				for j := range segment.Fields[i].Subfields {
					if segment.Fields[i].Subfields[j].Key == subfieldKey {
						segment.Fields[i].Subfields[j].Value = fmt.Sprintf("%v", newValue)
						typedSegments[segmentKey] = segment
						return true
					}
				}
			}
		}
		return false
	}

	// Handle map[string]interface{}
	if segmentsMap, ok := enhancedSegments.(map[string]interface{}); ok {
		return modifyHL7FieldInMap(segmentsMap, parts, newValue)
	}

	return false
}

// modifyHL7FieldInMap updates HL7 field in map structure
func modifyHL7FieldInMap(segments map[string]interface{}, parts []string, newValue interface{}) bool {
	segmentKey := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1])

	segmentData, exists := segments[segmentKey]
	if !exists {
		return false
	}

	segmentMap, ok := segmentData.(map[string]interface{})
	if !ok {
		return false
	}

	fields, ok := segmentMap["fields"].([]interface{})
	if !ok {
		return false
	}

	for i, f := range fields {
		fieldMap, ok := f.(map[string]interface{})
		if !ok {
			continue
		}

		if fieldMap["key"] == fieldKey {
			if len(parts) == 2 {
				fieldMap["value"] = fmt.Sprintf("%v", newValue)
				fields[i] = fieldMap
				return true
			}
			// Subfield update
			subfieldKey := strings.Join(parts, ".")
			subfields, ok := fieldMap["subfields"].([]interface{})
			if !ok {
				return false
			}

			for j, sf := range subfields {
				subfieldMap, ok := sf.(map[string]interface{})
				if !ok {
					continue
				}
				if subfieldMap["key"] == subfieldKey {
					subfieldMap["value"] = fmt.Sprintf("%v", newValue)
					subfields[j] = subfieldMap
					return true
				}
			}
		}
	}

	return false
}

// modifyFHIRFieldValue updates value in FHIR resource structure
func modifyFHIRFieldValue(data map[string]interface{}, path string, newValue interface{}) bool {
	// FHIR resources can be at root or under "fhirBundle" or "resource"
	var fhirData map[string]interface{}

	if bundle, ok := data["fhirBundle"].(map[string]interface{}); ok {
		fhirData = bundle
	} else if resource, ok := data["resource"].(map[string]interface{}); ok {
		fhirData = resource
	} else {
		fhirData = data
	}

	return modifyJSONPathValue(fhirData, path, newValue)
}

// modifyJSONPathValue updates value using dot notation with array support
func modifyJSONPathValue(data map[string]interface{}, path string, newValue interface{}) bool {
	if path == "" {
		return false
	}

	parts := parseJSONPath(path)
	if len(parts) == 0 {
		return false
	}

	// Navigate to parent of target
	var current interface{} = data
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if part.isArray {
			if arr, ok := current.([]interface{}); ok {
				if part.index >= 0 && part.index < len(arr) {
					current = arr[part.index]
				} else {
					return false
				}
			} else {
				return false
			}
		} else {
			if currentMap, ok := current.(map[string]interface{}); ok {
				next := currentMap[part.key]
				if next == nil {
					// Create intermediate map
					next = make(map[string]interface{})
					currentMap[part.key] = next
				}
				current = next
			} else {
				return false
			}
		}
	}

	// Set the final value
	lastPart := parts[len(parts)-1]
	if lastPart.isArray {
		if arr, ok := current.([]interface{}); ok {
			if lastPart.index >= 0 && lastPart.index < len(arr) {
				arr[lastPart.index] = newValue
				return true
			}
		}
		return false
	}

	if currentMap, ok := current.(map[string]interface{}); ok {
		currentMap[lastPart.key] = newValue
		return true
	}

	return false
}

// ===============================================================
// PATH CONVERSION UTILITIES
// ===============================================================

// GetAbsolutePath converts a short notation path to absolute JSON path
// Used for UI tooltip display
func GetAbsolutePath(path string) string {
	pathType := DetectPathType(path)

	switch pathType {
	case PathTypeHL7:
		return getHL7AbsolutePath(path)
	case PathTypeFHIR:
		return getFHIRAbsolutePath(path)
	default:
		return path
	}
}

// getHL7AbsolutePath converts HL7 notation to absolute JSON path
func getHL7AbsolutePath(hl7Path string) string {
	parts := strings.Split(hl7Path, ".")
	if len(parts) < 2 {
		return hl7Path
	}

	segment := parts[0]

	if len(parts) == 2 {
		return fmt.Sprintf("enhancedSegments.%s.fields[key=%s].value", segment, hl7Path)
	} else if len(parts) == 3 {
		parentField := fmt.Sprintf("%s.%s", segment, parts[1])
		return fmt.Sprintf("enhancedSegments.%s.fields[key=%s].subfields[key=%s].value", segment, parentField, hl7Path)
	}

	return hl7Path
}

// getFHIRAbsolutePath converts FHIR notation to absolute JSON path
func getFHIRAbsolutePath(fhirPath string) string {
	// FHIR paths are already fairly absolute
	// Could add "fhirBundle." prefix if needed
	return fhirPath
}

// ===============================================================
// UNIVERSAL VARIABLE RESOLVER (Phase 2 — Format-Agnostic Pipeline)
// ===============================================================
//
// ResolveVariable resolves any path from the full pipeline inputData map.
// Priority order:
//  1. _semantic_index in message envelope  (semantic paths: "patient.name.family")
//  2. Existing GetFieldValue               (HL7 PID.5.1, FHIR Patient.name[0].given, JSON dot-path, steps.*)
//
// All existing executors that call GetFieldValue continue to work unchanged.
// New executors call ResolveVariable to get semantic path support for free.
func ResolveVariable(inputData map[string]interface{}, path string) interface{} {
	// 1. Try semantic index inside the message envelope
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		if idx, ok := msg["_semantic_index"].(map[string]interface{}); ok {
			if nativePath, ok := idx[path].(string); ok {
				if v := GetFieldValue(msg, nativePath); v != nil {
					return v
				}
			}
		}
	}
	// 2. Fall through to existing format-agnostic resolver (HL7/FHIR/JSON/steps.*)
	return GetFieldValue(inputData, path)
}

// SetVariable sets a value using semantic path resolution (mirrors ResolveVariable).
// Returns true if the field was found and updated.
func SetVariable(inputData map[string]interface{}, path string, value interface{}) bool {
	// 1. Try semantic index inside the message envelope
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		if idx, ok := msg["_semantic_index"].(map[string]interface{}); ok {
			if nativePath, ok := idx[path].(string); ok {
				return UpdateFieldValue(msg, nativePath, value)
			}
		}
	}
	// 2. Fall through to existing format-agnostic setter
	return UpdateFieldValue(inputData, path, value)
}
