// cda/builder/header_requirements.go
//
// Header-field conformance (SHALL/SHOULD) for the guided-configuration
// UI's Requirements checklist — see requirements_catalog.go for the
// section-level counterpart.
//
// cda/schemas/ccda_2_1.json already carries curated, IG-vetted conformance
// data for the document header in three isHeader:true pseudo-sections —
// "header.patient", "header.author", "header.custodian" — used today only
// by the CDA->FHIR PARSE-direction declarative engine
// (services/cda_fhir/document_mapper.go's MapCustodian and friends). Their
// field-key vocabulary (patientId, givenName, custodianOrgName, ...) is a
// DIFFERENT vocabulary than this package's BUILD-direction canonical JSON
// keys (firstName, lastName, name, ...) that header_fields.go's
// writeXHeader functions actually read — a deliberate, documented split
// (see header_fields.go's own file header comment: a translation layer was
// explicitly avoided there for the parse/build split).
//
// Rather than hand-guessing SHALL/SHOULD levels for the guided UI (a second,
// error-prone judgment call on top of an already-vetted one), this file
// bridges the two vocabularies with a small translation table and reads
// conformance straight from the schema — single source of truth, no
// duplicated regulatory judgment.
//
// authorOrgName and authorTime are deliberately absent from the "author"
// translation table below: document_builder.go's writeAuthorHeader writes
// both unconditionally (author/time is always "now"; the represented
// organization name always falls back to resolveOrgName(opts)) regardless
// of canonical input, so neither can ever actually be "missing" — including
// them in a guided completeness checklist would nag about something the
// user cannot violate.
//
// custodian's translation table only covers custodianOrgName/custodianId —
// header.custodian in ccda_2_1.json defines no address/telecom fields at
// all (unlike header.patient), so this package does not assert an
// address/telecom requirement invented out of nothing. cda.build's own
// Custodian step config still accepts address/phone as useful, purely
// optional data entry (see cda_build_executor.go's CdaCustodianConfig).
package builder

import (
	"sort"
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/uscdi"
)

// headerKeyTranslation maps schemaKey (a header.<group> field's own Key in
// ccda_2_1.json) to canonicalKey (the key header_fields.go's writeXHeader
// functions read from the canonical JSON header, i.e. the same vocabulary
// HeaderFieldCatalog already exposes for cda.map_to_canonical's mapping UI).
var headerKeyTranslation = map[string]map[string]string{
	"patient": {
		"patientId":    "ids", // patientRole/id — repeating group, see idItemFields
		"givenName":    "firstName",
		"familyName":   "lastName",
		"nameSuffix":   "nameSuffix",
		"birthDate":    "dateOfBirth",
		"gender":       "sex",
		"deceasedInd":  "deceasedInd",
		"deceasedTime": "deceasedTime",
		"race":         "race",
		"ethnicity":    "ethnicity",
		"addressLine":  "address.street",
		"city":         "address.city",
		"state":        "address.state",
		"postalCode":   "address.postalCode",
		"telecom":      "phone",
	},
	"author": {
		"authorNPI":    "npi",
		"authorGiven":  "given",
		"authorFamily": "family",
	},
	"custodian": {
		"custodianOrgName": "name",
		"custodianId":      "id",
	},
	// legalAuthenticator's own translation table, unlike custodian's, DOES
	// cover address/telecom — CONF:1198-5589/5595 genuinely require them
	// (SHALL at least one) once the element exists at all, a real
	// difference from custodian's own spec (which has no such requirement —
	// see this file's header comment on why custodian's table deliberately
	// omits them). writeLegalAuthenticatorHeader itself still treats
	// addr/telecom as conditionally-written (only when configured), same as
	// every other optional data-entry field in this package — the
	// difference here is purely about what the Requirements-tab badge
	// should HONESTLY tell a user filling this in, not a new enforcement
	// mechanism.
	"legalAuthenticator": {
		"legalAuthenticatorGiven":       "given",
		"legalAuthenticatorFamily":      "family",
		"legalAuthenticatorNPI":         "npi",
		"legalAuthenticatorAddressLine": "street",
		"legalAuthenticatorCity":        "city",
		"legalAuthenticatorState":       "state",
		"legalAuthenticatorPostalCode":  "postalCode",
		"legalAuthenticatorTelecom":     "phone",
	},
}

