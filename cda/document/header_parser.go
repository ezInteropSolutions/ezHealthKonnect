// cda/document/header_parser.go
// headerParser extracts the complete CDA document header into a typed CDAHeader.
//
// Covered header sections:
//   - id, code, title, setId, versionNumber, effectiveTime, confidentialityCode
//   - recordTarget  → CDAPatient  (ALL ids, names, addresses, telecoms, languages)
//   - author[]      → []CDAAuthor (can repeat per C-CDA)
//   - custodian     → CDACustodian
//   - documentationOf/serviceEvent → *CDAServiceEvent
//   - componentOf/encompassingEncounter → *CDAEncounter
//   - relatedDocument[] → []CDARelatedDocument (can repeat)
//
// Invariants: SelectElements is used for every XML element that can repeat.
// nav() is used for path navigation — no FindElement calls in this file.

package cdadocument

import (
	"strings"

	"github.com/beevik/etree"
)

type headerParser struct{}

// parseHeader builds the complete CDAHeader from the ClinicalDocument root.
func (h *headerParser) parseHeader(root *etree.Element) CDAHeader {
	if root == nil {
		return CDAHeader{}
	}
	hdr := CDAHeader{}

	if idEl := root.SelectElement("id"); idEl != nil {
		hdr.DocumentId = parseII(idEl)
	}
	if codeEl := root.SelectElement("code"); codeEl != nil {
		hdr.DocumentCode = parseCD(codeEl)
	}
	if titleEl := root.SelectElement("title"); titleEl != nil {
		hdr.Title = strings.TrimSpace(titleEl.Text())
	}
	if setIdEl := root.SelectElement("setId"); setIdEl != nil {
		hdr.SetId = parseII(setIdEl)
	}
	if vnEl := root.SelectElement("versionNumber"); vnEl != nil {
		hdr.VersionNumber = vnEl.SelectAttrValue("value", "")
	}
	if etEl := root.SelectElement("effectiveTime"); etEl != nil {
		hdr.EffectiveTime = parseTS(etEl)
	}
	if ccEl := root.SelectElement("confidentialityCode"); ccEl != nil {
		hdr.ConfidentialityCode = parseCD(ccEl)
	}

	hdr.Patient = h.parsePatient(root)
	hdr.Authors = h.parseAuthors(root)
	hdr.Custodian = h.parseCustodian(root)
	hdr.DocumentOf = h.parseDocumentOf(root)
	hdr.EncompassingEncounter = h.parseComponentOf(root)
	hdr.RelatedDocuments = h.parseRelatedDocuments(root)

	return hdr
}

// ========================
// recordTarget → CDAPatient
// ========================

func (h *headerParser) parsePatient(root *etree.Element) CDAPatient {
	role := nav(root, "recordTarget", "patientRole")
	if role == nil {
		return CDAPatient{}
	}

	patient := CDAPatient{}

	// ALL <id> elements — SSN, MRN, Medicare, encounter number, etc.
	for _, idEl := range role.SelectElements("id") {
		patient.Ids = append(patient.Ids, parseII(idEl))
	}

	// ALL <addr> elements
	for _, addrEl := range role.SelectElements("addr") {
		patient.Addresses = append(patient.Addresses, parseAD(addrEl))
	}

	// ALL <telecom> elements
	for _, telEl := range role.SelectElements("telecom") {
		patient.Telecoms = append(patient.Telecoms, parseTEL(telEl))
	}

	// <patient> demographics
	patEl := role.SelectElement("patient")
	if patEl == nil {
		return patient
	}

	// ALL <name> elements (legal, maiden, alias, etc.)
	for _, nameEl := range patEl.SelectElements("name") {
		patient.Names = append(patient.Names, parsePN(nameEl))
	}

	if btEl := patEl.SelectElement("birthTime"); btEl != nil {
		patient.BirthDate = parseTS(btEl)
	}
	if gcEl := patEl.SelectElement("administrativeGenderCode"); gcEl != nil {
		patient.Gender = parseCD(gcEl)
	}
	if msEl := patEl.SelectElement("maritalStatusCode"); msEl != nil {
		patient.MaritalStatus = parseCD(msEl)
	}
	if rcEl := patEl.SelectElement("raceCode"); rcEl != nil {
		patient.Race = parseCD(rcEl)
	}
	if ecEl := patEl.SelectElement("ethnicGroupCode"); ecEl != nil {
		patient.Ethnicity = parseCD(ecEl)
	}

	// ALL <languageCommunication> elements
	for _, langEl := range patEl.SelectElements("languageCommunication") {
		lang := CDALanguage{}
		if lc := langEl.SelectElement("languageCode"); lc != nil {
			lang.Code = lc.SelectAttrValue("code", "")
		}
		if pi := langEl.SelectElement("preferenceInd"); pi != nil {
			lang.PreferenceInd = pi.SelectAttrValue("value", "") == "true"
		}
		if mc := langEl.SelectElement("modeCode"); mc != nil {
			lang.ModeCode = parseCD(mc)
		}
		if pl := langEl.SelectElement("proficiencyLevelCode"); pl != nil {
			lang.ProficiencyLevel = parseCD(pl)
		}
		patient.Languages = append(patient.Languages, lang)
	}

	return patient
}

