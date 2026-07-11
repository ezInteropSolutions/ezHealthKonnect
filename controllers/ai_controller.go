// controllers/ai_controller.go
// REST API for local AI capabilities.
// All inference runs on-premise via Ollama — no PHI leaves the network.
package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/ai"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// AIController exposes all local AI capabilities over REST.
type AIController struct {
	svc       *ai.AIService
	schemaDir string
	telemetry *services.TelemetryService
}

// NewAIController creates a new AIController.
func NewAIController(svc *ai.AIService, schemaDir string) *AIController {
	return &AIController{svc: svc, schemaDir: schemaDir}
}

// SetTelemetry wires in the telemetry service after construction.
func (c *AIController) SetTelemetry(t *services.TelemetryService) {
	c.telemetry = t
}

// requireAIEnabled returns a middleware that gates all AI endpoints.
// /status is intentionally exempt so the frontend can always query state.
func (c *AIController) requireAIEnabled() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !services.GetAppSettings().GetAIConfig().Enabled {
			ctx.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   "AI assistant is disabled. Enable it in System Settings → AI.",
				"code":    "ai_disabled",
			})
			return
		}
		ctx.Next()
	}
}

// requireAdminRole returns a middleware that restricts an endpoint to admin and super_admin users.
// The role is forwarded by the Node.js proxy in X-User-Role; this is defense-in-depth —
// the proxy layer already enforces the same constraint before the request reaches Go.
func (c *AIController) requireAdminRole() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		role := ctx.GetHeader("X-User-Role")
		if role != "admin" && role != "super_admin" {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin role required",
			})
			return
		}
		ctx.Next()
	}
}

// RegisterRoutes registers all AI endpoints under the given router group.
func (c *AIController) RegisterRoutes(group *gin.RouterGroup) {
	// Status — always reachable so the frontend can detect disabled state
	group.GET("/status", c.GetStatus)

	// All other routes are gated by the AI enabled check
	gated := group.Use(c.requireAIEnabled())

	// Core AI features
	gated.POST("/detect-format", c.DetectFormat)
	gated.POST("/suggest-mappings", c.SuggestMappings)
	gated.POST("/suggest-field-mappings", c.SuggestFieldMappings)
	gated.POST("/explain-error", c.ExplainError)
	gated.POST("/ask", c.Ask)
	gated.POST("/ask/stream", c.AskStream)

	// Agent Mode — propose/approve/reject tool-calling loop
	gated.POST("/agent/ask", c.AgentAsk)
	gated.POST("/agent-actions/:id/approve", c.requireAdminRole(), c.ApproveAgentAction)
	gated.POST("/agent-actions/:id/reject", c.requireAdminRole(), c.RejectAgentAction)

	// Inline developer tools (all stream)
	gated.POST("/generate-script/stream", c.GenerateScriptStream)
	gated.POST("/trace-message", c.TraceMessage)
	gated.POST("/trace-message/stream", c.TraceMessageStream)
	gated.POST("/explain-step/stream", c.ExplainStepStream)

	// Conversation history
	gated.GET("/conversations", c.GetConversationHistory)

	// Model catalog + status — read-only, any authenticated user
	gated.GET("/models/catalog", c.GetModelCatalog)
	gated.GET("/models/local", c.GetLocalModels)

	// Feedback — user-facing writes
	gated.POST("/feedback/mapping", c.FeedbackMapping)
	gated.POST("/feedback/error-resolved", c.FeedbackErrorResolved)
	gated.POST("/feedback/response", c.FeedbackResponse)
	gated.GET("/feedback/summary", c.FeedbackSummary)

	// Admin-only: knowledge base ingest, model pull, feedback export/submit
	admin := group.Use(c.requireAIEnabled(), c.requireAdminRole())
	admin.POST("/ingest", c.IngestAll)
	admin.POST("/ingest/interface/:id", c.IngestInterface)
	admin.POST("/ingest/pipeline/:id", c.IngestPipeline)
	admin.POST("/ingest/docs", c.IngestDocs)
	admin.POST("/models/pull", c.PullModelStream)
	admin.GET("/feedback/export", c.FeedbackExport)
	admin.POST("/feedback/submit-to-team", c.FeedbackSubmitToTeam)
}

