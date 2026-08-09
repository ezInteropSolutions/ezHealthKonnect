package validator

import (
	"fmt"
	"sort"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/hl7/builder"
)

// canRepeatMap flattens builder.SchemaTree(schema) to a per-segment-name
// CanRepeat lookup. CanRepeat is intentionally not derived from
// RealSegmentDef.Repeat/MaxUse directly — MaxUse is never populated by real
// compiled schemas, and a segment's own Repeat is frequently "1" even though
// it legitimately repeats via its enclosing group (e.g. OBX's own repeat is
// "1"; it repeats because its parent OBSERVATION group's repeat is "*").
// SchemaTree's CanRepeat already accounts for this correctly.
func canRepeatMap(schema *hl7.RealHL7Schema) map[string]bool {
	out := make(map[string]bool)
	var walk func(nodes []*builder.SchemaTreeInfo)
	walk = func(nodes []*builder.SchemaTreeInfo) {
		for _, n := range nodes {
			if n.Kind == "segment" && n.CanRepeat {
				out[n.Name] = true
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(builder.SchemaTree(schema))
	return out
}

// validateCardinality flags a segment appearing more than once in the
// message when the schema doesn't allow it to repeat anywhere it occurs.
// Segments not defined in schema at all (Z-segments) are silently skipped —
// there's no schema data to judge their cardinality against.
func validateCardinality(schema *hl7.RealHL7Schema, msg *hl7.EnhancedParsedMessage) []hl7.ValidationError {
	canRepeat := canRepeatMap(schema)

	var errs []hl7.ValidationError
	for name, instances := range msg.SegmentGroups {
		if name == "MSH" {
			continue
		}
		if _, knownToSchema := schema.Segments[name]; !knownToSchema {
			continue
		}
		if len(instances) > 1 && !canRepeat[name] {
			errs = append(errs, hl7.ValidationError{
				Severity:   hl7.SeverityError,
				Code:       hl7.ErrorCodeCardinalityViolation,
				Message:    fmt.Sprintf("Segment %s appears %d times but this message type does not allow it to repeat", name, len(instances)),
				Segment:    name,
				Suggestion: fmt.Sprintf("Remove the extra %s occurrences, or confirm this message type is expected to repeat %s", name, name),
				RuleId:     "CARD_" + name,
			})
		}
	}

	sort.Slice(errs, func(i, j int) bool { return errs[i].Segment < errs[j].Segment })
	return errs
}
