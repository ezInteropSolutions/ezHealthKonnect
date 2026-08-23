package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
	cdastorage "ezhealthkonnect/services/cda_storage"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/executors/enrichment"
	"ezhealthkonnect/services/metrics"
	payloadexecutor "ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/transform"
	"ezhealthkonnect/services/executors/validation"
	"ezhealthkonnect/services/hl7assembly"
	"ezhealthkonnect/services/storage"
)

// StepExecutor interface - all executors must implement this
type StepExecutor interface {
	// Execute runs the transformation step
	Execute(
		ctx context.Context,
		step *models.TransformationStep,
		inputData map[string]interface{},
	) (map[string]interface{}, error)

	// GetStepType returns the step type this executor handles
	GetStepType() string

	// Validate validates step configuration
	Validate(step *models.TransformationStep) error
}

// ExecutorRegistry manages all available step executors (Factory Pattern + OOB)
type ExecutorRegistry struct {
	executors map[string]StepExecutor
	db        *sql.DB
	credStore *CredentialStore // nil = passthrough (dev/test only)
}

// NewExecutorRegistry creates a new executor registry with auto-registration (OOB).
// credStore may be nil; if set, encrypted connectivity configs are decrypted transparently
// when executors read credentials from the database.
func NewExecutorRegistry(db *sql.DB, credStore *CredentialStore) *ExecutorRegistry {
	registry := &ExecutorRegistry{
		executors: make(map[string]StepExecutor),
		db:        db,
		credStore: credStore,
	}

	// OOB: Auto-register all built-in executors
	registry.autoRegisterExecutors()

	log.Printf("✅ Executor Registry initialized with %d executors", len(registry.executors))

	return registry
}

// SetCodeTemplateService wires a CodeTemplateService into the script enrichment executor
// so that function libraries are injected into every script step's goja VM.
// Call this after NewExecutorRegistry and after the CodeTemplateService is ready.
func (er *ExecutorRegistry) SetCodeTemplateService(svc *CodeTemplateService) {
	scriptExec := enrichment.NewScriptEnrichmentExecutorWithTemplates(svc)
	er.executors["enrichment.script"] = scriptExec
	er.executors["pre.enrichment.script"] = scriptExec
	log.Printf("📦 [CodeTemplates] Script executor upgraded with code template injection")
}

// SetDLQService replaces the connector.outbound executor with a DLQ-aware version.
// Call this after NewExecutorRegistry once the DLQService is ready.
func (er *ExecutorRegistry) SetDLQService(dlqSvc *connectors.DLQService) {
	outbound := transform.NewOutboundConnectorExecutorWithDLQ(dlqSvc)
	er.executors["connector.outbound"] = outbound
	log.Printf("📥 [DLQ] Outbound connector executor upgraded with DLQ support")
}

// SetCDADocumentStore upgrades the cda.parse executor with a document store so that
// every successfully parsed CDA document is persisted asynchronously. Also wires the
// same store into cda.to_fhir so its auto-parse path (used when a pipeline has no
// separate cda.parse step) persists its parsed content too — cda.to_fhir's Execute()
// only actually saves when it took the auto-parse path, so this never double-saves
// a document a cda.parse step already persisted.
// Call this after NewExecutorRegistry once the CDADocumentStore is ready.
func (er *ExecutorRegistry) SetCDADocumentStore(store cdastorage.CDADocumentStore) {
	er.executors["cda.parse"] = transform.NewCDAParseExecutorWithStore(store)
	log.Printf("📄 [CDA Storage] cda.parse executor upgraded with document storage")

	if exec, ok := er.executors["cda.to_fhir"].(*transform.CDAToFHIRExecutor); ok {
		exec.SetCDADocumentStore(store)
		log.Printf("📄 [CDA Storage] cda.to_fhir executor upgraded with document storage (auto-parse persistence)")
	}
}

// SetCDAToFHIRObjectStorage wires the ObjectStorageService into the cda.to_fhir executor
// so that the MappingLog produced by each document conversion is persisted asynchronously.
// Call this after NewExecutorRegistry once the ObjectStorageService is ready.
func (er *ExecutorRegistry) SetCDAToFHIRObjectStorage(svc *storage.ObjectStorageService) {
	if exec, ok := er.executors["cda.to_fhir"].(*transform.CDAToFHIRExecutor); ok {
		exec.SetObjectStorageService(svc)
		log.Printf("📋 [MappingLog] cda.to_fhir executor upgraded with object storage for mapping logs")
	}
}

