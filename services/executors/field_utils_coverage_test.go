package executors

import "testing"

// TestResolveCoverageTracker_FindsDirectTracker covers the shape a first
// pipeline step's already-flat envelope has (e.g. FHIR-as-inbound-source, or
// any already-unwrapped message map passed straight to GetFieldValue) —
// _coverageTracker sits directly at data's own top level.
func TestResolveCoverageTracker_FindsDirectTracker(t *testing.T) {
	tracker := NewCDACoverageTracker()
	data := map[string]interface{}{"_coverageTracker": tracker, "name": "Doe"}

	got := resolveCoverageTracker(data)
	if got != tracker {
		t.Fatalf("expected the direct tracker to be found, got %v", got)
	}
}

// TestResolveCoverageTracker_FindsMessageWrappedTracker is the regression
// test for the real, previously-latent gap this fix closes: a LATER pipeline
// step's own raw inputData (as executeStepWithContext actually builds it,
// {"message": execCtx.Message, "steps": ..., ...}) has _coverageTracker
// living one level down inside "message", never as a sibling of "steps" at
// inputData's own top level. Every "steps.<alias>.step_output...." sourcePath
// — the dominant address shape real EDI/NCPDP OOB fhir.build configs use,
// since they read a prior enrichment.script/parse step's own output — is
// resolved against exactly this raw, wrapped shape. Before this fix,
// resolveJSONPathValue/resolveHL7FieldValue's tracker check only looked at
// data's own top level and would silently never find the tracker for any
// such path, even though the underlying field VALUE resolves correctly
// (resolvePathParts walks "steps" as a genuine top-level key of the same raw
// data) — found while building NCPDP's own Coverage Audit adapter, but a
// real, format-agnostic gap, not NCPDP-specific (see
// services/cda_coverage/ncpdp_adapter_test.go's own doc comment).
func TestResolveCoverageTracker_FindsMessageWrappedTracker(t *testing.T) {
	tracker := NewCDACoverageTracker()
	inputData := map[string]interface{}{
		"message": map[string]interface{}{
			"_coverageTracker": tracker,
			"resourceType":     "Patient",
		},
		"steps": map[string]interface{}{
			"parse_new_rx": map[string]interface{}{
				"step_output": map[string]interface{}{
					"parsed_ncpdp": map[string]interface{}{
						"body": map[string]interface{}{"pharmacy": "value"},
					},
				},
			},
		},
	}

	got := resolveCoverageTracker(inputData)
	if got != tracker {
		t.Fatalf("expected the message-wrapped tracker to be found via fallback, got %v", got)
	}

	// End-to-end: a real "steps.X.step_output...." sourcePath read via
	// GetFieldValue must both (a) resolve the real value AND (b) record the
	// hit — proving the fix in its actual call context, not just in
	// isolation.
	path := "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy"
	value := GetFieldValue(inputData, path)
	if value != "value" {
		t.Fatalf("expected GetFieldValue to resolve the real value, got %v", value)
	}
	if !tracker.Touched(path) {
		t.Errorf("expected the real GetFieldValue read to have recorded the literal sourcePath as touched")
	}
}

// TestResolveCoverageTracker_NoTrackerEitherShape_ReturnsNil confirms the
// nil-safe, zero-cost-when-disabled contract every other Coverage Audit hook
// in this codebase relies on: no tracker anywhere just means no tracking, not
// a panic or an error.
func TestResolveCoverageTracker_NoTrackerEitherShape_ReturnsNil(t *testing.T) {
	inputData := map[string]interface{}{
		"message": map[string]interface{}{"resourceType": "Patient"},
		"steps":   map[string]interface{}{},
	}
	if got := resolveCoverageTracker(inputData); got != nil {
		t.Fatalf("expected nil when no tracker is attached anywhere, got %v", got)
	}
	if got := resolveCoverageTracker(map[string]interface{}{}); got != nil {
		t.Fatalf("expected nil for a completely empty map, got %v", got)
	}
}

// TestResolveCoverageTracker_DirectTakesPriorityOverMessageWrapped confirms
// the direct check is tried FIRST — the original, pre-existing behavior for
// every already-working case must stay byte-for-byte the same; the
// message-wrapped fallback only ever adds a NEW case, never changes an
// existing one.
func TestResolveCoverageTracker_DirectTakesPriorityOverMessageWrapped(t *testing.T) {
	directTracker := NewCDACoverageTracker()
	messageTracker := NewCDACoverageTracker()
	data := map[string]interface{}{
		"_coverageTracker": directTracker,
		"message":          map[string]interface{}{"_coverageTracker": messageTracker},
	}
	got := resolveCoverageTracker(data)
	if got != directTracker {
		t.Fatalf("expected the direct tracker to win when both are present, got %v", got)
	}
}
