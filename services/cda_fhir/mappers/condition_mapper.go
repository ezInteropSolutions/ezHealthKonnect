package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapConditions converts typed CDA problem/health-concern section entries to FHIR
// Condition resources. Both the problems and healthConcerns sections use this mapper.
//
// C-CDA structure: outer concern act → SUBJ entryRelationship → problem observation
//   - Problem code from observation value (ICD-10, SNOMED)
//   - Clinical status from observation statusCode
//   - Onset/abatement from observation effectiveTime
func MapConditions(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildConditionResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildConditionResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Condition",
		"id":           fmt.Sprintf("condition-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	// Find the problem observation via SUBJ entryRelationship
	problemObs := findRelByTypeCode(entry.EntryRelationships, "SUBJ")
	if problemObs == nil {
		problemObs = &entry // flat structure
	}

	// Code from observation value (CD: ICD-10, SNOMED)
	if problemObs.Value != nil {
		if cc := transforms.CDAValueToFHIR(*problemObs.Value); cc != nil {
			if ccMap, ok := cc.(map[string]interface{}); ok {
				r["code"] = ccMap
			}
		}
	}

	// Clinical status
	r["clinicalStatus"] = transforms.ConditionStatusToFHIR(problemObs.StatusCode)

	// Verification status: defaulted to confirmed
	r["verificationStatus"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/condition-ver-status",
				"code":    "confirmed",
				"display": "Confirmed",
			},
		},
	}

	// Category: problem-list-item
	r["category"] = []interface{}{
		map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://terminology.hl7.org/CodeSystem/condition-category",
					"code":    "problem-list-item",
					"display": "Problem List Item",
				},
			},
		},
	}

	// Onset from observation effectiveTime.low
	if onset := transforms.CDATimeRangeToOnset(problemObs.EffectiveTime); onset != "" {
		r["onsetDateTime"] = onset
	}

	// Abatement from effectiveTime.high (non-empty = resolved)
	if abate := transforms.CDATimeToFHIRDateTime(problemObs.EffectiveTime.High); abate != "" {
		r["abatementDateTime"] = abate
	}

	return r
}
