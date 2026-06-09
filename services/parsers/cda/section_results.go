// services/parsers/cda/section_results.go
// Lab results use the same organizer/component pattern as vitals but the
// value element may carry a PQ (quantity), ST (string), or CD (coded) type.
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&resultsProcessor{}) }

type resultsProcessor struct{ baseSectionProcessor }

func (p *resultsProcessor) SectionKey() string { return "results" }

func (p *resultsProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		organizer := entry.FindElement("organizer")
		if organizer == nil {
			// Some documents use standalone observations for results.
			if obs := entry.FindElement("observation"); obs != nil {
				if rec := p.extractResultObservation(obs); len(rec) > 0 {
					result.Entries = append(result.Entries, rec)
				}
			}
			continue
		}

		observedAt := p.attr(organizer, "effectiveTime", "value")

		for _, comp := range organizer.SelectElements("component") {
			obs := comp.FindElement("observation")
			if obs == nil {
				continue
			}
			rec := p.extractResultObservation(obs)
			if observedAt != "" {
				rec["effectiveTime"] = observedAt
			}
			if len(rec) > 0 {
				result.Entries = append(result.Entries, rec)
			}
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}

func (p *resultsProcessor) extractResultObservation(obs *etree.Element) map[string]interface{} {
	rec := make(map[string]interface{})

	if cv := p.extractCodedValue(obs, "code"); cv != nil {
		rec["testCode"] = cv["code"]
		rec["testDisplay"] = cv["display"]
		rec["testSystem"] = cv["system"]
	}

	if valEl := obs.FindElement("value"); valEl != nil {
		// Numeric result (PQ)
		if v := valEl.SelectAttrValue("value", ""); v != "" {
			rec["resultValue"] = v
		}
		// Unit
		if u := valEl.SelectAttrValue("unit", ""); u != "" {
			rec["resultUnit"] = u
		}
		// Coded or string result
		if d := valEl.SelectAttrValue("displayName", ""); d != "" {
			rec["resultDisplay"] = d
		}
		if code := valEl.SelectAttrValue("code", ""); code != "" {
			rec["resultCode"] = code
		}
		// Free text result
		if txt := valEl.Text(); txt != "" {
			rec["resultText"] = txt
		}
	}

	// Reference range
	if rrEl := obs.FindElement("referenceRange/observationRange/value"); rrEl != nil {
		low := rrEl.SelectAttrValue("value", "")
		high := p.attr(obs, "referenceRange/observationRange/value/high", "value")
		if low != "" {
			rec["referenceRangeLow"] = low
		}
		if high != "" {
			rec["referenceRangeHigh"] = high
		}
	}

	p.setIfNotEmpty(rec, "resultStatus", p.attr(obs, "statusCode", "code"))
	p.setIfNotEmpty(rec, "effectiveTime", p.attr(obs, "effectiveTime", "value"))

	return rec
}