// autoRegisterExecutors registers all built-in executors (OOB pattern)
func (er *ExecutorRegistry) autoRegisterExecutors() {
	// Essential OOB executor
	er.Register(NewPassthroughExecutor())

	// Validation executors
	er.Register(validation.NewFieldValidationExecutor()) // field_validation
	er.Register(validation.NewFHIRValidationExecutor())  // fhir_validation

	// Control flow executors
	er.Register(control.NewIfThenElseExecutor())  // if_then_else
	er.Register(control.NewSwitchCaseExecutor())  // switch_case
	er.Register(control.NewLoopExecutor())        // control.loop
	er.Register(control.NewTryCatchExecutor())    // control.try_catch
	er.Register(control.NewRetryExecutor())       // control.retry

	// Enrichment executors
	er.Register(enrichment.NewAPIEnrichmentExecutor())           // enrichment.api
	er.Register(enrichment.NewDatabaseEnrichmentExecutor(er.db)) // enrichment.database
	er.Register(enrichment.NewScriptEnrichmentExecutor())        // enrichment.script

	// Transformation executors
	hl7FhirExecutor := NewHL7FHIRMappingExecutor(er.db)
	er.Register(hl7FhirExecutor)                             // hl7_fhir_transform
	er.Register(enrichment.NewFieldMappingExecutor())        // field_mapping
	er.Register(enrichment.NewFileParserExecutor(er.db, er.credStore.DecryptConfigBytes)) // file_parser

	// HL7 structural assembly executors
	er.Register(transform.NewHL7AssembleObservationsExecutor()) // hl7.assemble_observations

	// Data transform executors
	er.Register(transform.NewDataMaskingExecutor())          // data_masking
	er.Register(transform.NewRemoveDuplicatesExecutor())     // remove_duplicates
	er.Register(transform.NewNormalizerExecutor())           // normalizer
	er.Register(transform.NewDeidentifyExecutor())           // deidentify (HIPAA P1)

	// CDA transform executors
	er.Register(transform.NewCDAToFHIRExecutor(er.db))     // cda.to_fhir
	er.Register(transform.NewFHIRToCDAExecutor())          // fhir.to_cda
	er.Register(transform.NewCDABuildExecutor())           // cda.build
	er.Register(transform.NewMapToCanonicalExecutor())     // cda.map_to_canonical
	er.Register(transform.NewCDANormalizerExecutor())      // cda.normalize
	er.Register(transform.NewCDAParseExecutor())           // cda.parse
	er.Register(transform.NewCDASectionToCSVExecutor())    // cda.section_to_csv
	er.Register(transform.NewCDADedupeExecutor(er.db))     // cda.dedupe

	// FHIR transform executors
	er.Register(transform.NewFHIRBuildExecutor()) // fhir.build

	// HL7 transform executors
	er.Register(transform.NewHL7BuildExecutor()) // hl7.build

	// Connector bridge executors (DLQ service wired later via SetDLQService)
	er.Register(transform.NewOutboundConnectorExecutor())    // connector.outbound
	er.Register(transform.NewInboundConnectorExecutor())     // connector.inbound

	// Payload builder executor
	payloadBuilder := payloadexecutor.NewPayloadBuilderExecutor(er.db)
	er.Register(payloadBuilder)                              // payload.builder
	er.executors["payload_builder"] = payloadBuilder        // alias: underscore variant

	// Custom executors
	er.Register(NewGenericExecutor())

	// Backward-compat aliases: old layer-prefixed type names → new executors
	er.executors["pre.validation"] = er.executors["field_validation"]
	er.executors["core.validation"] = er.executors["field_validation"]
	er.executors["post.validation"] = er.executors["fhir_validation"]
	er.executors["pre.logic"] = er.executors["if_then_else"]
	er.executors["pre.logic.switch"] = er.executors["switch_case"]
	er.executors["pre.enrichment.api"] = er.executors["enrichment.api"]
	er.executors["pre.enrichment.database"] = er.executors["enrichment.database"]
	er.executors["pre.enrichment.script"] = er.executors["enrichment.script"]
	er.executors["core.mapping"] = hl7FhirExecutor
	er.executors["hl7_to_fhir_mapping"] = hl7FhirExecutor
	er.executors["core.transformation"] = er.executors["field_mapping"]
	er.executors["post.data_masking"] = er.executors["data_masking"]
	er.executors["post.remove_duplicates"] = er.executors["remove_duplicates"]
	er.executors["post.normalizer"] = er.executors["normalizer"]
	er.executors["pre.deidentify"] = er.executors["deidentify"]
	er.executors["post.deidentify"] = er.executors["deidentify"]

	// Da Vinci PAS envelope mapping step — guided UI, same field_mapping executor
	er.executors["pas_envelope_mapping"] = er.executors["field_mapping"]

	log.Println("  ✓ All executors registered with backward-compatible aliases")
}

// Register adds an executor to the registry
func (er *ExecutorRegistry) Register(executor StepExecutor) {
	stepType := executor.GetStepType()
	er.executors[stepType] = executor
	log.Printf("  📝 Registered executor: %s", stepType)
}

// GetExecutor returns the executor for a given step type
func (er *ExecutorRegistry) GetExecutor(stepType string) StepExecutor {
	if executor, exists := er.executors[stepType]; exists {
		return executor
	}

	// Fallback to generic executor
	log.Printf("⚠️  No specific executor for '%s', using generic executor", stepType)
	return er.executors["generic"]
}

