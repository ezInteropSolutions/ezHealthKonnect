package ncpdpscriptparser

import (
	"os"
	"testing"
)

// TestParse_SetsRawField is the regression proof for a real, live-verification-
// caught bug (found 2026-09-27): a real file dropped on a real file_listener
// connector, running the actual shipped V258 template's own unmodified
// ncpdp.parse step, failed with "source field \"raw\" is empty or not a
// string" — because this parser's own Parse() never wrote the original
// message text into ParsedJSON["raw"], unlike its FHIR/HL7/CDA siblings. A
// Test Pipeline run of the SAME unmodified pipeline had succeeded, masking
// the gap, since Test Pipeline's own envelope construction supplies raw
// content a different way that a genuinely live message never receives.
func TestParse_SetsRawField(t *testing.T) {
	raw, err := os.ReadFile("../../../ncpdp/testdata/self_authored/cancel_rx_sample.xml")
	if err != nil {
		t.Fatalf("failed to read real sample fixture: %v", err)
	}

	parser, err := NewFromSchemaDir("../../../ncpdp/schemas/script_2017071")
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
