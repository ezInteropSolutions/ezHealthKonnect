// services/fhir_narrative/practitioner_narrative.go
// Generates XHTML narrative for a FHIR R4 Practitioner resource.

package fhirnarrative

// GeneratePractitionerNarrative produces FHIR-compliant XHTML for a Practitioner resource.
func GeneratePractitionerNarrative(r map[string]interface{}) string {
	// Name (first HumanName entry)
	var displayName string
	if names := fhirArr(r, "name"); len(names) > 0 {
		if n, ok := names[0].(map[string]interface{}); ok {
			displayName = fhirStr(n, "text")
			if displayName == "" {
				family := fhirStr(n, "family")
				given := ""
				if givens := fhirArr(n, "given"); len(givens) > 0 {
					given, _ = givens[0].(string)
				}
				if given != "" {
					displayName = given + " " + family
				} else {
					displayName = family
				}
			}
		}
	}

	rows := tableRow("Practitioner", displayName)

	// NPI or other identifier
	if ids := fhirArr(r, "identifier"); len(ids) > 0 {
		if id, ok := ids[0].(map[string]interface{}); ok {
			rows += tableRow("NPI", fhirStr(id, "value"))
		}
	}

	if quals := fhirArr(r, "qualification"); len(quals) > 0 {
		if q, ok := quals[0].(map[string]interface{}); ok {
			rows += tableRow("Qualification", ccText(q["code"]))
		}
	}

	if telecom := fhirArr(r, "telecom"); len(telecom) > 0 {
		if t, ok := telecom[0].(map[string]interface{}); ok {
			rows += tableRow(telecomSystemLabel(fhirStr(t, "system")), fhirStr(t, "value"))
		}
	}

	return wrapDiv(heading("Practitioner") + buildTable(rows))
}
