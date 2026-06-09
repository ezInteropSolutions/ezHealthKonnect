// services/parsers/cda/section_social_history.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&socialHistoryProcessor{}) }

type socialHistoryProcessor struct{ baseSectionProcessor }

func (p *socialHistoryProcessor) SectionKey() string { return "socialHistory" }

func (p *socialHistoryProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		obs := entry.FindElement("observation")
		if obs == nil {
			continue
		}

		rec := make(map[string]interface{})

		// Observation type (smoking, alcohol, employment, etc.)
		if cv := p.extractCodedValue(obs, "code"); cv != nil {
			rec["observationCode"] = cv["code"]
			rec["observationDisplay"] = cv["display"]
		}

		// Value — may be coded (smoking status), quantity (pack years), or string
		if valEl := obs.FindElement("value"); valEl != nil {
			if code := valEl.SelectAttrValue("code", ""); code != "" {
				rec["valueCode"] = code
				p.setIfNotEmpty(rec, "valueDisplay", valEl.SelectAttrValue("displayName", ""))
			} else if val := valEl.SelectAttrValue("value", ""); val != "" {
				rec["value"] = val
				p.setIfNotEmpty(rec, "valueUnit", valEl.SelectAttrValue("unit", ""))
			} else if txt := valEl.Text(); txt != "" {
				rec["value"] = txt
			}
		}

		p.setIfNotEmpty(rec, "effectiveTime", p.attr(obs, "effectiveTime", "value"))

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
