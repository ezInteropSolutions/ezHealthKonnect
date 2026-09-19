// services/executors/transform/ncpdp_validate_executor.go
// NCPDPValidateExecutor — pipeline step type "ncpdp.validate".
//
// Checks a parsed NCPDP SCRIPT message against ncpdp/validator.Validate's
// required-field/group checks. A deliberate DEVIATION from edi.validate's
// own precedent: edi.validate must re-parse from RAW content because
// edi/validator's SyntaxRule checks need the TYPED edi.ParseResult data a
// prior edi.parse step's JSON-shaped output loses. ncpdp.ParseResult has no
// such typed-only data — Header/Body are already plain
// map[string]interface{} trees — so this step defaults to reading a PRIOR
// ncpdp.parse step's own output (sourceField "parsedNCPDP") and reconstructs
// a *ncpdp.ParseResult from it directly, with a "raw" fallback for standalone
// use (no prior ncpdp.parse step in the pipeline).
//
// Config keys:
//   sourceField — dot-path to a prior ncpdp.parse step's ParsedJSON, OR raw
//                 XML content if that's what's found there (default: "parsedNCPDP")
//   outputField — top-level key to write the validation Result under (default: "ncpdpValidation")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/ncpdp/validator"
	"ezhealthkonnect/services/executors"
)

// NCPDPValidateExecutor re-checks a parsed NCPDP SCRIPT message's required
// fields/groups. Self-initialises from the schema directory at construction
// time — same convention as NCPDPParseExecutor.
type NCPDPValidateExecutor struct {
	*executors.BaseExecutor
	loader *ncpdp.NCPDPSchemaLoader
}

// NewNCPDPValidateExecutor constructs the executor and loads schemas from
// the default schema directory "./ncpdp/schemas/script_2017071".
func NewNCPDPValidateExecutor() *NCPDPValidateExecutor {
	exec := &NCPDPValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.validate", models.ExecutorMetadata{
			Name:        "NCPDP SCRIPT Validator",
			Description: "Checks a parsed NCPDP SCRIPT message against the schema's own required field/group flags — see ncpdp/validator's own doc comment for the two-severity model",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Transform",
		}),
	}

	loader, err := ncpdp.NewNCPDPSchemaLoader("./ncpdp/schemas/script_2017071")
	if err != nil {
		log.Printf("⚠️  [ncpdp.validate] NCPDP SCRIPT schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdp.validate] NCPDP SCRIPT schema loaded from ./ncpdp/schemas/script_2017071")
	return exec
}

type ncpdpValidateConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute resolves a *ncpdp.ParseResult (either from a prior ncpdp.parse
// step's ParsedJSON, or by parsing raw XML found at sourceField/"raw"
// directly) and validates it, writing the Result to outputField.
func (e *NCPDPValidateExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("ncpdp.validate: NCPDP SCRIPT schema is not initialised (schema directory missing)")
	}

	cfg := ncpdpValidateConfig{SourceField: "parsedNCPDP", OutputField: "ncpdpValidation"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedNCPDP"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ncpdpValidation"
	}

	parsed, err := resolveNCPDPParseResult(e.loader, inputData, cfg.SourceField)
	if err != nil {
		return nil, fmt.Errorf("ncpdp.validate: %w", err)
	}

	result := validator.Validate(e.loader.Spec(), parsed)

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdp.validate] Validated %s: valid=%v, %d issue(s) in %dms",
		parsed.TransactionType, result.Valid, len(result.Issues), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}

	resultMap := ncpdpValidationResultToMap(result)
	executors.SetNestedValue(outputData, cfg.OutputField, resultMap)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			cfg.OutputField: resultMap,
			"valid":         result.Valid,
			"issueCount":    len(result.Issues),
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"valid":       result.Valid,
			"issue_count": len(result.Issues),
		},
	)

	return outputData, nil
}

// resolveNCPDPParseResult reads sourceField from inputData (with the usual
// "message" envelope-unwrap fallback). If the value is a map already
// shaped like ncpdp.ParserService's own ParsedJSON (has "transactionType"/
// "header"/"body" keys), it's reconstructed directly — no re-parsing. If
// it's a raw XML string instead (sourceField pointed at "raw", or a prior
// step never ran), it's parsed fresh via ncpdp.ParseMessage.
func resolveNCPDPParseResult(loader *ncpdp.NCPDPSchemaLoader, inputData map[string]interface{}, sourceField string) (*ncpdp.ParseResult, error) {
	v := executors.GetFieldValue(inputData, sourceField)
	if v == nil {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			v = executors.GetFieldValue(msg, sourceField)
		}
	}
	if v == nil {
		if raw, ok := inputData["raw"].(string); ok {
			v = raw
		}
	}

	switch t := v.(type) {
	case map[string]interface{}:
		txType, _ := t["transactionType"].(string)
		if txType == "" {
			return nil, fmt.Errorf("source field %q is an object but has no \"transactionType\" — expected a prior ncpdp.parse step's ParsedJSON", sourceField)
		}
		header, _ := t["header"].(map[string]interface{})
		body, _ := t["body"].(map[string]interface{})
		msgAttrs := map[string]string{}
		if rawAttrs, ok := t["messageAttrs"].(map[string]interface{}); ok {
			for k, av := range rawAttrs {
				if s, ok := av.(string); ok {
					msgAttrs[k] = s
				}
			}
		}
		return &ncpdp.ParseResult{TransactionType: txType, MessageAttrs: msgAttrs, Header: header, Body: body}, nil
	case string:
		if t == "" {
			return nil, fmt.Errorf("source field %q is empty", sourceField)
		}
		return ncpdp.ParseMessage(loader.Spec(), t)
	default:
		return nil, fmt.Errorf("source field %q is empty or not a recognized shape", sourceField)
	}
}

func ncpdpValidationResultToMap(result *validator.Result) map[string]interface{} {
	issues := make([]map[string]interface{}, 0, len(result.Issues))
	for _, iss := range result.Issues {
		issues = append(issues, map[string]interface{}{
			"severity": string(iss.Severity),
			"path":     iss.Path,
			"message":  iss.Message,
		})
	}
	return map[string]interface{}{
		"valid":  result.Valid,
		"issues": issues,
	}
}

// Validate checks step configuration.
func (e *NCPDPValidateExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the validation Result output for the field picker.
func (e *NCPDPValidateExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Valid", Path: "_stepOutput.valid", DataType: "boolean",
			Description: "true unless a REQUIRED field/group is missing or empty", Category: "NCPDP Transform"},
		{Name: "Issue count", Path: "_stepOutput.issueCount", DataType: "number",
			Description: "Total number of validation issues found", Category: "NCPDP Transform"},
		{Name: "Validation result", Path: "_stepOutput.ncpdpValidation", DataType: "object",
			Description: "Full validation result: {valid, issues: [{severity, path, message}]}", Category: "NCPDP Transform"},
	}
}
