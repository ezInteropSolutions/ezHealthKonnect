// services/fhir_narrative/condition_narrative.go
// Generates XHTML narrative for a FHIR R4 Condition resource.
// USCDI v3 class: Problems — Active Conditions, Date of Diagnosis/Resolution.

package fhirnarrative

// GenerateConditionNarrative produces FHIR-compliant XHTML narrative for a
// Condition resource. USCDI v3 labels are used for all field headings.
func GenerateConditionNarrative(r map[string]interface{}) string {
	rows := tableRow("Active Condition", ccText(r["code"]))
	rows += tableRow("Clinical Status", ccText(r["clinicalStatus"]))
	rows += tableRow("Verification Status", ccText(r["verificationStatus"]))
	rows += tableRow("Date of Diagnosis", fhirStr(r, "onsetDateTime"))
	rows += tableRow("Date of Resolution", fhirStr(r, "abatementDateTime"))

	return wrapDiv(heading("Problem") + buildTable(rows))
}
