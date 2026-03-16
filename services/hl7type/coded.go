// services/hl7type/coded.go
// HL7 coded element parsers: CE, CWE, CNE → FHIR CodeableConcept.
// All functions are lenient and never return errors.
package hl7type

import (
	"strings"
)

// MapCodingSystemToURI maps an HL7 coding system name to its canonical FHIR URI.
// For unknown systems the original name is returned unchanged so no data is lost.
func MapCodingSystemToURI(hl7System string) string {
	if hl7System == "" {
		return ""
	}

	// Static map of well-known coding systems
	uriMap := map[string]string{
		// LOINC
		"LN":     "http://loinc.org",
		"LOINC":  "http://loinc.org",
		// SNOMED
		"SCT":        "http://snomed.info/sct",
		"SNOMEDCT":   "http://snomed.info/sct",
		"SNOMED-CT":  "http://snomed.info/sct",
		// RxNorm
		"RXNORM": "http://www.nlm.nih.gov/research/umls/rxnorm",
		// ICD
		"ICD10":   "http://hl7.org/fhir/sid/icd-10",
		"ICD-10":  "http://hl7.org/fhir/sid/icd-10",
		"ICD10CM": "http://hl7.org/fhir/sid/icd-10-cm",
		"ICD-10-CM": "http://hl7.org/fhir/sid/icd-10-cm",
		"ICD9CM":  "http://hl7.org/fhir/sid/icd-9-cm",
		"ICD-9-CM": "http://hl7.org/fhir/sid/icd-9-cm",
		"I9":      "http://hl7.org/fhir/sid/icd-9-cm",
		"I10":     "http://hl7.org/fhir/sid/icd-10",
		// CPT
		"CPT4":  "http://www.ama-assn.org/go/cpt",
		"CPT-4": "http://www.ama-assn.org/go/cpt",
		"C4":    "http://www.ama-assn.org/go/cpt",
		"CPT":   "http://www.ama-assn.org/go/cpt",
		// HCPCS
		"HCPCS": "http://www.nlm.nih.gov/research/umls/hcpcs",
		// UCUM
		"UCUM": "http://unitsofmeasure.org",
		// NCI
		"NCI":  "http://ncimeta.nci.nih.gov",
		"NCIT": "http://ncimeta.nci.nih.gov",
		// NDC
		"NDC": "http://hl7.org/fhir/sid/ndc",
		// CVX (Vaccine)
		"CVX": "http://hl7.org/fhir/sid/cvx",
		// MVX (Manufacturer of Vaccines)
		"MVX": "http://terminology.hl7.org/CodeSystem/MVX",
		// HL7 v2 tables – pattern HL7NNNNN maps to v2-NNNNN
		// Common tables pre-registered here for quick access:
		"HL70001": "http://terminology.hl7.org/CodeSystem/v2-0001",
		"HL70002": "http://terminology.hl7.org/CodeSystem/v2-0002",
		"HL70003": "http://terminology.hl7.org/CodeSystem/v2-0003",
		"HL70004": "http://terminology.hl7.org/CodeSystem/v2-0004",
		"HL70005": "http://terminology.hl7.org/CodeSystem/v2-0005",
		"HL70006": "http://terminology.hl7.org/CodeSystem/v2-0006",
		"HL70007": "http://terminology.hl7.org/CodeSystem/v2-0007",
		"HL70008": "http://terminology.hl7.org/CodeSystem/v2-0008",
		"HL70061": "http://terminology.hl7.org/CodeSystem/v2-0061",
		"HL70063": "http://terminology.hl7.org/CodeSystem/v2-0063",
		"HL70078": "http://terminology.hl7.org/CodeSystem/v2-0078",
		"HL70085": "http://terminology.hl7.org/CodeSystem/v2-0085",
		"HL70096": "http://terminology.hl7.org/CodeSystem/v2-0096",
		"HL70105": "http://terminology.hl7.org/CodeSystem/v2-0105",
		"HL70116": "http://terminology.hl7.org/CodeSystem/v2-0116",
		"HL70123": "http://terminology.hl7.org/CodeSystem/v2-0123",
		"HL70125": "http://terminology.hl7.org/CodeSystem/v2-0125",
		"HL70131": "http://terminology.hl7.org/CodeSystem/v2-0131",
		"HL70136": "http://terminology.hl7.org/CodeSystem/v2-0136",
		"HL70155": "http://terminology.hl7.org/CodeSystem/v2-0155",
		"HL70162": "http://terminology.hl7.org/CodeSystem/v2-0162",
		"HL70185": "http://terminology.hl7.org/CodeSystem/v2-0185",
		"HL70190": "http://terminology.hl7.org/CodeSystem/v2-0190",
		"HL70200": "http://terminology.hl7.org/CodeSystem/v2-0200",
		"HL70201": "http://terminology.hl7.org/CodeSystem/v2-0201",
		"HL70202": "http://terminology.hl7.org/CodeSystem/v2-0202",
		"HL70203": "http://terminology.hl7.org/CodeSystem/v2-0203",
		"HL70204": "http://terminology.hl7.org/CodeSystem/v2-0204",
		"HL70207": "http://terminology.hl7.org/CodeSystem/v2-0207",
		"HL70255": "http://terminology.hl7.org/CodeSystem/v2-0255",
		"HL70301": "http://terminology.hl7.org/CodeSystem/v2-0301",
		"HL70354": "http://terminology.hl7.org/CodeSystem/v2-0354",
		"HL70396": "http://terminology.hl7.org/CodeSystem/v2-0396",
		"HL70406": "http://terminology.hl7.org/CodeSystem/v2-0406",
		"HL70441": "http://terminology.hl7.org/CodeSystem/v2-0441",
		"HL70480": "http://terminology.hl7.org/CodeSystem/v2-0480",
	}

	// Direct lookup (case-insensitive prefix matching for HL70xxx)
	upper := strings.ToUpper(hl7System)
	if uri, ok := uriMap[upper]; ok {
		return uri
	}

	// Dynamic HL7 v2 table pattern: HL7NNNNN
	if strings.HasPrefix(upper, "HL7") {
		tableNum := upper[3:]
		if isAllDigits(tableNum) && len(tableNum) > 0 {
			return "http://terminology.hl7.org/CodeSystem/v2-" + tableNum
		}
	}

	// Unknown system — return as-is so no information is lost
	return hl7System
}

