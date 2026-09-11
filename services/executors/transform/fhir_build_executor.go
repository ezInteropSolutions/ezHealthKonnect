// services/executors/transform/fhir_build_executor.go
// FHIRBuildExecutor — pipeline step type "fhir.build".
//
// The no-code, format-agnostic on-ramp for a single FHIR R4 resource: maps
// CSV columns, DB query columns, or arbitrary JSON fields directly onto a
// FHIR resource's own element paths (e.g. "birthDate", "identifier[0].value"),
// the mirror image of cda.map_to_canonical's "any source -> canonical CDA
// JSON" role — except here there is no separate "build" step, since a FHIR
// resource already IS the JSON shape the pipeline needs; this step produces
// it directly.
//
// Field-level value shaping reuses services/cda_fhir.DeclarativeTransformRegistry
// (cda.to_fhir's own transform dispatch) via its optional "transform" name —
// e.g. "cda_gender_to_fhir", "cda_quantity_to_fhir", "string_direct". Two
// input shapes exist among its ~50 registered transforms, both usable here:
//   - Bare-scalar transforms (string_direct, the *_status_to_fhir family,
//     cda_decimal_string_to_number, ...) take a plain string/number straight
//     from any CSV/DB/JSON column — the common case for a non-CDA source.
//   - Compound-shaped transforms (cda_code_to_codeable_concept,
//     cda_quantity_to_fhir, cda_name_to_fhir, cda_address_to_fhir,
//     cda_telecom_to_fhir, cda_time_to_fhir_date/datetime, ...) expect the
//     resolved source value to already be a JSON object shaped like the
//     corresponding cdadocument.CDA* struct (e.g. {"code":"...",
//     "codeSystem":"...","displayName":"..."} for a CodeableConcept-producing
//     transform) — only reachable when sourcePath points at a nested object
//     already carrying those field names (e.g. a JSON API response), not a
//     flat CSV column. A shape mismatch degrades safely: remarshalInto simply
//     produces zero-value fields and the transform returns nil (skip the
//     write), never a wrong value — every transform in that registry already
//     guarantees this for its CDA callers.
// No new transform code is added here; leaving "transform" empty passes the
// resolved value through unchanged (DeclarativeTransformRegistry.Apply's own
// convention).
//
// Writing into the (possibly nested/repeating) FHIR resource path reuses
// services/cda_fhir.SetFHIRPath/IndexedPath — exported wrappers around the
// same array-growing path writer the CDA→FHIR declarative engine already
// uses — NOT executors.UpdateFieldValue, which does not grow arrays (see
// SetFHIRPath's own doc comment). Reading source data DOES use
// executors.GetFieldValue, same as map_to_canonical_executor.go.
//
// Config keys:
//
//	resourceType    — FHIR resource type, e.g. "Patient" (required)
//	profile         — "base" (default) | "us-core" | ...
//	version         — "R4" (default)
//	outputField     — dot-path to write the built resource (default: "fhirResource")
//	fields          — []{targetPath, sourcePath, fallbackPaths?, valueMap?,
//	                  literalValue?, transform?, condition?} — flat/nested,
//	                  non-repeating element writes
//	repeatingGroups — []{targetPath, rowsPath, fields: [...], repeatingGroups?,
//	                  condition?, groupCondition?} — one sub-object written
//	                  per source row, for repeating elements (identifier[],
//	                  name[], telecom[], ...)
//	rowsPath        — optional; when set, builds ONE resource PER ROW found at
//	                  this path (relative to inputData) instead of one
//	                  resource from inputData directly — outputField then
//	                  receives an ARRAY. fields/repeatingGroups resolve
//	                  relative to each row, same convention repeatingGroups'
//	                  own Fields already use. See fhirBuildConfig.RowsPath's
//	                  own doc comment for why this exists (N independent
//	                  resources of one type from one array, with no
//	                  control.loop wrapper step needed).
//
// Assembling multiple resources into one Bundle needs no new code: add one
// fhir.build step per resource type, then feed each step's outputField into
// an existing services/executors/payload.PayloadBuilderExecutor "fhir_bundle"
// mode step — resourcePaths accepts both a single-resource path and an
// array-valued one (e.g. this rowsPath mode's own output) in the same list.
//
// Conditional field/row population: fields and repeatingGroups both take an
// optional condition (fhirFieldMappingRow.Condition / fhirRepeatingGroup.Condition)
// evaluated via the shared services/executors.EvaluateCondition{Values}
// engine — the same {field, operator, value|compareToField} shape and
// mergeWithFallback(row, topLevel) convention hl7_build_executor.go's own
// per-field/per-segment Condition already uses. A control.if_then_else/
// switch_case step upstream still works for branching that applies before
// this step runs at all, but it has no concept of "for each row this step is
// about to loop over via repeatingGroups, conditionally include it" — that
// loop only exists inside applyRepeatingGroup itself, so per-row filtering
// has to live here too (e.g. skipping blank-padded EDI 835 CAS adjustment
// trios while keeping populated ones in the same adjudication[] list — see
// fhir_build_executor_test.go). repeatingGroups additionally takes
// groupCondition, evaluated ONCE against the parent row/inputData before
// rowsPath is even resolved (contrast with condition, evaluated once per
// resolved row) — a fhirRepeatingGroup has no single/repeating cardinality
// switch the way an hl7SegmentConfig does, so "should this whole list exist
// at all" and "should this one row qualify" are two independent questions
// needing two independent fields, not one field whose meaning depends on
// context that doesn't exist here.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	"ezhealthkonnect/services/executors"
)

