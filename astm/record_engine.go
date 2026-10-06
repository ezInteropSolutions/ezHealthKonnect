// astm/record_engine.go
// ONE generic walker, ParseMessage, implementing ASTM's own fixed message
// shape as plain Go control flow (not a schema-driven recursive loop tree —
// see schema_types.go's own doc comment for why):
//
//	H(1) -> [ C(0..n) ] -> [ P(1..n) -> [ C(0..n) ] ->
//	    [ O(1..n) -> [ R(0..n) | C(0..n) ] ]* ]* -> [ Q(0..n) ]* -> L(1)
//
// Comment (C) records are accepted at three real-world positions — between H
// and the first P (header-level), between a P and its first O (patient-level,
// e.g. a specimen-level note), and interleaved with a given O's own R records
// (result-level, ASTM's most common real usage) — collected into whichever
// block most recently opened, mirroring how real analyzers interleave them.
package astm

import (
	"fmt"
)

// ASTMField is one field occurrence's own value, addressed by a flat,
// block-qualified Path — mirrors edi.EDIField.
type ASTMField struct {
	Path            string // e.g. "patientBlocks[1].orderBlocks[2].results[1].4"
	Value           string
	Key             string // the field's own raw position (as a string) — Path's own trailing component
	DataType        string
	SegmentPosition int // this field's own record occurrence's 1-based position in the message
}

// RecordInstance is one record occurrence's own raw field values, keyed by
// POSITION (1-based) — mirrors edi.SegmentInstance.
type RecordInstance struct {
	RecordID string
	Position int
	RawByPos map[string]string
}

// ParseResult is the structured output of one message parse.
type ParseResult struct {
	MessageProfile  string
	Header          map[string]interface{}
	HeaderComments  []map[string]interface{}
	PatientBlocks   []map[string]interface{}
	QueryBlocks     []map[string]interface{}
	Trailer         map[string]interface{}
	Fields          map[string]*ASTMField
	RecordInstances []RecordInstance
}

type recordToken struct {
	ID     string
	Fields []string
}

// ParseMessage parses raw content (already destuffed of ENQ/STX/checksum/ETX
// framing — see services/connectors/astm_framing.go, which owns that layer)
// against spec, using the named message profile for record-type
// documentation (UsesComments/UsesQuery — advisory only; this walker accepts
// C/Q records regardless, matching this project's own "flexible, not rigid"
// validation philosophy).
func ParseMessage(spec *ASTMSpecDef, messageProfileID string, content string) (*ParseResult, error) {
	rawRecords := SplitRecords(content)
	if len(rawRecords) == 0 {
		return nil, fmt.Errorf("astm: no records found in content")
	}

	delimiters, hFound := DetectDelimiters(rawRecords)
	if !hFound {
		return nil, fmt.Errorf("astm: no H (Header) record found")
	}

	tokens := make([]recordToken, 0, len(rawRecords))
	for _, raw := range rawRecords {
		id, fields := SplitFields(raw, delimiters)
		tokens = append(tokens, recordToken{ID: id, Fields: fields})
	}

	w := &walker{
		spec:       spec,
		delimiters: delimiters,
		tokens:     tokens,
		result: &ParseResult{
			MessageProfile: messageProfileID,
			Fields:         map[string]*ASTMField{},
		},
	}
	if err := w.walk(); err != nil {
		return nil, err
	}
	return w.result, nil
}

type walker struct {
	spec       *ASTMSpecDef
	delimiters Delimiters
	tokens     []recordToken
	pos        int
	position   int // running 1-based counter over every record encountered, mirroring edi's SegmentInstance.Position
	result     *ParseResult
}

func (w *walker) peek() (recordToken, bool) {
	if w.pos >= len(w.tokens) {
		return recordToken{}, false
	}
	return w.tokens[w.pos], true
}

