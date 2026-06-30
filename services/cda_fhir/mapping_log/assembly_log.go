package mappinglog

// AssemblyEvent records one action taken by an assembly rule after all section
// mappers have completed. Used by DeduplicationRule, BPPanelSynthesisRule, etc.
type AssemblyEvent struct {
	Rule         string   `json:"rule"`
	Action       string   `json:"action"`               // "deduplicated" | "synthesized" | "merged"
	ResourceType string   `json:"resourceType"`
	IDs          []string `json:"ids"`                  // FHIR resource IDs involved
	SurvivorID   string   `json:"survivorId,omitempty"` // keeper for deduplicated events
	Detail       string   `json:"detail,omitempty"`

	// Lineage holds per-participant CDA provenance (section + entry index + CDA
	// II identity) for every resource ID listed in IDs, keyed by FHIR resource
	// ID — covering both the survivor and any discarded/merged resource, since
	// the latter's provenance is otherwise lost once it's dropped from the
	// bundle. Populated only when the owning interface has deep lineage enabled
	// (debug_logging/log_level=debug — see services.ResolveInterfaceDebugLevel);
	// nil/omitted otherwise, so non-debug-mode mapping logs are unchanged in
	// shape from before this field existed.
	Lineage map[string]ResourceLineage `json:"lineage,omitempty"`
}

// ResourceLineage is the CDA-side provenance of one FHIR resource at the moment
// an assembly rule consumed it (survivor or discarded/merged). v1 granularity:
// CDA section key + entry index within that section, NOT a true per-XML-element
// XPath — CDA sections are flat and stably identified by templateId/LOINC, but
// individual elements aren't tracked through XML parsing today.
type ResourceLineage struct {
	SectionKey string   `json:"sectionKey,omitempty"`
	EntryIndex int      `json:"entryIndex"`
	CDAIds     []string `json:"cdaIds,omitempty"` // "root:extension" strings
}
