// services/cda_coverage/element_classifier.go
//
// Classifies an element-level path (report.go's ElementGap.Path, inventory.
// go's walkEntryElements convention) as CDA-infrastructure/provenance noise
// vs a genuine clinical field. Generic by construction -- three small,
// section-agnostic tag sets rooted in CDA's own RIM vocabulary, never a
// per-section allowlist (the same "irrespective of what section it is"
// principle the element-level tracking feature itself was built on).
package cdacoverage

import "strings"

// administrativeWrapperTags are CDA RIM Participation tags -- WHO was
// involved in recording/performing an act (author, performer, informant,
// participant, dataEnterer, custodian), never a clinical fact about the
// patient. Everything nested under one of these (assignedAuthor/
// assignedEntity/representedOrganization/addr/telecom/name/id/...) is
// identity/provenance about that participant, not the act's own clinical
// content -- excluded as a whole subtree regardless of how deep it goes or
// which section it's in.
var administrativeWrapperTags = map[string]bool{
	"author":      true,
	"performer":   true,
	"informant":   true,
	"participant": true,
	"dataEnterer": true,
	"custodian":   true,
}

// administrativeLeafTags are pure CDA-specification plumbing -- present on
// nearly every element for schema/versioning reasons, never carrying
// clinical meaning themselves. Deliberately does NOT include "id": a
// clinical/business identifier (e.g. an accession number, an order ID) is
// something clinical users do care about, unlike templateId/nullFlavor/
// realmCode which exist purely for CDA conformance machinery.
var administrativeLeafTags = map[string]bool{
	"templateId": true,
	"nullFlavor": true,
	"realmCode":  true,
}

// structuralContainerTags are CDA act-wrapper elements -- navigation to a
// nested act, not a fact in their own right. Their OWN clinical fields
// (code/value/statusCode/effectiveTime/...) are separately walked as their
// children and stay clinical; only the bare wrapper node itself (e.g.
// "entryRelationship[0].observation[0]" appearing as its own item, distinct
// from ".observation[0].code[0]") is excluded.
//
// "patient", "location", "healthCareFacility" are the same pattern found in
// the document-header translation (CDAHeader.Patient/CDAEncounter.Location/
// CDALocation.HealthCareFacility, cda_element_translation_table.go): each is
// a pure single-purpose navigation node reached through exactly one parent
// (patientRole.patient, encounter.location, location.healthCareFacility) --
// no rule ever reads the wrapper's own value, only its children -- so the
// bare wrapper item was always "missed", a false gap (e.g.
// "recordTarget[0].patientRole[0].patient[0]" reported as a clinical miss
// even though every one of patient's own children was correctly evaluated
// for coverage separately).
var structuralContainerTags = map[string]bool{
	"component":               true,
	"entry":                   true,
	"entryRelationship":       true,
	"act":                     true,
	"observation":             true,
	"organizer":               true,
	"substanceAdministration": true,
	"procedure":               true,
	"encounter":                true,
	"supply":                   true,
	"patient":                  true,
	"location":                 true,
	"healthCareFacility":       true,
}


// isAdministrativeElementPath reports whether path (an entry-relative
// element path in the index-every-segment convention shared by
// cda_element_translation.go and inventory.go's walkEntryElements) is
// CDA-infrastructure/provenance noise rather than a genuine clinical field.
func isAdministrativeElementPath(path string) bool {
	segments := strings.Split(path, ".")
	last := len(segments) - 1
	for i, seg := range segments {
		tag := elementTagName(seg)
		if administrativeWrapperTags[tag] {
			return true
		}
		if i == last && (administrativeLeafTags[tag] || structuralContainerTags[tag]) {
			return true
		}
	}
	return false
}

// elementTagName strips a path segment's "[idx]" suffix, e.g.
// "entryRelationship[0]" -> "entryRelationship".
func elementTagName(segment string) string {
	if br := strings.IndexByte(segment, '['); br >= 0 {
		return segment[:br]
	}
	return segment
}
