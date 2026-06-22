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

	// CDA's Coverage Activity statusCode is unconditionally fixed to
	// "completed" (CONF:1198-19094, verified against build.fhir.org/ig/HL7/
	// CDA-ccda-2.2's definitions page 2026-06-22) -- there is no CDA-side
	// signal to derive a real-world active/inactive coverage status from
	// (CDA act-status and FHIR Coverage.status are different axes
	// entirely), so "active" is the only defensible default, not a gap.
	r["status"] = "active"

	if patientRef != "" {
		r["beneficiary"] = ref(patientRef)
		r["subscriber"] = ref(patientRef)
	}

	// Coverage type from entry code; correct LOINC display when needed.
	if cc := transforms.CoverageTypeToCodeableConcept(entry.Code); cc != nil {
		r["type"] = cc
	}

	// Period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["period"] = period
	}

	// Payer: first try outer HLD/PRF participants, then fall back to the nested
	// policy-activity act's performers (C-CDA 2.1 Coverage Activity template places
	// the payer organisation inside entryRelationship[COMP]/act/performer).
	payorFound := false
	for _, p := range entry.Participants {
		if p.TypeCode == "HLD" || p.TypeCode == "PRF" {
			if p.ParticipantRole.ScopingEntity != nil && p.ParticipantRole.ScopingEntity.Desc != "" {
				r["payor"] = []interface{}{map[string]interface{}{"display": p.ParticipantRole.ScopingEntity.Desc}}
				payorFound = true
			}
			break
		}
	}
	if !payorFound {
		for _, rel := range entry.EntryRelationships {
			if rel.TypeCode != "COMP" {
				continue
			}
			for _, perf := range rel.Entry.Performers {
				org := perf.AssignedEntity.RepresentedOrganization
				if org != nil && len(org.Names) > 0 && org.Names[0] != "" {
					r["payor"] = []interface{}{map[string]interface{}{"display": org.Names[0]}}
					payorFound = true
					break
				}
			}
			if payorFound {
				break
			}
		}
	}
	// us-core-coverage requires payor; use a placeholder when truly unavailable.
	if !payorFound {
		r["payor"] = []interface{}{map[string]interface{}{"display": "Unknown"}}
	}

	// Relationship: subscriber-to-patient relationship required by us-core-coverage.
	// When subscriber == beneficiary (the patient is their own subscriber), it is "self".
	r["relationship"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
				"code":    "self",
				"display": "Self",
			},
		},
	}

	// Member / subscriber ID from COMP policy-activity entry identifiers.
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
