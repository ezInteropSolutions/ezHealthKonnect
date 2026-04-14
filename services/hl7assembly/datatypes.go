// services/hl7assembly/datatypes.go
//
// Canonical FHIR R4 data type builders for HL7 v2 → FHIR conversions.
//
// Design principle (per FHIR spec): FHIR is built from a small set of
// reusable complex types (CodeableConcept, Identifier, HumanName, Address,
// ContactPoint, Attachment, Quantity …).  Every resource that carries a coded
// value uses CodeableConcept; every identifier uses Identifier; and so on.
// One builder per type, called from anywhere — no per-resource one-offs.
//
// Public API (exported):
//   NormalizeCodingSystem      HL7 Table 0396 abbreviation → FHIR system URI
//   FacilityNamespaceURI       MSH.4 → "urn:facility:{slug}" Tier-3 URI
//   BuildCodeableConceptFromCE HL7 CE/CWE/CNE → FHIR CodeableConcept
//   CEToCodeableConcept        Convenience wrapper (no facility namespace)
//   NormalizeIdentifierSystem  Any HL7 system string → absolute FHIR URI
//   BuildIdentifierFromCX      One HL7 CX repetition → FHIR Identifier
//   BuildIdentifiersFromCX     Multiple CX repetitions (~ separator) → []Identifier
//   BuildHumanNameFromXPN      HL7 XPN → FHIR HumanName
//   BuildAddressFromXAD        HL7 XAD → FHIR Address
//   BuildContactPointFromXTN   HL7 XTN / plain email → FHIR ContactPoint
//   BuildAttachmentFromED      HL7 ED  → FHIR Attachment (inline data)
//   BuildAttachmentFromRP      HL7 RP  → FHIR Attachment (by reference)
//   ToISO                      HL7 TS timestamp → ISO 8601 string
//   HL7V2IdentifierTypeSystem  FHIR system URI for HL7 v2 Table 0203
//   Table0203                  HL7 Table 0203 code → display name
//
// Status mappers (already exported in assembly.go; re-exported here for
// package-wide visibility without import-cycle risk):
//   OBRStatusToDRStatus, ORCStatusToDRStatus, OBXStatusToObsStatus
//
// This file belongs to package hl7assembly and may import only standard
// library and packages below it in the dependency graph (currently none).
package hl7assembly

import (
	"encoding/base64"
	"strings"
)

// ──────────────────────────────────────────────────────────────────────────────
// Constants & look-up tables
// ──────────────────────────────────────────────────────────────────────────────

// HL7V2IdentifierTypeSystem is the FHIR system URI for HL7 v2 Table 0203 codes.
const HL7V2IdentifierTypeSystem = "http://terminology.hl7.org/CodeSystem/v2-0203"

// Table0203 maps HL7 Table 0203 identifier type codes to their canonical
// FHIR display values.
var Table0203 = map[string]string{
	"MR":  "Medical record number",
	"PI":  "Patient internal identifier",
	"PT":  "Patient external identifier",
	"AN":  "Account number",
	"SS":  "Social Security number",
	"DL":  "Driver's license number",
	"PPN": "Passport number",
	"NI":  "National unique individual identifier",
	"MA":  "Patient Medicaid number",
	"MC":  "Patient's Medicare number",
	"EN":  "Employer number",
	"TAX": "Tax ID number",
	"PRN": "Provider number",
	"SB":  "Social Beneficiary Identifier",
	"PN":  "Person number",
	"VS":  "Visit number",
	"RR":  "Railroad Retirement number",
	"RRI": "Regional registry ID",
	"LN":  "License number",
	"LI":  "Labor and industries number",
	"NPI": "National Provider Identifier",
	"DEA": "Drug Enforcement Administration registration number",
	"TRL": "Training license number",
	"NH":  "National Health Plan Identifier",
	"EI":  "Employee number",
	"RI":  "Resource identifier",
	"PA":  "Patient account number",
	"JHN": "Jurisdictional health number",
	"PRC": "Permanent Resident Card Number",
	"CN":  "Staff Enterprise Number",
	"CY":  "County number",
	"BA":  "Bank Account Number",
	"BR":  "Birth registry number",
}

