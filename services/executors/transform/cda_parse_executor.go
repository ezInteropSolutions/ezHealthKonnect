// services/executors/transform/cda_parse_executor.go
// CDAParseExecutor — pipeline step type "cda.parse".
//
// Reads raw CDA/CCD XML from a configurable input field and produces
// a fully-parsed, USCDI-keyed ParsedJSON map that downstream steps
// (cda.normalize, cda.to_fhir) can consume without re-parsing.
//
// Config keys:
//   sourceField  — dot-path to the field holding raw CDA XML (default: "raw")
//   outputField  — top-level key to write ParsedJSON under (default: "parsedCDA")
//                  Use "" to merge ParsedJSON into the root of outputData directly.
//   schemaDir    — filesystem path to CDA schema directory (default: "./cda/schemas")

package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	cdastorage "ezhealthkonnect/services/cda_storage"
	cdaparser "ezhealthkonnect/services/parsers/cda"
	"ezhealthkonnect/services/executors"
)

// CDAParseExecutor parses raw CDA/CCD XML into a structured ParsedJSON map.
// It self-initialises from the schema directory at construction time; if the
// schema directory is missing the executor logs a warning and falls back to
// returning an error on Execute() (parsing cannot be degraded gracefully).
//
// When a CDADocumentStore is injected (via NewCDAParseExecutorWithStore), each
// successfully parsed document is persisted asynchronously; store errors are
// logged but never fail the pipeline.
type CDAParseExecutor struct {
	*executors.BaseExecutor
	parser *cdaparser.CDAParserService
	store  cdastorage.CDADocumentStore // optional; nil = storage disabled
}

// NewCDAParseExecutor constructs the executor and loads schemas from the
// default schema directory "./cda/schemas". Logs a warning if init fails;
// Execute() will return a descriptive error in that case.
func NewCDAParseExecutor() *CDAParseExecutor {
	return newCDAParseExecutorInternal(nil)
}

// NewCDAParseExecutorWithStore constructs the executor with an optional document store.
// If store is non-nil, every successfully parsed document is persisted asynchronously.
func NewCDAParseExecutorWithStore(store cdastorage.CDADocumentStore) *CDAParseExecutor {
	return newCDAParseExecutorInternal(store)
}

func newCDAParseExecutorInternal(store cdastorage.CDADocumentStore) *CDAParseExecutor {
	exec := &CDAParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.parse", models.ExecutorMetadata{
			Name:        "CDA Parser (XML → ParsedJSON)",
			Description: "Parses C-CDA 2.1 / C32 XML into a USCDI-keyed ParsedJSON document map",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
		store: store,
	}

	parser, err := cdaparser.NewFromSchemaDir("./cda/schemas")
	if err != nil {
		log.Printf("⚠️  [cda.parse] CDA parser init failed (%v) — executor will error on Execute()", err)
		return exec
	}
	exec.parser = parser
	if store != nil {
		log.Printf("✅ [cda.parse] CDAParserService loaded from ./cda/schemas (document storage: enabled)")
	} else {
		log.Printf("✅ [cda.parse] CDAParserService loaded from ./cda/schemas")
	}
	return exec
}

type cdaParseConfig struct {
	SourceField string `json:"sourceField"`
	OutputField string `json:"outputField"`
	SchemaDir   string `json:"schemaDir"`
}

