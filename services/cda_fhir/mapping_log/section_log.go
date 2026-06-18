package mappinglog

import "time"

// SectionLog is the per-section transformation trace: how many CDA entries came
// in, how many FHIR resources came out, how long it took, and any errors.
type SectionLog struct {
	SectionKey       string       `json:"sectionKey"`
	Title            string       `json:"title,omitempty"`
	EntriesIn        int          `json:"entriesIn"`
	ResourcesOut     int          `json:"resourcesOut"`
	ProcessingTimeMs int64        `json:"processingTimeMs"`
	Errors           []string     `json:"errors,omitempty"`
	Entries          []EntryTrace `json:"entries,omitempty"`
}

// EntryTrace records which FHIR resource(s) a single CDA entry produced —
// the answer to "which element from which entry was mapped to which FHIR
// resource". One entry can produce zero, one, or several resources (e.g. a
// Care Team organizer entry produces one Practitioner per member plus one
// CareTeam); EntryIndex is the entry's position within its section.
type EntryTrace struct {
	EntryIndex   int      `json:"entryIndex"`
	EntryType    string   `json:"entryType,omitempty"`  // "observation" | "act" | "organizer" | ...
	Code         string   `json:"code,omitempty"`       // entry's own Code.Code, if present
	CodeSystem   string   `json:"codeSystem,omitempty"`
	DisplayName  string   `json:"displayName,omitempty"`
	EntryID      string   `json:"entryId,omitempty"` // "root:extension" of the entry's own first II, if present
	ResourceRefs []string `json:"resourceRefs"`       // e.g. ["Observation/observation-3"], final post-renumbering refs

	// Resources holds direct references to the FHIR resource maps this entry
	// produced, set by the caller (document_mapper.go) at mapping time — before
	// global ID renumbering reassigns "id" on these same map objects in place.
	// ResolveRefs reads the final resourceType/id off these maps afterward.
	// Internal wiring only — never serialized.
	Resources []map[string]interface{} `json:"-"`
}

// ResolveRefs populates ResourceRefs by reading the final resourceType/id off
// each linked resource map. Must be called after global ID renumbering (see
// document_mapper.go) so the refs reflect final, stable bundle IDs.
func (t *EntryTrace) ResolveRefs() {
	t.ResourceRefs = make([]string, 0, len(t.Resources))
	for _, r := range t.Resources {
		rt, _ := r["resourceType"].(string)
		id, _ := r["id"].(string)
		if rt != "" && id != "" {
			t.ResourceRefs = append(t.ResourceRefs, rt+"/"+id)
		}
	}
}

// SectionBuilder accumulates state for one section goroutine and builds a SectionLog.
// Not thread-safe — each goroutine owns its own SectionBuilder (pre-allocated slot).
type SectionBuilder struct {
	sectionKey string
	title      string
	startedAt  time.Time
	errors     []string
	entriesIn  int
	entries    []EntryTrace
}

// NewSectionBuilder creates a builder for a single CDA section.
func NewSectionBuilder(sectionKey, title string, entriesIn int) *SectionBuilder {
	return &SectionBuilder{
		sectionKey: sectionKey,
		title:      title,
		startedAt:  time.Now(),
		entriesIn:  entriesIn,
	}
}

// AddError records a section-level error string.
func (b *SectionBuilder) AddError(err string) {
	b.errors = append(b.errors, err)
}

// AddEntry records the entry-to-resource trace for one CDA entry in this section.
func (b *SectionBuilder) AddEntry(trace EntryTrace) {
	b.entries = append(b.entries, trace)
}

// Build finalises and returns the SectionLog. resourcesOut is the count of FHIR
// resources produced by the mapper (known only after the mapper returns).
func (b *SectionBuilder) Build(resourcesOut int) SectionLog {
	sl := SectionLog{
		SectionKey:       b.sectionKey,
		Title:            b.title,
		EntriesIn:        b.entriesIn,
		ResourcesOut:     resourcesOut,
		ProcessingTimeMs: time.Since(b.startedAt).Milliseconds(),
	}
	if len(b.errors) > 0 {
		sl.Errors = b.errors
	}
	if len(b.entries) > 0 {
		sl.Entries = b.entries
	}
	return sl
}
