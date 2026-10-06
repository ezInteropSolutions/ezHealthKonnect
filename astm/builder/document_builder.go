// astm/builder/document_builder.go
// Build direction: canonical JSON -> ASTM record text. Mirrors
// edi/builder/document_builder.go's entry-point shape (one BuildInput ->
// BuildDocument), simplified for ASTM's own fixed message tree (see
// astm/record_engine.go's own doc comment for why this isn't a generic
// recursive loop walk).
//
// Output is the destuffed record stream only (records joined by CR) — ENQ/
// STX/checksum/EOT framing is owned by services/connectors/astm_framing.go
// at the transport layer, exactly mirroring astm.ParseMessage's own
// "already destuffed" input contract on the read side.
package builder

import (
	"fmt"
	"strconv"
	"strings"

	"ezhealthkonnect/astm"
)

// BuildInput is the canonical shape BuildDocument consumes — field keys match
// astm.ParseResult's own canonical keys exactly, so a parse->build round trip
// (or a hand-authored canonical payload from astm.map_to_canonical) needs no
// translation layer.
type BuildInput struct {
	MessageProfile string
	Header         map[string]interface{}
	HeaderComments []map[string]interface{}
	PatientBlocks  []map[string]interface{} // each entry may carry "comments" []map[string]interface{} and "orders" []map[string]interface{}; each order may carry "results"/"comments" []map[string]interface{}
	QueryBlocks    []map[string]interface{}
	Trailer        map[string]interface{}
}

// outputDelimiters is the standard ASTM convention this builder always
// writes with — a message's own H record is the single source of truth for
// what delimiters IT used on the wire, so the builder declares them here
// once rather than accepting a caller override that could silently disagree
// with what it actually writes.
func outputDelimiters() astm.Delimiters {
	return astm.Delimiters{Field: "|", Repeat: "\\", Component: "^", Escape: "&"}
}

// BuildDocument renders input into the full ASTM record stream for its
// message profile.
func BuildDocument(spec *astm.ASTMSpecDef, input BuildInput) (string, error) {
	d := outputDelimiters()
	w := &buildWalker{spec: spec, delimiters: d}

	var records []string

	hRec := spec.Records["H"]
	if hRec == nil {
		return "", fmt.Errorf("astm/builder: spec has no H record definition")
	}
	headerValues := cloneMap(input.Header)
	headerValues["delimiterDefinition"] = d.Repeat + d.Component + d.Escape
	line, err := w.writeRecord(hRec, "H", 1, headerValues)
	if err != nil {
		return "", fmt.Errorf("astm/builder: header: %w", err)
	}
	records = append(records, line)

	for _, c := range input.HeaderComments {
		line, err := w.writeRecord(spec.Records["C"], "C", w.nextSeq("C"), c)
		if err != nil {
			return "", fmt.Errorf("astm/builder: header comment: %w", err)
		}
		records = append(records, line)
	}

	for pIdx, patient := range input.PatientBlocks {
		pRec := spec.Records["P"]
		if pRec == nil {
			return "", fmt.Errorf("astm/builder: spec has no P record definition")
		}
		w.resetSeq("P", "O", "R", "C")
		line, err := w.writeRecord(pRec, "P", pIdx+1, patient)
		if err != nil {
			return "", fmt.Errorf("astm/builder: patientBlocks[%d]: %w", pIdx, err)
		}
		records = append(records, line)

		for _, c := range toMapSlice(patient["comments"]) {
			line, err := w.writeRecord(spec.Records["C"], "C", w.nextSeq("C"), c)
			if err != nil {
				return "", fmt.Errorf("astm/builder: patientBlocks[%d].comments: %w", pIdx, err)
			}
			records = append(records, line)
		}

		for oIdx, order := range toMapSlice(patient["orders"]) {
			oRec := spec.Records["O"]
			if oRec == nil {
				return "", fmt.Errorf("astm/builder: spec has no O record definition")
			}
			line, err := w.writeRecord(oRec, "O", w.nextSeq("O"), order)
			if err != nil {
				return "", fmt.Errorf("astm/builder: patientBlocks[%d].orders[%d]: %w", pIdx, oIdx, err)
			}
			records = append(records, line)

			for _, r := range toMapSlice(order["results"]) {
				line, err := w.writeRecord(spec.Records["R"], "R", w.nextSeq("R"), r)
				if err != nil {
					return "", fmt.Errorf("astm/builder: patientBlocks[%d].orders[%d].results: %w", pIdx, oIdx, err)
				}
				records = append(records, line)
			}
			for _, c := range toMapSlice(order["comments"]) {
				line, err := w.writeRecord(spec.Records["C"], "C", w.nextSeq("C"), c)
				if err != nil {
					return "", fmt.Errorf("astm/builder: patientBlocks[%d].orders[%d].comments: %w", pIdx, oIdx, err)
				}
				records = append(records, line)
			}
		}
	}

	w.resetSeq("Q")
	for qIdx, q := range input.QueryBlocks {
		qRec := spec.Records["Q"]
		if qRec == nil {
			return "", fmt.Errorf("astm/builder: spec has no Q record definition")
		}
		line, err := w.writeRecord(qRec, "Q", qIdx+1, q)
		if err != nil {
			return "", fmt.Errorf("astm/builder: queryBlocks[%d]: %w", qIdx, err)
		}
		records = append(records, line)
	}

	lRec := spec.Records["L"]
	if lRec == nil {
		return "", fmt.Errorf("astm/builder: spec has no L record definition")
	}
	trailerValues := cloneMap(input.Trailer)
	if _, ok := trailerValues["terminationCode"]; !ok {
		trailerValues["terminationCode"] = "N"
	}
	line, err = w.writeRecord(lRec, "L", 1, trailerValues)
	if err != nil {
		return "", fmt.Errorf("astm/builder: trailer: %w", err)
	}
	records = append(records, line)

	return strings.Join(records, "\r") + "\r", nil
}

