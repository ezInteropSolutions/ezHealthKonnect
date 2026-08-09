// services/executors/transform/map_to_canonical_executor_test.go
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/models"
)

func runMapToCanonical(t *testing.T, config map[string]interface{}, inputData map[string]interface{}) map[string]interface{} {
	t.Helper()
	executor := NewMapToCanonicalExecutor()
	step := &models.TransformationStep{
		StepName: "Test Map to Canonical",
		StepType: "cda.map_to_canonical",
		Enabled:  true,
		Config:   config,
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return output
}

func canonicalDocFrom(t *testing.T, output map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	doc, ok := output[field].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output[%q] to be a map, got %T", field, output[field])
	}
	return doc
}

func sectionEntries(t *testing.T, doc map[string]interface{}, sectionKey string) []map[string]interface{} {
	t.Helper()
	sections, ok := doc["sections"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected doc[\"sections\"] to be a map")
	}
	sec, ok := sections[sectionKey].(map[string]interface{})
	if !ok {
		t.Fatalf("expected section %q to be present, sections=%v", sectionKey, sections)
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

// TestMapToCanonical_CSVLikeRows_FileParserRecordsField simulates
// file_parser_executor.go's fixed "records" output field — the CSV-to-CCD
// case this step exists for.
func TestMapToCanonical_CSVLikeRows_FileParserRecordsField(t *testing.T) {
	config := map[string]interface{}{
		"outputField": "parsedCDA",
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "problems",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "conditionCode", "sourcePath": "icd10Code"},
					map[string]interface{}{"canonicalField": "conditionCodeDisplay", "sourcePath": "description"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"icd10Code": "E11.9", "description": "Type 2 diabetes mellitus"},
			map[string]interface{}{"icd10Code": "I10", "description": "Essential hypertension"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "problems")

	if len(entries) != 2 {
		t.Fatalf("expected 2 problem entries, got %d", len(entries))
	}
	if got := entries[0]["conditionCode"]; got != "E11.9" {
		t.Errorf("entries[0].conditionCode = %v, want E11.9", got)
	}
	if got := entries[0]["conditionCodeDisplay"]; got != "Type 2 diabetes mellitus" {
		t.Errorf("entries[0].conditionCodeDisplay = %v, want Type 2 diabetes mellitus", got)
	}
	if got := entries[1]["conditionCode"]; got != "I10" {
		t.Errorf("entries[1].conditionCode = %v, want I10", got)
	}
}

// TestMapToCanonical_DBLikeRows_RowsFieldWithValueMap simulates
// database_enrichment_executor.go's fixed "rows" output field, plus a
// ValueMap translation — the "CSV/DB status column doesn't match the
// canonical vocabulary" case.
func TestMapToCanonical_DBLikeRows_RowsFieldWithValueMap(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "allergiesAndIntolerances",
				"rowsPath":   "rows",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "medicationAllergyCode", "sourcePath": "allergen_code"},
					map[string]interface{}{
						"canonicalField": "status",
						"sourcePath":     "status_flag",
						"valueMap":       map[string]interface{}{"A": "active", "R": "resolved"},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"rows": []interface{}{
			map[string]interface{}{"allergen_code": "7980", "status_flag": "A"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "allergiesAndIntolerances")

	if len(entries) != 1 {
		t.Fatalf("expected 1 allergy entry, got %d", len(entries))
	}
	if got := entries[0]["medicationAllergyCode"]; got != "7980" {
		t.Errorf("medicationAllergyCode = %v, want 7980", got)
	}
	if got := entries[0]["status"]; got != "active" {
		t.Errorf("status = %v, want active (translated from A via valueMap)", got)
	}
}

// TestMapToCanonical_Transform_DateReformatting verifies a field mapping's
// Transform normalizes a raw CSV-style date into CDA's TS format before
// being written — the gap ValueMap alone couldn't cover.
func TestMapToCanonical_Transform_DateReformatting(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "problems",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "onsetDate",
						"sourcePath":     "onset",
						"transform":      "date_to_cda",
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"onset": "01/15/2024"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "problems")

	if got := entries[0]["onsetDate"]; got != "20240115" {
		t.Errorf("onsetDate = %v, want 20240115 (transformed from 01/15/2024)", got)
	}
}

// TestMapToCanonical_TransformThenValueMap_OrderingLocked verifies Transform
// runs BEFORE ValueMap: a "trim" transform normalizes whitespace, then
// ValueMap translates the trimmed (not raw) result. If the order were
// reversed, the ValueMap lookup below would miss (its key has no
// surrounding whitespace) and the raw untrimmed value would be written
// instead.
func TestMapToCanonical_TransformThenValueMap_OrderingLocked(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "allergiesAndIntolerances",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "status",
						"sourcePath":     "status_flag",
						"transform":      "trim",
						"valueMap":       map[string]interface{}{"A": "active"},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"status_flag": "  A  "},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "allergiesAndIntolerances")

	if got := entries[0]["status"]; got != "active" {
		t.Errorf("status = %v, want active (trim ran before valueMap lookup)", got)
	}
}

