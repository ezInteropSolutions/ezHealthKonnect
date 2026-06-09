// services/fhir_narrative/allergy_narrative.go
// Generates XHTML narrative for a FHIR R4 AllergyIntolerance resource.
// USCDI v3 class: Allergies and Intolerances.

package fhirnarrative

// GenerateAllergyNarrative produces FHIR-compliant XHTML narrative for an
// AllergyIntolerance resource. USCDI v3 labels are used for all field headings.
func GenerateAllergyNarrative(r map[string]interface{}) string {
	rows := tableRow("Medication Allergy Intolerance", ccText(r["code"]))
	rows += tableRow("Clinical Status", ccText(r["clinicalStatus"]))
	rows += tableRow("Criticality", fhirStr(r, "criticality"))
	rows += tableRow("Onset Date", fhirStr(r, "onsetDateTime"))

	// Reaction (first entry only — covers the most clinically relevant event)
	if reactions := fhirArr(r, "reaction"); len(reactions) > 0 {
		if reaction, ok := reactions[0].(map[string]interface{}); ok {
			if mfs := fhirArr(reaction, "manifestation"); len(mfs) > 0 {
				rows += tableRow("Reaction", ccText(mfs[0]))
			}
			rows += tableRow("Reaction Severity", fhirStr(reaction, "severity"))
		}
	}

	// Verification status (confirmed/unconfirmed/refuted)
	rows += tableRow("Verification Status", ccText(r["verificationStatus"]))

	return wrapDiv(heading("Allergy and Intolerance") + buildTable(rows))
}
