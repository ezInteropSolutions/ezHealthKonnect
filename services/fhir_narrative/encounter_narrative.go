// services/fhir_narrative/encounter_narrative.go
// Generates XHTML narrative for a FHIR R4 Encounter resource.

package fhirnarrative

// GenerateEncounterNarrative produces FHIR-compliant XHTML for an Encounter resource.
func GenerateEncounterNarrative(r map[string]interface{}) string {
	rows := tableRow("Encounter Type", ccText(r["type"]))
	rows += tableRow("Class", fhirStr(fhirMap(r, "class"), "code"))
	rows += tableRow("Status", fhirStr(r, "status"))

	if period := fhirMap(r, "period"); period != nil {
		rows += tableRow("Start", fhirStr(period, "start"))
		rows += tableRow("End", fhirStr(period, "end"))
	}

	if participants := fhirArr(r, "participant"); len(participants) > 0 {
		if p, ok := participants[0].(map[string]interface{}); ok {
			rows += tableRow("Participant", fhirStr(fhirMap(p, "individual"), "display"))
		}
	}

	if serviceProvider := fhirMap(r, "serviceProvider"); serviceProvider != nil {
		rows += tableRow("Service Provider", fhirStr(serviceProvider, "display"))
	}

	rows += tableRow("Reason", ccText(r["reasonCode"]))

	return wrapDiv(heading("Encounter") + buildTable(rows))
}
