// services/parsers/cda/cda_section_registry.go
// SectionProcessor interface, SectionResult, and SectionRegistry.
//
// Each section processor (allergies, medications, etc.) lives in its own file
// in this package and self-registers via init():
//
//   func init() { RegisterSection(&allergiesProcessor{}) }
//
// CDAParserService uses DefaultSectionRegistry() so callers never need to
// enumerate processors manually.

package cdaparser

import (
	"sort"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// SectionProcessor extracts structured entries and the XHTML narrative block
// from one CDA <section> element. Each implementation handles one clinical
// section (allergies, medications, problems, etc.).
type SectionProcessor interface {
	// SectionKey returns the schema key this processor handles.
	// Must match CDASectionDef.Key in cda/schemas/ccda_2_1.json.
	SectionKey() string

	// Process extracts entries and narrative from the section element.
	// schema is the matching CDASectionDef and may be nil if no schema was found.
	// Never returns nil — populate Error on failure.
	Process(sectionEl *etree.Element, schema *cdaSchema.CDASectionDef) *SectionResult
}

// SectionResult is the output of one SectionProcessor.Process call.
type SectionResult struct {
	SectionKey    string                   `json:"sectionKey"`
	NarrativeHTML string                   `json:"narrativeHTML,omitempty"`
	Entries       []map[string]interface{} `json:"entries"`
	EntryCount    int                      `json:"entryCount"`
	Error         string                   `json:"error,omitempty"`
}

// =====================================
// Registry
// =====================================

// SectionRegistry maps section keys to their processors.
// Construct with NewSectionRegistry; safe for concurrent reads after loading.
type SectionRegistry struct {
	processors map[string]SectionProcessor
}

// NewSectionRegistry creates an empty registry.
func NewSectionRegistry() *SectionRegistry {
	return &SectionRegistry{processors: make(map[string]SectionProcessor)}
}

// Register adds or replaces the processor for its section key.
func (r *SectionRegistry) Register(p SectionProcessor) {
	r.processors[p.SectionKey()] = p
}

// Get returns the processor for the given section key.
// Returns (nil, false) if no processor is registered for that key.
func (r *SectionRegistry) Get(key string) (SectionProcessor, bool) {
	p, ok := r.processors[key]
	return p, ok
}

// Keys returns all registered section keys in sorted order.
func (r *SectionRegistry) Keys() []string {
	keys := make([]string, 0, len(r.processors))
	for k := range r.processors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Len returns the number of registered processors.
func (r *SectionRegistry) Len() int { return len(r.processors) }

// =====================================
// Package-level default registry
// =====================================

// defaultRegistry is the registry populated by section processor init() calls.
var defaultRegistry = NewSectionRegistry()

// DefaultSectionRegistry returns the package-level registry.
// All section processor files in this package register into it via init().
func DefaultSectionRegistry() *SectionRegistry { return defaultRegistry }

// RegisterSection is the convenience wrapper called by each section processor's init().
func RegisterSection(p SectionProcessor) { defaultRegistry.Register(p) }
