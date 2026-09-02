// services/executors/transform/edi_map_to_canonical_executor.go
// EDIMapToCanonicalExecutor — pipeline step type "edi.map_to_canonical".
//
// The no-code, format-agnostic on-ramp for building an 835 from data that
// never went through edi.parse: maps CSV columns, DB query columns, or
// arbitrary JSON fields onto the SAME canonical header/loops JSON edi.parse's
// own ParsedJSON and edi/builder.BuildInput already use, so edi.build can
// serialize an interchange from a source system with zero new Go code —
// only step configuration.
//
// Deliberately NOT a reuse of cda.map_to_canonical (map_to_canonical_executor.go):
// that step is tightly coupled to CDA's own canonical shape (header.<patient|
// author>, sections.<key>.entries, one level of GroupBy nesting matching
// cda/builder's RepeatingGroup). EDI's canonical shape is segment-ID-keyed
// (header) and loop-ID-keyed with GENUINE recursive nesting (loops repeat
// AND nest inside each other — 835 alone needs 2000->2100->2110, 3 levels
// deep) — a fundamentally different shape needing its own mapper.
//
// UNLIKE an earlier draft of this file's own doc comment claimed, this step
// is NOT schema-blind at execution time — a real bug caught by its own
// round-trip test: edi/builder.writeLoops decides array-vs-single-map shape
// from the SCHEMA's own X12LoopDef.RepeatsMultiple() flag, not from whatever
// shape the source data happens to be (the same "RepeatsMultiple() is
// schema-driven, not observed-count-driven" rule edi/loop_engine.go's own
// parse-side tests already established). So this step self-initialises an
// *edi.X12SchemaLoader (same convention as edi.parse/validate/build) and
// consults the loaded transaction set's own loop tree to decide whether each
// mapped loop's output must be wrapped as a 1+-element []map[string]interface{}
// (schema says repeating) or written as a bare map (schema says single-
// instance) — independent of whether the step's own RowsPath was set, so a
// user mapping a single row into a schema-repeating loop (e.g. exactly one
// claim) still produces the array shape edi.build requires, not a bare map
// it would silently drop.
//
// Row/field resolution otherwise reuses map_to_canonical_executor.go's own
// package-level primitives directly (same package `transform`, not CDA-
// private): executors.GetFieldValue, resolveRows, stringifyValue,
// applyCanonicalTransform — no duplication.
//
// Deliberately out of scope for this pass: trailer/PLB mapping (a
// repeating, non-loop trailer segment) — named here rather than silently
// half-built, since PLB's own shape (a bare repeating segment, not a loop)
// doesn't fit the loop-mapping recursion cleanly and would need its own,
// separate UI this pass doesn't build. edi.build still accepts a
// hand-supplied "trailer" key in its own source data; it's just not
// produced by this step yet.
//
// Config keys:
//
//	outputField    — dot-path to write the canonical JSON (default: "canonicalEDI")
//	transactionSet — which transaction set's loop tree to consult, e.g. "835" (default: "835")
//	header         — []{segmentId, elementKey, sourcePath, transform?, literalValue?} —
//	                 flat, single-instance header segments (ST/BPR/TRN/CUR/REF/DTM)
//	loops          — []{loopId, rowsPath?, fields: [...same shape as header...],
//	                 loops?: [...recursive, same shape...]} — rowsPath resolves
//	                 which row(s) to map from (for a NESTED loop, relative to
//	                 the parent loop's own current row, not global inputData);
//	                 absent = map from the current single row. The schema's own
//	                 cardinality (not RowsPath) decides the OUTPUT shape — see
//	                 this file's own header comment above.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// EDIMapToCanonicalExecutor builds canonical EDI JSON (header/loops shape)
// from configured no-code field mappings against arbitrary upstream row
// data. Self-initialises from the schema directory at construction time —
// same convention as EDIParseExecutor/EDIValidateExecutor/EDIBuildExecutor.
type EDIMapToCanonicalExecutor struct {
	*executors.BaseExecutor
	loader *edi.X12SchemaLoader
}

