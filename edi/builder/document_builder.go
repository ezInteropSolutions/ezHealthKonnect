// edi/builder/document_builder.go
// The ONE generic function that builds a full ISA...IEA X12 interchange from
// canonical input data plus a *edi.X12SpecDef — the write-direction mirror
// of edi/loop_engine.go, walking the same Layer 1-4 schema types generically
// (no switch on transaction-set or segment identity anywhere).
//
// BuildInput's shape (Interchange/Header/Loops/Trailer maps) intentionally
// matches edi.ParseResult's own fields exactly, so BuildDocument(spec,
// asBuildInput(ParseTransactionSet(spec, original))) round-trips — the same
// correctness class as CDA's own builder/parser pair.
package builder

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/edi"
)

// BuildInput is the canonical data a document is built from.
type BuildInput struct {
	TransactionSet string
	Interchange    map[string]interface{} // senderId, receiverId, isaControlNumber, gsControlNumber, date, time, usageIndicator
	Header         map[string]interface{} // keyed by segment ID, e.g. "BPR", "TRN" — same shape edi.ParseResult.Header produces
	Loops          map[string]interface{} // keyed by loop ID — same shape edi.ParseResult.Loops produces
	Trailer        map[string]interface{} // keyed by segment ID, e.g. "PLB" — SE is always computed, never read from here
}

type delimiters struct{ Element, SubElement, Segment string }

// outputDelimiters are phase 1's fixed choice for BUILT documents — the
// overwhelmingly common X12 convention. (Detecting delimiters, as
// edi/segment_reader.go does, only matters when READING content someone
// else produced; a builder simply picks them.)
func outputDelimiters() delimiters {
	return delimiters{Element: "*", SubElement: ":", Segment: "~\n"}
}

// BuildDocument builds a complete ISA...IEA interchange for input.TransactionSet.
func BuildDocument(spec *edi.X12SpecDef, input BuildInput) (string, error) {
	txSet, ok := spec.TransactionSets[input.TransactionSet]
	if !ok {
		return "", fmt.Errorf("edi/builder: unknown transaction set %q", input.TransactionSet)
	}
	if spec.Envelope == nil {
		return "", fmt.Errorf("edi/builder: spec has no envelope definition")
	}

	d := outputDelimiters()
	w := &buildWalker{spec: spec, delimiters: d}

	isaControl := stringOr(input.Interchange, "isaControlNumber", generateControlNumber(9))
	gsControl := stringOr(input.Interchange, "gsControlNumber", generateControlNumber(9))
	stControl := stringOr(input.Interchange, "stControlNumber", generateControlNumber(4))

	interchangeValues := cloneWithDefaults(input.Interchange, map[string]string{
		"isaControlNumber":           isaControl,
		"gsControlNumber":            gsControl,
		"date":                       stringOr(input.Interchange, "date", time.Now().UTC().Format("060102")),
		"time":                       stringOr(input.Interchange, "time", time.Now().UTC().Format("1504")),
		"usageIndicator":             stringOr(input.Interchange, "usageIndicator", "P"),
		"functionalIdentifierCode":   txSet.FunctionalIdentifierCode,   // GS01 — no longer a shared envelope.json fixedValue, see X12TransactionSetDef
		"versionReleaseIndustryCode": txSet.VersionReleaseIndustryCode, // GS08
	})

	isaSeg := w.writeEnvelopeSegment("ISA", spec.Envelope.ISA, interchangeValues)
	gsSeg := w.writeEnvelopeSegment("GS", spec.Envelope.GS, interchangeValues)

	// Header (ST included — the ST segment is just another entry in
	// HeaderSegmentIDs, no special-casing; ST01/ST02 default from
	// TransactionSet/stControl below only when the caller's own Header data
	// doesn't already carry an "ST" entry, e.g. a fresh build with no prior
	// parse to round-trip from).
	header := cloneMap(input.Header)
	if _, hasST := header["ST"]; !hasST {
		// txSet.EffectiveST01() (not input.TransactionSet) — for a
		// disambiguated multi-variant set input.TransactionSet is the
		// human-legible schema id ("837P"), never the literal wire value
		// ST01 must carry ("837").
		header["ST"] = map[string]interface{}{"_default_01": txSet.EffectiveST01(), "_default_02": stControl}
	}

	body, err := w.writeSegmentSequence(txSet.HeaderSegmentIDs, header)
	if err != nil {
		return "", fmt.Errorf("edi/builder: header: %w", err)
	}

	loopSegs, err := w.writeLoops(txSet.Loops, input.Loops)
	if err != nil {
		return "", fmt.Errorf("edi/builder: loops: %w", err)
	}
	body = append(body, loopSegs...)

	trailer := cloneMap(input.Trailer)
	trailerSegs, err := w.writeSegmentSequence(removeID(txSet.TrailerSegmentIDs, "SE"), trailer)
	if err != nil {
		return "", fmt.Errorf("edi/builder: trailer: %w", err)
	}
	body = append(body, trailerSegs...)

	// SE is always computed, never sourced from canonical data: SE01 is the
	// total segment count from ST through SE inclusive (a real X12
	// requirement), and SE02 must equal ST02 exactly.
	seSegmentCount := len(body) + 1 // +1 for SE itself
	body = append(body, w.writeElements("SE", []*edi.X12ElementDef{
		{Pos: "01", Key: "_count"},
		{Pos: "02", Key: "_control"},
	}, map[string]interface{}{"_count": strconv.Itoa(seSegmentCount), "_control": stControl}))

	ge := w.writeElements("GE", []*edi.X12ElementDef{
		{Pos: "01", Key: "_count"},
		{Pos: "02", Key: "_control"},
	}, map[string]interface{}{"_count": "1", "_control": gsControl}) // phase 1: always exactly 1 transaction set per functional group

	iea := w.writeElements("IEA", []*edi.X12ElementDef{
		{Pos: "01", Key: "_count"},
		{Pos: "02", Key: "_control"},
	}, map[string]interface{}{"_count": "1", "_control": isaControl}) // phase 1: always exactly 1 functional group per interchange

	all := append([]string{isaSeg, gsSeg}, body...)
	all = append(all, ge, iea)
	return strings.Join(all, d.Segment) + d.Segment, nil
}

func removeID(ids []string, remove string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != remove {
			out = append(out, id)
		}
	}
	return out
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneWithDefaults(m map[string]interface{}, defaults map[string]string) map[string]interface{} {
	out := cloneMap(m)
	for k, v := range defaults {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func stringOr(m map[string]interface{}, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

// generateControlNumber produces a phase-1 timestamp/random-derived control
// number, zero-padded to digits length — no DB-backed counter table for
// phase 1 (user-confirmed: acceptable since no real trading partner is
// onboarded yet; revisit with a proper counter before any real payer
// pilot). Not guaranteed monotonically unique across process restarts.
func generateControlNumber(digits int) string {
	max := int64(1)
	for i := 0; i < digits; i++ {
		max *= 10
	}
	nanos := time.Now().UnixNano()
	randomTail := randomInt(1000)
	n := (nanos%max + int64(randomTail)) % max
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("%0*d", digits, n)
}

func randomInt(max int64) int64 {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UnixNano() % max
	}
	var n int64
	for _, c := range b {
		n = n<<8 | int64(c)
	}
	if n < 0 {
		n = -n
	}
	return n % max
}
