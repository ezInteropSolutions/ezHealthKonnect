// edi/loop_engine.go
// The ONE generic recursive engine that parses a raw X12 segment stream
// against a *X12TransactionSetDef's loop tree — no switch on transaction-set
// or segment identity anywhere. Loops are entered by matching the current
// segment's ID against each candidate loop's trigger, in schema order.
//
// Sibling loops can share the same trigger segment ID — e.g. 835's 1000A
// (payer) and 1000B (payee) both start with N1, really distinguished in the
// spec only by N101's value ("PR" vs "PE"). matchLoops resolves this
// WITHOUT needing an element-value discriminator: a non-repeating loop
// that's already been matched once is skipped on subsequent scans, so the
// second N1-led group in the data falls through 1000A (already consumed) to
// 1000B automatically, purely from schema order + "don't match the same
// non-repeating loop twice." This only holds when sibling loops with a
// shared trigger appear in the data in schema-declared order — true for any
// standards-conformant document, since the IG itself fixes that order — so
// no discriminator field was added to X12LoopDef for phase 1. If a future
// transaction set ever needs one (loops legitimately reorderable relative to
// each other), add an explicit element-value discriminator then, not now.
//
// edi/ has no dependency on models/ — this package's own ParseResult/
// EDIField types are converted to models.ParserResult by the adapter layer
// in services/parsers/edix12.
package edi

import (
	"fmt"
	"strings"
)

// EDIField is one flat, loop-qualified-addressed field extracted from a
// parsed message — the read-direction counterpart to how HL7/CDA already
// expose schema-annotated fields, using X12's own SEGMENT.pos convention
// instead of CDA's XPath or HL7's SEG.n.
type EDIField struct {
	Path       string // e.g. "2100.2110[2].SVC.02"
	Value      string
	Key        string // the element's raw position ("02") — Path's own trailing component; the addressing
	                   // convention is deliberately position-based, not canonical-key-based (unlike the
	                   // nested ParsedJSON tree, which DOES use canonical Keys where the schema defines them)
	DataType       string
	FixedValue     string // set when the schema declares this element as a business-fixed constant, e.g. ST01 == "835"
	MinLength      int
	MaxLength      int
	SegmentPosition int // this field's own segment occurrence's 1-based position in the transaction set — same value, same counter as SegmentInstance.Position, stamped per-field so a caller (edi.generate_999's own IK3 generation) never has to re-parse Path to correlate a field-level error back to its segment occurrence
}

// ParseResult is the structured output of one transaction-set parse.
type ParseResult struct {
	TransactionSet   string
	EnvelopePresent  bool
	Interchange      map[string]interface{}
	Header           map[string]interface{}
	Loops            map[string]interface{}
	Trailer          map[string]interface{}
	Fields           map[string]*EDIField // flat, loop-qualified addressing keyed by Path
	SegmentInstances []SegmentInstance    // every segment occurrence encountered, flat — see SegmentInstance's own doc comment
}

// SegmentInstance is one segment occurrence's own raw element values, keyed
// by POSITION ("01", "02", ...) — matching SyntaxRule.Positions directly, so
// edi/validator can check presence without a position-to-key lookup.
// Recorded flat, independent of where in the loop tree the occurrence was,
// purely so the validator can check a segment's own SyntaxRules (which
// relate element positions within ONE occurrence) without re-walking the
// nested ParsedJSON tree itself.
//
// Position is this occurrence's 1-based ordinal among EVERY segment matched
// from ST through SE inclusive — the real X12 "Segment Position in
// Transaction Set" a 999's own IK3 (Error Identification) segment reports.
// Every segment occurrence gets one (not just ones with SyntaxRules — a
// broader use than checkSyntaxRules alone needed, but a free byproduct of
// the same walk, and the only piece IK3 generation needs that the flat,
// loop-qualified EDIField.Path can't already answer on its own).
type SegmentInstance struct {
	SegmentID string
	Position  int
	RawByPos  map[string]string
}