// ========================
// author[] → []CDAAuthor
// ========================

func (h *headerParser) parseAuthors(root *etree.Element) []CDAAuthor {
	var authors []CDAAuthor
	for _, authorEl := range root.SelectElements("author") {
		author := CDAAuthor{}
		if t := authorEl.SelectElement("time"); t != nil {
			author.Time = parseTS(t)
		}
		if aaEl := authorEl.SelectElement("assignedAuthor"); aaEl != nil {
			author.AssignedAuthor = parseAssignedAuthor(aaEl)
		}
		authors = append(authors, author)
	}
	return authors
}

// parseAssignedAuthor parses an <assignedAuthor> element. Package-level (not a
// headerParser method, despite living in this file) so entryParser can reuse
// it for per-entry <author> elements (e.g. who ordered a medication) without
// duplicating this logic — same pattern as the free-function parseAssignedEntity
// and parseOrganization in datatypes.go.
func parseAssignedAuthor(el *etree.Element) CDAAssignedAuthor {
	aa := CDAAssignedAuthor{}
	for _, idEl := range el.SelectElements("id") {
		aa.Ids = append(aa.Ids, parseII(idEl))
	}
	for _, addrEl := range el.SelectElements("addr") {
		aa.Addresses = append(aa.Addresses, parseAD(addrEl))
	}
	for _, telEl := range el.SelectElements("telecom") {
		aa.Telecoms = append(aa.Telecoms, parseTEL(telEl))
	}
	if apEl := el.SelectElement("assignedPerson"); apEl != nil {
		person := CDAPerson{}
		for _, nameEl := range apEl.SelectElements("name") {
			person.Names = append(person.Names, parsePN(nameEl))
		}
		aa.AssignedPerson = &person
	}
	if adEl := el.SelectElement("assignedAuthoringDevice"); adEl != nil {
		device := CDADevice{}
		if sn := adEl.SelectElement("softwareName"); sn != nil {
			device.SoftwareName = strings.TrimSpace(sn.Text())
		}
		if mm := adEl.SelectElement("manufacturerModelName"); mm != nil {
			device.ManufacturerModelName = strings.TrimSpace(mm.Text())
		}
		aa.AssignedAuthoringDevice = &device
	}
	if orgEl := el.SelectElement("representedOrganization"); orgEl != nil {
		org := parseOrganization(orgEl)
		aa.RepresentedOrganization = &org
	}
	return aa
}

// ========================
// custodian → CDACustodian
// ========================

func (h *headerParser) parseCustodian(root *etree.Element) CDACustodian {
	custEl := nav(root, "custodian", "assignedCustodian")
	if custEl == nil {
		return CDACustodian{}
	}
	cust := CDACustodian{}
	if orgEl := custEl.SelectElement("representedCustodianOrganization"); orgEl != nil {
		cust.AssignedCustodian.RepresentedCustodianOrganization = parseOrganization(orgEl)
	}
	return cust
}

