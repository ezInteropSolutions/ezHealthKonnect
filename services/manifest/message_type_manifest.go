// services/manifest/message_type_manifest.go
//
// IG-driven resource manifest registry.
//
// Each entry declares which FHIR resource types the HL7 V2-to-FHIR
// Implementation Guide (https://build.fhir.org/ig/HL7/v2-to-fhir/) defines as
// valid outputs for a given HL7 message type.  The FilterByManifest pipeline
// stage uses this registry to remove resources assembled from segment mappings
// that are not permitted by the IG for the active message type.
//
// User control: interface_message_mappings.custom_mapping_config may carry a
// "resource_policy" object that overrides the default per resource type:
//
//	{
//	  "resource_policy": {
//	    "Encounter": "allow",    // include even though IG excludes it
//	    "Coverage":  "suppress"  // exclude even though IG permits it
//	  }
//	}
//
// To extend: add an entry to the registry map below and list any segment
// processors the message type requires in SegmentProcessors.
package manifest

import "strings"

// ResourcePolicy controls whether a resource type is produced.
type ResourcePolicy int

const (
	// PolicyIG follows the IG spec — the default when no interface override exists.
	PolicyIG ResourcePolicy = iota
	// PolicyAllow forces the resource into the output even when the IG excludes it.
	PolicyAllow
	// PolicySuppress removes the resource even when the IG permits it.
	PolicySuppress
)

// MessageTypeManifest is the IG contract for one HL7 message type.
type MessageTypeManifest struct {
	// AllowedResources lists every FHIR resourceType the IG defines as a valid
	// output.  MessageHeader is always implicitly allowed.
	AllowedResources []string
	// SegmentProcessors names the SegmentProcessor implementations that must run
	// for this message type (resolved via the SegmentProcessorRegistry).
	SegmentProcessors []string
	// FocusTypes lists the resource types that belong in MessageHeader.focus for
	// this message type.  The pipeline uses this after segment processors run to
	// ensure every instance of each type (including processor-added resources) has
	// a focus reference — without any per-processor MessageHeader logic.
	FocusTypes []string
}

// allowedSet returns AllowedResources as a fast O(1) lookup map.
// MessageHeader is always included regardless of the manifest entry.
func (m *MessageTypeManifest) allowedSet() map[string]bool {
	s := make(map[string]bool, len(m.AllowedResources)+1)
	s["MessageHeader"] = true
	for _, r := range m.AllowedResources {
		s[r] = true
	}
	return s
}

