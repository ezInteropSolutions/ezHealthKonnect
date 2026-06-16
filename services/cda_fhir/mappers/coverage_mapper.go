package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapCoverage converts typed CDA payors section entries to FHIR Coverage resources.
//
// C-CDA structure: act entry (coverage activity)
//   - Policy type from entry.Code
//   - Payer organization from participants with typeCode="PRF" or "HLD"
//   - Period from entry.EffectiveTime
//   - Member ID from entry relationships
func MapCoverage(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildCoverageResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildCoverageResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Coverage",
		"id":           fmt.Sprintf("coverage-%d", idx),
	}

	r["status"] = "active"

	if patientRef != "" {
		r["beneficiary"] = ref(patientRef)
		r["subscriber"] = ref(patientRef)
	}

	// Coverage type from entry code
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["type"] = cc
	}

	// Period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["period"] = period
	}

	// Payer from HLD (holder/payer) participant
	for _, p := range entry.Participants {
		if p.TypeCode == "HLD" || p.TypeCode == "PRF" {
			if p.ParticipantRole.ScopingEntity != nil {
				ent := p.ParticipantRole.ScopingEntity
				if ent.Desc != "" {
					r["payor"] = []interface{}{
						map[string]interface{}{"display": ent.Desc},
					}
				}
			}
			break
		}
	}

	// Member ID from entry relationships (coverage entitlement observation)
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode == "COMP" {
			for _, id := range rel.Entry.Id {
				if id.Root != "" || id.Extension != "" {
					memberId := id.Extension
					if memberId == "" {
						memberId = id.Root
					}
					r["subscriberId"] = memberId
					break
				}
			}
			break
		}
	}

	return r
}
