// services/executors/transform/edi_parse_executor.go
// EDIParseExecutor — pipeline step type "edi.parse".
//
// Reads raw X12 EDI content from a configurable input field and produces a
// structured ParsedJSON map (interchange/header/loops/trailer) that
// downstream steps (edi.validate, edi.map_to_canonical in a later phase)
// can consume without re-parsing. Mirrors cda_parse_executor.go's shape
// exactly — same config keys, same message-unwrap fallback.
//
// Config keys:
//   sourceField — dot-path to the field holding raw EDI content (default: "raw")
//   outputField — top-level key to write ParsedJSON under (default: "parsedEDI")
//   schemaDir   — filesystem path to the EDI schema directory (default: "./edi/schemas/x12_005010")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	edix12parser "ezhealthkonnect/services/parsers/edix12"
)

// EDIParseExecutor parses raw X12 EDI content into a structured ParsedJSON map.
// Self-initialises from the schema directory at construction time — same
// convention as CDAParseExecutor; if the schema directory is missing the
// executor logs a warning and Execute() returns a descriptive error.
type EDIParseExecutor struct {
	*executors.BaseExecutor
	parser *edix12parser.EDIX12ParserService
}

// NewEDIParseExecutor constructs the executor and loads schemas from the
// default schema directory "./edi/schemas/x12_005010".
func NewEDIParseExecutor() *EDIParseExecutor {
	exec := &EDIParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.parse", models.ExecutorMetadata{
			Name:        "EDI X12 Parser (Raw → ParsedJSON)",
			Description: "Parses X12 EDI content (835 phase 1) into a structured ParsedJSON document map",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "EDI Transform",
		}),
	}

	parser, err := edix12parser.NewFromSchemaDir("./edi/schemas/x12_005010")
	if err != nil {
		log.Printf("⚠️  [edi.parse] EDI X12 parser init failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	log.Printf("✅ [edi.parse] EDIX12ParserService loaded from ./edi/schemas/x12_005010")
	return exec
}

type ediParseConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads raw EDI content from sourceField, parses it, and writes the
// ParsedJSON map to outputField (default "parsedEDI") in the output data.
func (e *EDIParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("edi.parse: EDIX12ParserService is not initialised (schema directory missing)")
	}

	cfg := ediParseConfig{SourceField: "raw", OutputField: "parsedEDI"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedEDI"
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
	// ExecutePipeline wraps the actual message under inputData["message"] on
	// every step call — mirrors cda_parse_executor.go's own message-
	// unwrapping fallback so edi.parse works whether it's the pipeline's
	// first step or a later one.
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
		return nil, fmt.Errorf("edi.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	result := e.parser.Parse(rawEDI)
	if !result.Success {
		return nil, fmt.Errorf("edi.parse: %s", result.Error)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [edi.parse] Parsed EDI transaction set %v: %d fields in %dms",
		result.ParsedJSON["transactionSet"], len(result.EnhancedFields), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}

	if cfg.OutputField == "__root__" {
		for k, v := range result.ParsedJSON {
			outputData[k] = v
		}
	} else {
		outputData[cfg.OutputField] = result.ParsedJSON
	}

	if _, ok := outputData["_format"]; !ok {
		outputData["_format"] = "edi"
	}

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"parsedEDI":  result.ParsedJSON,
			"fieldCount": len(result.EnhancedFields),
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"field_count": len(result.EnhancedFields),
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *EDIParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *EDIParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed EDI", Path: "_stepOutput.parsedEDI", DataType: "object",
			Description: "Structured X12 EDI document map (interchange + header + loops + trailer)", Category: "EDI Transform"},
		{Name: "Transaction set", Path: "_stepOutput.parsedEDI.transactionSet", DataType: "string",
			Description: "X12 transaction set identifier, e.g. \"835\"", Category: "EDI Transform"},
		{Name: "Field count", Path: "_stepOutput.fieldCount", DataType: "number",
			Description: "Number of flat, loop-qualified fields extracted", Category: "EDI Transform"},
	}
}