// FHIRBuildExecutor builds one FHIR R4 resource from configured no-code field
// mappings against arbitrary upstream row data.
type FHIRBuildExecutor struct {
	*executors.BaseExecutor
	transformReg *cdafhir.DeclarativeTransformRegistry
}

// NewFHIRBuildExecutor constructs the executor. The transform registry is
// stateless (pure name -> function dispatch) so it's safe to build once here;
// unlike the registry, r4.GetRegistry() is deliberately NOT fetched/cached at
// construction time — see resolveProfile's doc comment.
func NewFHIRBuildExecutor() *FHIRBuildExecutor {
	return &FHIRBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("fhir.build", models.ExecutorMetadata{
			Name:        "FHIR Resource Builder",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into a single FHIR R4 resource",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "FHIR Transform",
		}),
		transformReg: cdafhir.NewDeclarativeTransformRegistry(),
	}
}

// fhirFieldMappingRow maps one FHIR element path to a source path relative to
// the row being applied (inputData for top-level fields, one repeating-group
// row for repeatingGroups fields).
type fhirFieldMappingRow struct {
	TargetPath    string            `json:"targetPath"`
	SourcePath    string            `json:"sourcePath"`
	FallbackPaths []string          `json:"fallbackPaths,omitempty"`
	ValueMap      map[string]string `json:"valueMap,omitempty"`
	LiteralValue  string            `json:"literalValue,omitempty"`
	// Transform is a cdafhir.DeclarativeTransformRegistry name; "" means the
	// resolved value is written as-is (see this file's package doc comment).
	Transform string `json:"transform,omitempty"`
	// Condition, when set, gates whether this field is written at all —
	// same {field, operator, value|compareToField} shape/evaluator
	// hl7_build_executor.go's hl7FieldMappingRow.Condition already uses (see
	// conditionMet below). A nil/empty condition always holds (today's
	// existing behavior, unchanged).
	Condition map[string]interface{} `json:"condition,omitempty"`
}

