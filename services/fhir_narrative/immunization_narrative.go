// services/fhir_narrative/immunization_narrative.go
// Generates XHTML narrative for a FHIR R4 Immunization resource.

package fhirnarrative

// GenerateImmunizationNarrative produces FHIR-compliant XHTML for an Immunization resource.
func GenerateImmunizationNarrative(r map[string]interface{}) string {
	rows := tableRow("Vaccine", ccText(r["vaccineCode"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Date", fhirStr(r, "occurrenceDateTime"))
	rows += tableRow("Lot Number", fhirStr(r, "lotNumber"))

	if performers := fhirArr(r, "performer"); len(performers) > 0 {
		if p, ok := performers[0].(map[string]interface{}); ok {
			rows += tableRow("Administering Provider", fhirStr(fhirMap(p, "actor"), "display"))
		}
	}

	if education := fhirArr(r, "education"); len(education) > 0 {
		if e, ok := education[0].(map[string]interface{}); ok {
			rows += tableRow("Education Publication", fhirStr(e, "publicationDate"))
		}
	}

	rows += tableRow("Primary Source", fhirStr(r, "primarySource"))

	return wrapDiv(heading("Immunization") + buildTable(rows))
}
