// services/executors/transform/ncpdp_map_to_canonical_executor.go
// NCPDPMapToCanonicalExecutor — pipeline step type "ncpdp.map_to_canonical".
//
// The no-code, format-agnostic on-ramp for building an NCPDP SCRIPT message
// from data that never went through ncpdp.parse: maps CSV columns, DB query
// columns, or arbitrary JSON fields onto the SAME canonical header/body JSON
// ncpdp.parse's own ParsedJSON and ncpdp/builder.BuildDocument already use,
// so ncpdp.build can serialize a message from a source system with zero new
// Go code — only step configuration. Mirrors edi.map_to_canonical's shape,
// adapted for NCPDP's own tree: a group's Repeatable flag is declared
// directly on its NCPDPGroupRef (unlike EDI's X12LoopDef.RepeatsMultiple(),
// no separate method call needed), and there are two independent top-level
// trees (Header, Body) rather than EDI's one (header + loops).
//
// Config keys:
//
//	outputField     — dot-path to write the canonical JSON (default: "parsedNCPDP")
//	transactionType — which transaction's group tree to consult, e.g. "NewRx" (default: "NewRx")
//	headerFields    — []{fieldKey, sourcePath, transform?, literalValue?} — flat Header fields
//	headerGroups    — []{groupKey, rowsPath?, fields, groups?} — nested Header groups (e.g. security)
//	bodyFields      — same shape as headerFields, for fields directly on the transaction body
//	bodyGroups      — same shape as headerGroups, for the transaction's own nested groups
//	                  (patient, pharmacy, prescriber, medicationPrescribed, ...), recursively
//	                  nestable to match NCPDPGroupDef's own tree shape.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/services/executors"
)

// NCPDPMapToCanonicalExecutor builds canonical NCPDP JSON (header/body
// shape) from configured no-code field mappings against arbitrary upstream
// row data. Self-initialises from the schema directory at construction
// time — same convention as the other ncpdp.* executors.
type NCPDPMapToCanonicalExecutor struct {
	*executors.BaseExecutor
	loader *ncpdp.NCPDPSchemaLoader
}

// NewNCPDPMapToCanonicalExecutor constructs the executor and loads the
// NCPDP schema from the default schema directory
// "./ncpdp/schemas/script_2017071".
func NewNCPDPMapToCanonicalExecutor() *NCPDPMapToCanonicalExecutor {
	exec := &NCPDPMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.map_to_canonical", models.ExecutorMetadata{
			Name:        "Map to Canonical NCPDP SCRIPT JSON",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into the canonical header/body JSON ncpdp.build consumes",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Transform",
		}),
	}

	loader, err := ncpdp.NewNCPDPSchemaLoader("./ncpdp/schemas/script_2017071")
	if err != nil {
		log.Printf("⚠️  [ncpdp.map_to_canonical] NCPDP SCRIPT schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdp.map_to_canonical] NCPDP SCRIPT schema loaded from ./ncpdp/schemas/script_2017071")
	return exec
}

// ncpdpFieldMappingRow maps one canonical field to a source value.
type ncpdpFieldMappingRow struct {
	FieldKey     string `json:"fieldKey"`
	SourcePath   string `json:"sourcePath,omitempty"`
	Transform    string `json:"transform,omitempty"`
	LiteralValue string `json:"literalValue,omitempty"`
}

// ncpdpGroupMapping is one group's mapping config, recursively nestable to
// match NCPDPGroupDef's own tree shape.
type ncpdpGroupMapping struct {
	GroupKey string                 `json:"groupKey"`
	RowsPath string                 `json:"rowsPath,omitempty"`
	Fields   []ncpdpFieldMappingRow `json:"fields,omitempty"`
	Groups   []ncpdpGroupMapping    `json:"groups,omitempty"`
}

type ncpdpMapToCanonicalConfig struct {
	OutputField     string                 `json:"outputField"`
	TransactionType string                 `json:"transactionType"`
	HeaderFields    []ncpdpFieldMappingRow `json:"headerFields,omitempty"`
	HeaderGroups    []ncpdpGroupMapping    `json:"headerGroups,omitempty"`
	BodyFields      []ncpdpFieldMappingRow `json:"bodyFields,omitempty"`
	BodyGroups      []ncpdpGroupMapping    `json:"bodyGroups,omitempty"`
}

// Execute builds canonical header/body JSON per the configured mappings and
// writes it to the configured output field.
func (e *NCPDPMapToCanonicalExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	if e.loader == nil {
		return nil, fmt.Errorf("ncpdp.map_to_canonical: NCPDP SCRIPT schema is not initialised (schema directory missing)")
	}

	cfg := ncpdpMapToCanonicalConfig{OutputField: "parsedNCPDP", TransactionType: "NewRx"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedNCPDP"
	}
	if cfg.TransactionType == "" {
		cfg.TransactionType = "NewRx"
	}

	spec := e.loader.Spec()
	tx, ok := spec.Transactions[cfg.TransactionType]
	if !ok {
		return nil, fmt.Errorf("ncpdp.map_to_canonical: unknown transaction type %q", cfg.TransactionType)
	}
	hdr := spec.Header()

	headerOut := map[string]interface{}{}
	for _, f := range cfg.HeaderFields {
		applyNcpdpFieldMapping(headerOut, inputData, f)
	}
	var headerGroupRefs []ncpdp.NCPDPGroupRef
	if hdr != nil {
		headerGroupRefs = hdr.Groups
	}
	for _, g := range cfg.HeaderGroups {
		buildNcpdpGroupMapping(spec, headerOut, g, findGroupRef(headerGroupRefs, g.GroupKey), inputData, inputData)
	}

	bodyOut := map[string]interface{}{}
	for _, f := range cfg.BodyFields {
		applyNcpdpFieldMapping(bodyOut, inputData, f)
	}
	for _, g := range cfg.BodyGroups {
		buildNcpdpGroupMapping(spec, bodyOut, g, findGroupRef(tx.Groups, g.GroupKey), inputData, inputData)
	}

	canonicalDoc := map[string]interface{}{
		"_format":         "ncpdpscript",
		"transactionType": cfg.TransactionType,
		"header":          headerOut,
		"body":            bodyOut,
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, canonicalDoc)

	log.Printf("  ✅ [ncpdp.map_to_canonical] Mapped %d header field(s), %d body field(s)/group(s) in %dms",
		len(headerOut), len(bodyOut), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{cfg.OutputField: canonicalDoc},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
		},
	)

	return outputData, nil
}

