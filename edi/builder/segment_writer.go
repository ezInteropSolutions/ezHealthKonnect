// edi/builder/segment_writer.go
// buildWalker holds the write-direction schema walk shared by
// document_builder.go's ISA/GS/GE/IEA envelope writer and the generic
// segment/loop writers used for the ST...SE body — the write-direction
// mirror of edi/loop_engine.go's walker.
package builder

import (
	"fmt"
	"strconv"
	"strings"

	"ezhealthkonnect/edi"
)

type buildWalker struct {
	spec       *edi.X12SpecDef
	delimiters delimiters
}

// writeEnvelopeSegment builds ISA or GS from element defs + a flat values
// map keyed by element Key. Envelope elements with a MaxLength are fixed-
// width per the X12 standard's own ISA/GS structure — padded with spaces or
// truncated to exactly that width, unlike body elements which are simply
// trimmed of trailing empties.
func (w *buildWalker) writeEnvelopeSegment(tag string, elements []*edi.X12ElementDef, values map[string]interface{}) string {
	parts := make([]string, len(elements))
	for i, el := range elements {
		raw := valueForElement(el, values)
		if el.MaxLength > 0 {
			raw = padOrTruncate(raw, el.MaxLength)
		}
		parts[i] = raw
	}
	return tag + w.delimiters.Element + strings.Join(parts, w.delimiters.Element)
}

// writeElements builds one segment from an explicit element-def list and a
// flat values map — used directly for the ad hoc ST/SE/GE/IEA control
// segments in document_builder.go, and by writeSegmentInstance for a
// schema-defined segment's own Elements.
func (w *buildWalker) writeElements(segID string, elements []*edi.X12ElementDef, values map[string]interface{}) string {
	parts := make([]string, len(elements))
	for i, el := range elements {
		parts[i] = valueForElement(el, values)
	}
	return segID + w.delimiters.Element + strings.Join(parts, w.delimiters.Element)
}

func valueForElement(el *edi.X12ElementDef, values map[string]interface{}) string {
	key := el.Key
	if key == "" {
		key = el.Pos
	}
	if v, ok := values[key]; ok {
		if s := fmt.Sprint(v); s != "" {
			return s
		}
	}
	return el.FixedValue
}

func padOrTruncate(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}

// writeSegmentSequence builds one segment per entry in segmentIDs whose data
// is present in data (keyed by segment ID) — required segments missing from
// data are an error, situational ones are simply skipped.
func (w *buildWalker) writeSegmentSequence(segmentIDs []string, data map[string]interface{}) ([]string, error) {
	var out []string
	for _, segID := range segmentIDs {
		segDef, ok := w.spec.Segments[segID]
		if !ok {
			return nil, fmt.Errorf("segment %q not found in shared library", segID)
		}

		value, present := data[segID]
		if !present {
			if segDef.IsRequired() {
				return nil, fmt.Errorf("required segment %q missing from canonical data", segID)
			}
			continue
		}

		if segDef.RepeatsMultiple() {
			for _, inst := range toInterfaceSlice(value) {
				instMap, ok := inst.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("segment %q repeat instance must be an object", segID)
				}
				out = append(out, w.writeSegmentInstance(segID, segDef, instMap))
			}
		} else {
			instMap, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("segment %q data must be an object", segID)
			}
			out = append(out, w.writeSegmentInstance(segID, segDef, instMap))
		}
	}
	return out, nil
}

