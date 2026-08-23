// services/transform_assembly_rules_coverage_test.go
//
// Coverage for the assembly-rules composite layer beyond the existing "focus"
// rule tests in transform_assembly_rules_test.go: the "reference"/"subject"/
// "encounter", "result", "performer"/"author", and "logical_ref" rule types,
// plus assemblyFindID, augmentMessageHeaderFocus, InvalidateAssemblyRuleCache,
// and applyAssemblyRules' cache-hit orchestration path (pre-populating the
// in-memory cache directly — same technique as every other DB-backed test in
// this codebase — bypasses the DB query entirely, no mocking needed).
//
// This is the layer that wires cross-resource references for ORU
// observations, MDM documents, DFT charges, VXU immunizations, and MFN
// master-file resources — previously almost entirely untested outside the
// one "focus" rule type.
package services

import (
	"context"
	"testing"
)

// ── "reference"/"subject"/"encounter" rule type ─────────────────────────────

func TestApplyOneAssemblyRule_Reference_WiresWhenAbsent(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Encounter", "id": "enc1"},
		{"resourceType": "Observation", "id": "obs1"},
	}
	rule := AssemblyRule{RuleType: "encounter", SourceResource: "Observation", TargetResource: "Encounter", ReferencePath: "encounter"}

	out := svc.applyOneAssemblyRule(rule, resources)
	obs := out[1]
	ref, ok := obs["encounter"].(map[string]interface{})
	if !ok || ref["reference"] != "Encounter/enc1" {
		t.Errorf("Observation.encounter = %+v, want reference Encounter/enc1", obs["encounter"])
	}
}

func TestApplyOneAssemblyRule_Reference_ArrayPathWrapsInSlice(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Appointment", "id": "appt1"},
		{"resourceType": "ServiceRequest", "id": "sr1"},
	}
	rule := AssemblyRule{RuleType: "reference", SourceResource: "ServiceRequest", TargetResource: "Appointment", ReferencePath: "basedOn"}

	out := svc.applyOneAssemblyRule(rule, resources)
	sr := out[1]
	arr, ok := sr["basedOn"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("ServiceRequest.basedOn = %+v, want a 1-element array (basedOn is a cardinality-array path)", sr["basedOn"])
	}
	refObj, _ := arr[0].(map[string]interface{})
	if refObj["reference"] != "Appointment/appt1" {
		t.Errorf("basedOn[0] = %+v, want reference Appointment/appt1", refObj)
	}
}

func TestApplyOneAssemblyRule_Reference_NonSubjectSkipsWhenAlreadySet(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Encounter", "id": "enc1"},
		{"resourceType": "Observation", "id": "obs1", "encounter": map[string]interface{}{"reference": "Encounter/already-set"}},
	}
	rule := AssemblyRule{RuleType: "encounter", SourceResource: "Observation", TargetResource: "Encounter", ReferencePath: "encounter"}

	out := svc.applyOneAssemblyRule(rule, resources)
	obs := out[1]
	ref := obs["encounter"].(map[string]interface{})
	if ref["reference"] != "Encounter/already-set" {
		t.Errorf("encounter rule overwrote an already-set value: got %v", ref["reference"])
	}
}

func TestApplyOneAssemblyRule_Subject_OverwritesDisplayOnlyValue(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "pat1"},
		{"resourceType": "Observation", "id": "obs1", "subject": map[string]interface{}{"display": "Doe, John"}},
	}
	rule := AssemblyRule{RuleType: "subject", SourceResource: "Observation", TargetResource: "Patient", ReferencePath: "subject"}

	out := svc.applyOneAssemblyRule(rule, resources)
	obs := out[1]
	ref := obs["subject"].(map[string]interface{})
	if ref["reference"] != "Patient/pat1" {
		t.Errorf("subject rule did not overwrite a display-only value: got %+v", ref)
	}
}

