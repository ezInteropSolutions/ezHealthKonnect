// services/hl7_fhir_transform_engine_test.go
//
// Unit coverage for the pure (no-DB) helper functions inside
// hl7_fhir_transform_service_v3.go — the main HL7->FHIR engine, which sat at
// 7.5% coverage overall. buildResourcesForType's repeat-instancing and
// PV1-priority-collision logic already has dedicated coverage in
// hl7_fhir_transform_service_v3_repeat_instancing_test.go; this file covers
// everything else in the hot path that every single Transform() call runs
// through: bundle assembly, context-link wiring, message-family matching,
// atomic-mapping conversion, and the delta merge logic that applies a user's
// saved custom-mapping overrides on top of the OOB base template (a bug here
// would silently drop or corrupt a customer's saved customization).
package services

import (
	"testing"

	"ezhealthkonnect/hl7"
)

// ── withSegmentOccurrence ────────────────────────────────────────────────────

func TestWithSegmentOccurrence_ReplacesOnlyTargetSegment(t *testing.T) {
	base := map[string]interface{}{
		"PID": map[string]interface{}{"fields": []interface{}{"unchanged"}},
	}
	occ := rtSeg("NK1", 1, rtField("NK1.2", "TEST^CINDY"))

	out := withSegmentOccurrence(base, "NK1", occ)

	if _, stillThere := out["PID"]; !stillThere {
		t.Errorf("PID key dropped from output: %+v", out)
	}
	nk1, ok := out["NK1"]
	if !ok {
		t.Fatalf("NK1 key missing from output: %+v", out)
	}
	if nk1 == nil {
		t.Errorf("NK1 value is nil, want the converted occurrence map")
	}
}

func TestWithSegmentOccurrence_DoesNotMutateInput(t *testing.T) {
	base := map[string]interface{}{"PID": "original"}
	occ := rtSeg("NK1", 0, rtField("NK1.2", "X"))

	withSegmentOccurrence(base, "NK1", occ)

	if len(base) != 1 {
		t.Errorf("input map was mutated: %+v", base)
	}
	if _, ok := base["NK1"]; ok {
		t.Errorf("input map gained an NK1 key it should not have: %+v", base)
	}
}

// ── applyContextLinks ─────────────────────────────────────────────────────

func TestApplyContextLinks_NilContextLinks_ReturnsUnchanged(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "p1"}}
	got := svc.applyContextLinks(resources, nil)
	if len(got) != 1 || got[0]["resourceType"] != "Patient" {
		t.Errorf("applyContextLinks(nil) = %+v, want resources unchanged", got)
	}
}

func TestApplyContextLinks_WiresDeclaredReference(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "p1"},
		{"resourceType": "Encounter", "id": "e1"},
	}
	cl := &ContextLinks{
		RoleToSegment: map[string]string{"patient": "PID"},
		ResourceLinks: map[string]map[string]string{
			"Encounter": {"subject": "patient"},
		},
	}

	got := svc.applyContextLinks(resources, cl)

	enc := got[1]
	subj, ok := enc["subject"].(map[string]interface{})
	if !ok {
		t.Fatalf("Encounter.subject = %v, want a reference object", enc["subject"])
	}
	if subj["reference"] != "Patient/p1" {
		t.Errorf("Encounter.subject.reference = %v, want Patient/p1", subj["reference"])
	}
}

func TestApplyContextLinks_DoesNotOverwriteExistingValue(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "p1"},
		{"resourceType": "Encounter", "id": "e1", "subject": map[string]interface{}{"reference": "Patient/already-set"}},
	}
	cl := &ContextLinks{
		RoleToSegment: map[string]string{"patient": "PID"},
		ResourceLinks: map[string]map[string]string{
			"Encounter": {"subject": "patient"},
		},
	}

	got := svc.applyContextLinks(resources, cl)

	subj := got[1]["subject"].(map[string]interface{})
	if subj["reference"] != "Patient/already-set" {
		t.Errorf("applyContextLinks overwrote an already-set field: got %v, want Patient/already-set", subj["reference"])
	}
}

