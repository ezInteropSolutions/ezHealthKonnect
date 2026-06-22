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
//   - Status from entry.StatusCode, overridden to "not-done" when entry.NegationInd is
//     set (refused/not administered) — see CONF:1198-8985
//   - Refusal reason from entryRelationship[@typeCode='RSON'] (Immunization Refusal Reason,
//     templateId .4.53), only read when NegationInd is set
//   - Date from entry.EffectiveTime.Value (single point in time)
//   - Route from entry.RouteCode
//   - Dose quantity from entry.DoseQuantity
//   - Performer from entry.Performers[0].AssignedEntity.AssignedPerson (the
//     <performer> element, distinct from entry.Participants/<participant>)
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

	r["status"] = transforms.ImmunizationStatusToFHIR(entry.StatusCode, entry.NegationInd)

	// Refusal reason (RSON -> Immunization Refusal Reason, templateId .4.53) — only
	// meaningful when the vaccine was actually refused/not given.
	if entry.NegationInd {
		if reason := findRelByTypeCode(entry.EntryRelationships, "RSON"); reason != nil {
			var cc map[string]interface{}
			if reason.Value != nil {
				if v, ok := transforms.CDAValueToFHIR(*reason.Value).(map[string]interface{}); ok {
					cc = v
				}
			}
			if cc == nil {
				cc = transforms.CDACodeToCodeableConcept(reason.Code)
			}
			if cc != nil {
				r["statusReason"] = cc
			}
		}
	}

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

	// Performer: from entry.Performers (parsed from <performer> elements --
	// see entry_parser.go's parsePerformers), NOT entry.Participants (parsed
	// from <participant>, a different XML element entirely). The previous
	// version of this loop read entry.Participants[typeCode=PRF], which
	// could never match real data: C-CDA's Immunization Activity carries its
	// performer via a dedicated <performer typeCode="PRF"> element, the same
	// shape medication_mapper.go's requesterReference already reads
	// correctly from entry.Performers.
	for _, p := range entry.Performers {
		if p.AssignedEntity.AssignedPerson != nil && len(p.AssignedEntity.AssignedPerson.Names) > 0 {
			r["performer"] = []interface{}{
				map[string]interface{}{
					"actor": map[string]interface{}{
						"display": buildDisplayFromName(p.AssignedEntity.AssignedPerson.Names[0]),
					},
				},
			}
			break
		}
	}

	return r
}