func TestApplyOneAssemblyRule_Subject_DoesNotOverwriteAlreadyWiredReference(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "pat1"},
		{"resourceType": "Observation", "id": "obs1", "subject": map[string]interface{}{"reference": "Patient/already-correct"}},
	}
	rule := AssemblyRule{RuleType: "subject", SourceResource: "Observation", TargetResource: "Patient", ReferencePath: "subject"}

	out := svc.applyOneAssemblyRule(rule, resources)
	ref := out[1]["subject"].(map[string]interface{})
	if ref["reference"] != "Patient/already-correct" {
		t.Errorf("subject rule overwrote an already-correctly-wired reference: got %v", ref["reference"])
	}
}

func TestApplyOneAssemblyRule_Reference_NoTargetFound_ReturnsUnchanged(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{{"resourceType": "Observation", "id": "obs1"}}
	rule := AssemblyRule{RuleType: "encounter", SourceResource: "Observation", TargetResource: "Encounter", ReferencePath: "encounter"}

	out := svc.applyOneAssemblyRule(rule, resources)
	if _, ok := out[0]["encounter"]; ok {
		t.Errorf("expected no encounter field when no Encounter exists, got %v", out[0]["encounter"])
	}
}

// ── "result" rule type ───────────────────────────────────────────────────────

func TestApplyOneAssemblyRule_Result_CollectsAllMatchingTargets(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "DiagnosticReport", "id": "dr1"},
		{"resourceType": "Observation", "id": "obs1"},
		{"resourceType": "Observation", "id": "obs2"},
	}
	rule := AssemblyRule{RuleType: "result", SourceResource: "DiagnosticReport", TargetResource: "Observation", ReferencePath: "result"}

	out := svc.applyOneAssemblyRule(rule, resources)
	refs, ok := out[0]["result"].([]interface{})
	if !ok || len(refs) != 2 {
		t.Fatalf("DiagnosticReport.result = %+v, want 2 refs (one per Observation)", out[0]["result"])
	}
}

func TestApplyOneAssemblyRule_Result_OverwritesExistingPlaceholder(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "DiagnosticReport", "id": "dr1", "result": []interface{}{map[string]interface{}{"reference": "Observation/placeholder"}}},
		{"resourceType": "Observation", "id": "obs1"},
	}
	rule := AssemblyRule{RuleType: "result", SourceResource: "DiagnosticReport", TargetResource: "Observation", ReferencePath: "result"}

	out := svc.applyOneAssemblyRule(rule, resources)
	refs := out[0]["result"].([]interface{})
	if len(refs) != 1 {
		t.Fatalf("expected the placeholder replaced by the 1 real Observation ref, got %d entries: %v", len(refs), refs)
	}
	refObj := refs[0].(map[string]interface{})
	if refObj["reference"] != "Observation/obs1" {
		t.Errorf("result[0] = %+v, want reference Observation/obs1 (placeholder must be replaced)", refObj)
	}
}

func TestApplyOneAssemblyRule_Result_NoTargets_LeavesExistingValueUntouched(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "DiagnosticReport", "id": "dr1", "result": "untouched"},
	}
	rule := AssemblyRule{RuleType: "result", SourceResource: "DiagnosticReport", TargetResource: "Observation", ReferencePath: "result"}

	out := svc.applyOneAssemblyRule(rule, resources)
	if out[0]["result"] != "untouched" {
		t.Errorf("expected result left unchanged when there are zero targets, got %v", out[0]["result"])
	}
}

// ── "performer"/"author" rule type ───────────────────────────────────────────

func TestApplyOneAssemblyRule_Performer_WiresWhenAbsent(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Practitioner", "id": "prac1"},
		{"resourceType": "Observation", "id": "obs1"},
	}
	rule := AssemblyRule{RuleType: "performer", SourceResource: "Observation", TargetResource: "Practitioner", ReferencePath: "performer"}

	out := svc.applyOneAssemblyRule(rule, resources)
	arr, ok := out[1]["performer"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("Observation.performer = %+v, want a 1-element array", out[1]["performer"])
	}
	refObj := arr[0].(map[string]interface{})
	if refObj["reference"] != "Practitioner/prac1" {
		t.Errorf("performer[0] = %+v, want reference Practitioner/prac1", refObj)
	}
}

