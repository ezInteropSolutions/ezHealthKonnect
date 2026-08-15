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

	// LegalAuthenticator configures the document's optional (0..1,
	// CONF:1198-5579) legal signer — deployment-level constant config, same
	// rationale as Custodian. Zero-value (no Given/Family) means "omit the
	// element entirely" — see LegalAuthenticatorOptions' own doc comment.
	LegalAuthenticator LegalAuthenticatorOptions

	// TimezoneOffset is a "+HHMM"/"-HHMM" string (e.g. "-0500") applied to
	// the internally-generated document effectiveTime and author time —
	// deployment-level constant config, same rationale as OrgName/Custodian
	// above, since these two timestamps are "now at build time", not
	// per-message source data. Empty defaults to UTC ("+0000"); a malformed
	// value falls back to UTC rather than erroring (see
	// nowCDATimestamp/parseTimezoneOffset in time.go).
	TimezoneOffset string
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
	IdRoot                                   string // defaults to npiOID when empty — preserves current hardcoded behavior
	IdExtension                              string // empty -> bare npiOID-rooted id, no extension (current behavior)
	OrgName                                  string // falls back to resolveOrgName(opts) when empty (current behavior)
	Street, City, State, PostalCode, Country string
	Phone                                    string
}

// LegalAuthenticatorOptions are the structured fields configurable directly
// on the cda.build step's own Legal Authenticator tab (services/executors/
// transform/cda_build_executor.go's CdaLegalAuthenticatorConfig) — the
// single person legally responsible for the document (CONF:1198-5579,
// SHOULD 0..1), distinct from its author. Unlike CustodianOptions (SHALL
// exactly one, always written, all-zero-value preserves pre-existing
// hardcoded output), this element is genuinely optional and an incomplete
// one (missing the SHALL assignedPerson/name) would be worse than omitting
// it — so writeLegalAuthenticatorHeader only writes anything at all when
// Given or Family is configured; every saved pipeline predating this
// feature simply never gets a legalAuthenticator, matching prior behavior
// exactly (there was none before).
type LegalAuthenticatorOptions struct {
	Given, Family                                            string
	NPI                                                      string
	SpecialtyCode, SpecialtyCodeSystem, SpecialtyCodeDisplay string
	Street, City, State, PostalCode, Country                 string
	Phone                                                    string
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

	// Unstructured Document has no fixed document-level LOINC (unlike every
	// other type) — per the IG, its <code> is selected per-document from
	// whatever content is actually embedded, not a constant. documentCode/
	// documentTitle in the canonical unstructuredContent map override
	// docMeta's own (empty) loincCode/title; a generic Note fallback applies
	// if neither the canonical data nor docMeta supplies one, mirroring
	// services/cda_fhir/narrative_section_mapper.go's own genericNoteLoincCode
	// fallback for the identical "nothing more specific is known" case.
	effectiveMeta := docMeta
	if unstructuredData, ok := canonicalDoc["unstructuredContent"].(map[string]interface{}); ok && docMeta != nil {
		m := *docMeta
		if code, ok := stringValue(unstructuredData["documentCode"]); ok {
			m.LOINCCode = code
		}
		if title, ok := stringValue(unstructuredData["documentTitle"]); ok {
			m.Title = title
		}
		if m.LOINCCode == "" {
			m.LOINCCode = unstructuredDocumentFallbackCode
		}
		if m.Title == "" {
			m.Title = unstructuredDocumentFallbackTitle
		}
		effectiveMeta = &m
	}

	writeDocumentBoilerplate(root, effectiveMeta, opts)

	headerMap, _ := canonicalDoc["header"].(map[string]interface{})
	writePatientHeader(root, headerMap)
	writeAuthorHeader(root, headerMap, opts)
	writeInformantsHeader(root, headerMap)
	writeCustodianHeader(root, opts)
	writeLegalAuthenticatorHeader(root, opts)
	writeDocumentationOfHeader(root, headerMap)
	writeEncompassingEncounterHeader(root, headerMap)

	// Unstructured Document has no sections at all — its <component> is a
	// <nonXMLBody> (embedded/referenced content), never a <structuredBody>,
	// per CDA's own ClinicalDocument.component CHOICE. Every other document
	// type takes the structuredBody path below unconditionally, matching
	// prior behavior exactly.
	if unstructuredData, ok := canonicalDoc["unstructuredContent"].(map[string]interface{}); ok {
		buildNonXMLBody(root.CreateElement("component"), unstructuredData)
	} else {
		structuredBody := root.CreateElement("component").CreateElement("structuredBody")

		sectionsMap, _ := canonicalDoc["sections"].(map[string]interface{})
		sectionKeys := append(append(append([]string{}, sectionInfo.SHALL...), sectionInfo.SHOULD...), sectionInfo.MAY...)
		for _, key := range sectionKeys {
			sec := loader.GetSection(key)
			if sec == nil {
				continue
			}
			secData, _ := sectionsMap[key].(map[string]interface{})
			entries := extractSubEntries(secData, "entries")

			altEntries := make([][]map[string]interface{}, len(sec.AlternateArchetypes))
			hasAltEntries := false
			for i, alt := range sec.AlternateArchetypes {
				altEntries[i] = extractSubEntries(secData, alt.EntriesKey)
				if len(altEntries[i]) > 0 {
					hasAltEntries = true
				}
			}

			isShall := containsString(sectionInfo.SHALL, key)
			if len(entries) == 0 && !hasAltEntries && !isShall {
				continue
			}
			component := structuredBody.CreateElement("component")
			component.AddChild(buildSectionElement(sec, entries, altEntries))
		}
	}

	ensureIntervalEffectiveTimeXsiType(root)
	ensureIntervalPQXsiType(root)
	ensurePeriodicIntervalXsiType(root)

	doc.Indent(2)
	return doc.WriteToString()
}