// fhirRepeatingGroup maps one repeating FHIR element (identifier[], name[],
// telecom[], ...) from an array of source rows found at RowsPath — mirrors
// map_to_canonical_executor.go's sectionMappingRow, one sub-object built per
// row instead of two independently-indexed CollectAll passes that could drift
// out of alignment (the same rationale declarative_schema.go's MappingRow.Fields
// documents for its own CollectAll+Fields primitive).
//
// RepeatingGroups nests one repeating group inside another — e.g. building
// item[].adjudication[] from each item row's own CAS-derived adjustment
// rows. A nested group's RowsPath resolves relative to the CURRENT row (the
// same row its own Fields are applied against), not the top-level input —
// same relative-resolution convention edi_map_to_canonical_executor.go's own
// nested "loops" config already established for a different step. When a
// nested group's own TargetPath names a field the parent row's Fields (or an
// earlier nested group) already populated as an array — e.g. two ordinary
// indexed fields writing fixed adjudication[0]/adjudication[1] entries — the
// nested group's own rows are appended after whatever's already there
// (see applyRepeatingGroup) rather than overwriting it; this, combined with
// field_utils.go's "[*]" wildcard-flatten path support (added alongside this
// for EDI's CAS[*].adjustments shape), is what lets one declarative config
// build a 2-level nested FHIR structure with no script.
type fhirRepeatingGroup struct {
	TargetPath      string                `json:"targetPath"`
	RowsPath        string                `json:"rowsPath"`
	Fields          []fhirFieldMappingRow `json:"fields"`
	RepeatingGroups []fhirRepeatingGroup  `json:"repeatingGroups,omitempty"`
	// Condition, when set, is evaluated once per row RESOLVED from RowsPath
	// (against that row, falling back to topLevel) — false skips only that
	// one row, leaving other rows in the same list untouched. This is the
	// "some rows in this array are real, others are padding" case (e.g. an
	// EDI 835 CAS segment's unused adjustment-trio slots) that a
	// pipeline-level control.if_then_else/switch_case step can't express,
	// since those evaluate once against the whole message with no notion of
	// "for each row this step is about to loop over."
	Condition map[string]interface{} `json:"condition,omitempty"`
	// GroupCondition, when set, is evaluated ONCE, against the PARENT
	// row/inputData, BEFORE RowsPath is resolved at all — false means this
	// group is never attempted: RowsPath is never resolved and TargetPath is
	// never written (stays absent, not an empty array). This is the "should
	// this whole list exist at all" case, a genuinely different question from
	// Condition's "does this one resolved row qualify" — a
	// fhirRepeatingGroup has no single/repeating cardinality switch the way
	// hl7SegmentConfig does to give one field two meanings, and
	// EvaluateCondition has no AND/OR to combine both questions into one
	// check, so both fields are independently necessary (see
	// fhir_build_executor_test.go's GroupConditionAndRowCondition_BothApplyTogether).
	GroupCondition map[string]interface{} `json:"groupCondition,omitempty"`
}

type fhirBuildConfig struct {
	ResourceType string `json:"resourceType"`
	Profile      string `json:"profile"`
	Version      string `json:"version"`
	OutputField  string `json:"outputField"`
	// RowsPath, when set, switches this step from building ONE resource out of
	// inputData to building ONE resource PER ROW found at this path (relative
	// to inputData) — outputField then receives an ARRAY of resources instead
	// of a single object. Fields/RepeatingGroups resolve relative to each row
	// (inputData stays reachable as the fallback "topLevel" — same
	// row-with-topLevel-fallback convention fhirRepeatingGroup.Fields already
	// uses, see applyFieldRow's own doc comment), so a config written for the
	// single-resource case can be reused unchanged just by adding RowsPath.
	//
	// Exists specifically so a step chain that needs N independent resources
	// of the SAME type from an array (e.g. one Patient/Coverage/Claim per
	// claim in a multi-claim EDI 837 file) never needs a control.loop wrapper
	// step — control.loop's own childStepIds config can only hold real
	// DB-assigned step IDs, which don't exist yet when an OOB interface
	// template is authored (see database/migrations/V231's own documented
	// finding: "no template JSON can pre-declare that link"), making
	// control.loop fundamentally unusable inside a template migration. This
	// keeps the "N resources from one array" case fully declarative and
	// template-safe. payload.builder's fhir_bundle mode already accepts an
	// array-valued resourcePaths entry (see its own doc comment), so the
	// array this produces plugs in directly, same as any other array source.
	RowsPath        string                `json:"rowsPath,omitempty"`
	Fields          []fhirFieldMappingRow `json:"fields,omitempty"`
	RepeatingGroups []fhirRepeatingGroup  `json:"repeatingGroups,omitempty"`
}

