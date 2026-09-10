// edi/validator/validator.go
// Schema-driven, spec-verification-independent validation of an already-
// parsed edi.ParseResult. Segment presence/usage (required-but-missing) is
// already enforced structurally by edi.ParseTransactionSet itself — a
// required segment missing there is a PARSE error, so a ParseResult
// wouldn't exist to validate at all. This validator's own job is the
// permissive-parse's counterpart: parsing deliberately extracts raw values
// without checking them (so a malformed message still parses far enough to
// be inspected), and Validate is the explicit, separate strict pass.
//
// Two distinct severities, on purpose (2026-09-01 product decision — see
// edi.SyntaxRule's own doc comment): a genuinely malformed VALUE (wrong data
// type, wrong length, a fixed-value mismatch) is an ERROR — that's broken
// data that would likely break FHIR conversion downstream too, and it makes
// Result.Valid false. A SyntaxRule violation (an element-relational
// constraint like "if BPR06 is present, BPR07 must be too") is a WARNING —
// informational only, NEVER makes Result.Valid false, and a pipeline is
// always free to ignore it. This project's own stated goal is to let users
// parse/build/convert without being too rigid; only genuinely broken data
// blocks anything here.
package validator

import (
	"fmt"
	"sort"

	"ezhealthkonnect/edi"
)

// Issue is one validation finding.
type Issue struct {
	Severity string `json:"severity"` // "error" | "warning"
	Path     string `json:"path"`
	Message  string `json:"message"`
	// Source is copied straight from the triggering edi.SyntaxRule's own
	// Source field (empty/omitted = an OOB spec rule; "custom" = a step-
	// config-supplied rule, see edi.SyntaxRule's own doc comment) -- this
	// package stays spec-driven only, it never decides "custom" itself.
	Source string `json:"source,omitempty"`
}

// Result is the outcome of validating one parsed transaction set. Valid is
// false only when at least one ERROR-severity Issue exists — warnings never
// affect it.
type Result struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues,omitempty"`
}

// Validate re-checks every field edi.ParseTransactionSet extracted against
// its own recorded data type, length bounds, and (when the schema declares
// one) fixed value — all ERROR severity — then checks every recorded
// SegmentInstance against its segment's own SyntaxRules — all WARNING
// severity, never blocking.
func Validate(spec *edi.X12SpecDef, result *edi.ParseResult) *Result {
	r := &Result{Valid: true}

	for _, field := range result.Fields {
		if field.Value == "" {
			continue // absent situational element — already excluded from output at parse time
		}

		if err := edi.Validate(edi.X12DataType(field.DataType), field.Value, field.MinLength, field.MaxLength); err != nil {
			r.Valid = false
			r.Issues = append(r.Issues, Issue{Severity: "error", Path: field.Path, Message: err.Error()})
		}

		if field.FixedValue != "" && field.Value != field.FixedValue {
			r.Valid = false
			r.Issues = append(r.Issues, Issue{
				Severity: "error",
				Path:     field.Path,
				Message:  fmt.Sprintf("expected fixed value %q, got %q", field.FixedValue, field.Value),
			})
		}
	}

	// checkEnvelopeIdentity's issues are always error-severity by
	// construction (see its own doc comment) — unlike checkSyntaxRules'
	// (always warning), so unlike that append below, this one must also
	// flip Valid.
	if envelopeIssues := checkEnvelopeIdentity(spec, result); len(envelopeIssues) > 0 {
		r.Valid = false
		r.Issues = append(r.Issues, envelopeIssues...)
	}

	r.Issues = append(r.Issues, checkSyntaxRules(spec, result)...)
	return r
}

