// services/fhir_narrative/family_history_narrative.go
// Generates XHTML narrative for a FHIR R4 FamilyMemberHistory resource.

package fhirnarrative

// GenerateFamilyMemberHistoryNarrative produces FHIR-compliant XHTML for a FamilyMemberHistory.
func GenerateFamilyMemberHistoryNarrative(r map[string]interface{}) string {
	rows := tableRow("Relationship", ccText(r["relationship"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Sex", ccText(r["sex"]))

	if born := fhirStr(r, "bornDate"); born != "" {
		rows += tableRow("Date of Birth", born)
	}
	if deceased := fhirStr(r, "deceasedBoolean"); deceased != "" {
		rows += tableRow("Deceased", deceased)
	}

	if conditions := fhirArr(r, "condition"); len(conditions) > 0 {
		for i, raw := range conditions {
			c, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			label := "Condition"
			if i > 0 {
				label = ""
			}
			rows += tableRow(label, ccText(c["code"]))
			rows += tableRow("Onset", fhirStr(c, "onsetString"))
		}
	}

	rows += tableRow("Note", fhirStr(r, "note"))

	return wrapDiv(heading("Family Member History") + buildTable(rows))
}
