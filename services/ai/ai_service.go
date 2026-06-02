// services/ai/ai_service.go
// Top-level AI service: format detection, mapping suggestions, error explanation,
// and contextual Q&A — all local via Ollama, no PHI leaves the network.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"ezhealthkonnect/services/mapping"
)

// ezCompanionSystemPrompt is the shared system persona for all chat endpoints.
// Kept compact (~250 tokens) to minimise Ollama prefill latency on CPU.
const ezCompanionSystemPrompt = `You are ezCompanion — co-developer, healthcare integration expert, and guide for ezHealthKonnect (an HL7→FHIR integration platform).

ezHealthKonnect: Interfaces connect healthcare systems via 32+ connectors (TCP/MLLP, HTTP/REST, SFTP, S3, PostgreSQL, MySQL, MongoDB, Kafka, RabbitMQ, Redis). Transformation Pipelines (per interface + message type) chain steps: FHIR validation, field mapping, JavaScript script enrichment, conditional branching, data masking, format conversion, outbound delivery. The Wizard guides interface setup with HL7→FHIR mapping. Pipeline Builder is a drag-and-drop visual editor.

Healthcare standards: HL7 v2 (ADT, ORU, ORM, MDM, SIU — segments MSH/PID/PV1/OBR/OBX/DG1), FHIR R4 (Patient, Encounter, Observation, Condition, Procedure, Bundle, Claim), X12 835/837/270/271, CCD/C-CDA R2.1, LOINC, SNOMED CT, ICD-10, RxNorm, CPT, NPI. Da Vinci PAS STU2 (CMS-0057-F): prior authorization via FHIR $submit operation; payer responds with ClaimResponse; X12 review action codes A1=AA (approved), A2=CP (partial approval), A3/A4=AD (denied), A6=PE (pended/pending review).

Rules: Be direct and practical. For pipeline script steps use function named transform(input) returning the modified object. Never invent field codes. Say "I don't know" when uncertain.`

// AIService wires together all local AI capabilities.
type AIService struct {
	llm         LLMProvider      // chat/generation provider (may be queued)
	embedder    LLMProvider      // embedding provider (always Ollama for RAG consistency)
	ollama      *OllamaClient    // kept for backward-compat; same as llm when using Ollama
	db          *sql.DB          // kept for startup chunk-count check in BackgroundIngestAll
	rag         *RAGService
	embedding   *EmbeddingService
	Ingestion   *KnowledgeIngestionService   // static schema files (HL7, FHIR, X12, CCD)
	Operational *OperationalIngestionService // your interfaces, pipelines, error patterns
	ContextBld  *ContextBuilder             // fetches live DB data for prompt enrichment
	Memory      *ConversationMemory         // multi-turn conversation history
	ScriptGen   *ScriptGeneratorService     // JS script generation with step context
	Tracer      *MessageTracerService       // step execution trace + LLM failure analysis
	Feedback    *FeedbackService            // user feedback storage + KB improvement loop
}

// NewAIServiceWithProvider constructs an AIService with injected providers.
// chatProvider is used for all generation; embedProvider is used for RAG embeddings.
// Keeping them separate allows OpenAI/Anthropic chat + Ollama embeddings.
// db may be nil for test scenarios that don't need DB access.
func NewAIServiceWithProvider(db *sql.DB, chatProvider LLMProvider, embedProvider LLMProvider) *AIService {
	embedding := newEmbeddingServiceWithProvider(db, embedProvider)
	operational := NewOperationalIngestionService(db, embedding)
	return &AIService{
		llm:         chatProvider,
		embedder:    embedProvider,
		db:          db,
		rag:         newRAGServiceWithProvider(db, chatProvider, embedProvider),
		embedding:   embedding,
		Ingestion:   NewKnowledgeIngestionService(embedding),
		Operational: operational,
		ContextBld:  NewContextBuilder(db),
		Memory:      NewConversationMemory(db),
		ScriptGen:   newScriptGeneratorService(db),
		Tracer:      newMessageTracerService(db),
		Feedback:    newFeedbackService(db, operational),
	}
}

// NewAIService wires together all AI sub-services using Ollama from env vars.
// Kept for backward compatibility; new code should use NewAIServiceWithProvider.
func NewAIService(db *sql.DB) *AIService {
	ollama := NewOllamaClient()
	svc := NewAIServiceWithProvider(db, ollama, ollama)
	svc.ollama = ollama // preserve for OllamaClient() accessor
	return svc
}

// OllamaClient returns the underlying Ollama client (backward compat; may be nil for non-Ollama providers).
func (s *AIService) OllamaClient() *OllamaClient { return s.ollama }

// LLMProvider returns the active chat provider.
func (s *AIService) LLMProvider() LLMProvider { return s.llm }

// ─── Status ───────────────────────────────────────────────────────────────────

// StatusResult reports AI subsystem health.
type StatusResult struct {
	// OllamaReachable kept for backward compat — use ProviderReachable in new code
	OllamaReachable  bool           `json:"ollama_reachable"`
	ProviderReachable bool          `json:"provider_reachable"`
	ProviderEnabled  bool           `json:"provider_enabled"`
	Provider         string         `json:"provider"`
	Models           []string       `json:"models"`
	ChatModel        string         `json:"chat_model"`
	EmbedModel       string         `json:"embed_model"`
	QueueDepth       int            `json:"queue_depth"`
	QueueCap         int            `json:"queue_cap"`
	KnowledgeChunks  int            `json:"knowledge_chunks"`
	ChunksBySource   map[string]int `json:"chunks_by_source"`
	Error            string         `json:"error,omitempty"`
}

// Status returns the health and readiness of the AI subsystem.
func (s *AIService) Status(ctx context.Context) StatusResult {
	result := StatusResult{
		ProviderEnabled: s.llm != nil,
		Provider:        s.llm.ProviderName(),
		ChatModel:       s.llm.ChatModelName(),
	}

	// Embed model name if available
	if ep, ok := s.embedder.(EmbedProvider); ok {
		result.EmbedModel = ep.EmbedModelName()
	}

	// Queue stats if available
	if q, ok := s.llm.(*LLMQueue); ok {
		result.QueueDepth = q.Depth()
		result.QueueCap = q.Cap()
	}

	if err := s.llm.Ping(ctx); err != nil {
		result.Error = err.Error()
		return result
	}
	result.ProviderReachable = true
	result.OllamaReachable = result.Provider == "ollama" // backward compat

	// List models if provider supports it
	if ep, ok := s.llm.(EmbedProvider); ok {
		result.Models, _ = ep.ListModels(ctx)
	} else if q, ok := s.llm.(*LLMQueue); ok {
		if ep, ok := q.inner.(EmbedProvider); ok {
			result.Models, _ = ep.ListModels(ctx)
		}
	}

	result.KnowledgeChunks, _ = s.embedding.CountChunks(ctx)
	result.ChunksBySource, _ = s.embedding.CountBySourceType(ctx)
	return result
}

// ─── Format Detection ─────────────────────────────────────────────────────────

// FormatDetection describes an auto-detected message format.
type FormatDetection struct {
	Format            string  `json:"format"`
	Version           string  `json:"version,omitempty"`
	TransactionType   string  `json:"transaction_type,omitempty"`
	Confidence        float64 `json:"confidence"`
	Description       string  `json:"description"`
	SuggestedPipeline string  `json:"suggested_pipeline,omitempty"`
}

// DetectFormat identifies the healthcare message format from raw content.
// Uses fast heuristics first; falls back to LLM for ambiguous cases.
func (s *AIService) DetectFormat(ctx context.Context, rawMessage string) (FormatDetection, error) {
	if det, ok := heuristicDetect(rawMessage); ok {
		return det, nil
	}

	question := fmt.Sprintf("Identify the healthcare message format of this sample:\n\n%s", truncate(rawMessage, 800))
	systemPrompt := `You are a healthcare integration expert. Identify the message format.
Output ONLY valid JSON: {"format":"","version":"","transaction_type":"","confidence":0.0,"description":"","suggested_pipeline":""}
format must be one of: hl7_v2, fhir_r4, x12_835, x12_837, x12_270, x12_271, ccd, csv, json, unknown.`

	answer, _, err := s.rag.Query(ctx, question, systemPrompt, 3, nil)
	if err != nil {
		return FormatDetection{Format: "unknown", Confidence: 0}, err
	}

	var det FormatDetection
	if extracted := extractJSON(answer); extracted != "" {
		if jsonErr := json.Unmarshal([]byte(extracted), &det); jsonErr == nil {
			return det, nil
		}
	}
	return FormatDetection{Format: "unknown", Description: answer, Confidence: 0.3}, nil
}

