// services/parsers/cda/section_problems.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&problemsProcessor{}) }

type problemsProcessor struct{ baseSectionProcessor }

func (p *problemsProcessor) SectionKey() string { return "problems" }

func (p *problemsProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		// Problem Concern Act wraps a Problem Observation
		obs := entry.FindElement("act/entryRelationship[@typeCode='SUBJ']/observation")
		if obs == nil {
			obs = entry.FindElement("observation")
		}
		if obs == nil {
			continue
		}

		rec := make(map[string]interface{})

		// Condition code (value element for problems)
		if cv := p.extractCodedValue(obs, "value"); cv != nil {
			rec["conditionCode"] = cv["code"]
			rec["conditionDisplay"] = cv["display"]
			rec["conditionSystem"] = cv["system"]
		}

		// Problem status
		p.setIfNotEmpty(rec, "status",
			p.attr(obs, "entryRelationship/observation/value", "displayName"))

		// Onset / resolution dates
		p.setIfNotEmpty(rec, "onsetDate", p.attr(obs, "effectiveTime/low", "value"))
		p.setIfNotEmpty(rec, "resolutionDate", p.attr(obs, "effectiveTime/high", "value"))

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
