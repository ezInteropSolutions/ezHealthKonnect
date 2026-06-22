package executors

import "testing"

// ===============================================================
// PHASE 4: CDA "document.*" PATH RESOLUTION TESTS
// ===============================================================

// negatedAllergyDocumentData mirrors the JSON shape ParsedJSON["document"] takes
// once cda_parser_service.go's typed CDADocument round-trips through
// encoding/json (see services/parsers/cda/cda_parser_service_test.go for the
// real end-to-end version of this fixture).
func negatedAllergyDocumentData() map[string]interface{} {
	return map[string]interface{}{
		"document": map[string]interface{}{
			"sectionsByKey": map[string]interface{}{
				"allergiesAndIntolerances": map[string]interface{}{
					"entries": []interface{}{
						map[string]interface{}{
							"entryType":  "act",
							"statusCode": "active",
							"entryRelationships": []interface{}{
								map[string]interface{}{
									"typeCode": "SUBJ",
									"entry": map[string]interface{}{
										"entryType":   "observation",
										"negationInd": true,
										"moodCode":    "EVN",
									},
								},
							},
						},
					},
				},
			},
		},
		// The curated short-path representation still exists alongside
		// "document" — proves Phase 4 doesn't disturb it.
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"medicationAllergyCode": "419199007"},
				},
			},
		},
	}
}

func TestIsCDADocumentPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"document.sectionsByKey.allergiesAndIntolerances", true},
		{"document.header.patient.gender", true},
		{"allergiesAndIntolerances.medicationAllergyCode", false}, // PathTypeCDA short-path
		{"PID.5", false},
		{"data.items[0].name", false},
	}
	for _, tc := range cases {
		if got := IsCDADocumentPath(tc.path); got != tc.want {
			t.Errorf("IsCDADocumentPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDetectPathType_CDADocument(t *testing.T) {
	if got := DetectPathType("document.sectionsByKey.medications"); got != PathTypeCDADocument {
		t.Errorf("DetectPathType = %v, want %v", got, PathTypeCDADocument)
	}
}

func TestGetFieldValue_CDADocumentPath_ResolvesNestedNegationInd(t *testing.T) {
	data := negatedAllergyDocumentData()
	path := "document.sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[0].entry.negationInd"

	got := GetFieldValue(data, path)
	negated, ok := got.(bool)
	if !ok || !negated {
		t.Fatalf("GetFieldValue(%q) = %v, want true — this is exactly the field the "+
			"curated sections.* short-path can never carry", path, got)
	}
}

func TestGetFieldValue_CDADocumentPath_MissingDocumentKey_ReturnsNilNotPanic(t *testing.T) {
	data := map[string]interface{}{"sections": map[string]interface{}{}}
	got := GetFieldValue(data, "document.sectionsByKey.medications.entries[0].drugCode")
	if got != nil {
		t.Errorf("expected nil when \"document\" key is absent, got %v", got)
	}
}

func TestGetFieldValue_CDADocumentPath_LiveGoStruct_ReturnsNilNotPanic(t *testing.T) {
	// When "document" holds the live *cdadocument.CDADocument Go object (the
	// in-process handoff used only by cda.to_fhir) rather than a JSON-round-
	// tripped map, resolution is a documented no-op, not a panic.
	data := map[string]interface{}{"document": struct{ Foo string }{Foo: "bar"}}
	got := GetFieldValue(data, "document.header.patient.gender")
	if got != nil {
		t.Errorf("expected nil for a non-map document value, got %v", got)
	}
}

func TestUpdateFieldValue_CDADocumentPath_SetsNestedValue(t *testing.T) {
	data := negatedAllergyDocumentData()
	path := "document.sectionsByKey.allergiesAndIntolerances.entries[0].statusCode"

	if ok := UpdateFieldValue(data, path, "completed"); !ok {
		t.Fatalf("UpdateFieldValue(%q) returned false", path)
	}
	got := GetFieldValue(data, path)
	if got != "completed" {
		t.Errorf("GetFieldValue after update = %v, want %q", got, "completed")
	}
}

func TestGetFieldValue_ExistingCDAShortPath_StillResolves(t *testing.T) {
	// Regression: the pre-existing sections.*-driven short-path (PathTypeCDA)
	// must keep working exactly as before — Phase 4 only adds a new path
	// form, it doesn't change this one.
	data := negatedAllergyDocumentData()
	got := GetFieldValue(data, "allergiesAndIntolerances.medicationAllergyCode")
	if got != "419199007" {
		t.Errorf("GetFieldValue(short-path) = %v, want %q", got, "419199007")
	}
}
