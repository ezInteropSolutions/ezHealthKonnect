// services/parsers/cda/section_encounters.go
package cdaparser

import (
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&encountersProcessor{}) }

type encountersProcessor struct{ baseSectionProcessor }

func (p *encountersProcessor) SectionKey() string { return "encounters" }

func (p *encountersProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		enc := entry.FindElement("encounter")
		if enc == nil {
			continue
		}

		rec := make(map[string]interface{})

		if cv := p.extractCodedValue(enc, "code"); cv != nil {
			rec["encounterCode"] = cv["code"]
			rec["encounterDisplay"] = cv["display"]
		}

		// Effective time — may be a single value or IVL_TS
		if et := p.attr(enc, "effectiveTime", "value"); et != "" {
			rec["effectiveTime"] = et
		} else {
			p.setIfNotEmpty(rec, "effectiveTime", p.attr(enc, "effectiveTime/low", "value"))
		}

		// Provider name
		nameEl := enc.FindElement("performer/assignedEntity/assignedPerson/name")
		if nameEl != nil {
			given := p.text(nameEl, "given")
			family := p.text(nameEl, "family")
			parts := []string{}
			if given != "" {
				parts = append(parts, given)
			}
			if family != "" {
				parts = append(parts, family)
			}
			if len(parts) > 0 {
				rec["performerName"] = strings.Join(parts, " ")
			}
		}

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