// ExecuteStep executes a transformation step (MVC Controller pattern).
// Before dispatching to the executor, any ENC:v1:-prefixed credential values in
// step.Config are transparently decrypted by the CredentialStore. This covers DB
// passwords, API keys, connection strings, etc. saved by pipelineController.js.
func (er *ExecutorRegistry) ExecuteStep(
	ctx context.Context,
	step models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	// Get appropriate executor for step type
	executor := er.GetExecutor(step.StepType)
	if executor == nil {
		return nil, fmt.Errorf("no executor available for step type: %s", step.StepType)
	}

	// Normalize aliased step types to their canonical executor type so that
	// BaseExecutor.PreExecute() doesn't fail the type-equality check.
	// e.g. "pre.enrichment.script" → "enrichment.script"
	if canonical := executor.GetStepType(); canonical != "" && canonical != "generic" && canonical != step.StepType {
		log.Printf("  📌 Normalizing step type alias: %s → %s", step.StepType, canonical)
		step.StepType = canonical
	}

	// Decrypt any encrypted credential values before the executor sees them.
	// DecryptConfigFields is a no-op when credStore is nil (dev mode) or when
	// no values start with ENC:v1: (backward compat with unencrypted configs).
	if len(step.Config) > 0 {
		decrypted, err := er.credStore.DecryptConfigFields(step.Config)
		if err != nil {
			log.Printf("⚠️  [CredentialStore] Failed to decrypt step config for %q: %v — executing with original config", step.StepName, err)
		} else {
			step.Config = decrypted
		}
	}

	// Validate step configuration
	if err := executor.Validate(&step); err != nil {
		return nil, fmt.Errorf("step validation failed: %w", err)
	}

	// Execute the step, timing it and recording outcome for Prometheus.
	start := time.Now()
	result, err := executor.Execute(ctx, &step, inputData)
	elapsed := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
	}
	if metrics.StepExecutions != nil {
		metrics.StepExecutions.WithLabelValues(step.StepType, status).Inc()
	}
	if metrics.StepDuration != nil {
		metrics.StepDuration.WithLabelValues(step.StepType).Observe(elapsed.Seconds())
	}

	// STEP EXEC lifecycle log point — see models.LogLifecycleEventFn's doc comment
	// for why this goes through a context callback instead of an import.
	if logFn := models.GetLogLifecycleEventFn(ctx); logFn != nil {
		details := map[string]interface{}{
			"step_type": step.StepType,
			"step_name": step.StepName,
			"step_seq":  step.Sequence,
			"duration_ms": elapsed.Milliseconds(),
			"status":    status,
		}
		if err != nil {
			details["error"] = err.Error()
			logFn("error", string(LogCategoryTransformation), fmt.Sprintf("Step failed: %s", step.StepName), details)
		} else {
			logFn("debug", string(LogCategoryTransformation), fmt.Sprintf("Step executed: %s", step.StepName), details)
		}
	}

	return result, err
}

// ListExecutors returns all registered executor types
func (er *ExecutorRegistry) ListExecutors() []string {
	types := make([]string, 0, len(er.executors))
	for stepType := range er.executors {
		types = append(types, stepType)
	}
	return types
}

// ===============================================================
// LEGACY EXECUTORS REMOVED
// ===============================================================
// The following executors have been removed during consolidation:
// - ValidationExecutor (replaced by FieldValidationExecutor)
// - MetadataEnrichmentExecutor (merged into FieldMappingExecutor)
// - JavaScriptExecutor (replaced by ScriptEnrichmentExecutor)
// - EnrichmentExecutor (placeholder with no functionality)
// See EXECUTOR_CONSOLIDATION_PLAN.md for details

// ===============================================================
// HL7→FHIR MAPPING EXECUTOR
// ===============================================================

// HL7FHIRMappingExecutor handles HL7 to FHIR transformation
type HL7FHIRMappingExecutor struct {
	db               *sql.DB
	transformService *HL7FHIRTransformServiceV3
	scorer           *TransformationScorer
}

func NewHL7FHIRMappingExecutor(db *sql.DB) *HL7FHIRMappingExecutor {
	return &HL7FHIRMappingExecutor{
		db:               db,
		transformService: NewHL7FHIRTransformServiceV3(db),
		scorer:           NewTransformationScorer(db),
	}
}

func (hme *HL7FHIRMappingExecutor) GetStepType() string {
	return "hl7_fhir_transform"
}