// Execute builds one FHIR resource per the configured field/repeatingGroup
// mappings and writes it to the configured output field.
func (e *FHIRBuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := fhirBuildConfig{Version: "R4", Profile: "base", OutputField: "fhirResource"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.Version == "" {
		cfg.Version = "R4"
	}
	if cfg.Profile == "" {
		cfg.Profile = "base"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "fhirResource"
	}
	if cfg.ResourceType == "" {
		return nil, fmt.Errorf("fhir.build: resourceType is required")
	}

	// r4.GetRegistry() is fetched fresh on every Execute() call, never cached
	// at construction: InitRegistry() can run in a background goroutine at
	// startup (see main.go), so a registry captured too early could still be
	// nil or incomplete. fhir_validation_executor.go follows this same
	// lazy-fetch discipline for the identical reason.
	reg := r4.GetRegistry()
	if reg == nil {
		return nil, fmt.Errorf("fhir.build: FHIR schema registry not yet initialized")
	}
	if _, ok := reg.Get(cfg.Version, cfg.ResourceType, cfg.Profile); !ok {
		return nil, fmt.Errorf("fhir.build: unknown resourceType/profile/version %q/%q/%q", cfg.ResourceType, cfg.Profile, cfg.Version)
	}

	durationMs := time.Since(start).Milliseconds()
	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}

	if cfg.RowsPath != "" {
		rows := resolveRows(inputData, cfg.RowsPath)
		resources := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			resource := map[string]interface{}{"resourceType": cfg.ResourceType}
			for _, f := range cfg.Fields {
				e.applyFieldRow(resource, row, inputData, f)
			}
			for _, rg := range cfg.RepeatingGroups {
				e.applyRepeatingGroup(resource, row, inputData, rg)
			}
			resources = append(resources, resource)
		}
		durationMs = time.Since(start).Milliseconds()
		executors.UpdateFieldValue(outputData, cfg.OutputField, resources)

		log.Printf("  ✅ [fhir.build] Built %d %s resource(s) from rowsPath %q (%d field(s), %d repeating group(s)) in %dms",
			len(resources), cfg.ResourceType, cfg.RowsPath, len(cfg.Fields), len(cfg.RepeatingGroups), durationMs)

		e.SetStepOutputWithDetails(outputData,
			map[string]interface{}{"fhirResource": resources},
			map[string]interface{}{
				"duration_ms":    durationMs,
				"success":        true,
				"resourceType":   cfg.ResourceType,
				"resourceCount":  len(resources),
				"transformation": "fhir_build",
			},
		)
		return outputData, nil
	}

	resource := map[string]interface{}{"resourceType": cfg.ResourceType}
	for _, f := range cfg.Fields {
		e.applyFieldRow(resource, inputData, inputData, f)
	}
	for _, rg := range cfg.RepeatingGroups {
		e.applyRepeatingGroup(resource, inputData, inputData, rg)
	}
	durationMs = time.Since(start).Milliseconds()
	executors.UpdateFieldValue(outputData, cfg.OutputField, resource)

	log.Printf("  ✅ [fhir.build] Built %s resource (%d field(s), %d repeating group(s)) in %dms",
		cfg.ResourceType, len(cfg.Fields), len(cfg.RepeatingGroups), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"fhirResource": resource},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"resourceType":   cfg.ResourceType,
			"transformation": "fhir_build",
		},
	)

	return outputData, nil
}

