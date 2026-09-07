// services/executors/transform/edi_validate_executor.go
// EDIValidateExecutor — pipeline step type "edi.validate".
//
// Re-checks a raw X12 EDI message against edi/validator.Validate — a
// permissive-parse's separate, explicit strict pass, exactly like
// edi.parse itself is permissive by design. A pipeline can branch on the
// result via control.if_then_else/control.switch_case.
//
// A deliberate deviation from this feature's own original design note
// (sourceField defaulting to "parsedEDI", the already-parsed step output):
// edi/validator's SyntaxRule checks (see edi/validator/validator.go) need
// the TYPED edi.ParseResult.SegmentInstances, which a prior edi.parse
// step's JSON-shaped ParsedJSON output does not carry — the pipeline's
// inter-step data flow is map[string]interface{}, not Go structs. Rather
// than build a cross-step typed-attachment mechanism (the CDA precedent —
// cda_document_resolution.go's setCDADocument/getCDADocument — exists but
// adds real complexity), this step independently re-parses from RAW
// content, the same cheap, deterministic, side-effect-free operation
// edi.parse itself performs. sourceField therefore defaults to "raw", not
// "parsedEDI".
//
// Config keys:
//   sourceField   — dot-path to the field holding raw EDI content (default: "raw")
//   outputField   — top-level key to write the validation Result under (default: "ediValidation")
//   disabledRules — []{segmentId, type, positions} — selectively suppresses specific OOB
//                   (schema-defined) SyntaxRules for this step only. positions are the
//                   raw, schema-native position strings GET /api/edi/schema/segments
//                   already returns for each rule (edi.SyntaxRule has no separate ID
//                   field, so (segmentId, type, positions) IS a rule's identity) — never
//                   mutates the shared, cached spec, same isolation guarantee customRules
//                   already provides.

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/edi/validator"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// EDIValidateExecutor re-checks a raw X12 EDI message's field values and
// business-rule (SyntaxRule) constraints. Self-initialises from the schema
// directory at construction time — same convention as EDIParseExecutor.
type EDIValidateExecutor struct {
	*executors.BaseExecutor
	loader *edi.X12SchemaLoader
}

// NewEDIValidateExecutor constructs the executor and loads schemas from the
// default schema directory "./edi/schemas/x12_005010".
func NewEDIValidateExecutor() *EDIValidateExecutor {
	exec := &EDIValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.validate", models.ExecutorMetadata{
			Name:        "EDI X12 Validator",
			Description: "Re-checks a raw X12 EDI message's field values (errors) and business-rule constraints (warnings, never blocking) — see edi/validator's own doc comment for the severity split",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "EDI Transform",
		}),
	}

	loader, err := edi.NewX12SchemaLoader("./edi/schemas/x12_005010")
	if err != nil {
		log.Printf("⚠️  [edi.validate] EDI schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [edi.validate] EDI schema loaded from ./edi/schemas/x12_005010")
	return exec
}

// ediCustomRule is one user-defined business rule, reusing the exact same
// P/C/L/R/E model edi.SyntaxRule already implements — see
// EdiValidateStepBuilder's own doc comment (public/js/pipeline/components/
// EDIStepBuilder.js) for why this isn't a second, simpler rule language.
// Positions are ELEMENT KEYS (the UI never shows raw numeric positions) —
// translateEdiKeysToPositions resolves them against the loaded schema.
type ediCustomRule struct {
	SegmentID string   `json:"segmentId"`
	Type      string   `json:"type"`
	Positions []string `json:"positions"`
}

// ediRuleRef identifies one OOB SyntaxRule to disable. Same
// (segmentId, type, positions) identity GetSegments's own syntaxRules
// summary returns — SyntaxRule has no separate ID field (edi/schema_types.go),
// so this composite key IS the identity, and the UI round-trips positions
// verbatim from that same API response rather than inventing its own IDs.
type ediRuleRef struct {
	SegmentID string   `json:"segmentId"`
	Type      string   `json:"type"`
	Positions []string `json:"positions"`
}

