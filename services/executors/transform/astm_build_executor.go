// services/executors/transform/astm_build_executor.go
// ASTMBuildExecutor — pipeline step type "astm.build".
//
// Builds a complete ASTM E1394-97 record stream from canonical JSON (the
// same header/headerComments/patientBlocks/queryBlocks/trailer shape
// astm.parse's own ParsedJSON produces, so a parse->build round trip needs
// no reshaping) via astm/builder.BuildDocument — the write-direction mirror
// of astm.parse. Mirrors ncpdptelecom_build_executor.go's shape.
//
// Output is the destuffed record stream only (records joined by CR) — ENQ/
// STX/checksum/EOT framing is added by the owning connector
// (services/connectors/astm_framing.go) at send time, not by this step.
//
// Config keys:
//   sourceField — dot-path to the canonical source data (default: "parsedASTM")
//   outputField — dot-path to write the built text (default: "astmMessage")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	astmbuilder "ezhealthkonnect/astm/builder"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	astmparser "ezhealthkonnect/services/parsers/astm"
)

// ASTMBuildExecutor builds a complete ASTM record stream from canonical
// JSON. Self-initialises from the schema directory at construction time.
type ASTMBuildExecutor struct {
	*executors.BaseExecutor
	parser *astmparser.ASTMParserService
}

// NewASTMBuildExecutor constructs the executor and loads the ASTM schema
// from the default schema directory "./astm/schemas/e1394_97" — the same
// self-init-at-construction convention astm.parse/astm.validate already use.
func NewASTMBuildExecutor() *ASTMBuildExecutor {
	exec := &ASTMBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.build", models.ExecutorMetadata{
			Name:        "ASTM E1394-97 Message Builder",
			Description: "Builds a complete ASTM E1394-97 record stream from canonical JSON (header/patientBlocks/orders/results/trailer)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "ASTM Transform",
		}),
	}

	parser, err := astmparser.NewFromSchemaDir("./astm/schemas/e1394_97")
	if err != nil {
		log.Printf("⚠️  [astm.build] ASTM schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	log.Printf("✅ [astm.build] ASTM schema loaded from ./astm/schemas/e1394_97")
	return exec
}

// AstmBuildConfig is step.Config's decoded shape for the astm.build step.
type AstmBuildConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads canonical header/patientBlocks/queryBlocks/trailer data,
// builds the ASTM record stream, and writes it to the configured output
// field.
func (e *ASTMBuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("astm.build: ASTM schema is not initialised (schema directory missing)")
	}

	cfg := AstmBuildConfig{SourceField: "parsedASTM", OutputField: "astmMessage"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedASTM"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "astmMessage"
	}

	source := executors.GetFieldValue(inputData, cfg.SourceField)
	if source == nil {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			source = executors.GetFieldValue(msg, cfg.SourceField)
		}
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("astm.build: source field %q is empty or not an object", cfg.SourceField)
	}

	built, err := astmbuilder.BuildDocument(e.parser.Spec(), astmbuilder.BuildInput{
		MessageProfile: "generic_lab_result",
		Header:         asMap(sourceMap["header"]),
		HeaderComments: toMapSliceForValidate(sourceMap["headerComments"]),
		PatientBlocks:  toMapSliceForValidate(sourceMap["patientBlocks"]),
		QueryBlocks:    toMapSliceForValidate(sourceMap["queryBlocks"]),
		Trailer:        asMap(sourceMap["trailer"]),
	})
	if err != nil {
		return nil, fmt.Errorf("astm.build: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [astm.build] Built ASTM message: %d bytes in %dms", len(built), durationMs)

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
func (e *ASTMBuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built ASTM text output for the field picker.
func (e *ASTMBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "ASTM message", Path: "_stepOutput.astmMessage", DataType: "string",
			Description: "Complete built ASTM E1394-97 record stream text", Category: "ASTM Transform"},
		{Name: "Byte count", Path: "_stepOutput.byteCount", DataType: "number",
			Description: "Length of the built message in bytes", Category: "ASTM Transform"},
	}
}