func heuristicDetect(msg string) (FormatDetection, bool) {
	msg = strings.TrimSpace(msg)
	switch {
	case strings.HasPrefix(msg, "MSH|"):
		msgType := extractHL7MsgType(msg)
		return FormatDetection{Format: "hl7_v2", Version: extractHL7Version(msg),
			TransactionType: msgType, Confidence: 0.99,
			Description: fmt.Sprintf("HL7 v2 message, type %s", msgType)}, true

	case strings.HasPrefix(msg, "ISA*") || strings.HasPrefix(msg, "ISA~"):
		txType := extractX12TransactionType(msg)
		format := "x12"
		switch txType {
		case "835":
			format = "x12_835"
		case "837":
			format = "x12_837"
		case "270":
			format = "x12_270"
		case "271":
			format = "x12_271"
		}
		return FormatDetection{Format: format, Version: "005010", TransactionType: txType,
			Confidence: 0.98, Description: fmt.Sprintf("X12 %s transaction", txType)}, true

	case strings.Contains(msg, `"resourceType"`):
		return FormatDetection{Format: "fhir_r4", Version: "R4", Confidence: 0.97,
			Description: "FHIR R4 JSON resource"}, true

	case strings.Contains(msg, "ClinicalDocument") || strings.Contains(msg, "urn:hl7-org:v3"):
		return FormatDetection{Format: "ccd", Confidence: 0.96,
			Description: "CCD/CDA clinical document"}, true
	}
	return FormatDetection{}, false
}

func extractHL7MsgType(msg string) string {
	if f := strings.Split(msg, "|"); len(f) > 8 {
		return f[8]
	}
	return ""
}
func extractHL7Version(msg string) string {
	if f := strings.Split(msg, "|"); len(f) > 11 {
		return f[11]
	}
	return ""
}
func extractX12TransactionType(msg string) string {
	idx := strings.Index(msg, "ST*")
	if idx == -1 {
		return ""
	}
	sub := msg[idx+3:]
	if end := strings.IndexAny(sub, "*~\r\n"); end != -1 {
		return sub[:end]
	}
	return sub
}

// ─── Mapping Suggestions ──────────────────────────────────────────────────────

// MappingSuggestion is a single field-to-field mapping recommendation.
type MappingSuggestion struct {
	SourceField string  `json:"source_field"`
	TargetField string  `json:"target_field"`
	Confidence  float64 `json:"confidence"`
	Reasoning   string  `json:"reasoning"`
	DataType     string `json:"data_type,omitempty"`
	// TransformKey is the exact key the Go transform engine uses in its switch
	// statement (e.g. "ts_to_date", "gender_mapping", "cx_to_identifier").
	// The frontend uses this when adding the mapping to atomicMappings so the
	// engine actually executes the right transformation.
	TransformKey string `json:"transform_key,omitempty"`
}

// SuggestMappings analyses a sample message and suggests field mappings.
// For HL7v2→FHIR it uses a built-in static mapping table for instant results;
// the LLM path is used only when no static mappings are available.
func (s *AIService) SuggestMappings(ctx context.Context, sampleMessage, targetFormat string, reqCtx RequestContext) ([]MappingSuggestion, []RetrievedChunk, error) {
	det, _ := s.DetectFormat(ctx, sampleMessage)

	// Fast path: message-centric analysis for HL7→FHIR.
	// staticHL7FHIRMappings derives suggestions from the fields actually present
	// in the message (not a fixed catalogue list). zSegmentSuggestions adds
	// Extension proposals for any custom Z-segment fields found.
	if det.Format == "hl7_v2" || det.Format == "hl7v2" {
		if targetFormat == "fhir_r4" || targetFormat == "fhir" {
			standard := staticHL7FHIRMappings(sampleMessage, det.Format, targetFormat)
			custom := zSegmentSuggestions(sampleMessage)
			combined := append(standard, custom...)
			if len(combined) > 0 {
				return qualifySuggestions(combined), nil, nil
			}
		}
	}

	// Slow path: ask the LLM (only for non-HL7 or unknown formats).
	filters := dedup([]string{sourceTypeFromFormat(det.Format), sourceTypeFromFormat(targetFormat), "operational"})

	ctxPrefix := ""
	if s.ContextBld != nil && !reqCtx.IsEmpty() {
		ctxPrefix = s.ContextBld.BuildSystemPromptPrefix(ctx, reqCtx)
	}

	question := fmt.Sprintf("%s→%s mappings. Sample:\n%s", det.Format, targetFormat, truncate(sampleMessage, 300))
	systemPrompt := ctxPrefix + fmt.Sprintf(
		`Healthcare integration expert. Map %s fields to %s.
Output ONLY a compact JSON array, no prose: [{"source_field":"","target_field":"","confidence":0.9,"reasoning":"","data_type":"string"}]
Return 3-5 mappings only. data_type: string|date|code|decimal|boolean.`,
		det.Format, targetFormat)

	answer, chunks, err := s.rag.QueryCapped(ctx, question, systemPrompt, 3, 250, filters)
	if err != nil {
		return nil, chunks, err
	}

	extracted := extractJSON(answer)
	if extracted == "" {
		return nil, chunks, fmt.Errorf("could not extract JSON from LLM response")
	}

	var suggestions []MappingSuggestion
	if err := json.Unmarshal([]byte(extracted), &suggestions); err != nil {
		var single MappingSuggestion
		if err2 := json.Unmarshal([]byte(extracted), &single); err2 == nil {
			suggestions = []MappingSuggestion{single}
		} else {
			return nil, chunks, fmt.Errorf("parse mapping suggestions: %w", err)
		}
	}
	return qualifySuggestions(suggestions), chunks, nil
}

// segmentResourceHints maps standard HL7 segment names to their primary FHIR
// resource for use when qualifying bare FHIR paths (e.g. "identifier" → "Patient.identifier").
var segmentResourceHints = map[string]string{
	"MSH": "MessageHeader", "PID": "Patient", "PD1": "Patient",
	"PV1": "Encounter", "PV2": "Encounter", "EVN": "Encounter",
	"OBR": "DiagnosticReport", "OBX": "Observation",
	"ORC": "ServiceRequest", "AL1": "AllergyIntolerance",
	"DG1": "Condition", "NK1": "RelatedPerson",
	"IN1": "Coverage", "IN2": "Coverage",
	"ROL": "PractitionerRole", "GT1": "RelatedPerson",
	"PR1": "Procedure", "FT1": "ChargeItem",
	"TXA": "DocumentReference", "SCH": "Appointment",
	"AIS": "Appointment", "AIG": "Appointment", "AIL": "Appointment", "AIP": "Appointment",
	"RXA": "Immunization", "RXR": "Immunization",
	"STF": "Practitioner", "PRA": "PractitionerRole",
	"LOC": "Location", "MFI": "MessageHeader", "MFE": "MessageHeader",
	"ACC": "Condition", "AUT": "ClaimResponse",
}

// fhirResourceQualifiedRe matches paths that already start with a FHIR resource
// name (e.g. "Patient.name", "Encounter.period.start").
var fhirResourceQualifiedRe = regexp.MustCompile(`^[A-Z][a-zA-Z]+\.`)