type segmentToken struct {
	ID       string
	Elements []string // raw element strings; Elements[i] is position i+1 (1-based, matching X12's own numbering)
}

// ParseTransactionSet parses raw content (with or without an ISA envelope —
// see edi/segment_reader.go's DetectDelimiters) against spec.
func ParseTransactionSet(spec *X12SpecDef, content string) (*ParseResult, error) {
	delimiters, envelopePresent := DetectDelimiters(content)
	rawSegments := SplitSegments(content, delimiters)

	tokens := make([]segmentToken, 0, len(rawSegments))
	for _, raw := range rawSegments {
		id, elems := ParseSegment(raw, delimiters)
		tokens = append(tokens, segmentToken{ID: id, Elements: elems})
	}

	interchange := map[string]interface{}{}
	gs08 := ""
	stIndex := -1
	for i, t := range tokens {
		switch t.ID {
		case "ISA":
			// ISA's 16 elements are fixed-width, space-padded to their exact
			// declared length when built (see edi/builder's writeEnvelopeSegment) —
			// unlike every other X12 segment, which is variable-length/delimited.
			// Trim that padding back off on read, standard practice for this one segment.
			interchange["senderId"] = strings.TrimRight(elementAt(t, 6), " ")
			interchange["receiverId"] = strings.TrimRight(elementAt(t, 8), " ")
			interchange["date"] = strings.TrimRight(elementAt(t, 9), " ")
			interchange["time"] = strings.TrimRight(elementAt(t, 10), " ")
			interchange["isaControlNumber"] = strings.TrimRight(elementAt(t, 13), " ")
			interchange["usageIndicator"] = strings.TrimRight(elementAt(t, 15), " ")
		case "GS":
			interchange["gsControlNumber"] = elementAt(t, 6)
			interchange["functionalIdentifierCode"] = elementAt(t, 1)
			interchange["versionReleaseIndustryCode"] = elementAt(t, 8)
			gs08 = elementAt(t, 8)
		case "ST":
			stIndex = i
		}
		if stIndex >= 0 {
			break
		}
	}
	if stIndex < 0 {
		return nil, fmt.Errorf("edi: no ST segment found in content")
	}

	txSetID := elementAt(tokens[stIndex], 1)
	interchange["stControlNumber"] = elementAt(tokens[stIndex], 2)

	// Multi-variant transaction sets (837P/837I both literally ST01=="837")
	// are disambiguated by GS08 — try the composite key first, then fall
	// back to the bare ST01 key, which covers every single-variant set (835,
	// 999) unchanged. See CLAUDE.md's EDI Phase 2 section for why GS08 (not
	// ST03) is the real disambiguator: it's REQUIRED in every GS segment,
	// unlike ST03, which many real trading partners leave blank.
	var txSet *X12TransactionSetDef
	var ok bool
	if gs08 != "" {
		txSet, ok = spec.TransactionSets[txSetID+":"+gs08]
	}
	if !ok {
		txSet, ok = spec.TransactionSets[txSetID]
	}
	if !ok {
		return nil, fmt.Errorf("edi: unknown transaction set %q (GS08 %q)", txSetID, gs08)
	}

	seIndex := -1
	for i := stIndex; i < len(tokens); i++ {
		if tokens[i].ID == "SE" {
			seIndex = i
			break
		}
	}
	endIndex := len(tokens)
	if seIndex >= 0 {
		endIndex = seIndex + 1 // inclusive of SE — TrailerSegmentIDs consumes it below
	}

	w := &walker{spec: spec, tokens: tokens[stIndex:endIndex], delimiters: delimiters}
	pos := 0

	header := map[string]interface{}{}
	if err := w.matchSegmentSequence(&pos, txSet.HeaderSegmentIDs, header, nil); err != nil {
		return nil, fmt.Errorf("edi: header: %w", err)
	}

	loops, err := w.matchLoops(&pos, txSet.Loops, nil)
	if err != nil {
		return nil, fmt.Errorf("edi: loops: %w", err)
	}

	trailer := map[string]interface{}{}
	if err := w.matchSegmentSequence(&pos, txSet.TrailerSegmentIDs, trailer, nil); err != nil {
		return nil, fmt.Errorf("edi: trailer: %w", err)
	}

	return &ParseResult{
		// txSet.TransactionSetID (not the raw txSetID/ST01 local var) — for a
		// disambiguated multi-variant set this is the human-legible resolved
		// id ("837P"), never the ambiguous bare "837" both variants share.
		// See X12TransactionSetDef's own doc comment.
		TransactionSet:   txSet.TransactionSetID,
		EnvelopePresent:  envelopePresent,
		Interchange:      interchange,
		Header:           header,
		Loops:            loops,
		Trailer:          trailer,
		Fields:           w.renderFields(),
		SegmentInstances: w.segmentInstances,
	}, nil
}

