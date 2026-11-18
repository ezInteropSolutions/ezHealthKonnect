package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// fakePipelines answers GetPipeline and runs a canned ExecutePipeline.
type fakePipelines struct {
	mu      sync.Mutex
	keys    []string
	inputs  []map[string]interface{}
	result  *models.TransformationExecutionResult
	err     error
	getErr  error
	delay   time.Duration
	running int
	maxRun  int
}

func (f *fakePipelines) GetPipeline(_ context.Context, _, key string) (*models.TransformationPipeline, error) {
	f.mu.Lock()
	f.keys = append(f.keys, key)
	f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &models.TransformationPipeline{}, nil
}

func (f *fakePipelines) ExecutePipeline(ctx context.Context, _ *models.TransformationPipeline, in map[string]interface{}) (*models.TransformationExecutionResult, error) {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	f.running++
	if f.running > f.maxRun {
		f.maxRun = f.running
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.running--; f.mu.Unlock() }()
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
	}
	return f.result, f.err
}

func apiStep(alias string, status int, body string, headers map[string]string) models.StepExecutionLog {
	return models.StepExecutionLog{StepAlias: alias, StepName: alias, Success: true, StepOutput: &models.StepOutput{
		ExecutionDetails: map[string]interface{}{"http_response": map[string]interface{}{
			"status_code": status, "body_raw": body, "content_type": "application/fhir+json", "headers": headers}}}}
}

func newSyncConnector(t *testing.T, sync map[string]interface{}, pe PipelineExecutor) *HTTPFHIRInboundConnector {
	t.Helper()
	cfg := map[string]interface{}{"port": 9, "interface_id": "if-1", "syncReply": sync}
	raw, _ := json.Marshal(cfg)
	c := NewHTTPFHIRInboundConnector().(*HTTPFHIRInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatal(err)
	}
	c.messageChan = make(chan *models.InboundMessage, 1)
	if pe != nil {
		c.SetPipelineExecutor(pe)
	}
	return c
}

func do(c *HTTPFHIRInboundConnector, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/fhir+json")
	w := httptest.NewRecorder()
	c.handleFHIRRequest(w, req)
	return w
}

const patient = `{"resourceType":"Patient","id":"p1","active":true}`

func TestSyncReply_ReturnsTheAnsweringSystemsStatusBodyAndHeaders(t *testing.T) {
	pe := &fakePipelines{result: &models.TransformationExecutionResult{ExecutionLog: []models.StepExecutionLog{
		apiStep("first", 200, `{"resourceType":"OperationOutcome","issue":[{"diagnostics":"first"}]}`, nil),
		apiStep("send", 412, `{"resourceType":"OperationOutcome","issue":[{"diagnostics":"stale"}]}`,
			map[string]string{"ETag": `"v2"`, "X-Secret": "no"}),
	}}}
	c := newSyncConnector(t, map[string]interface{}{"enabled": true}, pe)
	w := do(c, http.MethodPut, "/fhir/r4/Patient/p1", patient)
	if w.Code != 412 || !strings.Contains(w.Body.String(), "stale") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("ETag") != `"v2"` || w.Header().Get("X-Secret") != "" {
		t.Errorf("headers: %v", w.Header())
	}
	if len(c.messageChan) != 0 {
		t.Error("a synchronous request was also queued")
	}
	in := pe.inputs[0]
	h, _ := in["_http"].(map[string]interface{})
	if in["raw"] != patient {
		t.Errorf("raw body: %v", in["raw"])
	}
	if in["resourceType"] != "Patient" || h["method"] != "PUT" || h["resourceId"] != "p1" {
		t.Errorf("pipeline input: %v", in)
	}
	if pe.keys[0] != "FHIR:Patient:update" {
		t.Errorf("pipeline key %q", pe.keys[0])
	}
}

func TestSyncReply_ReplyStepAndPipelineKeyAreConfigurable(t *testing.T) {
	pe := &fakePipelines{result: &models.TransformationExecutionResult{ExecutionLog: []models.StepExecutionLog{
		apiStep("send", 200, "mine", nil), apiStep("audit", 201, "other", nil)}}}
	c := newSyncConnector(t, map[string]interface{}{"enabled": true, "replyStep": "SEND", "pipelineMessageType": "FHIR:Patient"}, pe)
	if w := do(c, http.MethodPost, "/fhir/r4/Patient", patient); w.Code != 200 || w.Body.String() != "mine" {
		t.Errorf("got %d %s", w.Code, w.Body.String())
	}
	if pe.keys[0] != "FHIR:Patient" {
		t.Errorf("key %q", pe.keys[0])
	}
}

