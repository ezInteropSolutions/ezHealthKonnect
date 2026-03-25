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
)

// ezCompanionSystemPrompt is the shared system persona for all chat endpoints.
// Kept compact (~250 tokens) to minimise Ollama prefill latency on CPU.
const ezCompanionSystemPrompt = `You are ezCompanion — co-developer, healthcare integration expert, and guide for ezHealthKonnect (an HL7→FHIR integration platform).

ezHealthKonnect: Interfaces connect healthcare systems via 32+ connectors (TCP/MLLP, HTTP/REST, SFTP, S3, PostgreSQL, MySQL, MongoDB, Kafka, RabbitMQ, Redis). Transformation Pipelines (per interface + message type) chain steps: FHIR validation, field mapping, JavaScript script enrichment, conditional branching, data masking, format conversion, outbound delivery. The Wizard guides interface setup with HL7→FHIR mapping. Pipeline Builder is a drag-and-drop visual editor.

Healthcare standards: HL7 v2 (ADT, ORU, ORM, MDM, SIU — segments MSH/PID/PV1/OBR/OBX/DG1), FHIR R4 (Patient, Encounter, Observation, Condition, Procedure, Bundle, Claim), X12 835/837/270/271, CCD/C-CDA R2.1, LOINC, SNOMED CT, ICD-10, RxNorm, CPT, NPI.

Rules: Be direct and practical. For pipeline script steps use function named transform(input) returning the modified object. Never invent field codes. Say "I don't know" when uncertain.`

// AIService wires together all local AI capabilities.
type AIService struct {
	llm         LLMProvider      // chat/generation provider (may be queued)
	embedder    LLMProvider      // embedding provider (always Ollama for RAG consistency)
	ollama      *OllamaClient    // kept for backward-compat; same as llm when using Ollama
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
	DataType    string  `json:"data_type,omitempty"`
}

// SuggestMappings analyses a sample message and suggests field mappings.
func (s *AIService) SuggestMappings(ctx context.Context, sampleMessage, targetFormat string, reqCtx RequestContext) ([]MappingSuggestion, []RetrievedChunk, error) {
	det, _ := s.DetectFormat(ctx, sampleMessage)
	filters := dedup([]string{sourceTypeFromFormat(det.Format), sourceTypeFromFormat(targetFormat), "operational"})

	// Build context prefix from current interface/pipeline if provided
	ctxPrefix := ""
	if s.ContextBld != nil && !reqCtx.IsEmpty() {
		ctxPrefix = s.ContextBld.BuildSystemPromptPrefix(ctx, reqCtx)
	}

	question := fmt.Sprintf(
		"Given this %s sample message, suggest specific field mappings to %s format.\n\nSample:\n%s",
		det.Format, targetFormat, truncate(sampleMessage, 600))

	systemPrompt := ctxPrefix + fmt.Sprintf(
		`You are a healthcare integration mapping expert specialising in %s and %s.
Analyse the sample and suggest field mappings. Consider any confirmed mappings shown in the context.
Output ONLY a JSON array: [{"source_field":"","target_field":"","confidence":0.0,"reasoning":"","data_type":""}]
data_type: string | date | code | decimal | boolean`,
		det.Format, targetFormat)

	answer, chunks, err := s.rag.Query(ctx, question, systemPrompt, 8, filters)
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
	return suggestions, chunks, nil
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
	// Layer 3: conversation history
	history := input.HistoryOverride
	if len(history) == 0 && input.SessionID != "" && s.Memory != nil {
		history, _ = s.Memory.LoadHistory(ctx, input.SessionID, 10)
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
	// Also search operational KB so answers reference YOUR interfaces, not hypothetical ones
	answer, chunks, err := s.rag.Query(ctx, input.Question, systemPrompt, defaultTopK, nil)
	if err != nil {
		return "", chunks, err
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

	// Fast RAG: topK=3, no filter — gives operational KB hits (confirmed mappings,
	// interface configs) without full-scale retrieval latency.
	var chunks []RetrievedChunk
	if s.rag != nil && s.rag.db != nil {
		chunks, _ = s.rag.Retrieve(ctx, input.Question, 3, nil) // non-fatal
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
		log.Printf("🤖 AI KB — starting background knowledge ingestion from %s", schemaDir)

		schemaResults := s.Ingestion.IngestAll(ctx, schemaDir)
		opResults := s.Operational.IngestAll(ctx)

		total := 0
		for _, r := range append(schemaResults, opResults...) {
			total += r.ChunksStored
		}
		log.Printf("✅ AI KB — ingestion complete: %d total chunks", total)
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
