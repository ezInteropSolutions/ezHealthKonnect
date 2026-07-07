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

	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"])
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

func TestBuildCSVRows_NoEntries_ReturnsEmptyNotNil(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{},
	}
	rows := buildCSVRows(documentMap, cdaCSVSectionTemplates["medications"])
	if rows == nil {
		t.Error("rows is nil, want an empty (but non-nil) slice")
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

// ── SupportedCDACSVSections ─────────────────────────────────────────────

func TestSupportedCDACSVSections_IncludesAllNineSections(t *testing.T) {
	got := SupportedCDACSVSections()
	want := []string{
		"allergiesAndIntolerances", "encounters", "immunizations", "medications",
		"problems", "procedures", "results", "socialHistory", "vitalSigns",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sections %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section[%d] = %q, want %q (sorted order)", i, got[i], want[i])
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
