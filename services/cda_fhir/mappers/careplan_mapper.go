package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapCarePlans converts typed CDA care plan / plan-of-care section entries to FHIR
// CarePlan resources.
//
// C-CDA structure: act, observation, or substanceAdministration entry
//   - Category from entry.Code
//   - Status from entry.StatusCode
//   - Period from entry.EffectiveTime
func MapCarePlans(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildCarePlanResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildCarePlanResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "CarePlan",
		"id":           fmt.Sprintf("careplan-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.CarePlanStatusToFHIR(entry.StatusCode)
	r["intent"] = "plan"

	// Category from entry code
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["category"] = []interface{}{cc}
	}

	// Period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["period"] = period
	}

	// Activities from entry relationships
	var activities []interface{}
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode != "COMP" && rel.TypeCode != "SUBJ" {
			continue
		}
		act := map[string]interface{}{}
		detail := map[string]interface{}{}
		if cc := transforms.CDACodeToCodeableConcept(rel.Entry.Code); cc != nil {
			detail["code"] = cc
		}
		detail["status"] = transforms.CarePlanStatusToFHIR(rel.Entry.StatusCode)
		if len(detail) > 0 {
			act["detail"] = detail
			activities = append(activities, act)
		}
	}
	if len(activities) > 0 {
		r["activity"] = activities
	}

	return r
}