// qualifyFHIRPath ensures a FHIR path starts with a resource name.
// If the path is already qualified ("Patient.identifier") it is returned unchanged.
// Otherwise the resource is derived from the source HL7 field segment.
func qualifyFHIRPath(targetField, sourceField string) string {
	if fhirResourceQualifiedRe.MatchString(targetField) {
		return targetField
	}
	// Derive segment name: "PID.3" → "PID"
	seg := strings.ToUpper(strings.SplitN(strings.TrimSpace(sourceField), ".", 2)[0])
	var resource string
	if len(seg) > 0 && seg[0] == 'Z' {
		resource = inferZSegmentResource(seg)
	} else if r, ok := segmentResourceHints[seg]; ok {
		resource = r
	}
	if resource == "" {
		return targetField // cannot infer — return unchanged rather than guess wrongly
	}
	return resource + "." + targetField
}

// qualifySuggestions runs qualifyFHIRPath on every suggestion in the slice.
func qualifySuggestions(suggestions []MappingSuggestion) []MappingSuggestion {
	for i := range suggestions {
		suggestions[i].TargetField = qualifyFHIRPath(suggestions[i].TargetField, suggestions[i].SourceField)
	}
	return suggestions
}

// staticHL7FHIRMappings returns HL7v2→FHIR R4 field mapping suggestions for
// the segments actually present in sampleMessage.
//
// Primary path: schema-driven generator (uses loaded HL7 + FHIR StructureDefinitions).
// Fallback path: built-in static catalogue (used when schema loaders are not
// initialised — e.g. during unit tests or cold start before schemas are loaded).
//
// Returns nil when the format/target combination is not HL7v2→FHIR so the
// caller falls back to the LLM.
func staticHL7FHIRMappings(sampleMessage, detectedFormat, targetFormat string) []MappingSuggestion {
	if detectedFormat != "hl7_v2" && detectedFormat != "hl7v2" {
		return nil
	}
	if targetFormat != "fhir_r4" && targetFormat != "fhir" {
		return nil
	}

	// ── Primary: schema-driven generator ────────────────────────────────
	gen := mapping.NewGenerator()
	hl7Version := extractHL7Version(sampleMessage)
	if hl7Version == "" {
		hl7Version = "2.5"
	}
	generatedMaps, _, err := gen.GenerateForPresentSegments(sampleMessage, hl7Version, mapping.FHIRVersionR4)
	if err == nil && len(generatedMaps) > 0 {
		suggestions := make([]MappingSuggestion, 0, len(generatedMaps))
		for _, gm := range generatedMaps {
			suggestions = append(suggestions, MappingSuggestion{
				SourceField:  gm.HL7Path,
				TargetField:  gm.FHIRPath,
				Confidence:   gm.Confidence,
				Reasoning:    gm.Notes,
				DataType:     gm.HL7DataType,
				TransformKey: gm.Transform,
			})
		}
		return suggestions
	}

	// ── Fallback: static catalogue (schemas not loaded) ──────────────────
	presentSegments := extractPresentSegments(sampleMessage)
	if len(presentSegments) == 0 {
		return nil
	}

	catalogue := map[string][]MappingSuggestion{
		// ── MSH — Message Header (present in every HL7v2 message) ──────────────
		"MSH": {
			{SourceField: "MSH.3", TargetField: "MessageHeader.source.name", Confidence: 0.95,
				Reasoning: "MSH.3 Sending Application (HD) → MessageHeader.source.name.", DataType: "HD", TransformKey: "string_direct"},
			{SourceField: "MSH.5", TargetField: "MessageHeader.destination[0].name", Confidence: 0.90,
				Reasoning: "MSH.5 Receiving Application (HD) → MessageHeader.destination[0].name.", DataType: "HD", TransformKey: "string_direct"},
			{SourceField: "MSH.9", TargetField: "MessageHeader.eventCoding.code", Confidence: 0.99,
				Reasoning: "MSH.9 Message Type (MSG) — event code e.g. A01, R01, M02.", DataType: "MSG", TransformKey: "string_direct"},
			{SourceField: "MSH.10", TargetField: "MessageHeader.id", Confidence: 0.99,
				Reasoning: "MSH.10 Message Control ID → MessageHeader.id.", DataType: "ST", TransformKey: "string_direct"},
			{SourceField: "MSH.7", TargetField: "MessageHeader.meta.lastUpdated", Confidence: 0.92,
				Reasoning: "MSH.7 Date/Time of Message (TS). ts_to_instant converts to ISO-8601 instant.", DataType: "TS", TransformKey: "ts_to_instant"},
		},

		// ── PID — Patient Identification ──────────────────────────────────────
		"PID": {
			{SourceField: "PID.3", TargetField: "Patient.identifier", Confidence: 0.97,
				Reasoning: "PID.3 Patient Identifier List (CX). cx_to_identifier extracts value/type/system per repetition.", DataType: "CX", TransformKey: "cx_to_identifier"},
			{SourceField: "PID.5", TargetField: "Patient.name[0]", Confidence: 0.97,
				Reasoning: "PID.5 Patient Name (XPN). xpn_to_humanname parses Family^Given^Middle components.", DataType: "XPN", TransformKey: "xpn_to_humanname"},
			{SourceField: "PID.7", TargetField: "Patient.birthDate", Confidence: 0.96,
				Reasoning: "PID.7 Date of Birth (TS). ts_to_date converts YYYYMMDD → YYYY-MM-DD.", DataType: "TS", TransformKey: "ts_to_date"},
			{SourceField: "PID.8", TargetField: "Patient.gender", Confidence: 0.94,
				Reasoning: "PID.8 Administrative Sex (IS). gender_mapping: M→male, F→female, U→unknown, O→other.", DataType: "IS", TransformKey: "gender_mapping"},
			{SourceField: "PID.11", TargetField: "Patient.address[0]", Confidence: 0.93,
				Reasoning: "PID.11 Patient Address (XAD). xad_to_address maps street/city/state/postal.", DataType: "XAD", TransformKey: "xad_to_address"},
			{SourceField: "PID.13", TargetField: "Patient.telecom[0]", Confidence: 0.91,
				Reasoning: "PID.13 Home Phone (XTN). xtn_to_contactpoint: system=phone, use=home.", DataType: "XTN", TransformKey: "xtn_to_contactpoint"},
			{SourceField: "PID.18", TargetField: "Patient.identifier[1].value", Confidence: 0.88,
				Reasoning: "PID.18 Patient Account Number → additional FHIR identifier.", DataType: "CX", TransformKey: "account_to_identifier"},
		},

		// ── PV1 — Patient Visit ────────────────────────────────────────────────
		"PV1": {
			{SourceField: "PV1.2", TargetField: "Encounter.class.code", Confidence: 0.92,
				Reasoning: "PV1.2 Patient Class (I/O/E/P). patient_class_to_coding maps to FHIR ActCode.", DataType: "IS", TransformKey: "patient_class_to_coding"},
			{SourceField: "PV1.3", TargetField: "Encounter.location[0].location.display", Confidence: 0.88,
				Reasoning: "PV1.3 Assigned Patient Location (PL) — room/bed.", DataType: "PL", TransformKey: "string_direct"},
			{SourceField: "PV1.7", TargetField: "Encounter.participant[0].individual.display", Confidence: 0.85,
				Reasoning: "PV1.7 Attending Doctor (XCN) — display name.", DataType: "XCN", TransformKey: "string_direct"},
			{SourceField: "PV1.44", TargetField: "Encounter.period.start", Confidence: 0.91,
				Reasoning: "PV1.44 Admit Date/Time (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
			{SourceField: "PV1.45", TargetField: "Encounter.period.end", Confidence: 0.91,
				Reasoning: "PV1.45 Discharge Date/Time (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── EVN — Event Type ───────────────────────────────────────────────────
		"EVN": {
			{SourceField: "EVN.2", TargetField: "Encounter.period.start", Confidence: 0.88,
				Reasoning: "EVN.2 Recorded Date/Time (TS) → Encounter.period.start.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── OBR — Observation Request ──────────────────────────────────────────
		"OBR": {
			{SourceField: "OBR.3", TargetField: "DiagnosticReport.identifier[0].value", Confidence: 0.93,
				Reasoning: "OBR.3 Filler Order Number → DiagnosticReport.identifier.", DataType: "EI", TransformKey: "string_direct"},
			{SourceField: "OBR.4", TargetField: "DiagnosticReport.code", Confidence: 0.95,
				Reasoning: "OBR.4 Universal Service Identifier (CE). ce_to_codeableconcept maps code^display^system.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "OBR.7", TargetField: "DiagnosticReport.effectiveDateTime", Confidence: 0.91,
				Reasoning: "OBR.7 Observation Date/Time (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
			{SourceField: "OBR.25", TargetField: "DiagnosticReport.status", Confidence: 0.89,
				Reasoning: "OBR.25 Result Status (F=final, P=preliminary, C=corrected). obr_status_to_dr_status.", DataType: "ID", TransformKey: "obr_status_to_dr_status"},
		},

		// ── OBX — Observation/Result ──────────────────────────────────────────
		"OBX": {
			{SourceField: "OBX.3", TargetField: "Observation.code", Confidence: 0.95,
				Reasoning: "OBX.3 Observation Identifier (CWE). ce_to_codeableconcept → Observation.code.", DataType: "CWE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "OBX.5", TargetField: "Observation.value[x]", Confidence: 0.88,
				Reasoning: "OBX.5 Observation Value. Type from OBX.2 (NM→valueQuantity, ST→valueString, CE→valueCodeableConcept).", DataType: "varies", TransformKey: "obx_value_by_type"},
			{SourceField: "OBX.6", TargetField: "Observation.valueQuantity.unit", Confidence: 0.88,
				Reasoning: "OBX.6 Units (CE) → valueQuantity.unit.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "OBX.8", TargetField: "Observation.interpretation[0].coding[0].code", Confidence: 0.87,
				Reasoning: "OBX.8 Abnormal Flags (N/L/H/A). abnormal_flag_to_interpretation.", DataType: "IS", TransformKey: "abnormal_flag_to_interpretation"},
			{SourceField: "OBX.11", TargetField: "Observation.status", Confidence: 0.92,
				Reasoning: "OBX.11 Result Status (F/P/C). obx_status_to_obs_status.", DataType: "ID", TransformKey: "obx_status_to_obs_status"},
			{SourceField: "OBX.14", TargetField: "Observation.effectiveDateTime", Confidence: 0.90,
				Reasoning: "OBX.14 Date/Time of Observation (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── ORC — Common Order ────────────────────────────────────────────────
		"ORC": {
			{SourceField: "ORC.1", TargetField: "ServiceRequest.status", Confidence: 0.90,
				Reasoning: "ORC.1 Order Control (NW/CA/DC). orc_status_to_sr_status.", DataType: "ID", TransformKey: "orc_status_to_sr_status"},
			{SourceField: "ORC.3", TargetField: "ServiceRequest.identifier[0].value", Confidence: 0.92,
				Reasoning: "ORC.3 Filler Order Number → ServiceRequest identifier.", DataType: "EI", TransformKey: "string_direct"},
			{SourceField: "ORC.9", TargetField: "ServiceRequest.authoredOn", Confidence: 0.91,
				Reasoning: "ORC.9 Date/Time of Transaction (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── AL1 — Allergy Information ─────────────────────────────────────────
		"AL1": {
			{SourceField: "AL1.2", TargetField: "AllergyIntolerance.category[0]", Confidence: 0.90,
				Reasoning: "AL1.2 Allergen Type Code (DA=medication, FA=food, EA=environment). allergy_category_mapping.", DataType: "IS", TransformKey: "allergy_category_mapping"},
			{SourceField: "AL1.3", TargetField: "AllergyIntolerance.code", Confidence: 0.95,
				Reasoning: "AL1.3 Allergen Code/Description (CE). ce_to_codeableconcept.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "AL1.4", TargetField: "AllergyIntolerance.reaction[0].severity", Confidence: 0.88,
				Reasoning: "AL1.4 Allergy Severity (SV/MO/MI). allergy_severity_mapping.", DataType: "IS", TransformKey: "allergy_severity_mapping"},
		},

		// ── DG1 — Diagnosis ───────────────────────────────────────────────────
		"DG1": {
			{SourceField: "DG1.3", TargetField: "Condition.code", Confidence: 0.95,
				Reasoning: "DG1.3 Diagnosis Code/Description (CE). ce_to_codeableconcept → Condition.code.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "DG1.5", TargetField: "Condition.onsetDateTime", Confidence: 0.88,
				Reasoning: "DG1.5 Diagnosis Date/Time (TS). ts_to_datetime → ISO-8601.", DataType: "TS", TransformKey: "ts_to_datetime"},
			{SourceField: "DG1.6", TargetField: "Condition.category[0].coding[0].code", Confidence: 0.85,
				Reasoning: "DG1.6 Diagnosis Type (A=admitting, F=final, W=working). diagnosis_type_mapping.", DataType: "IS", TransformKey: "diagnosis_type_mapping"},
		},

		// ── NK1 — Next of Kin ─────────────────────────────────────────────────
		"NK1": {
			{SourceField: "NK1.2", TargetField: "RelatedPerson.name[0]", Confidence: 0.93,
				Reasoning: "NK1.2 Next of Kin Name (XPN). xpn_to_humanname.", DataType: "XPN", TransformKey: "xpn_to_humanname"},
			{SourceField: "NK1.3", TargetField: "RelatedPerson.relationship[0].coding[0].code", Confidence: 0.90,
				Reasoning: "NK1.3 Relationship (CE) — e.g. MTH=mother, SPO=spouse.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "NK1.5", TargetField: "RelatedPerson.telecom[0]", Confidence: 0.85,
				Reasoning: "NK1.5 Phone Number (XTN). xtn_to_contactpoint.", DataType: "XTN", TransformKey: "xtn_to_contactpoint"},
		},

		// ── IN1 — Insurance ───────────────────────────────────────────────────
		"IN1": {
			{SourceField: "IN1.2", TargetField: "Coverage.type.coding[0].code", Confidence: 0.88,
				Reasoning: "IN1.2 Insurance Plan ID (CE) → Coverage.type.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "IN1.3", TargetField: "Coverage.payor[0].identifier[0].value", Confidence: 0.90,
				Reasoning: "IN1.3 Insurance Company ID (CX) → Coverage.payor identifier.", DataType: "CX", TransformKey: "string_direct"},
			{SourceField: "IN1.4", TargetField: "Coverage.payor[0].display", Confidence: 0.88,
				Reasoning: "IN1.4 Insurance Company Name → Coverage.payor display.", DataType: "ST", TransformKey: "string_direct"},
			{SourceField: "IN1.36", TargetField: "Coverage.identifier[0].value", Confidence: 0.85,
				Reasoning: "IN1.36 Policy Number → Coverage.identifier.", DataType: "ST", TransformKey: "string_direct"},
		},

		// ── MFI — Master File Identification ──────────────────────────────────
		"MFI": {
			{SourceField: "MFI.1", TargetField: "MessageHeader.meta.tag[0].code", Confidence: 0.85,
				Reasoning: "MFI.1 Master File Identifier (CE) — identifies the type of master file being updated (STF=staff, LOC=location, CDM=charge).", DataType: "CE", TransformKey: "string_direct"},
			{SourceField: "MFI.3", TargetField: "MessageHeader.meta.tag[1].code", Confidence: 0.80,
				Reasoning: "MFI.3 File-Level Event Code (UPD=update, REP=replace, NE=not present).", DataType: "ID", TransformKey: "string_direct"},
		},

		// ── MFE — Master File Entry ───────────────────────────────────────────
		"MFE": {
			{SourceField: "MFE.1", TargetField: "meta.tag[0].code", Confidence: 0.88,
				Reasoning: "MFE.1 Record-Level Event Code (MAD=add, MUP=update, MDL=delete, MDC=deactivate) — applied to each produced resource as meta.tag.", DataType: "ID", TransformKey: "string_direct"},
			{SourceField: "MFE.2", TargetField: "meta.versionId", Confidence: 0.82,
				Reasoning: "MFE.2 MFN Control ID → meta.versionId for change tracking.", DataType: "ST", TransformKey: "string_direct"},
		},

		// ── STF — Staff Identification (MFN^M02) ──────────────────────────────
		"STF": {
			{SourceField: "STF.1", TargetField: "Practitioner.identifier[0].value", Confidence: 0.95,
				Reasoning: "STF.1 Primary Key Value (CE) — staff primary identifier.", DataType: "CE", TransformKey: "string_direct"},
			{SourceField: "STF.3", TargetField: "Practitioner.name[0]", Confidence: 0.97,
				Reasoning: "STF.3 Staff Name (XPN). xpn_to_humanname parses Family^Given^Middle.", DataType: "XPN", TransformKey: "xpn_to_humanname"},
			{SourceField: "STF.4", TargetField: "Practitioner.qualification[0].code.text", Confidence: 0.85,
				Reasoning: "STF.4 Staff Type (IS) — e.g. Nursing, Physician, Admin.", DataType: "IS", TransformKey: "string_direct"},
			{SourceField: "STF.5", TargetField: "Practitioner.gender", Confidence: 0.92,
				Reasoning: "STF.5 Administrative Sex (IS). gender_mapping: M→male, F→female.", DataType: "IS", TransformKey: "gender_mapping"},
			{SourceField: "STF.6", TargetField: "Practitioner.birthDate", Confidence: 0.90,
				Reasoning: "STF.6 Date of Birth (TS). ts_to_date → YYYY-MM-DD.", DataType: "TS", TransformKey: "ts_to_date"},
			{SourceField: "STF.7", TargetField: "Practitioner.active", Confidence: 0.92,
				Reasoning: "STF.7 Active/Inactive Flag (A=active, I=inactive). boolean_yn_mapping.", DataType: "ID", TransformKey: "boolean_yn_mapping"},
			{SourceField: "STF.10", TargetField: "Practitioner.telecom[0]", Confidence: 0.88,
				Reasoning: "STF.10 Phone (XTN). xtn_to_contactpoint: system=phone.", DataType: "XTN", TransformKey: "xtn_to_contactpoint"},
		},

		// ── PRA — Practitioner Detail (MFN^M02, complements STF) ─────────────
		"PRA": {
			{SourceField: "PRA.3", TargetField: "PractitionerRole.specialty[0].coding[0].code", Confidence: 0.90,
				Reasoning: "PRA.3 Practitioner Category (IS) — specialty code.", DataType: "IS", TransformKey: "string_direct"},
			{SourceField: "PRA.5", TargetField: "Practitioner.qualification[0].identifier[0].value", Confidence: 0.88,
				Reasoning: "PRA.5 Specialty (SPD) — board certification or specialty credentials.", DataType: "SPD", TransformKey: "string_direct"},
		},

		// ── LOC — Patient Location (MFN^M05) ─────────────────────────────────
		"LOC": {
			{SourceField: "LOC.1", TargetField: "Location.identifier[0].value", Confidence: 0.95,
				Reasoning: "LOC.1 Primary Key Value (PL) — location identifier.", DataType: "PL", TransformKey: "string_direct"},
			{SourceField: "LOC.2", TargetField: "Location.name", Confidence: 0.95,
				Reasoning: "LOC.2 Location Description (ST) → Location.name.", DataType: "ST", TransformKey: "string_direct"},
			{SourceField: "LOC.3", TargetField: "Location.type[0].coding[0].code", Confidence: 0.85,
				Reasoning: "LOC.3 Location Type (IS) — N=nursing unit, R=room, B=bed, etc.", DataType: "IS", TransformKey: "string_direct"},
			{SourceField: "LOC.4", TargetField: "Location.address", Confidence: 0.85,
				Reasoning: "LOC.4 Location Address (XAD). xad_to_address.", DataType: "XAD", TransformKey: "xad_to_address"},
		},

		// ── CDM — Charge Description Master (MFN^M04) ────────────────────────
		"CDM": {
			{SourceField: "CDM.1", TargetField: "ChargeItemDefinition.identifier[0].value", Confidence: 0.95,
				Reasoning: "CDM.1 Primary Key Value (CWE) — charge code.", DataType: "CWE", TransformKey: "string_direct"},
			{SourceField: "CDM.3", TargetField: "ChargeItemDefinition.title", Confidence: 0.92,
				Reasoning: "CDM.3 Charge Description Short (ST) → title.", DataType: "ST", TransformKey: "string_direct"},
			{SourceField: "CDM.4", TargetField: "ChargeItemDefinition.description", Confidence: 0.88,
				Reasoning: "CDM.4 Charge Description Long (ST) → description.", DataType: "ST", TransformKey: "string_direct"},
		},

		// ── OM1 — General Observation Properties (MFN^M12) ───────────────────
		"OM1": {
			{SourceField: "OM1.2", TargetField: "ObservationDefinition.identifier[0].value", Confidence: 0.95,
				Reasoning: "OM1.2 Producer's Service/Test/Observation ID (CWE) — test code.", DataType: "CWE", TransformKey: "string_direct"},
			{SourceField: "OM1.7", TargetField: "ObservationDefinition.code", Confidence: 0.93,
				Reasoning: "OM1.7 Observation ID (CWE). ce_to_codeableconcept → ObservationDefinition.code.", DataType: "CWE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "OM1.11", TargetField: "ObservationDefinition.preferredReportName", Confidence: 0.88,
				Reasoning: "OM1.11 Preferred Report Name (ST) → preferredReportName.", DataType: "ST", TransformKey: "string_direct"},
		},

		// ── SCH — Schedule Activity Information (SIU) ─────────────────────────
		"SCH": {
			{SourceField: "SCH.1", TargetField: "Appointment.identifier[0].value", Confidence: 0.93,
				Reasoning: "SCH.1 Placer Appointment ID (EI) → Appointment.identifier.", DataType: "EI", TransformKey: "string_direct"},
			{SourceField: "SCH.7", TargetField: "Appointment.appointmentType.coding[0].code", Confidence: 0.88,
				Reasoning: "SCH.7 Appointment Type Reason (CE) → Appointment.appointmentType.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "SCH.9", TargetField: "Appointment.description", Confidence: 0.85,
				Reasoning: "SCH.9 Appointment Duration (NM) — used for scheduling context.", DataType: "NM", TransformKey: "string_direct"},
			{SourceField: "SCH.11", TargetField: "Appointment.start", Confidence: 0.93,
				Reasoning: "SCH.11 Appointment Timing Quantity (TQ) — start time. ts_to_datetime.", DataType: "TQ", TransformKey: "ts_to_datetime"},
		},

		// ── AIS — Appointment Information (SIU) ──────────────────────────────
		"AIS": {
			{SourceField: "AIS.3", TargetField: "Appointment.participant[0].actor.display", Confidence: 0.88,
				Reasoning: "AIS.3 Universal Service Identifier (CE) — the booked service/resource.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "AIS.4", TargetField: "Appointment.participant[0].period.start", Confidence: 0.85,
				Reasoning: "AIS.4 Appointment Start Date/Time (TS). ts_to_datetime.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── TXA — Transcription Document Header (MDM) ────────────────────────
		"TXA": {
			{SourceField: "TXA.2", TargetField: "DocumentReference.type.coding[0].code", Confidence: 0.92,
				Reasoning: "TXA.2 Document Type (IS) — e.g. DS=discharge summary, PN=progress note.", DataType: "IS", TransformKey: "string_direct"},
			{SourceField: "TXA.4", TargetField: "DocumentReference.date", Confidence: 0.90,
				Reasoning: "TXA.4 Activity Date/Time (TS). ts_to_datetime → DocumentReference.date.", DataType: "TS", TransformKey: "ts_to_datetime"},
			{SourceField: "TXA.5", TargetField: "DocumentReference.author[0].display", Confidence: 0.88,
				Reasoning: "TXA.5 Primary Activity Provider (XCN) — document author.", DataType: "XCN", TransformKey: "string_direct"},
			{SourceField: "TXA.9", TargetField: "DocumentReference.identifier[0].value", Confidence: 0.90,
				Reasoning: "TXA.9 Originator Code/Name (XCN) — document originator ID.", DataType: "XCN", TransformKey: "string_direct"},
			{SourceField: "TXA.17", TargetField: "DocumentReference.status", Confidence: 0.88,
				Reasoning: "TXA.17 Document Completion Status (AU/LA/OB/PA/UN/DI). document_status_mapping.", DataType: "ID", TransformKey: "document_status_mapping"},
		},

		// ── RXA — Pharmacy/Treatment Administration (VXU) ────────────────────
		"RXA": {
			{SourceField: "RXA.3", TargetField: "Immunization.occurrenceDateTime", Confidence: 0.95,
				Reasoning: "RXA.3 Date/Time Start of Administration (TS). ts_to_datetime.", DataType: "TS", TransformKey: "ts_to_datetime"},
			{SourceField: "RXA.5", TargetField: "Immunization.vaccineCode", Confidence: 0.95,
				Reasoning: "RXA.5 Administered Code (CE) — CVX vaccine code. ce_to_codeableconcept.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "RXA.6", TargetField: "Immunization.doseQuantity.value", Confidence: 0.88,
				Reasoning: "RXA.6 Administered Amount (NM) → Immunization.doseQuantity.value.", DataType: "NM", TransformKey: "string_direct"},
			{SourceField: "RXA.9", TargetField: "Immunization.reportOrigin.coding[0].code", Confidence: 0.85,
				Reasoning: "RXA.9 Administration Notes (CE) — administered by/at. ce_to_codeableconcept.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "RXA.20", TargetField: "Immunization.status", Confidence: 0.90,
				Reasoning: "RXA.20 Completion Status (CP=complete, RE=refused, NA=not admin). immunization_status_mapping.", DataType: "ID", TransformKey: "immunization_status_mapping"},
		},

		// ── GT1 — Guarantor (financial guarantor demographics) ────────────────
		"GT1": {
			{SourceField: "GT1.3", TargetField: "RelatedPerson.name[0]", Confidence: 0.92,
				Reasoning: "GT1.3 Guarantor Name (XPN). xpn_to_humanname.", DataType: "XPN", TransformKey: "xpn_to_humanname"},
			{SourceField: "GT1.5", TargetField: "RelatedPerson.address[0]", Confidence: 0.88,
				Reasoning: "GT1.5 Guarantor Address (XAD). xad_to_address.", DataType: "XAD", TransformKey: "xad_to_address"},
			{SourceField: "GT1.8", TargetField: "RelatedPerson.birthDate", Confidence: 0.88,
				Reasoning: "GT1.8 Guarantor Date of Birth (TS). ts_to_date.", DataType: "TS", TransformKey: "ts_to_date"},
			{SourceField: "GT1.9", TargetField: "RelatedPerson.gender", Confidence: 0.88,
				Reasoning: "GT1.9 Guarantor Administrative Sex (IS). gender_mapping.", DataType: "IS", TransformKey: "gender_mapping"},
		},

		// ── ROL — Role ────────────────────────────────────────────────────────
		"ROL": {
			{SourceField: "ROL.3", TargetField: "PractitionerRole.code[0].coding[0].code", Confidence: 0.88,
				Reasoning: "ROL.3 Role-ROL (CE) — role code e.g. AD=admitting, AT=attending, CP=consulting.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "ROL.4", TargetField: "PractitionerRole.practitioner.display", Confidence: 0.90,
				Reasoning: "ROL.4 Role Person (XCN) — the practitioner in this role.", DataType: "XCN", TransformKey: "string_direct"},
		},

		// ── PR1 — Procedures ──────────────────────────────────────────────────
		"PR1": {
			{SourceField: "PR1.3", TargetField: "Procedure.code", Confidence: 0.93,
				Reasoning: "PR1.3 Procedure Code (CE). ce_to_codeableconcept → Procedure.code.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "PR1.5", TargetField: "Procedure.performedDateTime", Confidence: 0.90,
				Reasoning: "PR1.5 Procedure Date/Time (TS). ts_to_datetime.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},

		// ── FT1 — Financial Transaction ───────────────────────────────────────
		"FT1": {
			{SourceField: "FT1.6", TargetField: "ChargeItem.code", Confidence: 0.90,
				Reasoning: "FT1.6 Transaction Code (CE). ce_to_codeableconcept → ChargeItem.code.", DataType: "CE", TransformKey: "ce_to_codeableconcept"},
			{SourceField: "FT1.4", TargetField: "ChargeItem.occurrenceDateTime", Confidence: 0.88,
				Reasoning: "FT1.4 Transaction Date (TS). ts_to_datetime.", DataType: "TS", TransformKey: "ts_to_datetime"},
		},
	}

	// Build a field-level lookup so we can check field-by-field below.
	catalogueLookup := map[string]MappingSuggestion{}
	for _, segSuggestions := range catalogue {
		for _, s := range segSuggestions {
			catalogueLookup[s.SourceField] = s
		}
	}

	// Walk the actual message lines and emit a catalogue suggestion only when
	// the corresponding field has a non-empty value in the submitted HL7.
	// This makes the fallback message-centric: the suggestions are derived from
	// what is actually in the HL7, not from a fixed catalogue list.
	var result []MappingSuggestion
	seen := map[string]bool{}
	normalised := strings.ReplaceAll(strings.ReplaceAll(sampleMessage, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(normalised, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		pipeIdx := strings.Index(line, "|")
		if pipeIdx <= 0 {
			continue
		}
		segName := strings.ToUpper(strings.TrimSpace(line[:pipeIdx]))
		if len(segName) < 2 || segName[0] == 'Z' {
			continue // Z-segments are handled by zSegmentSuggestions in SuggestMappings
		}
		for i, val := range strings.Split(line[pipeIdx+1:], "|") {
			if strings.TrimSpace(val) == "" {
				continue
			}
			fieldPath := fmt.Sprintf("%s.%d", segName, i+1)
			if seen[fieldPath] {
				continue
			}
			seen[fieldPath] = true
			if s, found := catalogueLookup[fieldPath]; found {
				result = append(result, s)
			}
		}
	}
	return result
}

// zSegmentSuggestions inspects the raw HL7 message for custom Z-segments
// (names starting with "Z") and returns heuristic FHIR Extension suggestions
// for each non-empty field. These are not in any standard catalogue and would
// otherwise be invisible to the AI Suggest gap analysis.
func zSegmentSuggestions(message string) []MappingSuggestion {
	var suggestions []MappingSuggestion
	normalised := strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(normalised, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		pipeIdx := strings.Index(line, "|")
		if pipeIdx <= 0 {
			continue
		}
		segName := strings.ToUpper(strings.TrimSpace(line[:pipeIdx]))
		if len(segName) < 2 || segName[0] != 'Z' {
			continue
		}

		resource := inferZSegmentResource(segName)
		fields := strings.Split(line[pipeIdx+1:], "|")
		for i, val := range fields {
			val = strings.TrimSpace(val)
			if val == "" {
				continue
			}
			fieldNum := i + 1
			extType, dataType, transformKey := inferZFieldExtension(val)
			// e.g. "Extension.valueCode" → "Patient.extension.valueCode"
			fhirPath := resource + "." + strings.TrimPrefix(extType, "Extension.")
			suggestions = append(suggestions, MappingSuggestion{
				SourceField:  fmt.Sprintf("%s.%d", segName, fieldNum),
				TargetField:  fhirPath,
				Confidence:   0.65,
				Reasoning:    fmt.Sprintf("Custom Z-segment %s.%d → %s FHIR Extension (value: %q). Attach to %s based on segment name prefix.", segName, fieldNum, extType, truncate(val, 25), resource),
				DataType:     dataType,
				TransformKey: transformKey,
			})
		}
	}
	return suggestions
}

// inferZSegmentResource maps a Z-segment name to the most likely FHIR resource
// that should own the Extension, based on common naming conventions.
func inferZSegmentResource(seg string) string {
	seg = strings.ToUpper(seg)
	switch {
	case strings.HasPrefix(seg, "ZP") || strings.HasPrefix(seg, "ZD"):
		// ZPD, ZPI, ZPA, ZPR, ZDM → patient demographics / identifiers
		return "Patient"
	case strings.HasPrefix(seg, "ZV") || strings.HasPrefix(seg, "ZE") || strings.HasPrefix(seg, "ZA"):
		// ZVI, ZVS, ZEN, ZAD → visit / encounter / admission
		return "Encounter"
	case strings.HasPrefix(seg, "ZO") || strings.HasPrefix(seg, "ZR"):
		// ZOB, ZOR, ZRS → observation / result
		return "Observation"
	case strings.HasPrefix(seg, "ZI") || strings.HasPrefix(seg, "ZC"):
		// ZIN, ZCO → insurance / coverage / condition
		return "Coverage"
	case strings.HasPrefix(seg, "ZM") || strings.HasPrefix(seg, "ZH"):
		// ZMH, ZHD → message header level
		return "MessageHeader"
	default:
		// Unknown prefix — Patient is the safest default for most clinical Z-segments
		return "Patient"
	}
}

// inferZFieldExtension picks the most appropriate FHIR Extension value type
// based on the field's value pattern.
func inferZFieldExtension(val string) (fhirPath, dataType, transformKey string) {
	// 14-digit timestamp: YYYYMMDDHHmmss
	if matched, _ := regexp.MatchString(`^\d{14}$`, val); matched {
		return "Extension.valueDateTime", "TS", "ts_to_datetime"
	}
	// 8-digit date: YYYYMMDD
	if matched, _ := regexp.MatchString(`^\d{8}$`, val); matched {
		return "Extension.valueDate", "DT", "ts_to_date"
	}
	// Short uppercase alpha code (1-5 chars): IS/ID
	if matched, _ := regexp.MatchString(`^[A-Z]{1,5}$`, val); matched {
		return "Extension.valueCode", "IS", "string_direct"
	}
	// Numeric (integer or decimal)
	if matched, _ := regexp.MatchString(`^-?\d+(\.\d+)?$`, val); matched {
		return "Extension.valueDecimal", "NM", "string_direct"
	}
	// Boolean-like
	if val == "Y" || val == "N" || val == "true" || val == "false" {
		return "Extension.valueBoolean", "ID", "string_direct"
	}
	// Default: string
	return "Extension.valueString", "ST", "string_direct"
}

// extractPresentSegments returns a set of the unique HL7 segment names
// (3-character codes) found in the message. Works regardless of whether
// lines are delimited by \r, \r\n, or \n.
func extractPresentSegments(msg string) map[string]bool {
	present := make(map[string]bool)
	// normalise all line endings to \n
	normalised := strings.ReplaceAll(msg, "\r\n", "\n")
	normalised = strings.ReplaceAll(normalised, "\r", "\n")
	for _, line := range strings.Split(normalised, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		// segment name is the first token before | (or the first 3 chars if no |)
		seg := line
		if idx := strings.Index(line, "|"); idx > 0 {
			seg = line[:idx]
		}
		seg = strings.TrimSpace(seg)
		if len(seg) >= 2 && len(seg) <= 4 {
			present[seg] = true
		}
	}
	return present
}


// ─── Error Explanation ────────────────────────────────────────────────────────

// ErrorExplanation is a plain-English breakdown of a processing error.
type ErrorExplanation struct {
	Summary     string   `json:"summary"`
	RootCause   string   `json:"root_cause"`
	Suggestions []string `json:"suggestions"`
	Severity    string   `json:"severity"` // critical | warning | info
}

// ExplainError translates a technical error into plain English with fix suggestions.
func (s *AIService) ExplainError(ctx context.Context, errorMsg, messageContext string, reqCtx RequestContext) (ErrorExplanation, []RetrievedChunk, error) {
	ctxPrefix := ""
	if s.ContextBld != nil && !reqCtx.IsEmpty() {
		ctxPrefix = s.ContextBld.BuildSystemPromptPrefix(ctx, reqCtx)
	}

	question := fmt.Sprintf(
		"Explain this healthcare integration error and suggest fixes:\n\nError: %s\n\nMessage context: %s",
		truncate(errorMsg, 400), truncate(messageContext, 400))

	systemPrompt := ctxPrefix + `You are a healthcare integration support expert.
Explain the error in plain English, identify the root cause, and list actionable fix steps.
Output ONLY valid JSON: {"summary":"","root_cause":"","suggestions":[],"severity":"critical|warning|info"}`

	answer, chunks, err := s.rag.Query(ctx, question, systemPrompt, 6, []string{"operational"})
	if err != nil {
		return ErrorExplanation{}, chunks, err
	}

	extracted := extractJSON(answer)
	if extracted == "" {
		return ErrorExplanation{Summary: answer}, chunks, nil
	}
	var explanation ErrorExplanation
	if jsonErr := json.Unmarshal([]byte(extracted), &explanation); jsonErr != nil {
		return ErrorExplanation{Summary: answer}, chunks, nil
	}
	return explanation, chunks, nil
}

// ─── Contextual Q&A ───────────────────────────────────────────────────────────

// AskInput bundles everything needed for a contextual AI question.
type AskInput struct {
	Question        string            // required
	SessionID       string            // for conversation memory (optional)
	UserID          string            // for saving turns (optional)
	RequestContext  RequestContext    // current UI context (optional)
	HistoryOverride []ConversationTurn // explicit history; if set, skips DB lookup
}

// AskQuestion answers a question using three context layers:
//  1. Retrieved KB chunks (RAG)
//  2. Live system context (interface/pipeline/step data)
//  3. Conversation history (session memory)
func (s *AIService) AskQuestion(ctx context.Context, input AskInput) (string, []RetrievedChunk, error) {
	// Guardrail: return pre-written authoritative answer for high-stakes question classes.
	// Bypasses the LLM entirely to guarantee correctness where small models hallucinate.
	if guardrailAnswer, matched := guardrailCheck(input.Question); matched {
		return guardrailAnswer, nil, nil
	}

	// Layer 3: conversation history
	history := input.HistoryOverride
	if len(history) == 0 && input.SessionID != "" && s.Memory != nil {
		history, _ = s.Memory.LoadHistory(ctx, input.SessionID, 2)
	}

	// Layer 2: live system context
	ctxPrefix := ""
	if s.ContextBld != nil && !input.RequestContext.IsEmpty() {
		ctxPrefix = s.ContextBld.BuildSystemPromptPrefix(ctx, input.RequestContext)
	}

	// Format history for injection
	historyText := ""
	if s.Memory != nil && len(history) > 0 {
		historyText = s.Memory.FormatHistory(history)
	}

	systemPrompt := ctxPrefix + historyText + ezCompanionSystemPrompt

	// Layer 1: RAG retrieval + LLM generation
	// topK=4 and maxTokens=400 keep total round-trip within 600s on CPU-only hardware.
	// On CPU, llama3.2:3b evaluates ~6 input tokens/sec and generates ~1 token/sec;
	// 400-token output cap + reduced history/RAG keeps total time to ~450s worst-case.
	answer, chunks, err := s.rag.QueryCapped(ctx, input.Question, systemPrompt, 4, 400, nil)
	if err != nil {
		return "", chunks, err
	}

	// Confidence signal: when RAG found no relevant chunks, flag the answer as ungrounded.
	if len(chunks) == 0 {
		answer = lowConfidencePrefix + answer
	}

	// Persist turns
	if input.SessionID != "" && s.Memory != nil {
		meta := map[string]interface{}{"chunks": len(chunks)}
		if !input.RequestContext.IsEmpty() {
			meta["context"] = input.RequestContext
		}
		_ = s.Memory.SaveTurn(ctx, input.SessionID, input.UserID, "user", input.Question, nil)
		_ = s.Memory.SaveTurn(ctx, input.SessionID, input.UserID, "assistant", answer, meta)
	}

	return answer, chunks, nil
}

// AskQuestionStream is the streaming variant of AskQuestion.
// It does a fast RAG retrieval (topK=3) before streaming so confirmed mappings
// and operational context appear in the answer without 20-40s full-embed latency.
func (s *AIService) AskQuestionStream(ctx context.Context, input AskInput, onToken func(string) error) ([]RetrievedChunk, error) {
	history := input.HistoryOverride
	if len(history) == 0 && input.SessionID != "" && s.Memory != nil {
		history, _ = s.Memory.LoadHistory(ctx, input.SessionID, 10)
	}

	ctxPrefix := ""
	if s.ContextBld != nil && !input.RequestContext.IsEmpty() {
		ctxPrefix = s.ContextBld.BuildSystemPromptPrefix(ctx, input.RequestContext)
	}

	historyText := ""
	if s.Memory != nil && len(history) > 0 {
		historyText = s.Memory.FormatHistory(history)
	}

	// Guardrail: stream pre-written answer for high-stakes question classes.
	if guardrailAnswer, matched := guardrailCheck(input.Question); matched {
		_ = onToken(guardrailAnswer)
		return nil, nil
	}

	// Fast RAG: topK=3, no filter — gives operational KB hits (confirmed mappings,
	// interface configs) without full-scale retrieval latency.
	var chunks []RetrievedChunk
	if s.rag != nil && s.rag.db != nil {
		chunks, _ = s.rag.Retrieve(ctx, input.Question, 3, nil) // non-fatal
	}

	// Confidence signal: stream a caveat first when no KB context was found.
	if len(chunks) == 0 {
		_ = onToken(lowConfidencePrefix)
	}

	systemPrompt := ctxPrefix + historyText + ezCompanionSystemPrompt
	prompt := buildRAGPrompt(systemPrompt, input.Question, chunks)
	err := s.llm.GenerateStream(ctx, prompt, onToken)

	// Persist turns after streaming completes
	if err == nil && input.SessionID != "" && s.Memory != nil {
		_ = s.Memory.SaveTurn(ctx, input.SessionID, input.UserID, "user", input.Question, nil)
	}

	return chunks, err
}

// ─── Script Generation ────────────────────────────────────────────────────────

// GenerateScriptStream generates a JavaScript transform(input) function and
// streams tokens as they arrive. The step + pipeline context is injected so
// the LLM knows what data is available at that pipeline position.
func (s *AIService) GenerateScriptStream(ctx context.Context, input ScriptGenInput, onToken func(string) error) error {
	return s.ScriptGen.GenerateScriptStream(ctx, input, s.llm, onToken)
}

// ─── Message Trace Analysis ───────────────────────────────────────────────────

// TraceMessage fetches the step execution trace for a message and returns
// the raw trace. Call TraceMessageStream for LLM analysis.
func (s *AIService) TraceMessage(ctx context.Context, messageID, interfaceID string) (*MessageTraceResult, error) {
	return s.Tracer.Trace(ctx, messageID, interfaceID)
}

// TraceMessageStream fetches the trace then streams an LLM explanation into onToken.
func (s *AIService) TraceMessageStream(ctx context.Context, messageID, interfaceID string, onToken func(string) error) (*MessageTraceResult, error) {
	trace, err := s.Tracer.Trace(ctx, messageID, interfaceID)
	if err != nil {
		return nil, err
	}

	return s.TraceMessageStreamWithTrace(ctx, trace, onToken)
}

// TraceMessageStreamWithTrace streams LLM analysis for a pre-fetched trace.
// Used in tests to bypass DB lookup.
func (s *AIService) TraceMessageStreamWithTrace(ctx context.Context, trace *MessageTraceResult, onToken func(string) error) (*MessageTraceResult, error) {
	traceText := BuildTracePrompt(trace)
	prompt := buildRAGPrompt(messageTraceSystemPrompt, traceText, nil)
	if streamErr := s.llm.GenerateStream(ctx, prompt, onToken); streamErr != nil {
		return trace, streamErr
	}
	return trace, nil
}

// ─── Step Explanation ────────────────────────────────────────────────────────

// ExplainStepStream reads a step's config from DB and streams a human-readable
// explanation of what it does, what inputs it expects, and what it outputs.
func (s *AIService) ExplainStepStream(ctx context.Context, stepID string, onToken func(string) error) error {
	if s.ContextBld == nil {
		return fmt.Errorf("context builder not available")
	}
	stepCtx := s.ContextBld.fetchStepContext(ctx, stepID)
	if stepCtx == "" {
		return fmt.Errorf("step %s not found", stepID)
	}

	sysPrompt := `You are ezCompanion — a healthcare integration expert for ezHealthKonnect.
Explain this pipeline step in plain English. Cover:
1. What it does (one sentence summary)
2. What input data it reads/expects
3. What it outputs or modifies
4. Any important side effects or error conditions
Be concise and developer-friendly. Use bullet points.`

	prompt := buildRAGPrompt(sysPrompt, "Explain this pipeline step:\n\n"+stepCtx, nil)
	return s.llm.GenerateStream(ctx, prompt, onToken)
}

// ConversationTurn is a single message in a conversation.
type ConversationTurn struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// ─── Background tasks ─────────────────────────────────────────────────────────

// BackgroundIngestAll ingests static schemas + operational data asynchronously.
func (s *AIService) BackgroundIngestAll(schemaDir string) {
	go func() {
		ctx := context.Background()

		// Skip expensive static schema ingest (HL7, FHIR, X12, CCD) when the KB
		// already has app_docs and schema chunks from a previous run. Static schemas
		// don't change between restarts — only re-ingest them via POST /api/ai/ingest.
		// Operational data (interfaces, pipelines) is always refreshed because it changes.
		skipStatic := false
		if s.db != nil {
			var n int
			if err := s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM ai_knowledge_chunks WHERE source_type IN ('app_docs','ig_anchors','ig_assemblyrules','ig_valuesets','x12','ccd')`).
				Scan(&n); err == nil && n > 0 {
				skipStatic = true
				log.Printf("🤖 AI KB — %d static chunks already present, skipping schema re-ingest (restart optimisation)", n)
			}
		}

		var schemaResults []IngestionResult
		if !skipStatic {
			log.Printf("🤖 AI KB — starting full knowledge ingestion from %s", schemaDir)
			schemaResults = s.Ingestion.IngestAll(ctx, schemaDir)
		}
		opResults := s.Operational.IngestAll(ctx)

		total := 0
		for _, r := range append(schemaResults, opResults...) {
			total += r.ChunksStored
		}
		log.Printf("✅ AI KB — ingestion complete: %d chunks stored", total)
	}()
}

// BackgroundIngestInterface re-ingests a single interface after it is created/updated.
func (s *AIService) BackgroundIngestInterface(interfaceID string) {
	go func() {
		ctx := context.Background()
		if err := s.Operational.IngestInterface(ctx, interfaceID); err != nil {
			log.Printf("⚠️  AI KB — interface %s re-ingestion failed: %v", interfaceID, err)
		} else {
			log.Printf("✅ AI KB — interface %s re-ingested", interfaceID)
		}
	}()
}

// BackgroundIngestPipeline re-ingests a single pipeline after it is saved.
func (s *AIService) BackgroundIngestPipeline(pipelineID string) {
	go func() {
		ctx := context.Background()
		if err := s.Operational.IngestPipeline(ctx, pipelineID); err != nil {
			log.Printf("⚠️  AI KB — pipeline %s re-ingestion failed: %v", pipelineID, err)
		} else {
			log.Printf("✅ AI KB — pipeline %s re-ingested", pipelineID)
		}
	}()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\[.*?\\]|\\{.*?\\})\\s*```")
var jsonInlineRe = regexp.MustCompile(`(?s)(\[[\s\S]*?\]|\{[\s\S]*?\})`)

func extractJSON(s string) string {
	if m := jsonBlockRe.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	if m := jsonInlineRe.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func sourceTypeFromFormat(format string) string {
	switch {
	case strings.HasPrefix(format, "hl7"):
		return "hl7_v2"
	case strings.HasPrefix(format, "fhir"):
		return "fhir_r4"
	case strings.HasPrefix(format, "x12"):
		return "x12"
	case format == "ccd":
		return "ccd"
	default:
		return ""
	}
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
