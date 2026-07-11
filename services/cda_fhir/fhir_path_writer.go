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
// Example: "extension[url=http://...].valueCodeableConcept" finds-or-creates
// the extension[] element with that url and sets its valueCodeableConcept —
// see resolveBracketIndex for the predicate-matching semantics that makes
// generic FHIR extension writing possible without any extension-specific
// code in this function.
func setFHIRPath(obj map[string]interface{}, path string, value interface{}) {
	if path == "" || obj == nil || value == nil {
		return
	}
	dot := findTopLevelDot(path)
	if dot == -1 {
		// Leaf node — handle array index notation like "coding[0]" (or, less
		// commonly, a bare predicate with nothing after it — resolved the
		// same way, though every real extension TargetPath in this engine
		// has a trailing ".valueXxx" and goes through the branch below instead).
		if bracket := strings.Index(path, "["); bracket != -1 {
			key := path[:bracket]
			idx := resolveBracketIndex(obj, key, path[bracket:])
			arr, _ := obj[key].([]interface{})
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
		idx := resolveBracketIndex(obj, key, seg[bracket:])
		arr, _ := obj[key].([]interface{})
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

// findTopLevelDot returns the index of the first "." that is NOT inside a
// [...] bracket, or -1 if none exists. Needed because a bracket predicate's
// value can itself contain literal dots — a real extension URL like
// "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race" has three —
// so a naive strings.Index(path, ".") would incorrectly split inside the
// domain name instead of at the real segment boundary after the closing "]".
func findTopLevelDot(path string) int {
	depth := 0
	for i, ch := range path {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// resolveBracketIndex extracts the array to write into (obj[key]) and the
// index within it that bracketExpr designates, creating/mutating obj[key]
// as needed. Two forms:
//   - Plain integer ("[0]"): behaves exactly as the old parseBracketIndex+
//     ensureArray pair did — pads the array to at least idx+1 elements with
//     empty maps if needed. Malformed/non-numeric content still defensively
//     resolves to index 0, unchanged from prior behavior.
//   - Predicate ("[url=http://...]", "[key=value]" generally): searches the
//     existing array for an element whose key==value (string comparison),
//     reusing its index — or appends a fresh {key: value} object and
//     returns its new index. This find-or-create is exactly the semantics
//     a repeating FHIR extension slot needs (match by url, not position),
//     and falls out of a single general rule rather than any
//     extension-specific code.
func resolveBracketIndex(obj map[string]interface{}, key, bracketExpr string) int {
	start := strings.Index(bracketExpr, "[")
	end := strings.Index(bracketExpr, "]")
	if start == -1 || end == -1 || end <= start {
		arr := ensureArray(obj, key, 1)
		obj[key] = arr
		return 0
	}

	content := bracketExpr[start+1 : end]
	if eq := strings.Index(content, "="); eq != -1 {
		predKey, predVal := content[:eq], content[eq+1:]
		arr, _ := obj[key].([]interface{})
		for i, el := range arr {
			if m, ok := el.(map[string]interface{}); ok {
				if s, ok := m[predKey].(string); ok && s == predVal {
					return i
				}
			}
		}
		arr = append(arr, map[string]interface{}{predKey: predVal})
		obj[key] = arr
		return len(arr) - 1
	}

	n, err := strconv.Atoi(content)
	if err != nil {
		n = 0
	}
	arr := ensureArray(obj, key, n+1)
	obj[key] = arr
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

// SetFHIRPath is the exported entry point to setFHIRPath for callers outside
// this package (e.g. services/executors/transform/fhir_build_executor.go)
// that need to write a value into a FHIR resource map at a dot/bracket path,
// growing arrays on demand exactly as the CDA→FHIR declarative engine already
// does — see setFHIRPath's own doc comment for the path grammar.
func SetFHIRPath(obj map[string]interface{}, path string, value interface{}) {
	setFHIRPath(obj, path, value)
}

// IndexedPath is the exported entry point to indexedPath (declarative_engine.go)
// for callers outside this package that need to rewrite a repeating field's
// TargetPath into one array-element path per match, e.g. "identifier" ->
// "identifier[0]", "identifier[1]", ... — see indexedPath's own doc comment.
func IndexedPath(targetPath string, idx int) string {
	return indexedPath(targetPath, idx)
}

func stripResourcePrefix(fhirPath, resourceType string) string {
	prefix := resourceType + "."
	if strings.HasPrefix(fhirPath, prefix) {
		return fhirPath[len(prefix):]
	}
	return fhirPath
}
