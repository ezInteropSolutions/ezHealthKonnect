// services/executors/transform/ncpdptelecom_map_to_canonical_executor.go
// NCPDPTelecomMapToCanonicalExecutor — pipeline step type
// "ncpdptelecom.map_to_canonical".
//
// The no-code, format-agnostic on-ramp for building a D.0 transmission from
// data that never went through ncpdptelecom.parse: maps CSV columns, DB
// query columns, or arbitrary JSON fields onto the SAME canonical header/
// transmissionGroup/transactionGroups JSON ncpdptelecom.parse's own
// ParsedJSON and ncpdptelecom/builder.BuildTransmission already use, so
// ncpdptelecom.build can serialize a transmission from a source system with
// zero new Go code — only step configuration. Simpler than
// ncpdp.map_to_canonical/edi.map_to_canonical: D.0 segments have no nested
// sub-groups at all (a flat field list per segment), and there is exactly
// ONE repeating construct (transaction groups — one cluster per claim/drug),
// not an arbitrarily-nestable tree.
//
// Config keys:
//
//	outputField               — dot-path to write the canonical JSON (default: "parsedTelecom")
//	transactionCode           — e.g. "B1" (default: "B1")
//	direction                 — "request" | "response" (default: "request")
//	headerFields              — []{fieldKey, sourcePath, transform?, literalValue?} — the fixed-width
//	                            wire header (binNumber, version, transactionCode, serviceProviderId,
//	                            dateOfService, ...). transactionCode/version default from the
//	                            transactionCode/direction config above and the loaded schema's own
//	                            Version if not otherwise mapped — a real, load-bearing default: an
//	                            unpopulated transactionCode header field produces a transmission with a
//	                            BLANK transaction-code byte on the wire, unparseable by any D.0 reader
//	                            (including this engine's own re-parse) even when every segment's own
//	                            content was mapped correctly — found and fixed via this executor's own
//	                            round-trip test, not assumed safe.
//	transmissionGroupSegments — []{segmentKey, fields: [{fieldKey, sourcePath, transform?, literalValue?}]}
//	                            built ONCE from top-level source data
//	transactionGroupRowsPath  — dot-path to an array of rows, one per claim/drug (optional —
//	                            omitted means "build exactly one transaction group from top-level data")
//	transactionGroupSegments  — same field-row shape as transmissionGroupSegments, applied PER ROW
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/services/executors"
)

// NCPDPTelecomMapToCanonicalExecutor builds canonical D.0 JSON (header/
// transmissionGroup/transactionGroups shape) from configured no-code field
// mappings against arbitrary upstream row data. Self-initialises from the
// schema directory at construction time — same convention as the other
// ncpdptelecom.* executors.
type NCPDPTelecomMapToCanonicalExecutor struct {
	*executors.BaseExecutor
	loader *ncpdptelecom.TelecomSchemaLoader
}

// NewNCPDPTelecomMapToCanonicalExecutor constructs the executor and loads
// the D.0 schema from the default schema directory
// "./ncpdptelecom/schemas/telecom_d0".
func NewNCPDPTelecomMapToCanonicalExecutor() *NCPDPTelecomMapToCanonicalExecutor {
	exec := &NCPDPTelecomMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.map_to_canonical", models.ExecutorMetadata{
			Name:        "Map to Canonical NCPDP Telecom D.0 JSON",
			Description: "No-code field mapping from CSV/DB/generic-JSON rows into the canonical header/transmissionGroup/transactionGroups JSON ncpdptelecom.build consumes",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Telecom Transform",
		}),
	}

	loader, err := ncpdptelecom.NewTelecomSchemaLoader("./ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		log.Printf("⚠️  [ncpdptelecom.map_to_canonical] NCPDP Telecom D.0 schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdptelecom.map_to_canonical] NCPDP Telecom D.0 schema loaded from ./ncpdptelecom/schemas/telecom_d0")
	return exec
}

type ncpdpTelecomFieldMappingRow struct {
	FieldKey     string `json:"fieldKey"`
	SourcePath   string `json:"sourcePath,omitempty"`
	Transform    string `json:"transform,omitempty"`
	LiteralValue string `json:"literalValue,omitempty"`
}

type ncpdpTelecomSegmentMapping struct {
	SegmentKey string                        `json:"segmentKey"`
	Fields     []ncpdpTelecomFieldMappingRow `json:"fields,omitempty"`
}

type ncpdpTelecomMapToCanonicalConfig struct {
	OutputField               string                        `json:"outputField"`
	TransactionCode           string                        `json:"transactionCode"`
	Direction                 string                        `json:"direction"`
	HeaderFields              []ncpdpTelecomFieldMappingRow `json:"headerFields,omitempty"`
	TransmissionGroupSegments []ncpdpTelecomSegmentMapping  `json:"transmissionGroupSegments,omitempty"`
	TransactionGroupRowsPath  string                        `json:"transactionGroupRowsPath,omitempty"`
	TransactionGroupSegments  []ncpdpTelecomSegmentMapping  `json:"transactionGroupSegments,omitempty"`
}