func elementAt(t segmentToken, pos int) string {
	idx := pos - 1
	if idx < 0 || idx >= len(t.Elements) {
		return ""
	}
	return t.Elements[idx]
}

// pathStep is one component of a flat field's path — a loop or repeating
// segment name plus its 1-based occurrence index within its immediate
// parent. Index is 0 for anything that never repeats at that position
// (never bracketed). Rendering (bracket or not) is deferred to
// renderFields, since the plan's own addressing rule — "[n] only when it
// repeats more than once IN THIS SPECIFIC MESSAGE" — can only be decided
// once every occurrence has actually been counted, not while still walking.
type pathStep struct {
	Name  string
	Index int
}

type pendingField struct {
	Steps           []pathStep
	Value           string
	Key             string
	DataType        string
	FixedValue      string
	MinLength       int
	MaxLength       int
	SegmentPosition int
}

// walker carries the token stream and accumulates flat fields as it walks;
// index bookkeeping and final path rendering happen once, at the end
// (renderFields), since a step's own final bracket-or-not decision depends
// on the total count observed across the whole walk.
type walker struct {
	spec             *X12SpecDef
	tokens           []segmentToken
	delimiters       Delimiters
	pending          []pendingField
	segmentInstances []SegmentInstance
	segmentPosition  int // running 1-based count of every segment matched from ST — see SegmentInstance.Position's own doc comment
}

// matchSegmentSequence treats segmentIDs as the SET of segments expected at
// this one level (a transaction set's header/trailer, or one loop's own
// segment list) and greedily consumes tokens from *pos for as long as the
// current token's ID is a member of that set — WITHOUT requiring segmentIDs'
// own list order to match the data's actual order among themselves.
//
// This matters because two mutually-independent OPTIONAL segments at the
// same level have no real ordering constraint in X12 itself, and real
// trading partners disagree on which one they emit first — e.g. an 835
// header's REF (receiver ID) and DTM (production date): one real, unedited
// 005010X221A1 sample from eMedNY emits REF before DTM, another real,
// unedited sample from a dental payer emits DTM before REF. A strict
// single-pass walk over segmentIDs in schema-declared order (the previous
// implementation) would find DTM where it expected REF, give up on REF
// forever, and leave the real REF token stranded — which then poisons every
// later structural boundary the walker checks against it, surfacing as
// completely unrelated failures like "required segment SE not found at
// position N". Consuming this level's own segment IDs as an unordered set —
// still stopping the instant a token isn't a member of the set at all, which
// is what correctly ends this level and hands off to whatever structurally
// follows (a loop trigger, a trailer, or a genuinely unrelated segment) —
// fixes this for every transaction set and every loop's own segment list in
// one place, matching this engine's own "flexible, not rigid" design intent
// (see edi/validator's syntax-rule severity split for the same principle
// applied to element-relational constraints instead of segment order).
func (w *walker) matchSegmentSequence(pos *int, segmentIDs []string, out map[string]interface{}, parentPath []pathStep) error {
	idSet := make(map[string]bool, len(segmentIDs))
	for _, id := range segmentIDs {
		idSet[id] = true
	}
	matchedOnce := make(map[string]bool, len(segmentIDs))

	for *pos < len(w.tokens) {
		id := w.tokens[*pos].ID
		if !idSet[id] {
			break // not part of this level — reached a loop trigger, the trailer, or an unrelated segment
		}
		segDef, ok := w.spec.Segments[id]
		if !ok {
			return fmt.Errorf("segment %q not found in shared library", id)
		}
		if matchedOnce[id] && !segDef.RepeatsMultiple() {
			// A second, non-contiguous occurrence of a non-repeating ID
			// structurally belongs to whatever comes next (a later loop
			// instance sharing the same segment type, or non-conformant
			// data) — not to this level a second time. Same exit as an
			// unrelated segment, not an error.
			break
		}

		index := 0
		if segDef.RepeatsMultiple() {
			index = countExisting(out[id]) + 1
		}
		instance := w.parseSegmentInstance(w.tokens[*pos], segDef, append(parentPath, pathStep{Name: id, Index: index}))
		*pos++
		matchedOnce[id] = true

		if segDef.RepeatsMultiple() {
			arr, _ := out[id].([]map[string]interface{})
			out[id] = append(arr, instance)
		} else {
			out[id] = instance
		}
	}

	for _, segID := range segmentIDs {
		segDef := w.spec.Segments[segID]
		if segDef != nil && segDef.IsRequired() && !matchedOnce[segID] {
			return fmt.Errorf("required segment %q not found at position %d", segID, *pos)
		}
	}
	return nil
}