type buildWalker struct {
	spec       *astm.ASTMSpecDef
	delimiters astm.Delimiters
	seq        map[string]int
}

// nextSeq returns record-type-scoped, auto-incrementing sequence numbers for
// C/O/R — the overwhelming real-world convention (sequence numbers reset per
// patient and increment per record of that same type within the current
// patient), matching every one of this feature's own sourced examples.
func (w *buildWalker) nextSeq(recordID string) int {
	if w.seq == nil {
		w.seq = map[string]int{}
	}
	w.seq[recordID]++
	return w.seq[recordID]
}

func (w *buildWalker) resetSeq(recordIDs ...string) {
	if w.seq == nil {
		w.seq = map[string]int{}
	}
	for _, id := range recordIDs {
		w.seq[id] = 0
	}
}

// writeRecord renders one record occurrence from canonical values, defaulting
// its own sequence-number field (standard position 2) to defaultSeq when the
// caller's values map doesn't already supply one.
func (w *buildWalker) writeRecord(recDef *astm.ASTMRecordDef, recordID string, defaultSeq int, values map[string]interface{}) (string, error) {
	if recDef == nil {
		return "", fmt.Errorf("no record definition for %q", recordID)
	}
	if values == nil {
		values = map[string]interface{}{}
	}

	maxPos := 2 // every record has at least the sequence-number field at position 2
	for _, f := range recDef.Fields {
		if f.Pos > maxPos {
			maxPos = f.Pos
		}
		if f.Repeat != nil {
			// repeat fields occupy exactly one position regardless of item count
			continue
		}
	}

	// positions[i] holds standard field position i+2 (position 1 is the
	// record-type letter itself, written separately below; position 2 is
	// the sequence number, defaulted here unless the schema overrides it).
	positions := make([]string, maxPos-1)
	positions[0] = strconv.Itoa(defaultSeq)

	byPos := map[int]*astm.ASTMFieldDef{}
	for _, f := range recDef.Fields {
		byPos[f.Pos] = f
	}

	for pos := 2; pos <= maxPos; pos++ {
		def := byPos[pos]
		if def == nil {
			continue
		}
		raw, ok := values[def.Key]
		if !ok || raw == nil {
			if def.FixedValue != "" {
				positions[pos-2] = def.FixedValue
			}
			continue
		}

		var out string
		var err error
		switch {
		case def.Component != nil:
			out, err = w.writeComponent(raw, def.Component)
		case def.Repeat != nil:
			out, err = w.writeRepeat(raw, def.Repeat)
		default:
			out, err = astm.Format(def.DataType, raw)
		}
		if err != nil {
			return "", fmt.Errorf("field %q (pos %d): %w", def.Key, pos, err)
		}
		positions[pos-2] = out
	}

	// Sequence number (position 2) may be explicitly overridden by the
	// caller's own canonical value — most callers rely on the auto-increment
	// above, but a round-tripped parse->build must be able to reproduce the
	// exact original sequence numbers too.
	if seqDef := byPos[2]; seqDef != nil {
		if raw, ok := values[seqDef.Key]; ok && raw != nil {
			formatted, err := astm.Format(seqDef.DataType, raw)
			if err == nil && formatted != "" {
				positions[0] = formatted
			}
		}
	}

	for len(positions) > 0 && positions[len(positions)-1] == "" {
		positions = positions[:len(positions)-1]
	}

	return recordID + w.delimiters.Field + strings.Join(positions, w.delimiters.Field), nil
}

func (w *buildWalker) writeComponent(raw interface{}, def *astm.ASTMComponentDef) (string, error) {
	m, ok := raw.(map[string]interface{})
	if !ok {
		// Caller supplied a plain scalar for a component field — write it as
		// the first sub-component and leave the rest empty, rather than
		// erroring; a canonical payload built by hand often only cares about
		// one sub-part (e.g. just the test code).
		return fmt.Sprintf("%v", raw), nil
	}
	maxIdx := 0
	for i := range def.SubFields {
		maxIdx = i
	}
	parts := make([]string, maxIdx+1)
	for i, sub := range def.SubFields {
		v, ok := m[sub.Key]
		if !ok || v == nil {
			continue
		}
		formatted, err := astm.Format(sub.DataType, v)
		if err != nil {
			return "", err
		}
		parts[i] = formatted
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, w.delimiters.Component), nil
}

func (w *buildWalker) writeRepeat(raw interface{}, def *astm.ASTMRepeatDef) (string, error) {
	items := toInterfaceSlice(raw)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if def.Item != nil && def.Item.Component != nil {
			s, err := w.writeComponent(item, def.Item.Component)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
			continue
		}
		dt := astm.TypeST
		if def.Item != nil {
			dt = def.Item.DataType
		}
		s, err := astm.Format(dt, item)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, w.delimiters.Repeat), nil
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toMapSlice(v interface{}) []map[string]interface{} {
	switch arr := v.(type) {
	case []map[string]interface{}:
		return arr
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func toInterfaceSlice(v interface{}) []interface{} {
	switch arr := v.(type) {
	case []interface{}:
		return arr
	case []map[string]interface{}:
		out := make([]interface{}, len(arr))
		for i, item := range arr {
			out[i] = item
		}
		return out
	case []string:
		out := make([]interface{}, len(arr))
		for i, item := range arr {
			out[i] = item
		}
		return out
	default:
		if v == nil {
			return nil
		}
		return []interface{}{v}
	}
}
