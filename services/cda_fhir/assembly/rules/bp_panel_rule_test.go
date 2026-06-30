package rules

import (
	"testing"

	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

func makeBPObs(id, loincCode, subject string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Observation",
		"id":           id,
		"status":       "final",
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": loincSystem,
					"code":   loincCode,
				},
			},
		},
		"subject":           map[string]interface{}{"reference": subject},
		"effectiveDateTime": "2024-01-15T10:30:00-05:00",
		"valueQuantity": map[string]interface{}{
			"value": 120.0,
			"unit":  "mm[Hg]",
		},
	}
}

func TestBPPanelSynthesisRule_SynthesizesPair(t *testing.T) {
	sys := makeBPObs("obs-1", bpSystolicLOINC, "urn:uuid:patient-uuid")
	dia := makeBPObs("obs-2", bpDiastolicLOINC, "urn:uuid:patient-uuid")

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ctx.Removed["Observation/obs-1"] {
		t.Error("systolic should be marked for removal")
	}
	if !ctx.Removed["Observation/obs-2"] {
		t.Error("diastolic should be marked for removal")
	}

	// A new panel resource should have been appended.
	if len(ctx.Resources) != 3 {
		t.Fatalf("expected 3 resources (2 originals + 1 panel), got %d", len(ctx.Resources))
	}
	panel := ctx.Resources[2]
	if panel["resourceType"] != "Observation" {
		t.Errorf("panel resourceType should be Observation, got %v", panel["resourceType"])
	}
	panelCode := firstCodingCode(panel)
	if panelCode != bpPanelLOINC {
		t.Errorf("panel LOINC should be %s, got %s", bpPanelLOINC, panelCode)
	}

	comps, ok := panel["component"].([]interface{})
	if !ok || len(comps) != 2 {
		t.Errorf("panel should have 2 components, got %v", panel["component"])
	}

	// Redirects should point both original obs IDs → panel ID.
	panelID, _ := panel["id"].(string)
	if ctx.DedupRedirects["Observation/obs-1"] != "Observation/"+panelID {
		t.Errorf("sys redirect wrong: %s", ctx.DedupRedirects["Observation/obs-1"])
	}
	if ctx.DedupRedirects["Observation/obs-2"] != "Observation/"+panelID {
		t.Errorf("dia redirect wrong: %s", ctx.DedupRedirects["Observation/obs-2"])
	}
}

func TestBPPanelSynthesisRule_PerformerCopiedFromSystolic(t *testing.T) {
	// Real data loss found auditing the 99397 sample's Vital Signs section:
	// Systolic and Diastolic share the same /author (both components of one
	// Vital Signs Organizer), but extractComponent only ever copies
	// code+value[x] and buildBPPanel never set performer on the synthesized
	// panel -- the performer vanished once the two were merged into 85354-9.
	sys := makeBPObs("obs-1", bpSystolicLOINC, "urn:uuid:patient-uuid")
	sys["performer"] = []interface{}{map[string]interface{}{"reference": "PractitionerRole/role-1"}}
	dia := makeBPObs("obs-2", bpDiastolicLOINC, "urn:uuid:patient-uuid")
	dia["performer"] = []interface{}{map[string]interface{}{"reference": "PractitionerRole/role-1"}}

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	panel := ctx.Resources[2]
	perf, ok := panel["performer"].([]interface{})
	if !ok || len(perf) != 1 {
		t.Fatalf("expected panel.performer to be copied from systolic, got %v", panel["performer"])
	}
	ref, _ := perf[0].(map[string]interface{})
	if ref["reference"] != "PractitionerRole/role-1" {
		t.Errorf("panel.performer[0].reference = %v, want PractitionerRole/role-1", ref["reference"])
	}
}

func TestBPPanelSynthesisRule_NoPerformer_WhenNeitherComponentHasOne(t *testing.T) {
	sys := makeBPObs("obs-1", bpSystolicLOINC, "urn:uuid:patient-uuid")
	dia := makeBPObs("obs-2", bpDiastolicLOINC, "urn:uuid:patient-uuid")

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	panel := ctx.Resources[2]
	if _, has := panel["performer"]; has {
		t.Errorf("expected no performer key when neither source component has one, got %v", panel["performer"])
	}
}

func TestBPPanelSynthesisRule_NoSynthesis_DifferentSubject(t *testing.T) {
	sys := makeBPObs("obs-sys", bpSystolicLOINC, "urn:uuid:patient-A")
	dia := makeBPObs("obs-dia", bpDiastolicLOINC, "urn:uuid:patient-B")

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	_ = rule.Apply(ctx)

	if len(ctx.Removed) != 0 {
		t.Error("should not remove observations with different subjects")
	}
	if len(ctx.Resources) != 2 {
		t.Errorf("should not add new resources, got %d", len(ctx.Resources))
	}
}

