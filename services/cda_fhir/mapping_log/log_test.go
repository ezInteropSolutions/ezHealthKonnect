package mappinglog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLogBuilder_Build_Summary(t *testing.T) {
	lb := NewLogBuilder("doc-001")
	lb.AddSection(SectionLog{SectionKey: "medications", EntriesIn: 3, ResourcesOut: 3})
	lb.AddSection(SectionLog{SectionKey: "allergiesAndIntolerances", EntriesIn: 1, ResourcesOut: 1})
	lb.AddAssemblyEvent(AssemblyEvent{Rule: "deduplication", Action: "deduplicated"})
	lb.AddAssemblyEvent(AssemblyEvent{Rule: "bp_panel_synthesis", Action: "synthesized"})
	lb.SetTimings(250, 180, 30, 40)

	ml := lb.Build(42)

	if ml.DocumentID != "doc-001" {
		t.Errorf("documentId: got %q", ml.DocumentID)
	}
	if ml.Summary.TotalResources != 42 {
		t.Errorf("totalResources: got %d", ml.Summary.TotalResources)
	}
	if ml.Summary.TotalTimeMs != 250 {
		t.Errorf("totalTimeMs: got %d", ml.Summary.TotalTimeMs)
	}
	if ml.Summary.DeduplicatedCount != 1 {
		t.Errorf("deduplicatedCount: got %d", ml.Summary.DeduplicatedCount)
	}
	if ml.Summary.SynthesizedCount != 1 {
		t.Errorf("synthesizedCount: got %d", ml.Summary.SynthesizedCount)
	}
	if len(ml.Sections) != 2 {
		t.Errorf("sections count: got %d", len(ml.Sections))
	}
}

// TestLogBuilder_Build_PreservesLineageShape locks in the non-debug-mode shape
// guarantee: AssemblyEvent.Lineage must be present in the marshaled JSON when
// populated, and ABSENT (not "lineage":null) when nil -- so every mapping log
// persisted before deep lineage existed, and every one produced with the flag
// off, is byte-identical in shape to a log with the field's zero value.
func TestLogBuilder_Build_PreservesLineageShape(t *testing.T) {
	lb := NewLogBuilder("doc-001")
	lb.AddAssemblyEvent(AssemblyEvent{
		Rule:   "deduplication",
		Action: "deduplicated",
		Lineage: map[string]ResourceLineage{
			"Practitioner/practitioner-3": {SectionKey: "careTeam", EntryIndex: 2, CDAIds: []string{"2.16:123"}},
		},
	})
	lb.AddAssemblyEvent(AssemblyEvent{
		Rule:   "bp_panel_synthesis",
		Action: "synthesized",
		// Lineage intentionally left nil — DeepLineage was off.
	})

	ml := lb.Build(1)
	out, err := json.Marshal(ml)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if !strings.Contains(string(out), `"lineage":{"Practitioner/practitioner-3"`) {
		t.Errorf("expected populated lineage key in JSON, got: %s", out)
	}

	var decoded MappingLog
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.Assembly[1].Lineage != nil {
		t.Errorf("expected nil Lineage for the second event, got %v", decoded.Assembly[1].Lineage)
	}

	// Re-marshal just the nil-Lineage event in isolation and confirm the
	// "lineage" key is absent entirely (omitempty), not present as null.
	soloOut, err := json.Marshal(decoded.Assembly[1])
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(soloOut), "lineage") {
		t.Errorf("expected \"lineage\" key to be entirely absent when nil, got: %s", soloOut)
	}
}

func TestSectionBuilder_Build(t *testing.T) {
	sb := NewSectionBuilder("medications", "Medications", 5)
	sb.AddError("test error")

	// Brief pause so processingTimeMs > 0.
	time.Sleep(time.Millisecond)

	sl := sb.Build(4)
	if sl.SectionKey != "medications" {
		t.Errorf("sectionKey: got %q", sl.SectionKey)
	}
	if sl.Title != "Medications" {
		t.Errorf("title: got %q", sl.Title)
	}
	if sl.EntriesIn != 5 {
		t.Errorf("entriesIn: got %d", sl.EntriesIn)
	}
	if sl.ResourcesOut != 4 {
		t.Errorf("resourcesOut: got %d", sl.ResourcesOut)
	}
	if sl.ProcessingTimeMs < 0 {
		t.Errorf("processingTimeMs should be >= 0, got %d", sl.ProcessingTimeMs)
	}
	if len(sl.Errors) != 1 || sl.Errors[0] != "test error" {
		t.Errorf("errors: got %v", sl.Errors)
	}
}

func TestSectionBuilder_Build_Warnings(t *testing.T) {
	sb := NewSectionBuilder("functionalStatus", "Functional Status", 14)
	sb.AddWarning("entry 9, value: entry has more than one <value>; only the first was mapped")
	sb.AddWarning("entry 4, value: entry has more than one <value>; only the first was mapped")

	sl := sb.Build(19)
	if len(sl.Warnings) != 2 {
		t.Fatalf("warnings: got %d, want 2", len(sl.Warnings))
	}
	if sl.Warnings[0] != "entry 9, value: entry has more than one <value>; only the first was mapped" {
		t.Errorf("warnings[0]: got %q", sl.Warnings[0])
	}
	// A section with only warnings (no AddError calls) must not populate Errors.
	if len(sl.Errors) != 0 {
		t.Errorf("errors: got %v, want none (warnings must not leak into Errors)", sl.Errors)
	}
}

func TestSectionBuilder_Build_NoWarnings_OmitsField(t *testing.T) {
	sb := NewSectionBuilder("allergiesAndIntolerances", "Allergies", 3)
	sl := sb.Build(3)
	if sl.Warnings != nil {
		t.Errorf("warnings: got %v, want nil", sl.Warnings)
	}
}
