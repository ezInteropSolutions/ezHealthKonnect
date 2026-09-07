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

// ===============================================================
// PREDICATE BRACKET ("[field=value]") PATH RESOLUTION TESTS
//
// Added for EDI Phase 5's declarative 835->FHIR mapping: selecting one X12
// NM1 segment occurrence (patient vs. rendering provider vs. payer, all
// sharing the same array key) by its own entityIdentifierCode value, with no
// script and no repeating-group step — a plain fhir.build field sourcePath.
// ===============================================================

func nm1PredicateFixtureData() map[string]interface{} {
	return map[string]interface{}{
		"loops": map[string]interface{}{
			"2100": []interface{}{
				map[string]interface{}{
					"NM1": []interface{}{
						map[string]interface{}{
							"entityIdentifierCode":         "QC",
							"nameLastOrOrganizationName":   "Doe",
							"nameFirst":                    "Jane",
						},
						map[string]interface{}{
							"entityIdentifierCode":       "82",
							"nameLastOrOrganizationName": "Smith Clinic",
						},
					},
				},
			},
		},
	}
}

func TestParseJSONPath_PredicateBracket_ParsesFieldAndValue(t *testing.T) {
	parts := parseJSONPath("loops.2100[0].NM1[entityIdentifierCode=82].nameLastOrOrganizationName")

	var found bool
	for _, p := range parts {
		if p.isPredicate {
			found = true
			if p.predicateField != "entityIdentifierCode" || p.predicateValue != "82" {
				t.Errorf("predicate part = {field:%q value:%q}, want {field:\"entityIdentifierCode\" value:\"82\"}",
					p.predicateField, p.predicateValue)
			}
		}
	}
	if !found {
		t.Fatalf("expected a predicate part in %+v", parts)
	}
}

func TestGetFieldValue_PredicateBracket_SelectsMatchingArrayElement(t *testing.T) {
	data := nm1PredicateFixtureData()

	got := GetFieldValue(data, "loops.2100[0].NM1[entityIdentifierCode=82].nameLastOrOrganizationName")
	if got != "Smith Clinic" {
		t.Errorf("GetFieldValue(rendering provider name) = %v, want %q", got, "Smith Clinic")
	}

	got = GetFieldValue(data, "loops.2100[0].NM1[entityIdentifierCode=QC].nameFirst")
	if got != "Jane" {
		t.Errorf("GetFieldValue(patient first name) = %v, want %q", got, "Jane")
	}
}

func TestGetFieldValue_PredicateBracket_NoMatch_ReturnsNilNotPanic(t *testing.T) {
	data := nm1PredicateFixtureData()
	got := GetFieldValue(data, "loops.2100[0].NM1[entityIdentifierCode=ZZ].nameFirst")
	if got != nil {
		t.Errorf("expected nil for a predicate with no match, got %v", got)
	}
}

func TestGetFieldValue_PredicateBracket_NotAnArray_ReturnsNilNotPanic(t *testing.T) {
	data := map[string]interface{}{"CLP": map[string]interface{}{"claimStatusCode": "1"}}
	got := GetFieldValue(data, "CLP[claimStatusCode=1].foo")
	if got != nil {
		t.Errorf("expected nil when the predicate target isn't an array, got %v", got)
	}
}

// ===============================================================
// WILDCARD BRACKET ("[*]") FLATTEN PATH RESOLUTION TESTS
//
// Added for EDI Phase 5: an X12 CAS occurrence array where each occurrence
// carries its own "adjustments" trio sub-array (edi/loop_engine.go's own
// X12RepeatDef output shape) needs to become ONE flat row list for a
// fhir.build repeatingGroup's RowsPath — "CAS[*].adjustments" — with no
// script and no second nesting level of repeatingGroups.
// ===============================================================

func casAdjustmentsFixtureData() map[string]interface{} {
	return map[string]interface{}{
		"CAS": []interface{}{
			map[string]interface{}{
				"claimAdjustmentGroupCode": "CO",
				"adjustments": []interface{}{
					map[string]interface{}{"reasonCode": "45", "amount": "10.00"},
					map[string]interface{}{"reasonCode": "97", "amount": "5.00"},
				},
			},
			map[string]interface{}{
				"claimAdjustmentGroupCode": "PR",
				"adjustments": []interface{}{
					map[string]interface{}{"reasonCode": "1", "amount": "20.00"},
				},
			},
		},
	}
}

func TestGetFieldValue_WildcardBracket_FlattensNestedArrays(t *testing.T) {
	data := casAdjustmentsFixtureData()

	got := GetFieldValue(data, "CAS[*].adjustments")
	arr, ok := got.([]interface{})
	if !ok {
		t.Fatalf("GetFieldValue(wildcard flatten) = %T(%v), want []interface{}", got, got)
	}
	if len(arr) != 3 {
		t.Fatalf("flattened CAS[*].adjustments has %d entries, want 3 (2 from the first CAS occurrence + 1 from the second)", len(arr))
	}
	first, ok := arr[0].(map[string]interface{})
	if !ok || first["reasonCode"] != "45" {
		t.Errorf("arr[0] = %v, want the first CAS occurrence's first trio (reasonCode 45)", arr[0])
	}
	third, ok := arr[2].(map[string]interface{})
	if !ok || third["reasonCode"] != "1" {
		t.Errorf("arr[2] = %v, want the second CAS occurrence's own trio (reasonCode 1)", arr[2])
	}
}