type ediValidateConfig struct {
	SourceField   string          `json:"sourceField"`
	OutputField   string          `json:"outputField"`
	CustomRules   []ediCustomRule `json:"customRules,omitempty"`
	DisabledRules []ediRuleRef    `json:"disabledRules,omitempty"`
}

// positionsEqual compares two position sets for exact membership,
// order-independent (a disable request round-trips the same array the
// segments API returned, but comparing as a set rather than requiring
// identical order is a cheap, harmless robustness margin).
func positionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	remaining := make(map[string]int, len(a))
	for _, v := range a {
		remaining[v]++
	}
	for _, v := range b {
		remaining[v]--
		if remaining[v] < 0 {
			return false
		}
	}
	return true
}

// specWithRuleOverrides returns a spec with disabledRules removed from and
// customRules appended to their targeted segments' own SyntaxRules — never
// mutating spec itself (the loader's cached, shared instance reused across
// every Execute() call; mutating it would leak one pipeline's overrides
// into every other validation). Only segments actually targeted by an
// override get their own cloned *edi.X12SegmentDef (once per segment, even
// across multiple overrides targeting it); every other segment is the SAME
// shared pointer spec already has.
func specWithRuleOverrides(spec *edi.X12SpecDef, disabledRules []ediRuleRef, customRules []ediCustomRule) *edi.X12SpecDef {
	cloned := &edi.X12SpecDef{
		SpecVersion:     spec.SpecVersion,
		Segments:        make(map[string]*edi.X12SegmentDef, len(spec.Segments)),
		TransactionSets: spec.TransactionSets,
		Envelope:        spec.Envelope,
	}
	for id, seg := range spec.Segments {
		cloned.Segments[id] = seg
	}

	cloneOnce := func(segmentID string) *edi.X12SegmentDef {
		original, ok := spec.Segments[segmentID]
		if !ok {
			return nil
		}
		if current := cloned.Segments[segmentID]; current != original {
			return current // already cloned earlier in this same call
		}
		dup := *original
		dup.SyntaxRules = append([]edi.SyntaxRule{}, original.SyntaxRules...)
		cloned.Segments[segmentID] = &dup
		return &dup
	}

	for _, ref := range disabledRules {
		segCopy := cloneOnce(ref.SegmentID)
		if segCopy == nil {
			log.Printf("⚠️  [edi.validate] disabledRules references unknown segment %q — skipped", ref.SegmentID)
			continue
		}
		filtered := segCopy.SyntaxRules[:0]
		matched := false
		for _, r := range segCopy.SyntaxRules {
			if !matched && r.Type == ref.Type && positionsEqual(r.Positions, ref.Positions) {
				matched = true
				continue // drop it
			}
			filtered = append(filtered, r)
		}
		segCopy.SyntaxRules = filtered
		if !matched {
			log.Printf("⚠️  [edi.validate] disabledRules entry for segment %q type %q positions %v matched no OOB rule — skipped", ref.SegmentID, ref.Type, ref.Positions)
		}
	}

	for _, rule := range customRules {
		original, ok := spec.Segments[rule.SegmentID]
		if !ok {
			log.Printf("⚠️  [edi.validate] custom rule references unknown segment %q — skipped", rule.SegmentID)
			continue
		}
		positions := translateEdiKeysToPositions(original, rule.Positions)
		if len(positions) == 0 {
			log.Printf("⚠️  [edi.validate] custom rule for segment %q resolved to zero valid positions — skipped", rule.SegmentID)
			continue
		}

		segCopy := cloneOnce(rule.SegmentID)
		segCopy.SyntaxRules = append(segCopy.SyntaxRules, edi.SyntaxRule{
			Type: rule.Type, Positions: positions, Source: "custom",
		})
	}

	return cloned
}

