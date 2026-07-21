// cda/builder/document_builder.go
//
// BuildDocument is the top-level entry point: canonical USCDI-keyed JSON
// (the same shape services/parsers/cda/cda_parser_service.go's
// assembleJSON produces — {"header": {...}, "sections": {key: {"entries":
// [...]}}}) in, valid C-CDA 2.1 XML out. Section entries are built generically
// via buildEntry (entry_archetypes.go); the document header (patient/author/
// custodian) is hand-built here instead, because — unlike section entries,
// which follow GenericSectionProcessor's schema-driven field.Key convention
// — the canonical header shape comes from cda_parser_service.go's separate,
// hand-written patientToLegacyJSON/authorToJSON functions and uses a
// different, fixed key convention (firstName/lastName/dateOfBirth/sex/...).
// This mirrors the exact same split already present on the parse side.
package builder

import (
	"fmt"

	cdaSchema "ezhealthkonnect/cda"

	"github.com/beevik/etree"
)

// BuildOptions configures document construction beyond the canonical data
// itself — organization identity that has no canonical-JSON source (the
// header's "author"/"custodian" org name isn't populated by cda.parse today).
type BuildOptions struct {
	OrgName   string           // custodian + fallback author organization name (legacy — see CustodianOptions.OrgName)
	Custodian CustodianOptions // structured custodian fields — deployment-level constant config, not per-message source data (see cda.build step's own Custodian tab, not cda.map_to_canonical)
}

// CustodianOptions are the structured Custodian fields configurable
// directly on the cda.build step (services/executors/transform/
// cda_build_executor.go's CdaCustodianConfig). All-zero-value is
// byte-for-byte backward compatible with the pre-existing hardcoded
// writeCustodianHeader behavior (bare npiOID-rooted id, name from
// resolveOrgName(opts)) — every saved pipeline predating this feature keeps
// producing identical output.
//
// Address/Phone are accepted as useful, purely optional data entry:
// cda/schemas/ccda_2_1.json's header.custodian section defines no
// address/telecom fields at all (unlike header.patient), so this package
// does not assert them as spec-required — see header_requirements.go's file
// comment for why that's a deliberate, evidence-based omission rather than
// an oversight.
type CustodianOptions struct {
	IdRoot      string // defaults to npiOID when empty — preserves current hardcoded behavior
	IdExtension string // empty -> bare npiOID-rooted id, no extension (current behavior)
	OrgName     string // falls back to resolveOrgName(opts) when empty (current behavior)
	Street, City, State, PostalCode, Country string
	Phone       string
}

// BuildDocument converts canonicalDoc into a C-CDA 2.1 XML string for the
// given documentType (e.g. "CCD"; "" defaults to "CCD"). Includes every
// SHALL section always (even if empty, per CCD conformance) and every SHOULD
// section that has at least one entry.
func BuildDocument(loader *cdaSchema.CDASchemaLoader, canonicalDoc map[string]interface{}, documentType string, opts BuildOptions) (string, error) {
	if documentType == "" {
		documentType = "CCD"
	}
	sectionInfo := loader.GetDocumentTypeSections(documentType)
	if sectionInfo == nil {
		return "", fmt.Errorf("cda/builder: unknown document type %q", documentType)
	}
	docMeta := loader.GetDocumentTypeMetadata(documentType)

	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	root := doc.CreateElement("ClinicalDocument")
	root.CreateAttr("xmlns", "urn:hl7-org:v3")
	root.CreateAttr("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance")
	root.CreateAttr("xsi:schemaLocation", "urn:hl7-org:v3 CDA.xsd")

	writeDocumentBoilerplate(root, docMeta)

	headerMap, _ := canonicalDoc["header"].(map[string]interface{})
	writePatientHeader(root, headerMap)
	writeAuthorHeader(root, headerMap, opts)
	writeInformantsHeader(root, headerMap)
	writeCustodianHeader(root, opts)
	writeDocumentationOfHeader(root, headerMap)
	writeEncompassingEncounterHeader(root, headerMap)

	structuredBody := root.CreateElement("component").CreateElement("structuredBody")

	sectionsMap, _ := canonicalDoc["sections"].(map[string]interface{})
	sectionKeys := append(append(append([]string{}, sectionInfo.SHALL...), sectionInfo.SHOULD...), sectionInfo.MAY...)
	for _, key := range sectionKeys {
		sec := loader.GetSection(key)
		if sec == nil {
			continue
		}
		entries := extractEntries(sectionsMap, key)
		isShall := containsString(sectionInfo.SHALL, key)
		if len(entries) == 0 && !isShall {
			continue
		}
		component := structuredBody.CreateElement("component")
		component.AddChild(buildSectionElement(sec, entries))
	}

	doc.Indent(2)
	return doc.WriteToString()
}

