// Package mappinglog records CDA→FHIR transformation traceability: per-section
// input/output counts, timing, and post-assembly events (deduplication, synthesis).
// The log is built serially (all goroutines have completed before any write occurs)
// so no mutex is needed. A compact Summary is returned in the HTTP response;
// the full log is written to MongoDB asynchronously.
package mappinglog

import "time"

// MappingLog is the complete per-document transformation trace.
type MappingLog struct {
	DocumentID  string          `json:"documentId"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Sections    []SectionLog    `json:"sections"`
	Assembly    []AssemblyEvent `json:"assembly,omitempty"`
	Summary     Summary         `json:"summary"`
}

// LogBuilder accumulates sections and assembly events, then produces a MappingLog.
// Not thread-safe — call only after all section goroutines have completed.
type LogBuilder struct {
	documentID string
	startedAt  time.Time
	sections   []SectionLog
	assembly   []AssemblyEvent
	timings    logTimings
}

type logTimings struct {
	totalMs    int64
	mappingMs  int64
	assemblyMs int64
	serialMs   int64
}

// NewLogBuilder creates a builder for a document with the given identifier.
func NewLogBuilder(documentID string) *LogBuilder {
	return &LogBuilder{
		documentID: documentID,
		startedAt:  time.Now(),
	}
}

// AddSection appends a completed section log. Call after wg.Wait().
func (b *LogBuilder) AddSection(s SectionLog) {
	b.sections = append(b.sections, s)
}

// AddAssemblyEvent appends one assembly rule event.
func (b *LogBuilder) AddAssemblyEvent(e AssemblyEvent) {
	b.assembly = append(b.assembly, e)
}

// SetTimings records the four timing buckets from the caller.
func (b *LogBuilder) SetTimings(totalMs, mappingMs, assemblyMs, serialMs int64) {
	b.timings = logTimings{
		totalMs:    totalMs,
		mappingMs:  mappingMs,
		assemblyMs: assemblyMs,
		serialMs:   serialMs,
	}
}

// Build assembles and returns the final MappingLog. totalResources is the count
// after deduplication (i.e. what ends up in the bundle).
func (b *LogBuilder) Build(totalResources int) MappingLog {
	deduped, synth := 0, 0
	for _, ev := range b.assembly {
		switch ev.Action {
		case "deduplicated":
			deduped++
		case "synthesized":
			synth++
		}
	}

	return MappingLog{
		DocumentID:  b.documentID,
		GeneratedAt: b.startedAt,
		Sections:    b.sections,
		Assembly:    b.assembly,
		Summary: Summary{
			TotalResources:   totalResources,
			TotalTimeMs:      b.timings.totalMs,
			MappingTimeMs:    b.timings.mappingMs,
			AssemblyTimeMs:   b.timings.assemblyMs,
			SerialTimeMs:     b.timings.serialMs,
			DeduplicatedCount: deduped,
			SynthesizedCount: synth,
		},
	}
}