// translateEdiKeysToPositions resolves each element KEY to its real
// segment-relative position string. Scoped to the segment's plain Elements
// only — intra-segment Repeats groups (e.g. CAS's reason/amount/qty trios)
// aren't addressable by a custom rule in phase 1 (see EdiValidateStepBuilder's
// own doc comment for why). A key with no matching element is silently
// dropped rather than failing the whole rule — the same "degrade gracefully
// on stale config" convention applyCanonicalTransform's unknown-transform
// handling already establishes.
func translateEdiKeysToPositions(seg *edi.X12SegmentDef, keys []string) []string {
	var positions []string
	for _, key := range keys {
		for _, el := range seg.Elements {
			if el.Key == key {
				positions = append(positions, el.Pos)
				break
			}
		}
	}
	return positions
}

// Execute re-parses raw EDI content from sourceField and validates it,
// writing the result to outputField (default "ediValidation"). Never
// returns an error just because the message is invalid — Execute()
// failures are reserved for genuine execution problems (missing schema,
// missing source field, unparseable content); a message that parses but
// has validation errors/warnings still produces a normal step result the
// pipeline can branch on.
func (e *EDIValidateExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("edi.validate: EDI schema is not initialised (schema directory missing)")
	}

	cfg := ediValidateConfig{SourceField: "raw", OutputField: "ediValidation"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ediValidation"
	}

	rawEDI := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawEDI, _ = v.(string)
	}
	if rawEDI == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawEDI = v
		}
	}
	if rawEDI == "" {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			if v := executors.GetFieldValue(msg, cfg.SourceField); v != nil {
				rawEDI, _ = v.(string)
			}
			if rawEDI == "" {
				if v, ok := msg["raw"].(string); ok {
					rawEDI = v
				}
			}
		}
	}
	if rawEDI == "" {
		return nil, fmt.Errorf("edi.validate: source field %q is empty or not a string", cfg.SourceField)
	}

	spec := e.loader.Spec()
	if len(cfg.CustomRules) > 0 || len(cfg.DisabledRules) > 0 {
		// Must merge BEFORE parsing, not after: edi.ParseTransactionSet only
		// records a segment's raw values into ParseResult.SegmentInstances
		// (checkSyntaxRules' own data source) when that segment's OWN spec
		// entry already has at least one SyntaxRule — an optimization that
		// would silently skip every custom rule on a segment with zero OOB
		// rules (e.g. N3) if the merge happened after parsing instead.
		// Disabling rules has no equivalent ordering requirement (a segment
		// left with zero active rules simply stops needing to be recorded at
		// all — correct either way), but both overrides share one spec clone
		// per Execute() call regardless.
		spec = specWithRuleOverrides(spec, cfg.DisabledRules, cfg.CustomRules)
	}
	parsed, err := edi.ParseTransactionSet(spec, rawEDI)
	if err != nil {
		return nil, fmt.Errorf("edi.validate: %w", err)
	}
	result := validator.Validate(spec, parsed)

	durationMs := time.Since(start).Milliseconds()
	errorCount, warningCount := 0, 0
	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			errorCount++
		} else {
			warningCount++
		}
	}
	log.Printf("  ✅ [edi.validate] Validated %s: valid=%v, %d errors, %d warnings in %dms",
		parsed.TransactionSet, result.Valid, errorCount, warningCount, durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData[cfg.OutputField] = result

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"valid":         result.Valid,
			"issues":        result.Issues,
			"errorCount":    errorCount,
			"warningCount":  warningCount,
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"valid":       result.Valid,
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *EDIValidateExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the validation Result output for the field picker.
func (e *EDIValidateExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Valid", Path: "_stepOutput.valid", DataType: "boolean",
			Description: "false only when at least one ERROR-severity issue exists — warnings never affect this", Category: "EDI Transform"},
		{Name: "Issues", Path: "_stepOutput.issues", DataType: "array",
			Description: "All validation findings (severity, path, message) — errors are malformed values, warnings are business-rule (SyntaxRule) mismatches", Category: "EDI Transform"},
		{Name: "Error count", Path: "_stepOutput.errorCount", DataType: "number",
			Description: "Count of ERROR-severity issues", Category: "EDI Transform"},
		{Name: "Warning count", Path: "_stepOutput.warningCount", DataType: "number",
			Description: "Count of WARNING-severity issues (business-rule mismatches, never blocking)", Category: "EDI Transform"},
	}
}