// ─── GET /api/ai/status ───────────────────────────────────────────────────────

func (c *AIController) GetStatus(ctx *gin.Context) {
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	status := c.svc.Status(reqCtx)
	code := http.StatusOK
	if !status.ProviderReachable {
		code = http.StatusServiceUnavailable
	}
	ctx.JSON(code, gin.H{"success": status.ProviderReachable, "data": status})
}

// ─── POST /api/ai/detect-format ───────────────────────────────────────────────

type detectFormatRequest struct {
	Message string `json:"message" binding:"required"`
}

func (c *AIController) DetectFormat(ctx *gin.Context) {
	var req detectFormatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()

	det, err := c.svc.DetectFormat(reqCtx, req.Message)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": det})
}

// ─── POST /api/ai/suggest-mappings ────────────────────────────────────────────

type suggestMappingsRequest struct {
	Message      string            `json:"message" binding:"required"`
	TargetFormat string            `json:"target_format" binding:"required"`
	Context      ai.RequestContext `json:"context"`
}

func (c *AIController) SuggestMappings(ctx *gin.Context) {
	var req suggestMappingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 180*time.Second)
	defer cancel()

	suggestions, chunks, err := c.svc.SuggestMappings(reqCtx, req.Message, req.TargetFormat, req.Context)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"suggestions":    suggestions,
			"count":          len(suggestions),
			"context_chunks": len(chunks),
			"sources":        chunkSources(chunks),
		},
	})
}

// ─── POST /api/ai/suggest-field-mappings ──────────────────────────────────────
// Design-time only: proposes sourcePath -> targetField mappings for the
// fhir.build/hl7.build/cda.map_to_canonical step builders, given sample
// source rows and the step's OWN live target-field catalog (the frontend
// fetches this from /api/fhir/canonical-fields, /api/hl7/canonical-fields, or
// /api/cda/canonical-fields and passes it straight through — no new catalog
// logic lives here). Distinct from suggest-mappings above, whose contract is
// "paste one whole raw message" — see ai.MappingSuggesterService's own doc
// comment for why that endpoint isn't reused for this.

type suggestFieldMappingsRequest struct {
	StepType     string                   `json:"step_type"`
	SampleRows   []map[string]interface{} `json:"sample_rows" binding:"required"`
	TargetFields []ai.TargetFieldInfo     `json:"target_fields" binding:"required"`
}

func (c *AIController) SuggestFieldMappings(ctx *gin.Context) {
	var req suggestFieldMappingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 180*time.Second)
	defer cancel()

	suggestions, err := c.svc.MappingSuggester.SuggestFieldMappings(reqCtx, c.svc.LLMProvider(), ai.FieldMappingSuggestInput{
		StepType:     req.StepType,
		SampleRows:   req.SampleRows,
		TargetFields: req.TargetFields,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"suggestions": suggestions, "count": len(suggestions)},
	})
}

// ─── POST /api/ai/explain-error ───────────────────────────────────────────────

type explainErrorRequest struct {
	Error          string            `json:"error" binding:"required"`
	MessageContext string            `json:"message_context"`
	Context        ai.RequestContext `json:"context"` // which interface/step was running
}

func (c *AIController) ExplainError(ctx *gin.Context) {
	var req explainErrorRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 90*time.Second)
	defer cancel()

	explanation, _, err := c.svc.ExplainError(reqCtx, req.Error, req.MessageContext, req.Context)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": explanation})
}

// ─── POST /api/ai/ask ─────────────────────────────────────────────────────────

type askRequest struct {
	Question  string                `json:"question" binding:"required"`
	SessionID string                `json:"session_id"`  // enables conversation memory
	Context   ai.RequestContext     `json:"context"`     // current UI state
	History   []ai.ConversationTurn `json:"history"`     // explicit history (overrides DB lookup)
}

