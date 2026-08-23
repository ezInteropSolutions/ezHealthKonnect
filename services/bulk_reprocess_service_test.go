package services

import (
	"testing"
	"time"
)

func TestNextExplicitIDBatch_ReturnsCorrectSliceAndAdvancesByProcessedCount(t *testing.T) {
	svc := &BulkReprocessService{}
	job := &bulkJob{
		SelectionMode: "ids",
		ExplicitIDs:   []string{"a", "b", "c", "d", "e"},
		BatchSize:     2,
		LastProcessedID: "cursor-unused-for-ids-mode",
	}

	batch, cursor := svc.nextExplicitIDBatch(job)
	if len(batch) != 2 || batch[0] != "a" || batch[1] != "b" {
		t.Fatalf("expected first batch [a b], got %v", batch)
	}
	if cursor != job.LastProcessedID {
		t.Fatalf("ids-mode cursor should pass through unchanged, got %q", cursor)
	}

	job.ProcessedCount = 2
	batch, _ = svc.nextExplicitIDBatch(job)
	if len(batch) != 2 || batch[0] != "c" || batch[1] != "d" {
		t.Fatalf("expected second batch [c d], got %v", batch)
	}

	job.ProcessedCount = 4
	batch, _ = svc.nextExplicitIDBatch(job)
	if len(batch) != 1 || batch[0] != "e" {
		t.Fatalf("expected final partial batch [e], got %v", batch)
	}
}

func TestNextExplicitIDBatch_ExhaustedReturnsEmpty(t *testing.T) {
	svc := &BulkReprocessService{}
	job := &bulkJob{
		SelectionMode:  "ids",
		ExplicitIDs:    []string{"a", "b"},
		BatchSize:      2,
		ProcessedCount: 2,
	}
	batch, _ := svc.nextExplicitIDBatch(job)
	if len(batch) != 0 {
		t.Fatalf("expected empty batch once fully processed, got %v", batch)
	}
}

func TestNextExplicitIDBatch_EmptyExplicitIDsReturnsEmpty(t *testing.T) {
	svc := &BulkReprocessService{}
	job := &bulkJob{SelectionMode: "ids", ExplicitIDs: []string{}, BatchSize: 500}
	batch, _ := svc.nextExplicitIDBatch(job)
	if len(batch) != 0 {
		t.Fatalf("expected empty batch for empty ExplicitIDs, got %v", batch)
	}
}

func TestCapErrorSummary_KeepsOnlyMostRecentEntries(t *testing.T) {
	entries := make([]bulkErrorEntry, 0, bulkErrorSummaryCap+10)
	for i := 0; i < bulkErrorSummaryCap+10; i++ {
		entries = append(entries, bulkErrorEntry{MessageID: string(rune('a' + i%26))})
	}
	capped := capErrorSummary(entries)
	if len(capped) != bulkErrorSummaryCap {
		t.Fatalf("expected exactly %d entries, got %d", bulkErrorSummaryCap, len(capped))
	}
	// The cap must keep the TAIL (most recent), not the head — dropping the oldest
	// failures is the right trade-off since error_summary is best-effort visibility,
	// not the audit trail (that's still the source message's own last_error_message).
	if capped[0] != entries[10] {
		t.Fatalf("expected capped slice to start at the 11th original entry (dropping the oldest 10), got index mismatch")
	}
	if capped[len(capped)-1] != entries[len(entries)-1] {
		t.Fatalf("expected capped slice to end at the last original entry")
	}
}

func TestCapErrorSummary_BelowCapIsUnchanged(t *testing.T) {
	entries := []bulkErrorEntry{{MessageID: "1"}, {MessageID: "2"}}
	capped := capErrorSummary(entries)
	if len(capped) != 2 {
		t.Fatalf("expected untouched slice of 2, got %d", len(capped))
	}
}

func TestStatusSQLMap_CoversEveryStatusFilterOptionInTheUI(t *testing.T) {
	// Mirrors public/messages.html's <select id="filterStatus"> options exactly —
	// this test exists so a new status value added to the UI dropdown without a
	// matching entry here is caught immediately, rather than silently falling
	// through to "condition dropped, filter ignored" in nextFilterBatch.
	uiStatusOptions := []string{"received", "processing", "delivered", "completed", "pending_delivery", "failed"}
	for _, s := range uiStatusOptions {
		if _, ok := statusSQLMap[s]; !ok {
			t.Errorf("statusSQLMap is missing an entry for UI status option %q", s)
		}
	}
	if len(statusSQLMap) != len(uiStatusOptions) {
		t.Errorf("statusSQLMap has %d entries, expected exactly %d to match the UI dropdown — check for a stale or extra entry", len(statusSQLMap), len(uiStatusOptions))
	}
}

// fakeReprocessor lets tests exercise runBatch/submitBlocking without a real
// ProcessingEngine or database — it only needs to satisfy MessageReprocessor.
type fakeReprocessor struct {
	calls []string
}

func (f *fakeReprocessor) ReprocessMessageSync(interfaceID, messageID string) error {
	f.calls = append(f.calls, messageID)
	return nil
}

func TestBulkReprocessService_ImplementsMessageReprocessorInterfaceStructurally(t *testing.T) {
	// Compile-time-ish check that NewBulkReprocessService accepts anything
	// satisfying MessageReprocessor (not just a concrete *processing.ProcessingEngine)
	// — this is the whole point of the interface (see the doc comment on
	// MessageReprocessor: avoids an import cycle with the processing package).
	svc := NewBulkReprocessService(nil, &fakeReprocessor{})
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.pool == nil {
		t.Fatal("expected the shared bulk-reprocess worker pool to be initialized")
	}
}

func TestSubmitBlocking_RetriesUntilPoolAcceptsTheJob(t *testing.T) {
	svc := NewBulkReprocessService(nil, &fakeReprocessor{})
	done := make(chan struct{})
	svc.submitBlocking(func() { close(done) })
	select {
	case <-done:
		// submitted and ran
	case <-time.After(2 * time.Second):
		t.Fatal("submitBlocking never ran the job")
	}
}