// HeaderFieldRequirement is one canonical header field's conformance level,
// for the guided-configuration UI's Requirements checklist.
type HeaderFieldRequirement struct {
	Key         string `json:"key"`         // canonical key, e.g. "firstName" or "address.street"
	Label       string `json:"label"`       // human label, from the schema's USCDIElement
	Conformance string `json:"conformance"` // "SHALL" | "SHOULD"

	// USCDIClasses is the real, ONC-verified USCDI v3 class set this field
	// maps to (cda/schemas/uscdi_v3.json via uscdi.USCDIVocabulary), omitted
	// when unmapped — never fabricated. Resolved the same additive,
	// honestly-absent-when-unknown way Coverage Audit's own
	// CategoryStat.USCDIClasses already works (services/cda_coverage/
	// inventory.go's uscdiClassesForSection) — see classesForHeaderField's
	// own doc comment for the one wrinkle specific to header fields (a
	// dotted canonicalKey like "address.street" needs to match uscdi_v3.
	// json's coarser bare "address" cdaField).
	USCDIClasses []string `json:"uscdiClasses,omitempty"`
}

// HeaderRequirementsCatalog returns the canonical-keyed requirement list for
// one header group ("patient" | "author" | "custodian"), sourced live from
// loader.GetSection("header."+group) and translated via headerKeyTranslation
// above. vocabulary may be nil (graceful degradation — see
// classesForHeaderField). Returns nil for an unrecognized group or a schema
// section that can't be loaded (mirrors HeaderFieldCatalog's own
// nil-for-unknown convention in canonical_field_catalog.go).
func HeaderRequirementsCatalog(loader *cdaSchema.CDASchemaLoader, vocabulary *uscdi.USCDIVocabulary, group string) []HeaderFieldRequirement {
	translation, ok := headerKeyTranslation[group]
	if !ok {
		return nil
	}
	sec := loader.GetSection("header." + group)
	if sec == nil {
		return nil
	}

	out := make([]HeaderFieldRequirement, 0, len(translation))
	for _, f := range sec.Fields {
		canonicalKey, ok := translation[f.Key]
		if !ok {
			continue // schema field with no build-direction counterpart (e.g. patientIdRoot, custodianIdRoot — root OID is fixed at write time, not user-supplied)
		}
		out = append(out, HeaderFieldRequirement{
			Key:          canonicalKey,
			Label:        f.USCDIElement,
			Conformance:  f.Conformance,
			USCDIClasses: classesForHeaderField(vocabulary, group, canonicalKey),
		})
	}
	return out
}

// classesForHeaderField filters vocabulary's "header.<group>" elements down
// to the ones whose own CDAField matches canonicalKey, returning the
// deduped, alphabetically-sorted class set (nil when vocabulary is nil,
// unmapped, or — the common case for section-level-only USCDI elements like
// header.author's Provenance authorTime/authorOrganization, whose CDAField
// is "" since they don't correspond to any one specific canonical field —
// no match at all, correctly not attributed to an unrelated field like
// "given" or "family").
//
// canonicalKey may be dotted (e.g. "address.street", "address.city") —
// header_requirements.go's own translation table splits ONE schema field
// ("addressLine"/"city"/"state"/"postalCode" in header.patient.json, all
// backing the same canonical "address" object) into 4 separate UI-facing
// requirement rows, but uscdi_v3.json's own header.patient entry for address
// uses the coarser bare "address" cdaField — matching on the part before the
// first "." lets all 4 rows correctly resolve to the same real USCDI class
// instead of silently finding nothing due to a granularity mismatch that
// isn't a real absence.
func classesForHeaderField(vocabulary *uscdi.USCDIVocabulary, group, canonicalKey string) []string {
	if vocabulary == nil {
		return nil
	}
	prefix := canonicalKey
	if dot := strings.Index(canonicalKey, "."); dot != -1 {
		prefix = canonicalKey[:dot]
	}
	seen := make(map[string]bool)
	var classes []string
	for _, el := range vocabulary.GetByCDASection("header." + group) {
		if el.CDAField != canonicalKey && el.CDAField != prefix {
			continue
		}
		if seen[el.Class] {
			continue
		}
		seen[el.Class] = true
		classes = append(classes, el.Class)
	}
	sort.Strings(classes)
	return classes
}
