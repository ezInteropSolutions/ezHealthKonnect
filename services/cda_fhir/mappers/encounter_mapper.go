package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapEncounters converts typed CDA encounter section entries to FHIR Encounter resources.
//
// C-CDA structure: encounter entry
//   - Encounter type from entry.Code
//   - Status from entry.StatusCode
//   - Period from entry.EffectiveTime
//   - Performer participants → Encounter.participant
func MapEncounters(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildEncounterResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildEncounterResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Encounter",
		"id":           fmt.Sprintf("encounter-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.EncounterStatusToFHIR(entry.StatusCode)

	// Encounter class from code (inpatient, ambulatory, etc.)
	if entry.Code.Code != "" {
		class := map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":   entry.Code.Code,
		}
		// FHIR forbids an empty-string primitive (ele-1) — only set display
		// when the source CDA actually carried one.
		if entry.Code.DisplayName != "" {
			class["display"] = entry.Code.DisplayName
		}
		r["class"] = class
		// Also set type
		if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
			r["type"] = []interface{}{cc}
		}
	}

	// Period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["period"] = period
	}

	// Participants (performers → encounter.participant)
	var participants []interface{}
	for _, p := range entry.Participants {
		part := buildEncounterParticipant(p)
		if part != nil {
			participants = append(participants, part)
		}
	}
	if len(participants) > 0 {
		r["participant"] = participants
	}

	// Location from entry relationships
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode == "COMP" {
			for _, p := range rel.Entry.Participants {
				if p.TypeCode == "LOC" && p.ParticipantRole.PlayingEntity != nil {
					locName := ""
					if len(p.ParticipantRole.PlayingEntity.Names) > 0 {
						locName = p.ParticipantRole.PlayingEntity.Names[0].Family
					}
					if locName == "" {
						locName = p.ParticipantRole.PlayingEntity.Code.DisplayName
					}
					if locName != "" {
						r["location"] = []interface{}{
							map[string]interface{}{
								"location": map[string]interface{}{"display": locName},
							},
						}
					}
					break
				}
			}
		}
	}

	return r
}

func buildEncounterParticipant(p cdadocument.CDAParticipant) map[string]interface{} {
	if p.ParticipantRole.PlayingEntity == nil && p.TypeCode == "" {
		return nil
	}
	part := map[string]interface{}{}
	if p.TypeCode != "" {
		part["type"] = []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType",
						"code":   p.TypeCode,
					},
				},
			},
		}
	}
	if p.ParticipantRole.PlayingEntity != nil && len(p.ParticipantRole.PlayingEntity.Names) > 0 {
		display := buildDisplayFromName(p.ParticipantRole.PlayingEntity.Names[0])
		if display != "" {
			part["individual"] = map[string]interface{}{
				"display": display,
			}
		}
	}
	if len(part) == 0 {
		return nil
	}
	return part
}