// Execute reads raw CDA XML from sourceField, parses it, and writes the
// ParsedJSON map to outputField (default "parsedCDA") in the output data.
func (e *CDAParseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	if e.parser == nil {
		return nil, fmt.Errorf("cda.parse: CDAParserService is not initialised (schema directory missing)")
	}

	cfg := cdaParseConfig{SourceField: "raw", OutputField: "parsedCDA"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.SourceField == "" {
		cfg.SourceField = "raw"
	}
	if cfg.OutputField == "" {
		cfg.OutputField = "parsedCDA"
	}

	// Read raw XML from source field
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
	// cda.parse works whether it's the pipeline's first step or a later one.
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
		return nil, fmt.Errorf("cda.parse: source field %q is empty or not a string", cfg.SourceField)
	}

	// Parse
	result := e.parser.Parse(rawXML)
	if !result.Success {
		return nil, fmt.Errorf("cda.parse: %s", result.Error)
	}

	durationMs := time.Since(start).Milliseconds()

	// Count sections
	sectionCount := 0
	if sections, ok := result.ParsedJSON["sections"].(map[string]interface{}); ok {
		sectionCount = len(sections)
	}
	log.Printf("  ✅ [cda.parse] Parsed CDA document: %d sections, %d fields in %dms",
		sectionCount, len(result.EnhancedFields), durationMs)

	// Persist parsed document asynchronously (non-blocking, non-fatal).
	if e.store != nil && result.TypedDocument != nil {
		saveInput := &cdastorage.SaveInput{
			RawXML:          rawXML,
			ParsedJSON:      result.ParsedJSON,
			TypedDocument:   result.TypedDocument,
			ParseDurationMs: durationMs,
			SectionCount:    sectionCount,
			FieldCount:      len(result.EnhancedFields),
		}
		if ifID, ok := inputData["_interfaceId"].(string); ok {
			saveInput.InterfaceID = ifID
		}
		if msgID, ok := inputData["_messageId"].(string); ok {
			saveInput.MessageID = msgID
		}
		store := e.store
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := store.Save(ctx, saveInput); err != nil {
				log.Printf("⚠️  [cda.parse] Document storage failed (non-fatal): %v", err)
			}
		}()
	}

	// Build output
	outputData := make(map[string]interface{}, len(inputData)+4)
	for k, v := range inputData {
		outputData[k] = v
	}

	// Write ParsedJSON under the configured output field or merge into root
	if cfg.OutputField == "__root__" {
		for k, v := range result.ParsedJSON {
			outputData[k] = v
		}
	} else {
		outputData[cfg.OutputField] = result.ParsedJSON
	}

	// Also write format sentinel at root level so isCDAData() detects it
	if _, ok := outputData["_format"]; !ok {
		if fmt_, ok := result.ParsedJSON["_format"]; ok {
			outputData["_format"] = fmt_
		}
	}

	// Attach the Sprint B typed CDADocument for use by Sprint C/D validators and mappers.
	// setCDADocument stores it inside the shared message object (not a sibling
	// key on this step's own outputData) — the only form ExecutePipeline
	// actually carries forward to the next step; see cda_document_resolution.go.
	if typedDoc, ok := result.TypedDocument.(*cdadocument.CDADocument); ok {
		setCDADocument(outputData, typedDoc)
	}

	stepOut := map[string]interface{}{
		"parsedCDA":    result.ParsedJSON,
		"sectionCount": sectionCount,
		"fieldCount":   len(result.EnhancedFields),
	}
	if result.TypedDocument != nil {
		stepOut["cdaDocument"] = result.TypedDocument
	}
	e.SetStepOutputWithDetails(outputData,
		stepOut,
		map[string]interface{}{
			"duration_ms":   durationMs,
			"success":       true,
			"section_count": sectionCount,
			"field_count":   len(result.EnhancedFields),
		},
	)

	return outputData, nil
}

// Validate checks step configuration.
func (e *CDAParseExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// GetOutputVariables declares the ParsedJSON output for the field picker.
func (e *CDAParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Parsed CDA", Path: "_stepOutput.parsedCDA", DataType: "object",
			Description: "USCDI-keyed ParsedJSON document map (header + sections)", Category: "CDA Transform"},
		{Name: "CDA Profile", Path: "_stepOutput.parsedCDA.cdaProfile", DataType: "string",
			Description: "Detected CDA profile: C-CDA 2.1 | C32 | HITSP", Category: "CDA Transform"},
		{Name: "Section count", Path: "_stepOutput.sectionCount", DataType: "number",
			Description: "Number of CDA sections parsed", Category: "CDA Transform"},
		{Name: "Patient header", Path: "_stepOutput.parsedCDA.header.patient", DataType: "object",
			Description: "Patient demographics extracted from CDA document header", Category: "CDA Transform"},
		{Name: "Typed CDA document", Path: "message._cdaDocument", DataType: "object",
			Description: "Typed *CDADocument with full struct fidelity, consumed automatically by a following cda.to_fhir/cda.section_to_csv/cda.dedupe step — not directly reference-able as a field value", Category: "CDA Transform"},
	}
}
