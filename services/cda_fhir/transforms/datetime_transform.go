package transforms

import (
	"fmt"
	"strings"

	cdadocument "ezhealthkonnect/cda/document"
)

// CDATimeToFHIRDateTime converts a CDATime (HL7 TS) to an ISO 8601 dateTime string.
// Returns "" for null-flavored or empty values.
func CDATimeToFHIRDateTime(t cdadocument.CDATime) string {
	if t.NullFlavor != "" || t.Value == "" {
		return ""
	}
	return formatCDATimestamp(t.Value)
}

// CDATimeToFHIRDate converts a CDATime to a FHIR date string (YYYY-MM-DD only).
func CDATimeToFHIRDate(t cdadocument.CDATime) string {
	if t.NullFlavor != "" || t.Value == "" {
		return ""
	}
	return formatCDADate(t.Value)
}

// CDATimeRangeToPeriod converts a CDATimeRange (IVL<TS>) to a FHIR Period map.
// Returns nil when the range is null-flavored or contains no usable timestamps.
func CDATimeRangeToPeriod(tr cdadocument.CDATimeRange) map[string]interface{} {
	if tr.NullFlavor != "" {
		return nil
	}
	start := CDATimeToFHIRDateTime(tr.Low)
	end := CDATimeToFHIRDateTime(tr.High)
	single := CDATimeToFHIRDateTime(tr.Value)

	if start == "" && end == "" && single == "" {
		return nil
	}
	period := map[string]interface{}{}
	if start != "" {
		period["start"] = start
	} else if single != "" {
		period["start"] = single
	}
	if end != "" {
		period["end"] = end
	}
	return period
}

// CDATimeRangeToOnset picks the best available onset datetime from a CDATimeRange.
// Prefers low.value, then single value, then high.value.
func CDATimeRangeToOnset(tr cdadocument.CDATimeRange) string {
	if s := CDATimeToFHIRDateTime(tr.Low); s != "" {
		return s
	}
	if s := CDATimeToFHIRDateTime(tr.Value); s != "" {
		return s
	}
	return CDATimeToFHIRDateTime(tr.High)
}

// ============================================================
// private helpers
// ============================================================

func formatCDATimestamp(ts string) string {
	s := strings.TrimSpace(ts)
	if s == "" {
		return ""
	}
	// Preserve and strip timezone suffix
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
		dt := fmt.Sprintf("%s-%s-%sT%s:%s:%s",
			s[0:4], s[4:6], s[6:8], s[8:10], s[10:12], s[12:14])
		if tz != "" {
			dt += normalizeTZ(tz)
		}
		return dt
	case len(s) >= 8:
		return fmt.Sprintf("%s-%s-%s", s[0:4], s[4:6], s[6:8])
	case len(s) >= 6:
		return fmt.Sprintf("%s-%s", s[0:4], s[4:6])
	case len(s) == 4:
		return s
	default:
		return ""
	}
}

func formatCDADate(ts string) string {
	s := strings.TrimSpace(ts)
	for _, sep := range []string{"+", "-"} {
		if idx := strings.LastIndex(s, sep); idx >= 8 {
			s = s[:idx]
			break
		}
	}
	if len(s) >= 8 {
		return fmt.Sprintf("%s-%s-%s", s[0:4], s[4:6], s[6:8])
	}
	if len(s) >= 6 {
		return fmt.Sprintf("%s-%s", s[0:4], s[4:6])
	}
	if len(s) == 4 {
		return s
	}
	return ""
}

func normalizeTZ(tz string) string {
	if len(tz) == 5 {
		return tz[:3] + ":" + tz[3:]
	}
	return tz
}