// ensurePeriodicIntervalXsiType sets xsi:type="PIVL_TS", @operator="A", and
// @institutionSpecified="true" on every <effectiveTime> in the whole
// document that has a <period> child but no xsi:type yet — Medication
// Activity's own recurring-dose frequency (CONF:1098-7513/9106/28499,
// Figure 168's own worked example: <effectiveTime xsi:type="PIVL_TS"
// institutionSpecified="true" operator="A"><period value="12" unit="h"/>
// </effectiveTime>). SubstanceAdministration's base type permits effectiveTime
// to repeat (0..*) — this second one is a SEPARATE element from the
// medication's own start/stop effectiveTime (already covered by
// ensureIntervalEffectiveTimeXsiType, which only ever matches low/high
// children, never period), created via the field's own "effectiveTime[2]"
// positional xpath predicate (see doseFrequencyValue in ccda_2_1.json) — the
// SAME "no single field write owns the parent element" gap the other two
// interval-xsi:type passes exist for, just for the period-only shape.
// operator="A" (CONF:1098-9106, "average") and institutionSpecified="true"
// are both fixed per the worked example — this schema only ever represents
// a simple periodic frequency, never a complex multi-part schedule.
func ensurePeriodicIntervalXsiType(root *etree.Element) {
	for _, et := range root.FindElements(".//effectiveTime") {
		if et.SelectAttrValue("xsi:type", "") != "" {
			continue
		}
		if et.SelectElement("period") == nil {
			continue
		}
		et.CreateAttr("xsi:type", "PIVL_TS")
		et.CreateAttr("operator", "A")
		et.CreateAttr("institutionSpecified", "true")
	}
}

