// services/parsers/cda/cda_header_parser.go
// CDAHeaderParser extracts patient demographics and document metadata from
// the CDA ClinicalDocument header. Output fields use USCDI v3 element names
// so the result integrates cleanly with the FHIR mapper.

package cdaparser

import (
	"strings"

	"github.com/beevik/etree"
)

// CDAHeaderParser is stateless — construct once and share.
type CDAHeaderParser struct{}

// NewCDAHeaderParser returns a ready-to-use header parser.
func NewCDAHeaderParser() *CDAHeaderParser { return &CDAHeaderParser{} }

// ParseResult holds the structured output from the document header.
type ParseResult struct {
	Patient  map[string]interface{} `json:"patient"`
	Document map[string]interface{} `json:"document"`
	Author   map[string]interface{} `json:"author"`
}

// Parse extracts demographics and metadata from the ClinicalDocument root element.
// Returns a ParseResult where each field key matches the corresponding USCDI element
// name defined in uscdi/vocabulary_types.go.
func (h *CDAHeaderParser) Parse(root *etree.Element) *ParseResult {
	return &ParseResult{
		Patient:  h.parsePatient(root),
		Document: h.parseDocument(root),
		Author:   h.parseAuthor(root),
	}
}

// =====================================
// Patient (recordTarget)
// =====================================

func (h *CDAHeaderParser) parsePatient(root *etree.Element) map[string]interface{} {
	p := make(map[string]interface{})

	role := root.FindElement("recordTarget/patientRole")
	if role == nil {
		return p
	}

	pat := role.FindElement("patient")
	if pat != nil {
		// Name (use="L" preferred)
		name := h.selectName(pat)
		if name != nil {
			h.setStr(p, "firstName", h.elementText(name, "given"))
			h.setStr(p, "lastName", h.elementText(name, "family"))
			h.setStr(p, "middleName", h.elementText(name, "given[2]"))
			h.setStr(p, "suffix", h.elementText(name, "suffix"))
		}

		// Demographics
		h.setStr(p, "dateOfBirth", h.attrVal(pat, "birthTime", "value"))
		h.setStr(p, "sex", h.attrVal(pat, "administrativeGenderCode", "code"))
		h.setStr(p, "sexDisplay", h.attrVal(pat, "administrativeGenderCode", "displayName"))

		// Race and ethnicity
		h.setStr(p, "race", h.attrVal(pat, "raceCode", "code"))
		h.setStr(p, "raceDisplay", h.attrVal(pat, "raceCode", "displayName"))
		h.setStr(p, "ethnicity", h.attrVal(pat, "ethnicGroupCode", "code"))
		h.setStr(p, "ethnicityDisplay", h.attrVal(pat, "ethnicGroupCode", "displayName"))

		// Preferred language
		h.setStr(p, "preferredLanguage", h.attrVal(pat, "languageCommunication/languageCode", "code"))
	}

	// Address
	if addrEl := role.FindElement("addr"); addrEl != nil {
		addr := make(map[string]interface{})
		h.setStr(addr, "street", strings.TrimSpace(h.elementText(addrEl, "streetAddressLine")))
		h.setStr(addr, "city", strings.TrimSpace(h.elementText(addrEl, "city")))
		h.setStr(addr, "state", strings.TrimSpace(h.elementText(addrEl, "state")))
		h.setStr(addr, "postalCode", strings.TrimSpace(h.elementText(addrEl, "postalCode")))
		h.setStr(addr, "country", strings.TrimSpace(h.elementText(addrEl, "country")))
		if len(addr) > 0 {
			p["address"] = addr
		}
	}

	// Telecom — separate phone and email
	for _, tel := range role.SelectElements("telecom") {
		value := tel.SelectAttrValue("value", "")
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "tel:") {
			h.setStr(p, "phone", strings.TrimPrefix(value, "tel:"))
		} else if strings.HasPrefix(value, "mailto:") {
			h.setStr(p, "email", strings.TrimPrefix(value, "mailto:"))
		}
	}

	return p
}

// selectName returns the <name use="L"> element if present, otherwise the first <name>.
func (h *CDAHeaderParser) selectName(pat *etree.Element) *etree.Element {
	for _, n := range pat.SelectElements("name") {
		if n.SelectAttrValue("use", "") == "L" {
			return n
		}
	}
	return pat.FindElement("name")
}

// =====================================
// Document metadata
// =====================================

func (h *CDAHeaderParser) parseDocument(root *etree.Element) map[string]interface{} {
	d := make(map[string]interface{})

	h.setStr(d, "type", h.attrVal(root, "code", "code"))
	h.setStr(d, "typeDisplay", h.attrVal(root, "code", "displayName"))
	h.setStr(d, "title", strings.TrimSpace(h.elementText(root, "title")))
	h.setStr(d, "effectiveDate", h.attrVal(root, "effectiveTime", "value"))

	// Document ID
	h.setStr(d, "id", h.attrVal(root, "id", "extension"))
	if d["id"] == "" {
		h.setStr(d, "id", h.attrVal(root, "id", "root"))
	}

	// Custodian organisation
	h.setStr(d, "custodian",
		strings.TrimSpace(h.elementText(root, "custodian/assignedCustodian/representedCustodianOrganization/name")))

	return d
}

// =====================================
// Author
// =====================================

func (h *CDAHeaderParser) parseAuthor(root *etree.Element) map[string]interface{} {
	a := make(map[string]interface{})

	author := root.FindElement("author")
	if author == nil {
		return a
	}

	h.setStr(a, "time", h.attrVal(author, "time", "value"))

	assigned := author.FindElement("assignedAuthor")
	if assigned == nil {
		return a
	}

	// Author name
	nameEl := assigned.FindElement("assignedPerson/name")
	if nameEl != nil {
		given := strings.TrimSpace(h.elementText(nameEl, "given"))
		family := strings.TrimSpace(h.elementText(nameEl, "family"))
		parts := []string{}
		if given != "" {
			parts = append(parts, given)
		}
		if family != "" {
			parts = append(parts, family)
		}
		if len(parts) > 0 {
			a["name"] = strings.Join(parts, " ")
		}
	}

	// Organisation
	h.setStr(a, "organization",
		strings.TrimSpace(h.elementText(assigned, "representedOrganization/name")))

	return a
}

// =====================================
// Helpers
// =====================================

func (h *CDAHeaderParser) attrVal(parent *etree.Element, path, attr string) string {
	el := parent.FindElement(path)
	if el == nil {
		return ""
	}
	return el.SelectAttrValue(attr, "")
}

func (h *CDAHeaderParser) elementText(parent *etree.Element, path string) string {
	el := parent.FindElement(path)
	if el == nil {
		return ""
	}
	return el.Text()
}

func (h *CDAHeaderParser) setStr(m map[string]interface{}, key, value string) {
	if value != "" {
		m[key] = value
	}
}
