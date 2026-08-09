// services/executors/cda_coverage_tracker_test.go
package executors

import (
	"sync"
	"testing"
)

func TestCDAEntryKey_Format(t *testing.T) {
	if got := CDAEntryKey("medications", 2); got != "medications#2" {
		t.Errorf("CDAEntryKey(medications, 2) = %q, want %q", got, "medications#2")
	}
	if got := CDAEntryKey("problems", 0); got != "problems#0" {
		t.Errorf("CDAEntryKey(problems, 0) = %q, want %q", got, "problems#0")
	}
}

func TestCDACoverageTracker_RecordAndTouched(t *testing.T) {
	tr := NewCDACoverageTracker()
	key := CDAEntryKey("allergiesAndIntolerances", 1)

	if tr.Touched(key) {
		t.Fatal("key should not be touched before Record")
	}
	tr.Record(key)
	if !tr.Touched(key) {
		t.Fatal("key should be touched after Record")
	}
	if tr.Touched(CDAEntryKey("allergiesAndIntolerances", 2)) {
		t.Fatal("a different key must not be reported as touched")
	}
}

func TestCDACoverageTracker_NilSafe(t *testing.T) {
	var tr *CDACoverageTracker // deliberately nil — the common case (audit disabled)

	// None of these must panic.
	tr.Record("medications#0")
	if tr.Touched("medications#0") {
		t.Fatal("a nil tracker must never report anything as touched")
	}
	if snap := tr.Snapshot(); snap != nil {
		t.Fatalf("nil tracker's Snapshot() = %v, want nil", snap)
	}
}

func TestCDACoverageTracker_RecordEmptyKeyIsNoOp(t *testing.T) {
	tr := NewCDACoverageTracker()
	tr.Record("")
	if snap := tr.Snapshot(); len(snap) != 0 {
		t.Fatalf("recording an empty key should not add anything, got %v", snap)
	}
}

func TestCDACoverageTracker_Snapshot_IsIndependentCopy(t *testing.T) {
	tr := NewCDACoverageTracker()
	tr.Record("medications#0")

	snap := tr.Snapshot()
	if _, ok := snap["medications#0"]; !ok {
		t.Fatal("snapshot missing recorded key")
	}

	// Mutating the snapshot must not affect the tracker's own state.
	delete(snap, "medications#0")
	if !tr.Touched("medications#0") {
		t.Fatal("mutating a Snapshot() result should not affect the tracker")
	}
}

// TestCDACoverageTracker_ConcurrentRecord mirrors the real concurrency shape
// this tracker must survive: services/cda_fhir's declarative mapping engine
// spawns one goroutine per CDA section, all sharing a single tracker
// instance (see DeclarativeEngine.CoverageTracker's doc comment).
func TestCDACoverageTracker_ConcurrentRecord(t *testing.T) {
	tr := NewCDACoverageTracker()
	const sections = 20
	const entriesPerSection = 50

	var wg sync.WaitGroup
	for s := 0; s < sections; s++ {
		wg.Add(1)
		go func(sectionIdx int) {
			defer wg.Done()
			sectionKey := "section" + string(rune('a'+sectionIdx))
			for e := 0; e < entriesPerSection; e++ {
				tr.Record(CDAEntryKey(sectionKey, e))
			}
		}(s)
	}
	wg.Wait()

	snap := tr.Snapshot()
	if len(snap) != sections*entriesPerSection {
		t.Fatalf("got %d recorded keys, want %d (a race would drop or corrupt entries)", len(snap), sections*entriesPerSection)
	}
}
