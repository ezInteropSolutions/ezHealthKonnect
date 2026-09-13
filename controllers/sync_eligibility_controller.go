// controllers/sync_eligibility_controller.go
// SyncEligibilityController — a synchronous request/response endpoint for
// real-time 270/271 eligibility checks. Unlike every other inbound data path
// in this system (a connector enqueues onto a channel, a detached background
// goroutine eventually calls TransformationPipelineService.ExecutePipeline),
// this endpoint calls ExecutePipeline directly from inside the live HTTP
// handler and returns the pipeline's own real output in the SAME response —
// the same "call ExecutePipeline synchronously" pattern
// transformation_test_controller.go's TestPipeline already proves works,
// applied to a real (non-test) data path instead of a dry-run preview.
//
// This is pure transport/plumbing: the endpoint has no eligibility-decision
// logic of its own. It resolves the target interface's own configured "270"
// pipeline (authored the same way any other pipeline is — edi.parse, then
// whatever business/lookup steps a real deployment needs, then edi.build) and
// executes it exactly as configured, returning whatever that pipeline's own
// build step produces. If the resolved pipeline is missing, misconfigured, or
// fails, that is surfaced as a real HTTP error — this endpoint never invents
// a fallback eligibility answer.
//
// Scope boundary (named, not silently absent): the request body must be a
// raw 270 X12 payload — a JSON eligibility-request input mode is not
// implemented in this pass. Authentication is whatever already gates every
// other /api route (requireProxiedRequest() in Go, forwarded through the
// Node.js proxy) — this endpoint is not yet exposed with partner-facing
// authentication (API keys, its own listener) the way a real external AS2/
// eligibility trading-partner integration would eventually need.
package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// syncEligibilityMessageType is the message_type a synchronous eligibility
// request always resolves its pipeline by — an interface backing this
// endpoint is authored with a "270" pipeline the same way any other
// interface is authored with a pipeline for its own message type.
const syncEligibilityMessageType = "270"

// syncEligibilityTimeout bounds how long the HTTP caller waits. ExecutePipeline
// itself has no internal cancellation checks in its own step loop (confirmed
// by reading transformation_pipeline_helpers.go), so this timeout controls
// only how long the HTTP RESPONSE waits — a genuinely hung pipeline keeps
// running in its own goroutine to completion rather than truly aborting. That
// is a named, accepted limitation for this phase, not silently papered over.
const syncEligibilityTimeout = 25 * time.Second

// SyncEligibilityController implements the synchronous 270-in/271-out endpoint.
type SyncEligibilityController struct {
	pipelineService *services.TransformationPipelineService
}

// NewSyncEligibilityController constructs the controller with its own
// TransformationPipelineService instance — the same "dedicated instance"
// pattern main.go already uses for script-verification (see main.go's own
// scriptVerifyPipelineSvc comment), not a shared global.
func NewSyncEligibilityController(pipelineService *services.TransformationPipelineService) *SyncEligibilityController {
	return &SyncEligibilityController{pipelineService: pipelineService}
}

// RegisterRoutes mounts the endpoint under the provided RouterGroup. Caller
// passes api.Group("/eligibility") from main.go — sitting under the same
// "/api" group whose requireProxiedRequest() gate every other route here
// already relies on.
func (c *SyncEligibilityController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:interfaceId/check", c.Check)
}

// Check handles POST /api/eligibility/:interfaceId/check. The request body
// is a raw 270 X12 payload; the response body is whatever the resolved
// pipeline's own build step (edi.build, or any other build.* step type
// already in transformation_test_controller.go's buildStepContentFields map)
// produced — a real 271 X12 document for the intended use case, but this
// endpoint itself is transaction-set-agnostic.
func (c *SyncEligibilityController) Check(ctx *gin.Context) {
	interfaceID := ctx.Param("interfaceId")
	if interfaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId is required"})
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil || len(body) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "request body must contain a raw 270 X12 payload",
		})
		return
	}

	pipeline, err := c.pipelineService.GetPipeline(ctx.Request.Context(), interfaceID, syncEligibilityMessageType)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s: %v", syncEligibilityMessageType, interfaceID, err),
		})
		return
	}
	// GetPipeline gracefully falls back to ANY enabled pipeline for the
	// interface when no exact message_type match exists (a usability feature
	// for the Test Pipeline UI) — wrong for a partner-facing sync endpoint,
	// where silently running an unrelated pipeline (e.g. an 837 pipeline)
	// against a 270 payload would produce a confusing, not-actually-useful
	// result. Require an exact match here instead.
	if pipeline.MessageType != syncEligibilityMessageType {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s", syncEligibilityMessageType, interfaceID),
		})
		return
	}

	inputData := map[string]interface{}{"raw": string(body)}

	execCtx, cancel := context.WithTimeout(ctx.Request.Context(), syncEligibilityTimeout)
	defer cancel()

	type outcome struct {
		result *models.TransformationExecutionResult
		err    error
	}
	outcomeCh := make(chan outcome, 1)
	go func() {
		result, err := c.pipelineService.ExecutePipeline(execCtx, pipeline, inputData)
		outcomeCh <- outcome{result, err}
	}()

	select {
	case <-execCtx.Done():
		ctx.JSON(http.StatusGatewayTimeout, gin.H{
			"success": false,
			"error":   "eligibility check timed out",
		})
	case o := <-outcomeCh:
		c.respond(ctx, o.result, o.err)
	}
}

// respond maps a completed pipeline run onto a real HTTP status code and
// body — TestPipeline's own always-200-with-a-success-flag shape is wrong
// for a machine-to-machine endpoint, whose caller needs to branch on the
// status code itself.
func (c *SyncEligibilityController) respond(ctx *gin.Context, result *models.TransformationExecutionResult, err error) {
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   fmt.Sprintf("pipeline execution failed: %v", err),
		})
		return
	}

	if result.Status != "completed" {
		messages := make([]string, 0, len(result.Errors))
		for _, e := range result.Errors {
			messages = append(messages, e.Message)
		}
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"status":  result.Status,
			"errors":  messages,
		})
		return
	}

	content, contentType := extractFirstBuildStepPayload(result)
	if content == "" {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error":   "pipeline completed but produced no recognizable build-step output (no edi.build/fhir.build/etc. step ran)",
		})
		return
	}

	ctx.Data(http.StatusOK, contentType, []byte(content))
}

// extractFirstBuildStepPayload reuses transformation_test_controller.go's own
// buildStepContentFields map + extractBuildStepPayloadEntry helper — the
// exact same mechanism that already knows how to pull hl7.build/cda.build/
// fhir.build/edi.build's real output out of a completed step's normalized
// output, rather than writing a second parallel extraction path. Returns the
// LAST matching build step's output — the pipeline's own final artifact,
// which is what a caller of this endpoint actually wants back.
func extractFirstBuildStepPayload(result *models.TransformationExecutionResult) (content, contentType string) {
	normalizer := models.NewOutputNormalizer()
	for _, stepLog := range result.ExecutionLog {
		if !stepLog.Success || stepLog.StepOutput == nil {
			continue
		}
		spec, ok := buildStepContentFields[stepLog.StepType]
		if !ok {
			continue
		}
		normalized := normalizer.NormalizeStepOutput(stepLog.StepOutput.OutputData)
		entry := extractBuildStepPayloadEntry(stepLog.StepName, spec, normalized)
		if entry == nil {
			continue
		}
		if c, ok := entry["content"].(string); ok && c != "" {
			content = c
			contentType, _ = entry["content_type"].(string)
		}
	}
	return content, contentType
}
