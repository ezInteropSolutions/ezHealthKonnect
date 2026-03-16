// services/hl7type/identifier.go
// Assigning Authority → FHIR system URI resolution.
//
// HL7 v2 uses Hierarchical Designators (HD) as assigning authorities:
//   PID.3.4  = "EPIC"                     (namespace ID only)
//   PID.3.4  = "EPIC^Epic^ISO"            (namespace + OID + type)
//   PID.3.4  = "2.16.840.1.113883.3.1"    (OID directly)
//   PID.3.4  = "https://epic.com/mrn"     (URL directly)
//
// FHIR identifier.system must be an absolute URI.
package hl7type

import (
	"strings"
)

// knownOIDs maps well-known OID strings to their canonical FHIR system URIs.
// An OID lookup is tried before the generic urn:oid: wrapping so that
// standard terminology OIDs resolve to human-readable http:// URIs.
// Sources: HL7 OID registry (www.hl7.org/oid), FHIR naming systems.
var knownOIDs = map[string]string{
	// ─── US national identifiers ─────────────────────────────────────────────
	"2.16.840.1.113883.4.6":   "http://hl7.org/fhir/sid/us-npi",
	"2.16.840.1.113883.4.1":   "http://hl7.org/fhir/sid/us-ssn",
	"2.16.840.1.113883.4.7":   "http://hl7.org/fhir/sid/us-medicare-beneficiary",
	"2.16.840.1.113883.4.572": "http://hl7.org/fhir/sid/us-medicaid",
	"2.16.840.1.113883.4.9":   "http://hl7.org/fhir/sid/us-dea",

	// ─── Standard terminology systems ────────────────────────────────────────
	"2.16.840.1.113883.6.1":   "http://loinc.org",
	"2.16.840.1.113883.6.96":  "http://snomed.info/sct",
	"2.16.840.1.113883.6.88":  "http://www.nlm.nih.gov/research/umls/rxnorm",
	"2.16.840.1.113883.6.69":  "http://hl7.org/fhir/sid/ndc",
	"2.16.840.1.113883.12.292": "http://hl7.org/fhir/sid/cvx",
	"2.16.840.1.113883.6.8":   "http://unitsofmeasure.org",
	"2.16.840.1.113883.6.285": "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets",
	"2.16.840.1.113883.6.12":  "http://www.ama-assn.org/go/cpt",
	"2.16.840.1.113883.6.90":  "http://hl7.org/fhir/sid/icd-10-cm",
	"2.16.840.1.113883.6.4":   "http://www.cms.gov/Medicare/Coding/ICD10",
	"2.16.840.1.113883.6.103": "http://hl7.org/fhir/sid/icd-9-cm",

	// ─── EHR vendor identifier roots ─────────────────────────────────────────
	"1.2.840.114350":              "urn:oid:1.2.840.114350",          // Epic root
	"2.16.840.1.113883.3.1974":    "urn:oid:2.16.840.1.113883.3.1974", // Cerner
	"2.16.840.1.113883.3.157":     "urn:oid:2.16.840.1.113883.3.157",  // Meditech
}

// oidPrefixes maps OID tree roots to a descriptive FHIR system URI prefix.
// Used for prefix matching when an OID is a sub-node of a known vendor tree.
// The matched prefix is returned as-is (urn:oid:) since sub-nodes are instance-specific.
var oidPrefixes = []struct {
	prefix  string
	comment string
}{
	{"1.2.840.114350.", "Epic sub-OID"},
	{"2.16.840.1.113883.3.1974.", "Cerner sub-OID"},
	{"2.16.840.1.113883.3.157.", "Meditech sub-OID"},
	{"2.16.840.1.113883.3.4622.", "eClinicalWorks sub-OID"},
	{"2.16.840.1.113883.3.1618.", "Allscripts sub-OID"},
	{"2.16.840.1.113883.3.4828.", "NextGen sub-OID"},
	{"2.16.840.1.113883.5.", "HL7 v3 vocabulary"},
}

// resolveOID attempts to find a canonical FHIR system URI for a known OID.
// Returns the resolved URI, or "" if the OID is not in any known registry.
// Caller should fall back to "urn:oid:{oid}" for unrecognized OIDs.
func resolveOID(oid string) string {
	// Exact match first — handles standard terminology OIDs
	if uri, ok := knownOIDs[oid]; ok {
		return uri
	}
	// Prefix match — handles vendor sub-OIDs (instance-specific, stay as urn:oid:)
	for _, p := range oidPrefixes {
		if strings.HasPrefix(oid, p.prefix) {
			return "urn:oid:" + oid
		}
	}
	return ""
}

