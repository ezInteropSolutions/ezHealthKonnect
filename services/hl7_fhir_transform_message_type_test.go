// services/hl7_fhir_transform_message_type_test.go
//
// extractMessageType and extractEnhancedSegments run at the very start of
// every Transform() call — before any mapping/DB work happens — to pull the
// message type and segment data out of whatever shape the parsed HL7 arrived
// in (direct map, string, nested under "data", a Go struct via reflection, or
// an MSH.9 fallback). Pure, multi-branch, previously untested.
package services

import "testing"

// ── extractMessageType ───────────────────────────────────────────────────────

func TestExtractMessageType_DirectMapWithName(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsed := map[string]interface{}{"messageType": map[string]interface{}{"name": "ADT^A01"}}
	got, err := s.extractMessageType(parsed)
	if err != nil || got != "ADT^A01" {
		t.Errorf("got (%q, %v), want (\"ADT^A01\", nil)", got, err)
	}
}

func TestExtractMessageType_DirectString(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsed := map[string]interface{}{"messageType": "ORU^R01"}
	got, err := s.extractMessageType(parsed)
	if err != nil || got != "ORU^R01" {
		t.Errorf("got (%q, %v), want (\"ORU^R01\", nil)", got, err)
	}
}

func TestExtractMessageType_NestedUnderData(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsed := map[string]interface{}{
		"data": map[string]interface{}{"messageType": map[string]interface{}{"name": "VXU^V04"}},
	}
	got, err := s.extractMessageType(parsed)
	if err != nil || got != "VXU^V04" {
		t.Errorf("got (%q, %v), want (\"VXU^V04\", nil)", got, err)
	}
}

func TestExtractMessageType_StructNameField_ViaMapKey(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	// "struct read from MongoDB as Go struct" path: mapped through as
	// map[string]interface{} with a capitalized "Name" key (not "name").
	parsed := map[string]interface{}{"messageType": map[string]interface{}{"Name": "MDM^T02"}}
	got, err := s.extractMessageType(parsed)
	if err != nil || got != "MDM^T02" {
		t.Errorf("got (%q, %v), want (\"MDM^T02\", nil)", got, err)
	}
}

func TestExtractMessageType_MSH9Fallback(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsed := map[string]interface{}{
		"basicSegments": map[string]interface{}{
			"MSH": map[string]interface{}{"fields": map[string]interface{}{"MSH.9": "ADT^A08"}},
		},
	}
	got, err := s.extractMessageType(parsed)
	if err != nil || got != "ADT^A08" {
		t.Errorf("got (%q, %v), want (\"ADT^A08\", nil)", got, err)
	}
}

func TestExtractMessageType_NothingMatches_ReturnsError(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	_, err := s.extractMessageType(map[string]interface{}{"unrelated": "data"})
	if err == nil {
		t.Error("expected an error when no message-type shape matches, got nil")
	}
}

// ── extractEnhancedSegments ───────────────────────────────────────────────────

func TestExtractEnhancedSegments_DirectMap(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	segs := map[string]interface{}{"PID": segMap(fieldObj("PID.8", "M"))}
	parsed := map[string]interface{}{"enhancedSegments": segs}
	got := s.extractEnhancedSegments(parsed)
	if got == nil || len(got) != 1 {
		t.Errorf("got %+v, want the 1-segment map returned directly", got)
	}
}

func TestExtractEnhancedSegments_NestedUnderData(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsed := map[string]interface{}{
		"data": map[string]interface{}{"enhancedSegments": map[string]interface{}{"PID": segMap(fieldObj("PID.8", "M"))}},
	}
	got := s.extractEnhancedSegments(parsed)
	if got == nil || len(got) != 1 {
		t.Errorf("got %+v, want the 1-segment map found under the data wrapper", got)
	}
}

func TestExtractEnhancedSegments_NotFound_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	got := s.extractEnhancedSegments(map[string]interface{}{"unrelated": "data"})
	if got != nil {
		t.Errorf("got %+v, want nil when enhancedSegments is absent anywhere", got)
	}
}

func TestExtractEnhancedSegments_TypedMap_ConvertedViaJSON(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	// A non-map[string]interface{} type (e.g. a typed Go struct map coming
	// straight from the parser) exercises the JSON marshal/unmarshal fallback.
	typed := map[string]map[string]string{"PID": {"PID.8": "M"}}
	parsed := map[string]interface{}{"enhancedSegments": typed}
	got := s.extractEnhancedSegments(parsed)
	if got == nil || len(got) != 1 {
		t.Errorf("got %+v, want a converted 1-segment map via the JSON fallback path", got)
	}
}
