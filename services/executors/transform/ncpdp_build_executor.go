// services/executors/transform/ncpdp_build_executor.go
// NCPDPBuildExecutor — pipeline step type "ncpdp.build".
//
// Builds a complete NCPDP SCRIPT XML message from canonical JSON (the same
// header/body shape ncpdp.parse's own ParsedJSON produces, so a parse→build
// round trip needs no reshaping) via ncpdp/builder.BuildDocument — the
// write-direction mirror of ncpdp.parse. Mirrors edi_build_executor.go's
// shape.
//
// Config keys:
//   sourceField     — dot-path to the canonical source data (default: "parsedNCPDP")
//   transactionType — which transaction to build, e.g. "NewRx" (default: "NewRx")
//   outputField     — dot-path to write the built XML text (default: "ncpdpScript")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	ncpdpBuilder "ezhealthkonnect/ncpdp/builder"
	"ezhealthkonnect/services/executors"
)

// NCPDPBuildExecutor builds a complete NCPDP SCRIPT XML message from
// canonical JSON.
type NCPDPBuildExecutor struct {
	*executors.BaseExecutor
	loader *ncpdp.NCPDPSchemaLoader
}

// NewNCPDPBuildExecutor constructs the executor and loads the NCPDP schema
// from the default schema directory "./ncpdp/schemas/script_2017071" — the
// same self-init-at-construction convention ncpdp.parse/ncpdp.validate
// already use.
func NewNCPDPBuildExecutor() *NCPDPBuildExecutor {
	exec := &NCPDPBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.build", models.ExecutorMetadata{
			Name:        "NCPDP SCRIPT Document Builder",
			Description: "Builds a complete NCPDP SCRIPT XML message from canonical JSON (Phase 1: NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Transform",
		}),
	}

	loader, err := ncpdp.NewNCPDPSchemaLoader("./ncpdp/schemas/script_2017071")
	if err != nil {
		log.Printf("⚠️  [ncpdp.build] NCPDP SCRIPT schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdp.build] NCPDP SCRIPT schema loaded from ./ncpdp/schemas/script_2017071")
	return exec
}

// NcpdpBuildConfig is step.Config's decoded shape for the ncpdp.build step.
// Exported, matching EdiBuildConfig/CdaBuildConfig's own precedent.
type NcpdpBuildConfig struct {
	SourceField     string `json:"sourceField"`
	TransactionType string `json:"transactionType"`
	OutputField     string `json:"outputField"`
}

// Execute reads canonical header/body data, builds the NCPDP SCRIPT
// message, and writes it to the configured output field.
func (e *NCPDPBuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("ncpdp.build: NCPDP SCRIPT schema is not initialised (schema directory missing)")
	}

	cfg := NcpdpBuildConfig{SourceField: "parsedNCPDP", TransactionType: "NewRx", OutputField: "ncpdpScript"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedNCPDP"
	}
	if cfg.TransactionType == "" {
		cfg.TransactionType = "NewRx"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ncpdpScript"
	}

	source := executors.GetFieldValue(inputData, cfg.SourceField)
	if source == nil {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			source = executors.GetFieldValue(msg, cfg.SourceField)
		}
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("ncpdp.build: source field %q is empty or not an object", cfg.SourceField)
	}

	header := asMap(sourceMap["header"])
	body := asMap(sourceMap["body"])
	// Tolerate a source shaped as bare {header, body} without the
	// transactionType/messageAttrs wrapper too (e.g. a hand-authored
	// ncpdp.map_to_canonical output), by falling back to the source map
	// itself as the body when no nested "body" key is present.
	if len(body) == 0 && len(sourceMap) > 0 {
		if _, hasHeader := sourceMap["header"]; !hasHeader {
			body = sourceMap
		}
	}

	built, err := ncpdpBuilder.BuildDocument(e.loader.Spec(), cfg.TransactionType, header, body)
	if err != nil {
		return nil, fmt.Errorf("ncpdp.build: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdp.build] Built %s message: %d bytes in %dms", cfg.TransactionType, len(built), durationMs)

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

// Validate checks step configuration.
func (e *NCPDPBuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built NCPDP SCRIPT text output for the field picker.
func (e *NCPDPBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "NCPDP SCRIPT document", Path: "_stepOutput.ncpdpScript", DataType: "string",
			Description: "Complete built NCPDP SCRIPT XML message text", Category: "NCPDP Transform"},
		{Name: "Byte count", Path: "_stepOutput.byteCount", DataType: "number",
			Description: "Length of the built document in bytes", Category: "NCPDP Transform"},
	}
}
