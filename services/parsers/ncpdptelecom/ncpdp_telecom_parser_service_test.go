package ncpdptelecomparser

import (
	"os"
	"testing"
)

// TestParse_SetsRawField is the regression proof for the same real,
// live-verification-caught bug fixed in ncpdp_script_parser_service.go on
// 2026-09-27 (see that file's own test for the full story): this parser's own
// Parse() never wrote the original message text into ParsedJSON["raw"],
// unlike its FHIR/HL7/CDA siblings — so a genuinely live message could never
// resolve a mid-pipeline ncpdptelecom.parse step's default sourceField
// ("raw").
func TestParse_SetsRawField(t *testing.T) {
	raw, err := os.ReadFile("../../../ncpdptelecom/testdata/generated_samples/request_baseline_female.txt")
	if err != nil {
		t.Fatalf("failed to read real sample fixture: %v", err)
	}

	parser, err := NewFromSchemaDir("../../../ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}

	result := parser.Parse(string(raw))
	if !result.Success {
		t.Fatalf("expected successful parse, got error: %s", result.Error)
	}
	if result.ParsedJSON["raw"] != string(raw) {
		t.Errorf("expected ParsedJSON[\"raw\"] to equal the original message text, got %v", result.ParsedJSON["raw"])
	}
}
