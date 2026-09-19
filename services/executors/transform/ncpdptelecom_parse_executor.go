// services/executors/transform/ncpdptelecom_parse_executor.go
// NCPDPTelecomParseExecutor — pipeline step type "ncpdptelecom.parse".
//
// Reads raw NCPDP Telecommunication D.0 (real-time pharmacy claims) content
// from a configurable input field and produces a structured ParsedJSON map
// (transactionCode/direction/header/transmissionGroup/transactionGroups)
// that downstream steps (ncpdptelecom.validate, ncpdptelecom.build) can
// consume without re-parsing. Mirrors ncpdp_parse_executor.go's shape.
//
// Config keys:
//   sourceField — dot-path to the field holding raw D.0 content (default: "raw")
//   direction   — "request" | "response" (default: "" — auto-sniffed via
//                 ncpdptelecom.SniffDirection, since request and response
//                 share the identical wire transaction code; an explicit
//                 config value is always preferred when the caller knows it)
//   outputField — top-level key to write ParsedJSON under (default: "parsedTelecom")

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
	ncpdptelecomparser "ezhealthkonnect/services/parsers/ncpdptelecom"
)

// NCPDPTelecomParseExecutor parses raw D.0 content into a structured
// ParsedJSON map. Self-initialises from the schema directory at
// construction time — same convention as NCPDPParseExecutor.
type NCPDPTelecomParseExecutor struct {
	*executors.BaseExecutor
	parser *ncpdptelecomparser.NCPDPTelecomParserService
}

// NewNCPDPTelecomParseExecutor constructs the executor and loads schemas
// from the default schema directory "./ncpdptelecom/schemas/telecom_d0".
func NewNCPDPTelecomParseExecutor() *NCPDPTelecomParseExecutor {
	exec := &NCPDPTelecomParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.parse", models.ExecutorMetadata{
			Name:        "NCPDP Telecom D.0 Parser (Raw → ParsedJSON)",
			Description: "Parses NCPDP Telecommunication D.0 real-time pharmacy claim content (Phase 1: B1 Claim Billing request/response) into a structured ParsedJSON document map",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Telecom Transform",
		}),
	}

	parser, err := ncpdptelecomparser.NewFromSchemaDir("./ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		log.Printf("⚠️  [ncpdptelecom.parse] NCPDP Telecom D.0 parser init failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	log.Printf("✅ [ncpdptelecom.parse] NCPDPTelecomParserService loaded from ./ncpdptelecom/schemas/telecom_d0")
	return exec
}

type ncpdpTelecomParseConfig struct {
	SourceField string `json:"sourceField"`
	Direction   string `json:"direction"`
	OutputField string `json:"outputField"`
}

// Execute reads raw D.0 content from sourceField, parses it, and writes the
// ParsedJSON map to outputField (default "parsedTelecom") in the output data.
func (e *NCPDPTelecomParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("ncpdptelecom.parse: NCPDPTelecomParserService is not initialised (schema directory missing)")
	}

	cfg := ncpdpTelecomParseConfig{SourceField: "raw", OutputField: "parsedTelecom"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedTelecom"
	}

	rawContent := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawContent, _ = v.(string)
	}
	if rawContent == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawContent = v
		}
	}
	// ExecutePipeline wraps the actual message under inputData["message"] on
	// every step call — mirrors ncpdp_parse_executor.go's own message-
	// unwrapping fallback.
	if rawContent == "" {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			if v := executors.GetFieldValue(msg, cfg.SourceField); v != nil {
				rawContent, _ = v.(string)
			}
			if rawContent == "" {
				if v, ok := msg["raw"].(string); ok {
					rawContent = v
				}
			}
		}
	}
	if rawContent == "" {
		return nil, fmt.Errorf("ncpdptelecom.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	direction := cfg.Direction
	if direction == "" {
		direction = ncpdptelecom.SniffDirection(e.parser.Spec(), rawContent)
	}

	parsed, err := ncpdptelecom.ParseTransmission(e.parser.Spec(), direction, rawContent)
	if err != nil {
		return nil, fmt.Errorf("ncpdptelecom.parse: %w", err)
	}

	parsedJSON := map[string]interface{}{
		"transactionCode":   parsed.TransactionCode,
		"direction":         parsed.Direction,
		"header":            parsed.Header,
		"transmissionGroup": parsed.TransmissionGroup,
		"transactionGroups": parsed.TransactionGroups,
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdptelecom.parse] Parsed D.0 transaction %s (%s): %d transaction group(s) in %dms",
		parsed.TransactionCode, parsed.Direction, len(parsed.TransactionGroups), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}

	stepOutputVars := map[string]interface{}{"transactionGroupCount": len(parsed.TransactionGroups)}
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
		outputData["_format"] = "ncpdptelecom"
	}

	e.SetStepOutputWithDetails(outputData,
		stepOutputVars,
		map[string]interface{}{
			"duration_ms":             durationMs,
			"success":                 true,
			"transaction_group_count": len(parsed.TransactionGroups),
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *NCPDPTelecomParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *NCPDPTelecomParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed D.0 transmission", Path: "_stepOutput.parsedTelecom", DataType: "object",
			Description: "Structured D.0 document map (transactionCode + direction + header + transmissionGroup + transactionGroups)", Category: "NCPDP Telecom Transform"},
		{Name: "Transaction code", Path: "_stepOutput.parsedTelecom.transactionCode", DataType: "string",
			Description: "D.0 transaction code, e.g. \"B1\"", Category: "NCPDP Telecom Transform"},
		{Name: "Transaction group count", Path: "_stepOutput.transactionGroupCount", DataType: "number",
			Description: "Number of GS-delimited transaction-group clusters (one per claim/drug)", Category: "NCPDP Telecom Transform"},
	}
}
