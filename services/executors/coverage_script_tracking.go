// services/executors/coverage_script_tracking.go
//
// Closes Coverage Audit's biggest blind spot: EDI 837P claims, NCPDP renewal/
// dispense responses, and NCPDP Telecom B1 claim rows are all flattened by an
// "enrichment.script" (goja JS) step before fhir.build ever reads them — a
// plain vm.Set("input", inputData) exposes the Go map via goja's default
// reflection-based wrapping, which is completely opaque to Go-level
// instrumentation (no function call happens per property read). This file
// wraps the message sub-object being handed to a script in a goja
// DynamicObject/DynamicArray pair that records every property/index read —
// leaf or container — onto the same CDACoverageTracker resolveJSONPathValue
// and resolveHL7FieldValue already record into (field_utils.go), using the
// identical dotted/bracketed path convention, so a script's reads and a
// direct fhir.build sourcePath's reads land in the exact same key space.
//
// Deliberately opt-in and zero-cost when disabled: script_enrichment_executor.go
// only calls NewCoverageTrackingObject when a tracker is actually present on
// the message envelope; every other pipeline run keeps using the plain,
// unwrapped vm.Set("input", inputData) path unchanged.
package executors

import (
	"fmt"

	"github.com/dop251/goja"
)

// coverageScriptWrapper holds the two things every recursively-wrapped node
// needs: the runtime (to box scalars via vm.ToValue and to construct further
// DynamicObject/DynamicArray wrappers for nested containers) and the tracker
// every read is recorded into.
type coverageScriptWrapper struct {
	vm      *goja.Runtime
	tracker *CDACoverageTracker
}

// wrap boxes v for the JS VM: a map or slice becomes a further tracking
// wrapper (rooted at path, so a deeper read composes onto it), anything else
// is boxed the normal, untracked way — there is nothing further to read
// "into" once a caller reaches a scalar.
//
// A real, live-verification-caught bug (found 2026-09-27, EDI 835's own
// "Derive 835 Claim Context" script): repeating structures produced directly
// by a Go-native parser mid-pipeline (never round-tripped through JSON) are
// genuinely []map[string]interface{}, NOT []interface{} — confirmed directly
// in edi/loop_engine.go, whose matchLoops builds every repeating loop's value
// this way — the exact same native-Go-value distinction
// field_utils.go's asInterfaceSlice already exists to paper over for
// resolveJSONPathValue's own callers. This switch's ORIGINAL two cases never
// covered that shape, so a repeating EDI loop value fell into the `default`
// branch — plain, UNTRACKED goja reflection wrapping — meaning every read
// beneath a repeating loop (every claim's own CLP/NM1/SVC/CAS fields, for
// example) was invisible to coverage tracking, even though the script's own
// DATA EXTRACTION worked completely correctly (goja's reflection wrapping
// reads a []map[string]interface{} just fine — it just isn't instrumented).
// This went undiscovered through NCPDP's own Phase 3 (NewRx's body has no
// top-level repeating groups to exercise this) and was only caught by EDI
// 835's real, live, full-pipeline verification — the first enrichment.script
// in this codebase's history to be run with coverage tracking against a
// genuinely repeating, Go-native loop structure.
func (w *coverageScriptWrapper) wrap(v interface{}, path string) goja.Value {
	switch vv := v.(type) {
	case map[string]interface{}:
		return w.vm.NewDynamicObject(&coverageTrackingObject{w: w, data: vv, prefix: path})
	case []interface{}:
		return w.vm.NewDynamicArray(&coverageTrackingArray{w: w, data: vv, prefix: path})
	case []map[string]interface{}:
		asInterfaceSlice := make([]interface{}, len(vv))
		for i, m := range vv {
			asInterfaceSlice[i] = m
		}
		return w.vm.NewDynamicArray(&coverageTrackingArray{w: w, data: asInterfaceSlice, prefix: path})
	default:
		return w.vm.ToValue(v)
	}
}

// coverageTrackingObject is a read-only goja.DynamicObject view over one Go
// map — every Get records the accumulated path (leaf or container) before
// returning the (possibly further-wrapped) value. Property enumeration alone
// (Keys(), i.e. a bare Object.keys()/for...in with no value read) does NOT
// record anything — only actually reading a value counts as "used," matching
// this feature's existing "an ancestor read implicitly covers its children"
// semantics (report.go's isCovered) and its JSON.stringify-marks-the-whole-
// subtree-covered corollary, since goja's own JSON.stringify implementation
// calls Get for every key it serializes.
type coverageTrackingObject struct {
	w      *coverageScriptWrapper
	data   map[string]interface{}
	prefix string
}

func (o *coverageTrackingObject) Get(key string) goja.Value {
	path := joinCoveragePath(o.prefix, key)
	o.w.tracker.Record(path)
	v, ok := o.data[key]
	if !ok {
		return nil
	}
	return o.w.wrap(v, path)
}

func (o *coverageTrackingObject) Set(key string, val goja.Value) bool { return false } // read-only view
func (o *coverageTrackingObject) Has(key string) bool {
	_, ok := o.data[key]
	return ok
}
func (o *coverageTrackingObject) Delete(key string) bool { return false } // read-only view
func (o *coverageTrackingObject) Keys() []string {
	keys := make([]string, 0, len(o.data))
	for k := range o.data {
		keys = append(keys, k)
	}
	return keys
}

// coverageTrackingArray mirrors coverageTrackingObject for a []interface{} —
// see that type's own doc comment for the shared semantics.
type coverageTrackingArray struct {
	w      *coverageScriptWrapper
	data   []interface{}
	prefix string
}

func (a *coverageTrackingArray) Len() int { return len(a.data) }
func (a *coverageTrackingArray) Get(idx int) goja.Value {
	if idx < 0 || idx >= len(a.data) {
		return nil
	}
	path := fmt.Sprintf("%s[%d]", a.prefix, idx)
	a.w.tracker.Record(path)
	return a.w.wrap(a.data[idx], path)
}
func (a *coverageTrackingArray) Set(idx int, val goja.Value) bool { return false } // read-only view
func (a *coverageTrackingArray) SetLen(int) bool                  { return false } // read-only view
func (a *coverageTrackingArray) DeleteIdx(idx int) bool           { return false } // read-only view

func joinCoveragePath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// NewCoverageTrackingObject wraps data (typically a script step's own
// inputData["message"] sub-map — the source message envelope, not the whole
// script inputData, so recorded paths never carry a spurious "message."
// prefix and line up exactly with a direct fhir.build sourcePath's own
// convention) in a goja DynamicObject that records every property read into
// tracker. Returns nil if tracker is nil, so a caller can safely skip the
// wrap-and-swap when coverage tracking isn't enabled for this message.
func NewCoverageTrackingObject(vm *goja.Runtime, data map[string]interface{}, tracker *CDACoverageTracker) goja.Value {
	if tracker == nil || vm == nil {
		return nil
	}
	w := &coverageScriptWrapper{vm: vm, tracker: tracker}
	return vm.NewDynamicObject(&coverageTrackingObject{w: w, data: data, prefix: ""})
}
