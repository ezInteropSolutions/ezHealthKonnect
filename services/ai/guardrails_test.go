// services/ai/guardrails_test.go
// Regression coverage for isLowConfidence — the fix for a confidence signal
// that used to only fire when retrieval returned zero chunks, which almost
// never happens in practice since the similarity search always returns its
// topK nearest rows regardless of how weak the actual match is.
package ai

import "testing"

func TestIsLowConfidence_NoChunksAtAll(t *testing.T) {
	if !isLowConfidence(nil) {
		t.Error("no chunks at all should be low confidence")
	}
	if !isLowConfidence([]RetrievedChunk{}) {
		t.Error("empty chunk slice should be low confidence")
	}
}

func TestIsLowConfidence_WeakBestMatch(t *testing.T) {
	// Empirically measured "clearly unrelated question" similarity from this
	// deployment's real data (~0.49-0.50) — see the comment on
	// minRelevantSimilarity for the full calibration.
	chunks := []RetrievedChunk{
		{SourceRef: "fhir.to_cda", Similarity: 0.4961},
		{SourceRef: "cda.normalize", Similarity: 0.4890},
	}
	if !isLowConfidence(chunks) {
		t.Error("a weak best-match similarity should be low confidence, even with multiple chunks returned")
	}
}

func TestIsLowConfidence_StrongBestMatch(t *testing.T) {
	// Empirically measured "well-covered question" similarity (~0.71-0.74).
	chunks := []RetrievedChunk{
		{SourceRef: "hl7.build", Similarity: 0.7100},
		{SourceRef: "payload.builder", Similarity: 0.6854},
	}
	if isLowConfidence(chunks) {
		t.Error("a strong best-match similarity should not be flagged as low confidence")
	}
}

func TestIsLowConfidence_OnlyChecksTheBestMatch(t *testing.T) {
	// A weak first result still counts as low confidence even if a later
	// (less similar, since results are ordered descending) entry happens to
	// be stronger — that ordering assumption shouldn't silently invert.
	chunks := []RetrievedChunk{
		{SourceRef: "weak", Similarity: 0.40},
		{SourceRef: "irrelevant-but-higher", Similarity: 0.90},
	}
	if !isLowConfidence(chunks) {
		t.Error("isLowConfidence must key off chunks[0], not the max across all chunks")
	}
}

func TestIsLowConfidence_ThresholdBoundary(t *testing.T) {
	atFloor := []RetrievedChunk{{Similarity: minRelevantSimilarity}}
	if isLowConfidence(atFloor) {
		t.Errorf("similarity exactly at the floor (%v) should count as confident, not low", minRelevantSimilarity)
	}
	justBelow := []RetrievedChunk{{Similarity: minRelevantSimilarity - 0.001}}
	if !isLowConfidence(justBelow) {
		t.Errorf("similarity just below the floor (%v) should count as low confidence", minRelevantSimilarity)
	}
}
