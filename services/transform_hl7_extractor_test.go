// services/transform_hl7_extractor_test.go
//
// transform_hl7_extractor.go is the actual value-extraction engine for the
// HL7->FHIR pipeline: extractHL7ValueAtomic (and everything it delegates to)
// is the function EVERY single mapped field goes through, for every message,
// regardless of message type. It had zero tests despite being 100% pure logic
// (no DB, no I/O) — this file closes that gap.
package services

import (
	"reflect"
	"testing"

	"ezhealthkonnect/hl7"
)

// ── helpers: build the map[string]interface{} shape these functions expect ──

func segMap(fields ...map[string]interface{}) map[string]interface{} {
	fs := make([]interface{}, len(fields))
	for i, f := range fields {
		fs[i] = f
	}
	return map[string]interface{}{"fields": fs}
}

func fieldObj(key, value string) map[string]interface{} {
	return map[string]interface{}{"key": key, "value": value}
}

func fieldObjWithSubfields(key, value string, subfields ...map[string]interface{}) map[string]interface{} {
	sf := make([]interface{}, len(subfields))
	for i, s := range subfields {
		sf[i] = s
	}
	return map[string]interface{}{"key": key, "value": value, "subfields": sf}
}

func subfieldObj(key, value string) map[string]interface{} {
	return map[string]interface{}{"key": key, "value": value}
}

// ── extractHL7ValueAtomic ────────────────────────────────────────────────────

func TestExtractHL7ValueAtomic_SegmentNotFound_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "PID", HL7Field: "5"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false) for missing segment", val, found)
	}
}

func TestExtractHL7ValueAtomic_FieldsKeyMissing_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{"PID": map[string]interface{}{}}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "PID", HL7Field: "5"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false) when fields key is absent", val, found)
	}
}

func TestExtractHL7ValueAtomic_SimpleFieldMatch_ReturnsValue(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{
		"PID": segMap(fieldObj("PID.7", "19800101"), fieldObj("PID.8", "M")),
	}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "PID", HL7Field: "8"})
	if !found || val != "M" {
		t.Errorf("got (%q, %v), want (\"M\", true)", val, found)
	}
}

func TestExtractHL7ValueAtomic_FieldNotFound_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{"PID": segMap(fieldObj("PID.7", "19800101"))}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "PID", HL7Field: "99"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false) for a field key that isn't present", val, found)
	}
}

func TestExtractHL7ValueAtomic_ComponentMapping_DelegatesToSubfields(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{
		"PID": segMap(fieldObjWithSubfields("PID.5", "Doe^John", subfieldObj("PID.5.1", "Doe"), subfieldObj("PID.5.2", "John"))),
	}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "PID", HL7Field: "5", HL7Component: "2"})
	if !found || val != "John" {
		t.Errorf("got (%q, %v), want (\"John\", true) for PID.5.2", val, found)
	}
}

// TestExtractHL7ValueAtomic_ReflectSlicePath covers the MongoDB primitive.A
// fallback: fields arrives as some other slice type, not []interface{}.
func TestExtractHL7ValueAtomic_ReflectSlicePath_TypedSliceStillWorks(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	// A differently-typed slice (not []interface{}) exercises the reflect.Slice branch.
	typedFields := []map[string]interface{}{fieldObj("OBX.11", "F")}
	enhancedSegments := map[string]interface{}{
		"OBX": map[string]interface{}{"fields": typedFields},
	}
	val, found := s.extractHL7ValueAtomic(enhancedSegments, FieldMapping{SegmentName: "OBX", HL7Field: "11"})
	if !found || val != "F" {
		t.Errorf("got (%q, %v), want (\"F\", true) via reflect-based slice conversion", val, found)
	}
}

// ── extractHL7FieldDirect ─────────────────────────────────────────────────────

func TestExtractHL7FieldDirect_FieldFound(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{"OBX": segMap(fieldObj("OBX.2", "NM"), fieldObj("OBX.6", "mg/dL"))}
	if got := s.extractHL7FieldDirect(enhancedSegments, "OBX", "OBX.6"); got != "mg/dL" {
		t.Errorf("extractHL7FieldDirect = %q, want mg/dL", got)
	}
}

func TestExtractHL7FieldDirect_SegmentMissing_ReturnsEmpty(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.extractHL7FieldDirect(map[string]interface{}{}, "OBX", "OBX.2"); got != "" {
		t.Errorf("extractHL7FieldDirect = %q, want empty string for missing segment", got)
	}
}