// wellKnownExternalSystems is the set of canonical FHIR system URIs for
// established external code systems (LOINC, SNOMED, CPT, etc.).
// When a Coding uses one of these systems, the source display name from the HL7
// message is suppressed — it is almost always an unofficial abbreviation that
// will fail FHIR validator "Wrong Display Name" checks.
var wellKnownExternalSystems = map[string]bool{
	"http://loinc.org":                            true,
	"http://snomed.info/sct":                      true,
	"http://www.ama-assn.org/go/cpt":              true,
	"http://www.nlm.nih.gov/research/umls/rxnorm": true,
	"http://hl7.org/fhir/sid/icd-10-cm":           true,
	"http://hl7.org/fhir/sid/icd-9-cm":            true,
	"http://hl7.org/fhir/sid/ndc":                 true,
	"http://hl7.org/fhir/sid/cvx":                 true,
	"http://hl7.org/fhir/sid/mvx":                 true,
}

// localOnlySystems is the set of HL7 Table 0396 values that represent
// locally-defined code systems — they are not valid absolute URIs and
// must be replaced by a facility-namespace URI or dropped.
var localOnlySystems = map[string]bool{
	"L": true, "LOCAL": true, "99ZZZ": true,
}

// ──────────────────────────────────────────────────────────────────────────────
// Coding system normalisation  (HL7 Table 0396 → FHIR system URI)
// ──────────────────────────────────────────────────────────────────────────────

