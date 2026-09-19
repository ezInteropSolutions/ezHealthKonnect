// ncpdptelecom/validator/validator.go
//
// Validate checks a ncpdptelecom.ParseResult against the schema's own
// Required flags — mirroring ncpdp/validator's own dated (2026-09-01)
// "flexible, not rigid" two-severity product decision (see that package's
// doc comment for the full rationale): a missing/empty REQUIRED field or
// segment is a blocking error; the warning tier is left an intentionally-
// empty, ready-to-extend mechanism this phase — no free source of D.0's own
// conditional-requirement business rules was found (the same honest gap
// already named for NCPDP SCRIPT), so none are fabricated here either.
package validator

import (
	"fmt"

	"ezhealthkonnect/ncpdptelecom"
)

// Severity is either "error" (blocks Result.Valid) or "warning" (never
// does).
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one validation finding.
type Issue struct {
	Severity Severity
	Path     string
	Message  string
}

// Result is the outcome of validating one parsed transmission.
type Result struct {
	Valid  bool
	Issues []Issue
}

// Validate checks result's header, transmission group, and every
// transaction group against spec's own segment/field Required flags for the
// resolved transaction (code + direction).
func Validate(spec *ncpdptelecom.TelecomSpecDef, result *ncpdptelecom.ParseResult) *Result {
	var issues []Issue

	for _, f := range spec.HeaderFields {
		if !f.Required {
			continue
		}
		v, present := result.Header[f.Key]
		if !present || isEmptyValue(v) {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Path:     "Header." + f.Key,
				Message:  fmt.Sprintf("required header field %q is missing or empty", f.Key),
			})
		}
	}

	tx := spec.Transactions[ncpdptelecom.TransactionKey(result.TransactionCode, result.Direction)]
	if tx == nil {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Path:     "",
			Message:  fmt.Sprintf("unknown transaction %q direction %q", result.TransactionCode, result.Direction),
		})
		return finalize(issues)
	}

	for _, segKey := range tx.TransmissionGroupSegments {
		segDef := spec.Segments[segKey]
		if segDef == nil {
			continue
		}
		raw, present := result.TransmissionGroup[segKey]
		data, _ := raw.(map[string]interface{})
		issues = append(issues, validateSegmentPresence("TransmissionGroup", segKey, segDef, data, present)...)
	}

	if len(result.TransactionGroups) == 0 {
		for _, segKey := range tx.TransactionGroupSegments {
			segDef := spec.Segments[segKey]
			if segDef != nil && segmentHasRequiredFields(segDef) {
				issues = append(issues, Issue{
					Severity: SeverityError,
					Path:     "TransactionGroups",
					Message:  fmt.Sprintf("no transaction groups present, but segment %q is required", segKey),
				})
			}
		}
	}
	for i, rawCluster := range result.TransactionGroups {
		cluster, _ := rawCluster.(map[string]interface{})
		path := fmt.Sprintf("TransactionGroups[%d]", i)
		for _, segKey := range tx.TransactionGroupSegments {
			segDef := spec.Segments[segKey]
			if segDef == nil {
				continue
			}
			raw, present := cluster[segKey]
			data, _ := raw.(map[string]interface{})
			issues = append(issues, validateSegmentPresence(path, segKey, segDef, data, present)...)
		}
	}

	return finalize(issues)
}

func validateSegmentPresence(path, segKey string, segDef *ncpdptelecom.TelecomSegmentDef, data map[string]interface{}, present bool) []Issue {
	if !present {
		if segmentHasRequiredFields(segDef) {
			return []Issue{{
				Severity: SeverityError,
				Path:     path + "." + segKey,
				Message:  fmt.Sprintf("required segment %q is missing", segKey),
			}}
		}
		return nil
	}
	return validateFields(path+"."+segKey, segDef.Fields, data)
}

func validateFields(path string, fields []ncpdptelecom.TelecomFieldDef, data map[string]interface{}) []Issue {
	var issues []Issue
	for _, f := range fields {
		if !f.Required {
			continue
		}
		v, present := data[f.Key]
		if !present || isEmptyValue(v) {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Path:     path + "." + f.Key,
				Message:  fmt.Sprintf("required field %q (%s) is missing or empty", f.Key, f.FieldID),
			})
		}
	}
	return issues
}

func segmentHasRequiredFields(segDef *ncpdptelecom.TelecomSegmentDef) bool {
	if segDef == nil {
		return false
	}
	for _, f := range segDef.Fields {
		if f.Required {
			return true
		}
	}
	return false
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return s == ""
	}
	return false
}

func finalize(issues []Issue) *Result {
	valid := true
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			valid = false
			break
		}
	}
	return &Result{Valid: valid, Issues: issues}
}
