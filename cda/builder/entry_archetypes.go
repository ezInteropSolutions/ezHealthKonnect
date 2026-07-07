// cda/builder/entry_archetypes.go
//
// buildEntry is the single, schema-driven entry constructor used for every
// CCD section. It turned out not to need per-archetype functions
// (act+SUBJ-observation, substanceAdministration, organizer, plain
// observation, etc. all reduce to the same "create root anchor, create
// optional nested anchor, write every field via its own XPath" shape once
// WriteAtXPath exists). The only genuinely archetype-specific knowledge
// left is the classCode/moodCode/statusCode boilerplate each root/nested XML
// tag needs per the C-CDA 2.1 IG — a small, finite lookup table, not
// per-section Go code.
package builder

import (
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// tagBoilerplate is the fixed classCode/moodCode/statusCode every CDA entry
// root or nested-observation tag needs per the C-CDA 2.1 IG, keyed by the
// tag's own name (the last path segment of EntryElementPath/
// ObservationElementPath). StatusCode is only injected when non-empty and
// no statusCode is already present — several tags (substanceAdministration,
// encounter, procedure) get their real statusCode from a mapped field
// instead (e.g. medications.status), so no synthetic value is forced there.
var tagBoilerplate = map[string]struct {
	ClassCode  string
	MoodCode   string
	StatusCode string
}{
	"act":                     {"ACT", "EVN", "active"},
	"observation":             {"OBS", "EVN", "completed"},
	"substanceAdministration": {"SBADM", "EVN", ""},
	"organizer":               {"CLUSTER", "EVN", "completed"},
	"encounter":               {"ENC", "EVN", ""},
	"procedure":               {"PROC", "EVN", ""},
	"supply":                  {"SPLY", "EVN", ""},
}

// buildEntry builds one <entry> element (appended to sectionEl) from a
// single canonical record, using sec's schema metadata for templateIds and
// every field's own XPath (via WriteAtXPath) for values — no per-section Go
// logic. Canonical field keys follow generic_section_processor.go's exact
// convention: field.Key (primary), field.Key+"Display", +"System", +"Unit",
// +"Family" for the companion XPath variants.
func buildEntry(sectionEl *etree.Element, sec *cdaSchema.CDASectionDef, record map[string]interface{}) *etree.Element {
	entryEl := sectionEl.CreateElement("entry")
	entryEl.CreateAttr("typeCode", "DRIV")

	if sec.EntryElementPath != "" {
		rootEl := WriteAtXPath(entryEl, stripEntryPrefix(sec.EntryElementPath), "")
		applyTagBoilerplate(rootEl, lastSegmentTag(sec.EntryElementPath))
		if sec.EntryStatusCodeOverride != "" {
			if sc := rootEl.SelectElement("statusCode"); sc != nil {
				sc.CreateAttr("code", sec.EntryStatusCodeOverride)
			}
		}
		if sec.EntryTemplateID != "" {
			injectTemplateID(rootEl, sec.EntryTemplateID, sec.EntryTemplateIDExt)
		}

		if sec.ObservationElementPath != "" {
			obsEl := WriteAtXPath(entryEl, stripEntryPrefix(sec.ObservationElementPath), "")
			applyTagBoilerplate(obsEl, lastSegmentTag(sec.ObservationElementPath))
			if sec.ObsTemplateID != "" {
				injectTemplateID(obsEl, sec.ObsTemplateID, sec.ObsTemplateIDExt)
			}
		}
	}

	for _, field := range sec.Fields {
		writeFieldValue(entryEl, field, record)
	}

	for _, anchor := range sec.StructuralTemplateIDs {
		if el, found := TryFindAtXPath(entryEl, stripEntryPrefix(anchor.Path)); found {
			applyTagBoilerplate(el, lastSegmentTag(anchor.Path))
			injectTemplateID(el, anchor.TemplateID, anchor.TemplateIDExt)
		}
	}

	return entryEl
}

// writeFieldValue writes a single field's primary + companion (Display/
// System/Unit/Family) values via WriteAtXPath, mirroring
// generic_section_processor.go's extractValueByXPath field-key convention
// exactly, in reverse.
func writeFieldValue(entryEl *etree.Element, field *cdaSchema.CDAFieldDef, record map[string]interface{}) {
	if v, ok := stringValue(record[field.Key]); ok && field.XPath != "" {
		WriteAtXPath(entryEl, stripEntryPrefix(field.XPath), v)
	}
	if field.XPathDisplay != "" {
		if v, ok := stringValue(record[field.Key+"Display"]); ok {
			WriteAtXPath(entryEl, stripEntryPrefix(field.XPathDisplay), v)
		}
	}
	if field.XPathSystem != "" {
		if v, ok := stringValue(record[field.Key+"System"]); ok {
			WriteAtXPath(entryEl, stripEntryPrefix(field.XPathSystem), v)
		}
	}
	if field.XPathUnit != "" {
		if v, ok := stringValue(record[field.Key+"Unit"]); ok {
			WriteAtXPath(entryEl, stripEntryPrefix(field.XPathUnit), v)
		}
	}
	if field.XPathFamily != "" {
		if v, ok := stringValue(record[field.Key+"Family"]); ok {
			WriteAtXPath(entryEl, stripEntryPrefix(field.XPathFamily), v)
		}
	}
}

func applyTagBoilerplate(el *etree.Element, tag string) {
	bp, ok := tagBoilerplate[tag]
	if !ok {
		return
	}
	if el.SelectAttrValue("classCode", "") == "" {
		el.CreateAttr("classCode", bp.ClassCode)
	}
	if el.SelectAttrValue("moodCode", "") == "" {
		el.CreateAttr("moodCode", bp.MoodCode)
	}
	if bp.StatusCode != "" && el.SelectElement("statusCode") == nil {
		el.CreateElement("statusCode").CreateAttr("code", bp.StatusCode)
	}
}

// injectTemplateID adds (or, if a field write already created one via a
// "[templateId/@root='...']" disambiguating predicate — see
// StructuralTemplateAnchor — completes) a <templateId root extension>
// child. Reuses an existing <templateId> with a matching root instead of
// appending a duplicate, since exactly one is ever valid per element.
func injectTemplateID(el *etree.Element, oid string, extension string) {
	var tid *etree.Element
	for _, existing := range el.SelectElements("templateId") {
		if existing.SelectAttrValue("root", "") == oid {
			tid = existing
			break
		}
	}
	if tid == nil {
		tid = el.CreateElement("templateId")
		tid.CreateAttr("root", oid)
	}
	if extension != "" && tid.SelectAttrValue("extension", "") == "" {
		tid.CreateAttr("extension", extension)
	}
}

// stripEntryPrefix mirrors generic_section_processor.go's helper of the same
// name exactly (kept as a private copy here since builder and cdaparser are
// separate packages and this one-line helper isn't worth a shared import).
func stripEntryPrefix(xpath string) string {
	const prefix = "entry/"
	if strings.HasPrefix(xpath, prefix) {
		return xpath[len(prefix):]
	}
	return xpath
}

// lastSegmentTag returns the tag name of the LAST path segment (stripping
// any "entry/" prefix and any "[predicate]"), e.g.
// "entry/act/entryRelationship[@typeCode='SUBJ']/observation" -> "observation".
func lastSegmentTag(path string) string {
	segments := splitPathSegments(stripEntryPrefix(path))
	if len(segments) == 0 {
		return ""
	}
	last := segments[len(segments)-1]
	if i := strings.Index(last, "["); i != -1 {
		return last[:i]
	}
	return last
}

// stringValue converts a canonical record value to a string, returning
// ok=false for nil/missing/empty values so callers skip writing them.
func stringValue(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
