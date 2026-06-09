// services/executors/transform/cda_to_fhir_executor.go
// CDAToFHIRExecutor — pipeline step type "cda.to_fhir".
//
// Reads the parsed CDA document (ParsedJSON from CDAParserService) out of the
// pipeline input data and converts it to a FHIR R4 Bundle using CDATOFHIRMapper.
// The resulting bundle is written to both "fhirBundle" (top-level) and
// _stepOutput.fhirBundle so downstream steps can reference it.

package transform

import (
	"context"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	"ezhealthkonnect/services/executors"
)

// CDAToFHIRExecutor converts a parsed CDA document to a FHIR R4 Bundle.
type CDAToFHIRExecutor struct {
	*executors.BaseExecutor
	mapper *cdafhir.CDATOFHIRMapper
}

// NewCDAToFHIRExecutor returns a ready-to-use executor.
func NewCDAToFHIRExecutor() *CDAToFHIRExecutor {
	return &CDAToFHIRExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.to_fhir", models.ExecutorMetadata{
			Name:        "CDA → FHIR R4",
			Description: "Converts a parsed CDA/CCD document to a FHIR R4 Bundle",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
		mapper: cdafhir.NewCDATOFHIRMapper(),
	}
}

// Execute reads parsedCDA from the input context and writes a FHIR Bundle to the output.
func (e *CDAToFHIRExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	// Resolve the CDA parsed data — it may be at root or wrapped under "message".
	parsedCDA := e.resolveParsedCDA(inputData)
	if parsedCDA == nil {
		return nil, fmt.Errorf("cda.to_fhir: no parsed CDA data found in input (expected _format=ccda)")
	}

	log.Printf("  📄 [cda.to_fhir] Converting parsed CDA to FHIR Bundle (profile: %v)",
		parsedCDA["cdaProfile"])

	bundle, err := e.mapper.Map(parsedCDA)
	if err != nil {
		return nil, fmt.Errorf("cda.to_fhir: mapping failed: %w", err)
	}

	// Count resources for logging
	resourceCount := 0
	if entries, ok := bundle["entry"].([]interface{}); ok {
		resourceCount = len(entries)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.to_fhir] Produced FHIR Bundle with %d resources in %dms",
		resourceCount, durationMs)

	// Build output
	outputData := make(map[string]interface{}, len(inputData)+4)
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["fhirBundle"] = bundle

	// Propagate to message wrapper for downstream validators
	if msg, ok := outputData["message"].(map[string]interface{}); ok {
		msg["fhirBundle"] = bundle
	}

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"fhirBundle": bundle},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"resource_count": resourceCount,
			"format":         "fhir-r4",
			"transformation": "cda_to_fhir",
		},
	)

	return outputData, nil
}

// resolveParsedCDA walks the input looking for a map whose "_format" key equals
// "ccda" — that is the ParsedJSON produced by CDAParserService.
func (e *CDAToFHIRExecutor) resolveParsedCDA(data map[string]interface{}) map[string]interface{} {
	if isCDAData(data) {
		return data
	}
	// Check under "message" wrapper
	if msg, ok := data["message"].(map[string]interface{}); ok && isCDAData(msg) {
		return msg
	}
	// Walk one more level
	for _, v := range data {
		if m, ok := v.(map[string]interface{}); ok && isCDAData(m) {
			return m
		}
	}
	return nil
}

func isCDAData(m map[string]interface{}) bool {
	if m == nil {
		return false
	}
	format, _ := m["_format"].(string)
	return format == "ccda"
}

// Validate checks step configuration.
func (e *CDAToFHIRExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the FHIR Bundle output for the field picker.
func (e *CDAToFHIRExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "FHIR Bundle", Path: "fhirBundle", DataType: "object",
			Description: "FHIR R4 Bundle produced from CDA document", Category: "CDA Transform"},
		{Name: "Bundle entries", Path: "fhirBundle.entry", DataType: "array",
			Description: "Array of FHIR resources (Patient, AllergyIntolerance, Condition, ...)", Category: "CDA Transform"},
	}
}