// ── sameMessageFamily / isStructurallyHeterogeneousFamily ──────────────────

func TestSameMessageFamily(t *testing.T) {
	cases := []struct {
		incoming, configured string
		want                 bool
	}{
		{"ORU^R01", "ORU^R03", true},
		{"oru^r01", "ORU^R03", true}, // case-insensitive
		{"ADT^A01", "ADT^A08", true},
		{"MFN^M02", "ADT^A01", false},
		{"", "ADT^A01", false},
		{"ADT^A01", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := sameMessageFamily(c.incoming, c.configured); got != c.want {
			t.Errorf("sameMessageFamily(%q, %q) = %v, want %v", c.incoming, c.configured, got, c.want)
		}
	}
}

func TestIsStructurallyHeterogeneousFamily(t *testing.T) {
	cases := map[string]bool{
		"MFN^M02": true,
		"MFN^M05": true,
		"ACK^A01": true,
		"QBP^Q11": true,
		"RSP^K11": true,
		"ADT^A01": false,
		"ORU^R01": false,
		"VXU^V04": false,
	}
	for in, want := range cases {
		if got := isStructurallyHeterogeneousFamily(in); got != want {
			t.Errorf("isStructurallyHeterogeneousFamily(%q) = %v, want %v", in, got, want)
		}
	}
}

// ── convertToAtomicMappings ─────────────────────────────────────────────────

func TestConvertToAtomicMappings_BuildsDottedSourcePath(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	mappings := []FieldMapping{
		{ID: 1, SegmentName: "PID", HL7Field: "5", HL7Component: "1", FHIRResourceType: "Patient", FHIRElementPath: "name[0].family", DataTypeTransform: "xpn_to_humanname", IsRequired: true, Confidence: 0.9},
	}
	got := svc.convertToAtomicMappings(mappings)
	if len(got) != 1 {
		t.Fatalf("got %d atomic mappings, want 1", len(got))
	}
	if got[0].SourcePath != "PID.5.1" {
		t.Errorf("SourcePath = %q, want PID.5.1", got[0].SourcePath)
	}
	if got[0].TargetPath != "name[0].family" {
		t.Errorf("TargetPath = %q, want name[0].family", got[0].TargetPath)
	}
	if !got[0].IsRequired {
		t.Errorf("IsRequired = false, want true")
	}
}

func TestConvertToAtomicMappings_StaticValueHasEmptySourcePath(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	mappings := []FieldMapping{
		{ID: 2, SegmentName: "", HL7Field: "", FHIRResourceType: "Patient", FHIRElementPath: "active", DataTypeTransform: "static_value", StaticValue: "true"},
	}
	got := svc.convertToAtomicMappings(mappings)
	if got[0].SourcePath != "" {
		t.Errorf("static_value mapping SourcePath = %q, want empty (no HL7 source)", got[0].SourcePath)
	}
	if got[0].DefaultValue != "true" {
		t.Errorf("DefaultValue = %q, want the StaticValue carried through as %q", got[0].DefaultValue, "true")
	}
}

// ── createBundle ─────────────────────────────────────────────────────────────

func TestCreateBundle_MessageHeaderSortedFirst(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "p1"},
		{"resourceType": "Encounter", "id": "e1"},
		{"resourceType": "MessageHeader", "id": "mh1"},
	}
	bundle := svc.createBundle(resources, "req-123", "ADT^A01")

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 3 {
		t.Fatalf("bundle entries = %v, want 3 entries", bundle["entry"])
	}
	first, ok := entries[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first entry not a map: %v", entries[0])
	}
	firstResource, ok := first["resource"].(map[string]interface{})
	if !ok || firstResource["resourceType"] != "MessageHeader" {
		t.Errorf("first bundle entry resourceType = %v, want MessageHeader (bdl-12 requires it first)", firstResource["resourceType"])
	}
	if bundle["resourceType"] != "Bundle" || bundle["type"] != "message" {
		t.Errorf("bundle envelope = %+v, want resourceType=Bundle, type=message", bundle)
	}
}

