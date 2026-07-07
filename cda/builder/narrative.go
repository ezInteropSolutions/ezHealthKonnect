// cda/builder/narrative.go
//
// Builds each section's <text> narrative element directly as real etree
// child elements (not an HTML string). This differs from
// services/fhir_narrative's helpers, which return strings — appropriate
// there because FHIR's Narrative.div is a string VALUE inside a JSON
// resource. Here the narrative has to live inside the SAME XML tree as the
// rest of the document under construction, so building it as real elements
// avoids a parse-string-then-reimport round trip.
package builder

import (
	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// buildSectionNarrative creates a <text> element (appended to section)
// containing a simple table: one column per field that has a non-empty
// value in at least one entry, one row per entry.
func buildSectionNarrative(section *etree.Element, sec *cdaSchema.CDASectionDef, entries []map[string]interface{}) {
	text := section.CreateElement("text")

	if len(entries) == 0 {
		text.CreateElement("paragraph").SetText("No information available.")
		return
	}

	var cols []*cdaSchema.CDAFieldDef
	for _, f := range sec.Fields {
		for _, e := range entries {
			if s, ok := stringValue(e[f.Key]); ok && s != "" {
				cols = append(cols, f)
				break
			}
		}
	}
	if len(cols) == 0 {
		text.CreateElement("paragraph").SetText("No information available.")
		return
	}

	table := text.CreateElement("table")
	headerRow := table.CreateElement("thead").CreateElement("tr")
	for _, f := range cols {
		label := f.USCDIElement
		if label == "" {
			label = f.Key
		}
		headerRow.CreateElement("th").SetText(label)
	}

	tbody := table.CreateElement("tbody")
	for _, e := range entries {
		row := tbody.CreateElement("tr")
		for _, f := range cols {
			cell := row.CreateElement("td")
			if display, ok := stringValue(e[f.Key+"Display"]); ok {
				cell.SetText(display)
				continue
			}
			if v, ok := stringValue(e[f.Key]); ok {
				cell.SetText(v)
			}
		}
	}
}
