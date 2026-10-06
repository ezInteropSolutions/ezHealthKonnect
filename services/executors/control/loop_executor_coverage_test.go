package control

import (
	"testing"

	"ezhealthkonnect/services/executors"
)

// TestLoopExecutor_GetFieldValue_CoverageAudit_RecordsExactPath proves
// LoopExecutor's own minimal getFieldValue (backing control.loop's "while"
// condition check — the 5th, separate resolver in this codebase, no HL7/
// bracket support at all) records the literal path.
func TestLoopExecutor_GetFieldValue_CoverageAudit_RecordsExactPath(t *testing.T) {
	e := NewLoopExecutor()
	tracker := executors.NewCDACoverageTracker()
	data := map[string]interface{}{
		"_coverageTracker": tracker,
		"counter":          map[string]interface{}{"value": 3},
	}

	got := e.getFieldValue(data, "counter.value")
	if got != 3 {
		t.Fatalf("expected 3, got %v", got)
	}
	if !tracker.Touched("counter.value") {
		t.Errorf("expected exact key %q to be recorded, snapshot=%v", "counter.value", tracker.Snapshot())
	}
}

// getFieldValue has no "message"-unwrapping support at all (a plain, minimal
// dotted-path walker — confirmed by reading it directly, not assumed) —
// unlike every other resolver in this codebase, a wrapped envelope's fields
// genuinely cannot resolve through it. The tracker lookup still checks both
// shapes defensively (in case a future caller passes the wrapped envelope
// directly), so this proves that half works even though value resolution
// itself correctly returns nil for a path this resolver can't see into.
func TestLoopExecutor_GetFieldValue_CoverageAudit_RecordsThroughMessageWrapper(t *testing.T) {
	e := NewLoopExecutor()
	tracker := executors.NewCDACoverageTracker()
	wrapped := map[string]interface{}{
		"message": map[string]interface{}{
			"_coverageTracker": tracker,
			"counter":          map[string]interface{}{"value": 3},
		},
	}

	got := e.getFieldValue(wrapped, "counter.value")
	if got != nil {
		t.Fatalf("expected nil (this resolver can't see into a wrapped envelope), got %v", got)
	}
	if !tracker.Touched("counter.value") {
		t.Errorf("expected exact key %q to be recorded even though resolution itself can't reach it, snapshot=%v", "counter.value", tracker.Snapshot())
	}
}

func TestLoopExecutor_GetFieldValue_CoverageAudit_NoTracker_NoPanic(t *testing.T) {
	e := NewLoopExecutor()
	data := map[string]interface{}{"counter": map[string]interface{}{"value": 3}}
	if got := e.getFieldValue(data, "counter.value"); got != 3 {
		t.Fatalf("expected 3, got %v", got)
	}
}
