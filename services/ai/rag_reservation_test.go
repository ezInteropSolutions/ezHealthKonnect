package ai

import (
	"context"
	"errors"
	"testing"
)

func chunk(id, st string, sim float64) RetrievedChunk {
	return RetrievedChunk{ID: id, SourceType: st, Similarity: sim}
}

func ids(cs []RetrievedChunk) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func staticFetch(cs ...RetrievedChunk) func(context.Context, int, []string) ([]RetrievedChunk, error) {
	return func(_ context.Context, limit int, _ []string) ([]RetrievedChunk, error) {
		if limit < len(cs) {
			return cs[:limit], nil
		}
		return cs, nil
	}
}

func TestReservation_ReplacesLowestOperationalChunks(t *testing.T) {
	primary := []RetrievedChunk{
		chunk("op1", "operational", 0.60), chunk("op2", "operational", 0.58),
		chunk("op3", "operational", 0.57), chunk("op4", "operational", 0.50),
	}
	got := defaultDocReservation.reserveSlots(context.Background(), primary, 4, nil,
		staticFetch(chunk("doc1", "app_docs", 0.59), chunk("doc2", "pipeline_step_docs", 0.52)))

	want := []string{"op1", "doc1", "op2", "doc2"}
	if g := ids(got); len(g) != 4 || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] || g[3] != want[3] {
		t.Fatalf("got %v, want %v (op3/op4 dropped, ordered by similarity)", g, want)
	}
}

func TestReservation_AlreadyMet_Unchanged(t *testing.T) {
	primary := []RetrievedChunk{chunk("d1", "app_docs", 0.7), chunk("d2", "app_docs", 0.6), chunk("o1", "operational", 0.5)}
	called := false
	fetch := func(context.Context, int, []string) ([]RetrievedChunk, error) { called = true; return nil, nil }
	got := defaultDocReservation.reserveSlots(context.Background(), primary, 3, nil, fetch)
	if called || len(got) != 3 || got[2].ID != "o1" {
		t.Fatalf("reservation already met must not fetch or reorder; called=%v got=%v", called, ids(got))
	}
}

func TestReservation_SkipsDuplicatesAndKeepsPrimaryOnFetchError(t *testing.T) {
	primary := []RetrievedChunk{chunk("d1", "app_docs", 0.7), chunk("o1", "operational", 0.6), chunk("o2", "operational", 0.5)}
	got := defaultDocReservation.reserveSlots(context.Background(), primary, 3, nil,
		staticFetch(chunk("d1", "app_docs", 0.7), chunk("d2", "app_docs", 0.4)))
	if g := ids(got); g[0] != "d1" || g[1] != "o1" || g[2] != "d2" {
		t.Fatalf("got %v, want [d1 o1 d2]", g)
	}

	failing := func(context.Context, int, []string) ([]RetrievedChunk, error) { return nil, errors.New("db down") }
	if got := defaultDocReservation.reserveSlots(context.Background(), primary, 3, nil, failing); len(got) != 3 || got[2].ID != "o2" {
		t.Fatalf("fetch error must keep the primary result, got %v", ids(got))
	}
}

func TestReservation_RespectsCallerFilter(t *testing.T) {
	called := false
	fetch := func(context.Context, int, []string) ([]RetrievedChunk, error) { called = true; return nil, nil }
	defaultDocReservation.reserveSlots(context.Background(), nil, 4, []string{"hl7_v2", "fhir_r4"}, fetch)
	if called {
		t.Fatal("a filter excluding every documentation type must not fetch documentation")
	}
	if got := defaultDocReservation.allowedTypes([]string{"hl7_v2", "app_docs"}); len(got) != 1 || got[0] != "app_docs" {
		t.Fatalf("allowedTypes = %v, want [app_docs]", got)
	}
}

func TestReservation_SlotsCappedAtHalfOfTopK(t *testing.T) {
	cases := map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 10: 2}
	for topK, want := range cases {
		if got := defaultDocReservation.slotsFor(topK); got != want {
			t.Errorf("slotsFor(%d) = %d, want %d", topK, got, want)
		}
	}
	if (retrievalReservation{}).slotsFor(4) != 0 {
		t.Error("zero-value reservation must be a no-op")
	}
}
