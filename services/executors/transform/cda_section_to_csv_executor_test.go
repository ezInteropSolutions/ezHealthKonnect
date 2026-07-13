// services/executors/transform/cda_section_to_csv_executor_test.go
package transform

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

// ── Test helpers ──────────────────────────────────────────────────────────

// repoRoot resolves the absolute path to the repository root from this test
// file's own location, mirroring cda/document/parser_test.go's schemaDir helper.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// services/executors/transform/this_file.go -> up three levels -> repo root
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func loadCorpusTypedDocument(t *testing.T, filename string) *cdadocument.CDADocument {
	t.Helper()
	root := repoRoot(t)
	svc, err := cdaparser.NewFromSchemaDir(filepath.Join(root, "cda", "schemas"))
	if err != nil {
		t.Fatalf("NewFromSchemaDir: %v", err)
	}
	rawXML, err := os.ReadFile(filepath.Join(root, "cda", "document", "testdata", "corpus", filename))
	if err != nil {
		t.Fatalf("reading corpus file %s: %v", filename, err)
	}
	result := svc.Parse(string(rawXML))
	if !result.Success {
		t.Fatalf("parsing corpus file %s: %v", filename, result.Error)
	}
	doc, ok := result.TypedDocument.(*cdadocument.CDADocument)
	if !ok {
		t.Fatalf("corpus file %s: parse produced no typed document", filename)
	}
	return doc
}

// ── cdaValueToCSVString / cdaMapToCSVString ──────────────────────────────

func TestCdaValueToCSVString_Nil(t *testing.T) {
	if got := cdaValueToCSVString(nil); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestCdaValueToCSVString_PlainString(t *testing.T) {
	if got := cdaValueToCSVString("completed"); got != "completed" {
		t.Errorf("got %q, want %q", got, "completed")
	}
}

func TestCdaValueToCSVString_Float64WholeNumber_NoDecimal(t *testing.T) {
	// JSON numbers decode to float64 in Go -- a whole-number quantity (e.g.
	// dose "500") must render as "500", not "500.000000" or "5e+02".
	if got := cdaValueToCSVString(float64(500)); got != "500" {
		t.Errorf("got %q, want %q", got, "500")
	}
}

func TestCdaValueToCSVString_Float64Fractional(t *testing.T) {
	if got := cdaValueToCSVString(float64(98.6)); got != "98.6" {
		t.Errorf("got %q, want %q", got, "98.6")
	}
}

func TestCdaValueToCSVString_CDACodeShape_PrefersDisplayName(t *testing.T) {
	v := map[string]interface{}{"code": "38341003", "displayName": "Hypertension", "codeSystem": "2.16.840.1.113883.6.96"}
	if got := cdaValueToCSVString(v); got != "Hypertension" {
		t.Errorf("got %q, want %q", got, "Hypertension")
	}
}

func TestCdaValueToCSVString_CDACodeShape_FallsBackToCode(t *testing.T) {
	v := map[string]interface{}{"code": "38341003"}
	if got := cdaValueToCSVString(v); got != "38341003" {
		t.Errorf("got %q, want %q", got, "38341003")
	}
}

// TestCdaValueToCSVString_CDACodeShape_FallsBackToOriginalText covers the
// real-world Epic pattern this whole fix targets: no inline displayName, but
// section_parser.go's resolveEntryRefs already resolved the
// <originalText><reference value="#id"/></originalText> anchor into
// OriginalText — the CSV cell should use that, not fall all the way to the
// bare code.
func TestCdaValueToCSVString_CDACodeShape_FallsBackToOriginalText(t *testing.T) {
	v := map[string]interface{}{"code": "1053697", "originalText": "nystatin-triamcinolone (MYCOLOG II) ointment"}
	if got := cdaValueToCSVString(v); got != "nystatin-triamcinolone (MYCOLOG II) ointment" {
		t.Errorf("got %q, want the resolved originalText", got)
	}
}

// TestCdaValueToCSVString_CDACodeShape_DisplayNameBeatsOriginalText asserts
// the priority order: an inline displayName (when present) still wins over
// originalText, which is only ever a fallback.
func TestCdaValueToCSVString_CDACodeShape_DisplayNameBeatsOriginalText(t *testing.T) {
	v := map[string]interface{}{"displayName": "Hypertension", "originalText": "some narrative text"}
	if got := cdaValueToCSVString(v); got != "Hypertension" {
		t.Errorf("got %q, want %q", got, "Hypertension")
	}
}

func TestCdaValueToCSVString_CDAQuantityShape(t *testing.T) {
	v := map[string]interface{}{"value": "120", "unit": "mmHg"}
	if got := cdaValueToCSVString(v); got != "120 mmHg" {
		t.Errorf("got %q, want %q", got, "120 mmHg")
	}
}

func TestCdaValueToCSVString_CDATimeRangeShape_LowHigh(t *testing.T) {
	v := map[string]interface{}{
		"low":  map[string]interface{}{"value": "20200101"},
		"high": map[string]interface{}{"value": "20200110"},
	}
	if got := cdaValueToCSVString(v); got != "20200101 - 20200110" {
		t.Errorf("got %q, want %q", got, "20200101 - 20200110")
	}
}

func TestCdaValueToCSVString_EmptyMap_ReturnsEmpty(t *testing.T) {
	if got := cdaValueToCSVString(map[string]interface{}{}); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestCdaValueToCSVString_Array_JoinsWithSemicolon(t *testing.T) {
	v := []interface{}{"a", "b", "c"}
	if got := cdaValueToCSVString(v); got != "a; b; c" {
		t.Errorf("got %q, want %q", got, "a; b; c")
	}
}

// ── writeCSV ──────────────────────────────────────────────────────────────

func TestWriteCSV_PreservesDefinedColumnOrder_NotAlphabetical(t *testing.T) {
	columns := []CSVColumn{{Name: "MedicationName"}, {Name: "Code"}, {Name: "StartDate"}, {Name: "AbatementDate"}}
	rows := []map[string]string{{"MedicationName": "Lisinopril", "Code": "29046", "StartDate": "2020-01-01", "AbatementDate": ""}}

	got := writeCSV(columns, rows)
	r := csv.NewReader(strings.NewReader(got))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("re-parsing generated CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (header + 1 row)", len(records))
	}
	want := []string{"MedicationName", "Code", "StartDate", "AbatementDate"}
	for i, w := range want {
		if records[0][i] != w {
			t.Errorf("header[%d] = %q, want %q (alphabetical order would put AbatementDate first)", i, records[0][i], w)
		}
	}
	if records[1][0] != "Lisinopril" {
		t.Errorf("row[0] = %q, want %q", records[1][0], "Lisinopril")
	}
}

// ── buildCSVRows (synthetic documentMap, no real parse needed) ────────────

func TestBuildCSVRows_ResolvesColumnsPerEntry(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode": "completed",
						"moodCode":   "EVN",
						"consumable": map[string]interface{}{
							"manufacturedProduct": map[string]interface{}{
								"manufacturedMaterial": map[string]interface{}{
									"code": map[string]interface{}{"code": "29046", "displayName": "Lisinopril 10mg", "codeSystem": "2.16.840.1.113883.6.88"},
								},
							},
						},
						"doseQuantity": map[string]interface{}{"value": "10", "unit": "mg"},
					},
				},
			},
		},
	}

	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row["MedicationName"] != "Lisinopril 10mg" {
		t.Errorf("MedicationName = %q, want %q", row["MedicationName"], "Lisinopril 10mg")
	}
	if row["MedicationCode"] != "29046" {
		t.Errorf("MedicationCode = %q, want %q", row["MedicationCode"], "29046")
	}
	if row["Status"] != "completed" {
		t.Errorf("Status = %q, want %q", row["Status"], "completed")
	}
	if row["Dose"] != "10" {
		t.Errorf("Dose = %q, want %q", row["Dose"], "10")
	}
	if row["DoseUnit"] != "mg" {
		t.Errorf("DoseUnit = %q, want %q", row["DoseUnit"], "mg")
	}
}