func TestApplyOneAssemblyRule_Performer_SkipsWhenAlreadyPresent(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Practitioner", "id": "prac1"},
		{"resourceType": "Observation", "id": "obs1", "performer": []interface{}{map[string]interface{}{"reference": "Practitioner/already-set"}}},
	}
	rule := AssemblyRule{RuleType: "author", SourceResource: "Observation", TargetResource: "Practitioner", ReferencePath: "performer"}

	out := svc.applyOneAssemblyRule(rule, resources)
	arr := out[1]["performer"].([]interface{})
	if len(arr) != 1 || arr[0].(map[string]interface{})["reference"] != "Practitioner/already-set" {
		t.Errorf("performer/author rule overwrote an already-set field: got %v", arr)
	}
}

// ── "logical_ref" rule type ─────────────────────────────────────────────────

func TestApplyOneAssemblyRule_LogicalRef_ResolvesFromTargetResourceFields(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Organization", "id": "org1", "identifierValue": "PAYER123", "name": "Acme Insurance"},
		{"resourceType": "Coverage", "id": "cov1"},
	}
	rule := AssemblyRule{
		RuleType: "logical_ref", SourceResource: "Coverage", TargetResource: "Organization", ReferencePath: "payor",
		Config: map[string]interface{}{
			"identifier_system": "urn:local:payer",
			"identifier_field":  "identifierValue",
			"display_field":     "name",
		},
	}

	out := svc.applyOneAssemblyRule(rule, resources)
	arr, ok := out[1]["payor"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("Coverage.payor = %+v, want a 1-element array", out[1]["payor"])
	}
	ref := arr[0].(map[string]interface{})
	id, ok := ref["identifier"].(map[string]interface{})
	if !ok || id["system"] != "urn:local:payer" || id["value"] != "PAYER123" {
		t.Errorf("payor[0].identifier = %+v, want system=urn:local:payer, value=PAYER123", ref["identifier"])
	}
	if ref["display"] != "Acme Insurance" {
		t.Errorf("payor[0].display = %v, want Acme Insurance", ref["display"])
	}
}

func TestApplyOneAssemblyRule_LogicalRef_FallsBackToExistingDisplayOnlyValue(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	// No Organization resource in the bundle at all — must fall back to the
	// display-only reference the field mapper already produced.
	resources := []map[string]interface{}{
		{"resourceType": "Coverage", "id": "cov1", "payor": map[string]interface{}{"display": "Acme Insurance"}},
	}
	rule := AssemblyRule{
		RuleType: "logical_ref", SourceResource: "Coverage", TargetResource: "Organization", ReferencePath: "payor",
		Config: map[string]interface{}{"identifier_system": "urn:local:payer", "identifier_field": "identifierValue"},
	}

	out := svc.applyOneAssemblyRule(rule, resources)
	arr, ok := out[0]["payor"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("Coverage.payor = %+v, want a 1-element array via the display-only fallback", out[0]["payor"])
	}
	ref := arr[0].(map[string]interface{})
	id := ref["identifier"].(map[string]interface{})
	if id["value"] != "Acme Insurance" || ref["display"] != "Acme Insurance" {
		t.Errorf("payor[0] = %+v, want identifier.value and display both = Acme Insurance (fallback uses the display text as the identifier value)", ref)
	}
}

func TestApplyOneAssemblyRule_LogicalRef_NoIdentifierValueAnywhere_LeavesFieldAlone(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Coverage", "id": "cov1"}, // no payor field, no Organization in bundle
	}
	rule := AssemblyRule{
		RuleType: "logical_ref", SourceResource: "Coverage", TargetResource: "Organization", ReferencePath: "payor",
		Config: map[string]interface{}{"identifier_field": "identifierValue"},
	}

	out := svc.applyOneAssemblyRule(rule, resources)
	if _, ok := out[0]["payor"]; ok {
		t.Errorf("expected payor left unset when there's no identifier value to resolve, got %v", out[0]["payor"])
	}
}