// knownSystems maps common HL7 assigning authority names to their standard FHIR system URIs.
// Sources: HL7 OID registry, standard FHIR naming systems, vendor documentation.
var knownSystems = map[string]string{
	// ─── US National identifiers ─────────────────────────────────────────────
	"NPI":    "http://hl7.org/fhir/sid/us-npi",
	"SS":     "http://hl7.org/fhir/sid/us-ssn",
	"SSN":    "http://hl7.org/fhir/sid/us-ssn",
	"UPIN":   "http://hl7.org/fhir/sid/us-upin",
	"DEA":    "http://hl7.org/fhir/sid/us-dea",
	"NDC":    "http://hl7.org/fhir/sid/ndc",
	"CVX":    "http://hl7.org/fhir/sid/cvx",
	"MVX":    "http://hl7.org/fhir/sid/mvx",
	"HCPCS":  "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets",
	"CPT4":   "http://www.ama-assn.org/go/cpt",
	"CPT":    "http://www.ama-assn.org/go/cpt",

	// ─── Diagnosis / problem coding ──────────────────────────────────────────
	"ICD10CM": "http://hl7.org/fhir/sid/icd-10-cm",
	"ICD10PCS": "http://www.cms.gov/Medicare/Coding/ICD10",
	"ICD9CM":  "http://hl7.org/fhir/sid/icd-9-cm",
	"ICD9":    "http://hl7.org/fhir/sid/icd-9-cm",
	"SCT":     "http://snomed.info/sct",
	"SNOMED":  "http://snomed.info/sct",
	"LN":      "http://loinc.org",
	"LOINC":   "http://loinc.org",
	"RXNORM":  "http://www.nlm.nih.gov/research/umls/rxnorm",
	"UCUM":    "http://unitsofmeasure.org",

	// ─── Major EHR vendors (OID roots) ───────────────────────────────────────
	"EPIC":        "urn:oid:1.2.840.114350",
	"CERNER":      "urn:oid:2.16.840.1.113883.3.1974",
	"MEDITECH":    "urn:oid:2.16.840.1.113883.3.157",
	"ALLSCRIPTS":  "urn:oid:2.16.840.1.113883.3.1618",
	"ECLINICALWORKS": "urn:oid:2.16.840.1.113883.3.4622",
	"MCKESSON":    "urn:oid:2.16.840.1.113883.3.19866",
	"NEXTGEN":     "urn:oid:2.16.840.1.113883.3.4828",
	"ATHENA":      "urn:oid:2.16.840.1.113883.3.25338",
	"DRFIRST":     "urn:oid:2.16.840.1.113883.3.2899",
	"SURESCRIPTS": "urn:oid:2.16.840.1.113883.3.2999",

	// ─── HL7 / terminology infrastructure ────────────────────────────────────
	"HL7":  "http://terminology.hl7.org",
	"ISO":  "urn:ietf:rfc:3986",
	"UUID": "urn:ietf:rfc:4122",
}

// ResolveSystemURI converts any identifier system string to an absolute FHIR system URI.
//
// This is format-agnostic — the input may come from HL7, FHIR JSON, CSV, database,
// or any other source. It does not understand HL7 HD composite syntax (^-delimited).
// For HL7 HD composites, call MapAssigningAuthorityToURI which parses the composite
// first and then delegates here.
//
// Resolution order:
//  1. Already a URI (http/https/urn/ftp) → use as-is.
//  2. Looks like an OID → look up in knownOIDs; fall back to "urn:oid:{oid}".
//  3. Named registry lookup (case-insensitive) → canonical FHIR system URI.
//  4. Fallback → "urn:id:{lowercase-name}" — stable, always a valid absolute URI.
//
// For facility-specific names/OIDs not in any static registry, the caller should
// consult a per-interface OID registry table (future DB feature) before calling here.
func ResolveSystemURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// 1. Already a URI — use as-is regardless of source format
	if looksLikeURL(raw) {
		return raw
	}

	// 2. OID — resolve to canonical URI or wrap in urn:oid:
	if looksLikeOID(raw) {
		if resolved := resolveOID(raw); resolved != "" {
			return resolved
		}
		return "urn:oid:" + raw
	}

	// 3. Named system lookup
	if uri, ok := knownSystems[strings.ToUpper(raw)]; ok && uri != "" {
		return uri
	}

	// 4. Stable URN fallback — valid URI, consistent per source name
	return "urn:id:" + strings.ToLower(raw)
}

