// Package validator provides HL7 v2 conformance validation on top of the
// already-parsed output of hl7.ParseWithRealSchema, reusing the schema-tree
// helpers in hl7/builder (RequiredSpine, SchemaTree) that already compute
// which segments are truly required and which can legitimately repeat.
//
// This lives in its own package rather than inside package hl7 because
// hl7/builder imports hl7 — importing hl7/builder from inside package hl7
// itself would create an import cycle (hl7 -> hl7/builder -> hl7). Callers
// resolve a schema via hl7.ResolveSchemaForMessage and pass it in here,
// mirroring how fhir/r4 and cda/validator are invoked by callers outside
// their own parsers, never from within the parser itself.
package validator

import (
	"strings"

	"ezhealthkonnect/hl7"
)

// ValidationLevel selects how much of the conformance check runs.
type ValidationLevel int

const (
	// LevelBasic runs none of this package's checks — callers stay on
	// today's existing required-field-only validation
	// (hl7.validateRequiredFields, run unconditionally by ParseWithRealSchema).
	LevelBasic ValidationLevel = iota
	// LevelStandard runs all four categories. Missing-segment and
	// cardinality violations are ERROR; data-type-format and
	// table-binding issues are WARNING.
	LevelStandard
	// LevelStrict runs the same four categories as Standard, but
	// data-type-format and table-binding issues escalate to ERROR.
	LevelStrict
)

// ParseLevel maps a request-facing string ("basic"/"standard"/"strict",
// case-insensitive) to a ValidationLevel, defaulting to LevelBasic for any
// unrecognized or empty value so an unset request never accidentally runs
// the more expensive tiers.
func ParseLevel(s string) ValidationLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return LevelStrict
	case "standard":
		return LevelStandard
	default:
		return LevelBasic
	}
}

// ValidationOptions controls one ValidateMessage call.
type ValidationOptions struct {
	Level ValidationLevel
}

// ConformanceResult is the flat, severity-partitioned view of a validation
// run (mirroring fhir/r4's ResourceResult{Valid,Errors,Warnings}), alongside
// Detail — the category-partitioned view matching hl7.SchemaValidationResult
// for callers that want e.g. "just the table-binding errors".
type ConformanceResult struct {
	Valid    bool
	Errors   []hl7.ValidationError
	Warnings []hl7.ValidationError
	Info     []hl7.ValidationError
	Detail   hl7.SchemaValidationResult
}

// All flattens every category back into one array, in a stable order
// (segments -> cardinality -> data types -> table bindings) — this is what
// gets appended onto EnhancedParsedMessage.ValidationErrors for callers that
// only want the existing flat-array shape segment-viewer.js already renders.
func (r ConformanceResult) All() []hl7.ValidationError {
	out := make([]hl7.ValidationError, 0,
		len(r.Detail.SegmentErrors)+len(r.Detail.CardinalityErrors)+
			len(r.Detail.DataTypeErrors)+len(r.Detail.TableValidationErrors))
	out = append(out, r.Detail.SegmentErrors...)
	out = append(out, r.Detail.CardinalityErrors...)
	out = append(out, r.Detail.DataTypeErrors...)
	out = append(out, r.Detail.TableValidationErrors...)
	return out
}

// ValidateMessage runs HL7 v2 conformance validation for one already-parsed
// message against its resolved schema. At LevelBasic (or when schema/msg is
// nil) it's a no-op — the caller's existing required-field validation is
// left as the only signal, unchanged.
func ValidateMessage(schema *hl7.RealHL7Schema, msg *hl7.EnhancedParsedMessage, opts ValidationOptions) ConformanceResult {
	result := ConformanceResult{Valid: true}
	if opts.Level == LevelBasic || schema == nil || msg == nil {
		return result
	}

	detail := hl7.SchemaValidationResult{
		SchemaUsed: schema.MessageType + " v" + schema.Version,
	}
	detail.SegmentErrors = validateRequiredSegments(schema, msg)
	detail.CardinalityErrors = validateCardinality(schema, msg)
	detail.DataTypeErrors = validateDataTypes(msg, opts.Level)
	detail.TableValidationErrors = validateTableBindings(schema, msg, opts.Level)
	result.Detail = detail

	for _, e := range result.All() {
		switch e.Severity {
		case hl7.SeverityError:
			result.Errors = append(result.Errors, e)
		case hl7.SeverityWarning:
			result.Warnings = append(result.Warnings, e)
		default:
			result.Info = append(result.Info, e)
		}
	}
	result.Valid = len(result.Errors) == 0
	result.Detail.Valid = result.Valid

	return result
}
