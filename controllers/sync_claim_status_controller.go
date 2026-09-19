// controllers/sync_claim_status_controller.go
// SyncClaimStatusController — a synchronous request/response endpoint for
// real-time 276/277 claim status checks. A near-verbatim mirror of
// sync_eligibility_controller.go's own pattern (see that file's own doc
// comment for the full architecture rationale): unlike every other inbound
// data path in this system (a connector enqueues onto a channel, a detached
// background goroutine eventually calls
// TransformationPipelineService.ExecutePipeline), this endpoint calls
// ExecutePipeline directly from inside the live HTTP handler and returns the
// pipeline's own real output in the SAME response.
//
// This is pure transport/plumbing: the endpoint has no claim-adjudication
// logic of its own. It resolves the target interface's own configured "276"
// pipeline (authored the same way any other pipeline is — edi.parse, then
// whatever business/lookup steps a real deployment needs, then edi.build) and
// executes it exactly as configured, returning whatever that pipeline's own
// build step produces. If the resolved pipeline is missing, misconfigured, or
// fails, that is surfaced as a real HTTP error — this endpoint never invents
// a fallback claim status answer.
//
// Scope boundary (named, not silently absent): the request body must be a
// raw 276 X12 payload — a JSON claim-status-request input mode is not
// implemented in this pass. Authentication is whatever already gates every
// other /api route (requireProxiedRequest() in Go, forwarded through the
// Node.js proxy) — this endpoint is not yet exposed with partner-facing
// authentication (API keys, its own listener) the way a real external AS2/
// claim-status trading-partner integration would eventually need.
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

// syncClaimStatusMessageType is the message_type a synchronous claim status
// request always resolves its pipeline by — an interface backing this
// endpoint is authored with a "276" pipeline the same way any other
// interface is authored with a pipeline for its own message type.
const syncClaimStatusMessageType = "276"

// syncClaimStatusTimeout bounds how long the HTTP caller waits — same
// rationale and value as sync_eligibility_controller.go's own timeout.
const syncClaimStatusTimeout = 25 * time.Second

// SyncClaimStatusController implements the synchronous 276-in/277-out endpoint.
type SyncClaimStatusController struct {
	pipelineService *services.TransformationPipelineService
}

// NewSyncClaimStatusController constructs the controller with its own
// TransformationPipelineService instance — the same "dedicated instance"
// pattern SyncEligibilityController already uses, not a shared global.
func NewSyncClaimStatusController(pipelineService *services.TransformationPipelineService) *SyncClaimStatusController {
	return &SyncClaimStatusController{pipelineService: pipelineService}
}

// RegisterRoutes mounts the endpoint under the provided RouterGroup. Caller
// passes api.Group("/claim-status") from main.go — sitting under the same
// "/api" group whose requireProxiedRequest() gate every other route here
// already relies on.
func (c *SyncClaimStatusController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:interfaceId/check", c.Check)
}

// Check handles POST /api/claim-status/:interfaceId/check. The request body
// is a raw 276 X12 payload; the response body is whatever the resolved
// pipeline's own build step (edi.build, or any other build.* step type
// already in transformation_test_controller.go's buildStepContentFields map)
// produced — a real 277 X12 document for the intended use case, but this
// endpoint itself is transaction-set-agnostic.
func (c *SyncClaimStatusController) Check(ctx *gin.Context) {
	interfaceID := ctx.Param("interfaceId")
	if interfaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId is required"})
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil || len(body) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "request body must contain a raw 276 X12 payload",
		})
		return
	}

	pipeline, err := c.pipelineService.GetPipeline(ctx.Request.Context(), interfaceID, syncClaimStatusMessageType)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s: %v", syncClaimStatusMessageType, interfaceID, err),
		})
		return
	}
	// GetPipeline gracefully falls back to ANY enabled pipeline for the
	// interface when no exact message_type match exists (a usability feature
	// for the Test Pipeline UI) — wrong for a partner-facing sync endpoint,
	// where silently running an unrelated pipeline (e.g. an 837 pipeline)
	// against a 276 payload would produce a confusing, not-actually-useful
	// result. Require an exact match here instead.
	if pipeline.MessageType != syncClaimStatusMessageType {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s", syncClaimStatusMessageType, interfaceID),
		})
		return
	}

	inputData := map[string]interface{}{"raw": string(body)}

	execCtx, cancel := context.WithTimeout(ctx.Request.Context(), syncClaimStatusTimeout)
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
			"error":   "claim status check timed out",
		})
	case o := <-outcomeCh:
		c.respond(ctx, o.result, o.err)
	}
}

// respond maps a completed pipeline run onto a real HTTP status code and
// body — same shape as SyncEligibilityController.respond.
func (c *SyncClaimStatusController) respond(ctx *gin.Context, result *models.TransformationExecutionResult, err error) {
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
