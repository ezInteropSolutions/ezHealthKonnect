// astm/datatypes.go
// Layer 0 of the ASTM E1394-97 schema-driven engine: the small set of base
// field data types (ST/NM/DT/TM/ID) plus the frame-level protocol constants
// (control bytes, frame-number wraparound, checksum). Mirrors edi/datatypes.go
// in spirit — one shared registry every field validates/parses/formats
// through, never a per-record implementation — but ASTM's own type system is
// materially smaller than X12's (no composite/implied-decimal numeric family).
package astm

import (
	"fmt"
	"strconv"
	"strings"
)

// ASTMDataType identifies one of ASTM's base field data types.
type ASTMDataType string

const (
	TypeST ASTMDataType = "ST" // String — free text
	TypeNM ASTMDataType = "NM" // Numeric — integer or decimal, may carry a leading sign
	TypeDT ASTMDataType = "DT" // Date, YYYYMMDD
	TypeTM ASTMDataType = "TM" // Timestamp, YYYYMMDDHHMMSS (or a DT-length date-only value — both accepted)
	TypeID ASTMDataType = "ID" // Coded identifier/enumerated value — validated as a string; a ValueSet, when set, is documentation only in phase 1 (mirrors edi's own ID type)
)

// Validate checks raw against t's shape. An empty raw always passes (every
// ASTM field is effectively optional at the wire level — record-level
// requiredness, if any, is enforced by astm/validator, not here).
func Validate(t ASTMDataType, raw string) error {
	if raw == "" {
		return nil
	}

	switch t {
	case TypeST, TypeID:
		return nil

	case TypeDT:
		if len(raw) != 8 {
			return fmt.Errorf("date %q must be 8 digits (YYYYMMDD)", raw)
		}
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("date %q is not numeric: %w", raw, err)
		}
		return nil

	case TypeTM:
		if len(raw) != 8 && len(raw) != 14 {
			return fmt.Errorf("timestamp %q must be 8 digits (YYYYMMDD) or 14 digits (YYYYMMDDHHMMSS)", raw)
		}
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("timestamp %q is not numeric: %w", raw, err)
		}
		return nil

	case TypeNM:
		trimmed := strings.TrimPrefix(raw, "-")
		trimmed = strings.TrimPrefix(trimmed, "+")
		if trimmed == "" {
			return fmt.Errorf("numeric %q has no digits", raw)
		}
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return fmt.Errorf("numeric %q is not a valid number: %w", raw, err)
		}
		return nil

	default:
		return fmt.Errorf("unknown ASTM data type %q", t)
	}
}

// Parse converts raw into its semantic Go value: string for ST/ID/DT/TM (kept
// as the raw wire form — a caller needing a time.Time converts explicitly,
// mirroring edi.Parse's own convention for DT/TM), float64 for NM.
func Parse(t ASTMDataType, raw string) (interface{}, error) {
	if raw == "" {
		return "", nil
	}
	if err := Validate(t, raw); err != nil {
		return nil, err
	}
	if t == TypeNM {
		return strconv.ParseFloat(raw, 64)
	}
	return raw, nil
}

// Format is the inverse of Parse — used by the builder to render a canonical
// Go value back into the exact raw string ASTM expects on the wire.
func Format(t ASTMDataType, v interface{}) (string, error) {
	if v == nil {
		return "", nil
	}
	switch t {
	case TypeST, TypeID, TypeDT, TypeTM:
		switch s := v.(type) {
		case string:
			return s, nil
		case fmt.Stringer:
			return s.String(), nil
		default:
			return fmt.Sprintf("%v", v), nil
		}
	case TypeNM:
		if f, ok := asFloat(v); ok {
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		}
		if s, ok := v.(string); ok {
			// Already a wire-formatted numeric string (e.g. from map-to-canonical
			// mapping, which is string-based throughout this codebase) — pass
			// through after validating its shape, mirroring ncpdptelecom.Format's
			// own numeric-string acceptance fix (CLAUDE.md's "NCPDP Outbound
			// Direction" section) rather than rejecting a perfectly valid value.
			if err := Validate(TypeNM, s); err != nil {
				return "", err
			}
			return s, nil
		}
		return "", fmt.Errorf("expected numeric value for type %s, got %T", t, v)
	default:
		return "", fmt.Errorf("unknown ASTM data type %q", t)
	}
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

// ===========================================================================
// Frame-level protocol constants (the ENQ/ACK/STX/ETX/EOT transport layer —
// consumed by services/connectors/astm_framing.go, not by the record engine
// itself, which operates on already-destuffed record text).
// ===========================================================================

const (
	ENQ byte = 0x05
	ACK byte = 0x06
	NAK byte = 0x15
	STX byte = 0x02
	ETX byte = 0x03
	ETB byte = 0x17 // intermediate-block terminator (more frames follow for this same transmission)
	EOT byte = 0x04
	CR  byte = 0x0D
	LF  byte = 0x0A
)

// NextFrameNumber advances ASTM's rolling 0-7 frame counter.
func NextFrameNumber(current int) int {
	return (current + 1) % 8
}

// Checksum computes ASTM's frame checksum: the sum of every byte from the
// frame-number digit (inclusive, the byte immediately after STX — STX
// itself contributes nothing) through the ETX/ETB terminator (inclusive),
// modulo 256, rendered as 2 uppercase hex digits (left-zero-padded if the
// result is a single hex digit). frameAndContent is that exact byte range.
//
// Algorithm confirmed via this feature's own Pre-Phase sourcing gate against
// 2 independently-authored technical sources in full agreement: a real,
// shipped vendor LIS-interface manual (mediff "ASTM-Protokoll" v1.9,
// 2011-07-12, directly citing ASTM E-1381's own checksum definition) and a
// dedicated checksum reference page with worked code (Hendrickson Group,
// "Calculating the Checksum of an ASTM Document"), plus a generic
// web-aggregated secondary summary independently stating the same algorithm.
// One honest gap: neither readable source showed an actual numeric worked
// example with the real checksum byte values spelled out (every worked frame
// dump found redacted the checksum as a placeholder) — astm_framing.go's own
// tests are this engine's first byte-for-byte verification of the formula.
func Checksum(frameAndContent []byte) string {
	sum := 0
	for _, b := range frameAndContent {
		sum += int(b)
	}
	return fmt.Sprintf("%02X", sum%256)
}
