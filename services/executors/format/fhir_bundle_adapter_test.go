package format

import "testing"

// TestBundleToCanonicalDoc_AllergyIntolerance locks in the declarative
// resourceFieldMappings table's behavior for the field shape that most
// exercises it: code extraction, a value-translated status, a nested
// reaction manifestation two levels deep, and a plain scalar sibling field.
func TestBundleToCanonicalDoc_AllergyIntolerance(t *testing.T) {
	bundle := map[string]interface{}{
		"entry": []interface{}{
			map[string]interface{}{"resource": map[string]interface{}{
				"resourceType": "AllergyIntolerance",
				"code": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{"code": "7980", "display": "Penicillin", "system": "http://www.nlm.nih.gov/research/umls/rxnorm"},
					},
				},
				"clinicalStatus": map[string]interface{}{
					"coding": []interface{}{map[string]interface{}{"code": "active"}},
				},
				"reaction": []interface{}{
					map[string]interface{}{
						"manifestation": []interface{}{
							map[string]interface{}{"coding": []interface{}{
								map[string]interface{}{"code": "247472004", "display": "Hives"},
							}},
						},
						"severity": "moderate",
					},
				},
				"onsetDateTime": "2010-01-01",
			}},
		},
	}

	doc := BundleToCanonicalDoc(bundle)
	entries := entriesFor(t, doc, "allergiesAndIntolerances")
	if len(entries) != 1 {
		t.Fatalf("expected 1 allergy entry, got %d", len(entries))
	}
	e := entries[0]

	assertEq(t, e, "medicationAllergyCode", "7980")
	assertEq(t, e, "medicationAllergyCodeDisplay", "Penicillin")
	assertEq(t, e, "medicationAllergyCodeSystem", "2.16.840.1.113883.6.88") // RxNorm OID translation
	assertEq(t, e, "status", "Active")                                     // value-translated via allergyStatusDisplay
	assertEq(t, e, "reaction", "247472004")
	assertEq(t, e, "reactionDisplay", "Hives")
	assertEq(t, e, "severity", "moderate")
	assertEq(t, e, "onsetDate", "2010-01-01")
}

// TestBundleToCanonicalDoc_Condition verifies kindCodeValue with a nil
// ValueMap passes the raw code through unchanged (Condition.status has no
// display translation, unlike Allergy's).
func TestBundleToCanonicalDoc_Condition(t *testing.T) {
	bundle := map[string]interface{}{
		"entry": []interface{}{
			map[string]interface{}{"resource": map[string]interface{}{
				"resourceType": "Condition",
				"code": map[string]interface{}{
					"coding": []interface{}{map[string]interface{}{"code": "44054006", "display": "Diabetes"}},
				},
				"clinicalStatus": map[string]interface{}{
					"coding": []interface{}{map[string]interface{}{"code": "active"}},
				},
				"onsetDateTime":     "2019-06-01",
				"abatementDateTime": "2020-01-01",
			}},
		},
	}

	doc := BundleToCanonicalDoc(bundle)
	entries := entriesFor(t, doc, "problems")
	e := entries[0]

	assertEq(t, e, "conditionCode", "44054006")
	assertEq(t, e, "status", "active") // raw code, no display translation
	assertEq(t, e, "onsetDate", "2019-06-01")
	assertEq(t, e, "resolutionDate", "2020-01-01")
}

// TestBundleToCanonicalDoc_ObservationVariants verifies the three
// resolved-section mapping tables each write only their own section's real
// fields — the deliberate cleanup vs. the old behavior of writing
// vitalCode+testCode+observationCode simultaneously "just in case".
func TestBundleToCanonicalDoc_ObservationVariants(t *testing.T) {
	cases := []struct {
		category    string
		sectionKey  string
		wantCodeKey string
	}{
		{"vital-signs", "vitalSigns", "vitalCode"},
		{"laboratory", "results", "testCode"},
		{"social-history", "socialHistory", "observationCode"},
	}

	for _, c := range cases {
		bundle := map[string]interface{}{
			"entry": []interface{}{
				map[string]interface{}{"resource": map[string]interface{}{
					"resourceType": "Observation",
					"category": []interface{}{
						map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": c.category}}},
					},
					"code": map[string]interface{}{
						"coding": []interface{}{map[string]interface{}{"code": "8480-6", "display": "Systolic BP"}},
					},
					"valueQuantity":     map[string]interface{}{"value": 120.0, "unit": "mm[Hg]"},
					"status":            "final",
					"effectiveDateTime": "2024-01-01",
				}},
			},
		}

		doc := BundleToCanonicalDoc(bundle)
		entries := entriesFor(t, doc, c.sectionKey)
		if len(entries) != 1 {
			t.Fatalf("category %q: expected 1 entry in section %q, got %d", c.category, c.sectionKey, len(entries))
		}
		e := entries[0]
		assertEq(t, e, c.wantCodeKey, "8480-6")

		// Only vitalSigns/results carry a quantity value; socialHistory does not.
		if c.sectionKey != "socialHistory" {
			valueKey := "value"
			if c.sectionKey == "results" {
				valueKey = "resultValue"
			}
			assertEq(t, e, valueKey, "120")
		}

		// The OTHER two code keys must NOT appear on this entry — this is the
		// exact duplication the refactor removed.
		for _, other := range []string{"vitalCode", "testCode", "observationCode"} {
			if other == c.wantCodeKey {
				continue
			}
			if _, present := e[other]; present {
				t.Errorf("category %q: unexpected key %q present (should only carry %q)", c.category, other, c.wantCodeKey)
			}
		}
	}
}

func entriesFor(t *testing.T, doc map[string]interface{}, sectionKey string) []map[string]interface{} {
	t.Helper()
	sections, ok := doc["sections"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected doc[\"sections\"] to be a map")
	}
	sec, ok := sections[sectionKey].(map[string]interface{})
	if !ok {
		t.Fatalf("expected section %q to be present", sectionKey)
	}
	rawEntries, _ := sec["entries"].([]interface{})
	entries := make([]map[string]interface{}, 0, len(rawEntries))
	for _, re := range rawEntries {
		if m, ok := re.(map[string]interface{}); ok {
			entries = append(entries, m)
		}
	}
	return entries
}

func assertEq(t *testing.T, e map[string]interface{}, key, want string) {
	t.Helper()
	got, _ := e[key].(string)
	if got != want {
		t.Errorf("%s: got %q, want %q", key, got, want)
	}
}
