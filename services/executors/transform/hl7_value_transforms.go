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
	// uppercase: for coded values sent in upper case.
	"uppercase": func(v string) (string, bool) {
		return strings.ToUpper(v), v != ""
	},
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