// parseCoded is the shared parser for CE/CWE/CNE.
// CE format: identifier^text^nameOfCodingSystem^altIdentifier^altText^altCodingSystem
// CWE adds: ^codingSystemVersion^altCodingSystemVersion^originalText^...
func parseCoded(raw string, rules map[string]interface{}, allowAlt bool) (map[string]interface{}, []string) {
	if raw == "" {
		return map[string]interface{}{"coding": []interface{}{}}, nil
	}

	var warnings []string
	components := strings.Split(raw, "^")

	// Helper to safely get component by 0-based index
	get := func(idx int) string {
		if idx < len(components) {
			return strings.TrimSpace(components[idx])
		}
		return ""
	}

	codeableConcept := map[string]interface{}{}
	var codings []interface{}

	// Primary coding (components 0-2)
	code := get(0)
	display := get(1)
	system := get(2)

	if code != "" || system != "" {
		coding := map[string]interface{}{}
		if code != "" {
			coding["code"] = code
		}
		if display != "" {
			coding["display"] = display
		}
		systemURI := MapCodingSystemToURI(system)
		if systemURI != "" {
			coding["system"] = systemURI
		}

		// Coding system version (CWE component 6, 0-based index 6)
		if version := get(6); version != "" {
			coding["version"] = version
		}

		// Allow system override from rules
		if rules != nil {
			if sysOverride, ok := rules["system"].(string); ok && sysOverride != "" {
				coding["system"] = sysOverride
			}
		}

		codings = append(codings, coding)
	}

	// Alternate coding (components 3-5), only for CE and CWE (not CNE)
	if allowAlt {
		altCode := get(3)
		altDisplay := get(4)
		altSystem := get(5)

		if altCode != "" || altSystem != "" {
			altCoding := map[string]interface{}{}
			if altCode != "" {
				altCoding["code"] = altCode
			}
			if altDisplay != "" {
				altCoding["display"] = altDisplay
			}
			altSystemURI := MapCodingSystemToURI(altSystem)
			if altSystemURI != "" {
				altCoding["system"] = altSystemURI
			}

			// Alt coding system version (CWE component 7, 0-based index 7)
			if altVersion := get(7); altVersion != "" {
				altCoding["version"] = altVersion
			}

			codings = append(codings, altCoding)
		}
	}

	// Original text (CWE component 8, 0-based index 8)
	originalText := get(8)
	// Also use component 1 (text) as the human-readable text
	textDisplay := display
	if originalText != "" {
		textDisplay = originalText
	}
	if textDisplay != "" {
		codeableConcept["text"] = textDisplay
	}

	if len(codings) == 0 {
		// Lenient: if we have no codings, create a minimal one with the raw value
		warnings = append(warnings, "no code found in coded element, using raw value as text")
		codeableConcept["text"] = raw
		codings = []interface{}{}
	}

	codeableConcept["coding"] = codings

	return codeableConcept, warnings
}

// ParseCE converts an HL7 CE element to a FHIR CodeableConcept map.
// CE: identifier^text^nameOfCodingSystem^altIdentifier^altText^altCodingSystem
func ParseCE(raw string, rules map[string]interface{}) (map[string]interface{}, []string) {
	return parseCoded(raw, rules, true)
}

// ParseCWE converts an HL7 CWE element to a FHIR CodeableConcept map.
// CWE adds version and originalText components beyond CE.
func ParseCWE(raw string, rules map[string]interface{}) (map[string]interface{}, []string) {
	return parseCoded(raw, rules, true)
}

// ParseCNE converts an HL7 CNE element to a FHIR CodeableConcept map.
// CNE is like CWE but does not allow alternate coding systems.
func ParseCNE(raw string, rules map[string]interface{}) (map[string]interface{}, []string) {
	return parseCoded(raw, rules, false)
}
