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
	default:
		// Unmapped OID (e.g. ICD-9-CM 2.16.840.1.113883.6.103, a local payer code
		// system, HL7's MaritalStatus table 2.16.840.1.113883.5.2, ...). FHIR
		// requires Coding.system to be an absolute URI — bare OIDs are not — so
		// fall back to the standard "urn:oid:" form rather than passing the raw
		// OID through unprefixed. Values that aren't OID-shaped (already a URI,
		// or a free-text system name we don't recognize) are returned unchanged.
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
	"http://loinc.org":                     true,
	"http://hl7.org/fhir/sid/icd-10-cm":   true,
	"http://snomed.info/sct":               true,
	"http://hl7.org/fhir/sid/cvx":         true,
	"http://hl7.org/fhir/sid/ndc":         true,
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
	if code.NullFlavor != "" && code.Code == "" && code.DisplayName == "" && code.OriginalText == "" {
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

// IIToIdentifier converts a typed CDAII (HL7 Instance Identifier) to a FHIR Identifier.
// Returns nil when both Root and Extension are empty.
func IIToIdentifier(id cdadocument.CDAII) map[string]interface{} {
	value := id.Extension
	if value == "" {
		value = id.Root // some implementations encode the ID only in root
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