// ── extractComponentFromHL7Field ─────────────────────────────────────────────

func TestExtractComponentFromHL7Field_SubfieldMatch(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	field := fieldObjWithSubfields("PID.5", "Doe^John^M", subfieldObj("PID.5.1", "Doe"), subfieldObj("PID.5.2", "John"))
	val, found := s.extractComponentFromHL7Field(field, FieldMapping{SegmentName: "PID", HL7Field: "5", HL7Component: "1"})
	if !found || val != "Doe" {
		t.Errorf("got (%q, %v), want (\"Doe\", true)", val, found)
	}
}

func TestExtractComponentFromHL7Field_NoSubfields_FallsBackToManualParse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	field := fieldObj("PID.5", "Doe^John^M") // no subfields key at all
	val, found := s.extractComponentFromHL7Field(field, FieldMapping{SegmentName: "PID", HL7Field: "5", HL7Component: "2"})
	if !found || val != "John" {
		t.Errorf("got (%q, %v), want (\"John\", true) via manual ^ split fallback", val, found)
	}
}

func TestExtractComponentFromHL7Field_NoSubfieldsNoValue_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	field := map[string]interface{}{"key": "PID.5"} // no value, no subfields
	val, found := s.extractComponentFromHL7Field(field, FieldMapping{SegmentName: "PID", HL7Field: "5", HL7Component: "1"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", val, found)
	}
}

// ── manualParseComponentValue ─────────────────────────────────────────────────

func TestManualParseComponentValue_SimpleCaretSplit(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	val, found := s.manualParseComponentValue("Doe^John^M", FieldMapping{HL7Component: "3"})
	if !found || val != "M" {
		t.Errorf("got (%q, %v), want (\"M\", true)", val, found)
	}
}

func TestManualParseComponentValue_ComponentOutOfRange_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	val, found := s.manualParseComponentValue("Doe^John", FieldMapping{HL7Component: "5"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false) for an out-of-range component", val, found)
	}
}

func TestManualParseComponentValue_NonNumericComponent_ReturnsFalse(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	val, found := s.manualParseComponentValue("Doe^John", FieldMapping{HL7Component: "x"})
	if found || val != "" {
		t.Errorf("got (%q, %v), want (\"\", false) for a non-numeric component", val, found)
	}
}

// ── parseHL7Path ──────────────────────────────────────────────────────────────

func TestParseHL7Path(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	cases := []struct {
		in                              string
		wantSeg, wantField, wantComp string
	}{
		{"PID.5.1", "PID", "5", "1"},
		{"PID.5", "PID", "5", ""},
		{"MSH", "MSH", "", ""},
	}
	for _, c := range cases {
		seg, field, comp := s.parseHL7Path(c.in)
		if seg != c.wantSeg || field != c.wantField || comp != c.wantComp {
			t.Errorf("parseHL7Path(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, seg, field, comp, c.wantSeg, c.wantField, c.wantComp)
		}
	}
}

// ── extractSegmentGroup ───────────────────────────────────────────────────────

func TestExtractSegmentGroup_NoSegmentGroupsKey_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.extractSegmentGroup(map[string]interface{}{}, "NK1"); got != nil {
		t.Errorf("got %+v, want nil when segmentGroups key is absent", got)
	}
}

func TestExtractSegmentGroup_TypedPath(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"NK1": {{Key: "NK1", Fields: []hl7.FieldInfo{{Key: "NK1.2", Value: "TEST^CINDY", HasValue: true}}}},
		},
	}
	got := s.extractSegmentGroup(parsedHL7Data, "NK1")
	if len(got) != 1 || got[0].Fields[0].Value != "TEST^CINDY" {
		t.Errorf("extractSegmentGroup (typed path) = %+v, want 1 NK1 occurrence", got)
	}
}

func TestExtractSegmentGroup_JSONUnmarshaledPath(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	// Simulates data that arrived via JSON decoding (map[string]interface{} instead
	// of the typed map[string][]hl7.EnhancedSegment) — the code path that requires
	// the marshal/unmarshal round trip.
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string]interface{}{
			"NK1": []interface{}{
				map[string]interface{}{"key": "NK1", "fields": []interface{}{
					map[string]interface{}{"key": "NK1.2", "value": "TEST^CINDY", "hasValue": true},
				}},
			},
		},
	}
	got := s.extractSegmentGroup(parsedHL7Data, "NK1")
	if len(got) != 1 || got[0].Fields[0].Value != "TEST^CINDY" {
		t.Errorf("extractSegmentGroup (JSON path) = %+v, want 1 NK1 occurrence", got)
	}
}

