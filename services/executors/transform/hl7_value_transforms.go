// services/executors/transform/hl7_value_transforms.go
//
// Named value transforms for hl7.build field and component mappings
// ("transform": "hl7_date"). A registry, so adding a transform is one entry
// here, not a new code path in the executor. A transform runs on the value
// read from the message, before valueMap and escaping; a value it can't read
// yields nothing (the field is left empty rather than written malformed).
package transform

import (
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/language"
)

// hl7ValueTransform converts one value; ok=false means "no usable value".
type hl7ValueTransform func(v string) (string, bool)

var partialDate = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?`)

// hl7ValueTransforms is the registry of named transforms.
var hl7ValueTransforms = map[string]hl7ValueTransform{
	// hl7_date: ISO 8601 date, partial date or date-time ("1987", "1987-03",
	// "1987-03-14", "1987-03-14T09:30:00Z") -> HL7 DT ("19870314").
	"hl7_date": func(v string) (string, bool) {
		m := partialDate.FindStringSubmatch(strings.TrimSpace(v))
		if m == nil {
			return "", false
		}
		return m[1] + m[2] + m[3], true
	},
	// hl7_datetime: RFC 3339 date-time -> HL7 DTM with its offset
	// ("20261110111133+0000"); a bare date falls back to hl7_date.
	"hl7_datetime": func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.Format("20060102150405-0700"), true
		}
		m := partialDate.FindStringSubmatch(v)
		if m == nil || len(m[0]) != len(v) {
			return "", false
		}
		return m[1] + m[2] + m[3], true
	},
	// iso3166_alpha3: an ISO 3166-1 alpha-2 country code ("US") -> the
	// alpha-3 code HL7 v2 table 0399 uses ("USA"), from CLDR region data. An
	// alpha-3 code passes through; anything else yields nothing.
	"iso3166_alpha3": func(v string) (string, bool) {
		v = strings.ToUpper(strings.TrimSpace(v))
		if len(v) != 2 && len(v) != 3 {
			return "", false
		}
		r, err := language.ParseRegion(v)
		if err != nil || !r.IsCountry() {
			return "", false
		}
		return r.ISO3(), true
	},
	// iso3166_alpha2: the reverse, for a search: "USA" -> "US"; an alpha-2
	// code passes through.
	"iso3166_alpha2": func(v string) (string, bool) {
		v = strings.ToUpper(strings.TrimSpace(v))
		if len(v) != 2 && len(v) != 3 {
			return "", false
		}
		r, err := language.ParseRegion(v)
		if err != nil || !r.IsCountry() {
			return "", false
		}
		return r.String(), true
	},
	// uppercase: for coded values sent in upper case.
	"uppercase": func(v string) (string, bool) {
		return strings.ToUpper(v), v != ""
	},
	// hl7_universal_id: an identifier system URI -> HD.2 universal ID: an
	// OID or UUID without its "urn:oid:" / "urn:uuid:" prefix; any other URI
	// as it is.
	"hl7_universal_id": func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if id, kind := universalID(v); kind != "URI" {
			return id, id != ""
		}
		return v, v != ""
	},
	// hl7_universal_id_type: an identifier system URI -> HD.3 (table 0301):
	// ISO for an OID, UUID for a UUID, URI for anything else.
	"hl7_universal_id_type": func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if v == "" {
			return "", false
		}
		_, kind := universalID(v)
		return kind, true
	},
	// fhir_date: an HL7 DT or DTM ("19870314", "198703", "1987",
	// "19870314093000-0500") -> the ISO date a FHIR search takes
	// ("1987-03-14", "1987-03", "1987").
	"fhir_date": func(v string) (string, bool) {
		m := hl7Date.FindStringSubmatch(strings.TrimSpace(v))
		if m == nil {
			return "", false
		}
		out := m[1]
		if m[2] != "" {
			out += "-" + m[2]
		}
		if m[3] != "" {
			out += "-" + m[3]
		}
		return out, true
	},
}

var (
	hl7Date  = regexp.MustCompile(`^(\d{4})(\d{2})?(\d{2})?(?:\d|[+-]\d{4}|\.\d+)*$`)
	oidValue = regexp.MustCompile(`^[0-2](\.(0|[1-9]\d*))+$`)
	uuidText = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// universalID splits an identifier system into the HD universal ID and its
// type: "urn:oid:1.2.3" and "1.2.3" are ("1.2.3", ISO); "urn:uuid:<u>" and a
// bare UUID are (<u>, UUID); anything else is (system, URI).
func universalID(system string) (string, string) {
	lower := strings.ToLower(system)
	switch {
	case strings.HasPrefix(lower, "urn:oid:") && oidValue.MatchString(system[8:]):
		return system[8:], "ISO"
	case oidValue.MatchString(system):
		return system, "ISO"
	case strings.HasPrefix(lower, "urn:uuid:") && uuidText.MatchString(system[9:]):
		return system[9:], "UUID"
	case uuidText.MatchString(system):
		return system, "UUID"
	}
	return system, "URI"
}

// systemFromUniversalID is universalID's inverse for a query: an HD's
// universal ID (and type, when given) -> the identifier system URI.
func systemFromUniversalID(id, idType string) string {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return ""
	case strings.Contains(id, ":"):
		return id // already a URI
	case strings.EqualFold(idType, "ISO") || (idType == "" && oidValue.MatchString(id)):
		return "urn:oid:" + id
	case strings.EqualFold(idType, "UUID") || strings.EqualFold(idType, "GUID") || (idType == "" && uuidText.MatchString(id)):
		return "urn:uuid:" + strings.ToLower(id)
	}
	return id
}

// applyHL7Transform runs the named transform; an empty name is a no-op and
// an unknown one leaves the value unchanged (reported by the caller's log).
func applyHL7Transform(name, v string) (string, bool, bool) {
	if name == "" {
		return v, true, true
	}
	t, known := hl7ValueTransforms[name]
	if !known {
		return v, true, false
	}
	out, ok := t(v)
	return out, ok, true
}
