// ncpdptelecom/datatypes.go
// Layer 0 of the NCPDP Telecommunication D.0 schema-driven engine: the
// closed set of D.0-standard-defined base field data types. Every field
// (Layer 1), regardless of segment or transaction, validates/parses/formats
// through this one shared registry — mirroring edi/datatypes.go's own role
// for the X12 engine, never a per-segment or per-transaction implementation.
//
// D.0's own data types are simpler than X12's in every way except one: it
// has no separate composite/repeat-count numeric family (X12's N0-N4), but
// it introduces "signed overpunch" — a genuinely novel-to-this-codebase wire
// encoding where a decimal's sign AND its final digit are both packed into
// one trailing letter, a mainframe-era convention (zone punch) that persists
// in the D.0 standard today. See EncodeOverpunch/DecodeOverpunch below.
package ncpdptelecom

import (
	"fmt"
	"strconv"
	"strings"
)

// TelecomDataType identifies one of D.0's base field data types.
type TelecomDataType string

const (
	TypeString    TelecomDataType = "AN" // Alphanumeric (plain text, no special encoding)
	TypeDate      TelecomDataType = "DT" // Date, CCYYMMDD, no separators
	TypeInteger   TelecomDataType = "N"  // Integer, left-padded with zeros to the field's fixed length
	TypeDecimal   TelecomDataType = "R"  // Decimal, no decimal point — precision is implied by the field spec
	TypeOverpunch TelecomDataType = "RO" // Signed-overpunch decimal — see EncodeOverpunch/DecodeOverpunch
)

// overpunchPositive/overpunchNegative map a decimal digit (0-9, by index) to
// its overpunch letter, confirmed directly from the NCPDP D.0 wire-format
// reference (eduardonunesp/ncpdp-telecom-fmt-book's own data-types.md,
// itself sourced from datainsight.health) — this is the exact table real
// D.0 payer/pharmacy systems use, not a guessed encoding.
var overpunchPositive = [10]byte{'{', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I'}
var overpunchNegative = [10]byte{'}', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R'}

// overpunchDecode maps every valid overpunch letter back to (digit, negative).
var overpunchDecode = buildOverpunchDecodeTable()

func buildOverpunchDecodeTable() map[byte]struct {
	digit    int
	negative bool
} {
	table := make(map[byte]struct {
		digit    int
		negative bool
	}, 20)
	for digit, letter := range overpunchPositive {
		table[letter] = struct {
			digit    int
			negative bool
		}{digit, false}
	}
	for digit, letter := range overpunchNegative {
		table[letter] = struct {
			digit    int
			negative bool
		}{digit, true}
	}
	return table
}

// DecodeOverpunch converts a raw signed-overpunch field (e.g. "0000084F")
// into its real decimal value, given the field's own implied decimal-place
// count (e.g. 2 for a cents-precision money field). Mirrors the worked
// examples in the source documentation exactly: "0000084F" (places=2) -> 8.46,
// "0000084J" (places=2) -> -8.41, "0000000000{" (places=2) -> 0.00.
func DecodeOverpunch(raw string, places int) (float64, error) {
	if raw == "" {
		return 0, fmt.Errorf("ncpdptelecom: empty overpunch value")
	}
	last := raw[len(raw)-1]
	decoded, ok := overpunchDecode[last]
	if !ok {
		return 0, fmt.Errorf("ncpdptelecom: %q is not a valid overpunch letter (in value %q)", string(last), raw)
	}
	digits := raw[:len(raw)-1] + strconv.Itoa(decoded.digit)
	asInt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ncpdptelecom: overpunch value %q has non-numeric digits: %w", raw, err)
	}
	value := float64(asInt) / pow10(places)
	if decoded.negative {
		value = -value
	}
	return value, nil
}

// EncodeOverpunch is the inverse of DecodeOverpunch — used by the builder to
// render a canonical float64 back into the exact raw overpunch string D.0
// expects on the wire. width is the total field length (including the
// trailing overpunch letter) the raw value is zero-padded to.
func EncodeOverpunch(value float64, places, width int) (string, error) {
	negative := value < 0
	if negative {
		value = -value
	}
	scaled := int64(value*pow10(places) + 0.5) // round to nearest
	digits := strconv.FormatInt(scaled, 10)
	if len(digits) == 0 {
		digits = "0"
	}
	lastDigit := int(digits[len(digits)-1] - '0')
	var letter byte
	if negative {
		letter = overpunchNegative[lastDigit]
	} else {
		letter = overpunchPositive[lastDigit]
	}
	body := digits[:len(digits)-1]
	padded := zeroPad(body, width-1) // the letter occupies the field's final position
	raw := padded + string(letter)
	if len(raw) > width {
		return "", fmt.Errorf("ncpdptelecom: encoded overpunch value %q exceeds field width %d", raw, width)
	}
	return raw, nil
}

// Validate checks raw against t's shape. width of 0 means "not
// length-constrained" (used for variable-length AN fields).
func Validate(t TelecomDataType, raw string, width int) error {
	if width > 0 && len(raw) > width {
		return fmt.Errorf("value %q longer than field width %d for type %s", raw, width, t)
	}

	switch t {
	case TypeString:
		return nil

	case TypeDate:
		if len(raw) != 8 {
			return fmt.Errorf("date %q must be 8 digits (CCYYMMDD)", raw)
		}
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("date %q is not numeric: %w", raw, err)
		}
		return nil

	case TypeInteger:
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("integer %q is not numeric: %w", raw, err)
		}
		return nil

	case TypeDecimal:
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("decimal %q must be all digits (no decimal point on the wire): %w", raw, err)
		}
		return nil

	case TypeOverpunch:
		if len(raw) == 0 {
			return fmt.Errorf("overpunch value is empty")
		}
		if _, ok := overpunchDecode[raw[len(raw)-1]]; !ok {
			return fmt.Errorf("overpunch value %q has an invalid trailing letter %q", raw, string(raw[len(raw)-1]))
		}
		digits := raw[:len(raw)-1]
		if digits != "" {
			if _, err := strconv.Atoi(digits); err != nil {
				return fmt.Errorf("overpunch value %q has non-numeric leading digits: %w", raw, err)
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown D.0 data type %q", t)
	}
}