// ── enhancedSegmentToMap ──────────────────────────────────────────────────────

func TestEnhancedSegmentToMap_RoundTrip(t *testing.T) {
	seg := hl7.EnhancedSegment{Key: "NK1", Fields: []hl7.FieldInfo{{Key: "NK1.2", Value: "TEST^CINDY", HasValue: true}}}
	m := enhancedSegmentToMap(seg)
	if m == nil {
		t.Fatal("enhancedSegmentToMap returned nil")
	}
	if m["key"] != "NK1" {
		t.Errorf(`m["key"] = %v, want "NK1"`, m["key"])
	}
	fields, ok := m["fields"].([]interface{})
	if !ok || len(fields) != 1 {
		t.Fatalf(`m["fields"] = %v, want a 1-element slice`, m["fields"])
	}
}

// ── segFieldValue ─────────────────────────────────────────────────────────────

func TestSegFieldValue(t *testing.T) {
	seg := hl7.EnhancedSegment{Fields: []hl7.FieldInfo{{Key: "OBX.3", Value: "2823-3"}}}
	if got := segFieldValue(seg, "OBX.3"); got != "2823-3" {
		t.Errorf("segFieldValue = %q, want 2823-3", got)
	}
	if got := segFieldValue(seg, "OBX.5"); got != "" {
		t.Errorf("segFieldValue (missing key) = %q, want empty string", got)
	}
}

// ── mfeActionFromSegments ─────────────────────────────────────────────────────

func TestMfeActionFromSegments_NoMFESegment_ReturnsEmpty(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.mfeActionFromSegments(map[string]interface{}{}); got != "" {
		t.Errorf("got %q, want empty string when no MFE segment present", got)
	}
}

func TestMfeActionFromSegments_UppercasesAndTrims(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	enhancedSegments := map[string]interface{}{"MFE": segMap(fieldObj("MFE.1", "  mdl "))}
	if got := s.mfeActionFromSegments(enhancedSegments); got != "MDL" {
		t.Errorf("got %q, want MDL (uppercased and trimmed)", got)
	}
}

// ── resolveHL7FieldFromParsed ─────────────────────────────────────────────────

func TestResolveHL7FieldFromParsed_EmptyPath_ReturnsEmpty(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.resolveHL7FieldFromParsed(map[string]interface{}{}, ""); got != "" {
		t.Errorf("got %q, want empty string for empty fieldPath", got)
	}
}

func TestResolveHL7FieldFromParsed_TwoPartPath_ReturnsFieldValue(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"SCH": {{Key: "SCH", Fields: []hl7.FieldInfo{{Key: "SCH.25", Value: "BOOKED"}}}},
		},
	}
	if got := s.resolveHL7FieldFromParsed(parsedHL7Data, "SCH.25"); got != "BOOKED" {
		t.Errorf("got %q, want BOOKED", got)
	}
}

func TestResolveHL7FieldFromParsed_ThreePartPath_SubfieldMatch(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"PID": {{Key: "PID", Fields: []hl7.FieldInfo{{
				Key: "PID.5", Value: "Doe^John",
				Subfields: []hl7.SubfieldInfo{
					{Key: "PID.5.1", Name: "Family", Value: "Doe", HasValue: true},
					{Key: "PID.5.2", Name: "Given", Value: "John", HasValue: true},
				},
			}}}},
		},
	}
	got := s.resolveHL7FieldFromParsed(parsedHL7Data, "PID.5.2")
	if got != "John" {
		t.Errorf("got %q, want John (direct subfield value match)", got)
	}
}

func TestResolveHL7FieldFromParsed_ThreePartPath_NoSubfieldKeyMatch_FallsBackToCaretSplit(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			// Subfields present but none carry the key "PID.5.2" (e.g. an
			// incomplete/legacy dictionary entry) — must fall back to
			// splitting the parent field's raw value by "^".
			"PID": {{Key: "PID", Fields: []hl7.FieldInfo{{
				Key: "PID.5", Value: "Doe^John",
				Subfields: []hl7.SubfieldInfo{{Key: "PID.5.1", Name: "Family", Value: "Doe", HasValue: true}},
			}}}},
		},
	}
	got := s.resolveHL7FieldFromParsed(parsedHL7Data, "PID.5.2")
	if got != "John" {
		t.Errorf("got %q, want John (via caret-split fallback since no subfield carries key PID.5.2)", got)
	}
}

