package validator

import (
	"fmt"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/hl7/builder"
)

// validateRequiredSegments flags every segment builder.RequiredSpine says
// must appear (i.e. required down the FULL transitive ancestor chain — a
// segment nested inside an optional group, like ORU_R01's PID inside the
// optional PATIENT group, is correctly NOT flagged) but that has zero
// instances in msg.SegmentGroups. MSH is excluded by RequiredSpine itself.
func validateRequiredSegments(schema *hl7.RealHL7Schema, msg *hl7.EnhancedParsedMessage) []hl7.ValidationError {
	var errs []hl7.ValidationError
	for _, name := range builder.RequiredSpine(schema) {
		if len(msg.SegmentGroups[name]) > 0 {
			continue
		}
		errs = append(errs, hl7.ValidationError{
			Severity:   hl7.SeverityError,
			Code:       hl7.ErrorCodeMissingRequiredSegment,
			Message:    fmt.Sprintf("Required segment %s is missing from the message", name),
			Segment:    name,
			Suggestion: fmt.Sprintf("Add a %s segment to the message", name),
			RuleId:     "REQ_SEG_" + name,
		})
	}
	return errs
}
