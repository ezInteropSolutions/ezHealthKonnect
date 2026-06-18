package mappinglog

import (
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