// ========================
// documentationOf → *CDAServiceEvent
// ========================

func (h *headerParser) parseDocumentOf(root *etree.Element) *CDAServiceEvent {
	seEl := nav(root, "documentationOf", "serviceEvent")
	if seEl == nil {
		return nil
	}
	se := &CDAServiceEvent{
		ClassCode: seEl.SelectAttrValue("classCode", ""),
	}
	if etEl := seEl.SelectElement("effectiveTime"); etEl != nil {
		se.EffectiveTime = parseIVL_TS(etEl)
	}
	for _, perfEl := range seEl.SelectElements("performer") {
		perf := CDAPerformer{
			TypeCode: perfEl.SelectAttrValue("typeCode", ""),
		}
		if fc := perfEl.SelectElement("functionCode"); fc != nil {
			perf.FunctionCode = parseCD(fc)
		}
		if t := perfEl.SelectElement("time"); t != nil {
			perf.Time = parseIVL_TS(t)
		}
		if aeEl := perfEl.SelectElement("assignedEntity"); aeEl != nil {
			perf.AssignedEntity = parseAssignedEntity(aeEl)
		}
		se.Performers = append(se.Performers, perf)
	}
	return se
}

// ========================
// componentOf → *CDAEncounter
// ========================

func (h *headerParser) parseComponentOf(root *etree.Element) *CDAEncounter {
	eeEl := nav(root, "componentOf", "encompassingEncounter")
	if eeEl == nil {
		return nil
	}
	enc := &CDAEncounter{}
	if idEl := eeEl.SelectElement("id"); idEl != nil {
		enc.Id = parseII(idEl)
	}
	if etEl := eeEl.SelectElement("effectiveTime"); etEl != nil {
		enc.EffectiveTime = parseIVL_TS(etEl)
	}
	if ddEl := eeEl.SelectElement("dischargeDispositionCode"); ddEl != nil {
		enc.DischargeDispositionCode = parseCD(ddEl)
	}
	if locEl := eeEl.SelectElement("location"); locEl != nil {
		loc := &CDALocation{}
		if hcfEl := locEl.SelectElement("healthCareFacility"); hcfEl != nil {
			hcf := CDAHealthCareFacility{}
			if c := hcfEl.SelectElement("code"); c != nil {
				hcf.Code = parseCD(c)
			}
			if innerLocEl := hcfEl.SelectElement("location"); innerLocEl != nil {
				place := CDAPlace{}
				for _, n := range innerLocEl.SelectElements("name") {
					place.Names = append(place.Names, parsePN(n))
				}
				for _, a := range innerLocEl.SelectElements("addr") {
					place.Addresses = append(place.Addresses, parseAD(a))
				}
				hcf.Location = &place
			}
			if orgEl := hcfEl.SelectElement("serviceProviderOrganization"); orgEl != nil {
				org := parseOrganization(orgEl)
				hcf.ServiceProviderOrganization = &org
			}
			loc.HealthCareFacility = hcf
		}
		enc.Location = loc
	}
	return enc
}

// ========================
// relatedDocument[] → []CDARelatedDocument
// ========================

func (h *headerParser) parseRelatedDocuments(root *etree.Element) []CDARelatedDocument {
	var rels []CDARelatedDocument
	for _, rdEl := range root.SelectElements("relatedDocument") {
		rd := CDARelatedDocument{
			TypeCode: rdEl.SelectAttrValue("typeCode", ""),
		}
		if pdEl := rdEl.SelectElement("parentDocument"); pdEl != nil {
			pd := CDAParentDocument{}
			for _, idEl := range pdEl.SelectElements("id") {
				pd.Ids = append(pd.Ids, parseII(idEl))
			}
			if setIdEl := pdEl.SelectElement("setId"); setIdEl != nil {
				pd.SetId = parseII(setIdEl)
			}
			if vnEl := pdEl.SelectElement("versionNumber"); vnEl != nil {
				pd.VersionNumber = vnEl.SelectAttrValue("value", "")
			}
			rd.ParentDocument = pd
		}
		rels = append(rels, rd)
	}
	return rels
}
