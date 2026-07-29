// cda/builder/time.go
package builder

import (
	"regexp"
	"strconv"
	"time"
)

var offsetPattern = regexp.MustCompile(`^([+-])(\d{2})(\d{2})$`)

// parseTimezoneOffset turns a user-supplied "+HHMM"/"-HHMM" offset (e.g.
// "-0500", "+0530") into a fixed time.Location. Empty or malformed input
// falls back to UTC — this is best-effort formatting for a document
// timestamp, not a validation gate, so a bad config value should never
// break document generation.
func parseTimezoneOffset(offset string) *time.Location {
	m := offsetPattern.FindStringSubmatch(offset)
	if m == nil {
		return time.UTC
	}
	hours, _ := strconv.Atoi(m[2])
	minutes, _ := strconv.Atoi(m[3])
	totalSeconds := hours*3600 + minutes*60
	if m[1] == "-" {
		totalSeconds = -totalSeconds
	}
	return time.FixedZone(offset, totalSeconds)
}

// nowCDATimestamp returns the current time in CDA's TS format
// (YYYYMMDDHHMMSS+ZZZZ), expressed in the given "+HHMM"/"-HHMM" offset —
// empty or malformed offset defaults to UTC ("+0000"). Always includes the
// numeric offset suffix per CONF:10130 ("if more precise than day, SHOULD
// include time-zone offset").
func nowCDATimestamp(offset string) string {
	return time.Now().In(parseTimezoneOffset(offset)).Format("20060102150405-0700")
}

func timeNowUnixNano() int64 {
	return time.Now().UnixNano()
}