func TestSyncReply_DeleteHasNoBodyButSaysWhatItIsAbout(t *testing.T) {
	pe := &fakePipelines{result: &models.TransformationExecutionResult{ExecutionLog: []models.StepExecutionLog{apiStep("s", 204, "", nil)}}}
	c := newSyncConnector(t, map[string]interface{}{"enabled": true}, pe)
	w := do(c, http.MethodDelete, "/fhir/r4/Patient/p9", "")
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if h, _ := pe.inputs[0]["_http"].(map[string]interface{}); h["resourceId"] != "p9" || h["method"] != "DELETE" {
		t.Errorf("input: %v", pe.inputs[0])
	}
}

func TestSyncReply_Failures(t *testing.T) {
	cases := []struct {
		name   string
		pe     *fakePipelines
		cfg    map[string]interface{}
		body   string
		status int
	}{
		{"pipeline failed before any call", &fakePipelines{err: errors.New("boom"), result: &models.TransformationExecutionResult{}},
			map[string]interface{}{"enabled": true}, patient, 502},
		{"no pipeline", &fakePipelines{getErr: errors.New("none")}, map[string]interface{}{"enabled": true}, patient, 500},
		{"unreadable body", &fakePipelines{}, map[string]interface{}{"enabled": true}, `{not json`, 400},
		{"timeout", &fakePipelines{delay: 2 * time.Second, result: &models.TransformationExecutionResult{}},
			map[string]interface{}{"enabled": true, "timeoutSeconds": 1}, patient, 504},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newSyncConnector(t, tc.cfg, tc.pe)
			if w := do(c, http.MethodPut, "/fhir/r4/Patient/p1", tc.body); w.Code != tc.status ||
				!strings.Contains(w.Body.String(), "OperationOutcome") {
				t.Errorf("got %d %s, want %d", w.Code, w.Body.String(), tc.status)
			}
		})
	}
}

func TestSyncReply_NotWiredIsRefusedNotQueued(t *testing.T) {
	c := newSyncConnector(t, map[string]interface{}{"enabled": true}, nil)
	if w := do(c, http.MethodPut, "/fhir/r4/Patient/p1", patient); w.Code != 503 {
		t.Errorf("got %d", w.Code)
	}
	if len(c.messageChan) != 0 {
		t.Error("queued")
	}
}

func TestSyncReply_SerializesRequests(t *testing.T) {
	pe := &fakePipelines{delay: 30 * time.Millisecond, result: &models.TransformationExecutionResult{
		ExecutionLog: []models.StepExecutionLog{apiStep("s", 200, "ok", nil)}}}
	c := newSyncConnector(t, map[string]interface{}{"enabled": true}, pe)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() { defer wg.Done(); do(c, http.MethodPut, "/fhir/r4/Patient/p1", patient) }()
	}
	wg.Wait()
	if pe.maxRun != 1 {
		t.Errorf("%d requests ran at once", pe.maxRun)
	}
}

func TestSyncReply_OffByDefaultStillQueues(t *testing.T) {
	c := newSyncConnector(t, nil, &fakePipelines{})
	if w := do(c, http.MethodPut, "/fhir/r4/Patient/p1", patient); w.Code != 200 || len(c.messageChan) != 1 {
		t.Errorf("got %d queued=%d", w.Code, len(c.messageChan))
	}
}

func TestSyncReply_SwitchMayBeText(t *testing.T) {
	for in, want := range map[interface{}]bool{true: true, "true": true, " Yes ": true, "1": true, "false": false, "": false,
		"{{var.immediateReply}}": false, nil: false, 1.0: false} {
		if got := truthy(in); got != want {
			t.Errorf("truthy(%v) = %v, want %v", in, got, want)
		}
	}
	if !parseSyncReplyConfig(map[string]interface{}{"syncReply": map[string]interface{}{"enabled": "true", "timeoutSeconds": "5"}}).Enabled {
		t.Error("text switch not read")
	}
}