func (c *AIController) Ask(ctx *gin.Context) {
	var req askRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 600*time.Second)
	defer cancel()

	answer, chunks, err := c.svc.AskQuestion(reqCtx, ai.AskInput{
		Question:        req.Question,
		SessionID:       req.SessionID,
		UserID:          ctx.GetHeader("X-User-ID"),
		RequestContext:  req.Context,
		HistoryOverride: req.History,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"answer":         answer,
			"session_id":     req.SessionID,
			"context_chunks": len(chunks),
			"sources":        chunkSources(chunks),
		},
	})
}

// ─── POST /api/ai/ask/stream ──────────────────────────────────────────────────

// AskStream streams the AI answer as Server-Sent Events (SSE).
// Each token arrives as:  data: {"t":"..."}
// End of stream:          data: [DONE]
func (c *AIController) AskStream(ctx *gin.Context) {
	var req askRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	sseHeaders(ctx)

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 600*time.Second)
	defer cancel()

	writeSSE := sseWriter(ctx)
	_, err := c.svc.AskQuestionStream(reqCtx, ai.AskInput{
		Question:        req.Question,
		SessionID:       req.SessionID,
		UserID:          ctx.GetHeader("X-User-ID"),
		RequestContext:  req.Context,
		HistoryOverride: req.History,
	}, func(token string) error {
		b, _ := json.Marshal(token)
		return writeSSE(string(b))
	})
	if err != nil {
		writeSSE("[ERROR] " + err.Error())
	}
	writeSSE("[DONE]")
}

// ─── POST /api/ai/agent/ask ───────────────────────────────────────────────────

type agentAskRequest struct {
	Question  string `json:"question" binding:"required"`
	SessionID string `json:"session_id" binding:"required"`
}

// AgentAsk asks ezCompanion in Agent Mode: the model may answer directly, or
// propose exactly one tool call that the frontend must render for approval.
// Nothing is executed by this endpoint — see ApproveAgentAction.
func (c *AIController) AgentAsk(ctx *gin.Context) {
	var req agentAskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if c.svc.Agent == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Agent Mode is not available — the configured AI provider does not support tool calling.",
			"code":    "agent_unavailable",
		})
		return
	}

	// 600s to match /ask's own budget — verify_pipeline_script's Prepare can run
	// up to maxScriptVerifyAttempts full LLM generate+dry-run round trips before
	// a proposal is even recorded, on top of the bounded read-only tool loop.
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 600*time.Second)
	defer cancel()

	turn, err := c.svc.Agent.ProposeAction(reqCtx, req.SessionID, ctx.GetHeader("X-User-ID"), req.Question)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": turn})
}

// ─── POST /api/ai/agent-actions/:id/approve ───────────────────────────────────

type approveAgentActionRequest struct {
	// GitRepoID is optional — when set and the approved tool's EntityType is
	// "pipeline_step", the owning interface is auto-committed to this repo.
	GitRepoID string `json:"git_repo_id"`
}

// ApproveAgentAction executes a previously proposed tool call. Admin-only.
func (c *AIController) ApproveAgentAction(ctx *gin.Context) {
	if c.svc.Agent == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Agent Mode is not available"})
		return
	}
	actionID := ctx.Param("id")

	var req approveAgentActionRequest
	_ = ctx.ShouldBindJSON(&req) // body is optional — git_repo_id may be omitted

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 120*time.Second)
	defer cancel()

	result, err := c.svc.Agent.ApproveAction(reqCtx, actionID, ctx.GetHeader("X-User-ID"), req.GitRepoID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": result.Status == "executed", "data": result})
}

// ─── POST /api/ai/agent-actions/:id/reject ────────────────────────────────────

// RejectAgentAction marks a proposed tool call as rejected. Admin-only.
func (c *AIController) RejectAgentAction(ctx *gin.Context) {
	if c.svc.Agent == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Agent Mode is not available"})
		return
	}
	actionID := ctx.Param("id")

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	if err := c.svc.Agent.RejectAction(reqCtx, actionID, ctx.GetHeader("X-User-ID")); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── POST /api/ai/ingest ──────────────────────────────────────────────────────