func (hme *HL7FHIRMappingExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  🔄 HL7→FHIR transformation starting...")
	log.Printf("  🔍 [DEBUG] Transform service initialized: %v", hme.transformService != nil)
	log.Printf("  🔍 [DEBUG] Database connection: %v", hme.db != nil)

	// Extract the actual HL7 message data from the pipeline context
	// Pipeline wraps message under "message" key in executeStepWithContext
	parsedHL7Data := inputData
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		parsedHL7Data = msg
		log.Printf("  🔍 [DEBUG] Extracted HL7 data from 'message' wrapper")
	}

	// Get interface ID from config or input
	interfaceID, _ := step.Config["interface_id"].(string)
	if interfaceID == "" {
		interfaceID, _ = inputData["interface_id"].(string)
	}
	if interfaceID == "" {
		interfaceID, _ = parsedHL7Data["interface_id"].(string)
	}

	// resolveHL7Data walks nested "message" keys to find the innermost map that
	// contains "enhancedSegments" (the actual parsed HL7 data). This is necessary
	// because when a prior step's error is caught, execCtx.Message is replaced with
	// the full step output, burying the real HL7 data under successive "message" nesting.
	resolveHL7Data := func(m map[string]interface{}) map[string]interface{} {
		current := m
		for i := 0; i < 5; i++ { // guard against infinite nesting
			if _, hasSegments := current["enhancedSegments"]; hasSegments {
				return current
			}
			nested, ok := current["message"].(map[string]interface{})
			if !ok {
				break
			}
			current = nested
		}
		return m // return original if no enhancedSegments found
	}
	parsedHL7Data = resolveHL7Data(parsedHL7Data)

	// extractMsgType extracts the messageType string from a map value that may be
	// a plain string, a hl7.MessageTypeInfo struct, or a generic map.
	extractMsgType := func(m map[string]interface{}) string {
		switch mt := m["messageType"].(type) {
		case string:
			return mt
		case hl7.MessageTypeInfo:
			return mt.Name
		case map[string]interface{}:
			if name, ok := mt["name"].(string); ok {
				return name
			}
		}
		return ""
	}

	messageType := extractMsgType(parsedHL7Data)
	if messageType == "" {
		messageType = extractMsgType(inputData)
	}
	// Also check step config (production path injects message_type via ExecuteTransformation)
	if messageType == "" {
		messageType, _ = step.Config["message_type"].(string)
	}
	messageID, _ := parsedHL7Data["message_id"].(string)
	correlationID, _ := parsedHL7Data["correlation_id"].(string)

	log.Printf("  🔍 [DEBUG] MessageType: '%s', InterfaceID: '%s'", messageType, interfaceID)
	log.Printf("  🔍 [DEBUG] MessageID: '%s', CorrelationID: '%s'", messageID, correlationID)

	// Read selected_resources from step config (user-chosen FHIR resource types)
	var selectedResources []string
	if raw, ok := step.Config["selected_resources"]; ok {
		switch v := raw.(type) {
		case []string:
			selectedResources = v
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					selectedResources = append(selectedResources, s)
				}
			}
		}
	}

	// Call existing transformation service using Transform method
	_ = selectedResources // filter applied post-transform if needed

	// assembleObservations defaults to true; set to false in step config to disable entirely.
	assembleInline := true
	if v, ok := step.Config["assembleObservations"].(bool); ok {
		assembleInline = v
	}

	// assemblyRules — per-rule on/off from UI checkboxes saved in step.config.assemblyRules
	var assemblyRules hl7assembly.AssemblyRules
	if rulesRaw, ok := step.Config["assemblyRules"].(map[string]interface{}); ok {
		assemblyRules = hl7assembly.AssemblyRulesFromConfig(rulesRaw)
	}

	// embedded_mappings — wizard-saved field mappings stored directly in step config.
	// These take priority over DB lookups and survive message-type variant mismatches
	// (e.g. ORU^R03 arriving on an interface configured for ORU^R01).
	var embeddedMappings []map[string]interface{}
	if raw, ok := step.Config["embedded_mappings"]; ok {
		switch v := raw.(type) {
		case []map[string]interface{}:
			embeddedMappings = v
		case []interface{}:
			for _, item := range v {
				if m, ok2 := item.(map[string]interface{}); ok2 {
					embeddedMappings = append(embeddedMappings, m)
				}
			}
		}
		log.Printf("  🔍 [DEBUG] Using %d embedded_mappings from step config", len(embeddedMappings))
	}

	req := &TransformRequest{
		ParsedHL7Data:    parsedHL7Data,
		MessageType:      messageType,
		FHIRVersion:      "R4",
		CreateBundle:     true,
		InterfaceID:      interfaceID,
		SkipAssembly:     !assembleInline,
		AssemblyRules:    assemblyRules,
		EmbeddedMappings: embeddedMappings,
	}

	log.Printf("  🔍 [DEBUG] Calling Transform service...")
	resp, err := hme.transformService.Transform(ctx, req)
	log.Printf("  🔍 [DEBUG] Transform returned - err: %v, resp: %v", err, resp != nil)
	if err != nil {
		return nil, fmt.Errorf("HL7→FHIR transformation failed: %w", err)
	}

	// Score the transformation asynchronously — zero impact on delivery latency.
	if hme.scorer != nil {
		go hme.scorer.ScoreAndPersist(messageID, interfaceID, messageType, resp)
	}

	// Debug: Check what we got from transformation
	log.Printf("  🔍 [DEBUG] Transform response - Bundle: %v, Success: %v", resp.Bundle != nil, resp.Success)
	if resp.Bundle != nil {
		bundleJSON, _ := json.Marshal(resp.Bundle)
		preview := string(bundleJSON)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		log.Printf("  🔍 [DEBUG] Bundle size: %d bytes, preview: %s...", len(bundleJSON), preview)
	} else {
		log.Printf("  ⚠️  [DEBUG] Bundle is NIL!")
	}

	// Create DeliveryPayload for FHIR over HTTP transmission
	deliveryPayload := hme.createFHIRDeliveryPayload(
		ctx,
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		resp.Bundle,
		step.Config,
	)

	// Add to output data
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Store delivery payload (this will be extracted by engine for transmission)
	outputData["_deliveryPayload"] = deliveryPayload

	// Keep transformed content for storage and downstream access
	outputData["fhirBundle"] = resp.Bundle

	// Expose individual resources for downstream steps (e.g. hl7.assemble_observations)
	outputData["fhirResources"] = resp.FHIRResources

	// Also store in the message so downstream steps (FHIR Validation, etc.) can access it
	if msg, ok := outputData["message"].(map[string]interface{}); ok {
		msg["fhirBundle"] = resp.Bundle
	}

	// STANDARDIZED: Variables (the FHIR bundle) + execution details
	if resp.Bundle == nil {
		errMsg := fmt.Sprintf("No FHIR bundle produced for message type '%s' — no field mappings found", messageType)
		if len(resp.Errors) > 0 {
			errMsg = fmt.Sprintf("%s; transform errors: %v", errMsg, resp.Errors)
		}
		log.Printf("  ⚠️  HL7→FHIR: %s", errMsg)
		outputData["_stepOutput"] = map[string]interface{}{
			"fhirBundle": nil,
			"error":      errMsg,
			"errors":     resp.Errors,
		}
		outputData["_executionDetails"] = map[string]interface{}{
			"format":         "fhir-r4",
			"transformation": "hl7_to_fhir",
			"success":        false,
			"messageType":    messageType,
		}
	} else {
		outputData["_stepOutput"] = map[string]interface{}{
			"fhirBundle": resp.Bundle,
		}
		outputData["_executionDetails"] = map[string]interface{}{
			"format":         "fhir-r4",
			"resourceType":   resp.Bundle["resourceType"],
			"transformation": "hl7_to_fhir",
			"success":        true,
			"messageType":    messageType,
		}
	}

	log.Printf("  ✅ HL7→FHIR transformation complete with delivery payload and step output")

	return outputData, nil
}