func buildSectionElement(sec *cdaSchema.CDASectionDef, entries []map[string]interface{}) *etree.Element {
	section := etree.NewElement("section")
	if sec.TemplateID != "" {
		tid := section.CreateElement("templateId")
		tid.CreateAttr("root", sec.TemplateID)
		if sec.TemplateIDExt != "" {
			tid.CreateAttr("extension", sec.TemplateIDExt)
		}
	}
	if sec.LOINCCode != "" {
		code := section.CreateElement("code")
		code.CreateAttr("code", sec.LOINCCode)
		code.CreateAttr("codeSystem", "2.16.840.1.113883.6.1")
		code.CreateAttr("codeSystemName", "LOINC")
		if sec.DisplayName != "" {
			code.CreateAttr("displayName", sec.DisplayName)
		}
	}
	if sec.DisplayName != "" {
		section.CreateElement("title").SetText(sec.DisplayName)
	}

	buildSectionNarrative(section, sec, entries)

	for _, entry := range entries {
		buildEntry(section, sec, entry)
	}

	return section
}

func extractEntries(sectionsMap map[string]interface{}, key string) []map[string]interface{} {
	secData, ok := sectionsMap[key].(map[string]interface{})
	if !ok {
		return nil
	}
	rawEntries, ok := secData["entries"].([]interface{})
	if !ok {
		// Also accept the typed shape ([]map[string]interface{}) a caller
		// (e.g. the FHIR bundle adapter) may build in-process without a
		// JSON round-trip.
		if typed, ok := secData["entries"].([]map[string]interface{}); ok {
			return typed
		}
		return nil
	}
	entries := make([]map[string]interface{}, 0, len(rawEntries))
	for _, e := range rawEntries {
		if m, ok := e.(map[string]interface{}); ok {
			entries = append(entries, m)
		}
	}
	return entries
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ─── Document-level boilerplate ─────────────────────────────────────────────

func writeDocumentBoilerplate(root *etree.Element, meta *cdaSchema.DocumentTypeMetadata) {
	// US Realm Header (CONF:1198-*, "Conforms to US Realm Header (V3) template
	// urn:hl7ii:2.16.840.1.113883.10.20.22.1.1:2015-08-01") + document-type templateId.
	realmHeader := root.CreateElement("templateId")
	realmHeader.CreateAttr("root", "2.16.840.1.113883.10.20.22.1.1")
	realmHeader.CreateAttr("extension", "2015-08-01")
	if meta != nil && meta.TemplateID != "" {
		docTid := root.CreateElement("templateId")
		docTid.CreateAttr("root", meta.TemplateID)
		if meta.TemplateIDExt != "" {
			docTid.CreateAttr("extension", meta.TemplateIDExt)
		}
	}

	docID := fmt.Sprintf("ehk-%d", timeNowUnixNano())
	root.CreateElement("id").CreateAttr("root", docID)

	code := root.CreateElement("code")
	if meta != nil {
		code.CreateAttr("code", meta.LOINCCode)
		root.CreateElement("title").SetText(meta.Title)
	} else {
		code.CreateAttr("code", "34133-9")
		root.CreateElement("title").SetText("Continuity of Care Document")
	}
	code.CreateAttr("codeSystem", "2.16.840.1.113883.6.1")
	code.CreateAttr("codeSystemName", "LOINC")

	root.CreateElement("effectiveTime").CreateAttr("value", nowCDATimestamp())
	root.CreateElement("confidentialityCode").CreateAttr("code", "N")
	root.CreateElement("languageCode").CreateAttr("code", "en-US")
}

// ─── Patient / Author / Custodian ───────────────────────────────────────────
// Field-by-field construction is declarative (header_fields.go's mapping
// tables + writeHeaderFields/writeCodedFields/writeRepeatingGroup); this
// function is left doing only structural wiring (which element owns which
// mapping table) and the one piece of real logic that isn't a flat
// field-copy — the phone number's "tel:" prefix + fixed @use="HP".

const npiOID = "2.16.840.1.113883.4.6"

func writePatientHeader(root *etree.Element, header map[string]interface{}) {
	patientData, _ := header["patient"].(map[string]interface{})

	patientRole := root.CreateElement("recordTarget").CreateElement("patientRole")

	writeRepeatingGroup(patientRole, patientData, "ids", "id", idItemFields)

	if addr, ok := patientData["address"].(map[string]interface{}); ok {
		addrEl := patientRole.CreateElement("addr")
		writeHeaderFields(addrEl, addr, patientAddressFields)
	}

	if phone, ok := stringValue(patientData["phone"]); ok {
		tel := patientRole.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+phone)
		tel.CreateAttr("use", "HP")
	}

	writeHeaderFields(patientRole, patientData, patientScalarFields)
	writeCodedFields(patientRole, patientData, patientCodedFields)
}

