// services/connectors/http_fhir_sync_reply.go
// Optional synchronous reply for the HTTP FHIR Receiver.
//
// By default the receiver queues a request and answers 201/202 at once: the
// pipeline runs later, in order, and the sender never sees its outcome. With
// "syncReply.enabled" the receiver instead runs the interface's own pipeline
// before it answers, and returns what one of the pipeline's API steps got
// back from the system it called (its status, body and a few headers). A
// relay in front of a FHIR server then answers its client with that server's
// own OperationOutcome.
//
// It is generic: nothing in it knows what the pipeline calls. It is modelled
// on the TCP/MLLP receiver's host_query.
//
// What a synchronous reply gives up, by design:
//   - the request is not queued, so it does not appear in the interface's
//     message list (the pipeline's own execution is still logged);
//   - the engine's per-interface ordering is replaced by "serialize" (on by
//     default): one request is run at a time, in arrival order.
//
// Config (all optional except enabled):
//
//	"syncReply": {
//	  "enabled": true,
//	  "timeoutSeconds": 20,          // total, including waiting for its turn
//	  "serialize": true,             // one request at a time, in arrival order
//	  "replyStep": "",               // alias of the step whose answer is returned;
//	                                 // blank = the last API step that ran
//	  "pipelineMessageType": ""      // pipeline key; blank = the request's own
//	                                 // type, then the interface's own pipeline
//	}
//
// The pipeline's input is the parsed resource at the root (as for a queued
// request) plus "raw" (the body as received) and "_http": {method, resourceType, resourceId, query}, so a
// DELETE, which has no body, can still say what it is about.

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/parsers"
)

// syncReplyConfig is the parsed "syncReply" setting.
type syncReplyConfig struct {
	Enabled             bool
	Timeout             time.Duration
	Serialize           bool
	ReplyStep           string
	PipelineMessageType string
}

// parseSyncReplyConfig reads the nested "syncReply" object; a missing or
// disabled one leaves the receiver exactly as it was.
func parseSyncReplyConfig(cfg map[string]interface{}) syncReplyConfig {
	m, _ := cfg["syncReply"].(map[string]interface{})
	out := syncReplyConfig{
		Enabled:             truthy(m["enabled"]),
		Timeout:             time.Duration(getIntFromMap(m, "timeoutSeconds", 20)) * time.Second,
		Serialize:           getBoolFromMap(m, "serialize", true),
		ReplyStep:           strings.TrimSpace(getStringFromMap(m, "replyStep", "")),
		PipelineMessageType: strings.TrimSpace(getStringFromMap(m, "pipelineMessageType", "")),
	}
	if out.Timeout <= 0 {
		out.Timeout = 20 * time.Second
	}
	return out
}

// truthy reads a switch that may arrive as text, as a setup-form value does:
// true, "true", "yes", "on" and "1" are on; anything else (including an
// unfilled "{{var.x}}" placeholder) is off.
func truthy(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true", "yes", "on", "1":
			return true
		}
	}
	return false
}

// SetPipelineExecutor implements PipelineAwareConnector.
func (h *HTTPFHIRInboundConnector) SetPipelineExecutor(pe PipelineExecutor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pipelineExecutor = pe
}

// syncReplyAnswer is what the reply step got back.
type syncReplyAnswer struct {
	Status      int
	Body        string
	ContentType string
	Headers     map[string]string
}

// replyHeaders are the answering system's headers passed on to the client.
var replyHeaders = []string{"ETag", "Location", "Content-Location", "Last-Modified", "Retry-After"}

// answerFromResult finds the answer in a finished pipeline: the step named
// replyStep, else the last step that made an HTTP call. ok is false when no
// step answered.
func answerFromResult(result *models.TransformationExecutionResult, replyStep string) (syncReplyAnswer, bool) {
	var ans syncReplyAnswer
	found := false
	if result == nil {
		return ans, false
	}
	for _, l := range result.ExecutionLog {
		if l.StepOutput == nil || l.StepOutput.ExecutionDetails == nil {
			continue
		}
		resp, _ := l.StepOutput.ExecutionDetails["http_response"].(map[string]interface{})
		code, hasCode := intOf(resp["status_code"])
		if !hasCode {
			continue
		}
		if replyStep != "" && !strings.EqualFold(l.StepAlias, replyStep) && !strings.EqualFold(l.StepName, replyStep) {
			continue
		}
		ans = syncReplyAnswer{Status: code, Headers: map[string]string{}}
		ans.Body, _ = resp["body_raw"].(string)
		ans.ContentType, _ = resp["content_type"].(string)
		if hdrs, ok := resp["headers"].(map[string]string); ok {
			for _, k := range replyHeaders {
				for hk, hv := range hdrs {
					if strings.EqualFold(hk, k) {
						ans.Headers[k] = hv
					}
				}
			}
		} else if hdrs, ok := resp["headers"].(map[string]interface{}); ok {
			for _, k := range replyHeaders {
				for hk, hv := range hdrs {
					if s, ok := hv.(string); ok && strings.EqualFold(hk, k) {
						ans.Headers[k] = s
					}
				}
			}
		}
		found = true
	}
	return ans, found
}

