package edix12parser

import (
	"os"
	"testing"
)

// TestParse_SetsRawField is the regression proof for a real, live-verification-
// caught bug (found 2026-09-27 while verifying NCPDP's own identical gap, then
// confirmed here too): this parser's own Parse() never wrote the original
// message text into ParsedJSON["raw"], unlike its FHIR/HL7/CDA siblings — so a
// genuinely live message (never a Test Pipeline run, whose own envelope
// construction supplies raw content a different way) could never resolve a
// mid-pipeline edi.parse/edi.validate step's default sourceField ("raw").
func TestParse_SetsRawField(t *testing.T) {
	raw, err := os.ReadFile("../../../edi/testdata/real_samples/blue_cross_nc_sample.txt")
	if err != nil {
		t.Fatalf("failed to read real sample fixture: %v", err)
	}

	parser, err := NewFromSchemaDir("../../../edi/schemas/x12_005010")
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
