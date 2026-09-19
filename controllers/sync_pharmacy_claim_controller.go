// controllers/sync_pharmacy_claim_controller.go
// SyncPharmacyClaimController — a synchronous request/response endpoint for
// real-time NCPDP Telecommunication D.0 B1 (Claim Billing) pharmacy claims.
// A near-verbatim mirror of sync_claim_status_controller.go's own pattern
// (see that file's own doc comment for the full architecture rationale):
// unlike every other inbound data path in this system (a connector enqueues
// onto a channel, a detached background goroutine eventually calls
// TransformationPipelineService.ExecutePipeline), this endpoint calls
// ExecutePipeline directly from inside the live HTTP handler and returns the
// pipeline's own real output in the SAME response.
//
// Unlike SCRIPT/EDI's own initial async-only rollout (a synchronous endpoint
// added only later, on request), D.0's entire reason for existing is
// real-time point-of-sale adjudication — a synchronous request/response is
// the PRIMARY real-world delivery mode for this specific standard, so this
// endpoint ships as part of D.0's own Phase 1, not as a later add-on.
//
// This is pure transport/plumbing: the endpoint has no claim-adjudication
// logic of its own. It resolves the target interface's own configured
// "B1_REQUEST" pipeline (authored the same way any other pipeline is —
// ncpdptelecom.parse, then whatever business/lookup/adjudication steps a
// real deployment needs, then ncpdptelecom.build) and executes it exactly as
// configured, returning whatever that pipeline's own build step produces. If
// the resolved pipeline is missing, misconfigured, or fails, that is
// surfaced as a real HTTP error — this endpoint never invents a fallback
// adjudication answer.
//
// Scope boundary (named, not silently absent): the request body must be a
// raw D.0 B1 request payload — a JSON claim-request input mode is not
// implemented in this pass. Authentication is whatever already gates every
// other /api route (requireProxiedRequest() in Go, forwarded through the
// Node.js proxy) — this endpoint is not yet exposed with partner-facing
// authentication (API keys, its own listener) the way a real external
// pharmacy switch/VAN integration would eventually need.
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

// syncPharmacyClaimMessageType is the message_type a synchronous B1 claim
// request always resolves its pipeline by — an interface backing this
// endpoint is authored with a "B1_REQUEST" pipeline the same way any other
// interface is authored with a pipeline for its own message type.
const syncPharmacyClaimMessageType = "B1_REQUEST"

// syncPharmacyClaimTimeout bounds how long the HTTP caller waits — same
// rationale and value as sync_claim_status_controller.go's own timeout.
const syncPharmacyClaimTimeout = 25 * time.Second

// SyncPharmacyClaimController implements the synchronous B1-request-in/
// B1-response-out endpoint.
type SyncPharmacyClaimController struct {
	pipelineService *services.TransformationPipelineService
}

// NewSyncPharmacyClaimController constructs the controller with its own
// TransformationPipelineService instance — the same "dedicated instance"
// pattern SyncClaimStatusController/SyncEligibilityController already use,
// not a shared global.
func NewSyncPharmacyClaimController(pipelineService *services.TransformationPipelineService) *SyncPharmacyClaimController {
	return &SyncPharmacyClaimController{pipelineService: pipelineService}
}

// RegisterRoutes mounts the endpoint under the provided RouterGroup. Caller
// passes api.Group("/pharmacy-claim") from main.go — sitting under the same
// "/api" group whose requireProxiedRequest() gate every other route here
// already relies on.
func (c *SyncPharmacyClaimController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:interfaceId/submit", c.Submit)
}

// Submit handles POST /api/pharmacy-claim/:interfaceId/submit. The request
// body is a raw NCPDP Telecom D.0 B1 request transmission; the response body
// is whatever the resolved pipeline's own build step (ncpdptelecom.build, or
// any other build.* step type already in transformation_test_controller.go's
// buildStepContentFields map) produced — a real B1 adjudication response
// transmission for the intended use case, but this endpoint itself is
// transaction-code-agnostic.
func (c *SyncPharmacyClaimController) Submit(ctx *gin.Context) {
	interfaceID := ctx.Param("interfaceId")
	if interfaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId is required"})
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil || len(body) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "request body must contain a raw NCPDP Telecom D.0 B1 request transmission",
		})
		return
	}

	pipeline, err := c.pipelineService.GetPipeline(ctx.Request.Context(), interfaceID, syncPharmacyClaimMessageType)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s: %v", syncPharmacyClaimMessageType, interfaceID, err),
		})
		return
	}
	// GetPipeline gracefully falls back to ANY enabled pipeline for the
	// interface when no exact message_type match exists (a usability feature
	// for the Test Pipeline UI) — wrong for a partner-facing sync endpoint,
	// where silently running an unrelated pipeline against a B1 payload would
	// produce a confusing, not-actually-useful result. Require an exact match
	// here instead.
	if pipeline.MessageType != syncPharmacyClaimMessageType {
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no %s pipeline configured for interface %s", syncPharmacyClaimMessageType, interfaceID),
		})
		return
	}

	inputData := map[string]interface{}{"raw": string(body)}

	execCtx, cancel := context.WithTimeout(ctx.Request.Context(), syncPharmacyClaimTimeout)
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
			"error":   "pharmacy claim submission timed out",
		})
	case o := <-outcomeCh:
		c.respond(ctx, o.result, o.err)
	}
}

// respond maps a completed pipeline run onto a real HTTP status code and
// body — same shape as SyncClaimStatusController.respond.
func (c *SyncPharmacyClaimController) respond(ctx *gin.Context, result *models.TransformationExecutionResult, err error) {
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
			"error":   "pipeline completed but produced no recognizable build-step output (no ncpdptelecom.build/fhir.build/etc. step ran)",
		})
		return
	}

	ctx.Data(http.StatusOK, contentType, []byte(content))
}
