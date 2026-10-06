// services/executors/transform/dicom_parse_executor.go
// DICOMParseExecutor — pipeline step type "dicom.parse".
//
// Reads raw DICOM Part 10 content from a configurable input field and
// produces a structured ParsedJSON map (patient/study/series/instance
// metadata — never PixelData) that downstream steps can consume without
// re-decoding the file themselves. Mirrors edi_parse_executor.go's shape
// exactly — same config keys, same message-unwrap fallback.
//
// Config keys:
//
//	sourceField — dot-path to the field holding raw DICOM content (default: "raw")
//	outputField — top-level key to write ParsedJSON under (default: "parsedDICOM")
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	dicomparser "ezhealthkonnect/services/parsers/dicom"
)

// DICOMParseExecutor parses raw DICOM Part 10 content into a structured
// ParsedJSON map. Unlike CDAParseExecutor/EDIParseExecutor, there is no
// schema directory to self-initialise from — go-dicom's own data
// dictionary is compiled into the library — so construction can never fail.
type DICOMParseExecutor struct {
	*executors.BaseExecutor
	parser *dicomparser.DICOMParserService
}

// NewDICOMParseExecutor constructs the executor.
func NewDICOMParseExecutor() *DICOMParseExecutor {
	return &DICOMParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("dicom.parse", models.ExecutorMetadata{
			Name:        "DICOM Parser (Raw → ParsedJSON)",
			Description: "Parses a DICOM Part 10 file into a structured metadata ParsedJSON map (patient/study/series/instance identifiers; never pixel data)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "DICOM Transform",
		}),
		parser: dicomparser.NewDICOMParserService(),
	}
}

type dicomParseConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads raw DICOM content from sourceField, parses it, and writes
// the ParsedJSON map to outputField (default "parsedDICOM") in the output
// data.
func (e *DICOMParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := dicomParseConfig{SourceField: "raw", OutputField: "parsedDICOM"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedDICOM"
	}

	rawDICOM := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawDICOM, _ = v.(string)
	}
	if rawDICOM == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawDICOM = v
		}
	}
	// ExecutePipeline wraps the actual message under inputData["message"] on
	// every step call — same message-unwrapping fallback edi.parse/
	// astm.parse already use, so dicom.parse works whether it's the
	// pipeline's first step or a later one.
	if rawDICOM == "" {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			if v := executors.GetFieldValue(msg, cfg.SourceField); v != nil {
				rawDICOM, _ = v.(string)
			}
			if rawDICOM == "" {
				if v, ok := msg["raw"].(string); ok {
					rawDICOM = v
				}
			}
		}
	}
	if rawDICOM == "" {
		return nil, fmt.Errorf("dicom.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	result := e.parser.Parse(rawDICOM)
	if !result.Success {
		return nil, fmt.Errorf("dicom.parse: %s", result.Error)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [dicom.parse] Parsed DICOM instance %v: %d fields in %dms",
		result.ParsedJSON["sopInstanceUID"], len(result.EnhancedFields), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}

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
		outputData["_format"] = "dicom"
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
func (e *DICOMParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *DICOMParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed DICOM", Path: "_stepOutput.parsedDICOM", DataType: "object",
			Description: "Structured DICOM instance metadata (patient/study/series/SOP identifiers)", Category: "DICOM Transform"},
		{Name: "SOP Instance UID", Path: "_stepOutput.parsedDICOM.sopInstanceUID", DataType: "string",
			Description: "The received instance's own unique identifier", Category: "DICOM Transform"},
		{Name: "Field count", Path: "_stepOutput.fieldCount", DataType: "number",
			Description: "Number of metadata fields extracted", Category: "DICOM Transform"},
	}
}
