package enrichment

import (
	"testing"

	"ezhealthkonnect/services/executors"
)

// TestResolveSourceValue_CoverageAudit_LoopItem_RecordsExactPath proves
// resolveSourceValue's tracker hook fires for the loop-item HL7 branch
// (resolveFieldFromLoopItem operates on a single loop row with no tracker
// access of its own — the hook lives at this, its one caller, which has the
// full envelope).
func TestResolveSourceValue_CoverageAudit_LoopItem_RecordsExactPath(t *testing.T) {
	e := NewFieldMappingExecutor()
	tracker := executors.NewCDACoverageTracker()
	inputData := map[string]interface{}{
		"_coverageTracker": tracker,
		"item": map[string]interface{}{
			"key": "IN1",
			"fields": []interface{}{
				map[string]interface{}{"key": "IN1.2", "value": "PPO123"},
			},
		},
	}

	got, err := e.resolveSourceValue("IN1.2", inputData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "PPO123" {
		t.Fatalf("expected \"PPO123\", got %v", got)
	}
	if !tracker.Touched("IN1.2") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "IN1.2", tracker.Snapshot())
	}
}

func TestResolveSourceValue_CoverageAudit_NoTracker_NoPanic(t *testing.T) {
	e := NewFieldMappingExecutor()
	inputData := map[string]interface{}{
		"item": map[string]interface{}{
			"key": "IN1",
			"fields": []interface{}{
				map[string]interface{}{"key": "IN1.2", "value": "PPO123"},
			},
		},
	}
	got, err := e.resolveSourceValue("IN1.2", inputData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "PPO123" {
		t.Fatalf("expected \"PPO123\", got %v", got)
	}
}
