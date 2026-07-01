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
//   mappingLogEnabled     — bool (default: true) — persist the MappingLog to object
//                           storage; disable per-pipeline if log volume isn't needed

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
	"ezhealthkonnect/services/cda_fhir/assembly"
	cdastorage "ezhealthkonnect/services/cda_storage"
	"ezhealthkonnect/services/executors"
	cdaparser "ezhealthkonnect/services/parsers/cda"
	"ezhealthkonnect/services/storage"
)

// CDAToFHIRExecutor converts a parsed CDA document to a FHIR R4 Bundle.
type CDAToFHIRExecutor struct {
	*executors.BaseExecutor
	mapper        *cdafhir.GenericCDAFHIRMapper
	parserService *cdaparser.CDAParserService   // used for auto-parse when _cdaDocument is absent; also produces ParsedJSON for auto-persist
	objStorage    *storage.ObjectStorageService // optional; nil = no mapping log persistence
	cdaDocStore   cdastorage.CDADocumentStore   // optional; nil = no auto-persisted parsed content
}

// SetObjectStorageService wires in the object storage service so the executor can
// persist MappingLog documents asynchronously after each CDA→FHIR conversion.
func (e *CDAToFHIRExecutor) SetObjectStorageService(svc *storage.ObjectStorageService) {
	e.objStorage = svc
}

// SetCDADocumentStore wires in the document store so that, when this executor's
// auto-parse path runs (no separate cda.parse step in the pipeline), the parsed
// CDA content it already produces is also persisted — giving every CDA interface
// access to GET /api/cda/documents/:id/json without requiring a cda.parse step.
// Does nothing when a cda.parse step already ran (output came from "_cdaDocument"
// instead of auto-parse), since that step persists its own result already.
func (e *CDAToFHIRExecutor) SetCDADocumentStore(store cdastorage.CDADocumentStore) {
	e.cdaDocStore = store
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

	if svc, err := cdaparser.NewFromSchemaDir("./cda/schemas"); err == nil {
		exec.parserService = svc
	} else {
		log.Printf("⚠️  [cda.to_fhir] CDAParserService init failed (%v) — auto-parse persistence disabled", err)
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
	MappingLogEnabled     *bool    `json:"mappingLogEnabled"` // nil = default true
	DeepLineage           bool     `json:"_cdaDeepLineage"`   // system-injected, see transformation_pipeline_service.go
	// PlanOfCareEncounterTarget: "" (unset, default) | "Appointment" | "Encounter".
	// Per-step override for Plan-of-Care "entryType=encounter" entries' target
	// resource -- see CDAToFHIRConfig.PlanOfCareEncounterTarget's own doc comment
	// for the full precedence chain (step config > interface default > "Encounter").
	PlanOfCareEncounterTarget string `json:"planOfCareEncounterTarget"`
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

	// Extract interfaceID for three-tier template lookup and mapping-log storage keys.
	// inputData carries it only in ad-hoc/test invocations; real pipeline runs set it
	// on ctx (processing/engine_message_processor.go) instead, so check both.
	interfaceID := ""
	if v, ok := inputData["interfaceId"].(string); ok && v != "" {
		interfaceID = v
	} else if v, ok := inputData["interface_id"].(string); ok && v != "" {
		interfaceID = v
	} else if v, ok := ctx.Value("interface_id").(string); ok {
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
		PlanOfCareEncounterTarget: cfg.PlanOfCareEncounterTarget,
		Assembly: assembly.AssemblyConfig{
			DeepLineage: cfg.DeepLineage,
		},
	}

	// Prefer typed path: cda.parse stores *CDADocument at _cdaDocument.
	// DeclarativeMapDocument() uses zero XPath and operates on a documentMap
	// built from the fully-typed Go structs, driven by OOB declarative rules.
	var output *cdafhir.MapOutput
	if typed, ok := inputData["_cdaDocument"]; ok {
		if doc, ok := typed.(*cdadocument.CDADocument); ok {
			log.Printf("  📄 [cda.to_fhir] Using typed CDADocument path (declarative)")
			out, err := e.mapper.DeclarativeMapDocument(ctx, doc, mapConfig)
			if err != nil {
				return nil, fmt.Errorf("cda.to_fhir: DeclarativeMapDocument failed: %w", err)
			}
			output = out
		}
	}

	// Auto-parse path: extract raw XML from parsedCDA["raw"] and run the typed
	// DeclarativeMapDocument() path without requiring a separate cda.parse
	// pipeline step. Uses the same CDAParserService the cda.parse executor
	// uses (not just the bare typed parser) so the ParsedJSON it produces can
	// also be persisted below — giving every CDA interface structured
	// parsed-content storage even when its pipeline has no separate cda.parse
	// step.
	var autoParseResult *models.ParserResult
	var autoParseRawXML string
	if output == nil && e.parserService != nil {
		parsedCDA := e.resolveParsedCDA(inputData, cfg.SourceField)
		rawXML := ""
		if parsedCDA != nil {
			rawXML, _ = parsedCDA["raw"].(string)
		}
		if rawXML == "" {
			rawXML = e.resolveRawXML(inputData)
		}
		if rawXML != "" {
			result := e.parserService.Parse(rawXML)
			if !result.Success {
				log.Printf("  ⚠️  [cda.to_fhir] XML auto-parse failed: %v", result.Error)
			} else if doc, ok := result.TypedDocument.(*cdadocument.CDADocument); ok {
				log.Printf("  📄 [cda.to_fhir] Auto-parsed raw CDA XML → typed DeclarativeMapDocument path")
				out, err := e.mapper.DeclarativeMapDocument(ctx, doc, mapConfig)
				if err == nil {
					output = out
					autoParseResult = result
					autoParseRawXML = rawXML
				} else {
					log.Printf("  ⚠️  [cda.to_fhir] DeclarativeMapDocument failed after auto-parse: %v", err)
				}
			} else {
				log.Printf("  ⚠️  [cda.to_fhir] auto-parse produced no typed document")
			}
		}
	}

	if output == nil {
		return nil, fmt.Errorf("cda.to_fhir: no typed CDA document available for conversion (expected _cdaDocument or parseable raw XML)")
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

	// CDA SECTION lifecycle log points — one entry per section processed. Per-section
	// entry_count/resource_count aren't tracked by GenericCDAFHIRMapper today (only the
	// bundle-wide resourceCount above), so these carry section_key + outcome only.
	if logFn := models.GetLogLifecycleEventFn(ctx); logFn != nil {
		for _, sectionKey := range output.ProcessingResult.SuccessfulSections {
			logFn("debug", "transformation", "CDA section mapped", map[string]interface{}{
				"section_key":   sectionKey,
				"fhir_resource": "fhir-r4",
				"status":        "success",
			})
		}
		for _, secErr := range output.ProcessingResult.SectionErrors {
			logFn("warning", "transformation", "CDA section mapping error", map[string]interface{}{
				"section_key": secErr.SectionKey,
				"field_key":   secErr.FieldKey,
				"transform":   secErr.Transform,
				"error":       secErr.Error,
				"severity":    secErr.Severity,
			})
		}
	}

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

	warnings := formatSectionErrorWarnings(output.ProcessingResult.SectionErrors)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"fhirBundle":       output.FHIRBundle,
			"processingResult": output.ProcessingResult,
			"mappingLog":       output.MappingLog,
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
			"warnings":            warnings,
		},
	)

	messageID, _ := ctx.Value("message_id").(string)

	// Persist mapping log to object storage asynchronously (non-blocking), unless
	// disabled via step config (mappingLogEnabled: false). Keyed by the pipeline
	// message_id (not the CDA document's own ID) so it's retrievable the same way
	// as raw/parsed/transformed content via GET /api/messages/:messageId/mapping-log.
	mappingLogEnabled := cfg.MappingLogEnabled == nil || *cfg.MappingLogEnabled
	if mappingLogEnabled && e.objStorage != nil && messageID != "" {
		ml := output.MappingLog
		go e.writeMappingLog(interfaceID, messageID, ml)
	}

	// Persist the parsed CDA content from the auto-parse path (only set when no
	// separate cda.parse step already ran and persisted its own result).
	if e.cdaDocStore != nil && autoParseResult != nil {
		saveInput := &cdastorage.SaveInput{
			InterfaceID:   interfaceID,
			MessageID:     messageID,
			RawXML:        autoParseRawXML,
			ParsedJSON:    autoParseResult.ParsedJSON,
			TypedDocument: autoParseResult.TypedDocument,
			FieldCount:    len(autoParseResult.EnhancedFields),
			FHIRBundle:    output.FHIRBundle,
		}
		if sections, ok := autoParseResult.ParsedJSON["sections"].(map[string]interface{}); ok {
			saveInput.SectionCount = len(sections)
		}
		store := e.cdaDocStore
		go func() {
			saveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := store.Save(saveCtx, saveInput); err != nil {
				log.Printf("⚠️  [cda.to_fhir] Parsed document storage failed (non-fatal): %v", err)
			}
		}()
	}

	return outputData, nil
}