// applyRepeatingGroup builds one sub-object per row found at rg.RowsPath
// (relative to contextRow — inputData for a top-level group, or the current
// row for a nested one) and writes each at target[rg.TargetPath][idx] via
// cdafhir.IndexedPath/SetFHIRPath, continuing the index sequence from
// whatever's already at that path (see startingIndex) rather than always
// starting at 0 — see this struct's own doc comment for why. Any of rg's own
// RepeatingGroups are then applied against each row's own subObj, relative to
// that same row.
func (e *FHIRBuildExecutor) applyRepeatingGroup(target map[string]interface{}, contextRow map[string]interface{}, topLevel map[string]interface{}, rg fhirRepeatingGroup) {
	if rg.TargetPath == "" {
		return
	}
	if !e.conditionMet(rg.GroupCondition, contextRow, topLevel) {
		return
	}
	rows := resolveRows(contextRow, rg.RowsPath)
	idx := startingIndex(target, rg.TargetPath)
	for _, row := range rows {
		if !e.conditionMet(rg.Condition, row, topLevel) {
			continue
		}
		subObj := map[string]interface{}{}
		for _, f := range rg.Fields {
			e.applyFieldRow(subObj, row, topLevel, f)
		}
		for _, nested := range rg.RepeatingGroups {
			e.applyRepeatingGroup(subObj, row, topLevel, nested)
		}
		if len(subObj) == 0 {
			continue
		}
		cdafhir.SetFHIRPath(target, cdafhir.IndexedPath(rg.TargetPath, idx), subObj)
		idx++
	}
}

// startingIndex returns how many entries already sit at target[targetPath]
// (0 if absent or not yet an array) — the index a repeatingGroup's own writes
// should continue from, so a nested group appends after entries its parent
// row's ordinary Fields (or an earlier nested group) already wrote at the
// same TargetPath instead of clobbering them. targetPath is always a bare
// field name here (every repeatingGroup TargetPath in this engine is, e.g.
// "item", "identifier", "adjudication" — never a dotted/bracketed path), so a
// direct map lookup is correct.
func startingIndex(target map[string]interface{}, targetPath string) int {
	if arr, ok := target[targetPath].([]interface{}); ok {
		return len(arr)
	}
	return 0
}

// applyFieldRow resolves f's value against source (inputData for a top-level
// field, one repeating-group row for a group field) — trying SourcePath, then
// each FallbackPaths entry, then LiteralValue, the same "first present value
// wins" convention map_to_canonical_executor.go's applyFieldMapping uses —
// then applies f.Transform (if any) and writes the result into target at
// f.TargetPath via cdafhir.SetFHIRPath, skipping the write entirely when no
// value was found or the transform decided there is nothing to write (both
// are normal, expected outcomes — not an error, per DeclarativeTransformFn's
// own "return nil, nil for empty" convention).
func (e *FHIRBuildExecutor) applyFieldRow(target map[string]interface{}, source map[string]interface{}, topLevel map[string]interface{}, f fhirFieldMappingRow) {
	if f.TargetPath == "" {
		return
	}
	if !e.conditionMet(f.Condition, source, topLevel) {
		return
	}

	value := e.resolveRawValue(source, f)
	if isEmptyFieldValue(value) {
		return
	}

	transformed, err := e.transformReg.Apply(f.Transform, value, f.ValueMap)
	if err != nil {
		log.Printf("  ⚠️  [fhir.build] transform %q failed for target %q: %v", f.Transform, f.TargetPath, err)
		return
	}
	if isEmptyFieldValue(transformed) {
		return
	}

	cdafhir.SetFHIRPath(target, f.TargetPath, transformed)
}