// IngestAll rebuilds the full knowledge base: static schemas + operational data.
func (c *AIController) IngestAll(ctx *gin.Context) {
	schemaDir := c.resolveSchemaDir()

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Minute)
	defer cancel()

	schemaResults := c.svc.Ingestion.IngestAll(reqCtx, schemaDir)
	opResults := c.svc.Operational.IngestAll(reqCtx)
	allResults := append(schemaResults, opResults...)

	totalFiles, totalChunks, totalErrors := 0, 0, 0
	for _, r := range allResults {
		totalFiles += r.FilesScanned
		totalChunks += r.ChunksStored
		totalErrors += len(r.Errors)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"success": totalErrors == 0,
		"data": gin.H{
			"results":      allResults,
			"total_files":  totalFiles,
			"total_chunks": totalChunks,
			"total_errors": totalErrors,
		},
	})
}

// ─── POST /api/ai/ingest/interface/:id ───────────────────────────────────────

// IngestInterface re-embeds a single interface's configuration.
// Called automatically after interface create/update.
func (c *AIController) IngestInterface(ctx *gin.Context) {
	interfaceID := ctx.Param("id")
	if interfaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interface id required"})
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Minute)
	defer cancel()

	if err := c.svc.Operational.IngestInterface(reqCtx, interfaceID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"interface_id": interfaceID}})
}

// ─── POST /api/ai/ingest/pipeline/:id ────────────────────────────────────────

// IngestPipeline re-embeds a single pipeline after it is saved.
func (c *AIController) IngestPipeline(ctx *gin.Context) {
	pipelineID := ctx.Param("id")
	if pipelineID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "pipeline id required"})
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Minute)
	defer cancel()

	if err := c.svc.Operational.IngestPipeline(reqCtx, pipelineID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pipeline_id": pipelineID}})
}

// ─── POST /api/ai/feedback/mapping ───────────────────────────────────────────

type mappingFeedbackRequest struct {
	InterfaceID string `json:"interface_id" binding:"required"`
	SourceField string `json:"source_field" binding:"required"`
	TargetField string `json:"target_field" binding:"required"`
	Accepted    bool   `json:"accepted"`
	Reasoning   string `json:"reasoning"`
}

