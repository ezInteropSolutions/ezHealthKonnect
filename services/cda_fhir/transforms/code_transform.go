// Package transforms provides pure typed-struct CDA→FHIR conversion functions.
// All inputs are typed cdadocument structs; no etree, no XPath, no map inputs.
package transforms

import (
	"regexp"
	"strings"

	cdadocument "ezhealthkonnect/cda/document"
)

// oidPattern matches a bare HL7 OID such as "2.16.840.1.113883.6.103"
// (dot-separated digit groups, no leading scheme).
var oidPattern = regexp.MustCompile(`^\d+(\.\d+)+$`)

// normalizeSystem maps a CDA OID or code system name to its FHIR canonical URI.
// Internal — callers outside this package use the cdafhir.NormalizeSystem copy.
func normalizeSystem(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "RXNORM", "2.16.840.1.113883.6.88":
		return "http://www.nlm.nih.gov/research/umls/rxnorm"
	case "SNOMED CT", "SNOMED", "SNOMEDCT", "2.16.840.1.113883.6.96":
		return "http://snomed.info/sct"
	case "LOINC", "2.16.840.1.113883.6.1":
		return "http://loinc.org"
	case "ICD-10-CM", "ICD10CM", "ICD10", "2.16.840.1.113883.6.90":
		return "http://hl7.org/fhir/sid/icd-10-cm"
	case "CVX", "2.16.840.1.113883.12.292":
		return "http://hl7.org/fhir/sid/cvx"
	case "NDC", "2.16.840.1.113883.6.69":
		return "http://hl7.org/fhir/sid/ndc"
	case "CPT", "CPT4", "2.16.840.1.113883.6.12":
		return "http://www.ama-assn.org/go/cpt"
	case "NCI", "2.16.840.1.113883.3.26.1.1":
		return "http://ncithesaurus.nci.nih.gov"
	case "ACTCODE", "2.16.840.1.113883.5.4":
		return "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	case "MARITALSTATUS", "2.16.840.1.113883.5.2":
		return "http://terminology.hl7.org/CodeSystem/v3-MaritalStatus"
	case "ROLECODE", "2.16.840.1.113883.5.88":
		return "http://terminology.hl7.org/CodeSystem/v3-RoleCode"
	default:
		// Unmapped OID (e.g. ICD-9-CM 2.16.840.1.113883.6.103, a vendor-internal
		// proprietary code system, ...). FHIR requires Coding.system to be an
		// absolute URI — bare OIDs are not — so fall back to the standard
		// "urn:oid:" form rather than passing the raw OID through unprefixed.
		// This is the correct outcome for non-standard/vendor OIDs: there is no
		// universal canonical URL to guess at, and resolving vendor-specific
		// systems is an interface-builder/pipeline-configuration concern, not
		// something this generic mapper should hardcode. Values that aren't
		// OID-shaped (already a URI, or a free-text system name we don't
		// recognize) are returned unchanged.
		if oidPattern.MatchString(raw) {
			return "urn:oid:" + raw
		}
		return raw
	}
}

// terminologyValidatedSystems lists FHIR canonical system URIs whose display values
// are verified by the FHIR validator against the official terminology server.
// For these systems we omit the source CDA's displayName from codings — the source
// may carry a short/alias form that differs from the canonical display and would
// produce a hard validation error. The CodeableConcept.text field carries the
// human-readable text instead (set in CDACodeToCodeableConcept).
var terminologyValidatedSystems = map[string]bool{
	"http://loinc.org":                            true,
	"http://hl7.org/fhir/sid/icd-10-cm":           true,
	"http://snomed.info/sct":                      true,
	"http://hl7.org/fhir/sid/cvx":                 true,
	"http://hl7.org/fhir/sid/ndc":                 true,
	"http://www.nlm.nih.gov/research/umls/rxnorm": true,
}

// CDACodeToCoding converts a typed CDACode to a FHIR Coding map.
// Returns nil when the code carries no meaningful data.
func CDACodeToCoding(code cdadocument.CDACode) map[string]interface{} {
	if code.NullFlavor != "" && code.Code == "" && code.DisplayName == "" {
		return nil
	}
	c := map[string]interface{}{}
	sys := ""
	if code.CodeSystem != "" {
		sys = normalizeSystem(code.CodeSystem)
		c["system"] = sys
	}
	if code.Code != "" {
		c["code"] = code.Code
	}
	// Omit display for standard systems whose canonical text is validated by the
	// FHIR terminology server — the source CDA's displayName may be a short/alias
	// form and would produce a hard "Wrong Display Name" validation error.
	// display-only codings (no code) are kept so human-readable text is preserved.
	if code.DisplayName != "" && (code.Code == "" || !terminologyValidatedSystems[sys]) {
		c["display"] = code.DisplayName
	}
	if len(c) == 0 {
		return nil
	}
	return c
}

// CDACodeToCodeableConcept converts a typed CDACode to a FHIR CodeableConcept.
// Includes all Translations as additional codings.
// Returns nil for null-flavored or completely empty codes.
func CDACodeToCodeableConcept(code cdadocument.CDACode) map[string]interface{} {
	// Translations must gate this early return too -- a 747-file real-world
	// corpus run (github.com/chb/sample_ccdas) found the common idiom of a
	// nullFlavor'd primary code (Code/DisplayName/OriginalText all empty)
	// paired with a <translation> child carrying the actual usable code
	// (e.g. a vendor-internal code marked nullFlavor, SNOMED supplied only
	// as a translation). Bailing here before len(code.Translations) is even
	// checked silently dropped that data -- the single largest cause of
	// Condition.code coming back completely empty across the corpus.
	if code.NullFlavor != "" && code.Code == "" && code.DisplayName == "" && code.OriginalText == "" && len(code.Translations) == 0 {
		return nil
	}

	var codings []interface{}

	if primary := CDACodeToCoding(code); primary != nil {
		codings = append(codings, primary)
	}
	for _, t := range code.Translations {
		if coding := CDACodeToCoding(t); coding != nil {
			codings = append(codings, coding)
		}
	}

	if len(codings) == 0 && code.OriginalText == "" {
		return nil
	}

	cc := map[string]interface{}{}
	if len(codings) > 0 {
		cc["coding"] = codings
	}
	// text: DisplayName preferred, original text as fallback
	if code.DisplayName != "" {
		cc["text"] = code.DisplayName
	} else if code.OriginalText != "" {
		cc["text"] = code.OriginalText
	}

	if len(cc) == 0 {
		return nil
	}
	return cc
}

