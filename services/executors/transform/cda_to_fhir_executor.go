// services/executors/transform/cda_to_fhir_executor.go
// CDAToFHIRExecutor — pipeline step type "cda.to_fhir".
//
// Reads a parsed CDA document (ParsedJSON produced by cda.parse or cda.normalize)
// and converts it to a FHIR R4 Bundle using the schema-driven GenericCDAFHIRMapper.
// The resulting bundle and structured processingResult are written to _stepOutput.
//
// Config keys (all optional):
//   sourceField           — dot-path to the parsed CDA map (default: looks for _format=ccda)
//   docType               — CDA document type (default: inferred from parsedCDA or "CCD")
//   bundleType            — FHIR bundle type (default: "collection")
//   profileMode           — "us-core" | "base" (default: "us-core")
//   onSectionFailure      — "continue" | "fail-fast" (default: "continue")
//   enabledSections       — []string — whitelist of sections to process (default: all)
//   terminologyValidation — bool (default: false)

package transform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	"ezhealthkonnect/services/executors"
)

// CDAToFHIRExecutor converts a parsed CDA document to a FHIR R4 Bundle.
type CDAToFHIRExecutor struct {
	*executors.BaseExecutor
	mapper    *cdafhir.GenericCDAFHIRMapper
	cdaParser *cdadocument.CDAParser // used for auto-parse when _cdaDocument is absent
}

// NewCDAToFHIRExecutor constructs the executor with a live DB connection for
// three-tier template lookup (interface-level delta → OOB fallback).
// If the schema directory is unavailable the executor logs a warning; Execute()
// will still attempt DB-only template lookup through GenericCDAFHIRMapper.
func NewCDAToFHIRExecutor(db *sql.DB) *CDAToFHIRExecutor {
	exec := &CDAToFHIRExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.to_fhir", models.ExecutorMetadata{
			Name:        "CDA → FHIR R4",
			Description: "Converts a parsed CDA/CCD document to a FHIR R4 Bundle (schema-driven)",
			Version:     "2.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
	}

	loader, err := cdaSchema.NewCDASchemaLoader("./cda/schemas")
	if err != nil {
		log.Printf("⚠️  [cda.to_fhir] Schema loader init failed (%v) — using DB-only template lookup", err)
		loader = nil
	}

	exec.mapper = cdafhir.NewGenericCDAFHIRMapper(db, loader)
	if loader != nil {
		exec.cdaParser = cdadocument.NewCDAParser(loader)
	}
	log.Printf("✅ [cda.to_fhir] GenericCDAFHIRMapper initialised (schema-driven, three-tier lookup)")
	return exec
}

type cdaToFHIRConfig struct {
	SourceField           string   `json:"sourceField"`
	DocType               string   `json:"docType"`
	BundleType            string   `json:"bundleType"`
	ProfileMode           string   `json:"profileMode"`
	OnSectionFailure      string   `json:"onSectionFailure"`
	EnabledSections       []string `json:"enabledSections"`
	TerminologyValidation bool     `json:"terminologyValidation"`
}

