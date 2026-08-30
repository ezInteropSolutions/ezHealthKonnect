// services/ai/rag_hybrid_test.go
// Pure unit tests for fuseRRF (Reciprocal Rank Fusion) — no DB required, mirrors
// the pure-unit style of rag_filter_test.go and embedding_service_test.go.
package ai

import "testing"

func TestFuseRRF_DenseOnly_PreservesOrderAndSimilarity(t *testing.T) {
	dense := []RetrievedChunk{
		{ID: "a", Content: "first", Similarity: 0.90},
		{ID: "b", Content: "second", Similarity: 0.70},
	}
	got := fuseRRF(dense, nil, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("expected dense-only order [a b], got [%s %s]", got[0].ID, got[1].ID)
	}
	// Similarity must survive fusion untouched — guardrails.go's isLowConfidence
	// depends on this being a genuine cosine similarity, not a fused/RRF score.
	if got[0].Similarity != 0.90 {
		t.Errorf("expected chunk a's Similarity to survive fusion as 0.90, got %v", got[0].Similarity)
	}
}

func TestFuseRRF_LexicalOnly_PreservesOrderAndSimilarity(t *testing.T) {
	lexical := []RetrievedChunk{
		{ID: "x", Content: "exact match", Similarity: 0.42},
		{ID: "y", Content: "weaker match", Similarity: 0.20},
	}
	got := fuseRRF(nil, lexical, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "x" || got[1].ID != "y" {
		t.Errorf("expected lexical-only order [x y], got [%s %s]", got[0].ID, got[1].ID)
	}
	if got[0].Similarity != 0.42 {
		t.Errorf("expected chunk x's Similarity to survive fusion as 0.42, got %v", got[0].Similarity)
	}
}

func TestFuseRRF_OverlapInBothOutranksSingleListRankOne(t *testing.T) {
	// Chunk "a" is rank-0 in dense only: score = 1/(60+0+1) ≈ 0.016393
	// Chunk "b" is rank-1 in dense AND rank-0 in lexical:
	//   score = 1/(60+1+1) + 1/(60+0+1) ≈ 0.016129 + 0.016393 = 0.032523
	// "b" should outrank "a" despite ranking worse on the dense leg alone — this is
	// the entire point of hybrid retrieval: an exact lexical match can promote a
	// chunk that pure dense search buried.
	dense := []RetrievedChunk{
		{ID: "a", Content: "dense top hit", Similarity: 0.80},
		{ID: "b", Content: "dense second hit", Similarity: 0.60},
	}
	lexical := []RetrievedChunk{
		{ID: "b", Content: "dense second hit", Similarity: 0.60},
	}
	got := fuseRRF(dense, lexical, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "b" {
		t.Errorf("expected chunk b (found by both legs) to rank first, got %s first", got[0].ID)
	}
}

func TestFuseRRF_EmptyBoth_ReturnsEmptyNoPanic(t *testing.T) {
	got := fuseRRF(nil, nil, 5)
	if len(got) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(got))
	}
}

func TestFuseRRF_TruncatesToTopK(t *testing.T) {
	dense := []RetrievedChunk{
		{ID: "a", Similarity: 0.9},
		{ID: "b", Similarity: 0.8},
		{ID: "c", Similarity: 0.7},
		{ID: "d", Similarity: 0.6},
	}
	got := fuseRRF(dense, nil, 2)
	if len(got) != 2 {
		t.Fatalf("expected fusion to truncate to topK=2, got %d chunks", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("expected top-2 by fused score [a b], got [%s %s]", got[0].ID, got[1].ID)
	}
}

func TestFuseRRF_SkipsChunksWithEmptyID(t *testing.T) {
	// A chunk with no ID can't be deduplicated/fused safely — fuseRRF must skip it
	// rather than panic or silently corrupt the ranking.
	dense := []RetrievedChunk{
		{ID: "", Content: "no id — should be skipped"},
		{ID: "a", Content: "has id", Similarity: 0.5},
	}
	got := fuseRRF(dense, nil, 5)
	if len(got) != 1 {
		t.Fatalf("expected empty-ID chunk to be skipped, got %d chunks", len(got))
	}
	if got[0].ID != "a" {
		t.Errorf("expected surviving chunk to be 'a', got %q", got[0].ID)
	}
}
