// cda/document/section_parser.go
// sectionParser walks the CDA structured body and converts each <section>
// element into a typed CDASection, resolving the section key in this priority:
//   1. templateId lookup against ccda_2_1.json (GetSectionByTemplateID)
//   2. LOINC code on <code> child  (GetSectionByLOINC)
//   3. Title normalisation          (normalizeTitleToKey)
//
// SelectElements is used throughout — no FindElement calls in this file.

package cdadocument

import (
	"strings"

	cdaSchema "ezhealthkonnect/cda"

	"github.com/beevik/etree"
)

type sectionParser struct {
	schemaLoader *cdaSchema.CDASchemaLoader
}

// parseSections walks component/structuredBody/component/section hierarchy and
// returns all sections in document order.
func (sp *sectionParser) parseSections(root *etree.Element) []CDASection {
	if root == nil {
		return nil
	}

	// Locate the structured body.
	// CDA path: ClinicalDocument/component/structuredBody
	var body *etree.Element
	for _, compEl := range root.SelectElements("component") {
		if sb := compEl.SelectElement("structuredBody"); sb != nil {
			body = sb
			break
		}
	}
	if body == nil {
		return nil
	}

	ep := &entryParser{}
	var sections []CDASection

	for _, compEl := range body.SelectElements("component") {
		sectionEl := compEl.SelectElement("section")
		if sectionEl == nil {
			continue
		}
		sections = append(sections, sp.parseSection(sectionEl, ep))
	}

	return sections
}

// parseSection converts one <section> element into a CDASection.
func (sp *sectionParser) parseSection(el *etree.Element, ep *entryParser) CDASection {
	sec := CDASection{
		NullFlavor: el.SelectAttrValue("nullFlavor", ""),
	}

	// Collect ALL <templateId> elements — a section may carry more than one.
	for _, tidEl := range el.SelectElements("templateId") {
		if oid := tidEl.SelectAttrValue("root", ""); oid != "" {
			sec.TemplateIds = append(sec.TemplateIds, oid)
		}
	}

	sec.Key, sec.LoincCode = sp.resolveKey(el, sec.TemplateIds)

	if titleEl := el.SelectElement("title"); titleEl != nil {
		sec.Title = strings.TrimSpace(titleEl.Text())
	}

	var narrativeIndex map[string]string
	if textEl := el.SelectElement("text"); textEl != nil {
		narrativeIndex = buildNarrativeIndex(textEl)
		sec.NarrativeText = serializeElement(textEl)
	}

	sec.Entries = ep.parseEntries(el)

	// Resolve any "#id" reference placeholders against the narrative index.
	if len(narrativeIndex) > 0 {
		for i := range sec.Entries {
			resolveEntryRefs(&sec.Entries[i], narrativeIndex)
		}
	}

	return sec
}

// resolveKey determines the section key and LOINC code for a <section> element.
//
// Priority:
//  1. Each templateId OID is looked up in the schema; first match wins.
//  2. The <code> child's @code attribute is looked up as a LOINC code.
//  3. The section title is normalised into a camelCase key.
func (sp *sectionParser) resolveKey(el *etree.Element, templateIds []string) (key, loincCode string) {
	for _, oid := range templateIds {
		if sec := sp.schemaLoader.GetSectionByTemplateID(oid); sec != nil {
			return sec.Key, sec.LOINCCode
		}
	}

	if codeEl := el.SelectElement("code"); codeEl != nil {
		loinc := codeEl.SelectAttrValue("code", "")
		if loinc != "" {
			if sec := sp.schemaLoader.GetSectionByLOINC(loinc); sec != nil {
				return sec.Key, loinc
			}
			// LOINC code present but not in schema — store the code and fall through.
			codeSystem := codeEl.SelectAttrValue("codeSystem", "")
			if codeSystem == "2.16.840.1.113883.6.1" { // LOINC OID
				loincCode = loinc
			}
		}
	}

	return normalizeTitleToKey(el), loincCode
}