// writeMappingLog persists the MappingLog to object storage in a background goroutine.
func (e *CDAToFHIRExecutor) writeMappingLog(interfaceID, messageID string, ml interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	uri, err := e.objStorage.StoreMappingLog(ctx, interfaceID, messageID, ml)
	if err != nil {
		log.Printf("⚠️  [cda.to_fhir] Failed to persist mapping log for message %s: %v", messageID, err)
		return
	}
	log.Printf("📋 [cda.to_fhir] Mapping log persisted: %s", uri)
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

// formatSectionErrorWarnings promotes warning-severity SectionErrors into
// the generic "warnings" execution-detail key transformation_test_controller.go
// already merges into step_metadata for every executor (no CDA-specific
// wiring needed there) -- the pipeline-builder UI's per-step warning badge
// and top-level "Validation Warnings" panel both already render from this
// same generic shape, previously just fed an empty array. Real example this
// surfaces: declarative_engine.go's additionalValues check (a CDA entry with
// more than one <value> sibling -- only the first is ever mapped to
// Observation.value[x], the rest were silently dropped before that fix).
// Extracted from Execute() so the formatting itself is unit-testable
// without standing up a full executor.
func formatSectionErrorWarnings(sectionErrors []cdafhir.SectionError) []string {
	var warnings []string
	for _, se := range sectionErrors {
		if se.Severity != "warning" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("[%s] entry %d, %s: %s", se.SectionKey, se.EntryIndex, se.FieldKey, se.Error))
	}
	return warnings
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
		{Name: "Mapping log", Path: "_stepOutput.mappingLog", DataType: "object",
			Description: "Per-section timing, resource counts, dedup/synthesis events from CDA→FHIR assembly", Category: "CDA Transform"},
		{Name: "Mapping log summary", Path: "_stepOutput.mappingLog.summary", DataType: "object",
			Description: "Totals: resources, timing, deduplicated/synthesized counts", Category: "CDA Transform"},
	}
}
