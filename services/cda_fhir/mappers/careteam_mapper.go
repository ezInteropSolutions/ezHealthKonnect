// services/cda_fhir/mappers/careteam_mapper.go
//
// Maps the C-CDA Care Team Organizer (templateId 2.16.840.1.113883.10.20.22.4.500)
// to a FHIR CareTeam resource, per the C-CDA on FHIR IG: each participant:lead
// becomes a CareTeam.participant, with its sdtc:functionCode as the
// participant's role and a Practitioner resource (built the same way the
// existing Practitioner mapper builds one) as the participant's member.
package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapCareTeam converts typed CDA Care Team Organizer entries to FHIR CareTeam
// resources (plus one Practitioner per team member). One CareTeam resource is
// produced per organizer entry — a document can carry more than one team
// (e.g. a primary care team and a specialty team).
func MapCareTeam(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for _, entry := range entries {
		practitioners, participants := buildCareTeamParticipants(entry.Participants)
		resources = append(resources, practitioners...)

		if ct := buildCareTeamResource(len(resources)+1, entry, participants, patientRef); ct != nil {
			resources = append(resources, ct)
		}
	}
	return resources
}

// buildCareTeamParticipants builds one Practitioner per care team member
// participant and the corresponding CareTeam.participant entries referencing them.
func buildCareTeamParticipants(cdaParticipants []cdadocument.CDAParticipant) (practitioners []map[string]interface{}, ctParticipants []interface{}) {
	idx := 1
	for _, p := range cdaParticipants {
		if len(p.ParticipantRole.Ids) == 0 && p.ParticipantRole.PlayingEntity == nil {
			continue
		}

		pr := buildPractitionerResource(idx, p)
		if len(pr) <= 2 {
			continue
		}
		practitioners = append(practitioners, pr)
		idx++

		ctp := map[string]interface{}{
			"member": ref(fmt.Sprintf("Practitioner/%s", pr["id"])),
		}
		if p.FunctionCode.Code != "" {
			if cc := transforms.CDACodeToCodeableConcept(p.FunctionCode); cc != nil {
				ctp["role"] = []interface{}{cc}
			}
		}
		ctParticipants = append(ctParticipants, ctp)
	}
	return practitioners, ctParticipants
}

// buildCareTeamResource assembles the CareTeam resource itself. Returns nil
// when no usable participants were found (a CareTeam with zero members isn't
// worth emitting).
func buildCareTeamResource(idx int, entry cdadocument.CDAEntry, participants []interface{}, patientRef string) map[string]interface{} {
	if len(participants) == 0 {
		return nil
	}
	r := map[string]interface{}{
		"resourceType": "CareTeam",
		"id":           fmt.Sprintf("careTeam-%d", idx),
		"status":       transforms.CareTeamStatusToFHIR(entry.StatusCode),
		"participant":  participants,
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["category"] = []interface{}{cc}
	}
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["period"] = period
	}
	return r
}
