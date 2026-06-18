package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapPractitioners converts typed CDA careTeam section entries to FHIR Practitioner resources.
// Each care team member participant becomes one Practitioner (deduplicated by identifier or name).
//
// C-CDA care team members appear as participants within section entries.
// The participant's role contains the person details via playingEntity.
func MapPractitioners(entries []cdadocument.CDAEntry) []map[string]interface{} {
	var resources []map[string]interface{}
	seen := make(map[string]bool)
	idx := 1

	for _, entry := range entries {
		for _, p := range entry.Participants {
			if len(p.ParticipantRole.Ids) == 0 && p.ParticipantRole.PlayingEntity == nil {
				continue
			}
			var names []cdadocument.CDAName
			if p.ParticipantRole.PlayingEntity != nil {
				names = p.ParticipantRole.PlayingEntity.Names
			}
			key := dedupeKey(p.ParticipantRole.Ids, names)
			if seen[key] {
				continue
			}
			seen[key] = true

			r := buildPractitionerResource(idx, p)
			if len(r) > 2 {
				resources = append(resources, r)
				idx++
			}
		}
	}
	return resources
}

func buildPractitionerResource(idx int, p cdadocument.CDAParticipant) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Practitioner",
		"id":           fmt.Sprintf("practitioner-%d", idx),
	}

	// Names from playingEntity
	if p.ParticipantRole.PlayingEntity != nil && len(p.ParticipantRole.PlayingEntity.Names) > 0 {
		var names []interface{}
		for _, n := range p.ParticipantRole.PlayingEntity.Names {
			if hn := transforms.CDANameToFHIR(n); hn != nil {
				names = append(names, hn)
			}
		}
		if len(names) > 0 {
			r["name"] = names
		}
	}

	// Identifiers (NPI, etc.)
	if len(p.ParticipantRole.Ids) > 0 {
		var idents []interface{}
		for _, id := range p.ParticipantRole.Ids {
			if ident := transforms.IIToIdentifier(id); ident != nil {
				idents = append(idents, ident)
			}
		}
		if len(idents) > 0 {
			r["identifier"] = idents
		}
		// CDA identity for assembly-layer deduplication.
		embedCDAIds(r, p.ParticipantRole.Ids)
	}

	// Role code → qualification
	if p.ParticipantRole.Code.Code != "" {
		if cc := transforms.CDACodeToCodeableConcept(p.ParticipantRole.Code); cc != nil {
			r["qualification"] = []interface{}{
				map[string]interface{}{"code": cc},
			}
		}
	}

	// Telecoms
	if len(p.ParticipantRole.Telecoms) > 0 {
		var telecoms []interface{}
		for _, t := range p.ParticipantRole.Telecoms {
			if cp := transforms.CDATelecomToFHIR(t); cp != nil {
				telecoms = append(telecoms, cp)
			}
		}
		if len(telecoms) > 0 {
			r["telecom"] = telecoms
		}
	}

	return r
}

// dedupeKey builds a string key for a practitioner from identifiers or name.
func dedupeKey(ids []cdadocument.CDAII, names []cdadocument.CDAName) string {
	for _, id := range ids {
		if id.Root == "2.16.840.1.113883.4.6" { // NPI
			return "npi:" + id.Extension
		}
	}
	if len(names) > 0 {
		n := names[0]
		key := n.Family
		for _, g := range n.Given {
			key += ":" + g
		}
		return key
	}
	return fmt.Sprintf("%v", ids)
}
