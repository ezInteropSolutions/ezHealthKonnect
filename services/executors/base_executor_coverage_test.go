package executors

import (
	"testing"

	"ezhealthkonnect/hl7"
)

// TestGetNestedValue_CoverageAudit_RecordsExactPath proves GetNestedValue's
// tracker hook (added while closing a gap flagged by the user: the far more
// widely-used resolver behind field_mapping/enrichment.database/enrichment.api/
// file_parser/field_validation/fhir_validation/if_then_else/switch_case's own
// condition checks had no hook at all) records the literal path unmodified,
// whether the tracker sits directly on data or nested under data["message"].
func TestGetNestedValue_CoverageAudit_RecordsExactPath(t *testing.T) {
	tracker := NewCDACoverageTracker()
	data := map[string]interface{}{
		"_coverageTracker": tracker,
		"enriched": map[string]interface{}{
			"riskScore": 42,
		},
	}

	got := GetNestedValue(data, "enriched.riskScore")
	if got != 42 {
		t.Fatalf("expected 42, got %v", got)
	}
	if !tracker.Touched("enriched.riskScore") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "enriched.riskScore", tracker.Snapshot())
	}
}

func TestGetNestedValue_CoverageAudit_RecordsThroughMessageWrapper(t *testing.T) {
	tracker := NewCDACoverageTracker()
	wrapped := map[string]interface{}{
		"message": map[string]interface{}{
			"_coverageTracker": tracker,
			"parsedEDI": map[string]interface{}{
				"header": map[string]interface{}{
					"BPR": "value",
				},
			},
		},
	}

	got := GetNestedValue(wrapped, "parsedEDI.header.BPR")
	if got != "value" {
		t.Fatalf("expected \"value\", got %v", got)
	}
	if !tracker.Touched("parsedEDI.header.BPR") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "parsedEDI.header.BPR", tracker.Snapshot())
	}
}

// getHL7FieldValue (the function GetNestedValue's HL7 branch delegates to)
// only understands the TYPED map[string]hl7.EnhancedSegment shape, not a
// generic JSON-shaped map[string]interface{} — a real, pre-existing
// characteristic of this resolver (unlike field_utils.go's
// resolveHL7FieldValue, which handles both shapes), unrelated to the new
// coverage hook. The fixture below matches what this resolver actually needs.
func TestGetNestedValue_CoverageAudit_HL7Path_StillRecordsAndResolves(t *testing.T) {
	tracker := NewCDACoverageTracker()
	data := map[string]interface{}{
		"_coverageTracker": tracker,
		"enhancedSegments": map[string]hl7.EnhancedSegment{
			"PID": {
				Key: "PID",
				Fields: []hl7.FieldInfo{
					{Key: "PID.3", Value: "12345", HasValue: true},
				},
			},
		},
	}

	got := GetNestedValue(data, "PID.3")
	if got != "12345" {
		t.Fatalf("expected \"12345\", got %v", got)
	}
	if !tracker.Touched("PID.3") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "PID.3", tracker.Snapshot())
	}
}

func TestGetNestedValue_CoverageAudit_NoTracker_NoPanic(t *testing.T) {
	data := map[string]interface{}{
		"enriched": map[string]interface{}{"riskScore": 42},
	}
	if got := GetNestedValue(data, "enriched.riskScore"); got != 42 {
		t.Fatalf("expected 42, got %v", got)
	}
}
