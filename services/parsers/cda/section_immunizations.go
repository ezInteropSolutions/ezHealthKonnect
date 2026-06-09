// services/parsers/cda/section_immunizations.go
package cdaparser

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func init() { RegisterSection(&immunizationsProcessor{}) }

type immunizationsProcessor struct{ baseSectionProcessor }

func (p *immunizationsProcessor) SectionKey() string { return "immunizations" }

func (p *immunizationsProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		sa := entry.FindElement("substanceAdministration")
		if sa == nil {
			continue
		}

		rec := make(map[string]interface{})

		// Vaccine code — same path as medications but different code system (CVX/NDC)
		if cv := p.extractCodedValue(sa, "consumable/manufacturedProduct/manufacturedMaterial/code"); cv != nil {
			rec["vaccineCode"] = cv["code"]
			rec["vaccineDisplay"] = cv["display"]
			rec["vaccineSystem"] = cv["system"]
		}

		// Administration date
		if et := p.attr(sa, "effectiveTime", "value"); et != "" {
			rec["administrationDate"] = et
		}

		// Lot number
		p.setIfNotEmpty(rec, "lotNumber",
			p.text(sa, "consumable/manufacturedProduct/manufacturedMaterial/lotNumberText"))

		// Status
		p.setIfNotEmpty(rec, "status", p.attr(sa, "statusCode", "code"))

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}
