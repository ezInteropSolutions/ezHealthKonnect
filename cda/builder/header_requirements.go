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

import cdaSchema "ezhealthkonnect/cda"

// headerKeyTranslation maps schemaKey (a header.<group> field's own Key in
// ccda_2_1.json) to canonicalKey (the key header_fields.go's writeXHeader
// functions read from the canonical JSON header, i.e. the same vocabulary
// HeaderFieldCatalog already exposes for cda.map_to_canonical's mapping UI).
var headerKeyTranslation = map[string]map[string]string{
	"patient": {
		"patientId":   "ids", // patientRole/id — repeating group, see idItemFields
		"givenName":   "firstName",
		"familyName":  "lastName",
		"birthDate":   "dateOfBirth",
		"gender":      "sex",
		"race":        "race",
		"ethnicity":   "ethnicity",
		"addressLine": "address.street",
		"city":        "address.city",
		"state":       "address.state",
		"postalCode":  "address.postalCode",
		"telecom":     "phone",
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
}

// HeaderFieldRequirement is one canonical header field's conformance level,
// for the guided-configuration UI's Requirements checklist.
type HeaderFieldRequirement struct {
	Key         string `json:"key"`         // canonical key, e.g. "firstName" or "address.street"
	Label       string `json:"label"`       // human label, from the schema's USCDIElement
	Conformance string `json:"conformance"` // "SHALL" | "SHOULD"
}

// HeaderRequirementsCatalog returns the canonical-keyed requirement list for
// one header group ("patient" | "author" | "custodian"), sourced live from
// loader.GetSection("header."+group) and translated via headerKeyTranslation
// above. Returns nil for an unrecognized group or a schema section that
// can't be loaded (mirrors HeaderFieldCatalog's own nil-for-unknown
// convention in canonical_field_catalog.go).
func HeaderRequirementsCatalog(loader *cdaSchema.CDASchemaLoader, group string) []HeaderFieldRequirement {
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
			Key:         canonicalKey,
			Label:       f.USCDIElement,
			Conformance: f.Conformance,
		})
	}
	return out
}