// NormalizeCodingSystem converts an HL7 Table 0396 coding system abbreviation
// (e.g. "LN", "SCT", "I10") to the canonical FHIR Coding.system URI.
//
// Return values:
//   - (uri, false)  – a usable absolute URI was resolved.
//   - ("",  true)   – the system is a local-only code (L / LOCAL / 99ZZZ) and
//     the caller should apply a facility-namespace Tier-3 URI instead.
//   - (sys, false)  – the input was not recognised; returned unchanged so the
//     caller can decide whether to keep or drop it.
func NormalizeCodingSystem(system string) (string, bool) {
	upper := strings.ToUpper(strings.TrimSpace(system))
	if localOnlySystems[upper] {
		return "", true
	}
	switch upper {
	case "LN", "LOINC":
		return "http://loinc.org", false
	case "SCT", "SNM", "SNOMED", "SNOMEDCT", "SNOMED-CT":
		return "http://snomed.info/sct", false
	case "UCUM":
		return "http://unitsofmeasure.org", false
	case "RXNORM", "RXN":
		return "http://www.nlm.nih.gov/research/umls/rxnorm", false
	case "I10", "ICD10", "ICD-10", "ICD-10-CM":
		return "http://hl7.org/fhir/sid/icd-10-cm", false
	case "I9", "ICD9", "ICD-9", "ICD-9-CM":
		return "http://hl7.org/fhir/sid/icd-9-cm", false
	case "C4", "CPT", "CPT4", "CPT-4":
		return "http://www.ama-assn.org/go/cpt", false
	case "NDC":
		return "http://hl7.org/fhir/sid/ndc", false
	case "CVX":
		return "http://hl7.org/fhir/sid/cvx", false
	case "MVX":
		return "http://hl7.org/fhir/sid/mvx", false
	case "NPI":
		return "http://hl7.org/fhir/sid/us-npi", false
	case "HL70396", "0396":
		return "http://terminology.hl7.org/CodeSystem/v2-0396", false
	default:
		return system, false
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Facility namespace  (Tier-3 fallback)
// ──────────────────────────────────────────────────────────────────────────────

// FacilityNamespaceURI constructs a facility-scoped system URI from the
// sending facility name (typically MSH.4).
//
// Example: "XYZ LAB" → "urn:facility:xyz-lab"
//
// The result is a valid absolute URI (satisfies FHIR §4.1 Coding.system
// cardinality) while being honest that the code system is locally defined by
// the sending facility.  Apply only when no standard system is available
// (Tier-3 fallback in the coding resolution hierarchy).
func FacilityNamespaceURI(sendingFacility string) string {
	slug := strings.ToLower(strings.TrimSpace(sendingFacility))
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-' || r == '.':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "unknown"
	}
	return "urn:facility:" + s
}

// ──────────────────────────────────────────────────────────────────────────────
// CodeableConcept builder  (HL7 CE / CWE / CNE)
// ──────────────────────────────────────────────────────────────────────────────

// BuildCodeableConceptFromCE converts an HL7 CE/CWE/CNE field value to a
// FHIR R4 CodeableConcept map.
//
// CE/CWE wire format (components separated by ^):
//
//	Code ^ Text ^ CodingSystem ^ AltCode ^ AltText ^ AltCodingSystem
//
// facilityNS is the Tier-3 fallback system URI (e.g. "urn:facility:xyz-lab"),
// applied when no standard coding system is available so every Coding entry
// has a valid absolute URI.
//
// Returns the CodeableConcept and a bool that is true when the Tier-3 facility
// namespace was actually used — callers can emit a warning once per message.
func BuildCodeableConceptFromCE(ce, facilityNS string) (map[string]interface{}, bool) {
	components := strings.Split(ce, "^")
	cc := map[string]interface{}{}
	var codings []interface{}
	tier3Used := false

	if len(components) > 0 && components[0] != "" {
		coding := map[string]interface{}{"code": components[0]}

		if len(components) > 2 && components[2] != "" {
			sys, isLocal := NormalizeCodingSystem(components[2])
			if isLocal {
				if facilityNS != "" {
					coding["system"] = facilityNS
					tier3Used = true
				}
				if len(components) > 1 && components[1] != "" {
					coding["display"] = components[1]
					cc["text"] = components[1]
				}
			} else {
				coding["system"] = sys
				if !wellKnownExternalSystems[sys] {
					if len(components) > 1 && components[1] != "" {
						coding["display"] = components[1]
						cc["text"] = components[1]
					}
				} else if len(components) > 1 && components[1] != "" {
					cc["text"] = components[1]
				}
			}
		} else {
			// No system component — apply Tier-3 so Coding.system cardinality is met.
			if facilityNS != "" {
				coding["system"] = facilityNS
				tier3Used = true
			}
			if len(components) > 1 && components[1] != "" {
				coding["display"] = components[1]
				cc["text"] = components[1]
			}
		}
		codings = append(codings, coding)
	}

	// Alternate coding (CWE components 4-6, 1-indexed = 3-5 zero-indexed)
	if len(components) > 3 && components[3] != "" {
		alt := map[string]interface{}{"code": components[3]}
		if len(components) > 5 && components[5] != "" {
			sys, isLocal := NormalizeCodingSystem(components[5])
			if isLocal {
				if facilityNS != "" {
					alt["system"] = facilityNS
					tier3Used = true
				}
				if !wellKnownExternalSystems[facilityNS] {
					if len(components) > 4 && components[4] != "" {
						alt["display"] = components[4]
					}
				}
			} else {
				alt["system"] = sys
				if !wellKnownExternalSystems[sys] {
					if len(components) > 4 && components[4] != "" {
						alt["display"] = components[4]
					}
				}
			}
		} else {
			if facilityNS != "" {
				alt["system"] = facilityNS
				tier3Used = true
			}
			if len(components) > 4 && components[4] != "" {
				alt["display"] = components[4]
			}
		}
		if !codingIsDuplicate(alt, codings) {
			codings = append(codings, alt)
		}
	}

	cc["coding"] = codings
	return cc, tier3Used
}

// CEToCodeableConcept is the zero-facility-namespace variant of
// BuildCodeableConceptFromCE.  Use this when you have no MSH.4 context and
// therefore cannot apply a Tier-3 system URI.  Codings without a standard
// system will simply be emitted without a system field.
func CEToCodeableConcept(ce string) map[string]interface{} {
	cc, _ := BuildCodeableConceptFromCE(ce, "")
	return cc
}

// ──────────────────────────────────────────────────────────────────────────────
// Identifier system normalisation  (HL7 HD / CX.4)
// ──────────────────────────────────────────────────────────────────────────────

// NormalizeIdentifierSystem converts any HL7 identifier system string to a
// valid FHIR Identifier.system URI.  Handles three input forms:
//
//  1. Already a URI (http://, https://, urn:) — returned unchanged.
//  2. Bare OID digits (e.g. "2.16.840.1.113883.4.1") — prefixed with urn:oid:
//  3. HL7 HD (Hierarchic Designator) encoded as "NamespaceID&UniversalID&TypeCode"
//     — the form used in CX.4 (Assigning Authority).
//     Conversion follows HL7 Table 0301 universal ID type codes:
//     ISO  → urn:oid:{UniversalID}
//     UUID → urn:uuid:{UniversalID}
//     DNS  → urn:dns:{UniversalID}
//     URI/URL → {UniversalID} (used as-is)
//     If the type code is absent or unrecognised and the UniversalID looks like
//     an OID, urn:oid: is assumed.
//
// Returns "" when the system cannot be resolved to a valid absolute URI so
// callers can omit the field rather than write an invalid value.
func NormalizeIdentifierSystem(sys string) string {
	sys = strings.TrimSpace(sys)
	if sys == "" {
		return ""
	}
	if strings.HasPrefix(sys, "http://") || strings.HasPrefix(sys, "https://") ||
		strings.HasPrefix(sys, "urn:") {
		return sys
	}
	// HL7 HD field: "NamespaceID&UniversalID&UniversalIDType"
	if strings.Contains(sys, "&") {
		parts := strings.Split(sys, "&")
		universalID := ""
		universalIDType := ""
		if len(parts) >= 2 {
			universalID = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			universalIDType = strings.ToUpper(strings.TrimSpace(parts[2]))
		}
		switch universalIDType {
		case "ISO":
			if universalID != "" {
				return "urn:oid:" + universalID
			}
		case "UUID":
			if universalID != "" {
				return "urn:uuid:" + universalID
			}
		case "DNS":
			if universalID != "" {
				return "urn:dns:" + universalID
			}
		case "URI", "URL":
			if universalID != "" {
				return universalID
			}
		default:
			if universalID != "" && isOIDLike(universalID) {
				return "urn:oid:" + universalID
			}
		}
		return "" // unresolvable — omit rather than write invalid system
	}
	// Bare OID (starts with digit, contains dots)
	if len(sys) > 0 && sys[0] >= '0' && sys[0] <= '9' {
		return "urn:oid:" + sys
	}
	return sys
}

// isOIDLike returns true when s looks like an ISO OID (digits and dots only,
// at least one dot, e.g. "1.2.840.114398.1.100").
func isOIDLike(s string) bool {
	hasDot := false
	for _, c := range s {
		if c == '.' {
			hasDot = true
		} else if c < '0' || c > '9' {
			return false
		}
	}
	return hasDot
}

// ──────────────────────────────────────────────────────────────────────────────
// Identifier builder  (HL7 CX composite)
// ──────────────────────────────────────────────────────────────────────────────

// BuildIdentifierFromCX converts one HL7 CX repetition to a FHIR Identifier.
//
// CX wire format (HL7 v2.5+ Table 2.A.14):
//
//	CX.1  ID value
//	CX.2  Check Digit
//	CX.3  Check Digit Scheme
//	CX.4  Assigning Authority (HD: NamespaceID&UniversalID&TypeCode) → system
//	CX.5  Identifier Type Code (Table 0203: MR, SS, PI …)            → type
//	CX.6  Assigning Facility (HD)
//	CX.7  Effective Date
//	CX.8  Expiration Date
//
// rules is an optional map from TransformationRules that may override "use".
func BuildIdentifierFromCX(cx string, rules map[string]interface{}) map[string]interface{} {
	// Strip any remaining ~ repetitions (caller should split via BuildIdentifiersFromCX).
	if idx := strings.Index(cx, "~"); idx >= 0 {
		cx = cx[:idx]
	}
	components := strings.Split(cx, "^")
	identifier := map[string]interface{}{"use": "usual"}

	// CX.1 — the ID value itself
	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}

	// CX.4 — Assigning Authority HD → identifier.system
	if len(components) > 3 && components[3] != "" {
		if sys := NormalizeIdentifierSystem(components[3]); sys != "" {
			identifier["system"] = sys
		}
	}

	// CX.5 — Identifier Type Code (Table 0203) → identifier.type
	if len(components) > 4 && components[4] != "" {
		typeCode := strings.ToUpper(strings.TrimSpace(components[4]))
		typeDef := map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": HL7V2IdentifierTypeSystem,
					"code":   typeCode,
				},
			},
			"text": typeCode,
		}
		if label, ok := Table0203[typeCode]; ok {
			typeDef["text"] = label
			coding := typeDef["coding"].([]interface{})[0].(map[string]interface{})
			coding["display"] = label
		}
		identifier["type"] = typeDef
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}
	return identifier
}