// Parse converts raw into its semantic Go value: string for AN/DT, float64
// for N/R/RO (implied-decimal and overpunch values are expanded to their
// real numeric value here). places only matters for R/RO.
func Parse(t TelecomDataType, raw string, places int) (interface{}, error) {
	if err := Validate(t, raw, 0); err != nil {
		return nil, err
	}

	switch t {
	case TypeString, TypeDate:
		return raw, nil

	case TypeInteger:
		asInt, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, err
		}
		return float64(asInt), nil

	case TypeDecimal:
		asInt, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, err
		}
		return float64(asInt) / pow10(places), nil

	case TypeOverpunch:
		return DecodeOverpunch(raw, places)

	default:
		return nil, fmt.Errorf("unknown D.0 data type %q", t)
	}
}

// Format is the inverse of Parse — used by the builder to render a
// canonical Go value back into the exact raw string D.0 expects on the
// wire. width is the field's own fixed length (used for zero-padding
// integers/decimals and for overpunch's own trailing-letter placement).
func Format(t TelecomDataType, v interface{}, places, width int) (string, error) {
	switch t {
	case TypeString, TypeDate:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("expected string for type %s, got %T", t, v)
		}
		return s, nil

	case TypeInteger:
		f, ok := asFloat(v)
		if !ok {
			return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
		}
		return zeroPad(strconv.FormatInt(int64(f), 10), width), nil

	case TypeDecimal:
		f, ok := asFloat(v)
		if !ok {
			return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
		}
		scaled := int64(f*pow10(places) + sign(f)*0.5)
		return zeroPad(strconv.FormatInt(scaled, 10), width), nil

	case TypeOverpunch:
		f, ok := asFloat(v)
		if !ok {
			return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
		}
		return EncodeOverpunch(f, places, width)

	default:
		return "", fmt.Errorf("unknown D.0 data type %q", t)
	}
}

func zeroPad(s string, width int) string {
	if width <= 0 || len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
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
