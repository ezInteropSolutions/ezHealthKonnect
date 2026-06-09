// services/parsers/cda/section_vitals.go
// Vitals use an organizer/component pattern: one organizer per observation
// visit, containing multiple component/observation elements (one per vital sign).
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&vitalsProcessor{}) }

type vitalsProcessor struct{ baseSectionProcessor }

func (p *vitalsProcessor) SectionKey() string { return "vitalSigns" }

func (p *vitalsProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		organizer := entry.FindElement("organizer")
		if organizer == nil {
			continue
		}

		// Organizer-level timestamp applies to all observations in this group.
		observedAt := p.attr(organizer, "effectiveTime", "value")

		for _, comp := range organizer.SelectElements("component") {
			obs := comp.FindElement("observation")
			if obs == nil {
				continue
			}

			rec := make(map[string]interface{})

			if cv := p.extractCodedValue(obs, "code"); cv != nil {
				rec["vitalCode"] = cv["code"]
				rec["vitalDisplay"] = cv["display"]
			}

			// Value (PQ type: value + unit attributes)
			if valEl := obs.FindElement("value"); valEl != nil {
				p.setIfNotEmpty(rec, "value", valEl.SelectAttrValue("value", ""))
				p.setIfNotEmpty(rec, "unit", valEl.SelectAttrValue("unit", ""))
				// Some vitals use displayName (coded values like BP position)
				p.setIfNotEmpty(rec, "valueDisplay", valEl.SelectAttrValue("displayName", ""))
			}

			if observedAt != "" {
				rec["effectiveTime"] = observedAt
			} else {
				p.setIfNotEmpty(rec, "effectiveTime", p.attr(obs, "effectiveTime", "value"))
			}

			if len(rec) > 0 {
				result.Entries = append(result.Entries, rec)
			}
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
