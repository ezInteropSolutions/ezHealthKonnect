// services/fhir_narrative/care_plan_narrative.go
// Generates XHTML narrative for a FHIR R4 CarePlan resource.

package fhirnarrative

// GenerateCarePlanNarrative produces FHIR-compliant XHTML for a CarePlan resource.
func GenerateCarePlanNarrative(r map[string]interface{}) string {
	rows := tableRow("Title", fhirStr(r, "title"))
	rows += tableRow("Description", fhirStr(r, "description"))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow("Intent", fhirStr(r, "intent"))

	if period := fhirMap(r, "period"); period != nil {
		rows += tableRow("Start", fhirStr(period, "start"))
		rows += tableRow("End", fhirStr(period, "end"))
	}

	if categories := fhirArr(r, "category"); len(categories) > 0 {
		rows += tableRow("Category", ccText(categories[0]))
	}

	if goals := fhirArr(r, "goal"); len(goals) > 0 {
		if g, ok := goals[0].(map[string]interface{}); ok {
			rows += tableRow("Goal", fhirStr(g, "display"))
		}
	}

	if activities := fhirArr(r, "activity"); len(activities) > 0 {
		for i, raw := range activities {
			a, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			detail := fhirMap(a, "detail")
			if detail == nil {
				continue
			}
			label := "Activity"
			if i > 0 {
				label = ""
			}
			rows += tableRow(label, ccText(detail["code"]))
		}
	}

	return wrapDiv(heading("Care Plan") + buildTable(rows))
}
