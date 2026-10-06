// astm/validator/validator.go
// Schema-driven, spec-verification-independent validation of an already-
// parsed astm.ParseResult — mirrors edi/validator/validator.go's own real
// behavior exactly (not just its doc-comment framing): re-check every
// recorded field's VALUE against its own declared data type (ERROR
// severity, since a malformed value would likely break downstream
// conversion too), and nothing else. Record presence/structure
// (H.../L required, fixed message shape) is already enforced structurally
// by astm.ParseMessage itself — a malformed shape there is a PARSE error, so
// a ParseResult wouldn't exist to validate at all. Field-level Required
// flags are documentation only in phase 1, same "flexible, not rigid"
// philosophy this project applies to EDI X12/NCPDP — a missing optional-ish
// field never blocks Result.Valid.
//
// Checksum validation is deliberately NOT performed here — it's a frame-
// level transport concept (computed over the raw STX..ETX byte range, which
// no longer exists by the time content reaches ParseMessage), owned by
// services/connectors/astm_framing.go instead. See this feature's own plan
// doc for the reasoning.
package validator

import (
	"ezhealthkonnect/astm"
)

// Issue is one validation finding.
type Issue struct {
	Severity string `json:"severity"` // "error" | "warning"
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// Result is the outcome of validating one parsed message. Valid is false
// only when at least one ERROR-severity Issue exists.
type Result struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues,omitempty"`
}

// Validate re-checks every field astm.ParseMessage extracted against its own
// recorded data type.
func Validate(spec *astm.ASTMSpecDef, result *astm.ParseResult) *Result {
	r := &Result{Valid: true}
	if result == nil {
		return r
	}

	for _, field := range result.Fields {
		if field.Value == "" {
			continue // absent field — nothing to check
		}
		if field.DataType == "" {
			// A component or repeat field's OWN parent entry (e.g.
			// "header.5" for senderNameID) has no scalar DataType at this
			// flat-Fields layer by design — its raw value is a "^"-joined
			// string, not a single scalar value any ASTMDataType could
			// validate. This is never a silent gap: decodeComponent/
			// decodeRepeat (record_engine.go) separately register each
			// REAL sub-field/item (e.g. "header.5.manufacturer") as its own
			// Fields entry with its own real DataType, which this same loop
			// checks on a later iteration. Skipping the parent entry here
			// just avoids double-reporting / misreporting the composite
			// wrapper itself as "unknown ASTM data type".
			continue
		}
		if err := astm.Validate(astm.ASTMDataType(field.DataType), field.Value); err != nil {
			r.Valid = false
			r.Issues = append(r.Issues, Issue{Severity: "error", Path: field.Path, Message: err.Error()})
		}
	}

	return r
}