// ensureIntervalPQXsiType sets xsi:type="IVL_PQ" on every <value> in the
// whole document that has a <low> and/or <high> child but no xsi:type yet —
// the exact same shape/reason as ensureIntervalEffectiveTimeXsiType, just for
// Result Observation's referenceRange/observationRange/value (CONF:1198-7150/
// 7151/32175, Figure 212's own worked example: <value xsi:type="IVL_PQ">
// <low value="34.9" unit="%"/><high value="44.5" unit="%"/></value>).
// referenceRangeLow/referenceRangeHigh's own field writes only ever touch the
// <low>/<high> children directly (their xpath targets .../value/low/@value
// and .../value/high/@value), so writeFieldValue's injectValueXsiType never
// fires for them (it only acts on an element literally tagged <value>) —
// this is the same "no single field write owns the parent element" gap
// ensureIntervalEffectiveTimeXsiType exists for, just on <value> instead of
// <effectiveTime>. Scoped by the low/high-child check alone (mirroring that
// function exactly) — safe because every OTHER <value> in this schema is a
// scalar CD/PQ/CE with @code or @value attributes, never low/high children.
func ensureIntervalPQXsiType(root *etree.Element) {
	for _, v := range root.FindElements(".//value") {
		if v.SelectAttrValue("xsi:type", "") != "" {
			continue
		}
		if v.SelectElement("low") != nil || v.SelectElement("high") != nil {
			v.CreateAttr("xsi:type", "IVL_PQ")
		}
	}
}

// ensureIntervalEffectiveTimeXsiType sets xsi:type="IVL_TS" on every
// <effectiveTime> in the whole document that has a <low> and/or <high>
// child but no xsi:type yet. CDA's base SXCM_TS type — the default,
// unadorned declared type for effectiveTime almost everywhere it appears
// (SubstanceAdministration, Act, Observation, ServiceEvent...) — only
// supports a plain scalar shape (bare @value, no children); carrying
// <low>/<high> children legally requires explicitly selecting the more
// specific IVL_TS type via xsi:type. This engine builds every interval
// effectiveTime (onsetDate/resolutionDate pairs, medication start/stop
// dates, directive periods, documentationOf's own service period, ...) via
// plain WriteAtXPath calls that create <low>/<high> as children with no
// xsi:type at all — confirmed invalid via a real schema validator run
// (2026-07): the anonymous "effectiveTime must have no children, because
// the type's content type is empty" errors, PLUS (once xsi:type is
// missing) several validators cascade the resulting confusion onto that
// element's own later siblings too — this one fix was the root cause behind
// several separately-reported-looking complaints on Allergy/Reaction/
// Severity/Status/Tobacco Use's <value> (each sits right after an
// effectiveTime-with-children sibling), not just the effectiveTime itself.
//
// Deliberately does NOT touch a scalar effectiveTime (bare @value, no
// low/high) — that shape is already valid AS a plain, unadorned TS and
// needs no override; only elements that actually have low/high children
// are missing something. Run once on the whole document (not per-entry)
// since this affects header-level effectiveTime (documentationOf/
// serviceEvent) exactly the same way as entry-level ones.
func ensureIntervalEffectiveTimeXsiType(root *etree.Element) {
	for _, et := range root.FindElements(".//effectiveTime") {
		if et.SelectAttrValue("xsi:type", "") != "" {
			continue
		}
		if et.SelectElement("low") != nil || et.SelectElement("high") != nil {
			et.CreateAttr("xsi:type", "IVL_TS")
		}
	}
}

// unstructuredDocumentFallbackCode/Title are the last-resort values used
// when an Unstructured Document's canonical data supplies neither its own
// documentCode nor a registered docMeta.LOINCCode — the same "34109-9 Note"
// generic-clinical-note code services/cda_fhir/narrative_section_mapper.go's
// own genericNoteLoincCode already uses for the identical "nothing more
// specific is known" case on the parse/mapping side.
const (
	unstructuredDocumentFallbackCode  = "34109-9"
	unstructuredDocumentFallbackTitle = "Note"
)