func TestApplyOneAssemblyRule_LogicalRef_DisplayFieldDefaultsToIdentifierField(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "Organization", "id": "org1", "identifierValue": "PAYER123"},
		{"resourceType": "Coverage", "id": "cov1"},
	}
	rule := AssemblyRule{
		RuleType: "logical_ref", SourceResource: "Coverage", TargetResource: "Organization", ReferencePath: "payor",
		// display_field intentionally omitted — should default to identifier_field.
		Config: map[string]interface{}{"identifier_system": "urn:local:payer", "identifier_field": "identifierValue"},
	}

	out := svc.applyOneAssemblyRule(rule, resources)
	ref := out[1]["payor"].([]interface{})[0].(map[string]interface{})
	if ref["display"] != "PAYER123" {
		t.Errorf("display = %v, want PAYER123 (display_field should default to identifier_field's value)", ref["display"])
	}
}

// ── assemblyFindID ────────────────────────────────────────────────────────────

func TestAssemblyFindID(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "pat1"},
		{"resourceType": "Encounter", "id": "enc1"},
	}
	if got := assemblyFindID(resources, "Encounter"); got != "enc1" {
		t.Errorf("assemblyFindID(Encounter) = %q, want enc1", got)
	}
	if got := assemblyFindID(resources, "Observation"); got != "" {
		t.Errorf("assemblyFindID(Observation) = %q, want empty string (no match)", got)
	}
}

// ── augmentMessageHeaderFocus ─────────────────────────────────────────────────

func TestAugmentMessageHeaderFocus_NoFocusTypes_NoOp(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "MessageHeader", "id": "mh1"}}
	augmentMessageHeaderFocus(resources, nil)
	if _, ok := resources[0]["focus"]; ok {
		t.Errorf("expected no focus field to be added, got %v", resources[0]["focus"])
	}
}

func TestAugmentMessageHeaderFocus_NoMessageHeader_NoOp(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "pat1"}}
	// Must not panic when there's no MessageHeader to augment.
	augmentMessageHeaderFocus(resources, []string{"Patient"})
	if _, ok := resources[0]["focus"]; ok {
		t.Errorf("Patient resource unexpectedly got a focus field: %v", resources[0]["focus"])
	}
}

func TestAugmentMessageHeaderFocus_AddsReferencesForWantedTypes(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1"},
		{"resourceType": "Patient", "id": "pat1"},
		{"resourceType": "AllergyIntolerance", "id": "al1"},
		{"resourceType": "Encounter", "id": "enc1"}, // not in focusTypes — must be excluded
	}
	augmentMessageHeaderFocus(resources, []string{"Patient", "AllergyIntolerance"})

	mh := resources[0]
	focus, ok := mh["focus"].([]interface{})
	if !ok || len(focus) != 2 {
		t.Fatalf("MessageHeader.focus = %+v, want 2 entries (Patient + AllergyIntolerance, Encounter excluded)", mh["focus"])
	}
	seen := map[string]bool{}
	for _, f := range focus {
		ref, _ := f.(map[string]interface{})["reference"].(string)
		seen[ref] = true
	}
	if !seen["Patient/pat1"] || !seen["AllergyIntolerance/al1"] {
		t.Errorf("focus = %v, want Patient/pat1 and AllergyIntolerance/al1 present", focus)
	}
	if seen["Encounter/enc1"] {
		t.Errorf("focus unexpectedly includes Encounter/enc1, which was not in focusTypes")
	}
}

func TestAugmentMessageHeaderFocus_DoesNotDuplicateExistingReferences(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1", "focus": []interface{}{
			map[string]interface{}{"reference": "Patient/pat1"},
		}},
		{"resourceType": "Patient", "id": "pat1"},
	}
	augmentMessageHeaderFocus(resources, []string{"Patient"})

	focus := resources[0]["focus"].([]interface{})
	if len(focus) != 1 {
		t.Errorf("expected Patient/pat1 not duplicated, got %d entries: %v", len(focus), focus)
	}
}

