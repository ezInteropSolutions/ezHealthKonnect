// services/fhir_narrative/coverage_narrative.go
// Generates XHTML narrative for a FHIR R4 Coverage resource.

package fhirnarrative

// GenerateCoverageNarrative produces FHIR-compliant XHTML for a Coverage resource.
func GenerateCoverageNarrative(r map[string]interface{}) string {
	rows := tableRow("Coverage Type", ccText(r["type"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Member ID", fhirStr(r, "subscriberId"))
	rows += tableRow("Payer", fhirStr(fhirMap(r, "payor"), "display"))

	if period := fhirMap(r, "period"); period != nil {
		rows += tableRow("Coverage Start", fhirStr(period, "start"))
		rows += tableRow("Coverage End", fhirStr(period, "end"))
	}

	if classes := fhirArr(r, "class"); len(classes) > 0 {
		if c, ok := classes[0].(map[string]interface{}); ok {
			rows += tableRow("Plan", fhirStr(c, "name"))
			rows += tableRow("Plan ID", fhirStr(c, "value"))
		}
	}

	rows += tableRow("Group ID", fhirStr(r, "groupId"))
	rows += tableRow("Network", fhirStr(r, "network"))

	return wrapDiv(heading("Insurance Coverage") + buildTable(rows))
}