// conditionMet reports whether condition holds against source (a
// fhirRepeatingGroup row/field's own source, or contextRow for a
// GroupCondition check), falling back to topLevel for anything condition's
// field/compareToField doesn't find in source — mirrors
// hl7_build_executor.go's own conditionMet/mergeWithFallback exactly (same
// package `transform`, mergeWithFallback is reused directly, not
// reimplemented), so a nested row's condition can check either its own
// field or a genuinely top-level one without the config needing to know
// which. A nil/empty condition always holds (no condition configured =
// always build/populate, today's existing behavior unchanged).
//
// Deliberately resolves via executors.GetFieldValue, NOT
// executors.EvaluateCondition's own GetNestedValue: GetFieldValue is the
// same resolver this file already uses for SourcePath/FallbackPaths,
// including predicate-bracket paths (e.g. "NM1[entityIdentifierCode=82]")
// and native []map[string]interface{} slices (the shape EDI's parser
// produces) — GetNestedValue supports neither, which would make a condition
// on that same kind of path silently evaluate as "not met" even though the
// identical path resolves fine for an ordinary field mapping. Evaluation
// errors (e.g. a malformed regex) are logged and treated as not-met — a
// broken condition should suppress the field/row, not crash the whole build.
func (e *FHIRBuildExecutor) conditionMet(condition map[string]interface{}, source, topLevel map[string]interface{}) bool {
	if len(condition) == 0 {
		return true
	}
	merged := mergeWithFallback(source, topLevel)
	field, _ := condition["field"].(string)
	operator, _ := condition["operator"].(string)

	fieldValue := executors.GetFieldValue(merged, field)
	var compareValue interface{}
	if compareToField, _ := condition["compareToField"].(string); compareToField != "" {
		compareValue = executors.GetFieldValue(merged, compareToField)
	} else {
		compareValue = condition["value"]
	}

	met, err := executors.EvaluateConditionValues(operator, fieldValue, compareValue)
	if err != nil {
		log.Printf("  ⚠️  [fhir.build] condition evaluation failed, treating as not met: %v", err)
		return false
	}
	return met
}

// resolveRawValue tries SourcePath, then each FallbackPaths candidate, then
// LiteralValue — preserving whatever native type GetFieldValue resolves
// (string/float64/bool/map/slice), since compound transforms (cda_name_to_fhir,
// cda_quantity_to_fhir, ...) need the value's native shape, not a
// pre-stringified one.
func (e *FHIRBuildExecutor) resolveRawValue(source map[string]interface{}, f fhirFieldMappingRow) interface{} {
	if f.SourcePath != "" {
		if v := executors.GetFieldValue(source, f.SourcePath); !isEmptyFieldValue(v) {
			return v
		}
	}
	for _, fp := range f.FallbackPaths {
		if v := executors.GetFieldValue(source, fp); !isEmptyFieldValue(v) {
			return v
		}
	}
	if f.LiteralValue != "" {
		return f.LiteralValue
	}
	return nil
}

// isEmptyFieldValue reports whether v should be treated as "nothing resolved"
// — nil, or an empty string. Other zero values (0, false, empty map/slice)
// are meaningful FHIR content and are never treated as empty.
func isEmptyFieldValue(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
}

// Validate checks step configuration. Field/repeatingGroup rows are all
// optional (a step with none configured is a no-op, not an error), matching
// cda.build/cda.map_to_canonical's own permissive Validate — required-field
// enforcement belongs to the downstream fhir_validation step, not this one.
func (e *FHIRBuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built resource for the field picker
// (public/js/pipeline/utils/StepVariablesProvider.js -> FieldPathSearchComponent,
// e.g. payload.builder's Resource Paths picker). Reads THIS step's own
// configured outputField/resourceType rather than the "fhirResource"
// default — a step whose author renamed outputField (e.g. the EDI 835
// template's "message.paymentReconciliation") would otherwise have the
// picker offer a path that doesn't actually exist in the pipeline data,
// silently pointing users at the wrong field.
func (e *FHIRBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	outputField := "fhirResource"
	resourceType := "FHIR"
	if step != nil && step.Config != nil {
		if v, ok := step.Config["outputField"].(string); ok && v != "" {
			outputField = v
		}
		if v, ok := step.Config["resourceType"].(string); ok && v != "" {
			resourceType = v
		}
	}
	return []models.VariableDefinition{
		{Name: resourceType + " Resource", Path: outputField, DataType: "object",
			Description: "FHIR R4 " + resourceType + " resource built from this step's configured field mappings", Category: "FHIR Transform"},
	}
}