func TestAugmentMessageHeaderFocus_IgnoresResourcesWithNoID(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1"},
		{"resourceType": "Patient"}, // no id — must be skipped, not crash
	}
	// augmentMessageHeaderFocus always assigns mh["focus"] (even to an empty
	// slice) once focusTypes is non-empty and a MessageHeader was found — so
	// the key existing isn't the signal; its length is.
	augmentMessageHeaderFocus(resources, []string{"Patient"})
	if focus, ok := resources[0]["focus"].([]interface{}); ok && len(focus) != 0 {
		t.Errorf("expected zero focus entries for a Patient resource with no id, got %v", focus)
	}
}

// ── InvalidateAssemblyRuleCache ───────────────────────────────────────────────

func TestInvalidateAssemblyRuleCache_ClearsCache(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{
		assemblyRules: map[string][]AssemblyRule{"ADT^A01": {{RuleType: "focus"}}},
	}
	svc.InvalidateAssemblyRuleCache()
	if svc.assemblyRules != nil {
		t.Errorf("expected assemblyRules to be nil after invalidation, got %+v", svc.assemblyRules)
	}
}

// ── applyAssemblyRules (cache-hit orchestration path) ────────────────────────

func TestApplyAssemblyRules_EmptyRulesForMessageType_ReturnsUnchanged(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{
		assemblyRules: map[string][]AssemblyRule{"ORU^R01": {{RuleType: "focus"}}}, // populated cache, but not for ADT^A01
	}
	resources := []map[string]interface{}{{"resourceType": "Patient", "id": "pat1"}}
	out := svc.applyAssemblyRules(context.Background(), "ADT^A01", resources)
	if len(out) != 1 || out[0]["id"] != "pat1" {
		t.Errorf("expected resources unchanged when no rules exist for this message type, got %+v", out)
	}
}

func TestApplyAssemblyRules_AppliesEveryCachedRuleForMessageType(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{
		assemblyRules: map[string][]AssemblyRule{
			"ADT^A01": {
				{RuleType: "encounter", SourceResource: "Observation", TargetResource: "Encounter", ReferencePath: "encounter"},
			},
		},
	}
	resources := []map[string]interface{}{
		{"resourceType": "Encounter", "id": "enc1"},
		{"resourceType": "Observation", "id": "obs1"},
	}
	out := svc.applyAssemblyRules(context.Background(), "ADT^A01", resources)

	var obs map[string]interface{}
	for _, r := range out {
		if r["resourceType"] == "Observation" {
			obs = r
		}
	}
	ref, ok := obs["encounter"].(map[string]interface{})
	if !ok || ref["reference"] != "Encounter/enc1" {
		t.Errorf("applyAssemblyRules did not wire the cached rule: Observation.encounter = %v", obs["encounter"])
	}
}

// ── loadAssemblyRulesForType (cache-hit path — no DB touch) ─────────────────

func TestLoadAssemblyRulesForType_CacheHit_ReturnsWithoutTouchingDB(t *testing.T) {
	// svc.db stays nil — if this test reaches the DB query path at all, it
	// panics on a nil *sql.DB, proving the cache-hit fast path is what ran.
	svc := &HL7FHIRTransformServiceV3{
		assemblyRules: map[string][]AssemblyRule{
			"ADT^A01": {{RuleType: "focus", TargetResource: "Coverage"}},
		},
	}
	got := svc.loadAssemblyRulesForType(context.Background(), "ADT^A01")
	if len(got) != 1 || got[0].TargetResource != "Coverage" {
		t.Errorf("loadAssemblyRulesForType(cache hit) = %+v, want the single cached rule", got)
	}
}

func TestLoadAssemblyRulesForType_CacheHit_UnknownMessageType_ReturnsNil(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{
		assemblyRules: map[string][]AssemblyRule{"ORU^R01": {{RuleType: "focus"}}},
	}
	got := svc.loadAssemblyRulesForType(context.Background(), "ADT^A01")
	if got != nil {
		t.Errorf("got %+v, want nil for a message type with no cached rules", got)
	}
}