// TestMapToCanonical_HeaderField_TransformApplied verifies header field
// mappings also honor Transform, not just section fields.
func TestMapToCanonical_HeaderField_TransformApplied(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{
				"group":      "patient",
				"target":     "dateOfBirth",
				"sourcePath": "dob",
				"transform":  "date_to_cda",
			},
		},
	}
	inputData := map[string]interface{}{
		"dob": "1980-05-20",
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header := doc["header"].(map[string]interface{})
	patient := header["patient"].(map[string]interface{})

	if got := patient["dateOfBirth"]; got != "19800520" {
		t.Errorf("dateOfBirth = %v, want 19800520", got)
	}
}

// TestMapToCanonical_HeaderField_LiteralValue_WrittenWhenSourcePathEmpty
// verifies headerFieldRow.LiteralValue — a real gap found in Round 23: every
// SECTION field already supported a fixed literalValue (no SourcePath
// needed), but header fields had no equivalent at all (applyHeaderField's
// old `if ... || h.SourcePath == "" { return }` guard made a
// SourcePath-empty row a silent no-op regardless of any other field). Needed
// for a header coded field's own fixed companion codeSystem (e.g. patient
// languageCommunication/proficiencyLevelCode's HL7 LanguageAbilityProficiency
// OID), the same "codeSystem is just another mappable field" convention
// every section already has.
func TestMapToCanonical_HeaderField_LiteralValue_WrittenWhenSourcePathEmpty(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{
				"group":        "patient",
				"target":       "languageProficiencySystem",
				"sourcePath":   "",
				"literalValue": "2.16.840.1.113883.5.61",
			},
		},
	}

	output := runMapToCanonical(t, config, map[string]interface{}{})
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header := doc["header"].(map[string]interface{})
	patient := header["patient"].(map[string]interface{})

	if got := patient["languageProficiencySystem"]; got != "2.16.840.1.113883.5.61" {
		t.Errorf("languageProficiencySystem = %v, want 2.16.840.1.113883.5.61 (LiteralValue should be written even with no SourcePath/inputData)", got)
	}
}

// TestMapToCanonical_HeaderField_NoSourcePathNoLiteral_NoOp guards the
// opposite edge: a row with neither SourcePath nor LiteralValue must stay a
// silent no-op (matching pre-existing behavior for every already-saved
// pipeline config with a stray blank header row), not somehow write an
// empty string.
func TestMapToCanonical_HeaderField_NoSourcePathNoLiteral_NoOp(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{"group": "patient", "target": "sex", "sourcePath": ""},
		},
	}

	output := runMapToCanonical(t, config, map[string]interface{}{})
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header, _ := doc["header"].(map[string]interface{})
	if patient, ok := header["patient"].(map[string]interface{}); ok {
		if _, present := patient["sex"]; present {
			t.Errorf("expected no \"sex\" key written when both SourcePath and LiteralValue are empty, got: %v", patient)
		}
	}
}