// TestBuildCSVRows_StampsSourceFileOnEveryRow covers the SourceFile column:
// it's document-level metadata (the original inbound filename, stamped onto
// the message envelope at ingestion — see sourceFileFromInputData), not a
// per-entry field, so every row of every section must carry the same value
// regardless of what that entry itself contains.
func TestBuildCSVRows_StampsSourceFileOnEveryRow(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"statusCode": "completed"},
					map[string]interface{}{"statusCode": "active"},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "CCD_04_22_2026.3.xml")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if row["SourceFile"] != "CCD_04_22_2026.3.xml" {
			t.Errorf("row[%d] SourceFile = %q, want %q", i, row["SourceFile"], "CCD_04_22_2026.3.xml")
		}
	}
}

// ── expandCodeMetadataColumns / universalEntryColumns (Id/Author/CodeSystem) ─

func TestExpandCodeMetadataColumns_InsertsThreeSiblingsAfterFlaggedColumn(t *testing.T) {
	cols := expandCodeMetadataColumns([]CSVColumn{
		{Name: "MedicationName", Path: "consumable.manufacturedProduct.manufacturedMaterial.code", ExposeCodeMetadata: true},
		{Name: "MedicationCode", Path: "consumable.manufacturedProduct.manufacturedMaterial.code.code"},
	})
	want := []string{"MedicationName", "MedicationNameCodeSystem", "MedicationNameCodeSystemName", "MedicationNameOriginalText", "MedicationCode"}
	if len(cols) != len(want) {
		t.Fatalf("got %d columns, want %d: %+v", len(cols), len(want), cols)
	}
	for i, name := range want {
		if cols[i].Name != name {
			t.Errorf("column[%d].Name = %q, want %q", i, cols[i].Name, name)
		}
	}
	if cols[1].Path != "consumable.manufacturedProduct.manufacturedMaterial.code.codeSystem" {
		t.Errorf("CodeSystem column Path = %q, want the base path + \".codeSystem\"", cols[1].Path)
	}
}

