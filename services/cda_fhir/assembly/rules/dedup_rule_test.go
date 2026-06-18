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