func writeAuthorHeader(root *etree.Element, header map[string]interface{}, opts BuildOptions) {
	authorData, _ := header["author"].(map[string]interface{})

	author := root.CreateElement("author")
	author.CreateElement("time").CreateAttr("value", nowCDATimestamp())
	assignedAuthor := author.CreateElement("assignedAuthor")

	writeNPI(assignedAuthor, "id", authorData["npi"])
	writeHeaderFields(assignedAuthor, authorData, authorScalarFields)

	assignedAuthor.CreateElement("representedOrganization").CreateElement("name").SetText(resolveOrgName(opts))
}

// writeNPI writes a fixed-root/data-driven-extension <id> at xpath — the
// NPI shape every author/informant/performer identity uses. Always present
// (a bare NPI-rooted id with no extension when npi data is absent) to match
// the pre-refactor behavior exactly.
func writeNPI(root *etree.Element, xpath string, npi interface{}) {
	id := WriteAtXPath(root, xpath, "")
	id.CreateAttr("root", npiOID)
	if v, ok := stringValue(npi); ok {
		id.CreateAttr("extension", v)
	}
}

func resolveOrgName(opts BuildOptions) string {
	if opts.OrgName != "" {
		return opts.OrgName
	}
	return "ezHealthKonnect"
}

// writeCustodianHeader writes <custodian><assignedCustodian>
// <representedCustodianOrganization>. opts.Custodian's zero value reproduces
// the pre-existing hardcoded shape exactly (bare npiOID-rooted id, name from
// resolveOrgName(opts)) — every saved pipeline predating structured
// Custodian config keeps producing byte-for-byte identical output.
func writeCustodianHeader(root *etree.Element, opts BuildOptions) {
	cust := opts.Custodian
	org := root.CreateElement("custodian").CreateElement("assignedCustodian").CreateElement("representedCustodianOrganization")

	idRoot := cust.IdRoot
	if idRoot == "" {
		idRoot = npiOID
	}
	id := org.CreateElement("id")
	id.CreateAttr("root", idRoot)
	if cust.IdExtension != "" {
		id.CreateAttr("extension", cust.IdExtension)
	}

	name := cust.OrgName
	if name == "" {
		name = resolveOrgName(opts)
	}
	org.CreateElement("name").SetText(name)

	// Address/phone reuse patientAddressFields/writeHeaderFields verbatim —
	// same helper, same convention, no new XML-writing mechanism. Optional
	// data entry (see CustodianOptions' own doc comment for why these are
	// not spec-asserted requirements).
	if cust.Street != "" || cust.City != "" || cust.State != "" || cust.PostalCode != "" || cust.Country != "" {
		addr := org.CreateElement("addr")
		writeHeaderFields(addr, map[string]interface{}{
			"street": cust.Street, "city": cust.City, "state": cust.State,
			"postalCode": cust.PostalCode, "country": cust.Country,
		}, patientAddressFields)
	}
	if cust.Phone != "" {
		tel := org.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+cust.Phone)
		tel.CreateAttr("use", "WP")
	}
}

// writePersonReference writes one <tag><assignedEntity>...</assignedEntity></tag>
// element — the shape shared by document-level informants[] and
// documentationOf performers[]: an optional NPI-rooted id (only when npi
// data is present, unlike author's identity which always carries one),
// optional name, and (informants only) an optional represented
// organization. Returns the outer <tag> element so callers can add their
// own tag-specific attributes (e.g. performer's fixed typeCode="PRF").
func writePersonReference(root *etree.Element, tag string, person map[string]interface{}, includeOrg bool) *etree.Element {
	el := root.CreateElement(tag)
	assignedEntity := el.CreateElement("assignedEntity")

	if npi, ok := stringValue(person["npi"]); ok {
		writeNPI(assignedEntity, "id", npi)
	}
	writeHeaderFields(assignedEntity, person, personItemFields)
	if includeOrg {
		if orgName, ok := stringValue(person["orgName"]); ok {
			assignedEntity.CreateElement("representedOrganization").CreateElement("name").SetText(orgName)
		}
	}
	return el
}

