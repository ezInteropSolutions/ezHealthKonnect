// services/cda_fhir/code_mapper.go
// Shared utilities for CDA↔FHIR code system normalization and date conversion.

package cdafhir

import (
	"fmt"
	"strings"
)

// NormalizeSystem converts a CDA code system OID or short name to its FHIR
// canonical URI. Returns the input unchanged if no mapping is found.
func NormalizeSystem(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	// RxNorm
	case "RXNORM", "2.16.840.1.113883.6.88":
		return "http://www.nlm.nih.gov/research/umls/rxnorm"
	// SNOMED CT
	case "SNOMED CT", "SNOMED", "SNOMEDCT", "2.16.840.1.113883.6.96":
		return "http://snomed.info/sct"
	// LOINC
	case "LOINC", "2.16.840.1.113883.6.1":
		return "http://loinc.org"
	// ICD-10-CM
	case "ICD-10-CM", "ICD10CM", "ICD10", "2.16.840.1.113883.6.90":
		return "http://hl7.org/fhir/sid/icd-10-cm"
	// CVX (vaccine codes)
	case "CVX", "2.16.840.1.113883.12.292":
		return "http://hl7.org/fhir/sid/cvx"
	// NDC
	case "NDC", "2.16.840.1.113883.6.69":
		return "http://hl7.org/fhir/sid/ndc"
	// CPT
	case "CPT", "CPT4", "2.16.840.1.113883.6.12":
		return "http://www.ama-assn.org/go/cpt"
	// NCI Thesaurus
	case "NCI", "2.16.840.1.113883.3.26.1.1":
		return "http://ncithesaurus.nci.nih.gov"
	// HL7 ActCode
	case "ACTCODE", "2.16.840.1.113883.5.4":
		return "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	// HL7 AdministrativeGender
	case "ADMINGENDER", "2.16.840.1.113883.5.1":
		return "http://terminology.hl7.org/CodeSystem/v3-AdministrativeGender"
	default:
		return raw
	}
}

// FormatDate converts a CDA timestamp (YYYYMMDD, YYYYMMDDHHMMSS, or
// YYYYMMDDHHMMSS+HHMM) to an ISO 8601 date or dateTime string.
// Returns empty string if input is empty or unrecognisable.
func FormatDate(cda string) string {
	s := strings.TrimSpace(cda)
	if s == "" {
		return ""
	}
	// Strip timezone suffix for length check
	tz := ""
	for _, sep := range []string{"+", "-"} {
		if idx := strings.LastIndex(s, sep); idx >= 8 {
			tz = s[idx:]
			s = s[:idx]
			break
		}
	}

	switch {
	case len(s) >= 14:
		// YYYYMMDDHHMMSS → YYYY-MM-DDTHH:MM:SS
		dt := fmt.Sprintf("%s-%s-%sT%s:%s:%s",
			s[0:4], s[4:6], s[6:8],
			s[8:10], s[10:12], s[12:14])
		if tz != "" {
			dt += normalizeTimezone(tz)
		}
		return dt
	case len(s) >= 8:
		// YYYYMMDD → YYYY-MM-DD
		return fmt.Sprintf("%s-%s-%s", s[0:4], s[4:6], s[6:8])
	case len(s) >= 6:
		// YYYYMM → YYYY-MM
		return fmt.Sprintf("%s-%s", s[0:4], s[4:6])
	case len(s) == 4:
		return s // year only
	default:
		return s
	}
}

// normalizeTimezone converts "+0500" → "+05:00" or "-0800" → "-08:00".
func normalizeTimezone(tz string) string {
	if len(tz) == 5 {
		return tz[:3] + ":" + tz[3:]
	}
	return tz
}

// NewCoding returns a FHIR Coding map.
func NewCoding(code, display, system string) map[string]interface{} {
	c := map[string]interface{}{}
	if system != "" {
		c["system"] = NormalizeSystem(system)
	}
	if code != "" {
		c["code"] = code
	}
	if display != "" {
		c["display"] = display
	}
	return c
}

// NewCodeableConcept returns a FHIR CodeableConcept map with one coding.
func NewCodeableConcept(code, display, system string) map[string]interface{} {
	cc := map[string]interface{}{}
	coding := NewCoding(code, display, system)
	if len(coding) > 0 {
		cc["coding"] = []interface{}{coding}
	}
	if display != "" {
		cc["text"] = display
	}
	return cc
}

// strField returns the string value at key from m, or "" if absent.
func strField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