func TestExpandCodeMetadataColumns_UnflaggedColumnUnchanged(t *testing.T) {
	cols := expandCodeMetadataColumns([]CSVColumn{{Name: "Status", Path: "statusCode"}})
	if len(cols) != 1 || cols[0].Name != "Status" {
		t.Errorf("got %+v, want unchanged single Status column", cols)
	}
}

// TestBuildCSVRows_CodeMetadataAndUniversalColumns_ResolveOnRealShapedData
// covers the full rollout end-to-end: MedicationNameCodeSystem/
// CodeSystemName/OriginalText derived from MedicationName's own path, plus
// the universal EntryId/AuthorName/AuthorOrganization columns reading the
// entry's own <id>/<author> — the exact fields a real Social History
// observation carries (id, code.codeSystem/codeSystemName, value's
// originalText, author's assignedPerson/representedOrganization) that no
// template exposed before this.
func TestBuildCSVRows_CodeMetadataAndUniversalColumns_ResolveOnRealShapedData(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"socialHistory": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"id":         []interface{}{map[string]interface{}{"root": "226543f9-a99c-4610-6ab2-e3c95382a303"}},
						"statusCode": "completed",
						"code":       map[string]interface{}{"code": "72166-2", "codeSystem": "2.16.840.1.113883.6.1", "codeSystemName": "LOINC", "displayName": "Tobacco smoking status NHIS"},
						"value":      map[string]interface{}{"code": map[string]interface{}{"code": "8517006", "codeSystem": "2.16.840.1.113883.6.96", "codeSystemName": "SNOMED CT", "displayName": "Ex-smoker (finding)", "originalText": "Ex-smoker"}},
						"authors": []interface{}{
							map[string]interface{}{
								"assignedAuthor": map[string]interface{}{
									"assignedPerson":          map[string]interface{}{"names": []interface{}{map[string]interface{}{"given": []interface{}{"Kelly"}, "family": "Johnson"}}},
									"representedOrganization": map[string]interface{}{"names": []interface{}{"Cumberland Healthcare"}},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["socialHistory"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	checks := map[string]string{
		"EntryId":                       "226543f9-a99c-4610-6ab2-e3c95382a303",
		"ObservationType":               "Tobacco smoking status NHIS",
		"ObservationTypeCodeSystem":     "2.16.840.1.113883.6.1",
		"ObservationTypeCodeSystemName": "LOINC",
		"Value":                         "Ex-smoker (finding)",
		"ValueCodeSystem":               "2.16.840.1.113883.6.96",
		"ValueCodeSystemName":           "SNOMED CT",
		"ValueOriginalText":             "Ex-smoker",
		"AuthorName":                    "Kelly Johnson",
		"AuthorOrganization":            "Cumberland Healthcare",
	}
	for col, want := range checks {
		if got := row[col]; got != want {
			t.Errorf("%s = %q, want %q", col, got, want)
		}
	}
}

// TestBuildCSVRows_EntryId_FallsBackToRootWhenExtensionAbsent covers the
// common shape where an entry's <id> has only a root (a GUID-style unique
// identifier, no meaningful extension) -- e.g. every Social History
// observation in a real 01/29 CCD.
func TestBuildCSVRows_EntryId_FallsBackToRootWhenExtensionAbsent(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"id": []interface{}{map[string]interface{}{"root": "guid-only-1234"}}},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["EntryId"] != "guid-only-1234" {
		t.Errorf("EntryId = %q, want %q", rows[0]["EntryId"], "guid-only-1234")
	}
}

func TestSourceFileFromInputData_ReadsMessageEnvelopeField(t *testing.T) {
	inputData := map[string]interface{}{
		"message": map[string]interface{}{"_sourceFile": "some_upload.xml"},
	}
	if got := sourceFileFromInputData(inputData); got != "some_upload.xml" {
		t.Errorf("got %q, want %q", got, "some_upload.xml")
	}
}

func TestSourceFileFromInputData_MissingMessage_ReturnsEmpty(t *testing.T) {
	if got := sourceFileFromInputData(map[string]interface{}{}); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestCsvColumnsWithSourceFile_PrependsSourceFileColumn(t *testing.T) {
	cols := csvColumnsWithSourceFile([]CSVColumn{{Name: "MedicationName", Path: "code"}})
	if len(cols) != 2 || cols[0].Name != "SourceFile" || cols[1].Name != "MedicationName" {
		t.Errorf("got %+v, want [SourceFile, MedicationName]", cols)
	}
}

// TestResolveColumn_UsesFallbackPathWhenPrimaryResolvesNothing covers the
// real Epic-CCD encounter shape (LOC participant as a direct child of the
// entry, not wrapped in a COMP entryRelationship) that the encounters
// section's Location column relies on FallbackPaths to reach.
func TestResolveColumn_UsesFallbackPathWhenPrimaryResolvesNothing(t *testing.T) {
	node := map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{
				"typeCode": "LOC",
				"participantRole": map[string]interface{}{
					"playingEntity": map[string]interface{}{
						"names": []interface{}{
							map[string]interface{}{"family": "mumbai Women's Care mira road"},
						},
					},
				},
			},
		},
	}
	col := CSVColumn{
		Name:          "Location",
		Path:          "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC].participantRole.playingEntity.names[0].family",
		FallbackPaths: []string{"participants[typeCode=LOC].participantRole.playingEntity.names[0].family"},
	}
	if got := resolveColumn(node, col); got != "mumbai Women's Care mira road" {
		t.Errorf("got %q, want the fallback-resolved location name", got)
	}
}

func TestResolveColumn_PrimaryPathWinsWhenPresent(t *testing.T) {
	node := map[string]interface{}{"code": "already-resolved"}
	col := CSVColumn{Name: "X", Path: "code", FallbackPaths: []string{"neverUsed"}}
	if got := resolveColumn(node, col); got != "already-resolved" {
		t.Errorf("got %q, want primary path value", got)
	}
}

// ── buildRowsForEntry (organizer/component flattening) ────────────────────

// TestBuildRowsForEntry_FlattensOrganizerComponents mirrors the real 99397
// Epic sample's Vital Signs Organizer: one organizer entry wrapping several
// component observations (systolic BP, diastolic BP, ...), each with its own
// code/value but relying on the organizer's shared effectiveTime. Before this
// fix, buildCSVRows resolved columns against the organizer itself and
// produced exactly one row with no Value/Unit at all.
func TestBuildRowsForEntry_FlattensOrganizerComponents(t *testing.T) {
	entry := map[string]interface{}{
		"code":          map[string]interface{}{"code": "46680005", "displayName": "Vital signs"},
		"effectiveTime": map[string]interface{}{"value": map[string]interface{}{"value": "20260106161500+0000"}},
		"components": []interface{}{
			map[string]interface{}{
				"code":  map[string]interface{}{"code": "8480-6", "originalText": "Systolic blood pressure"},
				"value": map[string]interface{}{"quantity": map[string]interface{}{"value": "124", "unit": "mm[Hg]"}},
			},
			map[string]interface{}{
				"code":  map[string]interface{}{"code": "8462-4", "originalText": "Diastolic blood pressure"},
				"value": map[string]interface{}{"quantity": map[string]interface{}{"value": "72", "unit": "mm[Hg]"}},
			},
		},
	}

	rows := buildRowsForEntry(entry, cdaCSVSectionTemplates["vitalSigns"].Columns)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (one per component, not 1 for the organizer)", len(rows))
	}
	if rows[0]["VitalSignType"] != "Systolic blood pressure" || rows[0]["Value"] != "124" || rows[0]["Unit"] != "mm[Hg]" {
		t.Errorf("row[0] = %+v, want systolic BP with value/unit populated", rows[0])
	}
	if rows[1]["VitalSignType"] != "Diastolic blood pressure" || rows[1]["Value"] != "72" {
		t.Errorf("row[1] = %+v, want diastolic BP with value populated", rows[1])
	}
	// Neither component carries its own effectiveTime -- both rows must fall
	// back to the organizer's shared timestamp.
	for i, row := range rows {
		if row["Date"] != "20260106161500+0000" {
			t.Errorf("row[%d] Date = %q, want organizer's shared effectiveTime as fallback", i, row["Date"])
		}
	}
}

func TestBuildRowsForEntry_NoComponents_ReturnsSingleRow(t *testing.T) {
	entry := map[string]interface{}{
		"statusCode": "completed",
		"consumable": map[string]interface{}{
			"manufacturedProduct": map[string]interface{}{
				"manufacturedMaterial": map[string]interface{}{
					"code": map[string]interface{}{"code": "150", "displayName": "Influenza"},
				},
			},
		},
	}
	rows := buildRowsForEntry(entry, cdaCSVSectionTemplates["immunizations"].Columns)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (flat entry, no components)", len(rows))
	}
	if rows[0]["VaccineName"] != "Influenza" {
		t.Errorf("VaccineName = %q, want %q", rows[0]["VaccineName"], "Influenza")
	}
}