func countExisting(v interface{}) int {
	arr, ok := v.([]map[string]interface{})
	if !ok {
		return 0
	}
	return len(arr)
}

// parseSegmentInstance builds one segment occurrence's output map (keyed by
// each element's canonical Key, falling back to its raw position) and
// records the same data as pending flat fields, then applies the segment's
// own intra-segment X12RepeatDef groups (e.g. CAS) on top.
func (w *walker) parseSegmentInstance(t segmentToken, segDef *X12SegmentDef, path []pathStep) map[string]interface{} {
	w.segmentPosition++
	out := map[string]interface{}{}
	consumedPositions := map[int]bool{}
	rawByPos := map[string]string{}

	for _, el := range segDef.Elements {
		posNum := atoiSafe(el.Pos)
		consumedPositions[posNum] = true
		raw := elementAt(t, posNum)
		rawByPos[el.Pos] = raw
		if raw == "" && el.FixedValue == "" {
			continue // absent situational element — don't emit an empty field
		}

		key := el.Key
		if key == "" {
			key = el.Pos
		}

		if el.Composite != nil {
			// Composite sub-elements are exposed in the nested ParsedJSON
			// tree (out[key] below) but NOT in the flat, loop-qualified
			// EnhancedFields view — the addressing convention only covers
			// plain scalar elements for phase 1. A composite-aware flat key
			// (e.g. "...SVC.01.1") is a reasonable phase-2 addition if a
			// real need for field-picker access into composites shows up;
			// not built now since nothing in phase 1 requires it.
			out[key] = w.parseComposite(raw, el.Composite)
		} else {
			out[key] = raw
			w.pending = append(w.pending, pendingField{
				// path already ends in this segment's own step — matchSegmentSequence
				// appends it before calling parseSegmentInstance — so it's cloned
				// as-is here, not appended to again.
				Steps:           append([]pathStep{}, path...),
				Value:           raw,
				Key:             el.Pos,
				DataType:        string(el.DataType),
				FixedValue:      el.FixedValue,
				MinLength:       el.MinLength,
				MaxLength:       el.MaxLength,
				SegmentPosition: w.segmentPosition,
			})
		}
	}

	for _, group := range segDef.Repeats {
		instances := w.parseRepeatGroup(t, group, consumedPositions, rawByPos)
		if len(instances) > 0 {
			out[group.Key] = instances
		}
	}

	// Always recorded now (not gated on the segment having SyntaxRules) —
	// checkSyntaxRules' own inner loop over segDef.SyntaxRules is a no-op for
	// a segment with none, so this is a free, harmless superset for that
	// caller, and the only source of Position data edi.generate_999's own
	// IK3 (Error Identification) generation needs.
	w.segmentInstances = append(w.segmentInstances, SegmentInstance{SegmentID: segDef.ID, Position: w.segmentPosition, RawByPos: rawByPos})

	return out
}

