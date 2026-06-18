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
}
