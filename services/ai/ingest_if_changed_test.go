package ai

import (
	"context"
	"fmt"
	"testing"
)

// fakeEmbedder satisfies LLMProvider for the one method ingestion uses; any other call panics (nil embedded interface).
type fakeEmbedder struct {
	LLMProvider
	calls int
}

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	f.calls++
	v := make([]float32, 768)
	v[0] = 1
	return v, nil
}

func TestIngestTextIfChanged_SkipsUnchangedAndReplacesChanged(t *testing.T) {
	db := openTestDB(t)
	fe := &fakeEmbedder{}
	es := newEmbeddingServiceWithProvider(db, fe)
	ctx := context.Background()
	ref := fmt.Sprintf("test-ifchanged:%d", fe.calls) // unique enough: removed below
	defer db.Exec(`DELETE FROM ai_knowledge_chunks WHERE source_type = 'test_ifchanged'`)
	db.Exec(`DELETE FROM ai_knowledge_chunks WHERE source_type = 'test_ifchanged'`)

	n, skipped, err := es.IngestTextIfChanged(ctx, "test_ifchanged", ref, "test", "interface alpha, active", nil)
	if err != nil || skipped || n != 1 {
		t.Fatalf("first ingest: n=%d skipped=%v err=%v, want 1 chunk, not skipped", n, skipped, err)
	}
	embedsAfterFirst := fe.calls

	// Same text again (what every restart does): nothing is embedded.
	n, skipped, err = es.IngestTextIfChanged(ctx, "test_ifchanged", ref, "test", "interface alpha, active", nil)
	if err != nil || !skipped || n != 0 || fe.calls != embedsAfterFirst {
		t.Fatalf("unchanged ingest: n=%d skipped=%v err=%v embeds=%d (was %d); want skipped with no embedding", n, skipped, err, fe.calls, embedsAfterFirst)
	}

	// Changed text: replaced, and exactly one set of chunks remains.
	n, skipped, err = es.IngestTextIfChanged(ctx, "test_ifchanged", ref, "test", "interface alpha, INACTIVE", nil)
	if err != nil || skipped || n != 1 {
		t.Fatalf("changed ingest: n=%d skipped=%v err=%v, want 1 chunk, not skipped", n, skipped, err)
	}
	var count int
	var content string
	if err := db.QueryRow(`SELECT count(*), max(content) FROM ai_knowledge_chunks WHERE source_type='test_ifchanged' AND source_ref=$1`, ref).Scan(&count, &content); err != nil {
		t.Fatal(err)
	}
	if count != 1 || content != "interface alpha, INACTIVE" {
		t.Fatalf("after change: %d chunks, content %q; want 1 chunk with the new text", count, content)
	}
}

func TestIngestTextIfChanged_EmbedFailureKeepsOldChunks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	good := newEmbeddingServiceWithProvider(db, &fakeEmbedder{})
	defer db.Exec(`DELETE FROM ai_knowledge_chunks WHERE source_type = 'test_ifchanged2'`)
	if _, _, err := good.IngestTextIfChanged(ctx, "test_ifchanged2", "r", "test", "old text", nil); err != nil {
		t.Fatal(err)
	}
	bad := newEmbeddingServiceWithProvider(db, failingEmbedder{})
	if _, _, err := bad.IngestTextIfChanged(ctx, "test_ifchanged2", "r", "test", "new text", nil); err == nil {
		t.Fatal("expected the embed failure to be reported")
	}
	var content string
	if err := db.QueryRow(`SELECT content FROM ai_knowledge_chunks WHERE source_type='test_ifchanged2' AND source_ref='r'`).Scan(&content); err != nil || content != "old text" {
		t.Fatalf("after a failed re-embed the old chunk must remain; got %q, err %v", content, err)
	}
}

type failingEmbedder struct{ LLMProvider }

func (failingEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, fmt.Errorf("model unavailable")
}

func TestIngestProgress(t *testing.T) {
	var nilP *IngestProgress
	nilP.Begin("x", 1) // nil-safe
	nilP.Step(false)
	nilP.End()
	if nilP.Snapshot().Running {
		t.Fatal("nil progress must report not running")
	}
	p := NewIngestProgress()
	p.Begin("interfaces", 3)
	p.Step(true)
	p.Step(false)
	s := p.Snapshot()
	if !s.Running || s.Phase != "interfaces" || s.Done != 2 || s.Total != 3 || s.Changed != 1 {
		t.Fatalf("snapshot = %+v", s)
	}
	p.End()
	if p.Snapshot().Running {
		t.Fatal("End must stop running")
	}
}
