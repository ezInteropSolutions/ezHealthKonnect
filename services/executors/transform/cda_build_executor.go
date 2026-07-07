// services/executors/transform/cda_build_executor.go
// CDABuildExecutor — pipeline step type "cda.build".
//
// Format-agnostic successor to fhir.to_cda: converts canonical, USCDI-keyed
// JSON (the same shape cda.parse produces, or Phase 3's cda.map_to_canonical
// step will produce from CSV/DB/generic-JSON sources) into valid C-CDA 2.1
// XML via cda/builder.BuildDocument — a schema-driven engine covering all
// CCD SHALL+SHOULD sections, not the 4 sections fhir.to_cda's
// CDASerializer hardcodes. fhir.to_cda is left unchanged for existing
// pipelines; this step also accepts a raw FHIR Bundle directly (via
// format.BundleToCanonicalDoc) so FHIR-sourced pipelines can adopt it
// without an intermediate mapping step.
//
// Config keys (all optional):
//   sourceField  — dot-path to the source data (default: "parsedCDA")
//   inputFormat  — "canonical" (default) | "fhir_bundle"
//   outputField  — dot-path to write CDA XML (default: "cdaXML")
//   documentType — CCD | Discharge Summary | Referral Note | ... (default: "CCD")
//   orgName      — custodian + fallback author organization name

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	cdaBuilder "ezhealthkonnect/cda/builder"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/format"
)

// CDABuildExecutor builds a C-CDA 2.1 XML document from canonical JSON or a FHIR Bundle.
type CDABuildExecutor struct {
	*executors.BaseExecutor
	schemaLoader *cdaSchema.CDASchemaLoader
}

// NewCDABuildExecutor constructs the executor and loads the CDA schema from
// the default schema directory "./cda/schemas" — the same self-init-at-
// construction convention cda.parse/cda.to_fhir already use. Logs a warning
// if init fails; Execute() will return a descriptive error in that case.
func NewCDABuildExecutor() *CDABuildExecutor {
	exec := &CDABuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.build", models.ExecutorMetadata{
			Name:        "CDA/CCD Document Builder",
			Description: "Builds a C-CDA 2.1 document from canonical USCDI-keyed JSON or a FHIR Bundle, covering CCD's full SHALL+SHOULD section set",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}

	loader, err := cdaSchema.NewCDASchemaLoader("./cda/schemas")
	if err != nil {
		log.Printf("⚠️  [cda.build] CDA schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.schemaLoader = loader
	log.Printf("✅ [cda.build] CDA schema loaded from ./cda/schemas")
	return exec
}

type cdaBuildConfig struct {
	SourceField  string `json:"sourceField"`
	InputFormat  string `json:"inputFormat"`
	OutputField  string `json:"outputField"`
	DocumentType string `json:"documentType"`
	OrgName      string `json:"orgName"`
}

// Execute reads the configured source, converts it to the canonical shape
// BuildDocument expects (pass-through for "canonical", via
// format.BundleToCanonicalDoc for "fhir_bundle"), builds the CDA XML, and
// writes it to the configured output field.
func (e *CDABuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.schemaLoader == nil {
		return nil, fmt.Errorf("cda.build: CDA schema is not initialised (schema directory missing)")
	}

	cfg := cdaBuildConfig{
		SourceField:  "parsedCDA",
		InputFormat:  "canonical",
		OutputField:  "cdaXML",
		DocumentType: "CCD",
	}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedCDA"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "cdaXML"
	}
	if cfg.DocumentType == "" {
		cfg.DocumentType = "CCD"
	}

	source := e.resolveSource(inputData, cfg.SourceField)
	if source == nil {
		return nil, fmt.Errorf("cda.build: source data not found at field %q", cfg.SourceField)
	}

	canonicalDoc := source
	if cfg.InputFormat == "fhir_bundle" {
		canonicalDoc = format.BundleToCanonicalDoc(source)
	}

	cdaXML, err := cdaBuilder.BuildDocument(e.schemaLoader, canonicalDoc, cfg.DocumentType, cdaBuilder.BuildOptions{OrgName: cfg.OrgName})
	if err != nil {
		return nil, fmt.Errorf("cda.build: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.build] Produced %s CDA XML (%d bytes) in %dms", cfg.DocumentType, len(cdaXML), durationMs)

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
			"documentType":   cfg.DocumentType,
			"inputFormat":    cfg.InputFormat,
			"transformation": "cda_build",
		},
	)

	return outputData, nil
}

// resolveSource finds the source map at the given field path, falling back
// to checking under an ExecutePipeline "message" wrapper.
func (e *CDABuildExecutor) resolveSource(data map[string]interface{}, field string) map[string]interface{} {
	if v := executors.GetFieldValue(data, field); v != nil {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
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
func (e *CDABuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the CDA XML output for the field picker.
func (e *CDABuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "CDA XML", Path: "cdaXML", DataType: "string",
			Description: "C-CDA 2.1 XML document built from canonical JSON or a FHIR Bundle", Category: "CDA Transform"},
	}
}
