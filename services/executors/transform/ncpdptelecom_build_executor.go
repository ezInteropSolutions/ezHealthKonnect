// services/executors/transform/ncpdptelecom_build_executor.go
// NCPDPTelecomBuildExecutor — pipeline step type "ncpdptelecom.build".
//
// Builds a complete NCPDP Telecommunication D.0 transmission from canonical
// JSON (the same header/transmissionGroup/transactionGroups shape
// ncpdptelecom.parse's own ParsedJSON produces, so a parse→build round trip
// needs no reshaping) via ncpdptelecom/builder.BuildTransmission — the
// write-direction mirror of ncpdptelecom.parse. Mirrors
// ncpdp_build_executor.go's shape.
//
// Config keys:
//   sourceField     — dot-path to the canonical source data (default: "parsedTelecom")
//   transactionCode — which transaction to build, e.g. "B1" (default: "B1")
//   direction       — "request" | "response" (default: "request")
//   outputField     — dot-path to write the built text (default: "ncpdpTelecom")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	ncpdptelecomBuilder "ezhealthkonnect/ncpdptelecom/builder"
	"ezhealthkonnect/services/executors"
)

// NCPDPTelecomBuildExecutor builds a complete D.0 transmission from
// canonical JSON.
type NCPDPTelecomBuildExecutor struct {
	*executors.BaseExecutor
	loader *ncpdptelecom.TelecomSchemaLoader
}

// NewNCPDPTelecomBuildExecutor constructs the executor and loads the D.0
// schema from the default schema directory
// "./ncpdptelecom/schemas/telecom_d0" — the same self-init-at-construction
// convention ncpdptelecom.parse/ncpdptelecom.validate already use.
func NewNCPDPTelecomBuildExecutor() *NCPDPTelecomBuildExecutor {
	exec := &NCPDPTelecomBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.build", models.ExecutorMetadata{
			Name:        "NCPDP Telecom D.0 Transmission Builder",
			Description: "Builds a complete NCPDP Telecommunication D.0 transmission from canonical JSON (Phase 1: B1 Claim Billing request/response)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Telecom Transform",
		}),
	}

	loader, err := ncpdptelecom.NewTelecomSchemaLoader("./ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		log.Printf("⚠️  [ncpdptelecom.build] NCPDP Telecom D.0 schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdptelecom.build] NCPDP Telecom D.0 schema loaded from ./ncpdptelecom/schemas/telecom_d0")
	return exec
}

// NcpdpTelecomBuildConfig is step.Config's decoded shape for the
// ncpdptelecom.build step.
type NcpdpTelecomBuildConfig struct {
	SourceField     string `json:"sourceField"`
	TransactionCode string `json:"transactionCode"`
	Direction       string `json:"direction"`
	OutputField     string `json:"outputField"`
}

// Execute reads canonical header/transmissionGroup/transactionGroups data,
// builds the D.0 transmission, and writes it to the configured output field.
func (e *NCPDPTelecomBuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("ncpdptelecom.build: NCPDP Telecom D.0 schema is not initialised (schema directory missing)")
	}

	cfg := NcpdpTelecomBuildConfig{SourceField: "parsedTelecom", TransactionCode: "B1", Direction: "request", OutputField: "ncpdpTelecom"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedTelecom"
	}
	if cfg.TransactionCode == "" {
		cfg.TransactionCode = "B1"
	}
	if cfg.Direction == "" {
		cfg.Direction = "request"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ncpdpTelecom"
	}

	source := executors.GetFieldValue(inputData, cfg.SourceField)
	if source == nil {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			source = executors.GetFieldValue(msg, cfg.SourceField)
		}
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("ncpdptelecom.build: source field %q is empty or not an object", cfg.SourceField)
	}

	header := asMap(sourceMap["header"])
	transmissionGroup := asMap(sourceMap["transmissionGroup"])
	transactionGroups := asMapSlice(sourceMap["transactionGroups"])

	direction := cfg.Direction
	if d, ok := sourceMap["direction"].(string); ok && d != "" {
		direction = d
	}
	code := cfg.TransactionCode
	if c, ok := sourceMap["transactionCode"].(string); ok && c != "" {
		code = c
	}

	built, err := ncpdptelecomBuilder.BuildTransmission(e.loader.Spec(), code, direction, header, transmissionGroup, transactionGroups)
	if err != nil {
		return nil, fmt.Errorf("ncpdptelecom.build: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdptelecom.build] Built %s (%s) transmission: %d bytes in %dms", code, direction, len(built), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.SetNestedValue(outputData, cfg.OutputField, built)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			cfg.OutputField: built,
			"byteCount":     len(built),
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"byte_count":  len(built),
		},
	)

	return outputData, nil
}

// asMapSlice coerces v into []interface{} (the type
// ncpdptelecomBuilder.BuildTransmission's own transactionGroups parameter
// expects — deliberately NOT []map[string]interface{}, see
// ncpdptelecom.ParseResult.TransactionGroups's own doc comment for the real
// goja-array-conversion bug that convention exists to avoid), tolerating
// both a native []map[string]interface{} Go slice (constructed in-process,
// e.g. by a prior ncpdptelecom.parse step in the SAME execution) and a
// []interface{} of maps (the shape a JSON round trip through the pipeline
// engine produces).
func asMapSlice(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(t))
		for _, m := range t {
			out = append(out, m)
		}
		return out
	default:
		return nil
	}
}

// Validate checks step configuration.
func (e *NCPDPTelecomBuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built D.0 text output for the field picker.
func (e *NCPDPTelecomBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "D.0 transmission", Path: "_stepOutput.ncpdpTelecom", DataType: "string",
			Description: "Complete built NCPDP Telecommunication D.0 transmission text", Category: "NCPDP Telecom Transform"},
		{Name: "Byte count", Path: "_stepOutput.byteCount", DataType: "number",
			Description: "Length of the built transmission in bytes", Category: "NCPDP Telecom Transform"},
	}
}
