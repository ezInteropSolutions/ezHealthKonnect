// services/cda_fhir/fhir_path_writer.go
//
// Shared FHIR-path writer — moved out of generic_mapper.go (not duplicated)
// as part of Phase 2 of the CDA→FHIR Declarative Mapping Engine (see
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md, "Phase 2" design
// note): generic_mapper.go's createFHIRResourceFromSection (the dormant
// engine) and declarative_engine.go's BuildResources (the new engine) both
// need to write a value at a dot/bracket FHIR path — this has zero CDA-
// specific knowledge, so it lives in its own file both can call unchanged.
// When Phase 4 deletes createFHIRResourceFromSection, this file is untouched
// and keeps serving the new engine alone.
package cdafhir

import (
	"strconv"
	"strings"
)

// setFHIRPath navigates/creates the nested map structure described by a dot-separated
// FHIR path (with optional array indices) and sets the leaf value.
// Example: "code.coding[0].code" sets resource["code"]["coding"][0]["code"] = value.
func setFHIRPath(obj map[string]interface{}, path string, value interface{}) {
	if path == "" || obj == nil || value == nil {
		return
	}
	dot := strings.Index(path, ".")
	if dot == -1 {
		// Leaf node — handle array index notation like "coding[0]"
		if bracket := strings.Index(path, "["); bracket != -1 {
			key := path[:bracket]
			idx := parseBracketIndex(path[bracket:])
			arr := ensureArray(obj, key, idx+1)
			if m, ok := arr[idx].(map[string]interface{}); ok {
				_ = m // leaf — can't set on bare array element without a key
			}
			// Direct assignment to the array element if it's a leaf
			arr[idx] = value
			obj[key] = arr
		} else {
			obj[path] = value
		}
		return
	}

	seg := path[:dot]
	rest := path[dot+1:]

	if bracket := strings.Index(seg, "["); bracket != -1 {
		key := seg[:bracket]
		idx := parseBracketIndex(seg[bracket:])
		arr := ensureArray(obj, key, idx+1)
		child, ok := arr[idx].(map[string]interface{})
		if !ok {
			child = map[string]interface{}{}
		}
		setFHIRPath(child, rest, value)
		arr[idx] = child
		obj[key] = arr
	} else {
		child, ok := obj[seg].(map[string]interface{})
		if !ok {
			child = map[string]interface{}{}
		}
		setFHIRPath(child, rest, value)
		obj[seg] = child
	}
}

func parseBracketIndex(s string) int {
	// s looks like "[0]" or "[0].something" — extract the integer
	start := strings.Index(s, "[")
	end := strings.Index(s, "]")
	if start == -1 || end == -1 || end <= start {
		return 0
	}
	n, err := strconv.Atoi(s[start+1 : end])
	if err != nil {
		return 0
	}
	return n
}

func ensureArray(obj map[string]interface{}, key string, minLen int) []interface{} {
	existing, ok := obj[key].([]interface{})
	if !ok {
		existing = []interface{}{}
	}
	for len(existing) < minLen {
		existing = append(existing, map[string]interface{}{})
	}
	return existing
}

func stripResourcePrefix(fhirPath, resourceType string) string {
	prefix := resourceType + "."
	if strings.HasPrefix(fhirPath, prefix) {
		return fhirPath[len(prefix):]
	}
	return fhirPath
}