func TestCreateBundle_SanitizesRequestIDForFHIRID(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "p1"}}
	// Underscores are invalid in a FHIR id ([A-Za-z0-9\-\.]{1,64}).
	bundle := svc.createBundle(resources, "transform_v3_1775471815062417349", "ADT^A01")
	id, _ := bundle["id"].(string)
	for _, r := range id {
		if r == '_' {
			t.Errorf("bundle id %q still contains an underscore, invalid per FHIR id regex", id)
			break
		}
	}
	if len(id) > 64 {
		t.Errorf("bundle id length = %d, want <= 64", len(id))
	}
}

// ── stripArrayIndices / hl7PathKey ───────────────────────────────────────────

func TestStripArrayIndices(t *testing.T) {
	cases := map[string]string{
		"name[0].given[1]": "name.given",
		"identifier[0]":     "identifier",
		"birthDate":         "birthDate",
		"a[10].b[2].c":      "a.b.c",
	}
	for in, want := range cases {
		if got := stripArrayIndices(in); got != want {
			t.Errorf("stripArrayIndices(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHL7PathKey(t *testing.T) {
	cases := []struct {
		fm   FieldMapping
		want string
	}{
		{FieldMapping{SegmentName: "PID", HL7Field: "5"}, "PID.5"},
		{FieldMapping{SegmentName: "PID", HL7Field: "5", HL7Component: "1"}, "PID.5.1"},
	}
	for _, c := range cases {
		if got := hl7PathKey(c.fm); got != c.want {
			t.Errorf("hl7PathKey(%+v) = %q, want %q", c.fm, got, c.want)
		}
	}
}

// ── mergeMappings ─────────────────────────────────────────────────────────────

func TestMergeMappings_ReplaceAction_ChangesTargetAndTransform(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender", DataTypeTransform: "gender_mapping"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "replace", HL7Path: "PID.8", FHIRPath: "Patient.genderCustom", Transform: "custom_gender", IsRequired: true},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 1 {
		t.Fatalf("got %d mappings, want 1", len(got))
	}
	if got[0].FHIRElementPath != "genderCustom" {
		t.Errorf("FHIRElementPath = %q, want genderCustom", got[0].FHIRElementPath)
	}
	if got[0].DataTypeTransform != "custom_gender" {
		t.Errorf("DataTypeTransform = %q, want custom_gender", got[0].DataTypeTransform)
	}
	if !got[0].IsRequired {
		t.Errorf("IsRequired = false, want true (override set it)")
	}
}

func TestMergeMappings_ReplaceAction_UnknownPathIsNoOp(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "replace", HL7Path: "PID.99", FHIRPath: "Patient.bogus"},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 1 || got[0].FHIRElementPath != "gender" {
		t.Errorf("replace on unknown path mutated base: %+v", got)
	}
}

func TestMergeMappings_AddAction_AppendsNewMapping(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "add", HL7Path: "PID.29", FHIRPath: "Patient.deceasedDateTime", Transform: "ts_to_datetime"},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 2 {
		t.Fatalf("got %d mappings, want 2 (base + added)", len(got))
	}
	added := got[1]
	if added.SegmentName != "PID" || added.HL7Field != "29" {
		t.Errorf("added mapping HL7 source = %s.%s, want PID.29", added.SegmentName, added.HL7Field)
	}
	if added.FHIRResourceType != "Patient" || added.FHIRElementPath != "deceasedDateTime" {
		t.Errorf("added mapping FHIR target = %s.%s, want Patient.deceasedDateTime", added.FHIRResourceType, added.FHIRElementPath)
	}
}

func TestMergeMappings_AddAction_DuplicateHL7PathIsNoOp(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "add", HL7Path: "PID.8", FHIRPath: "Patient.genderDuplicate"},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 1 {
		t.Errorf("add with an already-indexed HL7 path should be a no-op, got %d mappings: %+v", len(got), got)
	}
}

