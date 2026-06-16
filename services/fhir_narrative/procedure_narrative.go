// services/fhir_narrative/procedure_narrative.go
// Generates XHTML narrative for a FHIR R4 Procedure resource.

package fhirnarrative

// GenerateProcedureNarrative produces FHIR-compliant XHTML for a Procedure resource.
func GenerateProcedureNarrative(r map[string]interface{}) string {
	rows := tableRow("Procedure", ccText(r["code"]))
	rows += tableRow("Status", fhirStr(r, "status"))

	// Performed date (may be dateTime or Period)
	if dt := fhirStr(r, "performedDateTime"); dt != "" {
		rows += tableRow("Performed", dt)
	} else if period := fhirMap(r, "performedPeriod"); period != nil {
		rows += tableRow("Start", fhirStr(period, "start"))
		rows += tableRow("End", fhirStr(period, "end"))
	}

	if performers := fhirArr(r, "performer"); len(performers) > 0 {
		if p, ok := performers[0].(map[string]interface{}); ok {
			rows += tableRow("Performer", fhirStr(fhirMap(p, "actor"), "display"))
		}
	}

	rows += tableRow("Body Site", ccText(r["bodySite"]))
	rows += tableRow("Outcome", ccText(r["outcome"]))

	return wrapDiv(heading("Procedure") + buildTable(rows))
}