func TestResolveHL7FieldFromParsed_FieldIndexFallback(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	// No field carries the key "PID.99" — falls back to positional (1-based) indexing.
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"PID": {{Key: "PID", Fields: []hl7.FieldInfo{
				{Key: "PID.1", Value: "first"},
				{Key: "PID.2", Value: "second"},
			}}},
		},
	}
	got := s.resolveHL7FieldFromParsed(parsedHL7Data, "PID.2")
	if got != "second" {
		t.Errorf("got %q, want second (direct key match)", got)
	}
}

func TestResolveHL7FieldFromParsed_NoSegmentOccurrence_ReturnsEmpty(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	got := s.resolveHL7FieldFromParsed(map[string]interface{}{}, "PID.5")
	if got != "" {
		t.Errorf("got %q, want empty string when the segment doesn't exist at all", got)
	}
}

// ── extractResourceTypes / filterMappingsForResource ─────────────────────────

func TestExtractResourceTypes_DedupesPreservingFirstSeenOrder(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	mappings := []FieldMapping{
		{FHIRResourceType: "Patient"},
		{FHIRResourceType: "Encounter"},
		{FHIRResourceType: "Patient"},
		{FHIRResourceType: "MessageHeader"},
	}
	got := s.extractResourceTypes(mappings)
	want := []string{"Patient", "Encounter", "MessageHeader"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractResourceTypes = %v, want %v", got, want)
	}
}

func TestFilterMappingsForResource(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	mappings := []FieldMapping{
		{FHIRResourceType: "Patient", HL7Field: "5"},
		{FHIRResourceType: "Encounter", HL7Field: "2"},
		{FHIRResourceType: "Patient", HL7Field: "7"},
	}
	got := s.filterMappingsForResource(mappings, "Patient")
	if len(got) != 2 || got[0].HL7Field != "5" || got[1].HL7Field != "7" {
		t.Errorf("filterMappingsForResource(Patient) = %+v, want the 2 Patient-targeted mappings in order", got)
	}
}

// ── extractFieldMappingsFromWizardArray ──────────────────────────────────────

func TestExtractFieldMappingsFromWizardArray_BasicConversion(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	arr := []map[string]interface{}{
		{"sourcePath": "PID.3.1", "targetPath": "Patient.identifier[0].value", "transformType": "string_direct", "isRequired": true},
	}
	got := s.extractFieldMappingsFromWizardArray(arr)
	if len(got) != 1 {
		t.Fatalf("got %d mappings, want 1", len(got))
	}
	fm := got[0]
	if fm.SegmentName != "PID" || fm.HL7Field != "3" || fm.HL7Component != "1" {
		t.Errorf("HL7 source = %s.%s.%s, want PID.3.1", fm.SegmentName, fm.HL7Field, fm.HL7Component)
	}
	if fm.FHIRResourceType != "Patient" || fm.FHIRElementPath != "identifier[0].value" {
		t.Errorf("FHIR target = %s.%s, want Patient.identifier[0].value", fm.FHIRResourceType, fm.FHIRElementPath)
	}
	if !fm.IsRequired {
		t.Errorf("IsRequired = false, want true")
	}
}

func TestExtractFieldMappingsFromWizardArray_SkipsMissingPaths(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	arr := []map[string]interface{}{
		{"sourcePath": "", "targetPath": "Patient.gender"},
		{"sourcePath": "PID.8", "targetPath": ""},
	}
	got := s.extractFieldMappingsFromWizardArray(arr)
	if len(got) != 0 {
		t.Errorf("got %d mappings, want 0 (both entries have a missing path)", len(got))
	}
}

func TestExtractFieldMappingsFromWizardArray_SkipsSequenceIDMappedToResourceID(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	arr := []map[string]interface{}{
		{"sourcePath": "OBR.1", "targetPath": "DiagnosticReport.id"}, // SI field -> id: must be skipped
		{"sourcePath": "OBR.4", "targetPath": "DiagnosticReport.code"},
	}
	got := s.extractFieldMappingsFromWizardArray(arr)
	if len(got) != 1 || got[0].HL7Field != "4" {
		t.Errorf("got %+v, want only the OBR.4 mapping (OBR.1->.id sequence-ID mapping must be skipped)", got)
	}
}

// ── extractFieldMappingsFromWizardConfig ─────────────────────────────────────

