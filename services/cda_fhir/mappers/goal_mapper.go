package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapGoals converts typed CDA goal section entries to FHIR Goal resources.
//
// C-CDA structure: observation entry
//   - Description from entry.Code or entry.Value
//   - Status from entry.StatusCode
//   - Target date from entry.EffectiveTime
func MapGoals(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildGoalResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildGoalResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Goal",
		"id":           fmt.Sprintf("goal-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["lifecycleStatus"] = transforms.GoalStatusToFHIR(entry.StatusCode)

	// Description: prefer Value (CD), fall back to Code
	if entry.Value != nil && entry.Value.Code != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.Value.Code); cc != nil {
			r["description"] = cc
		}
	}
	if _, hasDesc := r["description"]; !hasDesc {
		if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
			r["description"] = cc
		} else {
			r["description"] = map[string]interface{}{"text": "Goal"}
		}
	}

	// Target date from effectiveTime.low or single value
	if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["target"] = []interface{}{
			map[string]interface{}{"dueDate": dt},
		}
	}

	return r
}
