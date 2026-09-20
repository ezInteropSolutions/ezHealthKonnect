// services/executors/transform/canonical_value_transforms.go
//
// A small, deliberately narrow value-transform registry for
// cda.map_to_canonical field mappings — NOT
// services/cda_fhir.DeclarativeTransformRegistry (see
// map_to_canonical_executor.go's package doc comment for why): every one of
// that registry's ~50 transforms returns a FHIR-resource-shaped value
// (nested CodeableConcept/HumanName/Address maps), incompatible with
// cda.build's flat-string field convention
// (entry_archetypes.go's stringValue/writeFieldValue only ever accept a
// plain string). Every transform here is pure string -> string, so it
// structurally cannot reproduce that silent-drop failure mode.
//
// date_to_cda/datetime_to_cda follow the same "try a short list of known
// time.Parse layouts in order" idiom services/cda_storage/helpers.go's
// parseHL7DateTime already uses for the same class of problem (no existing
// helper in the repo converts an arbitrary source date string INTO CDA's TS
// format — every date helper found during Phase 3's investigation goes the
// other direction, CDA TS -> FHIR).
package transform

import (
	"log"
	"strings"
	"time"
)

// canonicalValueTransformFn converts one resolved field value. ok=false
// means the input didn't match this transform's expected shape (e.g. a date
// string in neither layout it knows) — the caller passes the ORIGINAL value
// through unchanged rather than writing a mangled/empty result.
type canonicalValueTransformFn func(string) (string, bool)

type canonicalValueTransformEntry struct {
	fn          canonicalValueTransformFn
	description string
}

var canonicalValueTransforms = map[string]canonicalValueTransformEntry{
	"trim": {
		fn:          func(v string) (string, bool) { return strings.TrimSpace(v), true },
		description: "Removes leading/trailing whitespace.",
	},
	"uppercase": {
		fn:          func(v string) (string, bool) { return strings.ToUpper(v), true },
		description: "Converts the value to upper case.",
	},
	"lowercase": {
		fn:          func(v string) (string, bool) { return strings.ToLower(v), true },
		description: "Converts the value to lower case.",
	},
	"date_to_cda": {
		fn:          canonicalDateToCDA,
		description: "Converts a date string (YYYY-MM-DD, MM/DD/YYYY, or M/D/YYYY) to CDA's YYYYMMDD format.",
	},
	"datetime_to_cda": {
		fn:          canonicalDateTimeToCDA,
		description: "Converts an ISO 8601/RFC 3339 timestamp to CDA's YYYYMMDDHHMMSS format.",
	},
	"date_to_x12": {
		// Reuses canonicalDateToCDA's own function unchanged -- its output
		// format (CCYYMMDD) is byte-for-byte identical to X12's own DT data
		// type (edi/datatypes.go's TypeDT), despite the function's CDA-era
		// name; a second wrapper function would just be indirection.
		fn:          canonicalDateToCDA,
		description: "Converts a date string (YYYY-MM-DD, MM/DD/YYYY, or M/D/YYYY) to X12's CCYYMMDD format.",
	},
	"time_to_x12": {
		fn:          canonicalTimeToX12,
		description: "Converts a time string (HH:MM or HH:MM:SS) to X12's HHMM format.",
	},
	"datetime_to_ncpdp_date": {
		fn:          canonicalDateTimeToNCPDPDate,
		description: "Extracts the date portion (YYYY-MM-DD) from an ISO 8601/RFC 3339 timestamp, or passes an already-date-only value through unchanged — for NCPDP SCRIPT's own DateWrapper date fields (e.g. WrittenDate), which need FHIR's authoredOn/effectiveDateTime reduced to a bare date.",
	},
}

// canonicalTimeLayouts is tried in order for time_to_x12.
var canonicalTimeLayouts = []string{"15:04:05", "15:04"}

func canonicalTimeToX12(v string) (string, bool) {
	for _, layout := range canonicalTimeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("1504"), true
		}
	}
	return v, false
}

// canonicalDateLayouts is tried in order; "2006-01-02" (ISO) is
// unambiguous with the two US MDY variants below since it always starts
// with a 4-digit year -- no risk of a slash-delimited MDY date being
// misread as ISO or vice versa.
var canonicalDateLayouts = []string{"2006-01-02", "01/02/2006", "1/2/2006"}

func canonicalDateToCDA(v string) (string, bool) {
	for _, layout := range canonicalDateLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("20060102"), true
		}
	}
	return v, false
}

// canonicalZonelessDateTimeLayouts are tried only after RFC3339 (which
// carries its own zone -- the only case UTC normalization applies to); these
// two are formatted as-is, matching CDA TS's own "no zone" form when the
// source data carried no zone info to convert from.
var canonicalZonelessDateTimeLayouts = []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"}

// canonicalDateTimeToNCPDPDate extracts just the date portion from an ISO
// 8601/RFC 3339 timestamp (FHIR's own convention for authoredOn/
// effectiveDateTime). A value that's already a bare "2006-01-02" date (no
// time component) passes through unchanged rather than failing the layout
// match -- a source system may populate either shape.
func canonicalDateTimeToNCPDPDate(v string) (string, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.Format("2006-01-02"), true
	}
	for _, layout := range canonicalZonelessDateTimeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.Format("2006-01-02"), true
	}
	return v, false
}

func canonicalDateTimeToCDA(v string) (string, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format("20060102150405"), true
	}
	for _, layout := range canonicalZonelessDateTimeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("20060102150405"), true
		}
	}
	return v, false
}

// applyCanonicalTransform dispatches to the named transform. An empty name
// is passthrough (matches DeclarativeTransformRegistry.Apply's own
// convention). An UNKNOWN name logs a warning and passes the raw value
// through unchanged rather than failing the whole step -- config can drift
// (a transform renamed/removed later); degrading gracefully here matches
// this CDA builder work's existing sanitizeXPath-style philosophy
// (services/parsers/cda/generic_section_processor.go) of never crashing on
// a bad/stale config value. A value the transform's own layout list doesn't
// match (ok=false) degrades the same way -- passthrough, not an error.
func applyCanonicalTransform(name, value string) string {
	if name == "" || value == "" {
		return value
	}
	entry, ok := canonicalValueTransforms[name]
	if !ok {
		log.Printf("⚠️  [map_to_canonical] unknown transform %q — passing value through unchanged", name)
		return value
	}
	result, ok := entry.fn(value)
	if !ok {
		return value
	}
	return result
}

// CanonicalTransformDescriptions exports name -> description for the
// cda.map_to_canonical mapping UI's Transform picker
// (controllers/cda_schema_controller.go's GetCanonicalTransforms), mirroring
// services/cda_fhir.DeclarativeTransformRegistry.AllDescriptions() exactly.
// The registry itself is shared with edi.map_to_canonical too (see
// date_to_x12/time_to_x12 above) — this exported function stays CDA-endpoint-
// named only because that's its one real caller today.
func CanonicalTransformDescriptions() map[string]string {
	out := make(map[string]string, len(canonicalValueTransforms))
	for name, entry := range canonicalValueTransforms {
		out[name] = entry.description
	}
	return out
}
