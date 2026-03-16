// services/hl7type/timestamp.go
// HL7 timestamp/date/time type parsers.
// All functions are lenient: they never return errors; malformed input produces
// the best partial result possible plus one or more warning strings.
package hl7type

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseTS parses an HL7 TS or DTM field value into a FHIR temporal string.
//
// HL7 TS/DTM format: YYYY[MM[DD[HH[MM[SS[.S[S[S[S]]]]]]]]][+/-ZZZZ]
//
// targetFHIRType must be one of: "dateTime", "date", "instant", "time"
// Returns (fhirValue, warnings).
func ParseTS(raw, targetFHIRType string) (string, []string) {
	if raw == "" {
		return "", nil
	}

	raw = strings.TrimSpace(raw)

	// Pass-through for already-ISO formats
	if looksISO(raw) {
		return normalizeISO(raw, targetFHIRType), nil
	}

	// Split off timezone suffix (+/-HHMM)
	digits, tz, tzWarnings := splitTZ(raw)

	var warnings []string
	warnings = append(warnings, tzWarnings...)

	// Trim fractional seconds suffix (.SSSS) if present
	fracPart := ""
	dotIdx := strings.Index(digits, ".")
	if dotIdx >= 0 {
		fracPart = digits[dotIdx+1:]
		digits = digits[:dotIdx]
	}
	dLen := len(digits)

	// Validate we at least have a 4-digit year
	if dLen < 4 {
		warnings = append(warnings, fmt.Sprintf("TS value too short to extract year: %q", raw))
		return raw, warnings
	}

	year := digits[0:4]

	if dLen == 4 {
		return year, warnings
	}

	if dLen < 6 {
		warnings = append(warnings, fmt.Sprintf("TS value has partial month: %q", raw))
		return year, warnings
	}
	month := digits[4:6]

	if dLen == 6 {
		datePart := fmt.Sprintf("%s-%s", year, month)
		if targetFHIRType == "date" {
			return datePart, warnings
		}
		return datePart, warnings
	}

	if dLen < 8 {
		warnings = append(warnings, fmt.Sprintf("TS value has partial day: %q", raw))
		return fmt.Sprintf("%s-%s", year, month), warnings
	}
	day := digits[6:8]
	datePart := fmt.Sprintf("%s-%s-%s", year, month, day)

	if dLen == 8 {
		if targetFHIRType == "date" {
			return datePart, warnings
		}
		return datePart, warnings
	}

	// Need time component
	if dLen < 10 {
		warnings = append(warnings, fmt.Sprintf("TS value has partial hour: %q", raw))
		return datePart, warnings
	}
	hour := digits[8:10]

	minute := "00"
	if dLen >= 12 {
		minute = digits[10:12]
	}

	sec := "00"
	if dLen >= 14 {
		sec = digits[12:14]
	}

	// Build fractional seconds for dateTime/instant
	fracStr := ""
	if fracPart != "" {
		// FHIR allows up to millisecond precision (.SSS)
		if len(fracPart) > 3 {
			fracPart = fracPart[:3]
		}
		fracStr = "." + fracPart
	}

	tzStr := buildTZStr(tz)

	switch targetFHIRType {
	case "date":
		return datePart, warnings
	case "time":
		return fmt.Sprintf("%s:%s:%s%s", hour, minute, sec, fracStr), warnings
	default: // dateTime, instant
		return fmt.Sprintf("%sT%s:%s:%s%s%s", datePart, hour, minute, sec, fracStr, tzStr), warnings
	}
}

