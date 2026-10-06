// services/executors/transform/astm_validate_executor.go
// ASTMValidateExecutor — pipeline step type "astm.validate".
//
// Checks a parsed ASTM message against astm/validator.Validate's field-value
// checks. Like ncpdp.validate/ncpdptelecom.validate (and unlike edi.validate),
// astm.ParseResult has no typed-only data a JSON round trip would lose —
// Header/PatientBlocks/etc. are already plain map[string]interface{} trees —
// so this step defaults to reading a PRIOR astm.parse step's own output,
// with a "raw" fallback for standalone use (no prior parse step in the
// pipeline).
//
// Config keys:
//   sourceField — dot-path to a prior astm.parse step's ParsedJSON, OR raw
//                 ASTM content if that's what's found there (default: "parsedASTM")
//   outputField — top-level key to write the validation Result under (default: "astmValidation")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/astm"
	"ezhealthkonnect/astm/validator"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// ASTMValidateExecutor re-checks a parsed ASTM message's field values.
// Self-initialises from the schema directory at construction time.
type ASTMValidateExecutor struct {
	*executors.BaseExecutor
	loader *astm.ASTMSchemaLoader
}

// NewASTMValidateExecutor constructs the executor and loads schemas from the
// default schema directory "./astm/schemas/e1394_97".
func NewASTMValidateExecutor() *ASTMValidateExecutor {
	exec := &ASTMValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.validate", models.ExecutorMetadata{
			Name:        "ASTM E1394-97 Validator",
			Description: "Re-checks every recorded ASTM field against its own declared data type — see astm/validator's own doc comment for the scope (value conformance only, never blocking on required-but-missing fields)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "ASTM Transform",
		}),
	}

	loader, err := astm.NewASTMSchemaLoader("./astm/schemas/e1394_97")
	if err != nil {
		log.Printf("⚠️  [astm.validate] ASTM schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [astm.validate] ASTM schema loaded from ./astm/schemas/e1394_97")
	return exec
}

type astmValidateConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute resolves a *astm.ParseResult (either from a prior astm.parse
// step's ParsedJSON, or by parsing raw content found at sourceField/"raw"
// directly) and validates it, writing the Result to outputField.
func (e *ASTMValidateExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("astm.validate: ASTM schema is not initialised (schema directory missing)")
	}

	cfg := astmValidateConfig{SourceField: "parsedASTM", OutputField: "astmValidation"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedASTM"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "astmValidation"
	}

	parsed, err := resolveASTMParseResult(e.loader, inputData, cfg.SourceField)
	if err != nil {
		return nil, fmt.Errorf("astm.validate: %w", err)
	}

	result := validator.Validate(e.loader.Spec(), parsed)

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [astm.validate] Validated ASTM message: valid=%v, %d issue(s) in %dms",
		result.Valid, len(result.Issues), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}

	resultMap := astmValidationResultToMap(result)
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

// resolveASTMParseResult reads sourceField from inputData (with the usual
// "message" envelope-unwrap fallback). If the value is a map already shaped
// like ASTMParseExecutor's own ParsedJSON (has a "header" key), it's
// reconstructed directly — no re-parsing. If it's a raw content string
// instead, it's parsed fresh via astm.ParseMessage.
func resolveASTMParseResult(loader *astm.ASTMSchemaLoader, inputData map[string]interface{}, sourceField string) (*astm.ParseResult, error) {
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
		header, hasHeader := t["header"].(map[string]interface{})
		if !hasHeader {
			return nil, fmt.Errorf("source field %q is an object but has no \"header\" — expected a prior astm.parse step's ParsedJSON", sourceField)
		}
		return &astm.ParseResult{
			Header:         header,
			HeaderComments: toMapSliceForValidate(t["headerComments"]),
			PatientBlocks:  toMapSliceForValidate(t["patientBlocks"]),
			QueryBlocks:    toMapSliceForValidate(t["queryBlocks"]),
			Trailer:        asMap(t["trailer"]),
		}, nil
	case string:
		if t == "" {
			return nil, fmt.Errorf("source field %q is empty", sourceField)
		}
		return astm.ParseMessage(loader.Spec(), "generic_lab_result", t)
	default:
		return nil, fmt.Errorf("source field %q is empty or not a recognized shape", sourceField)
	}
}

// toMapSliceForValidate tolerates both []map[string]interface{} (constructed
// in-process by a prior astm.parse step in the SAME execution) and
// []interface{} of maps (the shape a JSON round trip through the pipeline
// engine, or a goja enrichment.script step, produces) — a reconstructed
// *astm.ParseResult here is only ever read by astm/validator.Validate,
// which only reads Header/Fields, so this reconstruction's own map shape
// doesn't need to perfectly round-trip back into astm/builder.BuildInput.
func toMapSliceForValidate(v interface{}) []map[string]interface{} {
	switch t := v.(type) {
	case []map[string]interface{}:
		return t
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func astmValidationResultToMap(result *validator.Result) map[string]interface{} {
	issues := make([]map[string]interface{}, 0, len(result.Issues))
	for _, iss := range result.Issues {
		issues = append(issues, map[string]interface{}{
			"severity": iss.Severity,
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
func (e *ASTMValidateExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the validation Result output for the field picker.
func (e *ASTMValidateExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Valid", Path: "_stepOutput.valid", DataType: "boolean",
			Description: "true unless a field's value is malformed against its own declared data type", Category: "ASTM Transform"},
		{Name: "Issue count", Path: "_stepOutput.issueCount", DataType: "number",
			Description: "Total number of validation issues found", Category: "ASTM Transform"},
		{Name: "Validation result", Path: "_stepOutput.astmValidation", DataType: "object",
			Description: "Full validation result: {valid, issues: [{severity, path, message}]}", Category: "ASTM Transform"},
	}
}