// TestMapToCanonical_FallbackPaths_SecondPathUsedWhenFirstAbsent verifies
// FallbackPaths tries each path in order until one resolves — a source
// system with inconsistent column naming across rows.
func TestMapToCanonical_FallbackPaths_SecondPathUsedWhenFirstAbsent(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "medications",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "drugCode",
						"sourcePath":     "ndcCode",
						"fallbackPaths":  []interface{}{"rxnormCode"},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"rxnormCode": "310965"}, // no ndcCode on this row
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "medications")

	if len(entries) != 1 {
		t.Fatalf("expected 1 medication entry, got %d", len(entries))
	}
	if got := entries[0]["drugCode"]; got != "310965" {
		t.Errorf("drugCode = %v, want 310965 (from fallbackPaths)", got)
	}
}

// TestMapToCanonical_LiteralValue_UsedWhenNoPathResolves verifies a field
// with no matching data anywhere still gets a fixed LiteralValue.
func TestMapToCanonical_LiteralValue_UsedWhenNoPathResolves(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "immunizations",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vaccineCode", "sourcePath": "code"},
					map[string]interface{}{"canonicalField": "status", "sourcePath": "missingPath", "literalValue": "completed"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"code": "88"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "immunizations")

	if got := entries[0]["status"]; got != "completed" {
		t.Errorf("status = %v, want completed (from literalValue)", got)
	}
}

// TestMapToCanonical_RowWithNoMappedFields_Omitted verifies a row that
// produces zero mapped fields (every source path empty/absent, no
// fallback, no literal) is skipped entirely, not emitted as an empty entry.
func TestMapToCanonical_RowWithNoMappedFields_Omitted(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "procedures",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "procedureCode", "sourcePath": "code"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"code": "12345"},
			map[string]interface{}{"unrelatedField": "x"}, // no "code" -> should be skipped
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "procedures")

	if len(entries) != 1 {
		t.Fatalf("expected 1 procedure entry (empty row skipped), got %d", len(entries))
	}
}

// TestMapToCanonical_HeaderPatientAndAuthor verifies flat scalar header
// mapping, including the "address." nesting prefix
// (see cda/builder/canonical_field_catalog.go's prefixKeys).
func TestMapToCanonical_HeaderPatientAndAuthor(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{"group": "patient", "target": "firstName", "sourcePath": "patientFirstName"},
			map[string]interface{}{"group": "patient", "target": "lastName", "sourcePath": "patientLastName"},
			map[string]interface{}{"group": "patient", "target": "address.street", "sourcePath": "patientStreet"},
			map[string]interface{}{"group": "author", "target": "given", "sourcePath": "providerFirstName"},
		},
	}
	inputData := map[string]interface{}{
		"patientFirstName":  "Jane",
		"patientLastName":   "Doe",
		"patientStreet":     "123 Main St",
		"providerFirstName": "John",
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header, ok := doc["header"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected doc[\"header\"] to be a map")
	}
	patient, ok := header["patient"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header[\"patient\"] to be a map")
	}
	if got := patient["firstName"]; got != "Jane" {
		t.Errorf("patient.firstName = %v, want Jane", got)
	}
	if got := patient["lastName"]; got != "Doe" {
		t.Errorf("patient.lastName = %v, want Doe", got)
	}
	address, ok := patient["address"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header.patient.address to be a nested map, got %T", patient["address"])
	}
	if got := address["street"]; got != "123 Main St" {
		t.Errorf("patient.address.street = %v, want 123 Main St", got)
	}
	author, ok := header["author"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header[\"author\"] to be a map")
	}
	if got := author["given"]; got != "John" {
		t.Errorf("author.given = %v, want John", got)
	}
}

