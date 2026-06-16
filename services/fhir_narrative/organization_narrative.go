// services/fhir_narrative/organization_narrative.go
// Generates XHTML narrative for a FHIR R4 Organization resource.

package fhirnarrative

// GenerateOrganizationNarrative produces FHIR-compliant XHTML for an Organization resource.
func GenerateOrganizationNarrative(r map[string]interface{}) string {
	rows := tableRow("Organization", fhirStr(r, "name"))
	rows += tableRow("Type", ccText(r["type"]))

	if ids := fhirArr(r, "identifier"); len(ids) > 0 {
		if id, ok := ids[0].(map[string]interface{}); ok {
			rows += tableRow("ID", fhirStr(id, "value"))
		}
	}

	if telecom := fhirArr(r, "telecom"); len(telecom) > 0 {
		if t, ok := telecom[0].(map[string]interface{}); ok {
			rows += tableRow(fhirStr(t, "system"), fhirStr(t, "value"))
		}
	}

	if addresses := fhirArr(r, "address"); len(addresses) > 0 {
		if a, ok := addresses[0].(map[string]interface{}); ok {
			city := fhirStr(a, "city")
			state := fhirStr(a, "state")
			if city != "" || state != "" {
				rows += tableRow("Location", city+" "+state)
			}
		}
	}

	return wrapDiv(heading("Organization") + buildTable(rows))
}
