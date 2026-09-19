// ncpdptelecom/parser.go
// ParseTransmission is the read-direction mirror of
// ncpdptelecom/builder.BuildTransmission: ONE generic walk over the raw
// byte stream, driven entirely by schema data — no per-segment or
// per-transaction-code Go function, matching every other engine in this
// codebase (edi.ParseTransactionSet, ncpdp.ParseMessage).
//
// The wire shape (confirmed directly from source, see
// schemas/telecom_d0/manifest.json's own sourceRefs): a fixed-width header,
// then zero or more segments each starting with 0x1E (RS). A segment ends
// where the next RS, a 0x1D (GS), or EOF begins. GS marks "the
// currently-accumulating transaction-group cluster is complete" — since
// transmission-group segments never touch that cluster, a GS's exact
// position relative to them is harmless (confirmed against two independent
// real examples, one where GS trails a transmission-group segment and one
// where it trails a transaction-group segment).
package ncpdptelecom

import (
	"fmt"
	"strings"
)

const (
	fieldSeparator   = "\x1C" // FS — starts a field
	segmentSeparator = "\x1E" // RS — starts a segment
	groupSeparator   = "\x1D" // GS — ends the current transaction-group cluster
)

// ParseResult is the canonical output of parsing one D.0 transmission.
//
// TransactionGroups is deliberately []interface{} (holding
// map[string]interface{} elements), NOT the more specific
// []map[string]interface{} an earlier draft used — a real, engine-level bug
// found via a genuine goja script-consumption failure, not a style
// preference: goja's reflection-based Go-value wrapping does not expose a
// concretely-typed Go slice (e.g. []map[string]interface{}) as a normal,
// iterable JS array the way it does []interface{} — a script's own `for`
// loop over it silently sees zero elements, with no error at any layer.
// Every other engine in this codebase (ncpdp.parseNode's own `var items
// []interface{}` for repeatable groups, edi's own loop-instance slices)
// already follows this convention; TransactionGroups is the one place that
// hadn't, until this was caught.
type ParseResult struct {
	TransactionCode   string                 `json:"transactionCode"`
	Direction         string                 `json:"direction"`
	Header            map[string]interface{} `json:"header"`
	TransmissionGroup map[string]interface{} `json:"transmissionGroup"`
	TransactionGroups []interface{}          `json:"transactionGroups"`
}

// ParseTransmission parses a raw D.0 transmission against spec for the
// given direction ("request" or "response") — the caller always knows
// which one it's parsing (a pharmacy parses responses, a payer parses
// requests), so direction is never auto-detected from content.
func ParseTransmission(spec *TelecomSpecDef, direction string, raw string) (*ParseResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("ncpdptelecom: nil spec")
	}

	headerWidth := 0
	for _, f := range spec.HeaderFields {
		headerWidth += f.Width
	}
	if len(raw) < headerWidth {
		return nil, fmt.Errorf("ncpdptelecom: transmission shorter than the %d-byte fixed header", headerWidth)
	}

	header, err := parseHeader(spec.HeaderFields, raw[:headerWidth])
	if err != nil {
		return nil, err
	}

	code, _ := header["transactionCode"].(string)
	tx := spec.Transactions[TransactionKey(code, direction)]
	if tx == nil {
		return nil, fmt.Errorf("ncpdptelecom: unsupported transaction %q direction %q", code, direction)
	}

	transmissionGroup, transactionGroups, err := parseBody(spec, tx, raw[headerWidth:])
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		TransactionCode:   code,
		Direction:         direction,
		Header:            header,
		TransmissionGroup: transmissionGroup,
		TransactionGroups: transactionGroups,
	}, nil
}

func parseHeader(fields []TelecomFieldDef, raw string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	pos := 0
	for _, f := range fields {
		if pos+f.Width > len(raw) {
			return nil, fmt.Errorf("ncpdptelecom: header field %q exceeds transmission length", f.Key)
		}
		segment := raw[pos : pos+f.Width]
		pos += f.Width
		value, err := Parse(f.DataType, strings.TrimRight(segment, " "), f.Places)
		if err != nil {
			// A header field's own raw slice is positional, not
			// self-describing — an unparseable value (e.g. an
			// all-blank optional field) is recorded as-is rather than
			// failing the whole transmission, matching this codebase's
			// own flexible-not-rigid validator philosophy.
			out[f.Key] = strings.TrimRight(segment, " ")
			continue
		}
		out[f.Key] = value
	}
	return out, nil
}

