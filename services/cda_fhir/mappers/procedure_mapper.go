package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapProcedures converts typed CDA procedure section entries to FHIR Procedure resources.
//
// C-CDA structure: procedure, observation (surgical), or act entry
//   - Code from entry.Code (CPT, SNOMED)
//   - Status from entry.StatusCode
//   - Performed date from entry.EffectiveTime
//   - Performer participants
func MapProcedures(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildProcedureResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildProcedureResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Procedure",
		"id":           fmt.Sprintf("procedure-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.ProcedureStatusToFHIR(entry.StatusCode)

	// Code
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["code"] = cc
	}

	// Performed date/period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["performedPeriod"] = period
	} else if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["performedDateTime"] = dt
	}

	// Performer from participants
	var performers []interface{}
	for _, p := range entry.Participants {
		if p.TypeCode == "PRF" || p.TypeCode == "SPRF" {
			perf := map[string]interface{}{}
			if p.ParticipantRole.PlayingEntity != nil && len(p.ParticipantRole.PlayingEntity.Names) > 0 {
				if hn := transforms.CDANameToFHIR(p.ParticipantRole.PlayingEntity.Names[0]); hn != nil {
					perf["actor"] = map[string]interface{}{
						"display": buildDisplayFromName(p.ParticipantRole.PlayingEntity.Names[0]),
					}
				}
			}
			if len(perf) > 0 {
				performers = append(performers, perf)
			}
		}
	}
	if len(performers) > 0 {
		r["performer"] = performers
	}

	// Body site from <targetSiteCode> — a direct child of <procedure> in the
	// base CDA R2 schema (a sibling of <code>/<statusCode>), not a nested
	// entryRelationship. Only set for the Procedure Activity Procedure
	// template variant (EntryType=="procedure"); the Observation/Act variants
	// have no targetSiteCode in the base schema, so this is nil for those.
	if entry.TargetSiteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.TargetSiteCode); cc != nil {
			r["bodySite"] = []interface{}{cc}
		}
	}

	return r
}

// buildDisplayFromName produces a simple display string from a CDAName.
func buildDisplayFromName(n cdadocument.CDAName) string {
	s := ""
	for _, g := range n.Given {
		if g != "" {
			s += g + " "
		}
	}
	s += n.Family
	if s == "" {
		s = n.Prefix + " " + n.Suffix
	}
	return s
}
