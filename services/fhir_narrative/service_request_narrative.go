// services/fhir_narrative/service_request_narrative.go
// Generates XHTML narrative for a FHIR R4 ServiceRequest resource.

package fhirnarrative

// GenerateServiceRequestNarrative produces FHIR-compliant XHTML for a ServiceRequest resource.
func GenerateServiceRequestNarrative(r map[string]interface{}) string {
	rows := tableRow("Order", ccText(r["code"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Intent", fhirStr(r, "intent"))
	rows += tableRow("Reason", firstArrText(r, "reasonCode"))

	if occurrence := fhirMap(r, "occurrencePeriod"); occurrence != nil {
		rows += tableRow("Start", fhirStr(occurrence, "start"))
		rows += tableRow("End", fhirStr(occurrence, "end"))
	} else {
		rows += tableRow("Date", fhirStr(r, "occurrenceDateTime"))
	}

	rows += tableRow("Ordered On", fhirStr(r, "authoredOn"))

	return wrapDiv(heading("Service Request") + buildTable(rows))
}