// writeInformantsHeader writes zero or more <informant><assignedEntity>
// elements — sources of information for the whole document who are not its
// author (e.g. a referring provider). Schema order: after author, before
// custodian (enforced by the call site in BuildDocument).
func writeInformantsHeader(root *etree.Element, header map[string]interface{}) {
	informants, ok := header["informants"].([]interface{})
	if !ok {
		return
	}
	for _, infRaw := range informants {
		inf, ok := infRaw.(map[string]interface{})
		if !ok {
			continue
		}
		writePersonReference(root, "informant", inf, true)
	}
}

// writeDocumentationOfHeader writes <documentationOf><serviceEvent> — the
// overall clinical service this document documents, and who performed it.
// For CCD specifically this is a genuine SHALL (CONF:1198-8452/8453/8481/
// 8454/8455 in the C-CDA 2.1 IG, 2018 errata, Table 30) with a mandatory
// classCode="PCPR" and effectiveTime low/high — so when the canonical header
// carries no documentationOf data, a minimal nullFlavor="UNK" default is
// synthesized rather than omitting the element outright. Harmless for other
// document types where it's merely optional.
func writeDocumentationOfHeader(root *etree.Element, header map[string]interface{}) {
	docOf, _ := header["documentationOf"].(map[string]interface{})

	serviceEvent := root.CreateElement("documentationOf").CreateElement("serviceEvent")
	serviceEvent.CreateAttr("classCode", "PCPR")

	low, hasLow := stringValue(docOf["effectiveTimeLow"])
	high, hasHigh := stringValue(docOf["effectiveTimeHigh"])
	et := serviceEvent.CreateElement("effectiveTime")
	if hasLow {
		et.CreateElement("low").CreateAttr("value", low)
	} else {
		et.CreateElement("low").CreateAttr("nullFlavor", "UNK")
	}
	if hasHigh {
		et.CreateElement("high").CreateAttr("value", high)
	} else {
		et.CreateElement("high").CreateAttr("nullFlavor", "UNK")
	}

	performers, _ := docOf["performers"].([]interface{})
	for _, pRaw := range performers {
		p, ok := pRaw.(map[string]interface{})
		if !ok {
			continue
		}
		perf := writePersonReference(serviceEvent, "performer", p, false)
		perf.CreateAttr("typeCode", "PRF")
	}
}

// writeEncompassingEncounterHeader writes <componentOf><encompassingEncounter>
// — the specific encounter this document was generated for, distinct from
// the "encounters" SECTION's historical encounter list. Omitted entirely
// when the canonical header carries no encompassingEncounter data.
func writeEncompassingEncounterHeader(root *etree.Element, header map[string]interface{}) {
	enc, ok := header["encompassingEncounter"].(map[string]interface{})
	if !ok || len(enc) == 0 {
		return
	}

	ee := root.CreateElement("componentOf").CreateElement("encompassingEncounter")

	if id, ok := stringValue(enc["id"]); ok {
		idEl := ee.CreateElement("id")
		idEl.CreateAttr("root", npiOID)
		idEl.CreateAttr("extension", id)
	}

	low, hasLow := stringValue(enc["effectiveTimeLow"])
	high, hasHigh := stringValue(enc["effectiveTimeHigh"])
	if hasLow || hasHigh {
		et := ee.CreateElement("effectiveTime")
		if hasLow {
			et.CreateElement("low").CreateAttr("value", low)
		}
		if hasHigh {
			et.CreateElement("high").CreateAttr("value", high)
		}
	}

	if code, ok := stringValue(enc["dischargeDispositionCode"]); ok {
		ee.CreateElement("dischargeDispositionCode").CreateAttr("code", code)
	}

	_, hasFacilityName := stringValue(enc["facilityName"])
	_, hasFacilityOrgName := stringValue(enc["facilityOrgName"])
	_, hasFacilityTypeCode := stringValue(enc["facilityTypeCode"])
	if hasFacilityName || hasFacilityOrgName || hasFacilityTypeCode {
		hcf := ee.CreateElement("location").CreateElement("healthCareFacility")
		writeHeaderFields(hcf, enc, healthCareFacilityFields)
	}
}
