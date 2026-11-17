package validator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"ezhealthkonnect/hl7"
)

// tableSeverity mirrors dataTypeSeverity: table-binding issues are advisory
// (WARNING) at Standard, escalating to ERROR only at Strict.
func tableSeverity(level ValidationLevel) string {
	if level == LevelStrict {
		return hl7.SeverityError
	}
	return hl7.SeverityWarning
}

// validateTableBindings checks coded field/component values against the
// schema's own code->display table (RealFieldDef.Values /
// RealComponentDef.Values), joining by Key against the already-parsed field
// values — the same join pattern hl7.validateRequiredFields already uses.
// Works directly against RealHL7Schema + EnhancedParsedMessage rather than
// the flattened FieldInfo/SubfieldInfo API types, since the schema is
// already available in this scope and adding the full code table to every
// coded field in every /api/hl7/parse response would bloat the payload for
// no benefit.
func validateTableBindings(schema *hl7.RealHL7Schema, msg *hl7.EnhancedParsedMessage, level ValidationLevel) []hl7.ValidationError {
	sev := tableSeverity(level)
	var errs []hl7.ValidationError

	for segName, instances := range msg.SegmentGroups {
		segDef, ok := schema.Segments[segName]
		if !ok {
			continue
		}
		for _, seg := range instances {
			for _, f := range seg.Fields {
				fieldDef, ok := segDef.Fields[f.Key]
				if !ok {
					continue
				}
				// A coded field with components (CE, CWE, CNE, ...) is bound
				// by its code, the first component, in each repetition.
				for _, code := range tableCodes(f.Value, len(fieldDef.Components) > 0) {
					if e := checkTableBinding(segName, f.Key, "", fieldDef.TableId, fieldDef.Values, code, f.HasValue, sev); e != nil {
						errs = append(errs, *e)
						break
					}
				}
				for _, sf := range f.Subfields {
					compDef, ok := fieldDef.Components[sf.Key]
					if !ok {
						continue
					}
					if e := checkTableBinding(segName, f.Key, sf.Key, compDef.TableId, compDef.Values, sf.Value, sf.HasValue, sev); e != nil {
						errs = append(errs, *e)
					}
				}
			}
		}
	}

	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })
	return errs
}

// tableCodes splits a field's encoded value into the codes a table binds:
// one per repetition, and only the first component of a composite field.
func tableCodes(value string, composite bool) []string {
	reps := strings.Split(value, "~")
	if !composite {
		return reps
	}
	codes := make([]string, len(reps))
	for i, r := range reps {
		codes[i], _, _ = strings.Cut(r, "^")
	}
	return codes
}

// tableCodePatterns are codes a table defines by pattern rather than by
// listing them. Table 0396 (coding system) has "HL7nnnn" for any HL7 table
// and "99zzz" for a local coding system.
var tableCodePatterns = map[string]*regexp.Regexp{
	"0396": regexp.MustCompile(`^(HL7\d{4}|99[A-Za-z0-9]{3})$`),
}

// checkTableBinding reports value when tableId names a table with a known,
// non-empty code set and value isn't one of its keys. A TableId with an
// empty Values map is a confirmed-legitimate user-defined table (e.g. HL7
// table 0010, Physician ID) that has no fixed code set in the standard —
// silently skipped, never reported.
func checkTableBinding(segment, fieldKey, componentKey, tableId string, values map[string]string, value string, hasValue bool, severity string) *hl7.ValidationError {
	if tableId == "" || len(values) == 0 || !hasValue || value == "" {
		return nil
	}
	if _, ok := values[value]; ok {
		return nil
	}
	if p := tableCodePatterns[tableId]; p != nil && p.MatchString(value) {
		return nil
	}
	key := fieldKey
	if componentKey != "" {
		key = componentKey
	}
	return &hl7.ValidationError{
		Severity:   severity,
		Code:       hl7.ErrorCodeTableValueInvalid,
		Message:    fmt.Sprintf("%s: value %q is not a recognized code in HL7 table %s", key, value, tableId),
		Segment:    segment,
		Field:      fieldKey,
		Component:  componentKey,
		Suggestion: fmt.Sprintf("Use a valid code from HL7 table %s", tableId),
		RuleId:     "TBL_" + key,
	}
}