// MapAssigningAuthorityToURI converts an HL7 Hierarchical Designator (HD) value
// to an absolute FHIR system URI. This is HL7-specific: it first parses the
// HD composite format "namespaceID^universalID^universalIDType", then delegates
// the actual URI resolution to ResolveSystemURI.
//
// Call ResolveSystemURI directly for non-HL7 inputs (FHIR JSON, CSV, database).
func MapAssigningAuthorityToURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// ── HL7 HD composite parsing: "namespaceID^universalID^universalIDType" ───
	// Only applies when the input actually contains "^" (HL7 HD format).
	if strings.Contains(raw, "^") {
		parts := strings.SplitN(raw, "^", 3)
		namespaceID := strings.TrimSpace(parts[0])
		universalID := strings.TrimSpace(parts[1])
		universalIDType := ""
		if len(parts) == 3 {
			universalIDType = strings.TrimSpace(parts[2])
		}

		// Explicit universal ID type tells us exactly how to interpret universalID
		switch strings.ToUpper(universalIDType) {
		case "ISO", "HL7":
			if universalID != "" {
				return ResolveSystemURI(universalID) // OID in universalID slot
			}
		case "UUID":
			if universalID != "" {
				return "urn:uuid:" + strings.ToLower(universalID)
			}
		case "URI", "URL":
			if universalID != "" {
				return ResolveSystemURI(universalID)
			}
		}

		// No explicit type — try to resolve universalID by its content, then fall
		// back to the namespaceID (which may be a name like "EPIC")
		if universalID != "" {
			if looksLikeOID(universalID) || looksLikeURL(universalID) {
				return ResolveSystemURI(universalID)
			}
		}

		// universalID is not usable — resolve by namespace name
		if namespaceID != "" {
			return ResolveSystemURI(namespaceID)
		}
		return ""
	}

	// ── Not HL7 HD composite — treat as plain name/OID/URI ───────────────────
	return ResolveSystemURI(raw)
}

// looksLikeOID returns true for valid ISO OID strings (e.g. "2.16.840.1.113883").
//
// OID rules (ITU-T X.660):
//   - Only digits and dots
//   - Root arc must be 0, 1, or 2 (single character)
//   - At least 3 arcs (root.authority.specific)
//   - No arc may have a leading zero (except the arc "0" itself)
//   - Rejects IPv4 addresses: their second arc is always < 256, but OID second
//     arcs under root 2 go up to 999; we exclude the [0-9]{1,3}.[0-9]{1,3} pattern
//     by requiring the string contains a segment with value > 255 OR has >= 5 arcs
//     with root in {0,1,2}.  A simpler heuristic: reject if all arcs are <= 255
//     and there are exactly 4 arcs (classic IPv4 pattern).
func looksLikeOID(s string) bool {
	if !strings.Contains(s, ".") {
		return false
	}
	// Must contain only digits and dots
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	parts := strings.Split(s, ".")
	// Root arc must be exactly "0", "1", or "2"
	if len(parts) < 3 || (parts[0] != "0" && parts[0] != "1" && parts[0] != "2") {
		return false
	}
	// No arc may have a leading zero (except "0" itself)
	for _, p := range parts {
		if len(p) == 0 {
			return false // empty arc (trailing/double dot)
		}
		if len(p) > 1 && p[0] == '0' {
			return false
		}
	}
	// Reject IPv4 addresses: exactly 4 arcs all <= 255
	if len(parts) == 4 {
		allIPLike := true
		for _, p := range parts {
			v := 0
			for _, r := range p {
				v = v*10 + int(r-'0')
			}
			if v > 255 {
				allIPLike = false
				break
			}
		}
		if allIPLike {
			return false
		}
	}
	return true
}

// looksLikeURL returns true for http/https/urn/ftp prefixed strings.
func looksLikeURL(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http://") ||
		strings.HasPrefix(low, "https://") ||
		strings.HasPrefix(low, "urn:") ||
		strings.HasPrefix(low, "ftp://")
}
