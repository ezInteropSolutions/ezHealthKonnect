// services/executors/transform/fhir_to_cda_executor.go
// FHIRToCDAExecutor — pipeline step type "fhir.to_cda".
//
// Reads a FHIR R4 Bundle from the pipeline context and converts it to a
// C-CDA 2.1 XML string using CDASerializer. Intended as the final step in
// pipelines that deliver CDA to legacy downstream systems.
//
// Config keys (all optional):
//   sourceField  — dot-path to the FHIR bundle (default: "fhirBundle")
//   outputField  — dot-path to write CDA XML (default: "cdaXML")
//   profile      — CDA profile to target (default: "C-CDA 2.1"; unused in MVP)

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/format"
)

// FHIRToCDAExecutor converts a FHIR R4 Bundle to a C-CDA 2.1 XML document.
type FHIRToCDAExecutor struct {
	*executors.BaseExecutor
	serializer *format.CDASerializer
}

// NewFHIRToCDAExecutor returns a ready-to-use executor.
func NewFHIRToCDAExecutor() *FHIRToCDAExecutor {
	return &FHIRToCDAExecutor{
		BaseExecutor: executors.NewBaseExecutor("fhir.to_cda", models.ExecutorMetadata{
			Name:        "FHIR R4 → CDA",
			Description: "Converts a FHIR R4 Bundle to a C-CDA 2.1 XML document for legacy delivery",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
		serializer: format.NewCDASerializer(),
	}
}

type fhirToCDAConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
	Profile     string `json:"profile"`
}

// Execute reads a FHIR Bundle from the input context, serialises it to CDA XML,
// and writes the result to the configured output field.
func (e *FHIRToCDAExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := fhirToCDAConfig{
		SourceField: "fhirBundle",
		OutputField: "cdaXML",
		Profile:     "C-CDA 2.1",
	}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}

	// Resolve FHIR bundle from input
	bundle := e.resolveBundle(inputData, cfg.SourceField)
	if bundle == nil {
		return nil, fmt.Errorf("fhir.to_cda: FHIR Bundle not found at field %q", cfg.SourceField)
	}

	cdaXML, err := e.serializer.Serialize(bundle, cfg.Profile)
	if err != nil {
		return nil, fmt.Errorf("fhir.to_cda: serialization failed: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [fhir.to_cda] Produced CDA XML (%d bytes) in %dms", len(cdaXML), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.UpdateFieldValue(outputData, cfg.OutputField, cdaXML)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{"cdaXML": cdaXML},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"xml_bytes":      len(cdaXML),
			"profile":        cfg.Profile,
			"transformation": "fhir_to_cda",
		},
	)

	return outputData, nil
}

// resolveBundle attempts to find the FHIR Bundle at the given field path.
// Falls back to checking "fhirBundle" directly in the input map.
func (e *FHIRToCDAExecutor) resolveBundle(
	data map[string]interface{}, field string,
) map[string]interface{} {
	if v := executors.GetFieldValue(data, field); v != nil {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	// Unwrap "message" wrapper
	if msg, ok := data["message"].(map[string]interface{}); ok {
		if v := executors.GetFieldValue(msg, field); v != nil {
			if m, ok := v.(map[string]interface{}); ok {
				return m
			}
		}
	}
	return nil
}

// Validate checks step configuration.
func (e *FHIRToCDAExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the CDA XML output for the field picker.
func (e *FHIRToCDAExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "CDA XML", Path: "cdaXML", DataType: "string",
			Description: "C-CDA 2.1 XML document produced from FHIR Bundle", Category: "CDA Transform"},
	}
}