// ========================
// Title normalisation
// ========================

// titleKeyMap maps lowercase section titles to their ccda_2_1.json section keys.
// Used as the last-resort key resolver when no templateId or LOINC code matches.
var titleKeyMap = map[string]string{
	"allergies and adverse reactions":         "allergiesAndIntolerances",
	"allergies and intolerances":              "allergiesAndIntolerances",
	"allergies":                               "allergiesAndIntolerances",
	"medications":                             "medications",
	"active medications":                      "medications",
	"current medications":                     "medications",
	"discharge medications":                   "dischargeMedications",
	"admission medications":                   "admissionMedications",
	"problems":                                "problems",
	"problem list":                            "problems",
	"active problems":                         "problems",
	"vital signs":                             "vitalSigns",
	"results":                                 "results",
	"laboratory results":                      "results",
	"lab results":                             "results",
	"immunizations":                           "immunizations",
	"social history":                          "socialHistory",
	"encounters":                              "encounters",
	"procedures":                              "procedures",
	"family history":                          "familyHistory",
	"functional status":                       "functionalStatus",
	"assessment and plan":                     "assessmentAndPlan",
	"plan of care":                            "assessmentAndPlan",
	"plan of treatment":                       "assessmentAndPlan",
	"mental status":                           "mentalStatus",
	"payers":                                  "payersInsurance",
	"insurance providers":                     "payersInsurance",
	"goals":                                   "goals",
	"health concerns":                         "healthConcerns",
	"care teams":                              "careTeam",
	"care team":                               "careTeam",
	"advance directives":                      "advanceDirectives",
	"medical equipment":                       "medicalEquipment",
	"discharge diagnosis":                     "dischargeDiagnosis",
	"admission diagnosis":                     "admissionDiagnosis",
	"hospital course":                         "hospitalCourse",
	"discharge instructions":                  "dischargeInstructions",
	"reason for referral":                     "reasonForReferral",
	"chief complaint":                         "chiefComplaint",
	"history of present illness":              "historyOfPresentIllness",
	"physical examination":                    "physicalExamination",
	"past medical history":                    "pastMedicalHistory",
	"review of systems":                       "reviewOfSystems",
	"assessment":                              "assessment",
}

