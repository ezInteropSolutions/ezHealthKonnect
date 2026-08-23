// services/fhir_narrative/allergy_narrative.go
// Generates XHTML narrative for a FHIR R4 AllergyIntolerance resource.
// USCDI v3 class: Allergies and Intolerances.

package fhirnarrative

// GenerateAllergyNarrative produces FHIR-compliant XHTML narrative for an
// AllergyIntolerance resource. USCDI v3 labels are used for all field headings.
func GenerateAllergyNarrative(r map[string]interface{}) string {
	rows := tableRow("Medication Allergy Intolerance", ccText(r["code"]))
	rows += tableRow("Clinical Status", ccText(r["clinicalStatus"]))
	// criticality is spec'd as a plain `code`, but this engine's own AL1.4
	// mapping currently emits it as a CodeableConcept (a separate, known
	// upstream mapping-datatype bug) — anyText handles either shape so the
	// narrative doesn't silently drop the value either way.
	rows += tableRow("Criticality", anyText(r["criticality"]))
	rows += tableRow("Onset Date", fhirStr(r, "onsetDateTime"))

	// Reaction (first entry only — covers the most clinically relevant event).
	// Spec-correct AllergyIntolerance.reaction carries manifestation (Codeable-
	// Concept array); this engine's own AL1.5 mapping instead populates the
	// free-text .description field directly — check manifestation first, fall
	// back to description, so the narrative reflects whichever shape is present.
	if reactions := fhirArr(r, "reaction"); len(reactions) > 0 {
		if reaction, ok := reactions[0].(map[string]interface{}); ok {
			if mfs := fhirArr(reaction, "manifestation"); len(mfs) > 0 {
				rows += tableRow("Reaction", ccText(mfs[0]))
			} else {
				rows += tableRow("Reaction", fhirStr(reaction, "description"))
			}
			rows += tableRow("Reaction Severity", fhirStr(reaction, "severity"))
		}
	}

	// Verification status (confirmed/unconfirmed/refuted)
	rows += tableRow("Verification Status", ccText(r["verificationStatus"]))

	return wrapDiv(heading("Allergy and Intolerance") + buildTable(rows))
}
