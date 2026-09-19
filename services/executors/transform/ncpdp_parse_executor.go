// services/executors/transform/ncpdp_parse_executor.go
// NCPDPParseExecutor — pipeline step type "ncpdp.parse".
//
// Reads raw NCPDP SCRIPT (pharmacy e-prescribing) XML content from a
// configurable input field and produces a structured ParsedJSON map
// (transactionType/messageAttrs/header/body) that downstream steps
// (ncpdp.validate, ncpdp.build) can consume without re-parsing. Mirrors
// edi_parse_executor.go's shape exactly — same config keys, same
// message-unwrap fallback.
//
// Config keys:
//   sourceField — dot-path to the field holding raw NCPDP SCRIPT content (default: "raw")
//   outputField — top-level key to write ParsedJSON under (default: "parsedNCPDP")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	ncpdpscriptparser "ezhealthkonnect/services/parsers/ncpdpscript"
)

// NCPDPParseExecutor parses raw NCPDP SCRIPT XML content into a structured
// ParsedJSON map. Self-initialises from the schema directory at
// construction time — same convention as EDIParseExecutor; if the schema
// directory is missing the executor logs a warning and Execute() returns a
// descriptive error.
type NCPDPParseExecutor struct {
	*executors.BaseExecutor
	parser *ncpdpscriptparser.NCPDPScriptParserService
}

// NewNCPDPParseExecutor constructs the executor and loads schemas from the
// default schema directory "./ncpdp/schemas/script_2017071".
func NewNCPDPParseExecutor() *NCPDPParseExecutor {
	exec := &NCPDPParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.parse", models.ExecutorMetadata{
			Name:        "NCPDP SCRIPT Parser (Raw → ParsedJSON)",
			Description: "Parses NCPDP SCRIPT pharmacy e-prescribing XML (Phase 1: NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse) into a structured ParsedJSON document map",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Transform",
		}),
	}

	parser, err := ncpdpscriptparser.NewFromSchemaDir("./ncpdp/schemas/script_2017071")
	if err != nil {
		log.Printf("⚠️  [ncpdp.parse] NCPDP SCRIPT parser init failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	log.Printf("✅ [ncpdp.parse] NCPDPScriptParserService loaded from ./ncpdp/schemas/script_2017071")
	return exec
}

type ncpdpParseConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads raw NCPDP SCRIPT content from sourceField, parses it, and
// writes the ParsedJSON map to outputField (default "parsedNCPDP") in the
// output data.
func (e *NCPDPParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("ncpdp.parse: NCPDPScriptParserService is not initialised (schema directory missing)")
	}

	cfg := ncpdpParseConfig{SourceField: "raw", OutputField: "parsedNCPDP"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedNCPDP"
	}

	rawXML := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawXML, _ = v.(string)
	}
	if rawXML == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawXML = v
		}
	}
	// ExecutePipeline wraps the actual message under inputData["message"] on
	// every step call — mirrors edi_parse_executor.go's own message-
	// unwrapping fallback so ncpdp.parse works whether it's the pipeline's
	// first step or a later one.
	if rawXML == "" {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			if v := executors.GetFieldValue(msg, cfg.SourceField); v != nil {
				rawXML, _ = v.(string)
			}
			if rawXML == "" {
				if v, ok := msg["raw"].(string); ok {
					rawXML = v
				}
			}
		}
	}
	if rawXML == "" {
		return nil, fmt.Errorf("ncpdp.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	result := e.parser.Parse(rawXML)
	if !result.Success {
		return nil, fmt.Errorf("ncpdp.parse: %s", result.Error)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdp.parse] Parsed NCPDP SCRIPT transaction %v: %d fields in %dms",
		result.ParsedJSON["transactionType"], len(result.EnhancedFields), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}

	// stepOutputVars mirrors outputData's own field placement so the
	// steps.{alias}.step_output snapshot always carries the parsed result
	// under the SAME key the real, forwarded pipeline data uses — same
	// corrected-bug precedent edi_parse_executor.go already established.
	stepOutputVars := map[string]interface{}{"fieldCount": len(result.EnhancedFields)}
	if cfg.OutputField == "__root__" {
		for k, v := range result.ParsedJSON {
			outputData[k] = v
			stepOutputVars[k] = v
		}
	} else {
		outputData[cfg.OutputField] = result.ParsedJSON
		stepOutputVars[cfg.OutputField] = result.ParsedJSON
	}

	if _, ok := outputData["_format"]; !ok {
		outputData["_format"] = "ncpdpscript"
	}

	e.SetStepOutputWithDetails(outputData,
		stepOutputVars,
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"field_count": len(result.EnhancedFields),
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *NCPDPParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *NCPDPParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed NCPDP SCRIPT", Path: "_stepOutput.parsedNCPDP", DataType: "object",
			Description: "Structured NCPDP SCRIPT document map (transactionType + messageAttrs + header + body)", Category: "NCPDP Transform"},
		{Name: "Transaction type", Path: "_stepOutput.parsedNCPDP.transactionType", DataType: "string",
			Description: "SCRIPT transaction type, e.g. \"NewRx\"", Category: "NCPDP Transform"},
		{Name: "Field count", Path: "_stepOutput.fieldCount", DataType: "number",
			Description: "Number of flat, path-qualified fields extracted", Category: "NCPDP Transform"},
	}
}
