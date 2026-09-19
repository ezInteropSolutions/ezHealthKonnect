// ncpdp/validator/validator.go
//
// Validate checks a ncpdp.ParseResult against the schema's own Required
// flags — one generic recursive walk (validateNode), reused for both the
// Header and the transaction Body, matching ncpdp.parseNode/
// ncpdp/builder.buildNode's identical "one function handles every
// group/transaction" discipline.
//
// Two-severity model, mirroring edi/validator's own dated (2026-09-01)
// "flexible, not rigid" product decision: a missing/empty REQUIRED field or
// group is a blocking error (Result.Valid=false); anything conditional or
// business-rule-shaped (NCPDP's implementation guide almost certainly has
// "if X present, Y required"-style constraints, the same class EDI's own
// SyntaxRule/P-C-L-R-E model captures) would be a non-blocking warning —
// but NO such rules are authored yet for Phase 1: unlike EDI's PyX12/Stedi
// companion-guide sourcing, no equivalent free source of NCPDP's own
// conditional-requirement rules was found during this phase (see CLAUDE.md's
// NCPDP SCRIPT section for the full sourcing-discipline note). Rather than
// fabricate business rules with no real source, Issue.Severity's warning
// tier is left as an intentionally-empty, ready-to-extend mechanism — a
// named gap, not a silent omission.
package validator

import (
	"fmt"

	"ezhealthkonnect/ncpdp"
)

// Severity is either "error" (blocks Result.Valid) or "warning" (never
// does) — see this file's own doc comment for the product decision.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one validation finding, addressed by a dotted path into the
// canonical record (e.g. "NewRx.medicationPrescribed.quantity.value").
type Issue struct {
	Severity Severity
	Path     string
	Message  string
}

// Result is the outcome of validating one parsed message.
type Result struct {
	Valid  bool
	Issues []Issue
}

// Validate checks result's Header against spec's Header group and result's
// Body against the schema definition for result.TransactionType.
func Validate(spec *ncpdp.NCPDPSpecDef, result *ncpdp.ParseResult) *Result {
	var issues []Issue

	if hdr := spec.Header(); hdr != nil {
		issues = append(issues, validateNode("Header", hdr.Fields, hdr.Groups, result.Header, spec)...)
	}

	tx, ok := spec.Transactions[result.TransactionType]
	if !ok {
		issues = append(issues, Issue{
			Severity: SeverityError,
			Path:     "",
			Message:  fmt.Sprintf("unknown transaction type %q", result.TransactionType),
		})
	} else {
		issues = append(issues, validateNode(result.TransactionType, tx.Fields, tx.Groups, result.Body, spec)...)
	}

	valid := true
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			valid = false
			break
		}
	}
	return &Result{Valid: valid, Issues: issues}
}

// validateNode checks fields/groups against data at path — the single
// mechanism used for both NCPDPGroupDef and NCPDPTransactionDef, and for
// every nesting depth (including repeatable groups, indexed in the
// reported path), mirroring parseNode/buildNode's identical role.
func validateNode(path string, fields []ncpdp.NCPDPFieldDef, groups []ncpdp.NCPDPGroupRef, data map[string]interface{}, spec *ncpdp.NCPDPSpecDef) []Issue {
	var issues []Issue

	for _, f := range fields {
		if !f.Required {
			continue
		}
		v, present := data[f.Key]
		if !present {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Path:     path + "." + f.Key,
				Message:  fmt.Sprintf("required field %q (%s) is missing", f.Key, f.XPath),
			})
			continue
		}
		if s, ok := ncpdp.StringValue(v); !ok || s == "" {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Path:     path + "." + f.Key,
				Message:  fmt.Sprintf("required field %q is empty", f.Key),
			})
		}
	}

	for _, ref := range groups {
		childGroup, ok := spec.Groups[ref.GroupKey]
		if !ok {
			continue // already fail-fast validated at load time; defensive only
		}
		raw, present := data[ref.Key]
		if ref.Required && !present {
			issues = append(issues, Issue{
				Severity: SeverityError,
				Path:     path + "." + ref.Key,
				Message:  fmt.Sprintf("required group %q is missing", ref.Key),
			})
			continue
		}
		if !present {
			continue
		}
		if ref.Repeatable {
			items := ncpdp.ToSlice(raw)
			for i, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					childPath := fmt.Sprintf("%s.%s[%d]", path, ref.Key, i)
					issues = append(issues, validateNode(childPath, childGroup.Fields, childGroup.Groups, m, spec)...)
				}
			}
			continue
		}
		if m, ok := raw.(map[string]interface{}); ok {
			issues = append(issues, validateNode(path+"."+ref.Key, childGroup.Fields, childGroup.Groups, m, spec)...)
		}
	}

	return issues
}