// Execute resolves the parsed CDA map, drives GenericCDAFHIRMapper.Map(), and
// writes the FHIR bundle + processingResult to _stepOutput and the root data map.
func (e *CDAToFHIRExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := cdaToFHIRConfig{
		BundleType:       "collection",
		ProfileMode:      "us-core",
		OnSectionFailure: "continue",
	}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}

	// Extract interfaceID from execution context for three-tier template lookup
	interfaceID := ""
	if v, ok := inputData["interfaceId"].(string); ok {
		interfaceID = v
	} else if v, ok := inputData["interface_id"].(string); ok {
		interfaceID = v
	}

	mapConfig := cdafhir.CDAToFHIRConfig{
		DocType:               cfg.DocType,
		CCDAVersion:           "2.1",
		FHIRVersion:           "R4",
		InterfaceID:           interfaceID,
		BundleType:            cfg.BundleType,
		ProfileMode:           cfg.ProfileMode,
		EnabledSections:       cfg.EnabledSections,
		OnSectionFailure:      cfg.OnSectionFailure,
		TerminologyValidation: cfg.TerminologyValidation,
	}

	// Prefer typed path (Sprint D): cda.parse stores *CDADocument at _cdaDocument.
	// MapDocument() uses zero XPath and operates on fully-typed Go structs.
	var output *cdafhir.MapOutput
	if typed, ok := inputData["_cdaDocument"]; ok {
		if doc, ok := typed.(*cdadocument.CDADocument); ok {
			log.Printf("  📄 [cda.to_fhir] Using typed CDADocument path (Sprint D)")
			out, err := e.mapper.MapDocument(ctx, doc, mapConfig)
			if err != nil {
				return nil, fmt.Errorf("cda.to_fhir: MapDocument failed: %w", err)
			}
			output = out
		}
	}

	// Auto-parse path: extract raw XML from parsedCDA["raw"] and run the typed
	// MapDocument() path without requiring a separate cda.parse pipeline step.
	if output == nil && e.cdaParser != nil {
		parsedCDA := e.resolveParsedCDA(inputData, cfg.SourceField)
		rawXML := ""
		if parsedCDA != nil {
			rawXML, _ = parsedCDA["raw"].(string)
		}
		if rawXML == "" {
			rawXML = e.resolveRawXML(inputData)
		}
		if rawXML != "" {
			if doc, err := e.cdaParser.ParseFromRawXML(rawXML); err == nil {
				log.Printf("  📄 [cda.to_fhir] Auto-parsed raw CDA XML → typed MapDocument path")
				out, err := e.mapper.MapDocument(ctx, doc, mapConfig)
				if err == nil {
					output = out
				} else {
					log.Printf("  ⚠️  [cda.to_fhir] MapDocument failed after auto-parse: %v", err)
				}
			} else {
				log.Printf("  ⚠️  [cda.to_fhir] XML auto-parse failed: %v", err)
			}
		}
	}

	// Final fallback: legacy schema-driven map path
	if output == nil {
		parsedCDA := e.resolveParsedCDA(inputData, cfg.SourceField)
		if parsedCDA == nil {
			return nil, fmt.Errorf("cda.to_fhir: no parsed CDA data found in input (expected _format=ccda or sourceField=%q)", cfg.SourceField)
		}
		log.Printf("  📄 [cda.to_fhir] Using legacy map path (profile: %v)", parsedCDA["cdaProfile"])
		out, err := e.mapper.Map(ctx, parsedCDA, mapConfig)
		if err != nil {
			return nil, fmt.Errorf("cda.to_fhir: mapping failed: %w", err)
		}
		output = out
	}

	resourceCount := 0
	if entries, ok := output.FHIRBundle["entry"].([]interface{}); ok {
		resourceCount = len(entries)
	}

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.to_fhir] Produced FHIR Bundle with %d resources in %dms (sections ok=%d failed=%d)",
		resourceCount, durationMs,
		len(output.ProcessingResult.SuccessfulSections),
		len(output.ProcessingResult.FailedSections))

	// Build output
	outputData := make(map[string]interface{}, len(inputData)+4)
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["fhirBundle"] = output.FHIRBundle

	// Propagate to message wrapper for downstream validators
	if msg, ok := outputData["message"].(map[string]interface{}); ok {
		msg["fhirBundle"] = output.FHIRBundle
	}

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"fhirBundle":        output.FHIRBundle,
			"processingResult":  output.ProcessingResult,
		},
		map[string]interface{}{
			"duration_ms":         durationMs,
			"success":             true,
			"resource_count":      resourceCount,
			"format":              "fhir-r4",
			"transformation":      "cda_to_fhir",
			"sections_successful": len(output.ProcessingResult.SuccessfulSections),
			"sections_failed":     len(output.ProcessingResult.FailedSections),
			"partial_success":     output.ProcessingResult.PartialSuccess,
		},
	)

	return outputData, nil
}

// resolveParsedCDA locates the CDA parsed map, checking:
//  1. An explicit sourceField config key
//  2. Root-level data whose "_format" = "ccda"
//  3. "parsedCDA" key (written by cda.parse executor)
//  4. "message" wrapper
func (e *CDAToFHIRExecutor) resolveParsedCDA(data map[string]interface{}, sourceField string) map[string]interface{} {
	// Explicit source field
	if sourceField != "" {
		if v, ok := data[sourceField].(map[string]interface{}); ok && len(v) > 0 {
			return v
		}
	}
	// Root-level CDA data
	if isCDAData(data) {
		return data
	}
	// Standard key written by cda.parse executor
	if v, ok := data["parsedCDA"].(map[string]interface{}); ok && isCDAData(v) {
		return v
	}
	// "message" wrapper
	if msg, ok := data["message"].(map[string]interface{}); ok {
		if isCDAData(msg) {
			return msg
		}
		if v, ok := msg["parsedCDA"].(map[string]interface{}); ok && isCDAData(v) {
			return v
		}
	}
	// One-level deep walk
	for _, v := range data {
		if m, ok := v.(map[string]interface{}); ok && isCDAData(m) {
			return m
		}
	}
	return nil
}

// resolveRawXML looks for the original CDA XML string in the pipeline data.
// It checks the message wrapper ("content" field from InboundMessage) and a set
// of well-known root keys before giving up.
func (e *CDAToFHIRExecutor) resolveRawXML(data map[string]interface{}) string {
	candidates := []string{"content", "raw_content", "rawMessage", "rawXML", "cdaXML"}
	if msg, ok := data["message"].(map[string]interface{}); ok {
		for _, k := range candidates {
			if v, ok := msg[k].(string); ok && looksLikeCDA(v) {
				return v
			}
		}
	}
	for _, k := range candidates {
		if v, ok := data[k].(string); ok && looksLikeCDA(v) {
			return v
		}
	}
	return ""
}

func looksLikeCDA(s string) bool {
	t := strings.TrimSpace(s)
	return strings.Contains(t, "ClinicalDocument")
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

// GetOutputVariables declares the outputs for the field picker.
func (e *CDAToFHIRExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "FHIR Bundle", Path: "fhirBundle", DataType: "object",
			Description: "FHIR R4 Bundle produced from CDA document", Category: "CDA Transform"},
		{Name: "Bundle entries", Path: "fhirBundle.entry", DataType: "array",
			Description: "Array of FHIR resources (Patient, AllergyIntolerance, Condition, ...)", Category: "CDA Transform"},
		{Name: "Processing result", Path: "_stepOutput.processingResult", DataType: "object",
			Description: "Section-level success/failure breakdown with error details", Category: "CDA Transform"},
		{Name: "Failed sections", Path: "_stepOutput.processingResult.failedSections", DataType: "array",
			Description: "Section keys that encountered errors during mapping", Category: "CDA Transform"},
	}
}
