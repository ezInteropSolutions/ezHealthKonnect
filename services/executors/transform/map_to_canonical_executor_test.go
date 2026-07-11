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
