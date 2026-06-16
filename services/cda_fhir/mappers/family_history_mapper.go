package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapFamilyHistory converts typed CDA family history section entries to FHIR
// FamilyMemberHistory resources.
//
// C-CDA structure: organizer entry per family member
//   - Relationship from organizer.Subject.RelatedSubject.Code
//     (mapped via participants with typeCode="SBJ")
//   - Conditions from organizer.Components (observation entries)
func MapFamilyHistory(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildFamilyHistoryResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildFamilyHistoryResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "FamilyMemberHistory",
		"id":           fmt.Sprintf("familyhistory-%d", idx),
	}
	if patientRef != "" {
		r["patient"] = ref(patientRef)
	}

	// Required FHIR status
	r["status"] = "completed"

	// Relationship from the SBJ participant's role code
	for _, p := range entry.Participants {
		if p.TypeCode == "SBJ" {
			if cc := transforms.CDACodeToCodeableConcept(p.ParticipantRole.Code); cc != nil {
				r["relationship"] = cc
			}
			// Family member name
			if p.ParticipantRole.PlayingEntity != nil && len(p.ParticipantRole.PlayingEntity.Names) > 0 {
				if hn := transforms.CDANameToFHIR(p.ParticipantRole.PlayingEntity.Names[0]); hn != nil {
					r["name"] = fmt.Sprintf("%v", hn["family"])
				}
			}
			break
		}
	}

	// Conditions from component observations
	var conditions []interface{}
	for _, comp := range entry.Components {
		if comp.Value == nil {
			continue
		}
		cond := map[string]interface{}{}
		if cc := transforms.CDAValueToFHIR(*comp.Value); cc != nil {
			if ccMap, ok := cc.(map[string]interface{}); ok {
				cond["code"] = ccMap
			}
		}
		if onset := transforms.CDATimeRangeToOnset(comp.EffectiveTime); onset != "" {
			cond["onsetDateTime"] = onset
		}
		if len(cond) > 0 {
			conditions = append(conditions, cond)
		}
	}
	if len(conditions) > 0 {
		r["condition"] = conditions
	}

	return r
}