// BuildIdentifiersFromCX handles a CX field that may contain multiple
// repetitions separated by '~', returning a slice of FHIR Identifier maps.
//
// Example:  "12345^^^MIE&1.2.840.114398.1.100&ISO^MR~987654321^^^^SS"
// →  [{value:"12345", system:"urn:oid:1.2.840.114398.1.100", type:{MR}},
//
//	{value:"987654321", type:{SS}}]
func BuildIdentifiersFromCX(cx string, rules map[string]interface{}) []interface{} {
	repetitions := strings.Split(cx, "~")
	result := make([]interface{}, 0, len(repetitions))
	for _, rep := range repetitions {
		rep = strings.TrimSpace(rep)
		if rep == "" {
			continue
		}
		result = append(result, BuildIdentifierFromCX(rep, rules))
	}
	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// HumanName builder  (HL7 XPN)
// ──────────────────────────────────────────────────────────────────────────────

// BuildHumanNameFromXPN converts an HL7 XPN (Extended Person Name) field to a
// FHIR R4 HumanName map.
//
// XPN wire format:
//
//	FamilyName ^ GivenName ^ SecondName ^ Suffix ^ Prefix ^ Degree ^ NameTypeCode …
func BuildHumanNameFromXPN(xpn string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xpn, "^")
	name := map[string]interface{}{"use": "official"}

	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}
	var given []string
	if len(components) > 1 && components[1] != "" {
		given = append(given, components[1])
	}
	if len(components) > 2 && components[2] != "" {
		given = append(given, components[2])
	}
	if len(given) > 0 {
		name["given"] = given
	}
	if len(components) > 3 && components[3] != "" {
		name["suffix"] = []string{components[3]}
	}
	if len(components) > 4 && components[4] != "" {
		name["prefix"] = []string{components[4]}
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			name["use"] = use
		}
	}
	return name
}