func parseBody(spec *TelecomSpecDef, tx *TelecomTransactionDef, body string) (map[string]interface{}, []interface{}, error) {
	transmissionGroup := map[string]interface{}{}
	var transactionGroups []interface{}
	currentCluster := map[string]interface{}{}

	rawSegments := strings.Split(body, segmentSeparator)
	for _, rawSeg := range rawSegments {
		if rawSeg == "" {
			continue
		}

		endsGroup := strings.HasSuffix(rawSeg, groupSeparator)
		if endsGroup {
			rawSeg = strings.TrimSuffix(rawSeg, groupSeparator)
		}

		fields := strings.Split(rawSeg, fieldSeparator)
		fields = removeEmpty(fields)
		if len(fields) == 0 {
			continue
		}

		idField := fields[0]
		if len(idField) < 2 || idField[:2] != "AM" {
			continue // not a recognizable segment start — skip defensively rather than fail the whole transmission
		}
		identifier := idField[2:]
		segDef := spec.SegmentByIdentifier(identifier)
		if segDef != nil {
			parsedFields := parseSegmentFields(segDef, fields[1:])
			switch {
			case containsKey(tx.TransmissionGroupSegments, segDef.Key):
				transmissionGroup[segDef.Key] = parsedFields
			case containsKey(tx.TransactionGroupSegments, segDef.Key):
				currentCluster[segDef.Key] = parsedFields
			}
			// A segment recognized by the spec but not listed for this
			// specific transaction is silently ignored — a real-world
			// sender including an out-of-scope segment shouldn't fail
			// the whole transmission, matching this codebase's own
			// flexible-not-rigid philosophy.
		}

		if endsGroup && len(currentCluster) > 0 {
			transactionGroups = append(transactionGroups, currentCluster)
			currentCluster = map[string]interface{}{}
		}
	}
	if len(currentCluster) > 0 {
		transactionGroups = append(transactionGroups, currentCluster)
	}

	return transmissionGroup, transactionGroups, nil
}

func parseSegmentFields(segDef *TelecomSegmentDef, rawFields []string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, raw := range rawFields {
		if len(raw) < 2 {
			continue
		}
		fieldID := raw[:2]
		value := raw[2:]
		f := segDef.FieldByID(fieldID)
		if f == nil {
			continue // an unrecognized field within a known segment is skipped, not fatal
		}
		parsed, err := Parse(f.DataType, value, f.Places)
		if err != nil {
			out[f.Key] = value // keep the raw value rather than dropping it on a soft parse failure
			continue
		}
		out[f.Key] = parsed
	}
	return out
}

func containsKey(list []string, key string) bool {
	for _, k := range list {
		if k == key {
			return true
		}
	}
	return false
}

// SniffDirection makes a best-effort guess at whether raw is a request or a
// response, for callers (the generic MessageParser auto-detection path)
// that have no other way to know — the explicit ncpdptelecom.parse/build
// pipeline steps should always be configured with a real, known direction
// instead of relying on this. Request segments (01-16) and response
// segments (20-29) use fully disjoint identifier ranges on the wire (see
// schemas/telecom_d0/manifest.json's own sourceRefs) — this inspects the
// FIRST recognizable segment's own AM identifier and classifies by that
// range. Defaults to "request" if no segment can be found at all (an
// honest default, not a guess dressed up as certainty).
func SniffDirection(spec *TelecomSpecDef, raw string) string {
	headerWidth := 0
	for _, f := range spec.HeaderFields {
		headerWidth += f.Width
	}
	if len(raw) <= headerWidth {
		return "request"
	}
	body := raw[headerWidth:]
	for _, rawSeg := range strings.Split(body, segmentSeparator) {
		rawSeg = strings.TrimSuffix(rawSeg, groupSeparator)
		fields := strings.Split(rawSeg, fieldSeparator)
		fields = removeEmpty(fields)
		if len(fields) == 0 {
			continue
		}
		idField := fields[0]
		if len(idField) < 4 || idField[:2] != "AM" {
			continue
		}
		identifier := idField[2:]
		if identifier >= "20" {
			return "response"
		}
		return "request"
	}
	return "request"
}

func removeEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