// normalizeTitleToKey converts a section's <title> text to a camelCase key.
// Returns "" when no title is present.
func normalizeTitleToKey(el *etree.Element) string {
	titleEl := el.SelectElement("title")
	if titleEl == nil {
		return ""
	}
	title := strings.ToLower(strings.TrimSpace(titleEl.Text()))

	if key, ok := titleKeyMap[title]; ok {
		return key
	}

	// Generic camelCase conversion: first word lowercase, subsequent words title-cased.
	words := strings.Fields(title)
	if len(words) == 0 {
		return ""
	}
	result := words[0]
	for _, w := range words[1:] {
		if len(w) > 0 {
			result += strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return result
}

// ========================
// Narrative reference resolution
// ========================

// buildNarrativeIndex walks the section's <text> narrative element and builds a
// map from each element's ID attribute value to its plain-text content. This
// allows <reference value="#id"/> anchors in structured entries to be resolved
// to the human-readable text displayed to the clinician.
//
// Deliberately does NOT infer anything from surrounding narrative structure
// (e.g. a sibling <caption> on an ancestor <item>) -- narrative <text> layout
// is not standardized across EHR vendors, so any rule built on top of one
// vendor's particular convention is a bet that won't generalize. Resolving
// "#id" to exactly the text at that id is the one standard, reliable part of
// this mechanism (CDA Release 2 narrative block §4.3.5.1); anything beyond
// that is per-vendor guesswork this codebase does not take on.
func buildNarrativeIndex(el *etree.Element) map[string]string {
	idx := make(map[string]string)
	walkForIDs(el, idx)
	return idx
}

func walkForIDs(el *etree.Element, idx map[string]string) {
	if id := el.SelectAttrValue("ID", ""); id != "" {
		if text := narrativeInnerText(el); text != "" {
			idx[id] = text
		}
	}
	for _, child := range el.ChildElements() {
		walkForIDs(child, idx)
	}
}

// narrativeInnerText returns the concatenated plain-text of el and all its
// descendants. It walks el.Child directly so that mixed content — text nodes
// interspersed with child elements — is captured correctly.
func narrativeInnerText(el *etree.Element) string {
	if el == nil {
		return ""
	}
	var sb strings.Builder
	for _, token := range el.Child {
		switch t := token.(type) {
		case *etree.CharData:
			if s := strings.TrimSpace(t.Data); s != "" {
				if sb.Len() > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(s)
			}
		case *etree.Element:
			if s := narrativeInnerText(t); s != "" {
				if sb.Len() > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(s)
			}
		}
	}
	return sb.String()
}

// resolveTextRef replaces a "#id" placeholder in *text (e.g. CDAEntry.Text)
// with the resolved narrative text. No-op when text does not start with "#".
func resolveTextRef(text *string, idx map[string]string) {
	if !strings.HasPrefix(*text, "#") {
		return
	}
	anchor := strings.TrimPrefix(*text, "#")
	if resolved, ok := idx[anchor]; ok {
		*text = resolved
	} else {
		*text = ""
	}
}

// resolveCodeRef replaces a "#id" placeholder in code.OriginalText with the
// resolved narrative text. No-op when OriginalText does not start with "#".
func resolveCodeRef(code *CDACode, idx map[string]string) {
	if !strings.HasPrefix(code.OriginalText, "#") {
		return
	}
	anchor := strings.TrimPrefix(code.OriginalText, "#")
	if text, ok := idx[anchor]; ok {
		code.OriginalText = text
	} else {
		code.OriginalText = ""
	}
	for i := range code.Translations {
		resolveCodeRef(&code.Translations[i], idx)
	}
}

// resolveEntryRefs resolves all "#id" reference placeholders in every CDACode
// field (and the entry's own Text field) within an entry, recursing into
// components and entryRelationships.
func resolveEntryRefs(entry *CDAEntry, idx map[string]string) {
	resolveCodeRef(&entry.Code, idx)
	resolveTextRef(&entry.Text, idx)
	if entry.Value != nil && entry.Value.Code != nil {
		resolveCodeRef(entry.Value.Code, idx)
	}
	if entry.RouteCode != nil {
		resolveCodeRef(entry.RouteCode, idx)
	}
	if entry.Consumable != nil {
		if mat := entry.Consumable.ManufacturedProduct.ManufacturedMaterial; mat != nil {
			resolveCodeRef(&mat.Code, idx)
		}
	}
	for i := range entry.Participants {
		resolveCodeRef(&entry.Participants[i].FunctionCode, idx)
		resolveCodeRef(&entry.Participants[i].ParticipantRole.Code, idx)
		if pe := entry.Participants[i].ParticipantRole.PlayingEntity; pe != nil {
			resolveCodeRef(&pe.Code, idx)
		}
		if se := entry.Participants[i].ParticipantRole.ScopingEntity; se != nil {
			resolveCodeRef(&se.Code, idx)
		}
	}
	for i := range entry.Performers {
		resolveCodeRef(&entry.Performers[i].FunctionCode, idx)
		resolveCodeRef(&entry.Performers[i].AssignedEntity.Code, idx)
	}
	for i := range entry.Components {
		resolveEntryRefs(&entry.Components[i], idx)
	}
	for i := range entry.EntryRelationships {
		resolveEntryRefs(&entry.EntryRelationships[i].Entry, idx)
	}
}

// ========================
// XML serialisation helper
// ========================

// serializeElement converts an *etree.Element to its full XML string representation.
// Returns "" on serialisation error; never panics.
func serializeElement(el *etree.Element) string {
	if el == nil {
		return ""
	}
	doc := etree.NewDocument()
	doc.SetRoot(el.Copy())
	s, err := doc.WriteToString()
	if err != nil {
		return ""
	}
	return s
}