// Execute builds canonical header/transmissionGroup/transactionGroups JSON
// per the configured mappings and writes it to the configured output field.
func (e *NCPDPTelecomMapToCanonicalExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	if e.loader == nil {
		return nil, fmt.Errorf("ncpdptelecom.map_to_canonical: NCPDP Telecom D.0 schema is not initialised (schema directory missing)")
	}

	cfg := ncpdpTelecomMapToCanonicalConfig{OutputField: "parsedTelecom", TransactionCode: "B1", Direction: "request"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedTelecom"
	}
	if cfg.TransactionCode == "" {
		cfg.TransactionCode = "B1"
	}
	if cfg.Direction == "" {
		cfg.Direction = "request"
	}

	// header carries the fixed-width wire fields (BIN number, version,
	// transaction code, service provider ID, date of service, ...) — a
	// REAL, load-bearing gap an earlier draft of this executor omitted
	// entirely: without a populated "transactionCode" header field, the
	// built transmission's own wire bytes carry a blank transaction code,
	// making it unparseable by ANY D.0 reader (including this engine's own
	// re-parse), even though the segment content itself was built
	// correctly. transactionCode/version default from cfg so the common
	// case doesn't need to redundantly configure the same value twice.
	header := map[string]interface{}{}
	for _, f := range cfg.HeaderFields {
		if s := resolveNcpdpTelecomFieldMapping(f, inputData); s != "" {
			header[f.FieldKey] = s
		}
	}
	if _, present := header["transactionCode"]; !present {
		header["transactionCode"] = cfg.TransactionCode
	}
	if _, present := header["version"]; !present {
		header["version"] = e.loader.Spec().Version
	}

	transmissionGroup := map[string]interface{}{}
	for _, seg := range cfg.TransmissionGroupSegments {
		if m := buildNcpdpTelecomSegmentInstance(seg, inputData); m != nil {
			transmissionGroup[seg.SegmentKey] = m
		}
	}

	var rows []map[string]interface{}
	if cfg.TransactionGroupRowsPath != "" {
		rows = resolveRows(inputData, cfg.TransactionGroupRowsPath)
	} else if len(cfg.TransactionGroupSegments) > 0 {
		rows = []map[string]interface{}{inputData}
	}

	transactionGroups := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		cluster := map[string]interface{}{}
		for _, seg := range cfg.TransactionGroupSegments {
			if m := buildNcpdpTelecomSegmentInstance(seg, row); m != nil {
				cluster[seg.SegmentKey] = m
			}
		}
		if len(cluster) > 0 {
			transactionGroups = append(transactionGroups, cluster)
		}
	}

	canonicalDoc := map[string]interface{}{
		"_format":           "ncpdptelecom",
		"transactionCode":   cfg.TransactionCode,
		"direction":         cfg.Direction,
		"header":            header,
		"transmissionGroup": transmissionGroup,
		"transactionGroups": transactionGroups,
	}

	durationMs := time.Since(start).Milliseconds()

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, canonicalDoc)

	log.Printf("  ✅ [ncpdptelecom.map_to_canonical] Mapped %d transmission-group segment(s), %d transaction group(s) in %dms",
		len(transmissionGroup), len(transactionGroups), durationMs)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{cfg.OutputField: canonicalDoc},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
		},
	)

	return outputData, nil
}

// buildNcpdpTelecomSegmentInstance builds ONE segment's field-keyed map from
// row — mirrors ncpdp_map_to_canonical_executor.go's own applyNcpdpFieldMapping
// convention (SourcePath first, then LiteralValue). Returns nil when nothing
// was mapped at all, so an all-empty mapping doesn't leave a stray empty
// entry in the output.
func buildNcpdpTelecomSegmentInstance(seg ncpdpTelecomSegmentMapping, row map[string]interface{}) map[string]interface{} {
	if seg.SegmentKey == "" {
		return nil
	}
	inst := map[string]interface{}{}
	for _, f := range seg.Fields {
		if s := resolveNcpdpTelecomFieldMapping(f, row); s != "" {
			inst[f.FieldKey] = s
		}
	}
	if len(inst) == 0 {
		return nil
	}
	return inst
}

// resolveNcpdpTelecomFieldMapping resolves one field mapping row against
// row (SourcePath first, then LiteralValue — the same "first present value
// wins" convention every *.map_to_canonical executor in this codebase
// uses). Returns "" when f.FieldKey is empty or nothing resolved, so a
// caller can treat an empty return as "don't set this field" uniformly.
func resolveNcpdpTelecomFieldMapping(f ncpdpTelecomFieldMappingRow, row map[string]interface{}) string {
	if f.FieldKey == "" {
		return ""
	}
	if f.SourcePath != "" {
		if s, ok := stringifyValue(executors.GetFieldValue(row, f.SourcePath)); ok {
			if v := applyCanonicalTransform(f.Transform, s); v != "" {
				return v
			}
		}
	}
	if f.LiteralValue != "" {
		return applyCanonicalTransform(f.Transform, f.LiteralValue)
	}
	return ""
}

// Validate checks step configuration. Mapping rows are all optional (a step
// with no segment mappings configured is a no-op, not an error) — matching
// ncpdp.map_to_canonical/edi.map_to_canonical's own permissive Validate.
func (e *NCPDPTelecomMapToCanonicalExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the canonical JSON output for the field picker.
func (e *NCPDPTelecomMapToCanonicalExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Canonical D.0 JSON", Path: "_stepOutput.parsedTelecom", DataType: "object",
			Description: "transactionCode/direction/header/transmissionGroup/transactionGroups-keyed canonical JSON, ready for ncpdptelecom.build", Category: "NCPDP Telecom Transform"},
	}
}