func (w *walker) walk() error {
	// H — required, exactly once, must be first.
	h, ok := w.peek()
	if !ok || h.ID != "H" {
		return fmt.Errorf("astm: message must start with an H (Header) record")
	}
	w.result.Header = w.parseRecord(h, "header")
	w.pos++

	// Header-level comments, before the first P.
	w.result.HeaderComments = w.collectComments("header")

	// Patient blocks.
	for {
		t, ok := w.peek()
		if !ok || t.ID != "P" {
			break
		}
		patient, err := w.parsePatientBlock(len(w.result.PatientBlocks) + 1)
		if err != nil {
			return err
		}
		w.result.PatientBlocks = append(w.result.PatientBlocks, patient)
	}

	// Query blocks (host-query mode) — zero or more Q records.
	for {
		t, ok := w.peek()
		if !ok || t.ID != "Q" {
			break
		}
		path := fmt.Sprintf("queryBlocks[%d]", len(w.result.QueryBlocks)+1)
		w.result.QueryBlocks = append(w.result.QueryBlocks, w.parseRecord(t, path))
		w.pos++
	}

	// L — required, exactly once, must be last (any trailing comment before
	// it is accepted as a final header-level-style comment — real analyzers
	// rarely do this, but accepting it costs nothing and loses no data).
	w.result.HeaderComments = append(w.result.HeaderComments, w.collectComments("header-trailing")...)
	t, ok := w.peek()
	if !ok || t.ID != "L" {
		return fmt.Errorf("astm: message must end with an L (Terminator) record")
	}
	w.result.Trailer = w.parseRecord(t, "trailer")
	w.pos++

	if w.pos != len(w.tokens) {
		return fmt.Errorf("astm: unexpected record %q after L (Terminator) at position %d", w.tokens[w.pos].ID, w.pos+1)
	}
	return nil
}

