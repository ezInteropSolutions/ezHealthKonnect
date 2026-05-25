// services/manifest/optional_segments.go
//
// Registry of optional segment→resource mapping blocks.
//
// These are pre-built mapping sets that are OFF by default but can be toggled
// ON per interface via the mapping editor UI.  They exist because some segment→
// resource mappings are permitted by the IG for a given message type but are not
// produced by the standard OOB template:
//
//   PV1_Encounter  — ORU^R01 has PV1 but doesn't create Encounter by default.
//                    Sites that need visit context on lab results can enable this.
//   SPM_Specimen   — ORU^R01 SPM→Specimen is permitted by the IG but omitted by
//                    the OOB template since most EHRs don't send SPM in ORU.
//   ORC_Practitioner — ORC.12 ordering provider as Practitioner, linked to DR.
//
// Storage: the enabled set is persisted in
//   interface_message_mappings.custom_mapping_config["optional_segments"]
// as a flat map[string]bool keyed by block Key.
//
// Backend wire-up: applyOptionalSegmentBlocks() in hl7_fhir_transform_service_v3.go
// reads the enabled set and calls the corresponding assembler in hl7assembly/.
//
// To add a new block: add an entry to optionalRegistry and implement an assembly
// function in services/hl7assembly/ — no other files need to change.
package manifest

import (
	"encoding/json"
	"strings"
)

// OptionalSegmentBlock describes one optional mapping set.
type OptionalSegmentBlock struct {
	// Key is the machine-readable identifier stored in custom_mapping_config.
	Key string
	// DisplayName is shown in the UI toggle list.
	DisplayName string
	// Description is shown as help text next to the toggle.
	Description string
	// AppliesTo lists the event codes this block is available for.
	// Use []string{"*"} to make the block available for all message types.
	AppliesTo []string
	// DefaultOn is true when the block runs even without an explicit toggle
	// (reserved for future use — all current blocks default to false).
	DefaultOn bool
}

// optionalRegistry is the complete catalog of optional segment blocks.
var optionalRegistry = []OptionalSegmentBlock{
	{
		Key:         "PV1_Encounter",
		DisplayName: "Encounter from PV1",
		Description: "Maps PV1 visit information (class, period, location, attending provider) to an Encounter resource. PV1 is present in most ORU messages but Encounter is not produced by default — enable this when downstream systems require visit context on lab results.",
		AppliesTo:   []string{"R01", "T01", "T02", "T11"},
		DefaultOn:   false,
	},
	{
		Key:         "SPM_Specimen",
		DisplayName: "Specimen from SPM",
		Description: "Maps SPM segment fields (type, collection date, body site, condition) to a Specimen resource linked to DiagnosticReport. Enable when the receiving system requires specimen provenance.",
		AppliesTo:   []string{"R01"},
		DefaultOn:   false,
	},
	{
		Key:         "ORC_OrderingPractitioner",
		DisplayName: "Ordering Provider from ORC",
		Description: "Adds ORC.12 (ordering provider) as a Practitioner and links it to DiagnosticReport.basedOn. Enable when the ordering clinician is distinct from the results interpreter and must appear in the FHIR output.",
		AppliesTo:   []string{"R01", "O01", "O19"},
		DefaultOn:   false,
	},
}

// GetAvailableBlocks returns the optional blocks that apply to messageType.
// messageType may be in "TYPE^EVENT" format (e.g. "ORU^R01") or event-code
// only (e.g. "R01") — both are handled.
func GetAvailableBlocks(messageType string) []OptionalSegmentBlock {
	key := messageType
	if idx := strings.Index(messageType, "^"); idx >= 0 {
		key = messageType[idx+1:]
	}
	var out []OptionalSegmentBlock
	for _, b := range optionalRegistry {
		for _, t := range b.AppliesTo {
			if t == "*" || t == key {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

// LoadOptionalSegmentConfig parses the optional_segments map from the raw
// custom_mapping_config JSON blob.  Returns nil when the key is absent or the
// JSON cannot be decoded — callers treat nil as "no blocks enabled".
func LoadOptionalSegmentConfig(configBytes []byte) map[string]bool {
	if len(configBytes) == 0 {
		return nil
	}
	var config struct {
		OptionalSegments map[string]bool `json:"optional_segments"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil
	}
	return config.OptionalSegments
}

// IsBlockEnabled reports whether blockKey is enabled in the decoded optional
// segment config.  A block whose Key is in the registry with DefaultOn = true
// is always considered enabled regardless of the stored config.
func IsBlockEnabled(optSegments map[string]bool, blockKey string) bool {
	if optSegments[blockKey] {
		return true
	}
	// Check DefaultOn fallback
	for _, b := range optionalRegistry {
		if b.Key == blockKey {
			return b.DefaultOn
		}
	}
	return false
}
