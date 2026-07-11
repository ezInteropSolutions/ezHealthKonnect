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
		log.Printf("⚠️  [cda.map_to_canonical] unknown transform %q — passing value through unchanged", name)
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
func CanonicalTransformDescriptions() map[string]string {
	out := make(map[string]string, len(canonicalValueTransforms))
	for name, entry := range canonicalValueTransforms {
		out[name] = entry.description
	}
	return out
}