// checkEnvelopeIdentity re-checks GS01 (functionalIdentifierCode) and GS08
// (versionReleaseIndustryCode) against the transaction set ParseTransactionSet
// actually resolved — the read-direction counterpart to Fix 1's move of
// these two values off the shared envelope.json's own (now-removed)
// fixedValues and onto X12TransactionSetDef. ST01 needs no equivalent check
// here: it's the very value the parse-time lookup was keyed on, so by
// construction a successfully-parsed result's ST01 already matches its
// resolved transaction set (see edi/loop_engine.go's composite lookup) —
// checking it again here would be dead code. GS01 is never part of that
// lookup key at all (only ST01+GS08 are), and GS08 itself is unchecked when
// a single-variant set falls back to its bare ST01 key (835, 999) — so both
// checks catch real, otherwise-invisible mismatches, ERROR severity per this
// file's own two-severity model (a wrong envelope identity is malformed
// data, not a relational nuance).
func checkEnvelopeIdentity(spec *edi.X12SpecDef, result *edi.ParseResult) []Issue {
	txSet, ok := spec.TransactionSets[result.TransactionSet]
	if !ok {
		return nil
	}

	var issues []Issue
	if got, _ := result.Interchange["functionalIdentifierCode"].(string); got != "" && txSet.FunctionalIdentifierCode != "" && got != txSet.FunctionalIdentifierCode {
		issues = append(issues, Issue{
			Severity: "error",
			Path:     "GS.01",
			Message:  fmt.Sprintf("expected functional identifier code %q for transaction set %q, got %q", txSet.FunctionalIdentifierCode, result.TransactionSet, got),
		})
	}
	if got, _ := result.Interchange["versionReleaseIndustryCode"].(string); got != "" && txSet.VersionReleaseIndustryCode != "" && got != txSet.VersionReleaseIndustryCode {
		issues = append(issues, Issue{
			Severity: "error",
			Path:     "GS.08",
			Message:  fmt.Sprintf("expected version/release/industry code %q for transaction set %q, got %q", txSet.VersionReleaseIndustryCode, result.TransactionSet, got),
		})
	}
	return issues
}

// checkSyntaxRules walks every recorded edi.SegmentInstance and checks it
// against its own segment definition's SyntaxRules. Always warning-severity
// — see this file's own header comment for why.
func checkSyntaxRules(spec *edi.X12SpecDef, result *edi.ParseResult) []Issue {
	var issues []Issue
	for _, instance := range result.SegmentInstances {
		segDef, ok := spec.Segments[instance.SegmentID]
		if !ok {
			continue
		}
		for _, rule := range segDef.SyntaxRules {
			if msg := evaluateSyntaxRule(rule, instance.RawByPos); msg != "" {
				issues = append(issues, Issue{
					Severity: "warning",
					Path:     instance.SegmentID,
					Message:  msg,
					Source:   rule.Source,
				})
			}
		}
	}
	return issues
}

// evaluateSyntaxRule checks one rule against one segment instance's raw
// position values, returning a human-readable message if violated, or ""
// if satisfied. See edi.SyntaxRule's own doc comment for what each Type means.
func evaluateSyntaxRule(rule edi.SyntaxRule, rawByPos map[string]string) string {
	present := func(pos string) bool { return rawByPos[pos] != "" }

	switch rule.Type {
	case "P": // Paired: if any present, all must be.
		anyPresent, allPresent := false, true
		for _, pos := range rule.Positions {
			if present(pos) {
				anyPresent = true
			} else {
				allPresent = false
			}
		}
		if anyPresent && !allPresent {
			return fmt.Sprintf("paired elements %v: some present, not all", rule.Positions)
		}

	case "C": // Conditional: if Positions[0] present, all of Positions[1:] required.
		if len(rule.Positions) < 2 || !present(rule.Positions[0]) {
			return ""
		}
		for _, pos := range rule.Positions[1:] {
			if !present(pos) {
				return fmt.Sprintf("element %s present but conditionally-required element %s is missing", rule.Positions[0], pos)
			}
		}

	case "L": // List conditional: if any of Positions[1:] present, Positions[0] required.
		if len(rule.Positions) < 2 || present(rule.Positions[0]) {
			return ""
		}
		for _, pos := range rule.Positions[1:] {
			if present(pos) {
				return fmt.Sprintf("element %s present but its own anchor element %s is missing", pos, rule.Positions[0])
			}
		}

	case "R": // Required: at least one of Positions must be present.
		for _, pos := range rule.Positions {
			if present(pos) {
				return ""
			}
		}
		positions := append([]string{}, rule.Positions...)
		sort.Strings(positions)
		return fmt.Sprintf("at least one of elements %v must be present", positions)

	case "E": // Exclusion: at most one of Positions may be present.
		count := 0
		for _, pos := range rule.Positions {
			if present(pos) {
				count++
			}
		}
		if count > 1 {
			return fmt.Sprintf("at most one of elements %v may be present", rule.Positions)
		}
	}
	return ""
}