// GetOutputVariables declares the FHIR R4 output schema so the path picker can show
// all available fields without requiring a test pipeline run.
// The step always produces step_output.fhir_bundle (a FHIR Bundle), which after
// output normalization has snake_case keys. All standard Patient, Encounter, and
// Observation resource fields are declared here.
func (hme *HL7FHIRMappingExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	base := "fhir_bundle.entry[0].resource"
	return []models.VariableDefinition{
		// Bundle root
		{Name: "FHIR Bundle", Path: "fhir_bundle", DataType: "object", Description: "Full FHIR R4 Bundle produced by HL7→FHIR transform", Category: "FHIR Transform"},
		{Name: "Bundle entries", Path: "fhir_bundle.entry", DataType: "array", Description: "Array of FHIR Bundle entries (resources)", Category: "FHIR Transform"},
		// Patient
		{Name: "Patient ID", Path: base + ".patient.id", DataType: "string", Description: "FHIR Patient resource id", Category: "FHIR Patient"},
		{Name: "Patient family name", Path: base + ".patient.name[0].family", DataType: "string", Description: "Patient family (last) name", Category: "FHIR Patient"},
		{Name: "Patient given name", Path: base + ".patient.name[0].given[0]", DataType: "string", Description: "Patient first given name", Category: "FHIR Patient"},
		{Name: "Patient birth date", Path: base + ".patient.birth_date", DataType: "string", Description: "Patient date of birth (YYYY-MM-DD)", Category: "FHIR Patient"},
		{Name: "Patient gender", Path: base + ".patient.gender", DataType: "string", Description: "Patient administrative gender", Category: "FHIR Patient"},
		{Name: "Patient MRN", Path: base + ".patient.identifier[0].value", DataType: "string", Description: "Patient MRN (first identifier value)", Category: "FHIR Patient"},
		{Name: "Patient SSN", Path: base + ".patient.identifier[1].value", DataType: "string", Description: "Patient SSN (second identifier value, if mapped)", Category: "FHIR Patient"},
		{Name: "Patient phone", Path: base + ".patient.telecom[0].value", DataType: "string", Description: "Patient phone number", Category: "FHIR Patient"},
		{Name: "Patient email", Path: base + ".patient.telecom[1].value", DataType: "string", Description: "Patient email (if mapped)", Category: "FHIR Patient"},
		{Name: "Patient address street", Path: base + ".patient.address[0].line[0]", DataType: "string", Description: "Patient street address", Category: "FHIR Patient"},
		{Name: "Patient address city", Path: base + ".patient.address[0].city", DataType: "string", Description: "Patient city", Category: "FHIR Patient"},
		{Name: "Patient address state", Path: base + ".patient.address[0].state", DataType: "string", Description: "Patient state", Category: "FHIR Patient"},
		{Name: "Patient address zip", Path: base + ".patient.address[0].postal_code", DataType: "string", Description: "Patient postal code", Category: "FHIR Patient"},
		// Encounter
		{Name: "Encounter ID", Path: base + ".encounter.id", DataType: "string", Description: "FHIR Encounter resource id", Category: "FHIR Encounter"},
		{Name: "Encounter class", Path: base + ".encounter.class.code", DataType: "string", Description: "Encounter class code (IMP, AMB, etc.)", Category: "FHIR Encounter"},
		{Name: "Encounter status", Path: base + ".encounter.status", DataType: "string", Description: "Encounter status", Category: "FHIR Encounter"},
		{Name: "Encounter start", Path: base + ".encounter.period.start", DataType: "string", Description: "Encounter start date/time", Category: "FHIR Encounter"},
		{Name: "Encounter end", Path: base + ".encounter.period.end", DataType: "string", Description: "Encounter end date/time", Category: "FHIR Encounter"},
		// Observation
		{Name: "Observation ID", Path: base + ".observation.id", DataType: "string", Description: "FHIR Observation resource id", Category: "FHIR Observation"},
		{Name: "Observation status", Path: base + ".observation.status", DataType: "string", Description: "Observation status", Category: "FHIR Observation"},
		{Name: "Observation value", Path: base + ".observation.value_quantity.value", DataType: "number", Description: "Observation numeric value", Category: "FHIR Observation"},
		{Name: "Observation unit", Path: base + ".observation.value_quantity.unit", DataType: "string", Description: "Observation unit of measure", Category: "FHIR Observation"},
	}
}

