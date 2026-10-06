package control

import (
	"testing"

	"ezhealthkonnect/services/executors"
)

// TestGetNestedValue_CoverageAudit_RecordsExactPath proves this package's own
// getNestedValue (a genuinely separate implementation from
// executors.GetNestedValue, same name, different package) — backing
// if_then_else's copy_field action and switch_case's field/copy_field/
// transform-action reads — records the literal path, checked both when the
// tracker sits directly on data and when nested under data["message"].
func TestGetNestedValue_CoverageAudit_RecordsExactPath(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	data := map[string]interface{}{
		"_coverageTracker": tracker,
		"claim": map[string]interface{}{
			"amount": 100,
		},
	}

	got := getNestedValue(data, "claim.amount")
	if got != 100 {
		t.Fatalf("expected 100, got %v", got)
	}
	if !tracker.Touched("claim.amount") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "claim.amount", tracker.Snapshot())
	}
}

// This package's plain generic-path walk (unlike resolveHL7Field a few lines
// below it) does NOT unwrap a "message"-wrapped envelope on its own — a real,
// pre-existing characteristic of this resolver, not something the coverage
// hook changes. The tracker lookup still checks both shapes defensively (see
// the hook's own doc comment), so this test proves that half using an
// HL7-shaped path, the one case resolveHL7Field itself explicitly unwraps
// "message" for — position is a float64 here (fieldMap["position"].(float64)),
// matching what JSON-deserialized HL7 field maps actually look like in this
// resolver, not the "key"-string shape getHL7FieldValue (a different resolver
// entirely) expects.
func TestGetNestedValue_CoverageAudit_RecordsThroughMessageWrapper(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	wrapped := map[string]interface{}{
		"message": map[string]interface{}{
			"_coverageTracker": tracker,
			"enhancedSegments": map[string]interface{}{
				"PID": map[string]interface{}{
					"fields": []interface{}{
						map[string]interface{}{"position": float64(3), "value": "12345"},
					},
				},
			},
		},
	}

	got := getNestedValue(wrapped, "PID.3")
	if got != "12345" {
		t.Fatalf("expected \"12345\", got %v", got)
	}
	if !tracker.Touched("PID.3") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "PID.3", tracker.Snapshot())
	}
}

func TestGetNestedValue_CoverageAudit_NoTracker_NoPanic(t *testing.T) {
	data := map[string]interface{}{
		"claim": map[string]interface{}{"amount": 100},
	}
	if got := getNestedValue(data, "claim.amount"); got != 100 {
		t.Fatalf("expected 100, got %v", got)
	}
}