func intOf(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// handleSyncReply runs the pipeline for one request and writes the answer.
func (h *HTTPFHIRInboundConnector) handleSyncReply(w http.ResponseWriter, r *http.Request, body []byte,
	msgType, resourceType, resourceID string) {

	h.mu.RLock()
	pe, cfg, interfaceID := h.pipelineExecutor, h.syncReply, h.interfaceID
	h.mu.RUnlock()
	if pe == nil || interfaceID == "" {
		h.sendOperationOutcome(w, http.StatusServiceUnavailable, "error", "transient",
			"Synchronous replies are not available on this interface yet")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
	defer cancel()

	// One request at a time, in arrival order: what the engine's queue gives a
	// queued request.
	if cfg.Serialize {
		select {
		case h.syncGate <- struct{}{}:
			defer func() { <-h.syncGate }()
		case <-ctx.Done():
			h.sendOperationOutcome(w, http.StatusServiceUnavailable, "error", "timeout",
				"Too many requests waiting; please retry")
			return
		}
	}

	key := cfg.PipelineMessageType
	if key == "" {
		key = msgType
	}
	pipeline, err := pe.GetPipeline(ctx, interfaceID, key)
	if err != nil || pipeline == nil {
		log.Printf("❌ HTTP FHIR Inbound: sync reply has no pipeline for %s on %s: %v", key, interfaceID, err)
		h.sendOperationOutcome(w, http.StatusInternalServerError, "fatal", "exception",
			"No pipeline is set up to answer this request")
		return
	}

	input := map[string]interface{}{}
	if len(body) > 0 {
		// The FHIR parser passes unreadable text through, so check it here.
		var probe map[string]interface{}
		if jsonErr := json.Unmarshal(body, &probe); jsonErr != nil || probe["resourceType"] == nil {
			h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
				"The body is not a FHIR resource in JSON")
			return
		}
		if parsed := parsers.NewFHIRParserService().Parse(string(body)); parsed.Success {
			input = parsed.ParsedJSON
			input["raw"] = string(body)
			input["_format"] = string(models.FormatFHIR)
			if len(parsed.EnhancedFields) > 0 {
				input["enhancedFields"] = parsed.EnhancedFields
			}
		} else {
			h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
				fmt.Sprintf("The body could not be read as FHIR JSON: %v", parsed.Error))
			return
		}
	}
	input["_http"] = map[string]interface{}{
		"method": r.Method, "resourceType": resourceType, "resourceId": resourceID, "query": r.URL.RawQuery,
	}

	type outcome struct {
		result *models.TransformationExecutionResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		res, execErr := pe.ExecutePipeline(ctx, pipeline, input)
		done <- outcome{res, execErr}
	}()

	select {
	case <-ctx.Done():
		log.Printf("⏱️ HTTP FHIR Inbound: sync reply timed out after %s", cfg.Timeout)
		h.sendOperationOutcome(w, http.StatusGatewayTimeout, "error", "timeout", "The request took too long to process")
	case o := <-done:
		ans, ok := answerFromResult(o.result, cfg.ReplyStep)
		switch {
		case ok:
			h.writeAnswer(w, ans)
		case o.err != nil:
			log.Printf("❌ HTTP FHIR Inbound: sync reply pipeline failed: %v", o.err)
			h.sendOperationOutcome(w, http.StatusBadGateway, "error", "transient", "The request could not be processed")
		default:
			h.sendOperationOutcome(w, http.StatusOK, "information", "informational", "Processed; the system answered nothing")
		}
	}
}

// writeAnswer sends the answering system's status, body and headers on.
func (h *HTTPFHIRInboundConnector) writeAnswer(w http.ResponseWriter, a syncReplyAnswer) {
	for k, v := range a.Headers {
		w.Header().Set(k, v)
	}
	ct := a.ContentType
	if ct == "" {
		ct = "application/fhir+json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(a.Status)
	if a.Body != "" && a.Status != http.StatusNoContent {
		_, _ = w.Write([]byte(a.Body))
	}
}