// ──────────────────────────────────────────────────────────────────────────────
// Address builder  (HL7 XAD)
// ──────────────────────────────────────────────────────────────────────────────

// BuildAddressFromXAD converts an HL7 XAD (Extended Address) field to a
// FHIR R4 Address map.
//
// XAD wire format:
//
//	StreetLine1 ^ StreetLine2 ^ City ^ State ^ Zip ^ Country ^ AddressType …
func BuildAddressFromXAD(xad string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xad, "^")
	address := map[string]interface{}{"use": "home"}

	var lines []string
	if len(components) > 0 && components[0] != "" {
		lines = append(lines, components[0])
	}
	if len(components) > 1 && components[1] != "" {
		lines = append(lines, components[1])
	}
	if len(lines) > 0 {
		address["line"] = lines
	}
	if len(components) > 2 && components[2] != "" {
		address["city"] = components[2]
	}
	if len(components) > 3 && components[3] != "" {
		address["state"] = components[3]
	}
	if len(components) > 4 && components[4] != "" {
		address["postalCode"] = components[4]
	}
	if len(components) > 5 && components[5] != "" {
		address["country"] = components[5]
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			address["use"] = use
		}
	}
	return address
}

// ──────────────────────────────────────────────────────────────────────────────
// ContactPoint builder  (HL7 XTN / plain email)
// ──────────────────────────────────────────────────────────────────────────────