func TestMergeMappings_AddAction_StaticValueUsesFHIRPathDedupKey(t *testing.T) {
	base := []FieldMapping{}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "add", HL7Path: "", FHIRPath: "Patient.active", Transform: "static_value", StaticValue: "true"},
		// A second static_value add at a DIFFERENT FHIRPath must NOT be
		// treated as a duplicate just because both have an empty HL7Path.
		{Action: "add", HL7Path: "", FHIRPath: "Patient.deceasedBoolean", Transform: "static_value", StaticValue: "false"},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 2 {
		t.Fatalf("got %d static_value mappings, want 2 (distinct FHIRPath dedup keys)", len(got))
	}
	for _, fm := range got {
		if fm.SegmentName != "" || fm.HL7Field != "" {
			t.Errorf("static_value mapping has a non-empty HL7 source: %+v", fm)
		}
	}
}

func TestMergeMappings_RemoveAction_DropsMapping(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender"},
		{SegmentName: "PID", HL7Field: "7", FHIRResourceType: "Patient", FHIRElementPath: "birthDate"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "remove", HL7Path: "PID.8"},
	}}
	got := mergeMappings(base, delta)
	if len(got) != 1 {
		t.Fatalf("got %d mappings, want 1 (one removed)", len(got))
	}
	if got[0].HL7Field != "7" {
		t.Errorf("wrong mapping remained: %+v, want PID.7 (birthDate)", got[0])
	}
}

func TestMergeMappings_BaseSliceNotMutated(t *testing.T) {
	base := []FieldMapping{
		{SegmentName: "PID", HL7Field: "8", FHIRResourceType: "Patient", FHIRElementPath: "gender"},
	}
	delta := &MappingDelta{Overrides: []MappingOverride{
		{Action: "replace", HL7Path: "PID.8", FHIRPath: "Patient.genderChanged"},
	}}
	mergeMappings(base, delta)
	if base[0].FHIRElementPath != "gender" {
		t.Errorf("mergeMappings mutated the base slice in place: base[0] = %+v", base[0])
	}
}

// ── applyOptionalSegmentBlocks ───────────────────────────────────────────────

func TestApplyOptionalSegmentBlocks_EmptyEnabled_ReturnsUnchanged(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "p1"}}
	got := applyOptionalSegmentBlocks(resources, map[string]interface{}{}, "ADT^A01", nil)
	if len(got) != 1 {
		t.Errorf("applyOptionalSegmentBlocks with nil enabled map = %+v, want resources unchanged", got)
	}
}

func TestApplyOptionalSegmentBlocks_AssemblesEncounterWhenEnabledAndAbsent(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "p1"}}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"PV1": {rtSeg("PV1", 0, rtField("PV1.2", "I"))},
		},
	}
	enabled := map[string]bool{"PV1_Encounter": true}

	got := applyOptionalSegmentBlocks(resources, parsedHL7Data, "ADT^A01", enabled)

	var sawEncounter bool
	for _, r := range got {
		if rt, _ := r["resourceType"].(string); rt == "Encounter" {
			sawEncounter = true
		}
	}
	if !sawEncounter {
		t.Errorf("expected an assembled Encounter when PV1_Encounter is enabled and PV1 is present, got %+v", got)
	}
}

func TestApplyOptionalSegmentBlocks_IdempotentWhenEncounterAlreadyExists(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "p1"},
		{"resourceType": "Encounter", "id": "existing-enc"},
	}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"PV1": {rtSeg("PV1", 0, rtField("PV1.2", "I"))},
		},
	}
	enabled := map[string]bool{"PV1_Encounter": true}

	got := applyOptionalSegmentBlocks(resources, parsedHL7Data, "ADT^A01", enabled)

	var encounterCount int
	for _, r := range got {
		if rt, _ := r["resourceType"].(string); rt == "Encounter" {
			encounterCount++
		}
	}
	if encounterCount != 1 {
		t.Errorf("expected exactly 1 Encounter (no duplicate assembled), got %d: %+v", encounterCount, got)
	}
}
