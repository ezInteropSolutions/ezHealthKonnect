// services/hl7_fhir_transform_service_v3_repeat_instancing_test.go
//
// Regression coverage for buildResourcesForType's segment-repeat-instancing
// logic (see its doc comment in hl7_fhir_transform_service_v3.go). Confirms:
//   - A segment in segmentSpawnsSeparateResource (NK1, GT1, AL1, ...) produces
//     one resource PER OCCURRENCE, never merged with another occurrence of the
//     same segment or with a different segment mapped to the same resource
//     type — this is the exact bug found on a real ADT^A08 test message where
//     3 NK1 (next-of-kin) + 1 GT1 (guarantor) segments collapsed into a single
//     garbled RelatedPerson.
//   - A segment NOT in that table (EVN, PV1, ...) keeps the pre-existing
//     merge-into-one-resource behavior, unchanged.
//
// No database or FHIR server required — only the real FHIR schema registry
// (loaded from schemas/fhir on disk, same pattern as fhir/legacy_shape_test.go).
package services

import (
	"strings"
	"testing"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/hl7"
)

// ─── local test helpers (mirrors services/hl7assembly/uscore_validator_test.go) ──

func rtField(key, value string) hl7.FieldInfo {
	return hl7.FieldInfo{Key: key, Value: value, HasValue: value != ""}
}

func rtSeg(key string, index int, fields ...hl7.FieldInfo) hl7.EnhancedSegment {
	return hl7.EnhancedSegment{Key: key, Fields: fields, SegmentIndex: index}
}

func mustSchema(t *testing.T, svc *HL7FHIRTransformServiceV3, resourceType string) *fhir.FHIRSchema {
	t.Helper()
	if err := r4.InitRegistry("../schemas/fhir"); err != nil {
		t.Fatalf("InitRegistry: %v", err)
	}
	schema, err := svc.loadFHIRSchema(resourceType, "", "R4")
	if err != nil {
		t.Fatalf("loadFHIRSchema(%s): %v", resourceType, err)
	}
	return schema
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestBuildResourcesForType_NK1AndGT1SpawnSeparateRelatedPersons is a direct
// regression test for the reported bug: 3 NK1 (2 real next-of-kin + 1 blank)
// plus 1 GT1 (guarantor), all mapped to RelatedPerson, must produce 3 distinct
// resources — 2 from real NK1 data, 1 from GT1 data — with no cross-segment
// field bleed and the blank NK1 occurrence silently skipped (no data mapped).
func TestBuildResourcesForType_NK1AndGT1SpawnSeparateRelatedPersons(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	schema := mustSchema(t, svc, "RelatedPerson")

	mappings := []FieldMapping{
		{SegmentName: "NK1", HL7Field: "2", FHIRResourceType: "RelatedPerson", FHIRElementPath: "RelatedPerson.name[0]", DataTypeTransform: "xpn_to_humanname"},
		{SegmentName: "NK1", HL7Field: "3", FHIRResourceType: "RelatedPerson", FHIRElementPath: "RelatedPerson.relationship[0]", DataTypeTransform: "ce_to_codeableconcept"},
		{SegmentName: "GT1", HL7Field: "2", FHIRResourceType: "RelatedPerson", FHIRElementPath: "RelatedPerson.identifier[0]", DataTypeTransform: "cx_to_identifier"},
		{SegmentName: "GT1", HL7Field: "3", FHIRResourceType: "RelatedPerson", FHIRElementPath: "RelatedPerson.name[0]", DataTypeTransform: "xpn_to_humanname"},
	}

	nk1Occurrences := []hl7.EnhancedSegment{
		rtSeg("NK1", 0, rtField("NK1.2", "TEST^CINDY"), rtField("NK1.3", "MTH")),
		rtSeg("NK1", 1, rtField("NK1.2", "TEST^CINDY"), rtField("NK1.3", "MTH")),
		rtSeg("NK1", 2), // blank record (e.g. an employer NK1 with no name/relationship) — nothing to map
	}
	gt1Occurrences := []hl7.EnhancedSegment{
		rtSeg("GT1", 0, rtField("GT1.2", "4475683"), rtField("GT1.3", "TEST^MEGAN")),
	}

	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"NK1": nk1Occurrences,
			"GT1": gt1Occurrences,
		},
	}

	// Not asserting zero errs here: createResourceFromAtomicMappings' schema
	// validator flags RelatedPerson.patient as missing, which is populated by
	// wireSubjectReferences in Transform() (outside buildResourcesForType, the
	// unit under test) — irrelevant to the segment-isolation being verified.
	resources, _, _, _ := svc.buildResourcesForType("RelatedPerson", schema, mappings, map[string]interface{}{}, parsedHL7Data, nil)
	if len(resources) != 3 {
		t.Fatalf("expected 3 RelatedPerson resources (2 NK1 + 1 GT1, blank NK1 skipped), got %d: %+v", len(resources), resources)
	}

	var fromNK1, fromGT1 int
	for _, r := range resources {
		_, hasRelationship := r["relationship"]
		_, hasIdentifier := r["identifier"]
		if hasRelationship && hasIdentifier {
			t.Fatalf("resource has BOTH relationship (NK1) and identifier (GT1) fields — segments bled into each other: %+v", r)
		}
		if hasRelationship {
			fromNK1++
		}
		if hasIdentifier {
			fromGT1++
		}
	}
	if fromNK1 != 2 {
		t.Errorf("expected 2 resources built from NK1 (with relationship set), got %d", fromNK1)
	}
	if fromGT1 != 1 {
		t.Errorf("expected 1 resource built from GT1 (with identifier set), got %d", fromGT1)
	}
}