// BuildContactPointFromXTN converts an HL7 XTN (Extended Telecommunication
// Number) field or a plain email address string to a FHIR R4 ContactPoint map.
//
// XTN wire format (v2.5+):
//
//	TelephoneNumber ^ TelecomUseCode ^ TelecomEquipType ^ EmailAddress ^ …
//
// If the raw string contains no '^' it is treated as either a plain phone
// number or a plain email address (detected by the presence of '@').
func BuildContactPointFromXTN(xtn string, rules map[string]interface{}) map[string]interface{} {
	cp := map[string]interface{}{
		"system": "phone",
		"use":    "home",
	}

	if !strings.Contains(xtn, "^") {
		// Plain value — detect email vs phone
		if strings.Contains(xtn, "@") {
			cp["system"] = "email"
		}
		cp["value"] = xtn
	} else {
		components := strings.Split(xtn, "^")

		// XTN.4 = email address (present for NET / internet equipment type)
		email := ""
		if len(components) > 3 {
			email = strings.TrimSpace(components[3])
		}

		// XTN.1 = telephone number (deprecated in v2.7+ but still common)
		phone := ""
		if len(components) > 0 {
			phone = strings.TrimSpace(components[0])
		}

		// XTN.3 = equipment type (Table 0202): PH, FX, CP, BP, X.400, Internet …
		if len(components) > 2 && components[2] != "" {
			equipType := strings.ToUpper(strings.TrimSpace(components[2]))
			switch equipType {
			case "PH", "CP":
				cp["system"] = "phone"
			case "FX":
				cp["system"] = "fax"
			case "BP":
				cp["system"] = "pager"
			case "X.400", "INTERNET", "NET":
				cp["system"] = "email"
			}
		}

		// XTN.2 = telecom use code (Table 0201): PRN, WPN, ORN, PRS …
		if len(components) > 1 && components[1] != "" {
			useCode := strings.ToUpper(strings.TrimSpace(components[1]))
			switch useCode {
			case "PRN", "VHN":
				cp["use"] = "home"
			case "WPN":
				cp["use"] = "work"
			case "PRS":
				cp["use"] = "mobile"
			case "ORN", "BPN", "ASN", "EMR", "NET":
				cp["use"] = "temp"
			}
		}

		if cp["system"] == "email" && email != "" {
			cp["value"] = email
		} else if phone != "" {
			cp["value"] = phone
		} else if email != "" {
			cp["system"] = "email"
			cp["value"] = email
		}
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			cp["use"] = use
		}
		if sys, ok := rules["system"].(string); ok {
			cp["system"] = sys
		}
	}

	// Drop empty contact points
	if _, hasVal := cp["value"]; !hasVal {
		return nil
	}
	return cp
}

// ──────────────────────────────────────────────────────────────────────────────
// Attachment builders  (HL7 ED and RP)
// ──────────────────────────────────────────────────────────────────────────────

