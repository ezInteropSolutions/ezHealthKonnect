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

	// Find the problem observation via SUBJ entryRelationship (standard problem list).
	// Health concern sections use REFR instead — fall back to REFR when no SUBJ is found.
	problemObs := findRelByTypeCode(entry.EntryRelationships, "SUBJ")
	if problemObs == nil {
		problemObs = findRelByTypeCode(entry.EntryRelationships, "REFR")
	}
	if problemObs == nil {
		problemObs = &entry // flat structure (direct problem observation)
	}

	// Code: prefer observation value (CD: ICD-10/SNOMED for standard problem entries)
	if problemObs.Value != nil {
		if cc := transforms.CDAValueToFHIR(*problemObs.Value); cc != nil {
			if ccMap, ok := cc.(map[string]interface{}); ok {
				r["code"] = ccMap
			}
		}
	}
	// Fallback: use the observation's own code when the value yields nothing
	// (health concern acts and REFR observations carry the condition concept as code,
	// not value — e.g. PHQ-9 44261-6 or a problem coded at code rather than value).
	if _, hasCode := r["code"]; !hasCode {
		if cc := transforms.CDACodeToCodeableConcept(problemObs.Code); cc != nil {
			r["code"] = cc
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
