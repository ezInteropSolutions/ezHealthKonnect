// services/cda_fhir/fhir_path_writer_test.go
package cdafhir

import "testing"

// TestSetFHIRPath_NumericIndex_StillPlainIndex proves the predicate-matching
// addition to resolveBracketIndex left the plain "[0]" numeric-index case
// byte-for-byte unchanged — the backward-compatibility guarantee every
// existing OOB rule (e.g. "coding[0].code") depends on.
func TestSetFHIRPath_NumericIndex_StillPlainIndex(t *testing.T) {
	resource := map[string]interface{}{}
	setFHIRPath(resource, "code.coding[0].code", "12345")
	setFHIRPath(resource, "code.coding[0].system", "http://snomed.info/sct")

	code, _ := resource["code"].(map[string]interface{})
	coding, _ := code["coding"].([]interface{})
	if len(coding) != 1 {
		t.Fatalf("coding array length = %d, want 1 (both writes target the same index)", len(coding))
	}
	elem, _ := coding[0].(map[string]interface{})
	if elem["code"] != "12345" || elem["system"] != "http://snomed.info/sct" {
		t.Errorf("coding[0] = %#v, want both code and system merged into one element", elem)
	}
}

// TestSetFHIRPath_NumericIndex_SecondIndexIsSeparateElement guards the other
// half of numeric-index behavior: "[1]" must be a different array slot than
// "[0]", not accidentally coalesced by the new predicate-detection branch.
func TestSetFHIRPath_NumericIndex_SecondIndexIsSeparateElement(t *testing.T) {
	resource := map[string]interface{}{}
	setFHIRPath(resource, "identifier[0].value", "MRN123")
	setFHIRPath(resource, "identifier[1].value", "SSN456")

	ident, _ := resource["identifier"].([]interface{})
	if len(ident) != 2 {
		t.Fatalf("identifier array length = %d, want 2", len(ident))
	}
	e0, _ := ident[0].(map[string]interface{})
	e1, _ := ident[1].(map[string]interface{})
	if e0["value"] != "MRN123" || e1["value"] != "SSN456" {
		t.Errorf("identifier = %#v, want [{value:MRN123} {value:SSN456}]", ident)
	}
}

// TestSetFHIRPath_PredicateBracket_AppendsWhenNoMatch proves a
// "[key=value]" predicate on an empty/absent array creates a fresh element
// carrying the predicate key, then sets the trailing path on it — the
// find-or-create behavior a repeating extension slot needs.
func TestSetFHIRPath_PredicateBracket_AppendsWhenNoMatch(t *testing.T) {
	resource := map[string]interface{}{}
	setFHIRPath(resource, "extension[url=http://hl7.org/fhir/us/core/StructureDefinition/us-core-race].valueCodeableConcept", map[string]interface{}{"text": "Asian"})

	ext, ok := resource["extension"].([]interface{})
	if !ok || len(ext) != 1 {
		t.Fatalf("extension = %#v, want a single-element array", resource["extension"])
	}
	elem, _ := ext[0].(map[string]interface{})
	if elem["url"] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race" {
		t.Errorf("extension[0].url = %v, want the predicate value preserved on the created element", elem["url"])
	}
	vcc, _ := elem["valueCodeableConcept"].(map[string]interface{})
	if vcc["text"] != "Asian" {
		t.Errorf("extension[0].valueCodeableConcept = %#v, want {text: Asian}", elem["valueCodeableConcept"])
	}
}

// TestSetFHIRPath_PredicateBracket_FindsExistingElement proves a SECOND
// setFHIRPath call using the SAME url predicate reuses the same element
// (merging both sub-fields into one extension), rather than appending a
// duplicate — critical since a real MappingRow might write both the
// extension's url-identified slot AND a second property on it via two
// separate rows/calls (mirroring how numeric-index rows already merge, see
// the coding[0] test above).
func TestSetFHIRPath_PredicateBracket_FindsExistingElement(t *testing.T) {
	resource := map[string]interface{}{}
	url := "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race"
	setFHIRPath(resource, "extension[url="+url+"].valueCodeableConcept", map[string]interface{}{"text": "Asian"})
	setFHIRPath(resource, "extension[url="+url+"].id", "race-ext-1")

	ext, _ := resource["extension"].([]interface{})
	if len(ext) != 1 {
		t.Fatalf("extension array length = %d, want 1 (second write should reuse the same element, not append)", len(ext))
	}
	elem, _ := ext[0].(map[string]interface{})
	if elem["id"] != "race-ext-1" {
		t.Errorf("extension[0].id = %v, want race-ext-1", elem["id"])
	}
	if vcc, _ := elem["valueCodeableConcept"].(map[string]interface{}); vcc["text"] != "Asian" {
		t.Errorf("extension[0].valueCodeableConcept = %#v, want the first write's value still present", elem["valueCodeableConcept"])
	}
}

// TestSetFHIRPath_PredicateBracket_DifferentURLIsSeparateElement proves two
// DIFFERENT url predicates produce two distinct extension elements, not a
// merge — the find-or-create logic must actually match on value, not just
// find "an" element in the array.
func TestSetFHIRPath_PredicateBracket_DifferentURLIsSeparateElement(t *testing.T) {
	resource := map[string]interface{}{}
	setFHIRPath(resource, "extension[url=http://example.org/ext-a].valueString", "A")
	setFHIRPath(resource, "extension[url=http://example.org/ext-b].valueString", "B")

	ext, _ := resource["extension"].([]interface{})
	if len(ext) != 2 {
		t.Fatalf("extension array length = %d, want 2 (different urls must not merge)", len(ext))
	}
	urls := map[string]string{}
	for _, e := range ext {
		m, _ := e.(map[string]interface{})
		urls[m["url"].(string)] = m["valueString"].(string)
	}
	if urls["http://example.org/ext-a"] != "A" || urls["http://example.org/ext-b"] != "B" {
		t.Errorf("extension = %#v, want ext-a=A and ext-b=B as separate elements", ext)
	}
}
