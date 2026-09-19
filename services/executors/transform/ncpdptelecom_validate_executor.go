// services/executors/transform/ncpdptelecom_validate_executor.go
// NCPDPTelecomValidateExecutor — pipeline step type "ncpdptelecom.validate".
//
// Checks a parsed D.0 transmission against ncpdptelecom/validator.Validate's
// required-field/segment checks. Like ncpdp.validate (and unlike edi.validate),
// ncpdptelecom.ParseResult has no typed-only data a JSON round trip would
// lose — Header/TransmissionGroup/TransactionGroups are already plain
// map[string]interface{} trees — so this step defaults to reading a PRIOR
// ncpdptelecom.parse step's own output, with a "raw" fallback for standalone
// use (no prior parse step in the pipeline).
//
// Config keys:
//   sourceField — dot-path to a prior ncpdptelecom.parse step's ParsedJSON,
//                 OR raw D.0 content if that's what's found there (default: "parsedTelecom")
//   direction   — only consulted when sourceField resolves to RAW content
//                 (a prior parse step's own ParsedJSON already carries its
//                 own "direction"); default "" — auto-sniffed
//   outputField — top-level key to write the validation Result under (default: "telecomValidation")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/ncpdptelecom/validator"
	"ezhealthkonnect/services/executors"
)

// NCPDPTelecomValidateExecutor re-checks a parsed D.0 transmission's
// required fields/segments. Self-initialises from the schema directory at
// construction time — same convention as NCPDPTelecomParseExecutor.
type NCPDPTelecomValidateExecutor struct {
	*executors.BaseExecutor
	loader *ncpdptelecom.TelecomSchemaLoader
}

// NewNCPDPTelecomValidateExecutor constructs the executor and loads schemas
// from the default schema directory "./ncpdptelecom/schemas/telecom_d0".
func NewNCPDPTelecomValidateExecutor() *NCPDPTelecomValidateExecutor {
	exec := &NCPDPTelecomValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.validate", models.ExecutorMetadata{
			Name:        "NCPDP Telecom D.0 Validator",
			Description: "Checks a parsed D.0 transmission against the schema's own required field/segment flags — see ncpdptelecom/validator's own doc comment for the two-severity model",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "NCPDP Telecom Transform",
		}),
	}

	loader, err := ncpdptelecom.NewTelecomSchemaLoader("./ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		log.Printf("⚠️  [ncpdptelecom.validate] NCPDP Telecom D.0 schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [ncpdptelecom.validate] NCPDP Telecom D.0 schema loaded from ./ncpdptelecom/schemas/telecom_d0")
	return exec
}

type ncpdpTelecomValidateConfig struct {
	SourceField string `json:"sourceField"`
	Direction   string `json:"direction"`
	OutputField string `json:"outputField"`
}

// Execute resolves a *ncpdptelecom.ParseResult (either from a prior
// ncpdptelecom.parse step's ParsedJSON, or by parsing raw content found at
// sourceField/"raw" directly) and validates it, writing the Result to
// outputField.
func (e *NCPDPTelecomValidateExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("ncpdptelecom.validate: NCPDP Telecom D.0 schema is not initialised (schema directory missing)")
	}

	cfg := ncpdpTelecomValidateConfig{SourceField: "parsedTelecom", OutputField: "telecomValidation"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedTelecom"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "telecomValidation"
	}

	parsed, err := resolveNCPDPTelecomParseResult(e.loader, inputData, cfg.SourceField, cfg.Direction)
	if err != nil {
		return nil, fmt.Errorf("ncpdptelecom.validate: %w", err)
	}

	result := validator.Validate(e.loader.Spec(), parsed)

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [ncpdptelecom.validate] Validated %s (%s): valid=%v, %d issue(s) in %dms",
		parsed.TransactionCode, parsed.Direction, result.Valid, len(result.Issues), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}

	resultMap := ncpdpTelecomValidationResultToMap(result)
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

// resolveNCPDPTelecomParseResult reads sourceField from inputData (with the
// usual "message" envelope-unwrap fallback). If the value is a map already
// shaped like NCPDPTelecomParseExecutor's own ParsedJSON (has
// "transactionCode"/"header" keys), it's reconstructed directly — no
// re-parsing. If it's a raw content string instead, it's parsed fresh via
// ncpdptelecom.ParseTransmission (direction from config, or sniffed).
func resolveNCPDPTelecomParseResult(loader *ncpdptelecom.TelecomSchemaLoader, inputData map[string]interface{}, sourceField, configDirection string) (*ncpdptelecom.ParseResult, error) {
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
		code, _ := t["transactionCode"].(string)
		if code == "" {
			return nil, fmt.Errorf("source field %q is an object but has no \"transactionCode\" — expected a prior ncpdptelecom.parse step's ParsedJSON", sourceField)
		}
		direction, _ := t["direction"].(string)
		header, _ := t["header"].(map[string]interface{})
		transmissionGroup, _ := t["transmissionGroup"].(map[string]interface{})
		transactionGroups, _ := t["transactionGroups"].([]interface{})
		return &ncpdptelecom.ParseResult{
			TransactionCode:   code,
			Direction:         direction,
			Header:            header,
			TransmissionGroup: transmissionGroup,
			TransactionGroups: transactionGroups,
		}, nil
	case string:
		if t == "" {
			return nil, fmt.Errorf("source field %q is empty", sourceField)
		}
		direction := configDirection
		if direction == "" {
			direction = ncpdptelecom.SniffDirection(loader.Spec(), t)
		}
		return ncpdptelecom.ParseTransmission(loader.Spec(), direction, t)
	default:
		return nil, fmt.Errorf("source field %q is empty or not a recognized shape", sourceField)
	}
}

func ncpdpTelecomValidationResultToMap(result *validator.Result) map[string]interface{} {
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
func (e *NCPDPTelecomValidateExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the validation Result output for the field picker.
func (e *NCPDPTelecomValidateExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Valid", Path: "_stepOutput.valid", DataType: "boolean",
			Description: "true unless a REQUIRED field/segment is missing or empty", Category: "NCPDP Telecom Transform"},
		{Name: "Issue count", Path: "_stepOutput.issueCount", DataType: "number",
			Description: "Total number of validation issues found", Category: "NCPDP Telecom Transform"},
		{Name: "Validation result", Path: "_stepOutput.telecomValidation", DataType: "object",
			Description: "Full validation result: {valid, issues: [{severity, path, message}]}", Category: "NCPDP Telecom Transform"},
	}
}