// getDestinationConfig retrieves the target endpoint and auth headers from interface configuration
func (hme *HL7FHIRMappingExecutor) getDestinationConfig(
	ctx context.Context,
	interfaceID string,
	stepConfig map[string]interface{},
) (string, map[string]string) {
	authHeaders := make(map[string]string)
	// First check step config for override
	if endpoint, ok := stepConfig["destination_endpoint"].(string); ok && endpoint != "" {
		return endpoint, authHeaders
	}

	// Query interface target_config from database
	query := `
		SELECT target_config
		FROM interfaces
		WHERE id = $1 AND deleted_at IS NULL
	`

	var targetConfigJSON []byte
	err := hme.db.QueryRowContext(ctx, query, interfaceID).Scan(&targetConfigJSON)
	if err != nil {
		log.Printf("⚠️  Failed to get target_config for interface %s: %v, using default", interfaceID, err)
		return "http://localhost:8080/fhir", authHeaders // Fallback default
	}

	// Parse target_config
	var targetConfig map[string]interface{}
	if err := json.Unmarshal(targetConfigJSON, &targetConfig); err != nil {
		log.Printf("⚠️  Failed to parse target_config JSON: %v, using default", err)
		return "http://localhost:8080/fhir", authHeaders
	}

	var endpoint string

	// NEW FORMAT: Check for direct "endpoint" field first (wizard/edit modal unified format)
	if ep, ok := targetConfig["endpoint"].(string); ok && ep != "" {
		endpoint = ep
		log.Printf("📍 Using endpoint from unified format: %s", endpoint)
	} else {
		// LEGACY FORMAT: Build from components (protocol, host, port, path)
		protocol, _ := targetConfig["protocol"].(string)
		host, _ := targetConfig["host"].(string)
		path, _ := targetConfig["path"].(string)

		// Port can be float64 (from JSON) or string
		var port string
		switch v := targetConfig["port"].(type) {
		case float64:
			port = fmt.Sprintf("%.0f", v)
		case string:
			port = v
		}

		if protocol == "" || host == "" {
			log.Printf("⚠️  Incomplete target_config for interface %s, using default", interfaceID)
			return "http://localhost:8080/fhir", authHeaders
		}

		// Build full endpoint URL
		endpoint = fmt.Sprintf("%s://%s", protocol, host)
		if port != "" && port != "80" && port != "443" {
			endpoint += ":" + port
		}
		if path != "" {
			endpoint += path
		}
		log.Printf("📍 Built endpoint from legacy format: %s", endpoint)
	}

	// Extract authentication configuration
	if authType, ok := targetConfig["authType"].(string); ok {
		switch authType {
		case "basic":
			username, _ := targetConfig["username"].(string)
			password, _ := targetConfig["password"].(string)
			if username != "" && password != "" {
				// Base64 encode username:password for Basic Auth
				auth := username + ":" + password
				authHeaders["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
				log.Printf("📍 Added Basic Auth for user: %s", username)
			}
		case "bearer":
			if token, ok := targetConfig["bearerToken"].(string); ok && token != "" {
				authHeaders["Authorization"] = "Bearer " + token
				log.Printf("📍 Added Bearer token authentication")
			}
		case "apikey":
			if apiKey, ok := targetConfig["apiKey"].(string); ok && apiKey != "" {
				authHeaders["X-API-Key"] = apiKey
				log.Printf("📍 Added API Key authentication")
			}
		}
	}

	return endpoint, authHeaders
}

// createFHIRDeliveryPayload creates a delivery payload for FHIR over HTTP
func (hme *HL7FHIRMappingExecutor) createFHIRDeliveryPayload(
	ctx context.Context,
	messageID string,
	correlationID string,
	interfaceID string,
	pipelineID string,
	fhirBundle map[string]interface{},
	stepConfig map[string]interface{},
) *models.DeliveryPayload {

	// Serialize FHIR bundle to JSON bytes
	fhirJSON, err := json.Marshal(fhirBundle)
	if err != nil {
		log.Printf("⚠️  Failed to marshal FHIR bundle: %v", err)
		fhirJSON = []byte("{}")
	}

	// Get destination endpoint and auth headers from interface target_config
	destinationEndpoint, authHeaders := hme.getDestinationConfig(ctx, interfaceID, stepConfig)

	// Create HTTP headers for FHIR
	httpHeaders := map[string]string{
		"Content-Type": "application/fhir+json",
		"Accept":       "application/fhir+json",
	}

	// Merge authentication headers from target_config
	for k, v := range authHeaders {
		httpHeaders[k] = v
	}

	// Add authentication overrides from step config if present
	if authToken, ok := stepConfig["auth_token"].(string); ok && authToken != "" {
		httpHeaders["Authorization"] = "Bearer " + authToken
	}
	if apiKey, ok := stepConfig["api_key"].(string); ok && apiKey != "" {
		httpHeaders["X-API-Key"] = apiKey
	}

	// Create transmission payload (ONLY what gets sent over wire)
	transmission := models.NewFHIRTransmissionPayload(fhirJSON, httpHeaders)

	// Create full delivery payload
	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		pipelineID,
		transmission,
		"http_rest",                // Destination type
		destinationEndpoint,        // Destination endpoint
	)

	// Add format metadata
	payload.Format = "fhir"
	payload.FormatVersion = "R4"
	payload.MessageType = "Bundle"

	resourceType, _ := fhirBundle["resourceType"].(string)
	payload.ResourceType = resourceType

	payload.FormatMetadata = map[string]interface{}{
		"fhir_version":  "R4",
		"bundle_type":   fhirBundle["type"],
		"resource_type": resourceType,
	}

	// Add connectivity metadata
	payload.ConnectivityMetadata = map[string]interface{}{
		"method":           "POST",
		"timeout_seconds":  30,
		"retry_on_5xx":     true,
		"expected_status":  []int{200, 201},
	}

	// Set destination name
	payload.DestinationName = "FHIR Server"
	if name, ok := stepConfig["destination_name"].(string); ok {
		payload.DestinationName = name
	}

	return payload
}

