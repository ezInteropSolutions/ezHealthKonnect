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
	// AllowedResources lists the FHIR resourceTypes typically expected for this
	// message type. No longer used to gate FilterResources' output (see its doc
	// comment) — retained solely as the baseline transformation_scorer.go scores
	// quality against.
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
		// OBRProcessor rebuilds all Observations from OBX segments and wires
		// DiagnosticReport.result[], .subject, .encounter, and per-Observation
		// .subject.  It also maps raw HL7 status codes to FHIR values and
		// formats HL7 TS timestamps to FHIR instant.
		SegmentProcessors: []string{"OBRProcessor"},
		FocusTypes:        []string{"DiagnosticReport"},
	},
	// R03 — Unsolicited transmission of requested observation.
	// Structurally identical to R01 (same MSH/PID/ORC/OBR/OBX/NTE layout).
	"R03": {
		AllowedResources: []string{
			"Patient", "Encounter", "Observation", "DiagnosticReport",
			"Practitioner", "Specimen",
		},
		SegmentProcessors: []string{"OBRProcessor"},
		FocusTypes:        []string{"DiagnosticReport"},
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
	// TXAProcessor rebuilds the DocumentReference from TXA (metadata) and OBX
	// (document content) segments and drops any Observation resources the generic
	// template mapper produced from OBX — OBX in MDM is document content, not
	// clinical observations.  MessageHeader.focus is overwritten to point only at
	// the new DocumentReference.
	"T01": {
		AllowedResources:  []string{"Patient", "Encounter", "DocumentReference", "Practitioner"},
		SegmentProcessors: []string{"TXAProcessor"},
		FocusTypes:        []string{"DocumentReference"},
	},
	"T02": {
		AllowedResources:  []string{"Patient", "Encounter", "DocumentReference", "Practitioner"},
		SegmentProcessors: []string{"TXAProcessor"},
		FocusTypes:        []string{"DocumentReference"},
	},
	"T11": {
		AllowedResources:  []string{"Patient", "Encounter", "DocumentReference", "Practitioner"},
		SegmentProcessors: []string{"TXAProcessor"},
		FocusTypes:        []string{"DocumentReference"},
	},

	// ── VXU — Vaccination ────────────────────────────────────────────────────
	"V04": {AllowedResources: []string{"Patient", "Immunization", "Practitioner"}, SegmentProcessors: []string{"VXUProcessor"}, FocusTypes: []string{"Immunization"}},

	// ── MFN — Master File Notification ───────────────────────────────────────
	// Known event codes map 1:1 to a specific master file type and FHIR resource.
	// M01/M13/M14 are user-defined — the assembly dispatches on MFI.1, so
	// AllowedResources must cover all types the assembly can produce.
	"M01": {AllowedResources: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}},
	"M02": {AllowedResources: []string{"Practitioner"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"Practitioner"}},
	"M05": {AllowedResources: []string{"Location"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"Location"}},
	"M06": {AllowedResources: []string{"ChargeItemDefinition"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"ChargeItemDefinition"}},
	// M13/M14 are site-defined — MFI.1 drives the actual FHIR resource type.
	// EMP/INS/PAY/CLN → Organization; STF/PRA → Practitioner; LOC → Location; CDM → ChargeItemDefinition.
	"M13": {AllowedResources: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}},
	"M14": {AllowedResources: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}, SegmentProcessors: []string{"MFNProcessor"}, FocusTypes: []string{"Organization", "Practitioner", "Location", "ChargeItemDefinition"}},
	"M15": {AllowedResources: []string{"Substance"}, FocusTypes: []string{"Substance"}},

	// ── DFT — Detailed Financial Transaction ─────────────────────────────────
	// P03 focus is ChargeItem — the primary artifact of a financial transaction
	// posting. Patient and Encounter are reachable via ChargeItem.subject and
	// ChargeItem.context respectively.
	"P03": {AllowedResources: []string{"Patient", "Encounter", "Claim", "ChargeItem"}, FocusTypes: []string{"ChargeItem"}},
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

// FilterResources applies interface-level policy overrides to the resource set.
//
// A resource only ever reaches here because it was actually built from a
// segment genuinely present in the message via a valid template mapping — the
// template itself IS the IG-derived segment→resource map, so "was it built" is
// already an IG-conformant signal. This function used to ALSO gate output
// against a hand-maintained per-trigger-event AllowedResources allow-list
// (see MessageTypeManifest.AllowedResources), which required manually
// auditing every one of ~60+ HL7 trigger event codes and silently stripped
// correctly-built resources whenever that list was incomplete (e.g. AL1→
// AllergyIntolerance was missing from ADT^A08's list even though AL1 is a
// completely normal segment on real ADT^A08 messages). That gate is removed:
// everything the engine actually built now passes through by default, and
// resourcePolicy remains the only way to exclude a resource type, via an
// explicit interface-level "suppress" override.
//
// AllowedResources / Lookup() are NOT removed — they remain the sole input to
// transformation_scorer.go's quality-scoring baseline ("did this message type
// produce the resources we'd typically expect"), a softer, non-gating signal
// that is intentionally decoupled from output filtering by this change.
//
// resourcePolicy is decoded from the "resource_policy" key in
// interface_message_mappings.custom_mapping_config.  Pass nil or an empty map
// to apply no overrides (identity pass-through).
//
// Returns the filtered slice and the list of resource types that were removed.
func FilterResources(
	resources []map[string]interface{},
	messageType string,
	resourcePolicy map[string]string,
) (filtered []map[string]interface{}, removed []string) {
	suppressed := make(map[string]bool, len(resourcePolicy))
	for rt, policy := range resourcePolicy {
		if policy == "suppress" {
			suppressed[rt] = true
		}
	}

	if len(suppressed) == 0 {
		return resources, nil
	}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if suppressed[rt] {
			removed = append(removed, rt)
		} else {
			filtered = append(filtered, r)
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