func TestBuildCSVRows_NoEntries_ReturnsEmptyNotNil(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if rows == nil {
		t.Error("rows is nil, want an empty (but non-nil) slice")
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

// ── stripCDANarrativeXML / buildNarrativeRow (NarrativeOnly sections) ────

func TestStripCDANarrativeXML_DropsTagsAndJoinsText(t *testing.T) {
	xml := `<text><paragraph>Patient reports mild headache.</paragraph><paragraph>No fever.</paragraph></text>`
	got := stripCDANarrativeXML(xml)
	want := "Patient reports mild headache.\nNo fever."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCDANarrativeXML_TableRowsSeparatedByNewlines(t *testing.T) {
	xml := `<text><table><tbody><tr><td>Row one</td></tr><tr><td>Row two</td></tr></tbody></table></text>`
	got := stripCDANarrativeXML(xml)
	if !strings.Contains(got, "Row one") || !strings.Contains(got, "Row two") {
		t.Fatalf("got %q, want both row texts present", got)
	}
	if !strings.Contains(got, "Row one\n") {
		t.Errorf("got %q, want a newline between table rows", got)
	}
}

func TestStripCDANarrativeXML_EmptyInput_ReturnsEmpty(t *testing.T) {
	if got := stripCDANarrativeXML(""); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
	if got := stripCDANarrativeXML("   "); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestStripCDANarrativeXML_MalformedInput_NeverPanics(t *testing.T) {
	// Truncated tag mid-stream -- decoder gives up, but must return whatever
	// text was recovered before that point, never panic or error out.
	got := stripCDANarrativeXML(`<text><paragraph>Some text<param`)
	if got != "Some text" {
		t.Errorf("got %q, want %q", got, "Some text")
	}
}

// TestBuildNarrativeRow_EmitsOneRowWithPlainText covers the NarrativeOnly
// section dispatch (e.g. Assessment, Reason for Referral, History of Present
// Illness) -- these carry no structured <entry> in real documents, so the
// CSV is one row holding the section's own narrative text instead of a
// per-entry table.
func TestBuildNarrativeRow_EmitsOneRowWithPlainText(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"assessment": map[string]interface{}{
				"narrativeText": `<text><paragraph>Stable, continue current plan.</paragraph></text>`,
			},
		},
	}
	rows, columns := buildNarrativeRow(documentMap, "assessment", "note.xml")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["NarrativeText"] != "Stable, continue current plan." {
		t.Errorf("NarrativeText = %q, want %q", rows[0]["NarrativeText"], "Stable, continue current plan.")
	}
	if rows[0]["SourceFile"] != "note.xml" {
		t.Errorf("SourceFile = %q, want %q", rows[0]["SourceFile"], "note.xml")
	}
	if len(columns) != 2 || columns[0].Name != "SourceFile" || columns[1].Name != "NarrativeText" {
		t.Errorf("columns = %+v, want [SourceFile, NarrativeText]", columns)
	}
}

func TestBuildNarrativeRow_SectionAbsent_ReturnsZeroRows(t *testing.T) {
	documentMap := map[string]interface{}{"sectionsByKey": map[string]interface{}{}}
	rows, _ := buildNarrativeRow(documentMap, "assessment", "")
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0 (section absent)", len(rows))
	}
}

// ── New structured section templates (real-shape regression coverage) ───

// TestBuildCSVRows_CareTeam_FlattensOrganizerAndReadsPerformer covers the
// Care Team Organizer -> component/act -> performer shape found in a real
// CCD: the component act's own <code> is a fixed "Care Team Information"
// label, so MemberRole/MemberName must read the performer instead.
func TestBuildCSVRows_CareTeam_FlattensOrganizerAndReadsPerformer(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"careTeam": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"components": []interface{}{
							map[string]interface{}{
								"statusCode": "active",
								"performers": []interface{}{
									map[string]interface{}{
										"functionCode": map[string]interface{}{"originalText": "primary care physician"},
										"assignedEntity": map[string]interface{}{
											"assignedPerson": map[string]interface{}{
												"names": []interface{}{
													map[string]interface{}{"given": []interface{}{"Jennifer"}, "family": "Murdza"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["careTeam"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["MemberRole"] != "primary care physician" {
		t.Errorf("MemberRole = %q, want %q", rows[0]["MemberRole"], "primary care physician")
	}
	if rows[0]["MemberName"] != "Jennifer Murdza" {
		t.Errorf("MemberName = %q, want %q", rows[0]["MemberName"], "Jennifer Murdza")
	}
}

// TestBuildCSVRows_FamilyHistory_RelationshipFallsBackToOrganizer covers the
// Family History Organizer: Relationship lives on the organizer entry itself
// (RelatedSubjectCode, a sibling of Components), so each condition row must
// fall back to the parent organizer to read it.
func TestBuildCSVRows_FamilyHistory_RelationshipFallsBackToOrganizer(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"familyHistory": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"relatedSubjectCode": map[string]interface{}{"displayName": "mother"},
						"components": []interface{}{
							map[string]interface{}{
								"statusCode": "completed",
								"value":      map[string]interface{}{"code": map[string]interface{}{"displayName": "Malignant neoplasm"}},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["familyHistory"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Relationship"] != "mother" {
		t.Errorf("Relationship = %q, want %q", rows[0]["Relationship"], "mother")
	}
	if rows[0]["Condition"] != "Malignant neoplasm" {
		t.Errorf("Condition = %q, want %q", rows[0]["Condition"], "Malignant neoplasm")
	}
}

// TestBuildCSVRows_ProblemName_ResolvesViaValueCode covers the fix for a path
// that used to be flatly broken: "value.displayName" doesn't exist anywhere
// on CDAValue (only a nested "value.code.displayName"/"value.code"), so
// ProblemName silently resolved to nothing on every real document. Fixed to
// "value.code", matching AllergyType's already-working pattern.
func TestBuildCSVRows_ProblemName_ResolvesViaValueCode(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"problems": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "SUBJ",
								"entry": map[string]interface{}{
									"statusCode": "completed",
									"value":      map[string]interface{}{"code": map[string]interface{}{"code": "40930008", "originalText": "Hypothyroidism"}},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["problems"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["ProblemName"] != "Hypothyroidism" {
		t.Errorf("ProblemName = %q, want %q", rows[0]["ProblemName"], "Hypothyroidism")
	}
	if rows[0]["ProblemCode"] != "40930008" {
		t.Errorf("ProblemCode = %q, want %q", rows[0]["ProblemCode"], "40930008")
	}
}

// TestBuildCSVRows_MedicationSigAndComment_ScopedToDistinctTemplates covers
// the fix for Sig/Note (now the universal Comment column) sharing one
// unscoped path (any COMP entryRelationship's text), which always
// duplicated Sig into Note. Scoped by templateId to the Free Text Sig
// (.4.147) and Comment Activity (.4.64) templates respectively — a document
// with only a Free Text Sig COMP entry (the common real-world case, per the
// 99397 sample) must populate Sig and leave Comment empty, not copy Sig
// into both.
func TestBuildCSVRows_MedicationSigAndComment_ScopedToDistinctTemplates(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode": "active",
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "COMP",
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.147"},
									"text":        "Apply topically 2 (two) times a day.",
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Sig"] != "Apply topically 2 (two) times a day." {
		t.Errorf("Sig = %q, want the Free Text Sig entry's text", rows[0]["Sig"])
	}
	if rows[0]["Comment"] != "" {
		t.Errorf("Comment = %q, want empty (no Comment Activity present, must not duplicate Sig)", rows[0]["Comment"])
	}
}

// TestUniversalEntryColumns_Comment_ResolvesAcrossAnySection covers the
// Comment Activity pattern (templateId .4.64) as a UNIVERSAL column, not
// re-authored per section — the C-CDA on FHIR IG specifies the exact same
// annotation shape for every section.
func TestUniversalEntryColumns_Comment_ResolvesAcrossAnySection(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"problems": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "COMP",
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.64"},
									"text":        "Patient tolerating current regimen well.",
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["problems"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Comment"] != "Patient tolerating current regimen well." {
		t.Errorf("Comment = %q, want the Comment Activity's text", rows[0]["Comment"])
	}
}

// TestBuildCSVRows_MedicationSig_FallsBackToInstructionV2 covers the more
// common real-world Sig representation across audited documents:
// Instruction (V2) (.4.20) via a SUBJ/inversionInd=true entryRelationship,
// with no Free Text Sig (.4.147) COMP entry present at all.
func TestBuildCSVRows_MedicationSig_FallsBackToInstructionV2(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode": "active",
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode":     "SUBJ",
								"inversionInd": true,
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.20"},
									"text":        "TAKE 1 CAPSULE(100 MG) BY MOUTH EVERY NIGHT",
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Sig"] != "TAKE 1 CAPSULE(100 MG) BY MOUTH EVERY NIGHT" {
		t.Errorf("Sig = %q, want the Instruction (V2) entry's text via fallback", rows[0]["Sig"])
	}
}

// TestBuildCSVRows_Medication_ApproachSiteMaxDoseAndIndication covers the
// three substanceAdministration fields (ApproachSite, MaxDose, Indication)
// added after auditing real CCDs where they carried genuine (non-nullFlavor)
// data that entry_parser.go parsed nowhere and no CSV column exposed.
func TestBuildCSVRows_Medication_ApproachSiteMaxDoseAndIndication(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode":       "active",
						"approachSiteCode": map[string]interface{}{"originalText": "Right Deltoid"},
						"maxDoseQuantity":  map[string]interface{}{"numerator": map[string]interface{}{"value": "20.0", "unit": "MG"}},
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "RSON",
								"entry":    map[string]interface{}{"value": map[string]interface{}{"code": map[string]interface{}{"displayName": "Vulvar itching"}}},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["ApproachSite"] != "Right Deltoid" {
		t.Errorf("ApproachSite = %q, want %q", rows[0]["ApproachSite"], "Right Deltoid")
	}
	if rows[0]["MaxDose"] != "20.0 MG" {
		t.Errorf("MaxDose = %q, want %q", rows[0]["MaxDose"], "20.0 MG")
	}
	if rows[0]["Indication"] != "Vulvar itching" {
		t.Errorf("Indication = %q, want %q", rows[0]["Indication"], "Vulvar itching")
	}
}

// TestBuildCSVRows_Allergy_ReactionSeverityCriticality_AreSiblingRelationships
// covers the PDF-spec-verified structure (Section 3.103.1 "Allergy -
// Intolerance Observation (V2)", Table 491 constraints #14-16): Reaction,
// Severity, and Criticality are three SIBLING entryRelationships directly on
// the Allergy - Intolerance Observation, not Severity nested inside
// Reaction as an earlier (unverified, trained-memory-based) implementation
// assumed. Reaction uses typeCode=MFST; Severity and Criticality both use
// typeCode=SUBJ -- all three require inversionInd=true.
func TestBuildCSVRows_Allergy_ReactionSeverityCriticality_AreSiblingRelationships(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode":     "MFST",
								"inversionInd": true,
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.9"},
									"value":       map[string]interface{}{"code": map[string]interface{}{"code": "247472004", "displayName": "Hives"}},
								},
							},
							map[string]interface{}{
								"typeCode":     "SUBJ",
								"inversionInd": true,
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.8"},
									"value":       map[string]interface{}{"code": map[string]interface{}{"code": "24484000", "displayName": "Severe"}},
								},
							},
							map[string]interface{}{
								"typeCode":     "SUBJ",
								"inversionInd": true,
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.145"},
									"value":       map[string]interface{}{"code": map[string]interface{}{"code": "82606-5", "displayName": "High"}},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["allergiesAndIntolerances"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Reaction"] != "Hives" {
		t.Errorf("Reaction = %q, want %q", rows[0]["Reaction"], "Hives")
	}
	if rows[0]["Severity"] != "Severe" {
		t.Errorf("Severity = %q, want %q (sibling of Reaction, not nested inside it)", rows[0]["Severity"], "Severe")
	}
	if rows[0]["Criticality"] != "High" {
		t.Errorf("Criticality = %q, want %q", rows[0]["Criticality"], "High")
	}
}

// TestBuildCSVRows_PayersInsurance_SubscriberId_PrefersHLDOverCOV covers the
// PDF-spec-verified distinction (Section 3.70 "Policy Activity (V3)", Table
// 398, CONF:1198-8916/8934) between the COV "Covered Party" participant
// (whoever is covered, typically the patient) and the separate HLD
// "Policyholder" participant (who actually subscribes to the policy) --
// "Subscriber" maps to HLD, not COV as an earlier implementation assumed.
func TestBuildCSVRows_PayersInsurance_SubscriberId_PrefersHLDOverCOV(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"payersInsurance": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "COMP",
								"entry": map[string]interface{}{
									"participants": []interface{}{
										map[string]interface{}{
											"typeCode":        "COV",
											"participantRole": map[string]interface{}{"id": []interface{}{map[string]interface{}{"extension": "COV-999"}}},
										},
										map[string]interface{}{
											"typeCode":        "HLD",
											"participantRole": map[string]interface{}{"id": []interface{}{map[string]interface{}{"extension": "HLD-123"}}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["payersInsurance"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["SubscriberId"] != "HLD-123" {
		t.Errorf("SubscriberId = %q, want %q (HLD/Policyholder, not COV/Covered Party)", rows[0]["SubscriberId"], "HLD-123")
	}
}

// TestBuildCSVRows_PayersInsurance_SubscriberId_FallsBackToCOVWhenNoHLD
// covers the common patient-is-subscriber case, where a document has no
// distinct HLD participant at all -- SubscriberId must still resolve via
// the COV participant's id rather than coming back empty.
func TestBuildCSVRows_PayersInsurance_SubscriberId_FallsBackToCOVWhenNoHLD(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"payersInsurance": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "COMP",
								"entry": map[string]interface{}{
									"participants": []interface{}{
										map[string]interface{}{
											"typeCode":        "COV",
											"participantRole": map[string]interface{}{"id": []interface{}{map[string]interface{}{"extension": "COV-999"}}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["payersInsurance"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["SubscriberId"] != "COV-999" {
		t.Errorf("SubscriberId = %q, want %q (fallback to COV when no HLD present)", rows[0]["SubscriberId"], "COV-999")
	}
}

// TestBuildCSVRows_EncounterPeriodStart_FallsBackToSinglePointTime covers a
// real Encounter Activity (Bluestone Physician Services "AWV" sample) whose
// effectiveTime is a single-point TS ("<effectiveTime value=.../>", no
// low/high) rather than an interval -- both are valid per the base CDA R2
// schema. Before the FallbackPaths fix, PeriodStart/PeriodEnd came back
// empty on every single-point encounter even though the date was present.
func TestBuildCSVRows_EncounterPeriodStart_FallsBackToSinglePointTime(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"encounters": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode":    "completed",
						"code":          map[string]interface{}{"code": "G0439", "displayName": "Medicare Annual Wellness Visit"},
						"effectiveTime": map[string]interface{}{"value": map[string]interface{}{"value": "202605040000-0600"}},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["encounters"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["PeriodStart"] != "202605040000-0600" {
		t.Errorf("PeriodStart = %q, want the single-point effectiveTime value", rows[0]["PeriodStart"])
	}
	if rows[0]["PeriodEnd"] != "" {
		t.Errorf("PeriodEnd = %q, want empty (single-point time has no distinct end)", rows[0]["PeriodEnd"])
	}
}

// TestBuildCSVRows_EncounterDiagnosis_OuterRelationshipTypeCodeIsUnconstrained
// covers the PDF-spec finding (Section 3.22 "Encounter Activity (V3)", Table
// 268 constraint #11, CONF:1198-15492) that the entryRelationship wrapping
// Encounter Diagnosis inside Encounter Activity carries NO normative
// @typeCode constraint in the IG -- unlike every other entryRelationship in
// the document, no CONF# is given for it. An earlier (unverified) assumption
// of typeCode=COMP would silently fail to match a real document using any
// other typeCode; this fixture deliberately uses "COMP" to prove the [*]
// wildcard still matches it via the .4.80 templateId alone, and would
// equally match REFR, SUBJ, or any other value.
func TestBuildCSVRows_EncounterDiagnosis_OuterRelationshipTypeCodeIsUnconstrained(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"encounters": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"statusCode": "active",
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "COMP",
								"entry": map[string]interface{}{
									"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.80"},
									"entryRelationships": []interface{}{
										map[string]interface{}{
											"typeCode": "SUBJ",
											"entry": map[string]interface{}{
												"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.4"},
												"value":       map[string]interface{}{"code": map[string]interface{}{"code": "444814009", "displayName": "Viral sinusitis"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["encounters"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["Diagnosis"] != "Viral sinusitis" {
		t.Errorf("Diagnosis = %q, want %q", rows[0]["Diagnosis"], "Viral sinusitis")
	}
}

// TestBuildCSVRows_Problem_AgeAtOnset_UsesSUBJInversionIndTrue covers the
// PDF-spec finding (Section 3.4 "Age Observation", Problem Observation V3's
// own constraint list, CONF:1198-9059/9060/9069/15590) that Age Observation
// is wrapped by @typeCode="SUBJ" @inversionInd="true" -- an earlier
// (unverified) implementation used typeCode=REFR, which the same spec table
// actually reserves for a different, unrelated nested Prognosis Observation.
func TestBuildCSVRows_Problem_AgeAtOnset_UsesSUBJInversionIndTrue(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"problems": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "SUBJ",
								"entry": map[string]interface{}{
									"statusCode": "completed",
									"value":      map[string]interface{}{"code": map[string]interface{}{"code": "40930008", "originalText": "Hypothyroidism"}},
									"entryRelationships": []interface{}{
										map[string]interface{}{
											"typeCode":     "SUBJ",
											"inversionInd": true,
											"entry": map[string]interface{}{
												"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.31"},
												"value":       map[string]interface{}{"value": "45", "unit": "a"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["problems"], "")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["AgeAtOnset"] != "45 a" {
		t.Errorf("AgeAtOnset = %q, want %q", rows[0]["AgeAtOnset"], "45 a")
	}
}

// ── SupportedCDACSVSections ─────────────────────────────────────────────

func TestSupportedCDACSVSections_IsSortedWithNoDuplicates(t *testing.T) {
	got := SupportedCDACSVSections()
	if len(got) != len(cdaCSVSectionTemplates) {
		t.Fatalf("got %d sections, want %d (one per registered template)", len(got), len(cdaCSVSectionTemplates))
	}
	seen := make(map[string]bool, len(got))
	for i, key := range got {
		if seen[key] {
			t.Errorf("duplicate section key %q", key)
		}
		seen[key] = true
		if i > 0 && got[i-1] >= key {
			t.Errorf("not sorted: %q appears after %q", key, got[i-1])
		}
	}
}

// TestSupportedCDACSVSections_IncludesOriginalNineAndNewAdditions guards
// against accidentally dropping a section key during later edits — the
// original 9 (verified against real Epic CCDs) plus a sample of the 26
// sections added 2026-07-12 after auditing 5 real cross-vendor CCDs
// (Cerner, HITSP C32, and 3 Epic-family documents), covering both the
// structured (careTeam, payersInsurance, familyHistory) and
// NarrativeOnly (assessment, historyOfPresentIllness) shapes.
func TestSupportedCDACSVSections_IncludesOriginalNineAndNewAdditions(t *testing.T) {
	got := SupportedCDACSVSections()
	inGot := make(map[string]bool, len(got))
	for _, k := range got {
		inGot[k] = true
	}
	want := []string{
		"allergiesAndIntolerances", "encounters", "immunizations", "medications",
		"problems", "procedures", "results", "socialHistory", "vitalSigns",
		"careTeam", "payersInsurance", "familyHistory", "goals", "healthConcerns",
		"medicalEquipment", "advanceDirectives", "assessment", "historyOfPresentIllness",
	}
	for _, k := range want {
		if !inGot[k] {
			t.Errorf("SupportedCDACSVSections() missing %q", k)
		}
	}
}

// ── Full executor, real corpus document ──────────────────────────────────

func TestCDASectionToCSVExecutor_RealCorpus_ProducesPlausibleCSV(t *testing.T) {
	for _, file := range []string{"cerner_sample.xml", "kareo_sample.xml", "mtuitive_sample.xml", "practicefusion_sample.xml"} {
		t.Run(file, func(t *testing.T) {
			doc := loadCorpusTypedDocument(t, file)

			exec := NewCDASectionToCSVExecutor()
			step := &models.TransformationStep{StepType: "cda.section_to_csv", Enabled: true, Config: map[string]interface{}{}}
			inputData := map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}}

			out, err := exec.Execute(context.Background(), step, inputData)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			// At least one OOB section must have produced CSV text with a header row.
			foundNonEmpty := false
			for _, sectionKey := range SupportedCDACSVSections() {
				csvText, _ := out["csv_"+sectionKey].(string)
				if strings.TrimSpace(csvText) != "" {
					foundNonEmpty = true
					lines := strings.Split(strings.TrimRight(csvText, "\n"), "\n")
					if len(lines) < 1 {
						t.Errorf("%s: csv_%s has no header line", file, sectionKey)
					}
				}
			}
			if !foundNonEmpty {
				t.Errorf("%s: no section produced any CSV output at all", file)
			}
		})
	}
}
