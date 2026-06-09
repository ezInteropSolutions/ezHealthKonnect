// services/parsers/cda/section_medications.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&medicationsProcessor{}) }

type medicationsProcessor struct{ baseSectionProcessor }

func (p *medicationsProcessor) SectionKey() string { return "medications" }

func (p *medicationsProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		sa := entry.FindElement("substanceAdministration")
		if sa == nil {
			continue
		}

		rec := make(map[string]interface{})

		// Drug code
		if cv := p.extractCodedValue(sa, "consumable/manufacturedProduct/manufacturedMaterial/code"); cv != nil {
			rec["drugCode"] = cv["code"]
			rec["drugDisplay"] = cv["display"]
			rec["drugSystem"] = cv["system"]
		}

		// Dose
		if doseEl := sa.FindElement("doseQuantity"); doseEl != nil {
			p.setIfNotEmpty(rec, "doseValue", doseEl.SelectAttrValue("value", ""))
			p.setIfNotEmpty(rec, "doseUnit", doseEl.SelectAttrValue("unit", ""))
		}

		// Route
		if rv := p.extractCodedValue(sa, "routeCode"); rv != nil {
			rec["route"] = rv["display"]
			rec["routeCode"] = rv["code"]
		}

		// Status
		p.setIfNotEmpty(rec, "status", p.attr(sa, "statusCode", "code"))

		// Date range — handle both IVL_TS and simple effectiveTime
		if low := p.attr(sa, "effectiveTime/low", "value"); low != "" {
			rec["startDate"] = low
		} else {
			p.setIfNotEmpty(rec, "startDate", p.attr(sa, "effectiveTime", "value"))
		}
		p.setIfNotEmpty(rec, "stopDate", p.attr(sa, "effectiveTime/high", "value"))

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
