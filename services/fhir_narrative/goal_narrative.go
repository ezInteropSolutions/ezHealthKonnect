// services/fhir_narrative/goal_narrative.go
// Generates XHTML narrative for a FHIR R4 Goal resource.

package fhirnarrative

// GenerateGoalNarrative produces FHIR-compliant XHTML for a Goal resource.
func GenerateGoalNarrative(r map[string]interface{}) string {
	rows := tableRow("Goal", ccText(r["description"]))
	rows += tableRow("Life Cycle Status", fhirStr(r, "lifecycleStatus"))
	rows += tableRow("Achievement Status", ccText(r["achievementStatus"]))

	if targets := fhirArr(r, "target"); len(targets) > 0 {
		if t, ok := targets[0].(map[string]interface{}); ok {
			rows += tableRow("Target Measure", ccText(t["measure"]))
			rows += tableRow("Target Due Date", fhirStr(t, "dueDate"))
		}
	}

	rows += tableRow("Start Date", fhirStr(r, "startDate"))
	rows += tableRow("Priority", ccText(r["priority"]))

	return wrapDiv(heading("Goal") + buildTable(rows))
}
