// services/fhir_narrative/care_team_narrative.go
// Generates XHTML narrative for a FHIR R4 CareTeam resource.

package fhirnarrative

// GenerateCareTeamNarrative produces FHIR-compliant XHTML for a CareTeam resource.
func GenerateCareTeamNarrative(r map[string]interface{}) string {
	rows := tableRow("Care Team Type", firstArrText(r, "category"))
	rows += tableRow("Status", fhirStr(r, "status"))

	if participants := fhirArr(r, "participant"); len(participants) > 0 {
		if p, ok := participants[0].(map[string]interface{}); ok {
			rows += tableRow("Care Team Member", fhirStr(fhirMap(p, "member"), "display"))
			rows += tableRow("Role", firstArrText(p, "role"))
		}
	}

	if period := fhirMap(r, "period"); period != nil {
		rows += tableRow("Start", fhirStr(period, "start"))
		rows += tableRow("End", fhirStr(period, "end"))
	}

	return wrapDiv(heading("Care Team") + buildTable(rows))
}

// firstArrText extracts the human-readable text of the first CodeableConcept
// in a resource's array-valued field (e.g. category[], participant.role[]).
func firstArrText(m map[string]interface{}, key string) string {
	arr := fhirArr(m, key)
	if len(arr) == 0 {
		return ""
	}
	cc, ok := arr[0].(map[string]interface{})
	if !ok {
		return ""
	}
	return ccText(cc)
}
