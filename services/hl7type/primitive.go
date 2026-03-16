// services/hl7type/primitive.go
// HL7 primitive data type parsers: NM, SI, BL.
// All functions are lenient and never return errors.
package hl7type

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseNM parses an HL7 NM (Numeric) field.
//
// targetFHIRType controls how the numeric value is represented:
//   - "decimal"      → float64
//   - "integer"      → int64 (truncated)
//   - "positiveInt"  → int64 (must be > 0; clamped to 1 if ≤ 0)
//   - "unsignedInt"  → int64 (must be ≥ 0; clamped to 0 if negative)
//   - ""             → float64 (default)
//
// Returns (value interface{}, warnings []string).
func ParseNM(raw, targetFHIRType string) (interface{}, []string) {
	if raw == "" {
		return nil, nil
	}

	s := strings.TrimSpace(raw)
	var warnings []string

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Try removing trailing/leading whitespace and non-numeric junk
		cleaned := cleanNumeric(s)
		f, err = strconv.ParseFloat(cleaned, 64)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("NM value %q is not a valid number: %v", raw, err))
			return nil, warnings
		}
		warnings = append(warnings, fmt.Sprintf("NM value %q was cleaned to %q for parsing", raw, cleaned))
	}

	switch targetFHIRType {
	case "integer":
		return int64(math.Trunc(f)), warnings
	case "positiveInt":
		v := int64(math.Trunc(f))
		if v <= 0 {
			warnings = append(warnings, fmt.Sprintf("NM positiveInt must be > 0, got %v; clamped to 1", v))
			v = 1
		}
		return v, warnings
	case "unsignedInt":
		v := int64(math.Trunc(f))
		if v < 0 {
			warnings = append(warnings, fmt.Sprintf("NM unsignedInt must be >= 0, got %v; clamped to 0", v))
			v = 0
		}
		return v, warnings
	default: // "decimal" or ""
		return f, warnings
	}
}

// ParseSI parses an HL7 SI (Sequence ID) field.
// Returns (int64, warnings). On parse failure returns (0, warnings).
func ParseSI(raw string) (int64, []string) {
	if raw == "" {
		return 0, nil
	}
	s := strings.TrimSpace(raw)
	var warnings []string

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Try float truncation
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			warnings = append(warnings, fmt.Sprintf("SI value %q is not a valid integer: %v", raw, err))
			return 0, warnings
		}
		n = int64(math.Trunc(f))
		warnings = append(warnings, fmt.Sprintf("SI value %q truncated from float to int %d", raw, n))
	}

	return n, warnings
}

// ParseBL parses an HL7 BL (Boolean) field.
// Accepted true values:  Y, y, true, TRUE, 1
// Accepted false values: N, n, false, FALSE, 0
// Returns (bool, warnings). Unknown values return (false, warning).
func ParseBL(raw string) (bool, []string) {
	if raw == "" {
		return false, nil
	}

	s := strings.TrimSpace(raw)
	var warnings []string

	switch strings.ToUpper(s) {
	case "Y", "YES", "TRUE", "1":
		return true, nil
	case "N", "NO", "FALSE", "0":
		return false, nil
	default:
		warnings = append(warnings, fmt.Sprintf("BL value %q is not a recognized boolean; defaulting to false", raw))
		return false, warnings
	}
}

// ─────────────────────────── helpers ───────────────────────────

// cleanNumeric strips non-numeric characters except leading minus and decimal point,
// allowing lenient parsing of values like " 3.14 " or "42.0 mg".
func cleanNumeric(s string) string {
	var b strings.Builder
	seenDot := false
	seenMinus := false
	for i, r := range s {
		switch {
		case r == '-' && i == 0 && !seenMinus:
			b.WriteRune(r)
			seenMinus = true
		case r == '.' && !seenDot:
			b.WriteRune(r)
			seenDot = true
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
