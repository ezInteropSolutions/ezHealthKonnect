// services/fhir_narrative/location_narrative.go
// Generates XHTML narrative for a FHIR R4 Location resource.

package fhirnarrative

// GenerateLocationNarrative produces FHIR-compliant XHTML for a Location resource.
func GenerateLocationNarrative(r map[string]interface{}) string {
	rows := tableRow("Location", fhirStr(r, "name"))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Type", ccText(r["physicalType"]))

	if managingOrg := fhirMap(r, "managingOrganization"); managingOrg != nil {
		rows += tableRow("Managing Organization", fhirStr(managingOrg, "display"))
	}

	return wrapDiv(heading("Location") + buildTable(rows))
}