// BuildAttachmentFromED converts an HL7 ED (Encapsulated Data) field to a
// FHIR R4 Attachment with inline base64 data.
//
// ED wire format (HL7 v2 Table 0191 / 2.A.21):
//
//	SourceApplication ^ TypeOfData ^ DataSubtype ^ Encoding ^ Data
//
// FHIR R4 constraint att-1: contentType is required only when data is present.
func BuildAttachmentFromED(ed string) map[string]interface{} {
	parts := strings.Split(ed, "^")
	att := map[string]interface{}{}

	var dataType, dataSubtype, encoding, data string
	if len(parts) >= 2 {
		dataType = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	if len(parts) >= 3 {
		dataSubtype = strings.ToLower(strings.TrimSpace(parts[2]))
	}
	if len(parts) >= 4 {
		encoding = strings.ToUpper(strings.TrimSpace(parts[3]))
	}
	if len(parts) >= 5 {
		data = strings.TrimSpace(strings.Join(parts[4:], "^"))
	}

	if data != "" {
		cleanData := strings.TrimSpace(data)
		if isValidBase64Data(cleanData) {
			att["data"] = cleanData
			att["contentType"] = edToMIMEType(dataType, dataSubtype)
			// title from subtype / type
			if dataSubtype != "" {
				att["title"] = strings.ToUpper(dataSubtype) + " attachment"
			} else if dataType != "" {
				att["title"] = strings.ToUpper(dataType) + " attachment"
			}
			if encoding != "" && encoding != "BASE64" && encoding != "A" {
				title, _ := att["title"].(string)
				att["title"] = title + " [encoding:" + encoding + "]"
			}
		} else {
			// Data present but not valid base64 — omit both data AND contentType.
			// att-1 only requires contentType when data IS present; setting it
			// without data triggers unnecessary BCP-13 fragment validation.
			att["title"] = "(data omitted — not valid base64)"
		}
	}
	return att
}

// BuildAttachmentFromRP converts an HL7 RP (Reference Pointer) field to a
// FHIR R4 Attachment with a URL reference (no inline data).
//
// RP wire format (HL7 v2 Table 2.A.65):
//
//	Pointer ^ ApplicationID ^ TypeOfData ^ DataSubtype
func BuildAttachmentFromRP(rp string) map[string]interface{} {
	parts := strings.Split(rp, "^")
	att := map[string]interface{}{}

	var pointer, dataType, dataSubtype string
	if len(parts) >= 1 {
		pointer = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 3 {
		dataType = strings.ToLower(strings.TrimSpace(parts[2]))
	}
	if len(parts) >= 4 {
		dataSubtype = strings.ToLower(strings.TrimSpace(parts[3]))
	}

	if pointer != "" {
		att["url"] = pointer
	}
	if ct := edToMIMEType(dataType, dataSubtype); ct != "" && ct != "application/octet-stream" {
		att["contentType"] = ct
	}
	if dataSubtype != "" {
		att["title"] = strings.ToUpper(dataSubtype) + " reference"
	} else if dataType != "" {
		att["title"] = strings.ToUpper(dataType) + " reference"
	}
	if pointer == "" {
		att["title"] = rp
	}
	return att
}

// ──────────────────────────────────────────────────────────────────────────────
// Timestamp converter
// ──────────────────────────────────────────────────────────────────────────────

// ToISO converts an HL7 TS (Time Stamp) value to an ISO 8601 string.
//
// Handles YYYYMMDD, YYYYMMDDHHMM, YYYYMMDDHHMMSS, and timezone-suffixed forms.
// Exported from the package so callers outside hl7assembly can use it without
// importing a separate utility.
func ToISO(ts string) string {
	ts = strings.TrimSpace(ts)
	if plus := strings.IndexByte(ts, '+'); plus > 8 {
		ts = ts[:plus]
	} else if minus := strings.LastIndexByte(ts, '-'); minus > 8 {
		ts = ts[:minus]
	}
	if len(ts) < 8 {
		return ts
	}
	year, month, day := ts[0:4], ts[4:6], ts[6:8]
	if len(ts) >= 12 {
		hour, min := ts[8:10], ts[10:12]
		sec := "00"
		if len(ts) >= 14 {
			sec = ts[12:14]
		}
		return year + "-" + month + "-" + day + "T" + hour + ":" + min + ":" + sec + "+00:00"
	}
	return year + "-" + month + "-" + day
}

// ──────────────────────────────────────────────────────────────────────────────
// Status code mappers
// ──────────────────────────────────────────────────────────────────────────────

// OBRStatusToDRStatus maps OBR.25 result status (HL7 Table 0123) to
// FHIR R4 DiagnosticReport.status.
func OBRStatusToDRStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "F":
		return "final"
	case "P":
		return "preliminary"
	case "C":
		return "corrected"
	case "X":
		return "cancelled"
	case "I":
		return "partial"
	case "R":
		return "registered"
	case "S":
		return "amended"
	default:
		return "unknown"
	}
}

