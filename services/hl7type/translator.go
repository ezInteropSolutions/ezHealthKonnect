// services/hl7type/translator.go
// Main entry point for the hl7type translation library.
// AutoTranslate converts a raw HL7 string value to the appropriate Go/FHIR type
// based on the HL7 data type and/or FHIR target type from the schema.
package hl7type

import (
	"strings"
)

// TranslationResult holds the converted value and any warnings generated
// during the translation process.
type TranslationResult struct {
	Value    interface{}
	Warnings []string
}

// AutoTranslate is the main entry point for automatic type translation.
// Called when DataTypeTransform is empty (no explicit transform configured).
//
//   raw       - the raw HL7 string value to translate
//   hl7Type   - HL7 data type from schema (TS, CE, XPN, NM, …) — may be ""
//   fhirType  - FHIR element data type from schema (dateTime, CodeableConcept, …) — may be ""
//   rules     - optional transformation rules from mapping config (may be nil)
//
// Returns a best-effort translation; never returns an error (lenient).
// When both types are unknown, returns the raw string unchanged.
func AutoTranslate(raw, hl7Type, fhirType string, rules map[string]interface{}) TranslationResult {
	if raw == "" {
		return TranslationResult{Value: raw}
	}

	hl7Upper := strings.ToUpper(strings.TrimSpace(hl7Type))
	fhirLower := strings.ToLower(strings.TrimSpace(fhirType))

	// ─── Type Matrix: prefer (hl7Type, fhirType) pair lookups ───────────────────

	// Timestamp types
	if hl7Upper == "TS" || hl7Upper == "DTM" {
		return translateTimestamp(raw, fhirLower)
	}
	if hl7Upper == "DT" {
		v, w := ParseDT(raw)
		return TranslationResult{Value: v, Warnings: w}
	}
	if hl7Upper == "TM" {
		v, w := ParseTM(raw)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Numeric types
	if hl7Upper == "NM" {
		return translateNumeric(raw, fhirLower)
	}
	if hl7Upper == "SI" {
		v, w := ParseSI(raw)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Boolean
	if hl7Upper == "BL" {
		v, w := ParseBL(raw)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Coded element types
	if hl7Upper == "CE" {
		v, w := ParseCE(raw, rules)
		return TranslationResult{Value: v, Warnings: w}
	}
	if hl7Upper == "CWE" {
		v, w := ParseCWE(raw, rules)
		return TranslationResult{Value: v, Warnings: w}
	}
	if hl7Upper == "CNE" {
		v, w := ParseCNE(raw, rules)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Pass-through coded types
	if hl7Upper == "ID" || hl7Upper == "IS" {
		return TranslationResult{Value: raw}
	}

	// Pass-through string types
	if hl7Upper == "ST" || hl7Upper == "TX" || hl7Upper == "FT" {
		return TranslationResult{Value: raw}
	}

	// Complex person / contact / address types
	if hl7Upper == "XPN" {
		v := ParseXPN(raw, rules)
		return TranslationResult{Value: v}
	}
	if hl7Upper == "XAD" {
		v := ParseXAD(raw, rules)
		return TranslationResult{Value: v}
	}
	if hl7Upper == "XTN" {
		v := ParseXTN(raw, rules)
		return TranslationResult{Value: v}
	}
	if hl7Upper == "CX" {
		v := ParseCX(raw, rules)
		return TranslationResult{Value: v}
	}

	// Hierarchic / entity identifier types
	if hl7Upper == "HD" {
		// When the FHIR target is a URI (e.g. identifier.system), resolve the HD
		// assigning authority to a proper FHIR system URI instead of returning a map.
		fhirLower := strings.ToLower(fhirType)
		if fhirLower == "uri" || fhirLower == "url" || fhirLower == "canonical" {
			return TranslationResult{Value: MapAssigningAuthorityToURI(raw)}
		}
		v := ParseHD(raw)
		return TranslationResult{Value: v}
	}
	if hl7Upper == "EI" {
		v := ParseEI(raw, rules)
		return TranslationResult{Value: v}
	}

	// Organization name
	if hl7Upper == "XON" {
		v := ParseXON(raw)
		return TranslationResult{Value: v}
	}

	// Date/Time range → Period
	if hl7Upper == "DR" {
		v, w := ParseDR(raw)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Structured numeric → Quantity or Range
	if hl7Upper == "SN" {
		v, w := ParseSN(raw)
		return TranslationResult{Value: v, Warnings: w}
	}

	// Person location
	if hl7Upper == "PL" {
		v := ParsePL(raw)
		return TranslationResult{Value: v}
	}

	// ─── HL7 type unknown: fall back to FHIR type heuristics ────────────────────

	if fhirLower != "" {
		return translateByFHIRType(raw, fhirLower, rules)
	}

	// Both unknown — return raw unchanged
	return TranslationResult{Value: raw}
}

// translateTimestamp picks the right ParseTS target based on FHIR type.
func translateTimestamp(raw, fhirLower string) TranslationResult {
	var target string
	switch fhirLower {
	case "date":
		target = "date"
	case "instant":
		target = "instant"
	case "time":
		target = "time"
	default:
		target = "dateTime"
	}
	v, w := ParseTS(raw, target)
	return TranslationResult{Value: v, Warnings: w}
}

// translateNumeric picks the right ParseNM target based on FHIR type.
func translateNumeric(raw, fhirLower string) TranslationResult {
	var target string
	switch fhirLower {
	case "integer", "positiveint", "unsignedint":
		target = fhirLower
	default:
		target = "decimal"
	}
	v, w := ParseNM(raw, target)
	return TranslationResult{Value: v, Warnings: w}
}

// translateByFHIRType applies FHIR-type-based heuristics when the HL7 type is unknown.
func translateByFHIRType(raw, fhirLower string, rules map[string]interface{}) TranslationResult {
	switch fhirLower {
	case "datetime", "instant":
		v, w := ParseTS(raw, "dateTime")
		return TranslationResult{Value: v, Warnings: w}
	case "date":
		v, w := ParseTS(raw, "date")
		return TranslationResult{Value: v, Warnings: w}
	case "time":
		v, w := ParseTM(raw)
		return TranslationResult{Value: v, Warnings: w}
	case "decimal":
		v, w := ParseNM(raw, "decimal")
		return TranslationResult{Value: v, Warnings: w}
	case "integer":
		v, w := ParseNM(raw, "integer")
		return TranslationResult{Value: v, Warnings: w}
	case "positiveint":
		v, w := ParseNM(raw, "positiveInt")
		return TranslationResult{Value: v, Warnings: w}
	case "unsignedint":
		v, w := ParseNM(raw, "unsignedInt")
		return TranslationResult{Value: v, Warnings: w}
	case "boolean":
		v, w := ParseBL(raw)
		return TranslationResult{Value: v, Warnings: w}
	case "codeableconcept":
		v, w := ParseCE(raw, rules)
		return TranslationResult{Value: v, Warnings: w}
	case "coding":
		v, w := ParseCE(raw, rules)
		return TranslationResult{Value: v, Warnings: w}
	case "humanname":
		v := ParseXPN(raw, rules)
		return TranslationResult{Value: v}
	case "address":
		v := ParseXAD(raw, rules)
		return TranslationResult{Value: v}
	case "contactpoint":
		v := ParseXTN(raw, rules)
		return TranslationResult{Value: v}
	case "identifier":
		v := ParseCX(raw, rules)
		return TranslationResult{Value: v}
	case "period":
		v, w := ParseDR(raw)
		return TranslationResult{Value: v, Warnings: w}
	case "quantity", "range":
		v, w := ParseSN(raw)
		return TranslationResult{Value: v, Warnings: w}
	case "uri", "url", "canonical":
		// Resolve any name/OID/URL to a proper FHIR system URI
		return TranslationResult{Value: ResolveSystemURI(raw)}
	case "string", "markdown", "code", "id":
		return TranslationResult{Value: raw}
	default:
		return TranslationResult{Value: raw}
	}
}