// ParseDT parses an HL7 DT (date-only) field: YYYY[MM[DD]]
func ParseDT(raw string) (string, []string) {
	if raw == "" {
		return "", nil
	}
	raw = strings.TrimSpace(raw)

	var warnings []string

	// Remove any timezone (shouldn't be present in DT but be lenient)
	digits, _, tzW := splitTZ(raw)
	warnings = append(warnings, tzW...)

	// Remove fractional
	if dotIdx := strings.Index(digits, "."); dotIdx >= 0 {
		digits = digits[:dotIdx]
	}

	dLen := len(digits)
	if dLen < 4 {
		warnings = append(warnings, fmt.Sprintf("DT value too short: %q", raw))
		return raw, warnings
	}

	year := digits[0:4]
	if dLen == 4 {
		return year, warnings
	}
	if dLen < 6 {
		return year, append(warnings, fmt.Sprintf("DT partial month: %q", raw))
	}
	month := digits[4:6]
	if dLen == 6 {
		return fmt.Sprintf("%s-%s", year, month), warnings
	}
	if dLen < 8 {
		return fmt.Sprintf("%s-%s", year, month), append(warnings, fmt.Sprintf("DT partial day: %q", raw))
	}
	day := digits[6:8]
	return fmt.Sprintf("%s-%s-%s", year, month, day), warnings
}

// ParseTM parses an HL7 TM (time) field: HH[MM[SS[.S[S[S[S]]]]]][+/-ZZZZ]
// Returns a FHIR time string (HH:mm:ss or HH:mm:ss.SSS or with offset).
func ParseTM(raw string) (string, []string) {
	if raw == "" {
		return "", nil
	}
	raw = strings.TrimSpace(raw)

	var warnings []string

	digits, tz, tzW := splitTZ(raw)
	warnings = append(warnings, tzW...)

	fracPart := ""
	if dotIdx := strings.Index(digits, "."); dotIdx >= 0 {
		fracPart = digits[dotIdx+1:]
		digits = digits[:dotIdx]
	}

	dLen := len(digits)
	if dLen < 2 {
		warnings = append(warnings, fmt.Sprintf("TM value too short for hour: %q", raw))
		return raw, warnings
	}

	hour := digits[0:2]

	minute := "00"
	if dLen >= 4 {
		minute = digits[2:4]
	}

	sec := "00"
	if dLen >= 6 {
		sec = digits[4:6]
	}

	fracStr := ""
	if fracPart != "" {
		if len(fracPart) > 3 {
			fracPart = fracPart[:3]
		}
		fracStr = "." + fracPart
	}

	tzStr := buildTZStr(tz)

	return fmt.Sprintf("%s:%s:%s%s%s", hour, minute, sec, fracStr, tzStr), warnings
}

// ─────────────────────────── helpers ───────────────────────────

// splitTZ splits an HL7 TS value into the digit/decimal body and the timezone
// suffix (+HHMM or -HHMM).  Returns (body, tzSuffix, warnings).
func splitTZ(raw string) (body, tz string, warnings []string) {
	// Find last + or - that looks like a timezone (must be followed by exactly 4 digits)
	for i := len(raw) - 1; i >= 1; i-- {
		ch := raw[i]
		if ch == '+' || ch == '-' {
			tail := raw[i+1:]
			if len(tail) == 4 && isAllDigits(tail) {
				return raw[:i], string(ch) + tail, nil
			}
		}
	}
	return raw, "", nil
}

// buildTZStr converts a raw HL7 tz suffix (+HHMM) to FHIR format (+HH:mm).
// If tz is empty, returns "" (no timezone appended).
func buildTZStr(tz string) string {
	if tz == "" {
		return ""
	}
	if len(tz) == 5 { // ±HHMM
		sign := string(tz[0])
		hh := tz[1:3]
		mm := tz[3:5]
		return fmt.Sprintf("%s%s:%s", sign, hh, mm)
	}
	return tz // pass-through unexpected formats
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// looksISO returns true if the value appears to already be in ISO 8601 format
// (contains a '-' in the date portion or a 'T' separator).
func looksISO(s string) bool {
	if len(s) < 4 {
		return false
	}
	// Year-month dash: YYYY-
	if len(s) >= 5 && s[4] == '-' {
		return true
	}
	// Contains T for datetime separator
	if strings.ContainsRune(s, 'T') {
		return true
	}
	return false
}

// normalizeISO performs light normalization on already-ISO strings.
func normalizeISO(s, targetFHIRType string) string {
	switch targetFHIRType {
	case "date":
		// Return just the date portion
		if idx := strings.IndexRune(s, 'T'); idx >= 0 {
			return s[:idx]
		}
	}
	return s
}