func (hme *HL7FHIRMappingExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// FHIR VALIDATION EXECUTOR
// ===============================================================

// FHIRValidationExecutor - moved to services/executors/validation/fhir_validation_executor.go

// ===============================================================
// GENERIC EXECUTOR (Fallback)
// ===============================================================

// GenericExecutor is a fallback executor for unknown step types
type GenericExecutor struct{}

func NewGenericExecutor() *GenericExecutor {
	return &GenericExecutor{}
}

func (ge *GenericExecutor) GetStepType() string {
	return "generic"
}

func (ge *GenericExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  ⚙️  Generic executor: %s (pass-through)", step.StepName)

	// Generic executor just passes through the input
	return inputData, nil
}

func (ge *GenericExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// getNestedValue retrieves a value from nested map using dot notation
// Example: getNestedValue(data, "patient.name.family") => data["patient"]["name"]["family"]
func getNestedValue(data map[string]interface{}, path string) interface{} {
	log.Printf("[getNestedValue] Path: %s", path)
	keys := strings.Split(path, ".")
	var current interface{} = data

	for _, key := range keys {
		log.Printf("[getNestedValue]   Processing key: %s, current type: %T", key, current)

		// Handle array access like "fields[1]"
		if strings.Contains(key, "[") {
			parts := strings.Split(key, "[")
			mapKey := parts[0]
			indexStr := strings.TrimSuffix(parts[1], "]")
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				log.Printf("[getNestedValue]   Failed to parse index: %v", err)
				return nil
			}

			// Get map value
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				log.Printf("[getNestedValue]   Current is not a map")
				return nil
			}

			arrayValue, exists := currentMap[mapKey]
			if !exists {
				log.Printf("[getNestedValue]   Key %s not found in map", mapKey)
				return nil
			}

			// Convert to array
			array, ok := arrayValue.([]interface{})
			if !ok {
				log.Printf("[getNestedValue]   %s is not an array, type: %T", mapKey, arrayValue)
				return nil
			}

			// Check bounds
			if index < 0 || index >= len(array) {
				log.Printf("[getNestedValue]   Index %d out of bounds (len=%d)", index, len(array))
				return nil
			}

			current = array[index]
		} else {
			// Normal map access
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				log.Printf("[getNestedValue]   Current is not a map for key: %s", key)
				return nil
			}

			value, exists := currentMap[key]
			if !exists {
				log.Printf("[getNestedValue]   Key %s not found", key)
				return nil
			}

			current = value
		}
	}

	log.Printf("[getNestedValue] Final value: %v (type: %T)", current, current)
	return current
}



// ===============================================================
// PASSTHROUGH EXECUTOR (OOB - Most Common Use Case)
// ===============================================================

// PassthroughExecutor simply passes data through without transformation
// Used for raw HL7 delivery, JSON passthrough, etc.
type PassthroughExecutor struct{}

func NewPassthroughExecutor() *PassthroughExecutor {
	return &PassthroughExecutor{}
}

func (e *PassthroughExecutor) GetStepType() string {
	return "passthrough"
}

func (e *PassthroughExecutor) Validate(step *models.TransformationStep) error {
	// No validation needed for passthrough
	return nil
}

