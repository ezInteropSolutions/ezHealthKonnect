package executors

import (
	"testing"

	"github.com/dop251/goja"
)

// runScriptAgainst wraps data in a coverage-tracking object, exposes it as
// "input" in a fresh goja VM, runs script, and returns the tracker's snapshot
// (as a set of keys, for easy membership assertions) alongside the script's
// own result.
func runScriptAgainst(t *testing.T, data map[string]interface{}, script string) (map[string]struct{}, goja.Value) {
	t.Helper()
	vm := goja.New()
	tracker := NewCDACoverageTracker()
	wrapped := NewCoverageTrackingObject(vm, data, tracker)
	if wrapped == nil {
		t.Fatalf("NewCoverageTrackingObject returned nil for non-nil tracker")
	}
	if err := vm.Set("input", wrapped); err != nil {
		t.Fatalf("vm.Set(input): %v", err)
	}
	val, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	return tracker.Snapshot(), val
}

func hasKey(snap map[string]struct{}, key string) bool {
	_, ok := snap[key]
	return ok
}

func TestCoverageTrackingObject_LeafRead_RecordsFullPath(t *testing.T) {
	data := map[string]interface{}{
		"header": map[string]interface{}{
			"BPR": map[string]interface{}{
				"paymentEffectiveDate": "20260101",
			},
		},
	}
	snap, val := runScriptAgainst(t, data, `input.header.BPR.paymentEffectiveDate`)

	if val.Export() != "20260101" {
		t.Fatalf("expected script to read the real value, got %v", val.Export())
	}
	for _, want := range []string{"header", "header.BPR", "header.BPR.paymentEffectiveDate"} {
		if !hasKey(snap, want) {
			t.Errorf("expected key %q to be recorded, snapshot=%v", want, snap)
		}
	}
}

func TestCoverageTrackingObject_JSONStringify_MarksWholeSubtreeCovered(t *testing.T) {
	data := map[string]interface{}{
		"claim": map[string]interface{}{
			"amount":  100,
			"payerId": "PAYER1",
		},
	}
	snap, _ := runScriptAgainst(t, data, `JSON.stringify(input.claim)`)

	for _, want := range []string{"claim", "claim.amount", "claim.payerId"} {
		if !hasKey(snap, want) {
			t.Errorf("expected JSON.stringify to record %q (goja's stringify calls Get per key), snapshot=%v", want, snap)
		}
	}
}

func TestCoverageTrackingObject_ObjectKeysAlone_DoesNotMarkChildrenCovered(t *testing.T) {
	data := map[string]interface{}{
		"claim": map[string]interface{}{
			"amount":  100,
			"payerId": "PAYER1",
		},
	}
	snap, _ := runScriptAgainst(t, data, `Object.keys(input.claim)`)

	if !hasKey(snap, "claim") {
		t.Errorf("expected reading input.claim itself to be recorded, snapshot=%v", snap)
	}
	for _, notWant := range []string{"claim.amount", "claim.payerId"} {
		if hasKey(snap, notWant) {
			t.Errorf("Object.keys() alone must NOT record child key %q (no value was read), snapshot=%v", notWant, snap)
		}
	}
}

func TestCoverageTrackingObject_Array_RecordsIndexedPaths(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"code": "A"},
			map[string]interface{}{"code": "B"},
		},
	}
	snap, val := runScriptAgainst(t, data, `input.items[1].code`)

	if val.Export() != "B" {
		t.Fatalf("expected script to read the real value, got %v", val.Export())
	}
	for _, want := range []string{"items", "items[1]", "items[1].code"} {
		if !hasKey(snap, want) {
			t.Errorf("expected key %q to be recorded, snapshot=%v", want, snap)
		}
	}
	if hasKey(snap, "items[0]") || hasKey(snap, "items[0].code") {
		t.Errorf("index 0 was never read by the script and must not be recorded, snapshot=%v", snap)
	}
}

func TestCoverageTrackingObject_UnreadField_NeverRecorded(t *testing.T) {
	data := map[string]interface{}{
		"header": map[string]interface{}{
			"BPR": map[string]interface{}{
				"paymentEffectiveDate": "20260101",
				"payerName":            "ACME",
			},
		},
	}
	snap, _ := runScriptAgainst(t, data, `input.header.BPR.paymentEffectiveDate`)

	if hasKey(snap, "header.BPR.payerName") {
		t.Errorf("payerName was never read by the script and must not be recorded, snapshot=%v", snap)
	}
}

// TestCoverageTrackingObject_NativeGoSlice_RecordsIndexedPaths is the
// regression guard for a real, live-verification-caught bug (found
// 2026-09-27, EDI 835's own "Derive 835 Claim Context" script): a repeating
// structure produced directly by a Go-native parser mid-pipeline (never
// round-tripped through JSON) is genuinely []map[string]interface{}, NOT
// []interface{} — confirmed directly in edi/loop_engine.go, whose
// matchLoops builds every repeating loop's value this way. wrap()'s original
// type switch had no case for this shape, so it fell into the untracked
// `default` branch (goja's own plain reflection wrapping) — meaning every
// read beneath a repeating EDI loop (every claim's own CLP/NM1/SVC/CAS
// fields) was completely invisible to coverage tracking, even though the
// script's own data extraction worked perfectly (goja's reflection wrapping
// reads a []map[string]interface{} just fine on its own — it just was never
// instrumented). This is the SAME native-Go-value distinction
// field_utils.go's own asInterfaceSlice already exists to paper over for
// resolveJSONPathValue's callers; this test proves the goja wrapper now
// handles it too.
func TestCoverageTrackingObject_NativeGoSlice_RecordsIndexedPaths(t *testing.T) {
	data := map[string]interface{}{
		"loops": map[string]interface{}{
			"2100": []map[string]interface{}{
				{"CLP": map[string]interface{}{"claimPaymentAmount": "100.00"}},
				{"CLP": map[string]interface{}{"claimPaymentAmount": "200.00"}},
			},
		},
	}
	snap, val := runScriptAgainst(t, data, `input.loops["2100"][1].CLP.claimPaymentAmount`)

	if val.Export() != "200.00" {
		t.Fatalf("expected script to read the real value, got %v", val.Export())
	}
	for _, want := range []string{
		"loops", "loops.2100", "loops.2100[1]", "loops.2100[1].CLP", "loops.2100[1].CLP.claimPaymentAmount",
	} {
		if !hasKey(snap, want) {
			t.Errorf("expected key %q to be recorded for a native []map[string]interface{} value, snapshot=%v", want, snap)
		}
	}
	if hasKey(snap, "loops.2100[0]") || hasKey(snap, "loops.2100[0].CLP") {
		t.Errorf("index 0 was never read by the script and must not be recorded, snapshot=%v", snap)
	}
}

func TestNewCoverageTrackingObject_NilTracker_ReturnsNil(t *testing.T) {
	vm := goja.New()
	if v := NewCoverageTrackingObject(vm, map[string]interface{}{"a": 1}, nil); v != nil {
		t.Errorf("expected nil for a nil tracker, got %v", v)
	}
}
