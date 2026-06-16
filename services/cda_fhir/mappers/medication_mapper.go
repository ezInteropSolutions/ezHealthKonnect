package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapMedications converts typed CDA medication section entries to FHIR resources.
//
// CDA moodCode dispatch:
//   - "INT" (intent/prescription) → MedicationRequest  (intent: "order")
//   - "EVN" (event/administered) or default → MedicationStatement
//
// C-CDA structure: substanceAdministration entry
//   - Drug code from consumable.manufacturedProduct.manufacturedMaterial.code
//   - Status from substanceAdministration statusCode
//   - Effective period from effectiveTime
//   - Route from routeCode
//   - Dose from doseQuantity
func MapMedications(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		var r map[string]interface{}
		if entry.MoodCode == "INT" {
			r = buildMedicationRequestResource(i+1, entry, patientRef)
		} else {
			r = buildMedicationStatementResource(i+1, entry, patientRef)
		}
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

// buildMedicationRequestResource maps a CDA substanceAdministration with moodCode="INT"
// (intent / prescription order) to a FHIR R4 MedicationRequest resource.
func buildMedicationRequestResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "MedicationRequest",
		"id":           fmt.Sprintf("medication-%d", idx),
		"intent":       "order",
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.MedicationRequestStatusToFHIR(entry.StatusCode)

	// Requester — US Core (us-core-21) requires this whenever intent="order".
	r["requester"] = requesterReference(entry)

	// Medication code from consumable
	if entry.Consumable != nil {
		mp := entry.Consumable.ManufacturedProduct
		if mp.ManufacturedMaterial != nil {
			if cc := transforms.CDACodeToCodeableConcept(mp.ManufacturedMaterial.Code); cc != nil {
				r["medicationCodeableConcept"] = cc
			}
		}
	}

	// Authored on / effective period
	if onset := transforms.CDATimeRangeToOnset(entry.EffectiveTime); onset != "" {
		r["authoredOn"] = onset
	}

	// Dosage instruction
	dosage := map[string]interface{}{}
	if entry.RouteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.RouteCode); cc != nil {
			dosage["route"] = cc
		}
	}
	if entry.DoseQuantity != nil {
		if qty := transforms.CDAQuantityToFHIR(*entry.DoseQuantity); qty != nil {
			dosage["doseAndRate"] = []interface{}{
				map[string]interface{}{"doseQuantity": qty},
			}
		}
	}
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		dosage["timing"] = map[string]interface{}{
			"repeat": map[string]interface{}{
				"boundsPeriod": period,
			},
		}
	}
	if len(dosage) > 0 {
		r["dosageInstruction"] = []interface{}{dosage}
	}

	return r
}

// buildMedicationStatementResource maps a CDA substanceAdministration with moodCode="EVN"
// (event / historically administered medication) to a FHIR R4 MedicationStatement resource.
func buildMedicationStatementResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "MedicationStatement",
		"id":           fmt.Sprintf("medication-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.MedicationStatusToFHIR(entry.StatusCode)

	// Medication code from consumable
	if entry.Consumable != nil {
		mp := entry.Consumable.ManufacturedProduct
		if mp.ManufacturedMaterial != nil {
			if cc := transforms.CDACodeToCodeableConcept(mp.ManufacturedMaterial.Code); cc != nil {
				r["medicationCodeableConcept"] = cc
			}
		}
	}

	// Effective period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["effectivePeriod"] = period
	} else if onset := transforms.CDATimeRangeToOnset(entry.EffectiveTime); onset != "" {
		r["effectiveDateTime"] = onset
	}

	// Dosage
	dosage := map[string]interface{}{}
	if entry.RouteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.RouteCode); cc != nil {
			dosage["route"] = cc
		}
	}
	if entry.DoseQuantity != nil {
		if qty := transforms.CDAQuantityToFHIR(*entry.DoseQuantity); qty != nil {
			dosage["doseAndRate"] = []interface{}{
				map[string]interface{}{"doseQuantity": qty},
			}
		}
	}
	if len(dosage) > 0 {
		r["dosage"] = []interface{}{dosage}
	}

	return r
}

// requesterReference builds the FHIR Reference for MedicationRequest.requester
// (required by US Core's us-core-21 constraint whenever intent="order") from
// the substanceAdministration entry's own <performer>/<author> data — prefer
// the performer (who carried out/ordered the act), falling back to the entry's
// author. Mirrors the existing inline display-only Reference convention used
// for Encounter.participant (see buildEncounterParticipant in encounter_mapper.go)
// rather than minting a separate Practitioner resource. Always returns a
// non-empty Reference so the SHALL constraint is met even when the source
// document carries no name for either.
func requesterReference(entry cdadocument.CDAEntry) map[string]interface{} {
	for _, p := range entry.Performers {
		if p.AssignedEntity.AssignedPerson == nil || len(p.AssignedEntity.AssignedPerson.Names) == 0 {
			continue
		}
		if display := buildDisplayFromName(p.AssignedEntity.AssignedPerson.Names[0]); display != "" {
			return map[string]interface{}{"display": display}
		}
	}
	for _, a := range entry.Authors {
		if a.AssignedAuthor.AssignedPerson == nil || len(a.AssignedAuthor.AssignedPerson.Names) == 0 {
			continue
		}
		if display := buildDisplayFromName(a.AssignedAuthor.AssignedPerson.Names[0]); display != "" {
			return map[string]interface{}{"display": display}
		}
	}
	return map[string]interface{}{"display": "Ordering Provider"}
}
