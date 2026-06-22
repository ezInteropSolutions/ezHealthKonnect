package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapConditions converts typed CDA problem/health-concern section entries to FHIR
// Condition resources. Both the problems and healthConcerns sections use this mapper —
// category distinguishes them (see conditionCategoryCC), since US Core's
// us-core-condition-problems-health-concerns profile requires a different
// Condition.category coding for each (verified against the published "US Core
// Problem or Health Concern" value set, not assumed).
//
// C-CDA structure: outer concern act → SUBJ entryRelationship → problem observation
//   - Problem code from observation value (ICD-10, SNOMED)
//   - Clinical status from observation statusCode
//   - Onset/abatement from observation effectiveTime
func MapConditions(entries []cdadocument.CDAEntry, patientRef string, category string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildConditionResource(i+1, entry, patientRef, category)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

// conditionCategoryCC builds Condition.category for the given section's category
// code. Per the published US Core "US Core Problem or Health Concern" value set
// (https://hl7.org/fhir/us/core/ValueSet-us-core-problem-or-health-concern.html):
//   - "problem-list-item" -> base http://terminology.hl7.org/CodeSystem/condition-category
//   - "health-concern"    -> US Core-specific http://hl7.org/fhir/us/core/CodeSystem/condition-category
//
// These are two DIFFERENT CodeSystems, not one CodeSystem with two codes —
// "health-concern" does not exist in the base condition-category CodeSystem.
func conditionCategoryCC(category string) map[string]interface{} {
	system, display := "http://terminology.hl7.org/CodeSystem/condition-category", "Problem List Item"
	code := "problem-list-item"
	if category == "health-concern" {
		system, code, display = "http://hl7.org/fhir/us/core/CodeSystem/condition-category", "health-concern", "Health Concern"
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  system,
				"code":    code,
				"display": display,
			},
		},
	}
}

func buildConditionResource(idx int, entry cdadocument.CDAEntry, patientRef string, category string) map[string]interface{} {
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

	// Verification status: refuted when the problem observation (or the outer
	// concern act) is negated — e.g. "no known problems" — confirmed otherwise.
	verificationCode, verificationDisplay := "confirmed", "Confirmed"
	if entry.NegationInd || problemObs.NegationInd {
		verificationCode, verificationDisplay = "refuted", "Refuted"
	}
	r["verificationStatus"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/condition-ver-status",
				"code":    verificationCode,
				"display": verificationDisplay,
			},
		},
	}

	// Severity from a nested SUBJ entryRelationship whose observation is coded
	// "SEV" — same C-CDA idiom allergy_mapper.go already handles for reactions.
	// Condition.severity is a CodeableConcept (unlike AllergyIntolerance.reaction.severity,
	// which is a fixed mild|moderate|severe string), so this reuses
	// CDACodeToCodeableConcept directly rather than a string-mapping helper.
	if sevObs := findRelByTypeCode(problemObs.EntryRelationships, "SUBJ"); sevObs != nil &&
		sevObs.Code.Code == "SEV" && sevObs.Value != nil && sevObs.Value.Code != nil {
		if cc := transforms.CDACodeToCodeableConcept(*sevObs.Value.Code); cc != nil {
			r["severity"] = cc
		}
	}

	// Category: distinguishes the problems section from the healthConcerns
	// section (see conditionCategoryCC doc comment for the spec citation).
	r["category"] = []interface{}{conditionCategoryCC(category)}

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