// registry is the package-level IG manifest keyed by event code
// (the part after "^": "A01" for "ADT^A01", "R01" for "ORU^R01", etc.).
//
// FocusTypes declares which FHIR resource types belong in MessageHeader.focus
// for each message type.  The post-processor focus-augmentation pass uses this
// to ensure every instance of each type (including processor-added resources
// like the prior Patient in merge events) has a focus reference.
var registry = map[string]*MessageTypeManifest{

	// ── ADT — Patient Administration ─────────────────────────────────────────

	"A01": {
		AllowedResources: []string{
			"Patient", "Encounter", "RelatedPerson", "Coverage",
			"Condition", "Procedure", "AllergyIntolerance", "Observation",
		},
		FocusTypes: []string{"Patient", "Encounter"},
	},
	"A02": {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A03": {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A04": {
		AllowedResources: []string{"Patient", "Encounter", "RelatedPerson", "Coverage", "Condition"},
		FocusTypes:       []string{"Patient", "Encounter"},
	},
	"A05": {
		AllowedResources: []string{"Patient", "Encounter", "RelatedPerson", "Coverage"},
		FocusTypes:       []string{"Patient", "Encounter"},
	},
	"A06": {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A07": {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A08": {
		AllowedResources: []string{"Patient", "Encounter", "RelatedPerson", "Coverage", "Condition"},
		FocusTypes:       []string{"Patient", "Encounter"},
	},
	"A09":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A10":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A11":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A12":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A13":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A14":  {AllowedResources: []string{"Patient", "Encounter", "RelatedPerson", "Coverage"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A15":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A16":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A17":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A18":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A20":  {AllowedResources: []string{"Location"}, FocusTypes: []string{"Location"}},
	"A21":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A22":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A23":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A24":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A25":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A26":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A27":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	// A28/A31: demographic-only updates — IG defines no Encounter
	"A28":  {AllowedResources: []string{"Patient", "RelatedPerson", "Coverage"}, FocusTypes: []string{"Patient"}},
	"A29":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A31":  {AllowedResources: []string{"Patient", "RelatedPerson", "Coverage"}, FocusTypes: []string{"Patient"}},
	"A32":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A33":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A34":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A35":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A36":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A37":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A38":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	// A39–A43: merge events — both surviving and prior Patient must be in focus.
	// MRGProcessor adds the prior Patient; FocusTypes ensures the augmentation
	// pass picks up all Patient instances regardless of insertion order.
	"A39": {
		AllowedResources:  []string{"Patient"},
		SegmentProcessors: []string{"MRGProcessor"},
		FocusTypes:        []string{"Patient"},
	},
	"A40": {
		AllowedResources:  []string{"Patient"},
		SegmentProcessors: []string{"MRGProcessor"},
		FocusTypes:        []string{"Patient"},
	},
	"A41": {
		AllowedResources:  []string{"Patient"},
		SegmentProcessors: []string{"MRGProcessor"},
		FocusTypes:        []string{"Patient"},
	},
	"A42": {
		AllowedResources:  []string{"Patient", "Encounter"},
		SegmentProcessors: []string{"MRGProcessor"},
		FocusTypes:        []string{"Patient", "Encounter"},
	},
	"A43": {
		AllowedResources:  []string{"Patient"},
		SegmentProcessors: []string{"MRGProcessor"},
		FocusTypes:        []string{"Patient"},
	},
	"A44":  {AllowedResources: []string{"Patient", "Account"}, FocusTypes: []string{"Patient"}},
	"A45":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A46":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A47":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A48":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A49":  {AllowedResources: []string{"Patient"}, FocusTypes: []string{"Patient"}},
	"A50":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A51":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A52":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A53":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A54":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A55":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A60":  {AllowedResources: []string{"Patient", "Encounter", "AllergyIntolerance"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A61":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},
	"A62":  {AllowedResources: []string{"Patient", "Encounter"}, FocusTypes: []string{"Patient", "Encounter"}},

	// ── ORU — Observation Results ─────────────────────────────────────────────
	"R01": {
		AllowedResources: []string{
			"Patient", "Encounter", "Observation", "DiagnosticReport",
			"Practitioner", "Specimen",
		},
		FocusTypes: []string{"DiagnosticReport"},
	},

	// ── ORM / OMG — Orders ───────────────────────────────────────────────────
	"O01": {
		AllowedResources: []string{"Patient", "Encounter", "ServiceRequest", "Practitioner"},
		FocusTypes:       []string{"ServiceRequest"},
	},
	"O19": {
		AllowedResources: []string{"Patient", "Encounter", "ServiceRequest", "Practitioner", "Observation"},
		FocusTypes:       []string{"ServiceRequest"},
	},

	// ── SIU — Scheduling ─────────────────────────────────────────────────────
	"S12": {AllowedResources: []string{"Patient", "Appointment", "Practitioner", "Location"}, FocusTypes: []string{"Appointment"}},
	"S13": {AllowedResources: []string{"Patient", "Appointment", "Practitioner", "Location"}, FocusTypes: []string{"Appointment"}},
	"S14": {AllowedResources: []string{"Patient", "Appointment", "Practitioner", "Location"}, FocusTypes: []string{"Appointment"}},
	"S15": {AllowedResources: []string{"Patient", "Appointment"}, FocusTypes: []string{"Appointment"}},
	"S17": {AllowedResources: []string{"Patient", "Appointment"}, FocusTypes: []string{"Appointment"}},
	"S26": {AllowedResources: []string{"Patient", "Appointment"}, FocusTypes: []string{"Appointment"}},

	// ── MDM — Medical Document Management ────────────────────────────────────
	"T01": {AllowedResources: []string{"Patient", "Encounter", "DocumentReference", "Practitioner"}, FocusTypes: []string{"DocumentReference"}},
	"T02": {AllowedResources: []string{"Patient", "Encounter", "DocumentReference", "Practitioner"}, FocusTypes: []string{"DocumentReference"}},
	"T11": {AllowedResources: []string{"Patient", "Encounter", "DocumentReference", "Practitioner"}, FocusTypes: []string{"DocumentReference"}},

	// ── VXU — Vaccination ────────────────────────────────────────────────────
	"V04": {AllowedResources: []string{"Patient", "Immunization", "Practitioner"}, FocusTypes: []string{"Immunization"}},

	// ── MFN — Master File Notification ───────────────────────────────────────
	"M02": {AllowedResources: []string{"Practitioner"}, FocusTypes: []string{"Practitioner"}},
	"M05": {AllowedResources: []string{"Location"}, FocusTypes: []string{"Location"}},
	"M15": {AllowedResources: []string{"Substance"}, FocusTypes: []string{"Substance"}},

	// ── DFT — Detailed Financial Transaction ─────────────────────────────────
	"P03": {AllowedResources: []string{"Patient", "Encounter", "Claim", "ChargeItem"}, FocusTypes: []string{"Patient", "Encounter"}},
}

// Lookup returns the manifest for a message type string (e.g. "ADT^A40").
// The event code (after "^") is used as the registry key.
// Returns nil when the type is not registered — callers treat nil as open policy.
func Lookup(messageType string) *MessageTypeManifest {
	key := messageType
	if idx := strings.Index(messageType, "^"); idx >= 0 {
		key = messageType[idx+1:]
	}
	return registry[key]
}

// FilterResources removes resource types not permitted by the IG for messageType
// and applies any interface-level policy overrides.
//
// resourcePolicy is decoded from the "resource_policy" key in
// interface_message_mappings.custom_mapping_config.  Pass nil or an empty map
// to use the IG defaults with no overrides.
//
// Returns the filtered slice and the list of resource types that were removed.
func FilterResources(
	resources []map[string]interface{},
	messageType string,
	resourcePolicy map[string]string,
) (filtered []map[string]interface{}, removed []string) {
	m := Lookup(messageType)
	if m == nil {
		return resources, nil // no manifest → open policy, pass everything
	}

	allowed := m.allowedSet()

	for rt, policy := range resourcePolicy {
		switch policy {
		case "allow":
			allowed[rt] = true
		case "suppress":
			delete(allowed, rt)
		}
	}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if allowed[rt] {
			filtered = append(filtered, r)
		} else {
			removed = append(removed, rt)
		}
	}
	return filtered, removed
}

// SegmentProcessorNames returns the processor names registered for messageType.
func SegmentProcessorNames(messageType string) []string {
	m := Lookup(messageType)
	if m == nil {
		return nil
	}
	return m.SegmentProcessors
}

// GetFocusTypes returns the resource types that belong in MessageHeader.focus
// for messageType.  Returns nil when the type is not registered.
func GetFocusTypes(messageType string) []string {
	m := Lookup(messageType)
	if m == nil {
		return nil
	}
	return m.FocusTypes
}