// coveragePayerTypeDisplay corrects known LOINC display names that differ
// from the FHIR canonical value (the source CDA carries short display text,
// FHIR validators use the official LOINC long name). 48768-6 "Payment
// sources" is CONF:1198-19160's fixed code for Coverage Activity (verified
// against the actual IG 2026-06-22, R2.1) -- the correction is purely
// cosmetic/display, not a code change.
var coveragePayerTypeDisplay = map[string]string{
	"48768-6": "Payment sources Document",
}

// cdaToFHIRRelationship maps HL7 RoleCode values (system
// 2.16.840.1.113883.5.111) from participant[COV].participantRole.code to
// FHIR subscriber-relationship ValueSet codes.
// Unmapped codes fall back to "other" per FHIR binding guidance.
var cdaToFHIRRelationship = map[string]string{
	"SELF":      "self",
	"SPOUSE":    "spouse",
	"SPS":       "spouse",
	"CHILD":     "child",
	"CHLDADOPT": "child",
	"NCHILD":    "child",
	"MTH":       "parent",
	"FTH":       "parent",
	"PRNT":      "parent",
	"NPRNT":     "parent",
	"NMTH":      "parent",
	"NFTH":      "parent",
	"SIGOTHR":   "other",
	"DEP":       "other",
	"WARD":      "other",
	"HBRO":      "other",
	"HSIB":      "other",
	"HSIS":      "other",
	"NBRO":      "other",
	"NSIB":      "other",
	"NSIS":      "other",
	"SIB":       "other",
	"SIS":       "other",
}

var cdaRelationshipDisplay = map[string]string{
	"self":   "Self",
	"spouse": "Spouse",
	"child":  "Child",
	"parent": "Parent",
	"other":  "Other",
}

// CoverageRelationshipToFHIR converts a C-CDA participant[COV].participantRole.code
// (HL7 RoleCode system 2.16.840.1.113883.5.111 — SELF, SPOUSE, CHILD, etc.)
// to a FHIR Coverage.relationship CodeableConcept using the
// http://terminology.hl7.org/CodeSystem/subscriber-relationship system.
// Unmapped codes produce "other".
func CoverageRelationshipToFHIR(code cdadocument.CDACode) map[string]interface{} {
	fhirCode := cdaToFHIRRelationship[strings.ToUpper(code.Code)]
	if fhirCode == "" {
		fhirCode = "other"
	}
	display := cdaRelationshipDisplay[fhirCode]
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/subscriber-relationship",
				"code":    fhirCode,
				"display": display,
			},
		},
	}
}

// CoverageTypeToCodeableConcept converts a Policy Activity's SOP code to a
// FHIR Coverage.type CodeableConcept (CDACodeToCodeableConcept +
// coveragePayerTypeDisplay LOINC display correction for any stale callers
// still passing the outer 48768-6 code).
func CoverageTypeToCodeableConcept(code cdadocument.CDACode) map[string]interface{} {
	cc := CDACodeToCodeableConcept(code)
	if cc == nil {
		return nil
	}
	codings, ok := cc["coding"].([]interface{})
	if !ok || len(codings) == 0 {
		return cc
	}
	c0, ok := codings[0].(map[string]interface{})
	if !ok {
		return cc
	}
	if codeStr, _ := c0["code"].(string); codeStr != "" {
		if canonical, ok := coveragePayerTypeDisplay[codeStr]; ok {
			c0["display"] = canonical
			cc["text"] = canonical
		}
	}
	return cc
}

// IIToIdentifier converts a typed CDAII (HL7 Instance Identifier) to a FHIR Identifier.
// Returns nil when both Root and Extension are empty, or when Extension is empty and
// Root is a fixed national identifier system OID (see isFixedIdentifierSystem).
func IIToIdentifier(id cdadocument.CDAII) map[string]interface{} {
	value := id.Extension
	if value == "" && !isFixedIdentifierSystem(id.Root) {
		// Some implementations legitimately encode the identifier value only in
		// root for generic/facility-specific OIDs (no fixed national meaning) —
		// keep that fallback. But for NPI/SSN/Medicare/MBI/DL, root is ALWAYS the
		// system's own OID, never a patient-specific value: falling back here
		// would produce a schema-valid but semantically garbage Identifier
		// (e.g. value="2.16.840.1.113883.4.6", the NPI system OID itself, not an
		// actual 10-digit NPI). Confirmed against real vendor data: PracticeFusion's
		// corpus sample has 3 authors with exactly this malformed
		// <id root="2.16.840.1.113883.4.6"/> (no @extension) shape.
		value = id.Root
	}
	if value == "" {
		return nil
	}
	ident := map[string]interface{}{"value": value}
	if sys := IdentifierSystem(id.Root); sys != "" {
		ident["system"] = sys
	}
	typeCode := IdentifierTypeCode(id.Root)
	ident["type"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
				"code":   typeCode,
			},
		},
	}
	return ident
}
