// services/fhir_narrative/generic_narrative_test.go
//
// Regression coverage for generateGeneric and its wiring into Generate().
// The motivating bug: AllergyIntolerance had no dedicated builder anywhere and
// fell through to a scalar-only fallback (services/transform_narrative.go's
// addGenericRows, since deleted) that silently dropped code/criticality/reaction
// — all nested objects/arrays — leaving only onsetDateTime visible.
package fhirnarrative

import (
	"strings"
	"testing"
)

func TestGenerate_AllergyIntolerance_ShowsCodeAndCriticalityAndReaction(t *testing.T) {
	// AllergyIntolerance IS registered (GenerateAllergyNarrative) — this proves
	// the dedicated builder itself surfaces all three, using the exact shape
	// this engine's own field-mapping produces (confirmed against a real
	// transformed message): criticality mapped as a CodeableConcept instead of
	// the spec's plain code — a separate, known upstream mapping bug, but the
	// narrative renderer must not compound it by silently dropping the value.
	resource := map[string]interface{}{
		"resourceType": "AllergyIntolerance",
		"id":            "allergy-1",
		"code": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"code": "STRW", "display": "Strawberry"}},
		},
		"criticality": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"code": "High"}},
		},
		"reaction": []interface{}{
			map[string]interface{}{"description": "Hives"},
		},
		"onsetDateTime": "2026-07-10",
	}

	html := NewFHIRNarrativeGenerator().Generate(resource, nil, nil)
	for _, want := range []string{"Strawberry", "High", "Hives", "2026-07-10"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected narrative to contain %q, got: %s", want, html)
		}
	}
}

func TestGenerate_UnregisteredType_UsesGenericRendererNotEmptyString(t *testing.T) {
	// ChargeItemDefinition has no dedicated builder in this package. Before
	// this change, Generate's default case returned "" for any unregistered
	// type — now it falls back to the generic renderer instead.
	resource := map[string]interface{}{
		"resourceType": "ChargeItemDefinition",
		"id":            "cid-1",
		"title":         "Room Charge",
		"status":        "active",
	}
	html := NewFHIRNarrativeGenerator().Generate(resource, nil, nil)
	if html == "" {
		t.Fatal("expected non-empty narrative for an unregistered resource type")
	}
	for _, want := range []string{"Room Charge", "active"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected narrative to contain %q, got: %s", want, html)
		}
	}
}

func TestGenerateGeneric_RendersEveryShapeCategory(t *testing.T) {
	resource := map[string]interface{}{
		"resourceType": "TestResource",
		"id":            "tr-1",
		"scalarField":   "plain value",
		"codeableConceptField": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"code": "X", "display": "Ex Display"}},
		},
		"referenceField": map[string]interface{}{
			"reference": "Patient/p1", "display": "Jane Doe",
		},
		"quantityField": map[string]interface{}{"value": 5.0, "unit": "mg"},
		"periodField":   map[string]interface{}{"start": "2026-01-01", "end": "2026-01-02"},
		"arrayField":    []interface{}{"a", "b"},
	}
	html := generateGeneric("TestResource", resource, nil, nil)
	for _, want := range []string{"plain value", "Ex Display", "Jane Doe", "5 mg", "2026-01-01", "2026-01-02", "a; b"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected narrative to contain %q, got: %s", want, html)
		}
	}
}

func TestGenerateGeneric_EncounterClassCode_UsesFriendlyLabel(t *testing.T) {
	resource := map[string]interface{}{
		"resourceType": "Encounter",
		"id":            "enc-1",
		"class":         "I",
	}
	html := generateGeneric("Encounter", resource, nil, nil)
	if !strings.Contains(html, "Inpatient") {
		t.Errorf("expected 'Inpatient' friendly label for Encounter.class=I, got: %s", html)
	}
	if strings.Contains(html, ">I<") {
		t.Errorf("expected raw code 'I' to be translated, not shown literally, got: %s", html)
	}
}

func TestGenerate_FieldConfigRestriction_AppliesEvenToRegisteredType(t *testing.T) {
	// AllergyIntolerance has a dedicated builder, but a fieldConfig restriction
	// must still take effect — this is what makes per-interface narrative
	// configuration behave uniformly regardless of whether a type happens to
	// have hand-written logic.
	resource := map[string]interface{}{
		"resourceType": "AllergyIntolerance",
		"id":            "allergy-1",
		"code": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"code": "STRW", "display": "Strawberry"}},
		},
		"criticality": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"code": "High"}},
		},
		"onsetDateTime": "2026-07-10",
	}
	cfg := NarrativeFieldConfig{"AllergyIntolerance": {"code"}}

	html := NewFHIRNarrativeGenerator().Generate(resource, nil, cfg)
	if !strings.Contains(html, "Strawberry") {
		t.Errorf("expected the configured field 'code' to render, got: %s", html)
	}
	if strings.Contains(html, "High") || strings.Contains(html, "2026-07-10") {
		t.Errorf("expected fields NOT in the config (criticality, onsetDateTime) to be excluded, got: %s", html)
	}
}

func TestGenerate_ExistingNarrativePassesThroughUnchanged(t *testing.T) {
	resource := map[string]interface{}{
		"resourceType": "Patient",
		"text":          map[string]interface{}{"status": "additional", "div": "<div>already there</div>"},
	}
	html := NewFHIRNarrativeGenerator().Generate(resource, nil, nil)
	if html != "<div>already there</div>" {
		t.Errorf("expected pre-existing narrative to pass through unchanged, got: %s", html)
	}
}

func TestGenerate_NilResource_ReturnsEmptyString(t *testing.T) {
	if got := NewFHIRNarrativeGenerator().Generate(nil, nil, nil); got != "" {
		t.Errorf("expected empty string for nil resource, got: %q", got)
	}
}

func TestNarrativeFieldConfig_AllowsField(t *testing.T) {
	var nilCfg NarrativeFieldConfig
	if !nilCfg.AllowsField("Patient", "name") {
		t.Error("nil config should allow every field")
	}

	cfg := NarrativeFieldConfig{"Patient": {"name", "gender"}}
	if !cfg.AllowsField("Patient", "name") {
		t.Error("configured field should be allowed")
	}
	if cfg.AllowsField("Patient", "birthDate") {
		t.Error("unconfigured field should be excluded once a restriction exists for that type")
	}
	if !cfg.AllowsField("Encounter", "status") {
		t.Error("a resourceType absent from the config entirely should allow everything")
	}
}
