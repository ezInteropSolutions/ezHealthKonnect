// services/ai/rag_reservation.go
//
// Guarantees product documentation a share of every unfiltered retrieval.
//
// Operational chunks (one per configured interface/pipeline) outnumber the
// documentation by a wide margin on a busy install — 75% of the dev knowledge
// base on 2026-10-19 — and because each describes a real configuration they
// look superficially similar to almost any "how do I…" question. The release
// review found answers to "can I connect an RS232 device that sends HL7" and
// "how does a payer authenticate to the eligibility endpoint" built entirely
// from unrelated pipeline configs while the capabilities guide that answers
// both was never retrieved. A reservation tops the result up with the best
// documentation chunks, replacing the lowest-ranked non-documentation ones.
package ai

import (
	"context"
	"sort"
)

// retrievalReservation reserves up to Slots of the top-K for SourceTypes.
type retrievalReservation struct {
	SourceTypes []string
	Slots       int
}

// defaultDocReservation reserves documentation slots: prose docs (capabilities
// guide, runbook, architecture) and per-step pipeline documentation.
var defaultDocReservation = retrievalReservation{
	SourceTypes: []string{"app_docs", "pipeline_step_docs"},
	Slots:       2,
}

// allowedTypes returns the reserved source types the caller's filter permits
// (nil filter = all permitted).
func (res retrievalReservation) allowedTypes(filter []string) []string {
	if len(filter) == 0 {
		return res.SourceTypes
	}
	permitted := make(map[string]bool, len(filter))
	for _, f := range filter {
		permitted[f] = true
	}
	var out []string
	for _, t := range res.SourceTypes {
		if permitted[t] {
			out = append(out, t)
		}
	}
	return out
}

// slotsFor caps the reservation at half the result (rounded up), so it never
// crowds out the rest of the retrieval on a small top-K.
func (res retrievalReservation) slotsFor(topK int) int {
	half := (topK + 1) / 2
	if res.Slots < half {
		return res.Slots
	}
	return half
}

// reserveSlots tops primary up with reserved-type chunks fetched by fetch
// (best first). Returns primary unchanged when the reservation is already met
// or nothing qualifies.
func (res retrievalReservation) reserveSlots(
	ctx context.Context,
	primary []RetrievedChunk,
	topK int,
	filter []string,
	fetch func(ctx context.Context, limit int, types []string) ([]RetrievedChunk, error),
) []RetrievedChunk {
	types := res.allowedTypes(filter)
	slots := res.slotsFor(topK)
	if len(types) == 0 || slots <= 0 {
		return primary
	}
	isReserved := make(map[string]bool, len(types))
	for _, t := range types {
		isReserved[t] = true
	}
	have := 0
	present := make(map[string]bool, len(primary))
	for _, c := range primary {
		present[c.ID] = true
		if isReserved[c.SourceType] {
			have++
		}
	}
	need := slots - have
	if need <= 0 {
		return primary
	}

	candidates, err := fetch(ctx, slots+have, types)
	if err != nil {
		return primary // non-fatal: keep the primary answer
	}
	var extra []RetrievedChunk
	for _, c := range candidates {
		if len(extra) == need {
			break
		}
		if c.ID != "" && !present[c.ID] {
			extra = append(extra, c)
		}
	}
	return mergeReserved(primary, extra, topK, isReserved)
}

// mergeReserved drops the lowest-ranked non-reserved chunks to make room for
// extra, then orders the result by similarity (Retrieve's contract — the
// confidence guardrail reads chunks[0]).
func mergeReserved(primary, extra []RetrievedChunk, topK int, isReserved map[string]bool) []RetrievedChunk {
	if len(extra) == 0 {
		return primary
	}
	out := append([]RetrievedChunk(nil), primary...)
	for len(out)+len(extra) > topK {
		dropped := false
		for i := len(out) - 1; i >= 0; i-- {
			if !isReserved[out[i].SourceType] {
				out = append(out[:i], out[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}
	out = append(out, extra...)
	if len(out) > topK {
		out = out[:topK]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	return out
}