// buildNonXMLBody builds <nonXMLBody><text .../></nonXMLBody> under
// component — the Unstructured Document shape, mutually exclusive with
// <structuredBody> per CDA's own ClinicalDocument.component CHOICE. data's
// "referenceUrl" key (an external file reference) and "data" key (inline
// base64 content) are themselves mutually exclusive per CDA's ED
// (Encapsulated Data) datatype; referenceUrl wins if both are somehow
// present, since a reference is the more explicit statement of intent.
func buildNonXMLBody(component *etree.Element, data map[string]interface{}) {
	nonXMLBody := component.CreateElement("nonXMLBody")
	text := nonXMLBody.CreateElement("text")

	if mediaType, ok := stringValue(data["mediaType"]); ok {
		text.CreateAttr("mediaType", mediaType)
	}

	if refURL, ok := stringValue(data["referenceUrl"]); ok {
		text.CreateElement("reference").CreateAttr("value", refURL)
		return
	}
	if b64, ok := stringValue(data["data"]); ok {
		text.CreateAttr("representation", "B64")
		text.SetText(b64)
	}
}

// buildSectionElement builds one <component><section> element. altEntries,
// if non-nil, must be parallel to sec.AlternateArchetypes — altEntries[i] is
// AlternateArchetypes[i]'s own already-extracted rows (see
// AlternateEntryArchetype's own doc comment).
func buildSectionElement(sec *cdaSchema.CDASectionDef, entries []map[string]interface{}, altEntries [][]map[string]interface{}) *etree.Element {
	section := etree.NewElement("section")
	if sec.TemplateID != "" {
		tid := section.CreateElement("templateId")
		tid.CreateAttr("root", sec.TemplateID)
		if sec.TemplateIDExt != "" {
			tid.CreateAttr("extension", sec.TemplateIDExt)
		}
		ensureR1CompatTemplateID(section, sec.TemplateID, sec.TemplateIDExt)
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

	rowIDs, altRowIDs := buildSectionNarrative(section, sec, entries, altEntries)

	// Inject each entry's own narrative row ID (as "#id", ready for a
	// "_narrativeRef" field's xpath to write straight into an
	// originalText/reference or bare reference element) AFTER the narrative
	// itself is built — buildSectionNarrative's own column scan runs first,
	// against entries that don't have "_narrativeRef" set yet, so the
	// synthetic key never leaks into the human-readable table as a column.
	for i, entry := range entries {
		if i < len(rowIDs) {
			entry["_narrativeRef"] = "#" + rowIDs[i]
		}
		buildEntry(section, sec, entry)
	}

	for i := range sec.AlternateArchetypes {
		if i >= len(altEntries) {
			break
		}
		for j, record := range altEntries[i] {
			if i < len(altRowIDs) && j < len(altRowIDs[i]) {
				record["_narrativeRef"] = "#" + altRowIDs[i][j]
			}
			buildAlternateEntry(section, &sec.AlternateArchetypes[i], record)
		}
	}

	return section
}

func extractEntries(sectionsMap map[string]interface{}, key string) []map[string]interface{} {
	secData, _ := sectionsMap[key].(map[string]interface{})
	return extractSubEntries(secData, "entries")
}

// extractSubEntries reads one canonical sub-key (secData[subKey], e.g. the
// section's own "entries" or an AlternateEntryArchetype's own EntriesKey)
// and normalizes it to a typed slice — shared by extractEntries (primary
// shape) and BuildDocument's alternate-archetype handling so both accept the
// same two shapes: plain JSON-decoded []interface{}, or the typed
// []map[string]interface{} a caller (e.g. the FHIR bundle adapter) may build
// in-process without a JSON round-trip.
func extractSubEntries(secData map[string]interface{}, subKey string) []map[string]interface{} {
	if secData == nil {
		return nil
	}
	rawEntries, ok := secData[subKey].([]interface{})
	if !ok {
		if typed, ok := secData[subKey].([]map[string]interface{}); ok {
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

func writeDocumentBoilerplate(root *etree.Element, meta *cdaSchema.DocumentTypeMetadata, opts BuildOptions) {
	// realmCode + typeId are the first two children the CDA schema allows
	// before templateId (POCD_MT000040.ClinicalDocument's sequence is
	// realmCode?, typeId, templateId*, id*, code, ...) — both were entirely
	// missing before, which a real schema validator flags as invalid content
	// starting at the very first templateId (confirmed via a real CDA
	// validator run against this builder's own output, 2026-07).
	root.CreateElement("realmCode").CreateAttr("code", "US")
	typeID := root.CreateElement("typeId")
	typeID.CreateAttr("root", "2.16.840.1.113883.1.3")
	typeID.CreateAttr("extension", "POCD_HD000040")

	// US Realm Header (CONF:1198-*, "Conforms to US Realm Header (V3) template
	// urn:hl7ii:2.16.840.1.113883.10.20.22.1.1:2015-08-01") + document-type templateId.
	realmHeader := root.CreateElement("templateId")
	realmHeader.CreateAttr("root", "2.16.840.1.113883.10.20.22.1.1")
	realmHeader.CreateAttr("extension", "2015-08-01")
	ensureR1CompatTemplateID(root, "2.16.840.1.113883.10.20.22.1.1", "2015-08-01")
	if meta != nil && meta.TemplateID != "" {
		docTid := root.CreateElement("templateId")
		docTid.CreateAttr("root", meta.TemplateID)
		if meta.TemplateIDExt != "" {
			docTid.CreateAttr("extension", meta.TemplateIDExt)
		}
		ensureR1CompatTemplateID(root, meta.TemplateID, meta.TemplateIDExt)
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

	root.CreateElement("effectiveTime").CreateAttr("value", nowCDATimestamp(opts.TimezoneOffset))
	// "N" ("normal") confirmed as a real member of ValueSet HL7
	// BasicConfidentialityKind (urn:oid:2.16.840.1.113883.1.11.16926, Table 5)
	// via the IG PDF's own text — codeSystem is HL7Confidentiality
	// (2.16.840.1.113883.5.25), not LOINC. Was previously missing entirely,
	// which is almost certainly why a validator couldn't confirm this
	// against the STATIC valueset despite "N" itself being correct.
	confCode := root.CreateElement("confidentialityCode")
	confCode.CreateAttr("code", "N")
	confCode.CreateAttr("codeSystem", "2.16.840.1.113883.5.25")
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
		addrEl.CreateAttr("use", "HP")
		writeHeaderFields(addrEl, addr, patientAddressFields)
	}

	if phone, ok := stringValue(patientData["phone"]); ok {
		tel := patientRole.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+phone)
		tel.CreateAttr("use", "HP")
	}

	writeHeaderFields(patientRole, patientData, patientScalarFields)
	writeCodedFields(patientRole, patientData, patientCodedFields)

	// patientScalarFields (name/birthTime/languageCommunication) and
	// patientCodedFields (sex/race/ethnicity) are two independent
	// field-mapping tables written via two separate calls, so together they
	// can't produce CDA's required interleaved Patient sequence (name,
	// administrativeGenderCode, birthTime, ..., raceCode, ethnicGroupCode,
	// ..., languageCommunication) by call order alone — reorder once,
	// after both have run, rather than trying to choreograph two unrelated
	// lists into one combined order.
	if patient := patientRole.SelectElement("patient"); patient != nil {
		reorderChildrenByTag(patient, []string{
			"name", "administrativeGenderCode", "birthTime",
			"maritalStatusCode", "religiousAffiliationCode",
			"raceCode", "ethnicGroupCode", "guardian", "birthplace",
			"languageCommunication",
		})
	}
}

func writeAuthorHeader(root *etree.Element, header map[string]interface{}, opts BuildOptions) {
	authorData, _ := header["author"].(map[string]interface{})

	author := root.CreateElement("author")
	author.CreateElement("time").CreateAttr("value", nowCDATimestamp(opts.TimezoneOffset))
	assignedAuthor := author.CreateElement("assignedAuthor")

	writeNPI(assignedAuthor, "id", authorData["npi"])
	writeHeaderFields(assignedAuthor, authorData, authorScalarFields)
	writeHeaderFields(assignedAuthor, authorData, authorCodedFields)

	// addr/telecom reuse patientAddressFields verbatim — same helper, same
	// convention writeCustodianHeader already established, no new XML-
	// writing mechanism. Real SHALL requirements (US Realm Header,
	// CONF:5452/5428) confirmed missing entirely via a real schema
	// validator run (2026-07) — genuinely unsupported before this, not
	// just unmapped.
	if addr, ok := authorData["address"].(map[string]interface{}); ok {
		addrEl := assignedAuthor.CreateElement("addr")
		addrEl.CreateAttr("use", "WP")
		writeHeaderFields(addrEl, addr, patientAddressFields)
	}
	if phone, ok := stringValue(authorData["phone"]); ok {
		tel := assignedAuthor.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+phone)
		tel.CreateAttr("use", "WP")
	}

	assignedAuthor.CreateElement("representedOrganization").CreateElement("name").SetText(resolveOrgName(opts))

	// assignedAuthor's schema sequence is id*, code?, addr*, telecom*,
	// assignedPerson, representedOrganization? — addr/telecom are written
	// AFTER assignedPerson above (simplest call order), so reorder once at
	// the end rather than restructuring the write calls themselves.
	// representedOrganization deliberately isn't listed — it already comes
	// last by construction and belongs last per schema too.
	reorderChildrenByTag(assignedAuthor, []string{"id", "code", "addr", "telecom", "assignedPerson", "assignedAuthoringDevice"})
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
	//
	// telecom is written BEFORE addr — CustodianOrganization
	// (COCT_MT090102UV.Organization, the type backing
	// representedCustodianOrganization) declares its sequence as
	// id*, name?, telecom?, addr?, i.e. the OPPOSITE order from every other
	// org/person-ish element this file writes (author/patient both want
	// addr before telecom) — confirmed missing via a real schema validator
	// run (2026-07): "Invalid content was found starting with element
	// 'telecom'. No child element is expected at this point" once addr had
	// already been (wrongly) written first.
	if cust.Phone != "" {
		tel := org.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+cust.Phone)
		tel.CreateAttr("use", "WP")
	}
	if cust.Street != "" || cust.City != "" || cust.State != "" || cust.PostalCode != "" || cust.Country != "" {
		addr := org.CreateElement("addr")
		addr.CreateAttr("use", "WP")
		writeHeaderFields(addr, map[string]interface{}{
			"street": cust.Street, "city": cust.City, "state": cust.State,
			"postalCode": cust.PostalCode, "country": cust.Country,
		}, patientAddressFields)
	}
}

// writeLegalAuthenticatorHeader writes <legalAuthenticator><time/>
// <signatureCode code="S"/><assignedEntity>...</assignedEntity>
// </legalAuthenticator> — the document's optional (SHOULD 0..1,
// CONF:1198-5579/5580/5583) single legal signer, distinct from author.
// Schema sequence for the document root is author, informant*, custodian,
// informationRecipient*, legalAuthenticator?, ... (base CDA ClinicalDocument
// content model) — called right after custodian in BuildDocument, matching
// the real IG's own worked example (Figure 10).
//
// Only writes anything when a name is configured (Given or Family) — unlike
// Custodian (SHALL exactly one, always written), legalAuthenticator is
// genuinely optional, and an incomplete one (missing assignedPerson/name,
// itself SHALL once the element exists at all) would be a schema violation
// worse than omitting the whole element — every pipeline predating this
// feature simply never gets one, matching prior behavior exactly (there was
// none before).
//
// assignedEntity's own sequence (id*, code?, addr*, telecom*,
// assignedPerson) is the SAME order as assignedAuthor (unlike
// representedCustodianOrganization's reversed telecom-before-addr) —
// written in that exact order directly below, so no reorder pass is needed
// here (unlike writeAuthorHeader, which builds addr/telecom AFTER
// assignedPerson by construction order and must reorder after the fact).
func writeLegalAuthenticatorHeader(root *etree.Element, opts BuildOptions) {
	la := opts.LegalAuthenticator
	if la.Given == "" && la.Family == "" {
		return
	}

	legalAuth := root.CreateElement("legalAuthenticator")
	legalAuth.CreateElement("time").CreateAttr("value", nowCDATimestamp(opts.TimezoneOffset))
	legalAuth.CreateElement("signatureCode").CreateAttr("code", "S")

	assignedEntity := legalAuth.CreateElement("assignedEntity")
	writeNPI(assignedEntity, "id", la.NPI)

	if la.SpecialtyCode != "" {
		code := assignedEntity.CreateElement("code")
		code.CreateAttr("code", la.SpecialtyCode)
		if la.SpecialtyCodeSystem != "" {
			code.CreateAttr("codeSystem", la.SpecialtyCodeSystem)
			injectCodeSystemName(code, la.SpecialtyCodeSystem)
		}
		if la.SpecialtyCodeDisplay != "" {
			code.CreateAttr("displayName", la.SpecialtyCodeDisplay)
		}
	}

	if la.Street != "" || la.City != "" || la.State != "" || la.PostalCode != "" || la.Country != "" {
		addr := assignedEntity.CreateElement("addr")
		addr.CreateAttr("use", "WP")
		writeHeaderFields(addr, map[string]interface{}{
			"street": la.Street, "city": la.City, "state": la.State,
			"postalCode": la.PostalCode, "country": la.Country,
		}, patientAddressFields)
	}
	if la.Phone != "" {
		tel := assignedEntity.CreateElement("telecom")
		tel.CreateAttr("value", "tel:"+la.Phone)
		tel.CreateAttr("use", "WP")
	}

	assignedPerson := assignedEntity.CreateElement("assignedPerson")
	name := assignedPerson.CreateElement("name")
	if la.Given != "" {
		name.CreateElement("given").SetText(la.Given)
	}
	if la.Family != "" {
		name.CreateElement("family").SetText(la.Family)
	}
}

// writePersonReference writes one <tag><assignedEntity>...</assignedEntity></tag>
// element — the shape shared by document-level informants[] and
// documentationOf performers[]: an optional NPI-rooted id (only when npi
// data is present, unlike author's identity which always carries one),
// an optional specialty code (CONF:1198-14842 for documentationOf
// performers — genuinely absent before this, unlike every per-section
// author field which already has its own specialty code), optional name,
// and (informants only) an optional represented organization. Written in
// AssignedEntity's own real sequence (id, code, addr, telecom,
// assignedPerson, representedOrganization — same order Round 20's
// legalAuthenticator and Round 22's assignedEntity fix both already
// established) — code MUST come before assignedPerson/name. Returns the
// outer <tag> element so callers can add their own tag-specific attributes
// (e.g. performer's fixed typeCode="PRF").
func writePersonReference(root *etree.Element, tag string, person map[string]interface{}, includeOrg bool) *etree.Element {
	el := root.CreateElement(tag)
	assignedEntity := el.CreateElement("assignedEntity")

	if npi, ok := stringValue(person["npi"]); ok {
		writeNPI(assignedEntity, "id", npi)
	}
	if specialtyCode, ok := stringValue(person["specialtyCode"]); ok {
		code := assignedEntity.CreateElement("code")
		code.CreateAttr("code", specialtyCode)
		if system, ok := stringValue(person["specialtyCodeSystem"]); ok {
			code.CreateAttr("codeSystem", system)
			injectCodeSystemName(code, system)
		}
		if display, ok := stringValue(person["specialtyCodeDisplay"]); ok {
			code.CreateAttr("displayName", display)
		}
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
