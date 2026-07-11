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
//	                  literalValue?, transform?} — flat/nested, non-repeating
//	                  element writes
//	repeatingGroups — []{targetPath, rowsPath, fields: [...]} — one sub-object
//	                  written per source row, for repeating elements
//	                  (identifier[], name[], telecom[], ...)
//
// Assembling multiple resources into one Bundle needs no new code: add one
// fhir.build step per resource type, then feed each step's outputField into
// an existing services/executors/payload.PayloadBuilderExecutor "fhir_bundle"
// mode step.
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
}

// fhirRepeatingGroup maps one repeating FHIR element (identifier[], name[],
// telecom[], ...) from an array of source rows found at RowsPath — mirrors
// map_to_canonical_executor.go's sectionMappingRow, one sub-object built per
// row instead of two independently-indexed CollectAll passes that could drift
// out of alignment (the same rationale declarative_schema.go's MappingRow.Fields
// documents for its own CollectAll+Fields primitive).
type fhirRepeatingGroup struct {
	TargetPath string                `json:"targetPath"`
	RowsPath   string                `json:"rowsPath"`
	Fields     []fhirFieldMappingRow `json:"fields"`
}

type fhirBuildConfig struct {
	ResourceType    string                `json:"resourceType"`
	Profile         string                `json:"profile"`
	Version         string                `json:"version"`
	OutputField     string                `json:"outputField"`
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

	resource := map[string]interface{}{"resourceType": cfg.ResourceType}

	for _, f := range cfg.Fields {
		e.applyFieldRow(resource, inputData, f)
	}
	for _, rg := range cfg.RepeatingGroups {
		e.applyRepeatingGroup(resource, inputData, rg)
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
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
// (relative to inputData) and writes it at rg.TargetPath[idx] via
// cdafhir.IndexedPath/SetFHIRPath.
func (e *FHIRBuildExecutor) applyRepeatingGroup(resource map[string]interface{}, inputData map[string]interface{}, rg fhirRepeatingGroup) {
	if rg.TargetPath == "" {
		return
	}
	rows := resolveRows(inputData, rg.RowsPath)
	idx := 0
	for _, row := range rows {
		subObj := map[string]interface{}{}
		for _, f := range rg.Fields {
			e.applyFieldRow(subObj, row, f)
		}
		if len(subObj) == 0 {
			continue
		}
		cdafhir.SetFHIRPath(resource, cdafhir.IndexedPath(rg.TargetPath, idx), subObj)
		idx++
	}
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
func (e *FHIRBuildExecutor) applyFieldRow(target map[string]interface{}, source map[string]interface{}, f fhirFieldMappingRow) {
	if f.TargetPath == "" {
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

// GetOutputVariables declares the built resource for the field picker.
func (e *FHIRBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "FHIR Resource", Path: "fhirResource", DataType: "object",
			Description: "FHIR R4 resource built from configured field mappings", Category: "FHIR Transform"},
	}
}
