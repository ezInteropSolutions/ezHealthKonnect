// cda/builder/header_fields.go
//
// Declarative field mappings for CDA header construction — the header-side
// counterpart to entry_archetypes.go's schema-driven section engine. The
// canonical header JSON uses a fixed, flat key vocabulary
// (firstName/lastName/dateOfBirth/...) produced by
// services/parsers/cda/cda_parser_service.go's patientToLegacyJSON/
// authorToJSON — a DIFFERENT vocabulary than cda/schemas/ccda_2_1.json's
// header.patient/header.author field keys (givenName/authorGiven/...), so
// this file defines its own small mapping tables rather than reusing the
// section schema directly (doing that would require either a translation
// layer or a breaking change to cda.parse's canonical JSON contract —
// neither justified for this refactor).
//
// Three shapes cover every header field:
//   - headerFieldMapping + writeHeaderFields: flat scalar (canonical key -> one XPath)
//   - codedFieldMapping + writeCodedFields: a code/display pair sharing one
//     element, plus a FIXED codeSystem (sex/race/ethnicity)
//   - repeatingGroupMapping + writeRepeatingGroup: one new element per item
//     in a canonical array field (patient ids[], informants[], performers[])
package builder

import "github.com/beevik/etree"

// ─── Flat scalar fields ──────────────────────────────────────────────────────

type headerFieldMapping struct {
	CanonicalKey string
	XPath        string // relative to the element writeHeaderFields is called with
}

// writeHeaderFields writes every mapping whose CanonicalKey is present in
// data — the generic replacement for what used to be one hardcoded
// `if v, ok := ...; ok { el.CreateElement(...).SetText(v) }` block per field.
func writeHeaderFields(root *etree.Element, data map[string]interface{}, mappings []headerFieldMapping) {
	for _, m := range mappings {
		if v, ok := stringValue(data[m.CanonicalKey]); ok {
			WriteAtXPath(root, m.XPath, v)
		}
	}
}

// Order matters: writeHeaderFields creates elements in list order (each
// entry's first present field creates <patient> itself), and CDA's Patient
// schema requires name before administrativeGenderCode/birthTime/... —
// name-related fields MUST come first in this list, confirmed missing via a
// real schema validator run against this builder's own output (2026-07)
// which flagged <name> as invalid content once it followed <birthTime>.
var patientScalarFields = []headerFieldMapping{
	// firstName/middleName use positional predicates (given[1]/given[2]) so
	// two calls create two sibling <given> elements instead of one call
	// overwriting the other — WriteAtXPath's existing [N] positional-predicate
	// support (already built for CDA entries) applies unchanged here.
	{"firstName", "patient/name/given[1]"},
	{"middleName", "patient/name/given[2]"},
	{"lastName", "patient/name/family"},
	{"dateOfBirth", "patient/birthTime/@value"},
	{"preferredLanguage", "patient/languageCommunication/languageCode/@code"},
}

var patientAddressFields = []headerFieldMapping{
	{"street", "streetAddressLine"},
	{"city", "city"},
	{"state", "state"},
	{"postalCode", "postalCode"},
	{"country", "country"},
}

var authorScalarFields = []headerFieldMapping{
	{"given", "assignedPerson/name/given"},
	{"family", "assignedPerson/name/family"},
}

var healthCareFacilityFields = []headerFieldMapping{
	{"facilityTypeCode", "code/@code"},
	{"facilityName", "location/name"},
	{"facilityOrgName", "serviceProviderOrganization/name"},
}

// itemMappings for the three repeating-group fields (see writeRepeatingGroup).
var idItemFields = []headerFieldMapping{
	{"root", "@root"},
	{"extension", "@extension"},
}

// personItemFields is relative to an <assignedEntity> element directly
// (used by writePersonReference in document_builder.go, not
// writeRepeatingGroup — informant/performer identities need the fixed-root
// NPI shape writeNPIIfPresent provides, which doesn't fit the flat
// {canonicalKey, path} table itself).
var personItemFields = []headerFieldMapping{
	{"given", "assignedPerson/name/given"},
	{"family", "assignedPerson/name/family"},
}

// ─── Coded fields (code + display + fixed codeSystem) ────────────────────────

type codedFieldMapping struct {
	CodeKey    string // canonical key for the code, e.g. "sex"
	DisplayKey string // canonical key for the display, e.g. "sexDisplay"
	XPath      string // element path relative to root, e.g. "patient/administrativeGenderCode"
	CodeSystem string // fixed OID — only written when CodeKey is actually present
}

// writeCodedFields writes @code/@codeSystem/@displayName onto one element
// per mapping, but only when the code itself is present — never creates an
// empty coded element just to hold a fixed codeSystem with no real code.
func writeCodedFields(root *etree.Element, data map[string]interface{}, mappings []codedFieldMapping) {
	for _, m := range mappings {
		code, ok := stringValue(data[m.CodeKey])
		if !ok {
			continue
		}
		WriteAtXPath(root, m.XPath+"/@code", code)
		WriteAtXPath(root, m.XPath+"/@codeSystem", m.CodeSystem)
		if disp, ok := stringValue(data[m.DisplayKey]); ok {
			WriteAtXPath(root, m.XPath+"/@displayName", disp)
		}
	}
}

var patientCodedFields = []codedFieldMapping{
	{"sex", "sexDisplay", "patient/administrativeGenderCode", "2.16.840.1.113883.5.1"},
	{"race", "raceDisplay", "patient/raceCode", "2.16.840.1.113883.6.238"},
	{"ethnicity", "ethnicityDisplay", "patient/ethnicGroupCode", "2.16.840.1.113883.6.238"},
	{"maritalStatus", "maritalStatusDisplay", "patient/maritalStatusCode", "2.16.840.1.113883.5.2"},
}

// authorCodedFields covers assignedAuthor's own optional specialty/taxonomy
// code (CONF:1198-16787/16788 — "Since the assignedAuthor is an
// assignedPerson, the assignedAuthor SHOULD contain code"). Healthcare
// Provider Taxonomy (HIPAA) is a DYNAMIC valueset per the IG (no fixed
// codeSystem list to validate against at build time), so CodeSystem is left
// for the canonical data itself to supply via a companion System key exactly
// like every entry-level xpathSystem field already does — writeCodedFields'
// fixed-CodeSystem model doesn't fit a DYNAMIC-bound field, so this uses
// writeHeaderFields column-by-column instead of the coded-field helper.
var authorCodedFields = []headerFieldMapping{
	{"specialtyCode", "code/@code"},
	{"specialtyCodeSystem", "code/@codeSystem"},
	{"specialtyCodeDisplay", "code/@displayName"},
}

// ─── Repeating groups (one new element per array item) ──────────────────────

// writeRepeatingGroup creates one NEW <tag> child of root per item in
// data[arrayKey] ([]interface{} of maps), applying itemMappings (paths
// relative to that one new element) via WriteAtXPath. Reused for patient
// ids[], document informants[], and documentationOf performers[] — three
// header fields that are structurally "a list of records," not one-off.
// Always creates a fresh element per item (unlike WriteAtXPath's bare-tag
// find-or-reuse semantics), since repeating siblings — not one shared node
// — is exactly what these fields need.
func writeRepeatingGroup(root *etree.Element, data map[string]interface{}, arrayKey, tag string, itemMappings []headerFieldMapping) {
	items, ok := data[arrayKey].([]interface{})
	if !ok {
		return
	}
	for _, itemRaw := range items {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		itemEl := root.CreateElement(tag)
		writeHeaderFields(itemEl, item, itemMappings)
	}
}