// TestMapToCanonical_HeaderField_IdsArrayPassthrough verifies the one
// repeating-header-field exception: target "ids" whose SourcePath already
// resolves to a []{root,extension} array is passed through untransformed
// (patientRole's identifiers — e.g. MRN + SSN as two entries) rather than
// silently dropped by stringifyValue's scalar-only type switch.
func TestMapToCanonical_HeaderField_IdsArrayPassthrough(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{"group": "patient", "target": "ids", "sourcePath": "patientIdentifiers"},
		},
	}
	inputData := map[string]interface{}{
		"patientIdentifiers": []interface{}{
			map[string]interface{}{"root": "2.16.840.1.113883.19.5", "extension": "MRN-00482"},
			map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "521-08-4013"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header := doc["header"].(map[string]interface{})
	patient := header["patient"].(map[string]interface{})

	ids, ok := patient["ids"].([]interface{})
	if !ok || len(ids) != 2 {
		t.Fatalf("expected header.patient.ids to be a 2-item array, got %T: %v", patient["ids"], patient["ids"])
	}
	first := ids[0].(map[string]interface{})
	if first["root"] != "2.16.840.1.113883.19.5" || first["extension"] != "MRN-00482" {
		t.Errorf("ids[0] = %v, want MRN entry", first)
	}
	second := ids[1].(map[string]interface{})
	if second["root"] != "2.16.840.1.113883.4.1" || second["extension"] != "521-08-4013" {
		t.Errorf("ids[1] = %v, want SSN entry", second)
	}
}

// TestMapToCanonical_HeaderField_EmptyArraySourcePath_NoOp verifies an
// empty array source (e.g. a row present but with no identifiers) writes
// nothing, rather than an empty ids array that would still satisfy the
// completeness banner's "row exists" check incorrectly.
func TestMapToCanonical_HeaderField_EmptyArraySourcePath_NoOp(t *testing.T) {
	config := map[string]interface{}{
		"header": []interface{}{
			map[string]interface{}{"group": "patient", "target": "ids", "sourcePath": "patientIdentifiers"},
		},
	}
	inputData := map[string]interface{}{
		"patientIdentifiers": []interface{}{},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	header := doc["header"].(map[string]interface{})
	patient, _ := header["patient"].(map[string]interface{})
	if _, exists := patient["ids"]; exists {
		t.Errorf("expected no ids key written for an empty source array, got %v", patient["ids"])
	}
}

// TestMapToCanonical_EmptyConfig_ProducesEmptyCanonicalDoc verifies a step
// with no header/sections configured is a safe no-op, matching cda.build/
// cda.to_fhir's own permissive Validate.
func TestMapToCanonical_EmptyConfig_ProducesEmptyCanonicalDoc(t *testing.T) {
	output := runMapToCanonical(t, map[string]interface{}{}, map[string]interface{}{})
	doc := canonicalDocFrom(t, output, "parsedCDA")
	sections, ok := doc["sections"].(map[string]interface{})
	if !ok || len(sections) != 0 {
		t.Errorf("expected empty sections map, got %v", doc["sections"])
	}
}

// TestMapToCanonical_Condition_MatchWritesThenLiteralValue verifies a field
// with a Condition whose WhenPath/Equals matches the row writes
// ThenLiteralValue (through ThenTransform + ValueMap) instead of resolving
// SourcePath/FallbackPaths/LiteralValue normally.
func TestMapToCanonical_Condition_MatchWritesThenLiteralValue(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "allergiesAndIntolerances",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "negationInd",
						"sourcePath":     "shouldNeverBeUsed", // present but must be ignored — Condition wins
						"condition": map[string]interface{}{
							"whenPath":         "status_flag",
							"equals":           "not-observed",
							"thenLiteralValue": "true",
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"status_flag": "not-observed", "shouldNeverBeUsed": "WRONG"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "allergiesAndIntolerances")

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if got := entries[0]["negationInd"]; got != "true" {
		t.Errorf("negationInd = %v, want true (from Condition.ThenLiteralValue, not SourcePath)", got)
	}
}

// TestMapToCanonical_Condition_NoMatchFallsThroughToNormalResolution verifies
// a Condition whose WhenPath/Equals does NOT match falls through to the
// field's normal SourcePath resolution — Condition is a branch, not a filter.
func TestMapToCanonical_Condition_NoMatchFallsThroughToNormalResolution(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "allergiesAndIntolerances",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "negationInd",
						"sourcePath":     "explicitFlag",
						"condition": map[string]interface{}{
							"whenPath":         "status_flag",
							"equals":           "not-observed",
							"thenLiteralValue": "true",
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"status_flag": "active", "explicitFlag": "false"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "allergiesAndIntolerances")

	if got := entries[0]["negationInd"]; got != "false" {
		t.Errorf("negationInd = %v, want false (normal SourcePath resolution, Condition didn't match)", got)
	}
}

// TestMapToCanonical_Condition_MatchWithEmptyThenLiteralValue_WritesNothing
// verifies the documented "write nothing" branch: a matched Condition with no
// ThenLiteralValue must NOT fall through to SourcePath either — the field is
// simply absent from the entry.
func TestMapToCanonical_Condition_MatchWithEmptyThenLiteralValue_WritesNothing(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "allergiesAndIntolerances",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "medicationAllergyCode", "sourcePath": "code"},
					map[string]interface{}{
						"canonicalField": "negationInd",
						"sourcePath":     "explicitFlag",
						"condition": map[string]interface{}{
							"whenPath": "status_flag",
							"equals":   "not-observed",
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"code": "7980", "status_flag": "not-observed", "explicitFlag": "true"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "allergiesAndIntolerances")

	if _, present := entries[0]["negationInd"]; present {
		t.Errorf("expected negationInd to be absent (matched Condition with empty ThenLiteralValue), got %v", entries[0]["negationInd"])
	}
	if got := entries[0]["medicationAllergyCode"]; got != "7980" {
		t.Errorf("medicationAllergyCode = %v, want 7980 (sibling field unaffected)", got)
	}
}

// TestMapToCanonical_GroupBy_SingleColumn_BucketsRowsIntoSharedEntries
// verifies rows sharing one GroupBy column's value are bucketed into ONE
// canonical entry's GroupedItemsKey array, in first-seen bucket order — the
// core Vital Signs "one organizer, N components" no-code producer.
func TestMapToCanonical_GroupBy_SingleColumn_BucketsRowsIntoSharedEntries(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey":      "vitalSigns",
				"rowsPath":        "records",
				"groupBy":         []interface{}{"panelId"},
				"groupedItemsKey": "components",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vitalCode", "sourcePath": "loincCode"},
					map[string]interface{}{"canonicalField": "value", "sourcePath": "result"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"panelId": "P1", "loincCode": "8480-6", "result": "120"},
			map[string]interface{}{"panelId": "P1", "loincCode": "8462-4", "result": "80"},
			map[string]interface{}{"panelId": "P2", "loincCode": "8310-5", "result": "37.0"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "vitalSigns")

	if len(entries) != 2 {
		t.Fatalf("expected 2 grouped entries (P1, P2), got %d: %v", len(entries), entries)
	}
	p1Items, ok := entries[0]["components"].([]interface{})
	if !ok || len(p1Items) != 2 {
		t.Fatalf("expected entries[0].components to have 2 items, got %v", entries[0]["components"])
	}
	p2Items, ok := entries[1]["components"].([]interface{})
	if !ok || len(p2Items) != 1 {
		t.Fatalf("expected entries[1].components to have 1 item, got %v", entries[1]["components"])
	}
	first, ok := p1Items[0].(map[string]interface{})
	if !ok || first["vitalCode"] != "8480-6" || first["value"] != "120" {
		t.Errorf("p1Items[0] = %v, want vitalCode=8480-6 value=120", first)
	}
}

// TestMapToCanonical_GroupBy_EntryFields_ResolvedOnceFromFirstRow verifies
// EntryFields are applied ONCE per group (not once per item), sourced from
// the group's first-seen row — the organizer-level effectiveTime producer
// Vital Signs Organizer needs (CONF:1198-7288), distinct from each
// component's own per-item fields.
func TestMapToCanonical_GroupBy_EntryFields_ResolvedOnceFromFirstRow(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey":      "vitalSigns",
				"rowsPath":        "records",
				"groupBy":         []interface{}{"panelId"},
				"groupedItemsKey": "components",
				"entryFields": []interface{}{
					map[string]interface{}{"canonicalField": "effectiveTime", "sourcePath": "panelTime"},
				},
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vitalCode", "sourcePath": "loincCode"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"panelId": "P1", "panelTime": "20240115120000", "loincCode": "8480-6"},
			map[string]interface{}{"panelId": "P1", "panelTime": "20240115120500", "loincCode": "8462-4"}, // later time on 2nd row — first row wins
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "vitalSigns")

	if len(entries) != 1 {
		t.Fatalf("expected 1 grouped entry, got %d", len(entries))
	}
	if got := entries[0]["effectiveTime"]; got != "20240115120000" {
		t.Errorf("effectiveTime = %v, want 20240115120000 (from the group's first row, not the second)", got)
	}
	items, ok := entries[0]["components"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("expected entry-level effectiveTime to coexist with 2 per-component items, got %v", entries[0])
	}
}

// TestMapToCanonical_GroupBy_CompositeKey_TwoColumnsMustBothMatch verifies a
// composite (multi-column) GroupBy only buckets rows together when EVERY
// configured column matches — not just any one of them.
func TestMapToCanonical_GroupBy_CompositeKey_TwoColumnsMustBothMatch(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey":      "vitalSigns",
				"rowsPath":        "records",
				"groupBy":         []interface{}{"encounterId", "panelId"},
				"groupedItemsKey": "components",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vitalCode", "sourcePath": "loincCode"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"encounterId": "E1", "panelId": "P1", "loincCode": "8480-6"},
			map[string]interface{}{"encounterId": "E2", "panelId": "P1", "loincCode": "8462-4"}, // same panelId, different encounter
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "vitalSigns")

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (composite key differs), got %d: %v", len(entries), entries)
	}
}

