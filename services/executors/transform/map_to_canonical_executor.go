// services/executors/transform/map_to_canonical_executor.go
// MapToCanonicalExecutor — pipeline step type "cda.map_to_canonical".
//
// The no-code, format-agnostic on-ramp for CCD construction: maps CSV
// columns, DB query columns, or arbitrary JSON fields onto the SAME
// canonical, USCDI-keyed JSON shape cda.parse and format.BundleToCanonicalDoc
// already produce ({"header": {...}, "sections": {key: {"entries": [...]}}}),
// so cda.build can serialize a CCD from a source system with zero new Go
// code — only step configuration. Closes the last gap in the "CDA/FHIR/CSV/
// DB, any source" plan: cda.build handles the CDA-shaped output regardless
// of producer; this step is the producer for everything that isn't already
// CDA XML (cda.parse) or a FHIR Bundle (format.BundleToCanonicalDoc).
//
// Row/field resolution reuses services/executors/field_utils.go's
// GetFieldValue/UpdateFieldValue exactly as
// services/executors/format/fhir_bundle_adapter.go already does — no new
// path-resolution mechanism. Per-field ValueMap (simple string->string
// translation, e.g. a CSV status column's "A" -> the canonical "active")
// mirrors fhir_bundle_adapter.go's kindCodeValue exactly. A full named-
// transform mechanism (services/cda_fhir/declarative_transform_registry.go)
// was deliberately NOT reused here — every one of its ~50 registered
// transforms returns a FHIR-resource-shaped value (nested CodeableConcept/
// HumanName/Address/Quantity maps), not the flat scalar string
// entry_archetypes.go's writeFieldValue/stringValue expects; wiring it in
// would let a user pick a transform that silently produces no output (a
// non-string value fails stringValue's type assertion) instead of failing
// loudly. Instead, an optional per-field Transform dispatches to
// canonical_value_transforms.go's small, pure string->string registry
// (date/datetime reformatting, trim, upper/lowercase) — CDA-target-only, so
// it structurally cannot reproduce that silent-drop failure mode. Applied
// BEFORE ValueMap (see applyFieldMapping/applyHeaderField): Transform
// normalizes the raw value, ValueMap is the exact-match override on top of
// that normalized result.
//
// Config keys:
//
//	outputField — dot-path to write the canonical JSON (default: "parsedCDA")
//	header      — []{group: "patient"|"author", target, sourcePath,
//	              transform?} — flat scalar header fields only (see
//	              cda/builder/canonical_field_catalog.go's HeaderFieldCatalog
//	              for the full target-key vocabulary per group, including the
//	              "address.<key>" nesting convention for patient address).
//	              informant/documentationOf/encompassingEncounter are
//	              repeating/optional groups intentionally out of scope for
//	              this step (cda.build already synthesizes a safe fallback
//	              for documentationOf when absent).
//	sections    — []{sectionKey, rowsPath, fields: []{canonicalField,
//	              sourcePath, fallbackPaths?, transform?, valueMap?,
//	              literalValue?}}
package transform

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// MapToCanonicalExecutor builds canonical USCDI-keyed JSON from configured
// no-code field mappings against arbitrary upstream row data.
type MapToCanonicalExecutor struct {
	*executors.BaseExecutor
}

// NewMapToCanonicalExecutor constructs the executor.
func NewMapToCanonicalExecutor() *MapToCanonicalExecutor {
	return &MapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.map_to_canonical", models.ExecutorMetadata{
			Name:        "Map to Canonical CDA JSON",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into the canonical USCDI-keyed JSON shape cda.build consumes",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}
}

// headerFieldRow maps one flat header target key (relative to
// header.<Group>) to a source path in the pipeline's inputData.
type headerFieldRow struct {
	Group      string `json:"group"`               // "patient" | "author"
	Target     string `json:"target"`              // dot-path relative to header.<Group>, e.g. "address.street"
	SourcePath string `json:"sourcePath"`          // GetFieldValue-compatible path, relative to inputData
	Transform  string `json:"transform,omitempty"` // canonical_value_transforms.go transform name, applied before writing
}

// fieldMappingRow maps one canonical schema field key (CDAFieldDef.Key, or
// its +Display/+System/+Unit/+Family companion) to a source path relative to
// one row.
type fieldMappingRow struct {
	CanonicalField string            `json:"canonicalField"`
	SourcePath     string            `json:"sourcePath"`
	FallbackPaths  []string          `json:"fallbackPaths,omitempty"`
	Transform      string            `json:"transform,omitempty"` // canonical_value_transforms.go transform name, applied before ValueMap
	ValueMap       map[string]string `json:"valueMap,omitempty"`
	LiteralValue   string            `json:"literalValue,omitempty"`
}

// sectionMappingRow maps one canonical section's entries from an array of
// source rows found at RowsPath.
type sectionMappingRow struct {
	SectionKey string            `json:"sectionKey"`
	RowsPath   string            `json:"rowsPath"`
	Fields     []fieldMappingRow `json:"fields"`
}

type mapToCanonicalConfig struct {
	OutputField string              `json:"outputField"`
	Header      []headerFieldRow    `json:"header,omitempty"`
	Sections    []sectionMappingRow `json:"sections,omitempty"`
}

