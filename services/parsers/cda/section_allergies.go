// services/parsers/cda/section_allergies.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&allergiesProcessor{}) }

type allergiesProcessor struct{ baseSectionProcessor }

func (p *allergiesProcessor) SectionKey() string { return "allergiesAndIntolerances" }

func (p *allergiesProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	// Each allergy is wrapped in: entry/act/entryRelationship[@typeCode='SUBJ']/observation
	for _, entry := range p.findEntries(sectionEl) {
		obs := entry.FindElement("act/entryRelationship[@typeCode='SUBJ']/observation")
		if obs == nil {
			// Some documents use observation directly under entry.
			obs = entry.FindElement("observation")
		}
		if obs == nil {
			continue
		}

		rec := make(map[string]interface{})

		// Allergen: participant/participantRole/playingEntity/code
		if cv := p.extractCodedValue(obs, "participant/participantRole/playingEntity/code"); cv != nil {
			rec["medicationAllergyCode"] = cv["code"]
			rec["medicationAllergyDisplay"] = cv["display"]
			rec["medicationAllergySystem"] = cv["system"]
		}

		// Reaction
		if rv := p.extractCodedValue(obs, "entryRelationship[@typeCode='MFST']/observation/value"); rv != nil {
			rec["reaction"] = rv["display"]
			rec["reactionCode"] = rv["code"]
		}

		// Severity (Allergy Severity Observation template)
		if sv := p.extractCodedValue(obs, "entryRelationship/observation/value"); sv != nil {
			rec["severity"] = sv["display"]
		}

		// Clinical status
		p.setIfNotEmpty(rec, "status",
			p.attr(obs, "entryRelationship/observation/value[@codeSystem='2.16.840.1.113883.5.14']", "displayName"))
		if _, ok := rec["status"]; !ok {
			p.setIfNotEmpty(rec, "status", p.attr(obs, "entryRelationship/observation/value", "displayName"))
		}

		// Onset date
		p.setIfNotEmpty(rec, "onsetDate", p.attr(obs, "effectiveTime/low", "value"))

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