// TestMapToCanonical_GroupBy_MissingValue_BecomesOwnSingletonGroup verifies a
// row missing one of the configured GroupBy columns is never silently
// dropped — it becomes its own single-item group instead.
func TestMapToCanonical_GroupBy_MissingValue_BecomesOwnSingletonGroup(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey":      "vitalSigns",
				"rowsPath":        "records",
				"groupBy":         []interface{}{"panelId"},
				"groupedItemsKey": "components",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vitalCode", "sourcePath": "loincCode"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"panelId": "P1", "loincCode": "8480-6"},
			map[string]interface{}{"loincCode": "9999-9"}, // no panelId at all
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "vitalSigns")

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (P1 group + one singleton for the row missing panelId), got %d: %v", len(entries), entries)
	}
	foundSingleton := false
	for _, e := range entries {
		items, _ := e["components"].([]interface{})
		if len(items) == 1 {
			if m, ok := items[0].(map[string]interface{}); ok && m["vitalCode"] == "9999-9" {
				foundSingleton = true
			}
		}
	}
	if !foundSingleton {
		t.Errorf("expected the row missing panelId to appear as its own singleton group, entries=%v", entries)
	}
}

// TestMapToCanonical_GroupBy_RowsProducingNoMappedFields_ExcludedFromGroup
// verifies a row that maps to zero fields within its group doesn't produce a
// junk empty item wedged into the bucket's items array.
func TestMapToCanonical_GroupBy_RowsProducingNoMappedFields_ExcludedFromGroup(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey":      "vitalSigns",
				"rowsPath":        "records",
				"groupBy":         []interface{}{"panelId"},
				"groupedItemsKey": "components",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "vitalCode", "sourcePath": "loincCode"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{
			map[string]interface{}{"panelId": "P1", "loincCode": "8480-6"},
			map[string]interface{}{"panelId": "P1", "unrelatedField": "x"}, // maps to nothing
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "vitalSigns")

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	items, _ := entries[0]["components"].([]interface{})
	if len(items) != 1 {
		t.Errorf("expected 1 item in the group (unmapped row excluded), got %d: %v", len(items), items)
	}
}

