// services/executors/transform/edi_build_executor.go
// EDIBuildExecutor — pipeline step type "edi.build".
//
// Builds a complete ISA...IEA X12 EDI interchange from canonical JSON (the
// same Interchange/Header/Loops/Trailer shape edi.parse's own ParsedJSON
// produces, so a parse→build round trip needs no reshaping) via
// edi/builder.BuildDocument — the write-direction mirror of edi.parse.
//
// Config keys:
//   sourceField    — dot-path to the canonical source data (default: "parsedEDI")
//   transactionSet — which transaction set to build, e.g. "835" (default: "835")
//   isaSenderId / isaReceiverId       — ISA06/ISA08, deployment-level trading-partner identifiers
//   gsSenderCode / gsReceiverCode     — GS02/GS03; default to the ISA values above when unset,
//                                       the common real-world convention (some trading partners
//                                       do use distinct GS-level codes, hence separate knobs)
//   outputField    — dot-path to write the built EDI text (default: "ediX12")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/edi"
	ediBuilder "ezhealthkonnect/edi/builder"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// EDIBuildExecutor builds a complete X12 EDI interchange from canonical JSON.
type EDIBuildExecutor struct {
	*executors.BaseExecutor
	loader *edi.X12SchemaLoader
}

// NewEDIBuildExecutor constructs the executor and loads the EDI schema from
// the default schema directory "./edi/schemas/x12_005010" — the same self-
// init-at-construction convention edi.parse/edi.validate already use.
func NewEDIBuildExecutor() *EDIBuildExecutor {
	exec := &EDIBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.build", models.ExecutorMetadata{
			Name:        "EDI X12 Document Builder",
			Description: "Builds a complete ISA...IEA X12 interchange from canonical JSON (835 phase 1)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "EDI Transform",
		}),
	}

	loader, err := edi.NewX12SchemaLoader("./edi/schemas/x12_005010")
	if err != nil {
		log.Printf("⚠️  [edi.build] EDI schema load failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.loader = loader
	log.Printf("✅ [edi.build] EDI schema loaded from ./edi/schemas/x12_005010")
	return exec
}

// EdiBuildConfig is step.Config's decoded shape for the edi.build step.
// Exported (matching CdaBuildConfig's own precedent) so other packages can
// decode it the same way Execute() does, without a second, drifting copy.
type EdiBuildConfig struct {
	SourceField    string `json:"sourceField"`
	TransactionSet string `json:"transactionSet"`
	OutputField    string `json:"outputField"`
	ISASenderID    string `json:"isaSenderId,omitempty"`
	ISAReceiverID  string `json:"isaReceiverId,omitempty"`
	GSSenderCode   string `json:"gsSenderCode,omitempty"`
	GSReceiverCode string `json:"gsReceiverCode,omitempty"`
}

// Execute reads canonical source data, builds the EDI interchange, and
// writes it to the configured output field.
func (e *EDIBuildExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.loader == nil {
		return nil, fmt.Errorf("edi.build: EDI schema is not initialised (schema directory missing)")
	}

	cfg := EdiBuildConfig{SourceField: "parsedEDI", TransactionSet: "835", OutputField: "ediX12"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "parsedEDI"
	}
	if cfg.TransactionSet == "" {
		cfg.TransactionSet = "835"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "ediX12"
	}
	if cfg.GSSenderCode == "" {
		cfg.GSSenderCode = cfg.ISASenderID
	}
	if cfg.GSReceiverCode == "" {
		cfg.GSReceiverCode = cfg.ISAReceiverID
	}

	source := executors.GetFieldValue(inputData, cfg.SourceField)
	if source == nil {
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			source = executors.GetFieldValue(msg, cfg.SourceField)
		}
	}
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("edi.build: source field %q is empty or not an object", cfg.SourceField)
	}

	input := ediBuilder.BuildInput{
		TransactionSet: cfg.TransactionSet,
		Interchange:    mergeInterchangeConfig(asMap(sourceMap["interchange"]), cfg),
		Header:         asMap(sourceMap["header"]),
		Loops:          asMap(sourceMap["loops"]),
		Trailer:        asMap(sourceMap["trailer"]),
	}

	built, err := ediBuilder.BuildDocument(e.loader.Spec(), input)
	if err != nil {
		return nil, fmt.Errorf("edi.build: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [edi.build] Built %s interchange: %d bytes in %dms", cfg.TransactionSet, len(built), durationMs)

	outputData := make(map[string]interface{}, len(inputData)+2)
	for k, v := range inputData {
		outputData[k] = v
	}
	executors.SetNestedValue(outputData, cfg.OutputField, built)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			cfg.OutputField: built,
			"byteCount":     len(built),
		},
		map[string]interface{}{
			"duration_ms": durationMs,
			"success":     true,
			"byte_count":  len(built),
		},
	)

	return outputData, nil
}

// mergeInterchangeConfig layers the step's own ISA/GS config (deployment-
// level constants — who ezHealthKonnect is sending this interchange as) on
// top of whatever the source data already supplies, without overwriting
// values the source data explicitly set (e.g. a round-tripped
// isaControlNumber from a prior edi.parse) — same "config fills in what
// data doesn't supply" precedent CDA's own CdaCustodianConfig follows.
func mergeInterchangeConfig(existing map[string]interface{}, cfg EdiBuildConfig) map[string]interface{} {
	out := make(map[string]interface{}, len(existing)+4)
	for k, v := range existing {
		out[k] = v
	}
	setIfAbsent(out, "senderId", cfg.ISASenderID)
	setIfAbsent(out, "receiverId", cfg.ISAReceiverID)
	setIfAbsent(out, "gsSenderCode", cfg.GSSenderCode)
	setIfAbsent(out, "gsReceiverCode", cfg.GSReceiverCode)
	return out
}

func setIfAbsent(m map[string]interface{}, key, value string) {
	if value == "" {
		return
	}
	if existing, ok := m[key]; ok {
		if s, ok := existing.(string); ok && s != "" {
			return
		}
	}
	m[key] = value
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

// Validate checks step configuration.
func (e *EDIBuildExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the built EDI text output for the field picker.
func (e *EDIBuildExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "EDI X12 document", Path: "_stepOutput.ediX12", DataType: "string",
			Description: "Complete built ISA...IEA X12 interchange text", Category: "EDI Transform"},
		{Name: "Byte count", Path: "_stepOutput.byteCount", DataType: "number",
			Description: "Length of the built document in bytes", Category: "EDI Transform"},
	}
}
