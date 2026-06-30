// services/fhir_narrative/observation_narrative_test.go
package fhirnarrative

import (
	"strings"
	"testing"
)

func observationWithCategory(categoryCode string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Observation",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{"code": categoryCode},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"display": "Test Code Display"},
			},
		},
		"valueString": "some finding",
	}
}

func TestGenerateObservationNarrative_VitalSigns_UsesVitalSignLabels(t *testing.T) {
	html := GenerateObservationNarrative(observationWithCategory("vital-signs"))
	if !strings.Contains(html, "Vital Sign") {
		t.Errorf("expected 'Vital Sign' heading/label, got: %s", html)
	}
	if strings.Contains(html, "Laboratory Test") {
		t.Errorf("vital-signs narrative must not say 'Laboratory Test', got: %s", html)
	}
}

func TestGenerateObservationNarrative_Laboratory_UsesLaboratoryLabels(t *testing.T) {
	html := GenerateObservationNarrative(observationWithCategory("laboratory"))
	if !strings.Contains(html, "Laboratory Test") {
		t.Errorf("expected 'Laboratory Test' heading/label for an actual lab result, got: %s", html)
	}
}

// TestGenerateObservationNarrative_NonLabCategories_NeverSayLaboratoryTest is
// the regression test for the real gap found auditing Mental Status: every
// category that isn't vital-signs used to fall into the same "Laboratory
// Test" branch. Social History, Functional Status, and Cognitive Status are
// the three real, in-use categories this previously mislabeled.
func TestGenerateObservationNarrative_NonLabCategories_NeverSayLaboratoryTest(t *testing.T) {
	cases := map[string]string{
		"social-history":     "Social History",
		"functional-status":  "Functional Status",
		"cognitive-status":   "Cognitive Status",
	}
	for category, wantHeading := range cases {
		t.Run(category, func(t *testing.T) {
			html := GenerateObservationNarrative(observationWithCategory(category))
			if strings.Contains(html, "Laboratory Test") {
				t.Errorf("category=%s: narrative must not say 'Laboratory Test', got: %s", category, html)
			}
			if !strings.Contains(html, wantHeading) {
				t.Errorf("category=%s: expected heading/label %q, got: %s", category, wantHeading, html)
			}
		})
	}
}

func TestGenerateObservationNarrative_UnknownCategory_FallsBackToGenericObservation(t *testing.T) {
	html := GenerateObservationNarrative(observationWithCategory("some-future-category"))
	if strings.Contains(html, "Laboratory Test") {
		t.Errorf("unknown category must not guess 'Laboratory Test', got: %s", html)
	}
	if !strings.Contains(html, "Observation") {
		t.Errorf("expected generic 'Observation' fallback heading/label, got: %s", html)
	}
}

// TestGenerateObservationNarrative_LabExtras_OnlyShowForLaboratoryCategory
// confirms Result Unit/Reference Range/Result Status (previously gated on
// "!isVital", so EVERY non-vital category showed them) only render for the
// one category they're actually meaningful for.
func TestGenerateObservationNarrative_LabExtras_OnlyShowForLaboratoryCategory(t *testing.T) {
	r := observationWithCategory("cognitive-status")
	delete(r, "valueString")
	r["valueQuantity"] = map[string]interface{}{"value": "5", "unit": "points"}
	r["referenceRange"] = []interface{}{
		map[string]interface{}{"low": map[string]interface{}{"value": "0"}, "high": map[string]interface{}{"value": "10"}},
	}

	html := GenerateObservationNarrative(r)
	if strings.Contains(html, "Result Unit") || strings.Contains(html, "Reference Range") || strings.Contains(html, "Result Status") {
		t.Errorf("cognitive-status narrative must not show lab-only rows, got: %s", html)
	}

	labHTML := GenerateObservationNarrative(observationWithCategoryAndQuantity("laboratory", r))
	if !strings.Contains(labHTML, "Result Unit") || !strings.Contains(labHTML, "Reference Range") || !strings.Contains(labHTML, "Result Status") {
		t.Errorf("laboratory narrative should still show lab-only rows, got: %s", labHTML)
	}
}

func observationWithCategoryAndQuantity(categoryCode string, base map[string]interface{}) map[string]interface{} {
	r := observationWithCategory(categoryCode)
	delete(r, "valueString")
	r["valueQuantity"] = base["valueQuantity"]
	r["referenceRange"] = base["referenceRange"]
	return r
}