// TestMapToCanonical_RelatedRows_JoinsMatchingRowsFromDifferentArray is the
// core Option C (cross-table join) proof: Medications' 0..* Indication
// entryRelationship (CONF:1098-7536), joined from a SEPARATE
// "indicationRecords" pipeline array via a shared medicationId column — not
// the same rowset the medication row itself came from.
func TestMapToCanonical_RelatedRows_JoinsMatchingRowsFromDifferentArray(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "medications",
				"rowsPath":   "medicationRecords",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "drugCode", "sourcePath": "ndc"},
					map[string]interface{}{
						"canonicalField": "indications",
						"relatedRows": map[string]interface{}{
							"relatedRowsPath": "indicationRecords",
							"joinLocalKey":    "medId",
							"joinRelatedKey":  "medId",
							"fields": []interface{}{
								map[string]interface{}{"canonicalField": "indicationCode", "sourcePath": "diagnosisCode"},
							},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"medicationRecords": []interface{}{
			map[string]interface{}{"medId": "MED-1", "ndc": "0069-3150-83"},
			map[string]interface{}{"medId": "MED-2", "ndc": "0069-9999-01"},
		},
		"indicationRecords": []interface{}{
			map[string]interface{}{"medId": "MED-1", "diagnosisCode": "44054006"}, // matches MED-1
			map[string]interface{}{"medId": "MED-1", "diagnosisCode": "38341003"}, // also matches MED-1
			map[string]interface{}{"medId": "MED-9", "diagnosisCode": "99999999"}, // matches nothing
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "medications")

	if len(entries) != 2 {
		t.Fatalf("expected 2 medication entries, got %d", len(entries))
	}

	med1 := entries[0]
	if med1["drugCode"] != "0069-3150-83" {
		t.Fatalf("expected entries[0] to be MED-1, got %v", med1)
	}
	indications, ok := med1["indications"].([]interface{})
	if !ok || len(indications) != 2 {
		t.Fatalf("expected MED-1 to have 2 joined indications, got %v", med1["indications"])
	}
	first, _ := indications[0].(map[string]interface{})
	if first["indicationCode"] != "44054006" {
		t.Errorf("indications[0].indicationCode = %v, want 44054006", first["indicationCode"])
	}

	med2 := entries[1]
	if _, present := med2["indications"]; present {
		t.Errorf("expected MED-2 to have NO indications key (zero matches), got %v", med2["indications"])
	}
}

// TestMapToCanonical_RelatedRows_NoLocalKeyValue_NoJoinAttempted verifies a
// row missing its own JoinLocalKey value simply gets no related-rows field
// (not an error, not a join against every related row).
func TestMapToCanonical_RelatedRows_NoLocalKeyValue_NoJoinAttempted(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "medications",
				"rowsPath":   "medicationRecords",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "drugCode", "sourcePath": "ndc"},
					map[string]interface{}{
						"canonicalField": "indications",
						"relatedRows": map[string]interface{}{
							"relatedRowsPath": "indicationRecords",
							"joinLocalKey":    "medId",
							"joinRelatedKey":  "medId",
							"fields": []interface{}{
								map[string]interface{}{"canonicalField": "indicationCode", "sourcePath": "diagnosisCode"},
							},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"medicationRecords": []interface{}{
			map[string]interface{}{"ndc": "0069-3150-83"}, // no medId at all
		},
		"indicationRecords": []interface{}{
			map[string]interface{}{"medId": "MED-1", "diagnosisCode": "44054006"},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "medications")

	if len(entries) != 1 {
		t.Fatalf("expected 1 medication entry, got %d", len(entries))
	}
	if _, present := entries[0]["indications"]; present {
		t.Errorf("expected no indications key when the row has no local join key value, got %v", entries[0]["indications"])
	}
}

// TestMapToCanonical_RelatedRows_JoinedFieldsSupportTransformAndValueMap
// verifies a joined row's own field mapping is the SAME applyFieldMapping
// primitive as everywhere else — Transform/ValueMap keep working inside a
// cross-table join, not just for top-level/grouped fields.
func TestMapToCanonical_RelatedRows_JoinedFieldsSupportTransformAndValueMap(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "medications",
				"rowsPath":   "medicationRecords",
				"fields": []interface{}{
					map[string]interface{}{
						"canonicalField": "indications",
						"relatedRows": map[string]interface{}{
							"relatedRowsPath": "indicationRecords",
							"joinLocalKey":    "medId",
							"joinRelatedKey":  "medId",
							"fields": []interface{}{
								map[string]interface{}{
									"canonicalField": "indicationCode",
									"sourcePath":     "status_flag",
									"transform":      "trim",
									"valueMap":       map[string]interface{}{"A": "active"},
								},
							},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"medicationRecords": []interface{}{
			map[string]interface{}{"medId": "MED-1"},
		},
		"indicationRecords": []interface{}{
			map[string]interface{}{"medId": "MED-1", "status_flag": "  A  "},
		},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	entries := sectionEntries(t, doc, "medications")

	indications, ok := entries[0]["indications"].([]interface{})
	if !ok || len(indications) != 1 {
		t.Fatalf("expected 1 joined indication, got %v", entries[0]["indications"])
	}
	item, _ := indications[0].(map[string]interface{})
	if item["indicationCode"] != "active" {
		t.Errorf("indicationCode = %v, want active (trim then valueMap, same as any other field)", item["indicationCode"])
	}
}

// TestMapToCanonical_CustomOutputField verifies outputField is honored.
func TestMapToCanonical_CustomOutputField(t *testing.T) {
	config := map[string]interface{}{
		"outputField": "customCanonical",
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "problems",
				"rowsPath":   "records",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "conditionCode", "sourcePath": "code"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"records": []interface{}{map[string]interface{}{"code": "E11.9"}},
	}

	output := runMapToCanonical(t, config, inputData)
	if _, present := output["parsedCDA"]; present {
		t.Errorf("expected no default parsedCDA field when outputField is overridden")
	}
	doc := canonicalDocFrom(t, output, "customCanonical")
	entries := sectionEntries(t, doc, "problems")
	if len(entries) != 1 {
		t.Fatalf("expected 1 problem entry, got %d", len(entries))
	}
}

// TestMapToCanonical_EntriesKey_SecondSectionMappingRowMergesNotOverwrites
// verifies the AlternateEntryArchetype counterpart on the map side: two
// sectionMappingRows sharing the same SectionKey but different EntriesKey
// (the ordinary case defaults to "entries") must both land in
// sections.<key>, not have the second overwrite the first — the bug the
// pre-existing `sectionsOut[sm.SectionKey] = map[string]interface{}{...}`
// assignment had (map assignment, not merge).
func TestMapToCanonical_EntriesKey_SecondSectionMappingRowMergesNotOverwrites(t *testing.T) {
	config := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"sectionKey": "medicalEquipment",
				"rowsPath":   "equipmentRecords",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "equipmentCode", "sourcePath": "code"},
				},
			},
			map[string]interface{}{
				"sectionKey": "medicalEquipment",
				"rowsPath":   "procedureRecords",
				"entriesKey": "procedureEntries",
				"fields": []interface{}{
					map[string]interface{}{"canonicalField": "equipmentProcedureCode", "sourcePath": "code"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"equipmentRecords": []interface{}{map[string]interface{}{"code": "360008007"}},
		"procedureRecords": []interface{}{map[string]interface{}{"code": "87717006"}},
	}

	output := runMapToCanonical(t, config, inputData)
	doc := canonicalDocFrom(t, output, "parsedCDA")
	sections := doc["sections"].(map[string]interface{})
	sec, ok := sections["medicalEquipment"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected medicalEquipment section to be present, sections=%v", sections)
	}

	primaryEntries, _ := sec["entries"].([]interface{})
	if len(primaryEntries) != 1 {
		t.Fatalf("expected 1 primary entry to survive (not overwritten by the second sectionMappingRow), got %d: %v", len(primaryEntries), sec)
	}
	primary, _ := primaryEntries[0].(map[string]interface{})
	if primary["equipmentCode"] != "360008007" {
		t.Errorf("expected primary entry's equipmentCode=360008007, got %v", primary["equipmentCode"])
	}

	altEntries, _ := sec["procedureEntries"].([]interface{})
	if len(altEntries) != 1 {
		t.Fatalf("expected 1 alternate procedureEntries entry, got %d: %v", len(altEntries), sec)
	}
	alt, _ := altEntries[0].(map[string]interface{})
	if alt["equipmentProcedureCode"] != "87717006" {
		t.Errorf("expected alternate entry's equipmentProcedureCode=87717006, got %v", alt["equipmentProcedureCode"])
	}
}