// writeSegmentInstance builds one segment occurrence's raw text from its
// schema Elements (plain or composite) and any intra-segment X12RepeatDef
// groups (e.g. CAS), positioned generically by element position — no
// per-segment-ID code.
func (w *buildWalker) writeSegmentInstance(segID string, segDef *edi.X12SegmentDef, data map[string]interface{}) string {
	maxPos := 0
	for _, el := range segDef.Elements {
		if p := atoiSafe(el.Pos); p > maxPos {
			maxPos = p
		}
	}
	for _, group := range segDef.Repeats {
		if endPos := group.StartPos + group.MaxGroups*group.GroupSize - 1; endPos > maxPos {
			maxPos = endPos
		}
	}

	positions := make([]string, maxPos)

	for _, el := range segDef.Elements {
		key := el.Key
		if key == "" {
			key = el.Pos
		}

		var raw string
		if el.Composite != nil {
			if sub, ok := data[key].(map[string]interface{}); ok {
				raw = w.writeComposite(el.Composite, sub)
			}
		} else {
			raw = valueForElement(el, data)
			if raw == "" {
				if v, ok := data["_default_"+el.Pos]; ok { // ST's synthesized-when-missing defaults, see BuildDocument
					raw = fmt.Sprint(v)
				}
			}
		}

		if pos := atoiSafe(el.Pos); pos >= 1 && pos <= len(positions) {
			positions[pos-1] = raw
		}
	}

	for _, group := range segDef.Repeats {
		instances := toInterfaceSlice(data[group.Key])
		for i, inst := range instances {
			if i >= group.MaxGroups {
				break
			}
			instMap, _ := inst.(map[string]interface{})
			groupStart := group.StartPos + i*group.GroupSize
			for _, field := range group.GroupFields {
				fieldKey := field.Key
				if fieldKey == "" {
					fieldKey = field.Pos
				}
				var raw string
				// A repeat group's own field can itself be composite (e.g.
				// PLB's adjustment-identifier field) — mirrors the same check
				// writeSegmentInstance's plain-element path already does.
				if field.Composite != nil {
					if sub, ok := instMap[fieldKey].(map[string]interface{}); ok {
						raw = w.writeComposite(field.Composite, sub)
					}
				} else if v, ok := instMap[fieldKey]; ok {
					raw = fmt.Sprint(v)
				}
				if pos := groupStart + atoiSafe(field.Pos) - 1; pos >= 1 && pos <= len(positions) {
					positions[pos-1] = raw
				}
			}
		}
	}

	positions = trimTrailingEmpty(positions)
	return segID + w.delimiters.Element + strings.Join(positions, w.delimiters.Element)
}

func (w *buildWalker) writeComposite(def *edi.X12CompositeDef, data map[string]interface{}) string {
	parts := make([]string, len(def.SubElements))
	for i, sub := range def.SubElements {
		key := sub.Key
		if key == "" {
			key = sub.Pos
		}
		if v, ok := data[key]; ok {
			parts[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(trimTrailingEmpty(parts), w.delimiters.SubElement)
}

// writeLoops builds every loop in loops whose data is present in data
// (keyed by loop ID) — the write-direction mirror of edi/loop_engine.go's
// matchLoops. Unlike the read direction, there is no trigger-matching
// ambiguity to resolve here: the canonical data itself already names which
// loop each entry belongs to.
func (w *buildWalker) writeLoops(loops []*edi.X12LoopDef, data map[string]interface{}) ([]string, error) {
	var out []string
	for _, loop := range loops {
		value, present := data[loop.ID]
		if !present {
			continue
		}

		if loop.RepeatsMultiple() {
			for _, inst := range toInterfaceSlice(value) {
				instMap, ok := inst.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("loop %q repeat instance must be an object", loop.ID)
				}
				segs, err := w.writeLoopInstance(loop, instMap)
				if err != nil {
					return nil, err
				}
				out = append(out, segs...)
			}
		} else {
			instMap, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("loop %q data must be an object", loop.ID)
			}
			segs, err := w.writeLoopInstance(loop, instMap)
			if err != nil {
				return nil, err
			}
			out = append(out, segs...)
		}
	}
	return out, nil
}

func (w *buildWalker) writeLoopInstance(loop *edi.X12LoopDef, data map[string]interface{}) ([]string, error) {
	segs, err := w.writeSegmentSequence(loop.SegmentIDs, data)
	if err != nil {
		return nil, err
	}
	if len(loop.Loops) > 0 {
		if childData, ok := data["loops"].(map[string]interface{}); ok {
			childSegs, err := w.writeLoops(loop.Loops, childData)
			if err != nil {
				return nil, err
			}
			segs = append(segs, childSegs...)
		}
	}
	if len(loop.TrailerSegmentIDs) > 0 {
		// Same source map as SegmentIDs above (not childData) — a loop's own
		// trailer segments are keyed by segment ID at the SAME level as its
		// own leading segments, just written after Loops. See
		// X12LoopDef.TrailerSegmentIDs' own doc comment.
		trailerSegs, err := w.writeSegmentSequence(loop.TrailerSegmentIDs, data)
		if err != nil {
			return nil, err
		}
		segs = append(segs, trailerSegs...)
	}
	return segs, nil
}

// toInterfaceSlice accepts both []interface{} (the shape JSON-decoded
// pipeline data always produces) and []map[string]interface{} (the shape
// edi.ParseResult produces natively, before any JSON round trip) uniformly.
func toInterfaceSlice(v interface{}) []interface{} {
	switch arr := v.(type) {
	case []interface{}:
		return arr
	case []map[string]interface{}:
		out := make([]interface{}, len(arr))
		for i, m := range arr {
			out[i] = m
		}
		return out
	}
	return nil
}

func trimTrailingEmpty(parts []string) []string {
	end := len(parts)
	for end > 0 && parts[end-1] == "" {
		end--
	}
	return parts[:end]
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
