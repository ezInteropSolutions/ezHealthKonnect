// edi/datatypes.go
// Layer 0 of the X12 schema-driven engine: the closed, X12-standard-defined
// set of base data types (AN, ID, DT, TM, R, N0-N9, B). Every element
// (Layer 1), regardless of segment or transaction set, validates/parses/
// formats through this one shared registry — never a per-segment or
// per-transaction-set implementation.
package edi

import (
	"fmt"
	"strconv"
	"strings"
)

// X12DataType identifies one of X12's base element data types.
type X12DataType string

const (
	TypeAN X12DataType = "AN" // Alphanumeric (String)
	TypeID X12DataType = "ID" // Identifier/code — validated against a ValueSet when the element sets one
	TypeDT X12DataType = "DT" // Date, CCYYMMDD
	TypeTM X12DataType = "TM" // Time, HHMM, HHMMSS, or HHMMSSd(d)
	TypeR  X12DataType = "R"  // Decimal (explicit decimal point allowed in the raw value)
	TypeB  X12DataType = "B"  // Binary — passed through unvalidated (length only)
	TypeN0 X12DataType = "N0" // Numeric, 0 implied decimal places
	TypeN1 X12DataType = "N1" // Numeric, 1 implied decimal place
	TypeN2 X12DataType = "N2" // Numeric, 2 implied decimal places
	TypeN3 X12DataType = "N3" // Numeric, 3 implied decimal places
	TypeN4 X12DataType = "N4" // Numeric, 4 implied decimal places
)

// Validate checks raw against t's shape and the element's own length bounds.
// minLen/maxLen of 0 means "not constrained" for that bound.
func Validate(t X12DataType, raw string, minLen, maxLen int) error {
	if minLen > 0 && len(raw) < minLen {
		return fmt.Errorf("value %q shorter than minimum length %d for type %s", raw, minLen, t)
	}
	if maxLen > 0 && len(raw) > maxLen {
		return fmt.Errorf("value %q longer than maximum length %d for type %s", raw, maxLen, t)
	}

	switch {
	case t == TypeAN, t == TypeID, t == TypeB:
		return nil // any string within the length bounds already checked above

	case t == TypeDT:
		if len(raw) != 8 {
			return fmt.Errorf("date %q must be 8 digits (CCYYMMDD)", raw)
		}
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("date %q is not numeric: %w", raw, err)
		}
		return nil

	case t == TypeTM:
		if len(raw) != 4 && len(raw) != 6 && !(len(raw) > 6 && raw[6] == '.') {
			return fmt.Errorf("time %q must be HHMM, HHMMSS, or HHMMSSd(d)", raw)
		}
		digits := raw
		if idx := strings.IndexByte(raw, '.'); idx >= 0 {
			digits = raw[:idx]
		}
		if _, err := strconv.Atoi(digits); err != nil {
			return fmt.Errorf("time %q is not numeric: %w", raw, err)
		}
		return nil

	case t == TypeR:
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return fmt.Errorf("decimal %q is not a valid number: %w", raw, err)
		}
		return nil

	case isNumericType(t):
		digits := raw
		if strings.HasPrefix(digits, "-") {
			digits = digits[1:]
		}
		if digits == "" {
			return fmt.Errorf("numeric %q has no digits for type %s", raw, t)
		}
		if _, err := strconv.Atoi(digits); err != nil {
			return fmt.Errorf("numeric %q is not a valid integer for type %s: %w", raw, t, err)
		}
		return nil

	default:
		return fmt.Errorf("unknown X12 data type %q", t)
	}
}

// Parse converts raw into its semantic Go value: string for AN/ID/B, string
// for DT/TM (kept as the raw CCYYMMDD/HHMMSS form — callers needing a
// time.Time convert explicitly, since X12 dates/times are always used as
// exact strings in segment output), float64 for R and every Nx type
// (implied-decimal numerics are expanded to their real decimal value here).
func Parse(t X12DataType, raw string) (interface{}, error) {
	if err := Validate(t, raw, 0, 0); err != nil {
		return nil, err
	}

	switch {
	case t == TypeAN, t == TypeID, t == TypeB, t == TypeDT, t == TypeTM:
		return raw, nil

	case t == TypeR:
		return strconv.ParseFloat(raw, 64)

	case isNumericType(t):
		places := impliedDecimalPlaces(t)
		negative := strings.HasPrefix(raw, "-")
		digits := raw
		if negative {
			digits = digits[1:]
		}
		asInt, err := strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return nil, err
		}
		value := float64(asInt) / pow10(places)
		if negative {
			value = -value
		}
		return value, nil

	default:
		return nil, fmt.Errorf("unknown X12 data type %q", t)
	}
}

// Format is the inverse of Parse — used by the builder to render a canonical
// Go value back into the exact raw string X12 expects on the wire.
func Format(t X12DataType, v interface{}) (string, error) {
	switch {
	case t == TypeAN, t == TypeID, t == TypeB, t == TypeDT, t == TypeTM:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("expected string for type %s, got %T", t, v)
		}
		return s, nil

	case t == TypeR:
		f, ok := asFloat(v)
		if !ok {
			return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil

	case isNumericType(t):
		f, ok := asFloat(v)
		if !ok {
			return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
		}
		places := impliedDecimalPlaces(t)
		scaled := int64(f*pow10(places) + sign(f)*0.5) // round to nearest
		return strconv.FormatInt(scaled, 10), nil

	default:
		return "", fmt.Errorf("unknown X12 data type %q", t)
	}
}

func isNumericType(t X12DataType) bool {
	if len(t) != 2 || t[0] != 'N' {
		return false
	}
	_, err := strconv.Atoi(string(t[1]))
	return err == nil
}

func impliedDecimalPlaces(t X12DataType) int {
	places, _ := strconv.Atoi(string(t[1]))
	return places
}

func pow10(n int) float64 {
	result := 1.0
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

func asFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
