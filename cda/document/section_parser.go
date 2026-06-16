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

	if textEl := el.SelectElement("text"); textEl != nil {
		sec.NarrativeText = serializeElement(textEl)
	}

	sec.Entries = ep.parseEntries(el)

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