// parseComposite splits one composite element by the message's own detected
// sub-element separator and returns a map keyed by each sub-element's own
// Key (falling back to its 1-based position within the composite).
func (w *walker) parseComposite(raw string, def *X12CompositeDef) map[string]interface{} {
	out := map[string]interface{}{}
	if raw == "" {
		return out
	}
	parts := SplitSubElements(raw, w.delimiters)
	for i, sub := range def.SubElements {
		if i >= len(parts) {
			break
		}
		key := sub.Key
		if key == "" {
			key = sub.Pos
		}
		out[key] = parts[i]
	}
	return out
}

// parseRepeatGroup extracts an intra-segment X12RepeatDef's repeating
// reason/amount/... groups (e.g. CAS02-04, CAS05-07, ...) from the elements
// consumedPositions doesn't already own via a plain X12ElementDef. Also
// writes each field's raw value into rawByPos, keyed by its own ABSOLUTE
// position (e.g. "05", "06", "07" for trio 2) — without this, a SyntaxRule
// referencing positions that live entirely inside a repeat group (CAS's own
// L050607-style trio rules, the actual motivating real-world case for this
// mechanism) would never see any data, since parseSegmentInstance's own
// rawByPos only covers its plain Elements loop.
func (w *walker) parseRepeatGroup(t segmentToken, group *X12RepeatDef, consumedPositions map[int]bool, rawByPos map[string]string) []map[string]interface{} {
	var instances []map[string]interface{}
	for g := 0; g < group.MaxGroups; g++ {
		groupStart := group.StartPos + g*group.GroupSize
		instance := map[string]interface{}{}
		anyValue := false
		for _, field := range group.GroupFields {
			offset := atoiSafe(field.Pos) // 1-based offset within the group
			absolutePos := groupStart + offset - 1
			raw := elementAt(t, absolutePos)
			rawByPos[fmt.Sprintf("%02d", absolutePos)] = raw
			if raw != "" {
				anyValue = true
			}
			key := field.Key
			if key == "" {
				key = field.Pos
			}
			// A repeat group's own field can itself be composite — e.g. PLB's
			// adjustment-identifier field (industry code + reference ID) inside
			// its 6 repeating identifier/amount pairs. Mirrors the same check
			// parseSegmentInstance's plain-element path already does.
			if field.Composite != nil {
				instance[key] = w.parseComposite(raw, field.Composite)
			} else {
				instance[key] = raw
			}
		}
		if !anyValue {
			break // groups are consumed contiguously from the start; the first empty group ends the repeat
		}
		instances = append(instances, instance)
	}
	return instances
}

// loopTrigger returns the segment ID that starts loop — its own first
// SegmentIDs entry, or (for a loop with none of its own, composed purely of
// child loops) the first child loop's own trigger, recursively.
func loopTrigger(loop *X12LoopDef) string {
	if len(loop.SegmentIDs) > 0 {
		return loop.SegmentIDs[0]
	}
	for _, child := range loop.Loops {
		if t := loopTrigger(child); t != "" {
			return t
		}
	}
	return ""
}