// NewEDIMapToCanonicalExecutor constructs the executor and loads the EDI
// schema from the default schema directory "./edi/schemas/x12_005010".
func NewEDIMapToCanonicalExecutor() *EDIMapToCanonicalExecutor {
	exec := &EDIMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.map_to_canonical", models.ExecutorMetadata{
			Name:        "Map to Canonical EDI JSON",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into the canonical header/loops JSON edi.build consumes",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "EDI Transform",
		}),
	}

	loader, err := edi.NewX12SchemaLoader("./edi/schemas/x12_005010")
	if err != nil {
		log.Printf("⚠️  [edi.map_to_canonical] EDI schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [edi.map_to_canonical] EDI schema loaded from ./edi/schemas/x12_005010")
	return exec
}

// ediFieldMappingRow maps one segment element (segmentId + elementKey) to a
// source value — used identically for flat header rows and loop fields
// (both write into a segment-ID-keyed map the same way).
type ediFieldMappingRow struct {
	SegmentID    string `json:"segmentId"`
	ElementKey   string `json:"elementKey"`
	SourcePath   string `json:"sourcePath,omitempty"`
	Transform    string `json:"transform,omitempty"`
	LiteralValue string `json:"literalValue,omitempty"`
}

// ediLoopMapping is one loop's mapping config, recursively nestable to match
// edi.X12LoopDef's own tree shape.
type ediLoopMapping struct {
	LoopID   string               `json:"loopId"`
	RowsPath string               `json:"rowsPath,omitempty"`
	Fields   []ediFieldMappingRow `json:"fields,omitempty"`
	Loops    []ediLoopMapping     `json:"loops,omitempty"`
}

type ediMapToCanonicalConfig struct {
	OutputField    string               `json:"outputField"`
	TransactionSet string               `json:"transactionSet"`
	Header         []ediFieldMappingRow `json:"header,omitempty"`
	Loops          []ediLoopMapping     `json:"loops,omitempty"`
}

// Execute builds canonical header/loops JSON per the configured mappings and
// writes it to the configured output field.
func (e *EDIMapToCanonicalExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	if e.loader == nil {
		return nil, fmt.Errorf("edi.map_to_canonical: EDI schema is not initialised (schema directory missing)")
	}

	cfg := ediMapToCanonicalConfig{OutputField: "canonicalEDI", TransactionSet: "835"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "canonicalEDI"
	}
	if cfg.TransactionSet == "" {
		cfg.TransactionSet = "835"
	}

	txSet := e.loader.GetTransactionSet(cfg.TransactionSet)
	if txSet == nil {
		return nil, fmt.Errorf("edi.map_to_canonical: unknown transaction set %q", cfg.TransactionSet)
	}

	headerOut := map[string]interface{}{}
	for _, h := range cfg.Header {
		applyEdiFieldMapping(headerOut, inputData, h)
	}

	loopsOut := map[string]interface{}{}
	for _, l := range cfg.Loops {
		buildLoopMapping(loopsOut, l, findSchemaLoop(txSet.Loops, l.LoopID), inputData, inputData)
	}

	canonicalDoc := map[string]interface{}{
		"header": headerOut,
		"loops":  loopsOut,
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, canonicalDoc)

	log.Printf("  ✅ [edi.map_to_canonical] Mapped %d header segment(s), %d top-level loop(s) in %dms",
		len(headerOut), len(loopsOut), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{cfg.OutputField: canonicalDoc},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"loopCount":   len(loopsOut),
		},
	)

	return outputData, nil
}

