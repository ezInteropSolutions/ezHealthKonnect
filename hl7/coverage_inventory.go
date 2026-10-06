// hl7/coverage_inventory.go
//
// Builds the "everything that was actually populated in this source HL7
// message" ground-truth list for Coverage Audit — pure, DB-free, and
// independent of which HL7→FHIR mapping mechanism a pipeline uses (fhir.build
// or hl7_fhir_transform). Mirrors CDA's own inventory.go in spirit (walk what
// the message really contains, don't fabricate entries for schema-defined
// fields the message never populated).
//
// Two granularities, mirroring CDA's own entry-level/element-level split
// (services/cda_coverage/inventory.go): field-level (the original design,
// "PID.5" as one unit) and, per-interface opt-in, component-level
// (BuildFieldPresenceWithGranularity's elementLevel=true — "PID.5.1",
// "PID.5.2", ... when the field is a real composite with its own populated
// subfields). This was a named, deferred gap until 2026-10-02: both halves
// of the underlying data already existed (FieldInfo.Subfields, each with its
// own Key in the identical "SEGMENT.FIELD.COMPONENT" shape
// resolveHL7FieldValue already understands) — only the walk itself needed to
// go one level deeper.
package hl7

// FieldPresence is one populated field (or, at component granularity, one
// populated component within a field) within one segment instance of a
// parsed HL7 message.
type FieldPresence struct {
	// SegmentName is the raw segment key (e.g. "PID", "OBX").
	SegmentName string
	// SegmentIndex is this segment's 0-based position among every instance of
	// its own type (EnhancedSegment.SegmentIndex) — 0 for a singular segment,
	// 0/1/2/... for a repeating one (OBX, DG1, NK1, IN1, ...).
	SegmentIndex int
	// FieldPath is the "SEGMENT.POSITION" key (e.g. "PID.3"), or, at
	// component granularity, "SEGMENT.POSITION.COMPONENT" (e.g. "PID.5.1") —
	// the same string shape resolveHL7FieldValue/getHL7FieldValue/
	// FieldMapping.HL7Field(+HL7Component) all use, so a coverage adapter can
	// compare directly with no translation.
	FieldPath string
	// Value is the field's (or component's) own raw string value —
	// structural location only, callers building a persisted report should
	// not surface this verbatim (matching every other format's
	// coverage-audit's own no-PHI discipline).
	Value string
}

// BuildFieldPresence is BuildFieldPresenceWithGranularity with elementLevel
// false — field-level only, the original behavior, unchanged.
func BuildFieldPresence(msg *EnhancedParsedMessage) []FieldPresence {
	return BuildFieldPresenceWithGranularity(msg, false)
}

// BuildFieldPresenceWithGranularity walks every segment instance actually
// present in msg (msg.SegmentGroups — ALL instances of every segment type,
// not just the last-instance-per-type msg.EnhancedSegments map) and returns
// one FieldPresence per field (or, with elementLevel, per component) that
// genuinely has a value in this specific message. A field position the
// schema defines but this message left empty is not included — mirroring
// CDA's inventory, which likewise never fabricates an item for content that
// isn't really there.
//
// When elementLevel is true and a field has one or more genuinely-populated
// subfields (field.Subfields[i].HasValue), the COMPONENT-level items replace
// the coarse field-level item for that field entirely — never both, which
// would double-count one real data point as two separate report entries
// (the field's own raw composite value and its already-decomposed parts are
// the same underlying fact, not two). A field with no subfields at all, or
// whose subfields none carry their own value even though the field itself
// does (e.g. a simple non-composite field with no component dictionary),
// falls back to the field-level item exactly as elementLevel=false always
// produces — this mirrors fhir_adapter.go's own "decompose or fall back,
// never both" design precedent for the identical reason.
func BuildFieldPresenceWithGranularity(msg *EnhancedParsedMessage, elementLevel bool) []FieldPresence {
	if msg == nil {
		return nil
	}
	var items []FieldPresence
	for segmentName, instances := range msg.SegmentGroups {
		for _, seg := range instances {
			for _, field := range seg.Fields {
				if !field.HasValue {
					continue
				}
				if elementLevel {
					var subItems []FieldPresence
					for _, sf := range field.Subfields {
						if !sf.HasValue {
							continue
						}
						subItems = append(subItems, FieldPresence{
							SegmentName:  segmentName,
							SegmentIndex: seg.SegmentIndex,
							FieldPath:    sf.Key,
							Value:        sf.Value,
						})
					}
					if len(subItems) > 0 {
						items = append(items, subItems...)
						continue
					}
				}
				items = append(items, FieldPresence{
					SegmentName:  segmentName,
					SegmentIndex: seg.SegmentIndex,
					FieldPath:    field.Key,
					Value:        field.Value,
				})
			}
		}
	}
	return items
}
