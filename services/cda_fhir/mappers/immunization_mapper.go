package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapImmunizations converts typed CDA immunization section entries to FHIR
// Immunization resources.
//
// C-CDA structure: substanceAdministration entry
//   - Vaccine code from consumable.manufacturedProduct.manufacturedMaterial.code (CVX)
//   - Status from entry.StatusCode
//   - Date from entry.EffectiveTime.Value (single point in time)
//   - Route from entry.RouteCode
//   - Dose quantity from entry.DoseQuantity
func MapImmunizations(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildImmunizationResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildImmunizationResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Immunization",
		"id":           fmt.Sprintf("immunization-%d", idx),
	}
	if patientRef != "" {
		r["patient"] = ref(patientRef)
	}

	r["status"] = transforms.ImmunizationStatusToFHIR(entry.StatusCode)

	// Vaccine code (CVX)
	if entry.Consumable != nil {
		mp := entry.Consumable.ManufacturedProduct
		if mp.ManufacturedMaterial != nil {
			if cc := transforms.CDACodeToCodeableConcept(mp.ManufacturedMaterial.Code); cc != nil {
				r["vaccineCode"] = cc
			}
		}
	}

	// Occurrence date (single point in time or period low)
	if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["occurrenceDateTime"] = dt
	}

	// Route
	if entry.RouteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.RouteCode); cc != nil {
			r["route"] = cc
		}
	}

	// Dose quantity
	if entry.DoseQuantity != nil {
		if qty := transforms.CDAQuantityToFHIR(*entry.DoseQuantity); qty != nil {
			r["doseQuantity"] = qty
		}
	}

	// Lot number
	if entry.Consumable != nil && entry.Consumable.ManufacturedProduct.ManufacturedMaterial != nil {
		lot := entry.Consumable.ManufacturedProduct.ManufacturedMaterial.LotNumberText
		if lot != "" {
			r["lotNumber"] = lot
		}
	}

	// Performer from participants
	for _, p := range entry.Participants {
		if p.TypeCode == "PRF" {
			if p.ParticipantRole.PlayingEntity != nil && len(p.ParticipantRole.PlayingEntity.Names) > 0 {
				r["performer"] = []interface{}{
					map[string]interface{}{
						"actor": map[string]interface{}{
							"display": buildDisplayFromName(p.ParticipantRole.PlayingEntity.Names[0]),
						},
					},
				}
			}
			break
		}
	}

	return r
}