// findGroupRef finds the group-ref matching key among siblings — used both
// for top-level groups (against a Header's or transaction's own Groups) and,
// recursively, for a schema group's own children. Returns nil when the
// mapping config references a groupKey that doesn't exist in the loaded
// schema (a typo, or a future transaction type); buildNcpdpGroupMapping
// treats a nil ref as non-repeatable (the safe default) and logs a warning,
// rather than guessing or panicking.
func findGroupRef(refs []ncpdp.NCPDPGroupRef, key string) *ncpdp.NCPDPGroupRef {
	for i := range refs {
		if refs[i].Key == key {
			return &refs[i]
		}
	}
	return nil
}

// buildNcpdpGroupMapping resolves one group mapping config against
// contextData — global inputData at the top level, or the PARENT group's own
// current row when nested — and writes the result into out[g.GroupKey]. The
// OUTPUT SHAPE (bare map vs []interface{}) is decided by ref.Repeatable
// (schema-declared directly on the ref, unlike EDI's separate
// RepeatsMultiple() method call) — never by whether RowsPath happens to be set.
func buildNcpdpGroupMapping(spec *ncpdp.NCPDPSpecDef, out map[string]interface{}, g ncpdpGroupMapping, ref *ncpdp.NCPDPGroupRef, contextData map[string]interface{}, inputData map[string]interface{}) {
	if g.GroupKey == "" {
		return
	}

	var rows []map[string]interface{}
	if g.RowsPath != "" {
		rows = resolveRows(contextData, g.RowsPath)
	} else {
		rows = []map[string]interface{}{contextData}
	}
	if len(rows) == 0 {
		return
	}

	var childGroup *ncpdp.NCPDPGroupDef
	if ref != nil {
		childGroup = spec.Groups[ref.GroupKey]
	}

	if ref != nil && ref.Repeatable {
		instances := make([]interface{}, 0, len(rows))
		for _, row := range rows {
			if inst := buildNcpdpGroupInstance(spec, g, row, childGroup, inputData); inst != nil {
				instances = append(instances, inst)
			}
		}
		if len(instances) > 0 {
			out[g.GroupKey] = instances
		}
		return
	}

	if ref == nil {
		log.Printf("⚠️  [ncpdp.map_to_canonical] group %q not found in the loaded schema — treated as non-repeatable", g.GroupKey)
	}
	if inst := buildNcpdpGroupInstance(spec, g, rows[0], childGroup, inputData); inst != nil {
		out[g.GroupKey] = inst
	}
}

// buildNcpdpGroupInstance builds ONE group occurrence's field-keyed map plus
// (if the config has child group mappings) recursively-mapped nested groups
// — the exact shape ncpdp.ParseResult.Body/ncpdp/builder's own canonical
// input already use. Returns nil when nothing was mapped at all, so an
// all-empty mapping doesn't leave a stray empty entry in the output.
func buildNcpdpGroupInstance(spec *ncpdp.NCPDPSpecDef, g ncpdpGroupMapping, row map[string]interface{}, childGroup *ncpdp.NCPDPGroupDef, inputData map[string]interface{}) map[string]interface{} {
	inst := map[string]interface{}{}
	for _, f := range g.Fields {
		applyNcpdpFieldMapping(inst, row, f)
	}
	var childRefs []ncpdp.NCPDPGroupRef
	if childGroup != nil {
		childRefs = childGroup.Groups
	}
	for _, child := range g.Groups {
		buildNcpdpGroupMapping(spec, inst, child, findGroupRef(childRefs, child.GroupKey), row, inputData)
	}
	if len(inst) == 0 {
		return nil
	}
	return inst
}

// applyNcpdpFieldMapping resolves one field row against row and, if a
// non-empty value results (SourcePath first, then LiteralValue — the same
// "first present value wins" convention edi.map_to_canonical's own
// applyEdiFieldMapping uses), writes it to target[f.FieldKey].
func applyNcpdpFieldMapping(target map[string]interface{}, row map[string]interface{}, f ncpdpFieldMappingRow) {
	if f.FieldKey == "" {
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
	target[f.FieldKey] = value
}

// Validate checks step configuration. Mapping rows are all optional (a step
// with neither header nor body mappings configured is a no-op, not an
// error) — matching edi.map_to_canonical/cda.map_to_canonical's own
// permissive Validate.
func (e *NCPDPMapToCanonicalExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the canonical JSON output for the field picker.
func (e *NCPDPMapToCanonicalExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Canonical NCPDP SCRIPT JSON", Path: "_stepOutput.parsedNCPDP", DataType: "object",
			Description: "transactionType/header/body-keyed canonical JSON, ready for ncpdp.build", Category: "NCPDP Transform"},
	}
}
