package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// LOINC codes for the C-CDA Vital Signs Organizer's blood-pressure pair, and
// the "magic" panel code the FHIR "bp" / US Core vital-signs profile requires.
const (
	loincSystolicBP  = "8480-6"
	loincDiastolicBP = "8462-4"
	loincBPPanel     = "85354-9"
)

// MapObservations converts typed CDA observation entries to FHIR Observation resources.
// Used for vitalSigns, labResults, socialHistory, functionalStatus, mentalStatus,
// and assessmentAndPlan sections. The category parameter sets Observation.category.
//
// C-CDA organizer entries are flattened: each component becomes one Observation,
// except a Systolic+Diastolic blood-pressure pair (see extractBloodPressurePanels),
// which is combined into a single Observation with two `component`s as the bp/
// vital-signs profile requires. Direct observation entries (non-organizer) are
// mapped one-to-one.
func MapObservations(entries []cdadocument.CDAEntry, patientRef string, category string) []map[string]interface{} {
	remaining := entries
	var resources []map[string]interface{}

	if category == "vital-signs" {
		var bpResources []map[string]interface{}
		remaining, bpResources = extractBloodPressurePanels(entries, patientRef)
		resources = append(resources, bpResources...)
	}

	flat := flattenObservationEntries(remaining)
	for _, entry := range flat {
		r := buildObservationResource(len(resources)+1, entry, patientRef, category)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

// extractBloodPressurePanels scans entries for a vital-signs organizer whose
// Components include both Systolic (LOINC 8480-6) and Diastolic (LOINC 8462-4)
// blood pressure readings. Per the FHIR "bp" profile (and US Core's vital-signs
// pattern built on it), these MUST be reported as two `component`s of a single
// Observation coded 85354-9 ("Blood pressure panel"), not as two independent
// Observations. Any other components of the same organizer — and any entries
// that aren't a BP pair — are returned untouched in `remaining` for normal
// flattening by flattenObservationEntries.
func extractBloodPressurePanels(entries []cdadocument.CDAEntry, patientRef string) (remaining []cdadocument.CDAEntry, bpResources []map[string]interface{}) {
	for _, e := range entries {
		if e.EntryType != "organizer" || len(e.Components) == 0 {
			remaining = append(remaining, e)
			continue
		}

		var systolic, diastolic *cdadocument.CDAEntry
		var rest []cdadocument.CDAEntry
		for i := range e.Components {
			switch e.Components[i].Code.Code {
			case loincSystolicBP:
				systolic = &e.Components[i]
			case loincDiastolicBP:
				diastolic = &e.Components[i]
			default:
				rest = append(rest, e.Components[i])
			}
		}

		if systolic == nil || diastolic == nil {
			// No BP pair in this organizer — leave it untouched for normal flattening.
			remaining = append(remaining, e)
			continue
		}

		bpResources = append(bpResources, buildBloodPressureObservation(len(bpResources)+1, *systolic, *diastolic, patientRef))
		if len(rest) > 0 {
			organizerRest := e
			organizerRest.Components = rest
			remaining = append(remaining, organizerRest)
		}
	}
	return remaining, bpResources
}

// buildBloodPressureObservation assembles the combined BP panel Observation
// (code 85354-9) with Systolic/Diastolic as its two `component`s.
func buildBloodPressureObservation(idx int, systolic, diastolic cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Observation",
		"id":           fmt.Sprintf("observation-%d", idx),
		"status":       transforms.ObservationStatusToFHIR(systolic.StatusCode),
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  categorySystem("vital-signs"),
						"code":    "vital-signs",
						"display": observationCategoryDisplay("vital-signs"),
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://loinc.org",
					"code":   loincBPPanel,
				},
			},
			"text": "Blood pressure panel",
		},
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}
	if dt := transforms.CDATimeRangeToOnset(systolic.EffectiveTime); dt != "" {
		r["effectiveDateTime"] = dt
	} else if dt := transforms.CDATimeRangeToOnset(diastolic.EffectiveTime); dt != "" {
		r["effectiveDateTime"] = dt
	}

	var components []interface{}
	if comp := buildBPComponent(systolic); comp != nil {
		components = append(components, comp)
	}
	if comp := buildBPComponent(diastolic); comp != nil {
		components = append(components, comp)
	}
	if len(components) > 0 {
		r["component"] = components
	}

	return r
}