// FeedbackMapping records whether a mapping suggestion was accepted or rejected.
// Accepted mappings get embedded as high-confidence knowledge for future suggestions.
func (c *AIController) FeedbackMapping(ctx *gin.Context) {
	var req mappingFeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()

	var err error
	if req.Accepted {
		err = c.svc.Operational.IngestConfirmedMapping(reqCtx, req.InterfaceID, req.SourceField, req.TargetField, req.Reasoning)
	} else {
		err = c.svc.Operational.IngestRejectedMapping(reqCtx, req.InterfaceID, req.SourceField, req.TargetField, req.Reasoning)
	}

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── POST /api/ai/feedback/error-resolved ────────────────────────────────────

type errorResolvedRequest struct {
	InterfaceID  string `json:"interface_id" binding:"required"`
	ErrorMessage string `json:"error_message" binding:"required"`
	Solution     string `json:"solution" binding:"required"`
}

// FeedbackErrorResolved records a known error+solution pair.
// Future occurrences of the same error will get the solution as a suggestion.
func (c *AIController) FeedbackErrorResolved(ctx *gin.Context) {
	var req errorResolvedRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()

	if err := c.svc.Operational.IngestResolvedError(reqCtx, req.InterfaceID, req.ErrorMessage, req.Solution); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── POST /api/ai/generate-script/stream ──────────────────────────────────────

type generateScriptRequest struct {
	Description   string `json:"description" binding:"required"` // plain-English goal
	StepID        string `json:"step_id"`
	PipelineID    string `json:"pipeline_id"`
	InterfaceID   string `json:"interface_id"`
	MessageType   string `json:"message_type"`
	StepType      string `json:"step_type"`
	CurrentScript string `json:"current_script"` // existing script (for edit mode)
}

// GenerateScriptStream generates a JS transform(input) function and streams tokens.
func (c *AIController) GenerateScriptStream(ctx *gin.Context) {
	var req generateScriptRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	sseHeaders(ctx)

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 300*time.Second)
	defer cancel()

	writeSSE := sseWriter(ctx)
	err := c.svc.GenerateScriptStream(reqCtx, ai.ScriptGenInput{
		Description:   req.Description,
		StepID:        req.StepID,
		PipelineID:    req.PipelineID,
		InterfaceID:   req.InterfaceID,
		MessageType:   req.MessageType,
		StepType:      req.StepType,
		CurrentScript: req.CurrentScript,
	}, func(token string) error {
		b, _ := json.Marshal(token)
		return writeSSE(string(b))
	})
	if err != nil {
		writeSSE("[ERROR] " + err.Error())
	}
	writeSSE("[DONE]")
}

// ─── POST /api/ai/trace-message ───────────────────────────────────────────────

type traceMessageRequest struct {
	MessageID   string `json:"message_id" binding:"required"`
	InterfaceID string `json:"interface_id"`
}

// TraceMessage returns the raw step execution trace as JSON (no LLM analysis).
func (c *AIController) TraceMessage(ctx *gin.Context) {
	var req traceMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()

	trace, err := c.svc.TraceMessage(reqCtx, req.MessageID, req.InterfaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": trace})
}

// ─── POST /api/ai/trace-message/stream ───────────────────────────────────────

// TraceMessageStream fetches the trace then streams an LLM failure analysis.
func (c *AIController) TraceMessageStream(ctx *gin.Context) {
	var req traceMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	sseHeaders(ctx)

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 300*time.Second)
	defer cancel()

	writeSSE := sseWriter(ctx)

	// First, send the raw trace as a structured SSE event so the UI can render
	// the step-by-step table while the LLM analysis streams in below it.
	trace, err := c.svc.TraceMessage(reqCtx, req.MessageID, req.InterfaceID)
	if err != nil {
		writeSSE("[ERROR] " + err.Error())
		writeSSE("[DONE]")
		return
	}
	traceJSON, _ := json.Marshal(trace)
	writeSSE("[TRACE]" + string(traceJSON))

	// Now stream the LLM analysis
	_, streamErr := c.svc.TraceMessageStream(reqCtx, req.MessageID, req.InterfaceID, func(token string) error {
		b, _ := json.Marshal(token)
		return writeSSE(string(b))
	})
	if streamErr != nil {
		writeSSE("[ERROR] " + streamErr.Error())
	}
	writeSSE("[DONE]")
}

// ─── POST /api/ai/explain-step/stream ────────────────────────────────────────

type explainStepRequest struct {
	StepID string `json:"step_id" binding:"required"`
}

// ExplainStepStream reads a step config from DB and streams a plain-English explanation.
func (c *AIController) ExplainStepStream(ctx *gin.Context) {
	var req explainStepRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	sseHeaders(ctx)

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 300*time.Second)
	defer cancel()

	writeSSE := sseWriter(ctx)
	err := c.svc.ExplainStepStream(reqCtx, req.StepID, func(token string) error {
		b, _ := json.Marshal(token)
		return writeSSE(string(b))
	})
	if err != nil {
		writeSSE("[ERROR] " + err.Error())
	}
	writeSSE("[DONE]")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// sseHeaders sets the required SSE response headers and writes 200.
func sseHeaders(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Flush()
}

// sseWriter returns a closure that writes one SSE data line and flushes.
func sseWriter(ctx *gin.Context) func(string) error {
	return func(payload string) error {
		_, err := fmt.Fprintf(ctx.Writer, "data: %s\n\n", payload)
		if err != nil {
			return err
		}
		ctx.Writer.Flush()
		return nil
	}
}

func (c *AIController) resolveSchemaDir() string {
	dir := c.schemaDir
	if dir == "" {
		dir = os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	}
	if dir == "" {
		dir = "/app/schemas"
	}
	// Normalise: we want the parent containing hl7/ and fhir/ subdirectories
	if b := filepath.Base(dir); b == "hl7" || b == "fhir" {
		dir = filepath.Dir(dir)
	}
	return dir
}

// ─── POST /api/ai/feedback/response ──────────────────────────────────────────

type responseFeedbackRequest struct {
	Endpoint        string `json:"endpoint" binding:"required"` // ask | generate-script | trace-message | explain-step
	Sentiment       string `json:"sentiment" binding:"required"` // positive | negative
	Rating          *int   `json:"rating"`
	PromptPreview   string `json:"prompt_preview"`
	ResponsePreview string `json:"response_preview"`
	Comment         string `json:"comment"`
	SessionID       string `json:"session_id"`
	InterfaceID     string `json:"interface_id"`
	PipelineID      string `json:"pipeline_id"`
	StepID          string `json:"step_id"`
}

// FeedbackResponse records thumbs-up/down on any AI response.
// Positive feedback also triggers KB improvement (confirmed Q&A or script pairs).
func (c *AIController) FeedbackResponse(ctx *gin.Context) {
	var req responseFeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.Sentiment != "positive" && req.Sentiment != "negative" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "sentiment must be 'positive' or 'negative'"})
		return
	}

	userID := ""
	if u, ok := ctx.Get("userID"); ok {
		userID = fmt.Sprintf("%v", u)
	}

	fb := ai.FeedbackRecord{
		SessionID:       req.SessionID,
		UserID:          userID,
		Endpoint:        req.Endpoint,
		Sentiment:       req.Sentiment,
		Rating:          req.Rating,
		PromptPreview:   req.PromptPreview,
		ResponsePreview: req.ResponsePreview,
		Comment:         req.Comment,
		InterfaceID:     req.InterfaceID,
		PipelineID:      req.PipelineID,
		StepID:          req.StepID,
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	defer cancel()

	id, err := c.svc.Feedback.Save(reqCtx, fb)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Fire telemetry to Google Sheets — non-blocking, non-fatal.
	// promptPreview is intentionally omitted: it contains user-typed text that may
	// include PHI (patient names, MRNs, HL7 snippets). Only metadata is transmitted.
	if c.telemetry != nil {
		userEmail := ctx.GetHeader("X-User-Email")
		go c.telemetry.SendResponseFeedback(
			context.Background(),
			req.Sentiment, req.Endpoint, req.Comment,
			req.SessionID, req.InterfaceID, req.PipelineID, req.StepID,
			userEmail, "", // promptPreview withheld — potential PHI
		)
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": id}})
}

// ─── GET /api/ai/feedback/summary ────────────────────────────────────────────

// FeedbackSummary returns aggregate stats for the last N days (default 30).
// Admin-only endpoint for reviewing AI quality trends.
func (c *AIController) FeedbackSummary(ctx *gin.Context) {
	days := 30
	if v := ctx.Query("days"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &days)
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	summary, err := c.svc.Feedback.GetSummary(reqCtx, days)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// ─── GET /api/ai/feedback/export ─────────────────────────────────────────────

// FeedbackExport streams a CSV of all feedback for the last N days (default 90).
// Sanitized: only preview fields included, no full prompt/response content.
func (c *AIController) FeedbackExport(ctx *gin.Context) {
	days := 90
	if v := ctx.Query("days"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &days)
	}

	ctx.Header("Content-Type", "text/csv")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="ai-feedback-%s.csv"`,
		time.Now().Format("2006-01-02")))

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()

	if err := c.svc.Feedback.ExportCSV(reqCtx, days, ctx.Writer); err != nil {
		// Headers already sent; log the error but can't change status
		_ = err
	}
}

// ─── POST /api/ai/feedback/submit-to-team ────────────────────────────────────

type submitToTeamRequest struct {
	Days         int    `json:"days"`
	AdminComment string `json:"admin_comment"` // operator's notes to include
}

// FeedbackSubmitToTeam sends an anonymized aggregate report to the ezHealthKonnect
// dev team via the configured webhook URL. Only counts + admin comments — no PHI.
func (c *AIController) FeedbackSubmitToTeam(ctx *gin.Context) {
	var req submitToTeamRequest
	_ = ctx.ShouldBindJSON(&req)
	if req.Days <= 0 {
		req.Days = 30
	}

	webhookURL := services.GetAppSettings().GetAIConfig().FeedbackWebhookURL

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 15*time.Second)
	defer cancel()

	summary, err := c.svc.Feedback.GetSummary(reqCtx, req.Days)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Custom operator webhook — optional, skip silently if not configured.
	if webhookURL != "" {
		if err := c.svc.Feedback.SubmitToTeam(reqCtx, summary, req.AdminComment, webhookURL, "instance", "1.0"); err != nil {
			log.Printf("⚠️  Custom feedback webhook failed (non-critical): %v", err)
		}
	}

	// Built-in telemetry channel (Google Sheets) — always fires, non-blocking.
	if c.telemetry != nil {
		go c.telemetry.SendFeedback(context.Background(), summary, req.AdminComment)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Submitted %d feedback records to ezHealthKonnect team.", summary.Total),
	})
}

// ─── GET /api/ai/conversations ────────────────────────────────────────────────

// GetConversationHistory returns stored turns for a session so the frontend
// can replay history when the user reopens the chat panel.
func (c *AIController) GetConversationHistory(ctx *gin.Context) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "session_id required"})
		return
	}
	userID := ctx.GetHeader("X-User-ID")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "authentication required"})
		return
	}
	limit := 50
	if v := ctx.Query("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	msgs, err := c.svc.Memory.LoadHistoryWithTimestamps(reqCtx, sessionID, userID, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": msgs, "count": len(msgs)})
}

// ─── GET /api/ai/models/catalog ───────────────────────────────────────────────

// GetModelCatalog returns the curated list of recommended Ollama models.
func (c *AIController) GetModelCatalog(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": ai.ModelCatalog()})
}

// ─── GET /api/ai/models/local ─────────────────────────────────────────────────

// GetLocalModels lists models already downloaded on the Ollama instance.
func (c *AIController) GetLocalModels(ctx *gin.Context) {
	ollama := c.svc.OllamaClient()
	if ollama == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Ollama not configured"})
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	models, err := ollama.ListModels(reqCtx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": models, "count": len(models)})
}

// ─── POST /api/ai/models/pull ─────────────────────────────────────────────────

type pullModelRequest struct {
	Model string `json:"model" binding:"required"`
}

// PullModelStream triggers an Ollama model pull and streams progress as SSE.
// Each event: data: {"status":"...","completed":N,"total":N,"percent":N}
// End:         data: [DONE]
// Error:       data: [ERROR] message
func (c *AIController) PullModelStream(ctx *gin.Context) {
	var req pullModelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	ollama := c.svc.OllamaClient()
	if ollama == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Ollama not configured"})
		return
	}

	sseHeaders(ctx)
	writeSSE := sseWriter(ctx)

	// 30-minute timeout — large models take time
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Minute)
	defer cancel()

	err := ollama.PullModel(reqCtx, req.Model, func(event ai.PullProgress) error {
		b, _ := json.Marshal(event)
		return writeSSE(string(b))
	})
	if err != nil {
		writeSSE("[ERROR] " + err.Error())
	}
	writeSSE("[DONE]")
}

// ─── POST /api/ai/ingest/docs ─────────────────────────────────────────────────

// IngestDocs rebuilds the app documentation knowledge (architecture docs, connector docs).
// Does NOT ingest source code — only behavioural documentation safe to expose to users.
func (c *AIController) IngestDocs(ctx *gin.Context) {
	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 15*time.Minute)
	defer cancel()

	results := c.svc.Ingestion.IngestAppDocs(reqCtx, ".")
	totalChunks, totalErrors := 0, 0
	for _, r := range results {
		totalChunks += r.ChunksStored
		totalErrors += len(r.Errors)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"success": totalErrors == 0,
		"data": gin.H{
			"results":      results,
			"total_chunks": totalChunks,
			"total_errors": totalErrors,
		},
	})
}

func chunkSources(chunks []ai.RetrievedChunk) []string {
	seen := map[string]bool{}
	var sources []string
	for _, c := range chunks {
		key := c.SourceType + ":" + c.SourceRef
		if !seen[key] {
			seen[key] = true
			sources = append(sources, key)
		}
	}
	return sources
}
