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
	"fmt"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// buildSectionNarrative creates a <text> element (appended to section)
// containing a simple table: one column per field that has a non-empty
// value in at least one entry (across BOTH the section's own primary
// entries and any AlternateArchetypes' entries), one row per entry. Each
// row gets a unique @ID (sectionKey-N) — the anchor a "_narrativeRef"
// synthetic field (see buildSectionElement) targets when a real entry field
// needs to write <originalText><reference value="#id"/></originalText> or a
// bare <reference value="#id"/> back to its own narrative row (CONF:16326,
// 15593, 7658/15908, 8719, etc. — every C-CDA "SHOULD contain reference"
// warning this session's Round 23 found shares this one root cause: the
// narrative never had anything with an @ID for an entry to point at).
//
// Returns primaryRowIDs (parallel to entries) and altRowIDs (parallel to
// altEntries, each altRowIDs[i] parallel to altEntries[i]) so the caller can
// inject the right "#id" into each record before building its entry. Both
// are nil when the section has no information at all (the existing "No
// information available" fallback).
func buildSectionNarrative(section *etree.Element, sec *cdaSchema.CDASectionDef, entries []map[string]interface{}, altEntries [][]map[string]interface{}) (primaryRowIDs []string, altRowIDs [][]string) {
	text := section.CreateElement("text")

	total := len(entries)
	for _, ae := range altEntries {
		total += len(ae)
	}
	if total == 0 {
		text.CreateElement("paragraph").SetText("No information available.")
		return nil, nil
	}

	var cols []*cdaSchema.CDAFieldDef
	addCols := func(fields []*cdaSchema.CDAFieldDef, es []map[string]interface{}) {
		for _, f := range fields {
			for _, e := range es {
				if s, ok := stringValue(e[f.Key]); ok && s != "" {
					cols = append(cols, f)
					break
				}
			}
		}
	}
	addCols(sec.Fields, entries)
	for i, alt := range sec.AlternateArchetypes {
		if i < len(altEntries) {
			addCols(alt.Fields, altEntries[i])
		}
	}
	if len(cols) == 0 {
		text.CreateElement("paragraph").SetText("No information available.")
		return nil, nil
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
	rowNum := 0
	writeRow := func(e map[string]interface{}) string {
		rowNum++
		rowID := fmt.Sprintf("%s-%d", sec.Key, rowNum)
		row := tbody.CreateElement("tr")
		row.CreateAttr("ID", rowID)
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
		return rowID
	}

	primaryRowIDs = make([]string, len(entries))
	for i, e := range entries {
		primaryRowIDs[i] = writeRow(e)
	}
	altRowIDs = make([][]string, len(altEntries))
	for i, ae := range altEntries {
		ids := make([]string, len(ae))
		for j, e := range ae {
			ids[j] = writeRow(e)
		}
		altRowIDs[i] = ids
	}

	return primaryRowIDs, altRowIDs
}