func TestExtractFieldMappingsFromWizardConfig_BasicConversion(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	cfg := map[string]interface{}{
		"PID.3": map[string]interface{}{"fhirPath": "Patient.identifier", "transform": "cx_to_identifier", "required": true},
	}
	got := s.extractFieldMappingsFromWizardConfig(cfg)
	if len(got) != 1 {
		t.Fatalf("got %d mappings, want 1", len(got))
	}
	fm := got[0]
	if fm.SegmentName != "PID" || fm.HL7Field != "3" {
		t.Errorf("HL7 source = %s.%s, want PID.3", fm.SegmentName, fm.HL7Field)
	}
	if fm.FHIRResourceType != "Patient" || fm.FHIRElementPath != "identifier" {
		t.Errorf("FHIR target = %s.%s, want Patient.identifier", fm.FHIRResourceType, fm.FHIRElementPath)
	}
	if fm.DataTypeTransform != "cx_to_identifier" || !fm.IsRequired {
		t.Errorf("transform/required = %q/%v, want cx_to_identifier/true", fm.DataTypeTransform, fm.IsRequired)
	}
}

func TestExtractFieldMappingsFromWizardConfig_SkipsMissingFHIRPath(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	cfg := map[string]interface{}{
		"PID.3": map[string]interface{}{"transform": "cx_to_identifier"}, // no fhirPath
	}
	got := s.extractFieldMappingsFromWizardConfig(cfg)
	if len(got) != 0 {
		t.Errorf("got %d mappings, want 0 (fhirPath absent)", len(got))
	}
}

// ── getDataTypeTransform / getIsRequired ─────────────────────────────────────

func TestGetDataTypeTransform(t *testing.T) {
	if got := getDataTypeTransform(map[string]interface{}{"transform": "xpn_to_humanname"}); got != "xpn_to_humanname" {
		t.Errorf("got %q, want xpn_to_humanname", got)
	}
	if got := getDataTypeTransform(map[string]interface{}{}); got != "" {
		t.Errorf("got %q, want empty string when transform key absent", got)
	}
}

func TestGetIsRequired(t *testing.T) {
	if got := getIsRequired(map[string]interface{}{"required": true}); !got {
		t.Errorf("got false, want true")
	}
	if got := getIsRequired(map[string]interface{}{}); got {
		t.Errorf("got true, want false when required key absent")
	}
}

// ── extractContextLinks ───────────────────────────────────────────────────────

func TestExtractContextLinks_NoContextKey_ReturnsNil(t *testing.T) {
	if got := extractContextLinks(map[string]interface{}{}); got != nil {
		t.Errorf("got %+v, want nil for a v1.0 template with no context block", got)
	}
}

func TestExtractContextLinks_EmptyContext_ReturnsNil(t *testing.T) {
	if got := extractContextLinks(map[string]interface{}{"context": map[string]interface{}{}}); got != nil {
		t.Errorf("got %+v, want nil for an empty context block", got)
	}
}

func TestExtractContextLinks_PopulatesRoleToSegmentAndResourceLinks(t *testing.T) {
	templateData := map[string]interface{}{
		"context": map[string]interface{}{"patient": "PID", "encounter": "PV1"},
		"resources": map[string]interface{}{
			"Encounter": map[string]interface{}{
				"contextLinks": map[string]interface{}{"subject": "patient"},
			},
			"AllergyIntolerance": map[string]interface{}{
				"contextLinks": map[string]interface{}{"patient": "patient", "encounter": "encounter"},
			},
		},
	}
	cl := extractContextLinks(templateData)
	if cl == nil {
		t.Fatal("extractContextLinks returned nil, want a populated ContextLinks")
	}
	if cl.RoleToSegment["patient"] != "PID" || cl.RoleToSegment["encounter"] != "PV1" {
		t.Errorf("RoleToSegment = %+v, want patient->PID, encounter->PV1", cl.RoleToSegment)
	}
	if cl.ResourceLinks["Encounter"]["subject"] != "patient" {
		t.Errorf("ResourceLinks[Encounter] = %+v, want subject->patient", cl.ResourceLinks["Encounter"])
	}
	if cl.ResourceLinks["AllergyIntolerance"]["patient"] != "patient" || cl.ResourceLinks["AllergyIntolerance"]["encounter"] != "encounter" {
		t.Errorf("ResourceLinks[AllergyIntolerance] = %+v, want patient->patient, encounter->encounter", cl.ResourceLinks["AllergyIntolerance"])
	}
}
