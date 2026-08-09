// services/executors/cda_coverage_tracker.go
//
// CDACoverageTracker records which entries of a parsed CDA/CCD source
// document were read by any pipeline step during one message's processing —
// the CDA Coverage Audit feature (see architecture note in CLAUDE.md once
// this lands). Entry-level granularity ("was this specific allergy/
// medication/problem ever picked up"), not full-path granularity: the typed
// "document.*" tree and the generic "xml.*" mirror address the same
// underlying data with structurally different path shapes, so tracking raw
// resolved path strings would never line up with an inventory built by
// walking the "xml.*" mirror. Section key + entry index is the coarsest
// granularity that (a) both addressing schemes can agree on and (b) still
// answers the question this feature exists for: which clinical facts were
// never touched by anything downstream.
package executors

import (
	"strconv"
	"sync"
)

// CDACoverageTracker is attached once per message, only when the owning
// interface has opted into CDA Coverage Audit, via the same reserved-key
// idiom "_semantic_index"/"_cdaDocument" already use on the message
// envelope (inputData["message"]["_coverageTracker"]). A nil
// *CDACoverageTracker is the common case (audit disabled) — every method is
// nil-receiver-safe so call sites never need an "if tracker != nil" guard.
type CDACoverageTracker struct {
	mu      sync.Mutex
	touched map[string]struct{}
}

// NewCDACoverageTracker constructs an empty tracker for one message.
func NewCDACoverageTracker() *CDACoverageTracker {
	return &CDACoverageTracker{touched: make(map[string]struct{})}
}

// CDAEntryKey builds the canonical "sectionKey#entryIndex" tracking key. All
// recorders and the report builder's diff must use this exact form so
// tracked and inventoried keys line up.
func CDAEntryKey(sectionKey string, entryIndex int) string {
	return sectionKey + "#" + strconv.Itoa(entryIndex)
}

// Record marks the entry identified by key (see CDAEntryKey) as having been
// read by some pipeline step. Safe to call concurrently — the CDA→FHIR
// declarative mapping engine resolves entries from one goroutine per CDA
// section against a single shared tracker.
func (t *CDACoverageTracker) Record(key string) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.touched[key] = struct{}{}
}

// Touched reports whether key (see CDAEntryKey) was recorded.
func (t *CDACoverageTracker) Touched(key string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.touched[key]
	return ok
}

// Snapshot returns a copy of every key recorded so far. Intended for the
// async coverage-audit worker, which reads this only after the pipeline run
// (and thus all concurrent writers) has finished.
func (t *CDACoverageTracker) Snapshot() map[string]struct{} {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]struct{}, len(t.touched))
	for k := range t.touched {
		out[k] = struct{}{}
	}
	return out
}
