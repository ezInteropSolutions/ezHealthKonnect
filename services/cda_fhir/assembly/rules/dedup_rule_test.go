package rules

import (
	"testing"

	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

func makeResource(rt, id string, root, ext string) map[string]interface{} {
	r := map[string]interface{}{"resourceType": rt, "id": id}
	if root != "" || ext != "" {
		assembly.EmbedIdentityKeys(r, []map[string]interface{}{
			{"root": root, "extension": ext},
		})
	}
	return r
}

func TestDeduplicationRule_NoOp_WhenNoIDs(t *testing.T) {
	r1 := map[string]interface{}{"resourceType": "Condition", "id": "cond-1"}
	r2 := map[string]interface{}{"resourceType": "Condition", "id": "cond-2"}

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{r1, r2},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("test-doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Removed) != 0 {
		t.Errorf("expected no removals, got %v", ctx.Removed)
	}
}

func TestDeduplicationRule_DedupByNPI(t *testing.T) {
	npiRoot := "2.16.840.1.113883.4.6"
	survivor := makeResource("Practitioner", "author-1", npiRoot, "1013027903")
	dup := makeResource("Practitioner", "practitioner-1", npiRoot, "1013027903")

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{survivor, dup},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("test-doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctx.Removed["Practitioner/practitioner-1"] {
		t.Error("expected dup to be marked for removal")
	}
	if ctx.Removed["Practitioner/author-1"] {
		t.Error("survivor should NOT be marked for removal")
	}
	redirect, ok := ctx.DedupRedirects["Practitioner/practitioner-1"]
	if !ok {
		t.Error("expected redirect for dup")
	}
	if redirect != "Practitioner/author-1" {
		t.Errorf("expected redirect to survivor author-1, got %q", redirect)
	}
}

func TestDeduplicationRule_AssemblyEventLogged(t *testing.T) {
	npiRoot := "2.16.840.1.113883.4.6"
	survivor := makeResource("Practitioner", "author-1", npiRoot, "npi-abc")
	dup := makeResource("Practitioner", "practitioner-1", npiRoot, "npi-abc")

	lb := mappinglog.NewLogBuilder("doc-123")
	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{survivor, dup},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            lb,
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	_ = rule.Apply(ctx)

	log := lb.Build(1)
	if len(log.Assembly) != 1 {
		t.Fatalf("expected 1 assembly event, got %d", len(log.Assembly))
	}
	ev := log.Assembly[0]
	if ev.Action != "deduplicated" {
		t.Errorf("expected action=deduplicated, got %q", ev.Action)
	}
	if ev.SurvivorID != "author-1" {
		t.Errorf("expected survivorId=author-1, got %q", ev.SurvivorID)
	}
}

// TestDeduplicationRule_RicherLaterResource_BecomesSurvivor covers the Care
// Team scenario: a sparse Practitioner (NPI only) registers first, a richer
// one (name+telecom) for the SAME person arrives later -- the richer one must
// win even though it wasn't first, and the redirect must point FROM the
// sparse one's ref TO the richer one's ref (not the reverse, which plain
// first-wins would have produced).
func TestDeduplicationRule_RicherLaterResource_BecomesSurvivor(t *testing.T) {
	npiRoot := "2.16.840.1.113883.4.6"
	sparse := makeResource("Practitioner", "practitioner-1", npiRoot, "1013027903")
	rich := makeResource("Practitioner", "practitioner-2", npiRoot, "1013027903")
	rich["name"] = []interface{}{map[string]interface{}{"family": "Damek", "given": []interface{}{"Herman"}}}
	rich["telecom"] = []interface{}{map[string]interface{}{"system": "phone", "value": "+1-303-415-4355"}}

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sparse, rich},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctx.Removed["Practitioner/practitioner-1"] {
		t.Error("expected the sparse, first-registered Practitioner to be removed")
	}
	if ctx.Removed["Practitioner/practitioner-2"] {
		t.Error("the richer, later Practitioner should survive, not be removed")
	}
	if got := ctx.DedupRedirects["Practitioner/practitioner-1"]; got != "Practitioner/practitioner-2" {
		t.Errorf("expected sparse -> rich redirect, got %q", got)
	}
}

// TestDeduplicationRule_RicherResource_RetargetsExistingRedirectChain proves
// the single-hop AssembleEntriesWithRedirects lookup never sees a chain: a
// THIRD resource that already deduped against the sparse survivor must end up
// redirected straight to the final, richest survivor once it's swapped in --
// not still pointing at the now-removed sparse one.
func TestDeduplicationRule_RicherResource_RetargetsExistingRedirectChain(t *testing.T) {
	npiRoot := "2.16.840.1.113883.4.6"
	sparse := makeResource("Practitioner", "practitioner-1", npiRoot, "1013027903")
	alsoSparse := makeResource("Practitioner", "practitioner-2", npiRoot, "1013027903")
	rich := makeResource("Practitioner", "practitioner-3", npiRoot, "1013027903")
	rich["name"] = []interface{}{map[string]interface{}{"family": "Damek"}}

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sparse, alsoSparse, rich},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := ctx.DedupRedirects["Practitioner/practitioner-2"]; got != "Practitioner/practitioner-3" {
		t.Errorf("expected practitioner-2's redirect to be retargeted straight to the final survivor practitioner-3, got %q (a chain through the removed practitioner-1 would break AssembleEntriesWithRedirects' single-hop lookup)", got)
	}
	if got := ctx.DedupRedirects["Practitioner/practitioner-1"]; got != "Practitioner/practitioner-3" {
		t.Errorf("expected practitioner-1's redirect to practitioner-3, got %q", got)
	}
	if !ctx.Removed["Practitioner/practitioner-1"] || !ctx.Removed["Practitioner/practitioner-2"] {
		t.Error("both sparse resources should be removed")
	}
	if ctx.Removed["Practitioner/practitioner-3"] {
		t.Error("the richest resource should survive")
	}
}

func TestDeduplicationRule_SameOrgDifferentType_NotDeduped(t *testing.T) {
	pract := makeResource("Practitioner", "pr-1", "2.16", "same-id")
	org := makeResource("Organization", "org-1", "2.16", "same-id")

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{pract, org},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewDeduplicationRule()
	_ = rule.Apply(ctx)

	if len(ctx.Removed) != 0 {
		t.Errorf("cross-type resources with same ID should NOT be deduped: %v", ctx.Removed)
	}
}