func (w *walker) parsePatientBlock(index int) (map[string]interface{}, error) {
	t, _ := w.peek() // caller already confirmed t.ID == "P"
	path := fmt.Sprintf("patientBlocks[%d]", index)
	patient := w.parseRecord(t, path)
	w.pos++

	patientComments := w.collectComments(path)
	if len(patientComments) > 0 {
		patient["comments"] = patientComments
	}

	var orders []map[string]interface{}
	for {
		ot, ok := w.peek()
		if !ok || ot.ID != "O" {
			break
		}
		order, err := w.parseOrderBlock(path, len(orders)+1)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if len(orders) > 0 {
		patient["orders"] = orders
	}
	return patient, nil
}

func (w *walker) parseOrderBlock(patientPath string, index int) (map[string]interface{}, error) {
	t, _ := w.peek() // caller already confirmed t.ID == "O"
	path := fmt.Sprintf("%s.orders[%d]", patientPath, index)
	order := w.parseRecord(t, path)
	w.pos++

	var results []map[string]interface{}
	var comments []map[string]interface{}
	for {
		rt, ok := w.peek()
		if !ok {
			break
		}
		switch rt.ID {
		case "R":
			rpath := fmt.Sprintf("%s.results[%d]", path, len(results)+1)
			results = append(results, w.parseRecord(rt, rpath))
			w.pos++
		case "C":
			cpath := fmt.Sprintf("%s.comments[%d]", path, len(comments)+1)
			comments = append(comments, w.parseRecord(rt, cpath))
			w.pos++
		default:
			goto done
		}
	}
done:
	if len(results) > 0 {
		order["results"] = results
	}
	if len(comments) > 0 {
		order["comments"] = comments
	}
	return order, nil
}

// collectComments consumes consecutive leading C records at the current
// cursor position, tagging each with blockPath for Fields addressing.
func (w *walker) collectComments(blockPath string) []map[string]interface{} {
	var comments []map[string]interface{}
	for {
		t, ok := w.peek()
		if !ok || t.ID != "C" {
			break
		}
		cpath := fmt.Sprintf("%s.comments[%d]", blockPath, len(comments)+1)
		comments = append(comments, w.parseRecord(t, cpath))
		w.pos++
	}
	return comments
}

// parseRecord decodes one record occurrence into a canonical-keyed map,
// recording every present field into w.result.Fields (flat, path-qualified)
// and w.result.RecordInstances (position-qualified raw values) regardless of
// whether the schema defines that position — an undefined position still
// gets a generic "fieldN" key and ST-typed value rather than being silently
// dropped, the same "never lose instrument-specific data the schema hasn't
// caught up to yet" principle edi/ applies via its own per-position fallback.
func (w *walker) parseRecord(t recordToken, path string) map[string]interface{} {
	w.position++
	out := map[string]interface{}{"_recordType": t.ID}
	rawByPos := map[string]string{}

	recDef := w.spec.Records[t.ID]
	defByPos := map[int]*ASTMFieldDef{}
	if recDef != nil {
		for _, f := range recDef.Fields {
			defByPos[f.Pos] = f
		}
	}

	for i, raw := range t.Fields {
		// ASTM's own standard numbering counts the record-type letter itself
		// as field 1 (e.g. "H.2" = Delimiter Definition, "O.5" = Universal
		// Test ID — the numbering this feature's own sourced field
		// dictionary uses throughout). t.Fields was already split off from
		// the record-type letter by SplitFields, so t.Fields[0] is the
		// STANDARD's own field 2 — pos must start at 2, not 1, to keep
		// ASTMFieldDef.Pos values in schema JSON matching the standard's own
		// documented field numbers exactly.
		pos := i + 2
		rawByPos[fmt.Sprintf("%d", pos)] = raw

		def := defByPos[pos]
		key := fmt.Sprintf("field%d", pos)
		dataType := string(TypeST)
		var value interface{} = raw
		fieldPath := fmt.Sprintf("%s.%d", path, pos)

		if def != nil {
			key = def.Key
			dataType = string(def.DataType)
			switch {
			case def.Component != nil:
				value = w.decodeComponent(raw, def.Component, fieldPath)
			case def.Repeat != nil:
				value = w.decodeRepeat(raw, def.Repeat, fieldPath)
			default:
				parsed, err := Parse(def.DataType, raw)
				if err == nil {
					value = parsed
				} else {
					value = raw
				}
			}
		}
		out[key] = value

		w.result.Fields[fieldPath] = &ASTMField{
			Path:            fieldPath,
			Value:           raw,
			Key:             fmt.Sprintf("%d", pos),
			DataType:        dataType,
			SegmentPosition: w.position,
		}
	}

	w.result.RecordInstances = append(w.result.RecordInstances, RecordInstance{
		RecordID: t.ID,
		Position: w.position,
		RawByPos: rawByPos,
	})

	return out
}

// decodeComponent splits raw on the component delimiter and, for each
// sub-field the schema declares, both decodes its typed value for the
// nested canonical map AND registers it in w.result.Fields under
// "<fieldPath>.<subKey>" — giving astm/validator something real to check
// for composite fields (patientName, universalTestID, dataValue, etc.)
// instead of only the parent field's own raw, un-typed joined string. Found
// missing — and fixed — while writing this package's own first validator
// tests: without this, every composite field's flat Fields entry had an
// empty DataType (component fields have no scalar DataType of their own by
// design), which the validator correctly now skips (see validator.go) but
// which, before this fix, meant composite sub-values were never checked
// anywhere at all.
func (w *walker) decodeComponent(raw string, def *ASTMComponentDef, fieldPath string) map[string]interface{} {
	parts := SplitComponents(raw, w.delimiters)
	out := map[string]interface{}{}
	for i, sub := range def.SubFields {
		if i >= len(parts) {
			break
		}
		val, err := Parse(sub.DataType, parts[i])
		if err != nil {
			val = parts[i]
		}
		out[sub.Key] = val

		if parts[i] == "" {
			continue // absent sub-field — nothing to register/validate
		}
		subPath := fmt.Sprintf("%s.%s", fieldPath, sub.Key)
		w.result.Fields[subPath] = &ASTMField{
			Path:            subPath,
			Value:           parts[i],
			Key:             sub.Key,
			DataType:        string(sub.DataType),
			SegmentPosition: w.position,
		}
	}
	return out
}

// decodeRepeat mirrors decodeComponent's own Fields-registration discipline
// for repeat-delimited items, indexed as "<fieldPath>[<n>]" (1-based,
// matching this package's own patientBlocks[1]/orders[1]-style path
// convention elsewhere).
func (w *walker) decodeRepeat(raw string, def *ASTMRepeatDef, fieldPath string) []interface{} {
	items := SplitRepeats(raw, w.delimiters)
	// []interface{}, not []map[string]interface{} or []string — this value
	// can be read directly by a goja enrichment.script derive step (e.g. to
	// flatten multiple Universal Test IDs on one O record), and this
	// codebase has one documented, previously-production-breaking bug class
	// (CLAUDE.md's "NCPDP Telecommunication D.0 Engine" section) where a
	// concretely-typed Go slice silently failed to iterate as a JS array —
	// []interface{} is the one element type proven safe across every engine
	// in this codebase for a value a script might iterate directly.
	out := make([]interface{}, 0, len(items))
	for idx, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", fieldPath, idx+1)
		if def.Item != nil && def.Item.Component != nil {
			out = append(out, w.decodeComponent(item, def.Item.Component, itemPath))
			continue
		}
		dt := TypeST
		if def.Item != nil {
			dt = def.Item.DataType
		}
		val, err := Parse(dt, item)
		if err != nil {
			val = item
		}
		out = append(out, val)

		if item != "" {
			w.result.Fields[itemPath] = &ASTMField{
				Path:            itemPath,
				Value:           item,
				Key:             fmt.Sprintf("%d", idx+1),
				DataType:        string(dt),
				SegmentPosition: w.position,
			}
		}
	}
	return out
}