// Execute builds canonical JSON per the configured header/section mappings
// and writes it to the configured output field.
func (e *MapToCanonicalExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := mapToCanonicalConfig{OutputField: "parsedCDA"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedCDA"
	}

	canonicalDoc := map[string]interface{}{
		"header":   map[string]interface{}{},
		"sections": map[string]interface{}{},
	}

	for _, h := range cfg.Header {
		applyHeaderField(canonicalDoc, inputData, h)
	}

	sectionsOut := canonicalDoc["sections"].(map[string]interface{})
	for _, sm := range cfg.Sections {
		entries := buildSectionEntries(inputData, sm)
		if len(entries) > 0 {
			sectionsOut[sm.SectionKey] = map[string]interface{}{"entries": entries}
		}
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, canonicalDoc)

	log.Printf("  ✅ [cda.map_to_canonical] Mapped %d section(s) into canonical JSON in %dms", len(sectionsOut), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"parsedCDA": canonicalDoc},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"sectionCount":   len(sectionsOut),
			"transformation": "map_to_canonical",
		},
	)

	return outputData, nil
}

// applyHeaderField resolves one header row's SourcePath against inputData
// and, if a non-empty value is found, applies Transform (see
// canonical_value_transforms.go) and writes the result to
// canonicalDoc["header"][Group][Target].
func applyHeaderField(canonicalDoc map[string]interface{}, inputData map[string]interface{}, h headerFieldRow) {
	if h.Group == "" || h.Target == "" || h.SourcePath == "" {
		return
	}
	if s, ok := stringifyValue(executors.GetFieldValue(inputData, h.SourcePath)); ok {
		s = applyCanonicalTransform(h.Transform, s)
		executors.UpdateFieldValue(canonicalDoc, "header."+h.Group+"."+h.Target, s)
	}
}

// buildSectionEntries resolves sm.RowsPath to an array of source rows and
// applies every field mapping to each one, skipping rows that produce no
// mapped fields at all (empty entries would otherwise satisfy cda.build's
// "SHALL section, zero entries" narrative-only path incorrectly).
func buildSectionEntries(inputData map[string]interface{}, sm sectionMappingRow) []interface{} {
	rows := resolveRows(inputData, sm.RowsPath)
	entries := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		entry := map[string]interface{}{}
		for _, fm := range sm.Fields {
			applyFieldMapping(entry, row, fm)
		}
		if len(entry) > 0 {
			entries = append(entries, entry)
		}
	}
	return entries
}

func resolveRows(inputData map[string]interface{}, rowsPath string) []map[string]interface{} {
	arr, ok := executors.GetFieldValue(inputData, rowsPath).([]interface{})
	if !ok {
		return nil
	}
	rows := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

// applyFieldMapping writes fm.CanonicalField into entry from row, trying
// SourcePath, then each FallbackPaths entry in order, then LiteralValue —
// the same "first present value wins" convention
// services/cda_fhir MappingRow.FallbackPaths already uses. Once a raw value
// is found, Transform runs first (normalizing e.g. a source date into CDA's
// TS format — see canonical_value_transforms.go), then ValueMap translates
// the (possibly transformed) result — matching fhir_bundle_adapter.go's
// kindCodeValue behavior, just with a normalization step ahead of it.
func applyFieldMapping(entry map[string]interface{}, row map[string]interface{}, fm fieldMappingRow) {
	if fm.CanonicalField == "" {
		return
	}
	if s, ok := stringifyValue(executors.GetFieldValue(row, fm.SourcePath)); ok {
		entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, s), fm.ValueMap)
		return
	}
	for _, fp := range fm.FallbackPaths {
		if s, ok := stringifyValue(executors.GetFieldValue(row, fp)); ok {
			entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, s), fm.ValueMap)
			return
		}
	}
	if fm.LiteralValue != "" {
		entry[fm.CanonicalField] = mapValue(applyCanonicalTransform(fm.Transform, fm.LiteralValue), fm.ValueMap)
	}
}

func mapValue(s string, vm map[string]string) string {
	if vm == nil {
		return s
	}
	if mapped, ok := vm[s]; ok {
		return mapped
	}
	return s
}

// stringifyValue coerces a resolved field value (string/float64/bool from a
// JSON-shaped source row) to its canonical string form, returning ok=false
// for nil/empty — the same "skip, don't write empty" convention
// entry_archetypes.go's stringValue uses on the CDA-building side.
func stringifyValue(v interface{}) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		if t == "" {
			return "", false
		}
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	default:
		return "", false
	}
}

// Validate checks step configuration. Mapping rows are all optional
// (a step with neither header nor sections configured is a no-op, not an
// error — matching cda.build/cda.to_fhir's own permissive Validate).
func (e *MapToCanonicalExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the canonical JSON output for the field picker.
func (e *MapToCanonicalExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Canonical CDA JSON", Path: "parsedCDA", DataType: "object",
			Description: "USCDI-keyed canonical JSON (header + sections), ready for cda.build", Category: "CDA Transform"},
	}
}