// buildBPComponent builds one Observation.component entry (code + value) for
// a single Systolic or Diastolic blood-pressure reading.
func buildBPComponent(entry cdadocument.CDAEntry) map[string]interface{} {
	comp := map[string]interface{}{}
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		comp["code"] = cc
	}
	if entry.Value != nil {
		setObservationValue(comp, *entry.Value)
	}
	if len(comp) == 0 {
		return nil
	}
	return comp
}

// flattenObservationEntries expands organizer components into the top-level slice.
// Panel organizers (lab panels, vital sign panels) contain the individual results
// as Components. Non-organizer entries are returned as-is.
func flattenObservationEntries(entries []cdadocument.CDAEntry) []cdadocument.CDAEntry {
	var flat []cdadocument.CDAEntry
	for _, e := range entries {
		if e.EntryType == "organizer" && len(e.Components) > 0 {
			flat = append(flat, e.Components...)
		} else {
			flat = append(flat, e)
		}
	}
	return flat
}

func buildObservationResource(idx int, entry cdadocument.CDAEntry, patientRef string, category string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Observation",
		"id":           fmt.Sprintf("observation-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	// Status
	r["status"] = transforms.ObservationStatusToFHIR(entry.StatusCode)

	// Category (vital-signs, laboratory, social-history, etc.)
	if category != "" {
		r["category"] = []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  categorySystem(category),
						"code":    category,
						"display": observationCategoryDisplay(category),
					},
				},
			},
		}
	}

	// Code (LOINC observation code)
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["code"] = cc
	}

	// Effective time
	if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["effectiveDateTime"] = dt
	}

	// Value (polymorphic: PQ → valueQuantity, CD → valueCodeableConcept, ST → valueString, etc.)
	if entry.Value != nil {
		setObservationValue(r, *entry.Value)
	}

	// Interpretation from entry relationships (typeCode=COMP with interpretation code)
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode != "COMP" {
			continue
		}
		if rel.Entry.Value != nil && rel.Entry.Value.Code != nil {
			if cc := transforms.CDACodeToCodeableConcept(*rel.Entry.Value.Code); cc != nil {
				r["interpretation"] = []interface{}{cc}
			}
		}
		break
	}

	return r
}

// setObservationValue populates the correct FHIR value[x] key based on CDAValue type.
func setObservationValue(r map[string]interface{}, v cdadocument.CDAValue) {
	if v.NullFlavor != "" {
		return
	}
	switch v.Type {
	case "PQ":
		if v.Quantity != nil {
			if qty := transforms.CDAQuantityToFHIR(*v.Quantity); qty != nil {
				r["valueQuantity"] = qty
			}
		}
	case "CD", "CE", "CS":
		if v.Code != nil {
			if cc := transforms.CDACodeToCodeableConcept(*v.Code); cc != nil {
				r["valueCodeableConcept"] = cc
			}
		}
	case "ST", "ED":
		if v.Text != "" {
			r["valueString"] = v.Text
		}
	case "BL":
		if v.Boolean != nil {
			r["valueBoolean"] = *v.Boolean
		}
	case "INT":
		if v.Integer != nil {
			r["valueInteger"] = *v.Integer
		}
	case "REAL":
		if v.Real != nil {
			r["valueQuantity"] = map[string]interface{}{"value": *v.Real}
		}
	case "IVL_TS":
		if v.TimeRange != nil {
			if period := transforms.CDATimeRangeToPeriod(*v.TimeRange); period != nil {
				r["valuePeriod"] = period
			}
		}
	}
}

// Base FHIR vs. US Core Observation.category CodeSystems. A handful of
// US Core-defined category codes (functional-status, cognitive-status,
// disability-status, sdoh, ...) live in US Core's own CodeSystem, not the base
// FHIR terminology.hl7.org one — using the base system for them produces an
// "Unknown code" validation error.
const (
	baseObservationCategorySystem   = "http://terminology.hl7.org/CodeSystem/observation-category"
	usCoreObservationCategorySystem = "http://hl7.org/fhir/us/core/CodeSystem/us-core-category"
)

// categorySystem returns the correct CodeSystem URI for an Observation.category code.
func categorySystem(code string) string {
	switch code {
	case "functional-status", "cognitive-status", "disability-status", "sdoh",
		"treatment-intervention-preference", "care-experience-preference":
		return usCoreObservationCategorySystem
	default:
		return baseObservationCategorySystem
	}
}

func observationCategoryDisplay(code string) string {
	switch code {
	case "vital-signs":
		return "Vital Signs"
	case "laboratory":
		return "Laboratory"
	case "social-history":
		return "Social History"
	case "functional-status":
		return "Functional Status"
	case "survey":
		return "Survey"
	case "exam":
		return "Exam"
	default:
		return code
	}
}