// ORCStatusToDRStatus maps ORC.1 order control code (HL7 Table 0119) to a FHIR
// DiagnosticReport.status.  Used as fallback when OBR.25 is empty (e.g. ORM^O01
// order messages that have no results yet).
func ORCStatusToDRStatus(orc1 string) string {
	switch strings.ToUpper(strings.TrimSpace(orc1)) {
	case "NW", "SN", "NA", "XO", "RP", "RO": // New / send / enter
		return "registered"
	case "CA", "OC", "DC": // Cancel / discontinue
		return "cancelled"
	case "HD", "UD": // Hold / unable to fulfill
		return "registered"
	case "RE": // Observations to follow
		return "partial"
	case "":
		return "unknown"
	default:
		return "registered" // conservative: an order exists but no result yet
	}
}

// OBXStatusToObsStatus maps OBX.11 observation result status (HL7 Table 0085)
// to FHIR R4 Observation.status.
func OBXStatusToObsStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "F":
		return "final"
	case "P", "R":
		return "preliminary"
	case "C":
		return "amended"
	case "X":
		return "cancelled"
	case "I":
		return "registered"
	case "W":
		return "entered-in-error"
	default:
		return "unknown"
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Private helpers
// ──────────────────────────────────────────────────────────────────────────────

// codingIsDuplicate returns true if candidate has the same code+system as any
// existing coding in the list.
func codingIsDuplicate(candidate map[string]interface{}, existing []interface{}) bool {
	cCode, _ := candidate["code"].(string)
	cSys, _ := candidate["system"].(string)
	for _, e := range existing {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if em["code"] == cCode && em["system"] == cSys {
			return true
		}
	}
	return false
}

// isValidBase64Data returns true if s is a non-empty, decodable base64 string.
func isValidBase64Data(s string) bool {
	if s == "" {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(s); err == nil {
		return true
	}
	if _, err := base64.URLEncoding.DecodeString(s); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(s)
	return err == nil
}

// edToMIMEType converts HL7 ED TypeOfData + DataSubtype to a MIME type string.
// Follows HL7 v2 Table 0191 (type) and Table 0291 (subtype).
func edToMIMEType(dataType, subType string) string {
	switch subType {
	case "pdf":
		return "application/pdf"
	case "html", "htm":
		return "text/html"
	case "rtf":
		return "application/rtf"
	case "hl7":
		return "application/hl7-v2"
	case "cda", "cda+xml":
		return "application/cda+xml"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "tiff", "tif":
		return "image/tiff"
	case "bmp":
		return "image/bmp"
	case "dicom":
		return "application/dicom"
	case "xml":
		return "application/xml"
	case "json":
		return "application/json"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "mp4":
		return "video/mp4"
	case "mpeg":
		return "video/mpeg"
	case "csv":
		return "text/csv"
	case "zip":
		return "application/zip"
	case "txt", "text":
		return "text/plain"
	}
	switch dataType {
	case "text":
		return "text/plain"
	case "image":
		return "image/jpeg"
	case "audio":
		return "audio/mpeg"
	case "video":
		return "video/mp4"
	case "application":
		return "application/octet-stream"
	case "multipart":
		return "multipart/mixed"
	}
	return "application/octet-stream"
}
