// services/executors/transform/cda_normalizer_executor.go
// CDANormalizerExecutor — pipeline step type "cda.normalize".
//
// Upgrades a raw C32 / HITSP CDA XML string to C-CDA 2.1 by rewriting legacy
// templateId OIDs. This is a pre-parse step: it operates on raw XML field
// variables before the CDA parser runs.
//
// Config keys (all optional):
//   sourceField  — dot-path to the field holding raw CDA XML (default: "raw")
//   outputField  — dot-path to write normalised XML (default: overwrite sourceField)

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// CDANormalizerExecutor normalises C32/HITSP CDA XML to C-CDA 2.1.
// It self-initialises from the schema directory at construction time; if the
// schema directory is missing the executor logs a warning and passes XML through
// unchanged (graceful degradation).
type CDANormalizerExecutor struct {
	*executors.BaseExecutor
	normalizer *cdaSchema.C32TemplateNormalizer
}

// NewCDANormalizerExecutor constructs the executor and loads schemas from
// the default schema directory "./cda/schemas". Logs a warning if init fails
// and operates in passthrough mode.
func NewCDANormalizerExecutor() *CDANormalizerExecutor {
	exec := &CDANormalizerExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.normalize", models.ExecutorMetadata{
			Name:        "CDA Normalizer (C32 → C-CDA 2.1)",
			Description: "Upgrades C32/HITSP CDA template OIDs to C-CDA 2.1 equivalents",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}

	loader, err := cdaSchema.NewCDASchemaLoader("./cda/schemas")
	if err != nil {
		log.Printf("⚠️  [cda.normalize] Schema loader init failed (%v) — executor will pass XML through unchanged", err)
		return exec
	}
	exec.normalizer = cdaSchema.NewC32TemplateNormalizer(loader)
	log.Printf("✅ [cda.normalize] C32TemplateNormalizer loaded from ./cda/schemas")
	return exec
}

type cdaNormalizerConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
}

// Execute reads raw CDA XML from the configured source field, normalises it,
// and writes the result to the output field.
func (e *CDANormalizerExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := cdaNormalizerConfig{SourceField: "raw"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}

	// Read raw XML
	rawXML := ""
	if v := executors.GetFieldValue(inputData, cfg.SourceField); v != nil {
		rawXML, _ = v.(string)
	}
	if rawXML == "" {
		if v, ok := inputData["raw"].(string); ok {
			rawXML = v
		}
	}
	// ExecutePipeline (Test Pipeline and real message processing both route
	// through it) wraps the actual message under inputData["message"] on
	// every step call — the two top-level lookups above never see it there.
	// Mirrors cda_to_fhir_executor.go's own message-unwrapping fallback so
	// cda.normalize works whether it's the pipeline's first step or a later one.
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
		return nil, fmt.Errorf("cda.normalize: source field %q is empty or not a string", cfg.SourceField)
	}

	// Passthrough mode when normalizer could not be initialised
	if e.normalizer == nil {
		log.Printf("  ⚠️  [cda.normalize] Passthrough mode (schema not loaded)")
		outputData := make(map[string]interface{}, len(inputData))
		for k, v := range inputData {
			outputData[k] = v
		}
		e.SetStepOutputWithDetails(outputData,
			map[string]interface{}{"normalizedXML": rawXML, "substitutions": 0},
			map[string]interface{}{"duration_ms": 0, "success": true, "passthrough": true},
		)
		return outputData, nil
	}

	result, err := e.normalizer.Normalize(rawXML)
	if err != nil {
		return nil, fmt.Errorf("cda.normalize: normalisation failed: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.normalize] %d OID substitution(s) applied in %dms (profile: %s)",
		result.Substitutions, durationMs, result.OriginalProfile)

	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}

	// Write normalised XML to the target field
	targetField := cfg.OutputField
	if targetField == "" {
		targetField = cfg.SourceField
	}
	executors.UpdateFieldValue(outputData, targetField, result.NormalizedXML)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"normalizedXML": result.NormalizedXML,
			"substitutions": result.Substitutions,
			"sourceProfile": string(result.OriginalProfile),
		},
		map[string]interface{}{
			"duration_ms":   durationMs,
			"success":       true,
			"substitutions": result.Substitutions,
			"source_profile": string(result.OriginalProfile),
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *CDANormalizerExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the normalised XML output for the field picker.
func (e *CDANormalizerExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Normalized XML", Path: "_stepOutput.normalizedXML", DataType: "string",
			Description: "C-CDA 2.1 normalised XML (C32/HITSP OIDs upgraded)", Category: "CDA Transform"},
		{Name: "Substitutions", Path: "_stepOutput.substitutions", DataType: "number",
			Description: "Number of template OIDs rewritten", Category: "CDA Transform"},
		{Name: "Source profile", Path: "_stepOutput.sourceProfile", DataType: "string",
			Description: "CDA profile detected before normalisation (C32 / HITSP / C-CDA 2.1)", Category: "CDA Transform"},
	}
}