func (e *PassthroughExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	log.Printf("▶️  Passthrough: Creating delivery payload for original format")

	// Get metadata
	messageID, _ := inputData["message_id"].(string)
	correlationID, _ := inputData["correlation_id"].(string)
	interfaceID, _ := inputData["interface_id"].(string)
	format, _ := inputData["format"].(string)

	// Create delivery payload based on format
	var deliveryPayload *models.DeliveryPayload

	switch format {
	case "hl7v2":
		deliveryPayload = e.createHL7DeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	case "json":
		deliveryPayload = e.createJSONDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	case "xml":
		deliveryPayload = e.createXMLDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	default:
		deliveryPayload = e.createGenericDeliveryPayload(messageID, correlationID, interfaceID, step, inputData)
	}

	// Add delivery payload to output
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["_deliveryPayload"] = deliveryPayload

	return outputData, nil
}

// createHL7DeliveryPayload creates delivery payload for HL7 over MLLP
func (e *PassthroughExecutor) createHL7DeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Get raw HL7 content from parsed data
	var hl7Content string
	if rawMsg, ok := inputData["raw_message"].(string); ok {
		hl7Content = rawMsg
	} else {
		// If no raw message, serialize parsed content back to HL7 (fallback)
		hl7Content = "MSH|..." // Would need HL7 serializer
	}

	// Create transmission payload for HL7 MLLP
	transmission := models.NewHL7TransmissionPayload(hl7Content)

	// Get destination from step config
	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "localhost:6661" // Default MLLP port
	}

	// Create delivery payload
	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"tcp_mllp",
		destinationEndpoint,
	)

	// Add format metadata
	payload.Format = "hl7v2"
	if version, ok := inputData["version"].(string); ok {
		payload.FormatVersion = version
	}
	if msgType, ok := inputData["messageType"].(string); ok {
		payload.MessageType = msgType
	}

	payload.FormatMetadata = map[string]interface{}{
		"hl7_version":        payload.FormatVersion,
		"message_type":       payload.MessageType,
		"sending_application": inputData["sendingApplication"],
		"sending_facility":   inputData["sendingFacility"],
	}

	// Add connectivity metadata for MLLP
	payload.ConnectivityMetadata = map[string]interface{}{
		"protocol":          "MLLP",
		"timeout_seconds":   30,
		"expect_ack":        true,
		"ack_timeout":       5,
		"retry_on_nack":     true,
	}

	payload.DestinationName = "HL7 MLLP Receiver"
	if name, ok := step.Config["destination_name"].(string); ok {
		payload.DestinationName = name
	}

	return payload
}

// createJSONDeliveryPayload creates delivery payload for JSON over HTTP
func (e *PassthroughExecutor) createJSONDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Serialize JSON content
	jsonBytes, err := json.Marshal(inputData)
	if err != nil {
		log.Printf("⚠️  Failed to marshal JSON: %v", err)
		jsonBytes = []byte("{}")
	}

	// Create transmission payload
	httpHeaders := map[string]string{
		"Content-Type": "application/json",
	}
	transmission := models.NewJSONTransmissionPayload(jsonBytes, httpHeaders)

	// Get destination
	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "http://localhost:8080/json"
	}

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "json"
	payload.ConnectivityMetadata = map[string]interface{}{
		"method":          "POST",
		"timeout_seconds": 30,
	}

	payload.DestinationName = "JSON HTTP Endpoint"

	return payload
}

// createXMLDeliveryPayload creates delivery payload for XML over HTTP
func (e *PassthroughExecutor) createXMLDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Get XML content (would need XML serializer)
	xmlContent := []byte("<?xml version=\"1.0\"?>")

	httpHeaders := map[string]string{
		"Content-Type": "application/xml",
	}

	transmission := &models.TransmissionPayload{
		Content:     xmlContent,
		ContentType: "application/xml",
		Encoding:    "utf-8",
		Headers:     httpHeaders,
	}

	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)
	if destinationEndpoint == "" {
		destinationEndpoint = "http://localhost:8080/xml"
	}

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "xml"
	payload.DestinationName = "XML HTTP Endpoint"

	return payload
}

// createGenericDeliveryPayload creates delivery payload for unknown formats
func (e *PassthroughExecutor) createGenericDeliveryPayload(
	messageID string,
	correlationID string,
	interfaceID string,
	step *models.TransformationStep,
	inputData map[string]interface{},
) *models.DeliveryPayload {

	// Serialize as JSON as fallback
	jsonBytes, _ := json.Marshal(inputData)

	transmission := &models.TransmissionPayload{
		Content:     jsonBytes,
		ContentType: "application/octet-stream",
		Encoding:    "utf-8",
		Headers:     make(map[string]string),
	}

	destinationEndpoint, _ := step.Config["destination_endpoint"].(string)

	payload := models.NewDeliveryPayload(
		messageID,
		correlationID,
		interfaceID,
		step.PipelineID,
		transmission,
		"http_rest",
		destinationEndpoint,
	)

	payload.Format = "unknown"
	payload.DestinationName = "Generic Endpoint"

	return payload
}