// matchLoops repeatedly scans loops (in schema order) for one whose trigger
// matches the current token, processes exactly one instance of the winning
// loop, then restarts the scan from the top of loops — giving schema order
// priority among siblings and letting any of them repeat any number of
// times, in any interleaving the data actually presents. Stops when a full
// pass finds no match.
func (w *walker) matchLoops(pos *int, loops []*X12LoopDef, parentPath []pathStep) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for {
		matched := false
		for _, loop := range loops {
			if *pos >= len(w.tokens) {
				break
			}
			if w.tokens[*pos].ID != loopTrigger(loop) {
				continue
			}
			if !loop.RepeatsMultiple() {
				if _, already := out[loop.ID]; already {
					continue
				}
			}

			index := 0
			if loop.RepeatsMultiple() {
				index = countLoopInstances(out[loop.ID]) + 1
			}
			instance, err := w.matchLoopInstance(pos, loop, append(parentPath, pathStep{Name: loop.ID, Index: index}))
			if err != nil {
				return nil, err
			}

			if loop.RepeatsMultiple() {
				arr, _ := out[loop.ID].([]map[string]interface{})
				out[loop.ID] = append(arr, instance)
			} else {
				out[loop.ID] = instance
			}
			matched = true
			break
		}
		if !matched {
			break
		}
	}
	return out, nil
}

func countLoopInstances(v interface{}) int {
	arr, ok := v.([]map[string]interface{})
	if !ok {
		return 0
	}
	return len(arr)
}

// matchLoopInstance parses ONE occurrence of loop: its own segments, then
// its own child loops.
func (w *walker) matchLoopInstance(pos *int, loop *X12LoopDef, path []pathStep) (map[string]interface{}, error) {
	instance := map[string]interface{}{}
	if err := w.matchSegmentSequence(pos, loop.SegmentIDs, instance, path); err != nil {
		return nil, err
	}
	if len(loop.Loops) > 0 {
		childLoops, err := w.matchLoops(pos, loop.Loops, path)
		if err != nil {
			return nil, err
		}
		if len(childLoops) > 0 {
			instance["loops"] = childLoops
		}
	}
	if len(loop.TrailerSegmentIDs) > 0 {
		if err := w.matchSegmentSequence(pos, loop.TrailerSegmentIDs, instance, path); err != nil {
			return nil, err
		}
	}
	return instance, nil
}

// stepKey returns the identity key for step i within steps, scoped to the
// specific parent instance it occurred in (siblings under a DIFFERENT
// instance of a repeating ancestor get a different key, so a field that
// only repeats within ONE particular instance doesn't spuriously bracket
// every instance's own single occurrence — see renderFields' own doc
// comment). Also returns the prefix to use for the NEXT step's own key.
func stepKey(prefix string, s pathStep) (key, nextPrefix string) {
	key = prefix + "/" + s.Name
	nextPrefix = key
	if s.Index > 0 {
		nextPrefix += fmt.Sprintf("[%d]", s.Index)
	}
	return key, nextPrefix
}

// renderFields converts every pending field's step-based path into its
// final string form and returns the flat map keyed by that path. A step's
// "[n]" suffix is included only when that exact (parent-instance, name)
// grouping was observed more than once anywhere in the message — decided
// here, once, after the whole walk, per the plan's own addressing rule. Both
// passes below MUST build identical keys via the same stepKey helper, or the
// maxIndex lookups in the second pass won't align with what the first pass
// recorded.
func (w *walker) renderFields() map[string]*EDIField {
	maxIndex := map[string]int{}
	for _, p := range w.pending {
		prefix := ""
		for _, s := range p.Steps {
			var key string
			key, prefix = stepKey(prefix, s)
			if s.Index > maxIndex[key] {
				maxIndex[key] = s.Index
			}
		}
	}

	out := make(map[string]*EDIField, len(w.pending))
	for _, p := range w.pending {
		parts := make([]string, 0, len(p.Steps))
		prefix := ""
		for _, s := range p.Steps {
			var key string
			key, prefix = stepKey(prefix, s)
			segment := s.Name
			if s.Index > 0 && maxIndex[key] > 1 {
				segment = fmt.Sprintf("%s[%d]", s.Name, s.Index)
			}
			parts = append(parts, segment)
		}
		path := strings.Join(parts, ".") + "." + p.Key
		out[path] = &EDIField{
			Path: path, Value: p.Value, Key: p.Key, DataType: p.DataType,
			FixedValue: p.FixedValue, MinLength: p.MinLength, MaxLength: p.MaxLength,
			SegmentPosition: p.SegmentPosition,
		}
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