func TestGetFieldValue_WildcardBracket_SkipsElementsMissingTheSuffixField(t *testing.T) {
	// A CAS occurrence with no adjustments (situational, may be entirely
	// absent) contributes zero rows rather than an error or a nil entry.
	data := map[string]interface{}{
		"CAS": []interface{}{
			map[string]interface{}{"claimAdjustmentGroupCode": "CO"}, // no "adjustments" key at all
			map[string]interface{}{
				"claimAdjustmentGroupCode": "PR",
				"adjustments":              []interface{}{map[string]interface{}{"reasonCode": "2", "amount": "3.00"}},
			},
		},
	}
	got := GetFieldValue(data, "CAS[*].adjustments")
	arr, ok := got.([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("GetFieldValue(wildcard flatten, one empty occurrence) = %v, want a single-element array", got)
	}
}

func TestGetFieldValue_WildcardBracket_NotAnArray_ReturnsNilNotPanic(t *testing.T) {
	data := map[string]interface{}{"CAS": map[string]interface{}{"claimAdjustmentGroupCode": "CO"}}
	got := GetFieldValue(data, "CAS[*].adjustments")
	if got != nil {
		t.Errorf("expected nil when the wildcard target isn't an array, got %v", got)
	}
}

// ===============================================================
// []map[string]interface{} ARRAY-SHAPE TESTS
//
// edi/loop_engine.go's own parser builds repeating loops/segments as native
// []map[string]interface{} (matchSegmentSequence's own
// "arr, _ := out[segID].([]map[string]interface{})" pattern) — NOT
// []interface{}, the shape every array branch here originally assumed.
// Pipeline steps pass data between each other as live Go values with no
// JSON round-trip in between (services/transformation_pipeline_service.go
// passes a shallow copy of the same in-memory message map to each step's
// Execute call) — a []interface{}-only resolver silently returned nil for
// every EDI-parsed array a real pipeline run, caught by EDI Phase 5's own
// real-sample integration test (services/executors/transform/
// edi_835_to_fhir_test.go) rather than left latent. asInterfaceSlice fixes
// this for all three array-handling branches (numeric index, predicate,
// wildcard) — these tests pin that fix down explicitly.
// ===============================================================

func TestGetFieldValue_NumericIndex_AcceptsNativeMapSlice(t *testing.T) {
	data := map[string]interface{}{
		"CLP": []map[string]interface{}{
			{"patientControlNumber": "111"},
			{"patientControlNumber": "222"},
		},
	}
	got := GetFieldValue(data, "CLP[1].patientControlNumber")
	if got != "222" {
		t.Errorf("GetFieldValue([]map[string]interface{} numeric index) = %v, want 222", got)
	}
}

func TestGetFieldValue_PredicateBracket_AcceptsNativeMapSlice(t *testing.T) {
	data := map[string]interface{}{
		"NM1": []map[string]interface{}{
			{"entityIdentifierCode": "QC", "nameLastOrOrganizationName": "Doe"},
			{"entityIdentifierCode": "82", "nameLastOrOrganizationName": "Smith Clinic"},
		},
	}
	got := GetFieldValue(data, "NM1[entityIdentifierCode=82].nameLastOrOrganizationName")
	if got != "Smith Clinic" {
		t.Errorf("GetFieldValue([]map[string]interface{} predicate) = %v, want Smith Clinic", got)
	}
}

func TestGetFieldValue_WildcardBracket_AcceptsNativeMapSlice(t *testing.T) {
	data := map[string]interface{}{
		"CAS": []map[string]interface{}{
			{"claimAdjustmentGroupCode": "CO", "adjustments": []map[string]interface{}{
				{"reasonCode": "45", "amount": "10.00"},
			}},
			{"claimAdjustmentGroupCode": "PR", "adjustments": []map[string]interface{}{
				{"reasonCode": "1", "amount": "20.00"},
			}},
		},
	}
	got := GetFieldValue(data, "CAS[*].adjustments")
	arr, ok := got.([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("GetFieldValue([]map[string]interface{} wildcard flatten) = %v, want a 2-element []interface{}", got)
	}
	first := arr[0].(map[string]interface{})
	if first["reasonCode"] != "45" {
		t.Errorf("arr[0].reasonCode = %v, want 45", first["reasonCode"])
	}
}

func TestResolveRows_WildcardBracket_UsableAsRepeatingGroupSource(t *testing.T) {
	// resolveRows (services/executors/transform, shared by fhir.build and
	// map_to_canonical) delegates entirely to GetFieldValue -- confirms the
	// wildcard flatten is available to repeatingGroups' RowsPath with no
	// changes needed in that package at all.
	data := casAdjustmentsFixtureData()
	got := GetFieldValue(data, "CAS[*].adjustments")
	arr, ok := got.([]interface{})
	if !ok || len(arr) != 3 {
		t.Fatalf("expected the same 3-element flattened array a repeatingGroup RowsPath would consume, got %v", got)
	}
}

func TestParseJSONPath_NumericIndex_StillWorks(t *testing.T) {
	// Regression: adding predicate-bracket support must not disturb the
	// pre-existing numeric "[N]" index form.
	parts := parseJSONPath("data.items[0].name")
	if len(parts) != 4 || !parts[2].isArray || parts[2].index != 0 {
		t.Fatalf("parseJSONPath(numeric index) = %+v, want a numeric isArray part at index 2", parts)
	}
}
