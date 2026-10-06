// services/executors/transform/astm_parse_executor.go
// ASTMParseExecutor — pipeline step type "astm.parse".
//
// Reads raw ASTM E1394-97 content (already destuffed of its ENQ/STX/ETX/EOT
// transport framing by the owning connector) from a configurable input field
// and produces a structured ParsedJSON map (header/headerComments/
// patientBlocks/queryBlocks/trailer) that downstream steps (astm.validate,
// astm.build) can consume without re-parsing. Mirrors
// ncpdptelecom_parse_executor.go's shape.
//
// ASTM content is already parsed automatically after connector ingestion —
// the same generic path HL7/EDI/NCPDP use (processing/engine_message_processor.go
// -> MessageParserService.ParseToJSON -> FormatDetector.isASTM ->
// ParserFactory.GetParser(models.FormatASTM)) — so this step is NOT required
// to get structured JSON out of an inbound ASTM message; it exists for the
// same reason astm.parse's sibling steps do: re-parsing ASTM content that
// shows up mid-pipeline from somewhere other than the original connector.
//
// Config keys:
//   sourceField — dot-path to the field holding raw ASTM content (default: "raw")
//   outputField — top-level key to write ParsedJSON under (default: "parsedASTM")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/astm"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	astmparser "ezhealthkonnect/services/parsers/astm"
)

// ASTMParseExecutor parses raw ASTM content into a structured ParsedJSON map.
// Self-initialises from the schema directory at construction time.
type ASTMParseExecutor struct {
	*executors.BaseExecutor
	parser *astmparser.ASTMParserService
}

// NewASTMParseExecutor constructs the executor and loads schemas from the
// default schema directory "./astm/schemas/e1394_97".
func NewASTMParseExecutor() *ASTMParseExecutor {
	exec := &ASTMParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.parse", models.ExecutorMetadata{
			Name:        "ASTM E1394-97 Parser (Raw → ParsedJSON)",
			Description: "Parses ASTM E1394-97 lab instrument host-interface content into a structured ParsedJSON document map",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "ASTM Transform",
		}),
	}

	parser, err := astmparser.NewFromSchemaDir("./astm/schemas/e1394_97")
	if err != nil {
		log.Printf("⚠️  [astm.parse] ASTM parser init failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	log.Printf("✅ [astm.parse] ASTMParserService loaded from ./astm/schemas/e1394_97")
	return exec
}

type astmParseConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads raw ASTM content from sourceField, parses it, and writes the
// ParsedJSON map to outputField (default "parsedASTM") in the output data.
func (e *ASTMParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("astm.parse: ASTMParserService is not initialised (schema directory missing)")
	}

	cfg := astmParseConfig{SourceField: "raw", OutputField: "parsedASTM"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedASTM"
	}

	rawContent := resolveASTMRawContent(inputData, cfg.SourceField)
	if rawContent == "" {
		return nil, fmt.Errorf("astm.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	parsed, err := astm.ParseMessage(e.parser.Spec(), "generic_lab_result", rawContent)
	if err != nil {
		return nil, fmt.Errorf("astm.parse: %w", err)
	}

	parsedJSON := map[string]interface{}{
		"header":         parsed.Header,
		"headerComments": parsed.HeaderComments,
		"patientBlocks":  parsed.PatientBlocks,
		"queryBlocks":    parsed.QueryBlocks,
		"trailer":        parsed.Trailer,
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [astm.parse] Parsed ASTM message: %d patient block(s), %d record(s) in %dms",
		len(parsed.PatientBlocks), len(parsed.RecordInstances), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}

	stepOutputVars := map[string]interface{}{"patientBlockCount": len(parsed.PatientBlocks)}
	if cfg.OutputField == "__root__" {
		for k, v := range parsedJSON {
			outputData[k] = v
			stepOutputVars[k] = v
		}
	} else {
		outputData[cfg.OutputField] = parsedJSON
		stepOutputVars[cfg.OutputField] = parsedJSON
	}

	if _, ok := outputData["_format"]; !ok {
		outputData["_format"] = "astm"
	}

	e.SetStepOutputWithDetails(outputData,
		stepOutputVars,
		map[string]interface{}{
			"duration_ms":         durationMs,
			"success":             true,
			"patient_block_count": len(parsed.PatientBlocks),
		},
	)

	return outputData, nil
}

// resolveASTMRawContent reads sourceField from inputData, falling back to
// "raw" directly, and to both of those again inside inputData["message"] —
// the same "prior step's plain output is nested under message for a later
// step" unwrapping fallback every other format's parse executor in this
// codebase already applies.
func resolveASTMRawContent(inputData map[string]interface{}, sourceField string) string {
	if v := executors.GetFieldValue(inputData, sourceField); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if v, ok := inputData["raw"].(string); ok && v != "" {
		return v
	}
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		if v := executors.GetFieldValue(msg, sourceField); v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
		if v, ok := msg["raw"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Validate checks step configuration.
func (e *ASTMParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *ASTMParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed ASTM message", Path: "_stepOutput.parsedASTM", DataType: "object",
			Description: "Structured ASTM document map (header + headerComments + patientBlocks + queryBlocks + trailer)", Category: "ASTM Transform"},
		{Name: "Patient block count", Path: "_stepOutput.patientBlockCount", DataType: "number",
			Description: "Number of P (Patient) record blocks found", Category: "ASTM Transform"},
	}
}
