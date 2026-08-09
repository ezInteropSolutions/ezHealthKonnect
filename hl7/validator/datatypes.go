package validator

import (
	"fmt"
	"regexp"
	"sort"

	"ezhealthkonnect/hl7"
)

var (
	reNM = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)
	reSI = regexp.MustCompile(`^\d+$`)
	reDT = regexp.MustCompile(`^\d{4}(\d{2}(\d{2})?)?$`)
	reTM = regexp.MustCompile(`^\d{2}(\d{2}(\d{2}(\.\d{1,4})?)?)?([+-]\d{4})?$`)
	reTS = regexp.MustCompile(`^\d{4}(\d{2}(\d{2}(\d{2}(\d{2}(\d{2}(\.\d{1,4})?)?)?)?)?)?([+-]\d{4})?$`)
)

// dataTypeSeverity: format issues are advisory (WARNING) at Standard, and
// escalate to ERROR only at Strict — HL7 v2 has no equivalent of FHIR's
// BindingStrength to say which fields are safe to hard-fail on by default.
func dataTypeSeverity(level ValidationLevel) string {
	if level == LevelStrict {
		return hl7.SeverityError
	}
	return hl7.SeverityWarning
}

// validateDataTypes checks field/component values against a small set of
// common HL7 base data type formats. Runs over every instance in
// msg.SegmentGroups (not just the deduplicated EnhancedSegments map) so a
// bad value in the 2nd+ repeat of a segment (e.g. the second OBX) isn't
// silently dropped.
func validateDataTypes(msg *hl7.EnhancedParsedMessage, level ValidationLevel) []hl7.ValidationError {
	sev := dataTypeSeverity(level)
	var errs []hl7.ValidationError

	for segName, instances := range msg.SegmentGroups {
		for _, seg := range instances {
			for _, f := range seg.Fields {
				errs = append(errs, checkDataTypeFormat(segName, f.Key, "", f.DataType, f.Value, f.HasValue, f.Length, sev)...)
				for _, sf := range f.Subfields {
					errs = append(errs, checkDataTypeFormat(segName, f.Key, sf.Key, sf.DataType, sf.Value, sf.HasValue, sf.Length, sev)...)
				}
			}
		}
	}

	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })
	return errs
}

// checkDataTypeFormat validates one field or component value against its
// own data type's format. componentKey is "" for a field-level check, or the
// "SEGMENT.FIELD.COMPONENT" key for a component-level check nested under
// fieldKey. ID/IS are deliberately skipped here — their real "format" is
// table membership, owned entirely by validateTableBindings, so checking
// them here too would double-report the same bad value.
func checkDataTypeFormat(segment, fieldKey, componentKey, dataType, value string, hasValue bool, length int, severity string) []hl7.ValidationError {
	if !hasValue || value == "" {
		return nil
	}

	var reason string
	switch dataType {
	case hl7.DataTypeNM:
		if !reNM.MatchString(value) {
			reason = "is not a valid numeric (NM) value"
		}
	case hl7.DataTypeSI:
		if !reSI.MatchString(value) {
			reason = "is not a valid sequence ID (SI) value"
		}
	case hl7.DataTypeDT:
		if !reDT.MatchString(value) {
			reason = "is not a valid date (DT) value (expected YYYY, YYYYMM, or YYYYMMDD)"
		}
	case hl7.DataTypeTM:
		if !reTM.MatchString(value) {
			reason = "is not a valid time (TM) value"
		}
	case hl7.DataTypeTS:
		if !reTS.MatchString(value) {
			reason = "is not a valid timestamp (TS) value"
		}
	case hl7.DataTypeST, "TX", "FT":
		if length > 0 && len(value) > length {
			reason = fmt.Sprintf("exceeds the maximum length of %d characters", length)
		}
	case hl7.DataTypeID, hl7.DataTypeIS:
		// Table-membership is checked by validateTableBindings, not here.
	}

	if reason == "" {
		return nil
	}
	key := fieldKey
	if componentKey != "" {
		key = componentKey
	}
	return []hl7.ValidationError{{
		Severity:   severity,
		Code:       hl7.ErrorCodeInvalidDataType,
		Message:    fmt.Sprintf("%s: value %q %s", key, value, reason),
		Segment:    segment,
		Field:      fieldKey,
		Component:  componentKey,
		Suggestion: fmt.Sprintf("Correct %s to a valid %s value", key, dataType),
		RuleId:     "DT_" + key,
	}}
}
