// services/parsers/cda/section_procedures.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&proceduresProcessor{}) }

type proceduresProcessor struct{ baseSectionProcessor }

func (p *proceduresProcessor) SectionKey() string { return "procedures" }

func (p *proceduresProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		// C-CDA procedures may be act, observation, or procedure elements.
		var procEl *etree.Element
		for _, tag := range []string{"procedure", "act", "observation"} {
			if el := entry.FindElement(tag); el != nil {
				procEl = el
				break
			}
		}
		if procEl == nil {
			continue
		}

		rec := make(map[string]interface{})

		if cv := p.extractCodedValue(procEl, "code"); cv != nil {
			rec["procedureCode"] = cv["code"]
			rec["procedureDisplay"] = cv["display"]
			rec["procedureSystem"] = cv["system"]
		}

		// Status
		p.setIfNotEmpty(rec, "status", p.attr(procEl, "statusCode", "code"))

		// Date — try low first (for date ranges), then direct value
		if low := p.attr(procEl, "effectiveTime/low", "value"); low != "" {
			rec["effectiveTime"] = low
		} else {
			p.setIfNotEmpty(rec, "effectiveTime", p.attr(procEl, "effectiveTime", "value"))
		}

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