// TestBuildResourcesForType_SingleOccurrence_NoRegression confirms a message
// with only ONE occurrence of a spawn-eligible segment still produces exactly
// one resource, identical in shape to pre-fix behavior.
func TestBuildResourcesForType_SingleOccurrence_NoRegression(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	schema := mustSchema(t, svc, "RelatedPerson")

	mappings := []FieldMapping{
		{SegmentName: "NK1", HL7Field: "2", FHIRResourceType: "RelatedPerson", FHIRElementPath: "RelatedPerson.name[0]", DataTypeTransform: "xpn_to_humanname"},
	}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"NK1": {rtSeg("NK1", 0, rtField("NK1.2", "TEST^CINDY"))},
		},
	}

	// Not asserting zero errs — see the same note in the NK1+GT1 test above.
	resources, _, _, _ := svc.buildResourcesForType("RelatedPerson", schema, mappings, map[string]interface{}{}, parsedHL7Data, nil)
	if len(resources) != 1 {
		t.Fatalf("expected exactly 1 RelatedPerson for a single NK1 occurrence, got %d", len(resources))
	}
}

// TestBuildResourcesForType_NonRepeatingSegmentsStayMerged confirms EVN+PV1
// (neither in segmentSpawnsSeparateResource) still combine into ONE Encounter,
// exactly as before this change — no regression for the common merge case.
func TestBuildResourcesForType_NonRepeatingSegmentsStayMerged(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	schema := mustSchema(t, svc, "Encounter")

	mappings := []FieldMapping{
		{SegmentName: "EVN", HL7Field: "2", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.period.start", DataTypeTransform: "hl7_timestamp_to_fhir_date"},
		{SegmentName: "PV1", HL7Field: "2", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.class.code", DataTypeTransform: ""},
	}
	enhancedSegments := map[string]interface{}{
		"EVN": map[string]interface{}{"fields": []interface{}{
			map[string]interface{}{"key": "EVN.2", "value": "20260709224300"},
		}},
		"PV1": map[string]interface{}{"fields": []interface{}{
			map[string]interface{}{"key": "PV1.2", "value": "I"},
		}},
	}

	// Not asserting zero errs here: createResourceFromAtomicMappings' schema
	// validator flags Encounter.status as missing, which is populated by a
	// separate normalizer pass in Transform() (outside buildResourcesForType,
	// the unit under test) — irrelevant to the merge behavior being verified.
	resources, _, _, _ := svc.buildResourcesForType("Encounter", schema, mappings, enhancedSegments, map[string]interface{}{}, nil)
	if len(resources) != 1 {
		t.Fatalf("expected EVN+PV1 to merge into exactly 1 Encounter, got %d: %+v", len(resources), resources)
	}
	r := resources[0]
	if _, ok := r["period"]; !ok {
		t.Errorf("merged Encounter missing EVN-derived 'period' field: %+v", r)
	}
	if _, ok := r["class"]; !ok {
		t.Errorf("merged Encounter missing PV1-derived 'class' field: %+v", r)
	}
}

// TestBuildResourcesForType_PV1WinsOverEVNOnFieldCollision confirms
// segmentMergePriority forces PV1 to win over EVN when both segments map a
// value into the SAME FHIR path — deterministic by priority, not whichever
// happened to be listed last in the template JSON. Mirrors the real-world
// collision found in production: EVN.2 (event recorded date) and PV1.44
// (admit date) both mapping to Encounter.period.start.
func TestBuildResourcesForType_PV1WinsOverEVNOnFieldCollision(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	schema := mustSchema(t, svc, "Encounter")

	// EVN mapping listed FIRST in the array on purpose — if priority ordering
	// weren't applied, last-write-wins would already pick PV1 here by
	// accident; run it a second time with EVN listed LAST to prove priority
	// (not array position) is what decides the winner.
	runCase := func(mappings []FieldMapping) (map[string]interface{}, []string) {
		enhancedSegments := map[string]interface{}{
			"EVN": map[string]interface{}{"fields": []interface{}{
				map[string]interface{}{"key": "EVN.2", "value": "20260711142025"},
			}},
			"PV1": map[string]interface{}{"fields": []interface{}{
				map[string]interface{}{"key": "PV1.44", "value": "20260709224300"},
			}},
		}
		resources, warnings, _, _ := svc.buildResourcesForType("Encounter", schema, mappings, enhancedSegments, map[string]interface{}{}, nil)
		if len(resources) != 1 {
			t.Fatalf("expected exactly 1 merged Encounter, got %d", len(resources))
		}
		return resources[0], warnings
	}

	evnFirst := []FieldMapping{
		{SegmentName: "EVN", HL7Field: "2", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.period.start", DataTypeTransform: "hl7_timestamp_to_fhir_date"},
		{SegmentName: "PV1", HL7Field: "44", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.period.start", DataTypeTransform: "hl7_timestamp_to_fhir_date"},
	}
	pv1First := []FieldMapping{
		{SegmentName: "PV1", HL7Field: "44", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.period.start", DataTypeTransform: "hl7_timestamp_to_fhir_date"},
		{SegmentName: "EVN", HL7Field: "2", FHIRResourceType: "Encounter", FHIRElementPath: "Encounter.period.start", DataTypeTransform: "hl7_timestamp_to_fhir_date"},
	}

	for name, mappings := range map[string][]FieldMapping{"EVN listed first": evnFirst, "PV1 listed first": pv1First} {
		r, warnings := runCase(mappings)
		period, ok := r["period"].(map[string]interface{})
		if !ok {
			t.Fatalf("[%s] expected a period object, got %v", name, r["period"])
		}
		start, _ := period["start"].(string)
		if start == "" || start[:10] != "2026-07-09" {
			t.Errorf("[%s] expected PV1.44 (2026-07-09...) to win regardless of array order, got period.start=%q", name, start)
		}

		var sawCollisionWarning bool
		for _, w := range warnings {
			if strings.Contains(w, "PV1.44") && strings.Contains(w, "EVN.2") {
				sawCollisionWarning = true
			}
		}
		if !sawCollisionWarning {
			t.Errorf("[%s] expected a collision warning naming both EVN.2 and PV1.44, got warnings=%v", name, warnings)
		}
	}
}

// TestBuildResourcesForType_AL1RepeatsIntoSeparateAllergyIntolerances confirms
// 2 AL1 occurrences produce 2 independent AllergyIntolerance resources instead
// of the pre-fix behavior of collapsing to the last-parsed occurrence.
func TestBuildResourcesForType_AL1RepeatsIntoSeparateAllergyIntolerances(t *testing.T) {
	svc := NewHL7FHIRTransformServiceV3(nil)
	schema := mustSchema(t, svc, "AllergyIntolerance")

	mappings := []FieldMapping{
		{SegmentName: "AL1", HL7Field: "3", FHIRResourceType: "AllergyIntolerance", FHIRElementPath: "AllergyIntolerance.code", DataTypeTransform: "ce_to_codeableconcept", IsRequired: true},
	}
	parsedHL7Data := map[string]interface{}{
		"segmentGroups": map[string][]hl7.EnhancedSegment{
			"AL1": {
				rtSeg("AL1", 0, rtField("AL1.3", "^SULFA ANTIBIOTICS^")),
				rtSeg("AL1", 1, rtField("AL1.3", "STRW^STRAWBERRY^EXTELG")),
			},
		},
	}

	// Not asserting zero errs here: createResourceFromAtomicMappings' schema
	// validator flags AllergyIntolerance.patient as missing, which is populated
	// by wireSubjectReferences in Transform() (outside buildResourcesForType,
	// the unit under test) — irrelevant to the repeat-instancing being verified.
	resources, _, _, _ := svc.buildResourcesForType("AllergyIntolerance", schema, mappings, map[string]interface{}{}, parsedHL7Data, nil)
	if len(resources) != 2 {
		t.Fatalf("expected 2 AllergyIntolerance resources for 2 AL1 occurrences, got %d: %+v", len(resources), resources)
	}
	if resources[0]["id"] == resources[1]["id"] {
		t.Errorf("both AllergyIntolerance resources have the same id — occurrence disambiguation failed: %v", resources[0]["id"])
	}
}