// findSchemaLoop finds the loop definition matching id among siblings —
// used both for top-level loops (against txSet.Loops) and, recursively, for
// a schema loop's own children (against parent.Loops). Returns nil when the
// mapping config references a loop ID that doesn't exist in the loaded
// schema (a typo, or a future transaction set) — buildLoopMapping treats a
// nil schema loop as non-repeating (the safe default: write at most one
// instance) and logs a warning, rather than guessing or panicking.
func findSchemaLoop(siblings []*edi.X12LoopDef, id string) *edi.X12LoopDef {
	for _, l := range siblings {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// buildLoopMapping resolves one loop mapping config against contextData —
// global inputData at the top level, or the PARENT loop's own current row
// when nested (the local-vs-global distinction cda.map_to_canonical's own
// resolveRelatedRows already establishes for cross-table joins, applied here
// recursively instead of one level) — and writes the result into
// loopsOut[l.LoopID]. The OUTPUT SHAPE (bare map vs []map[string]interface{})
// is decided by schemaLoop.RepeatsMultiple() — see this file's own header
// comment for why that must come from the schema, not from whether RowsPath
// happens to be set.
func buildLoopMapping(loopsOut map[string]interface{}, l ediLoopMapping, schemaLoop *edi.X12LoopDef, contextData map[string]interface{}, inputData map[string]interface{}) {
	if l.LoopID == "" {
		return
	}

	var rows []map[string]interface{}
	if l.RowsPath != "" {
		rows = resolveRows(contextData, l.RowsPath)
	} else {
		rows = []map[string]interface{}{contextData}
	}
	if len(rows) == 0 {
		return
	}

	if schemaLoop != nil && schemaLoop.RepeatsMultiple() {
		instances := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			if inst := buildLoopInstance(l, row, schemaLoop, inputData); inst != nil {
				instances = append(instances, inst)
			}
		}
		if len(instances) > 0 {
			loopsOut[l.LoopID] = instances
		}
		return
	}

	if schemaLoop == nil {
		log.Printf("⚠️  [edi.map_to_canonical] loop %q not found in the loaded schema — treated as single-instance", l.LoopID)
	}
	if inst := buildLoopInstance(l, rows[0], schemaLoop, inputData); inst != nil {
		loopsOut[l.LoopID] = inst
	}
}

// buildLoopInstance builds ONE loop occurrence's segment-keyed field map plus
// (if the config has child loop mappings) a nested "loops" sub-map — the
// exact shape edi.ParseResult.Loops/edi/builder.BuildInput.Loops already
// use. Returns nil when nothing was mapped at all (no fields resolved, no
// child loop produced anything) so an all-empty mapping doesn't leave a
// stray empty entry in the output.
func buildLoopInstance(l ediLoopMapping, row map[string]interface{}, schemaLoop *edi.X12LoopDef, inputData map[string]interface{}) map[string]interface{} {
	inst := map[string]interface{}{}
	for _, f := range l.Fields {
		applyEdiFieldMapping(inst, row, f)
	}
	if len(l.Loops) > 0 {
		var schemaChildren []*edi.X12LoopDef
		if schemaLoop != nil {
			schemaChildren = schemaLoop.Loops
		}
		childLoops := map[string]interface{}{}
		for _, child := range l.Loops {
			buildLoopMapping(childLoops, child, findSchemaLoop(schemaChildren, child.LoopID), row, inputData)
		}
		if len(childLoops) > 0 {
			inst["loops"] = childLoops
		}
	}
	if len(inst) == 0 {
		return nil
	}
	return inst
}

// applyEdiFieldMapping resolves one field row against row and, if a
// non-empty value results (SourcePath first, then LiteralValue — the same
// "first present value wins" convention map_to_canonical_executor.go's own
// applyFieldMapping uses), writes it to target[f.SegmentID][f.ElementKey],
// grouping every field for the same segment into one nested map.
func applyEdiFieldMapping(target map[string]interface{}, row map[string]interface{}, f ediFieldMappingRow) {
	if f.SegmentID == "" || f.ElementKey == "" {
		return
	}
	value := ""
	if f.SourcePath != "" {
		if s, ok := stringifyValue(executors.GetFieldValue(row, f.SourcePath)); ok {
			value = applyCanonicalTransform(f.Transform, s)
		}
	}
	if value == "" && f.LiteralValue != "" {
		value = applyCanonicalTransform(f.Transform, f.LiteralValue)
	}
	if value == "" {
		return
	}
	segMap, ok := target[f.SegmentID].(map[string]interface{})
	if !ok {
		segMap = map[string]interface{}{}
		target[f.SegmentID] = segMap
	}
	segMap[f.ElementKey] = value
}

// Validate checks step configuration. Mapping rows are all optional (a step
// with neither header nor loops configured is a no-op, not an error —
// matching edi.build/cda.map_to_canonical's own permissive Validate).
func (e *EDIMapToCanonicalExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the canonical JSON output for the field picker.
func (e *EDIMapToCanonicalExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Canonical EDI JSON", Path: "_stepOutput.canonicalEDI", DataType: "object",
			Description: "Header/loops-keyed canonical JSON, ready for edi.build", Category: "EDI Transform"},
	}
}