func TestBPPanelSynthesisRule_NoOp_WhenNoMatches(t *testing.T) {
	cond := map[string]interface{}{"resourceType": "Condition", "id": "cond-1"}
	obs := makeBPObs("obs-other", "8302-2", "urn:uuid:patient-A") // height, not BP

	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{cond, obs},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	_ = rule.Apply(ctx)

	if len(ctx.Removed) != 0 || len(ctx.Resources) != 2 {
		t.Error("rule should be a no-op when no BP candidates found")
	}
}

func TestBPPanelSynthesisRule_LineageCaptured_WhenDeepLineageOn(t *testing.T) {
	sys := makeBPObs("sys-1", bpSystolicLOINC, "urn:uuid:pat")
	sys["_cdaSection"] = "vitalSigns"
	sys["_cdaEntryIndex"] = 2
	dia := makeBPObs("dia-1", bpDiastolicLOINC, "urn:uuid:pat")
	dia["_cdaSection"] = "vitalSigns"
	dia["_cdaEntryIndex"] = 3

	lb := mappinglog.NewLogBuilder("doc")
	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            lb,
		Config:         assembly.AssemblyConfig{DeepLineage: true},
	}

	rule := NewBPPanelSynthesisRule()
	if err := rule.Apply(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	log := lb.Build(1)
	if len(log.Assembly) != 1 {
		t.Fatalf("expected 1 assembly event, got %d", len(log.Assembly))
	}
	ev := log.Assembly[0]
	if len(ev.Lineage) != 2 {
		t.Fatalf("expected lineage for both component observations, got %d: %v", len(ev.Lineage), ev.Lineage)
	}
	sysLineage, ok := ev.Lineage["sys-1"]
	if !ok || sysLineage.EntryIndex != 2 {
		t.Errorf("unexpected systolic lineage: ok=%v %+v", ok, sysLineage)
	}
	diaLineage, ok := ev.Lineage["dia-1"]
	if !ok || diaLineage.EntryIndex != 3 {
		t.Errorf("unexpected diastolic lineage: ok=%v %+v", ok, diaLineage)
	}
	panelID := ev.SurvivorID
	if _, ok := ev.Lineage[panelID]; ok {
		t.Errorf("synthesized panel %q has no CDA origin of its own and should be absent from Lineage", panelID)
	}
}

func TestBPPanelSynthesisRule_LineageAbsent_WhenDeepLineageOff(t *testing.T) {
	sys := makeBPObs("sys-1", bpSystolicLOINC, "urn:uuid:pat")
	dia := makeBPObs("dia-1", bpDiastolicLOINC, "urn:uuid:pat")

	lb := mappinglog.NewLogBuilder("doc")
	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            lb,
		Config:         assembly.AssemblyConfig{}, // DeepLineage: false (zero value)
	}

	rule := NewBPPanelSynthesisRule()
	_ = rule.Apply(ctx)

	log := lb.Build(1)
	if len(log.Assembly) != 1 {
		t.Fatalf("expected 1 assembly event, got %d", len(log.Assembly))
	}
	if log.Assembly[0].Lineage != nil {
		t.Errorf("expected Lineage to be nil when DeepLineage is off, got %v", log.Assembly[0].Lineage)
	}
}

func TestBPPanelSynthesisRule_SynthesisEvent_Logged(t *testing.T) {
	sys := makeBPObs("sys-1", bpSystolicLOINC, "urn:uuid:pat")
	dia := makeBPObs("dia-1", bpDiastolicLOINC, "urn:uuid:pat")

	lb := mappinglog.NewLogBuilder("doc")
	ctx := &assembly.AssemblyContext{
		Resources:      []map[string]interface{}{sys, dia},
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            lb,
		Config:         assembly.AssemblyConfig{},
	}

	rule := NewBPPanelSynthesisRule()
	_ = rule.Apply(ctx)

	log := lb.Build(1)
	if len(log.Assembly) != 1 {
		t.Fatalf("expected 1 assembly event, got %d", len(log.Assembly))
	}
	ev := log.Assembly[0]
	if ev.Action != "synthesized" {
		t.Errorf("expected action=synthesized, got %q", ev.Action)
	}
	if ev.Rule != "bp_panel_synthesis" {
		t.Errorf("expected rule=bp_panel_synthesis, got %q", ev.Rule)
	}
}
