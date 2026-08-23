// services/transform_assembly_rules_test.go
//
// Regression coverage for the "focus" assembly rule case now collecting ALL
// matching TargetResource instances instead of just the first. Motivating bug:
// once repeat-instancing (see hl7_fhir_transform_service_v3.go's
// buildResourcesForType) lets IN1->Coverage and ROL->PractitionerRole produce
// more than one resource per message, the live assembly_rules rows that link
// MessageHeader.focus to Coverage (18 ADT event codes) and PractitionerRole
// (MFN^M02) would have silently linked only the first instance, dropping the
// rest, unless "focus" collects every match the same way the pre-existing
// "result" case already does.
package services

import "testing"

func TestApplyOneAssemblyRule_FocusLinksEveryMatchingInstance(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}

	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1"},
		{"resourceType": "Coverage", "id": "cov1"},
		{"resourceType": "Coverage", "id": "cov2"},
		{"resourceType": "Coverage", "id": "cov3"},
	}

	rule := AssemblyRule{
		RuleType:       "focus",
		SourceResource: "MessageHeader",
		TargetResource: "Coverage",
		ReferencePath:  "focus",
	}

	out := svc.applyOneAssemblyRule(rule, resources)

	var mh map[string]interface{}
	for _, r := range out {
		if r["resourceType"] == "MessageHeader" {
			mh = r
		}
	}
	if mh == nil {
		t.Fatal("MessageHeader missing from output")
	}

	focus, ok := mh["focus"].([]interface{})
	if !ok {
		t.Fatalf("expected focus to be a []interface{}, got %T: %v", mh["focus"], mh["focus"])
	}
	if len(focus) != 3 {
		t.Fatalf("expected 3 focus refs (one per Coverage instance), got %d: %v", len(focus), focus)
	}

	seen := make(map[string]bool)
	for _, f := range focus {
		fm, ok := f.(map[string]interface{})
		if !ok {
			t.Fatalf("focus entry is not a reference object: %v", f)
		}
		ref, _ := fm["reference"].(string)
		seen[ref] = true
	}
	for _, want := range []string{"Coverage/cov1", "Coverage/cov2", "Coverage/cov3"} {
		if !seen[want] {
			t.Errorf("expected focus to contain %q, got %v", want, focus)
		}
	}
}

func TestApplyOneAssemblyRule_FocusIsDedupSafe(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}

	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1", "focus": []interface{}{
			map[string]interface{}{"reference": "Coverage/cov1"},
		}},
		{"resourceType": "Coverage", "id": "cov1"},
		{"resourceType": "Coverage", "id": "cov2"},
	}

	rule := AssemblyRule{
		RuleType:       "focus",
		SourceResource: "MessageHeader",
		TargetResource: "Coverage",
		ReferencePath:  "focus",
	}

	out := svc.applyOneAssemblyRule(rule, resources)
	mh := out[0]
	focus, _ := mh["focus"].([]interface{})
	if len(focus) != 2 {
		t.Fatalf("expected cov1 (already present) not duplicated and cov2 added, got %d entries: %v", len(focus), focus)
	}
}

func TestApplyOneAssemblyRule_FocusNoMatchingTargetReturnsUnchanged(t *testing.T) {
	svc := &HL7FHIRTransformServiceV3{}
	resources := []map[string]interface{}{
		{"resourceType": "MessageHeader", "id": "mh1"},
	}
	rule := AssemblyRule{RuleType: "focus", SourceResource: "MessageHeader", TargetResource: "Coverage", ReferencePath: "focus"}

	out := svc.applyOneAssemblyRule(rule, resources)
	if _, ok := out[0]["focus"]; ok {
		t.Errorf("expected no focus field when no Coverage instances exist, got %v", out[0]["focus"])
	}
}
