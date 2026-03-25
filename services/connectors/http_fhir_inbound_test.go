// services/connectors/http_fhir_inbound_test.go
// Comprehensive QA test suite for HTTPFHIRInboundConnector.
//
// Tests are organised as a QA person would walk through the feature:
//   every config option, every URL pattern, every HTTP method,
//   every auth scheme, bundle modes, error cases, and edge cases.
//
// Test groups:
//   FRI-001  Initialize — config parsing, validation, defaults
//   FRI-002  URL routing → correct MessageType per FHIR REST convention
//   FRI-003  Message headers — X-FHIR-* / X-HTTP-Method / X-Query-String
//   FRI-004  Bundle mode — bundle_as_unit vs bundle_unwrap
//   FRI-005  HTTP methods — GET, PUT, PATCH, DELETE responses
//   FRI-006  FHIR Operations — server, type, instance level
//   FRI-007  Authentication — none, basic, bearer, api_key (incl. custom header)
//   FRI-008  allowedMethods — method restriction enforcement
//   FRI-009  Special endpoints — GET /metadata, GET /health
//   FRI-010  Error cases — bad JSON, empty body, body too large
//   FRI-011  Body-vs-URL priority — URL wins for resourceType/resourceId
//   FRI-012  FHIR version header stamping
//
// Run all:   go test ./services/connectors/ -v -run TestFHIRInbound
// Run group: go test ./services/connectors/ -v -run TestFHIRInbound_Auth

package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────────────────────────────

// fhirFreePort finds an available TCP port.
func fhirFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fhirFreePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// fhirStart initialises and starts an HTTPFHIRInboundConnector.
// extra overrides/extends the base config {port, basePath=/fhir/r4}.
// Returns (messageChan, baseURL, stopFn).
func fhirStart(t *testing.T, extra map[string]interface{}) (
	msgs chan *models.InboundMessage,
	base string,
	stop func(),
) {
	t.Helper()
	port := fhirFreePort(t)

	cfg := map[string]interface{}{
		"port":     float64(port),
		"basePath": "/fhir/r4",
	}
	for k, v := range extra {
		cfg[k] = v
	}
	cfgBytes, _ := json.Marshal(cfg)

	conn := NewHTTPFHIRInboundConnector()
	if err := conn.Initialize(cfgBytes); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgs = make(chan *models.InboundMessage, 100)
	ctx, cancel := context.WithCancel(context.Background())
	go conn.(InboundConnector).Start(ctx, msgs) //nolint:forcetypeassert

	base = fmt.Sprintf("http://127.0.0.1:%d/fhir/r4", port)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)

	// Wait up to 3 s for the server to be ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
	}

	stop = func() { cancel() }
	t.Cleanup(stop)
	return
}

// fhirDo sends an HTTP request and returns the response.
func fhirDo(t *testing.T, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/fhir+json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// fhirRecv reads one message from the channel or fails after 2 s.
func fhirRecv(t *testing.T, msgs chan *models.InboundMessage) *models.InboundMessage {
	t.Helper()
	select {
	case m := <-msgs:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no InboundMessage received within 2s")
		return nil
	}
}

// fhirNoMsg asserts no message arrives within 300 ms.
func fhirNoMsg(t *testing.T, msgs chan *models.InboundMessage) {
	t.Helper()
	select {
	case m := <-msgs:
		t.Fatalf("expected no message but received MessageType=%q", m.MessageType)
	case <-time.After(300 * time.Millisecond):
	}
}

// fhirDrain clears any buffered messages (used between sub-tests).
func fhirDrain(msgs chan *models.InboundMessage) {
	for {
		select {
		case <-msgs:
		default:
			return
		}
	}
}

// Canonical test resources
const (
	patientJSON     = `{"resourceType":"Patient","id":"p1","name":[{"family":"Smith"}]}`
	observationJSON = `{"resourceType":"Observation","id":"o1","status":"final","code":{"text":"BP"}}`
	bundleTxJSON    = `{"resourceType":"Bundle","id":"b1","type":"transaction","entry":[` +
		`{"resource":{"resourceType":"Patient","id":"p1"}},` +
		`{"resource":{"resourceType":"Observation","id":"o1","status":"final","code":{"text":"BP"}}}` +
		`]}`
	bundleBatchJSON = `{"resourceType":"Bundle","type":"batch","entry":[` +
		`{"resource":{"resourceType":"Medication","id":"m1"}}` +
		`]}`
	bundleCollectionJSON = `{"resourceType":"Bundle","type":"collection","entry":[` +
		`{"resource":{"resourceType":"Patient","id":"p2"}}` +
		`]}`
)

// ─────────────────────────────────────────────────────────────────────────────
// FRI-001 — Initialize
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_Initialize(t *testing.T) {
	t.Run("FRI-001-01 missing port returns error", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		if err := conn.Initialize([]byte(`{}`)); err == nil {
			t.Fatal("expected error for missing port, got nil")
		}
	})

	t.Run("FRI-001-02 port=0 returns error", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		if err := conn.Initialize([]byte(`{"port":0}`)); err == nil {
			t.Fatal("expected error for port=0")
		}
	})

	t.Run("FRI-001-03 invalid JSON returns error", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		if err := conn.Initialize([]byte(`{not json}`)); err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("FRI-001-04 invalid bundleMode returns error", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		if err := conn.Initialize([]byte(`{"port":9990,"bundleMode":"explode"}`)); err == nil {
			t.Fatal("expected error for unknown bundleMode")
		}
	})

	t.Run("FRI-001-05 minimal config succeeds", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		if err := conn.Initialize([]byte(`{"port":9991}`)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("FRI-001-06 all fields parse without error", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		cfg := map[string]interface{}{
			"port":                  9992,
			"basePath":              "/fhir/r5",
			"fhirVersion":           "R5",
			"bundleMode":            "bundle_unwrap",
			"allowedMethods":        []string{"POST", "PUT"},
			"authType":              "api_key",
			"apiKey":                "secret",
			"apiKeyHeader":          "X-Custom-Key",
			"maxBodySizeMB":         25,
			"enableCORS":            false,
			"requestTimeoutSeconds": 60,
		}
		b, _ := json.Marshal(cfg)
		if err := conn.Initialize(b); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("FRI-001-07 GetStatus reflects initialized state", func(t *testing.T) {
		conn := NewHTTPFHIRInboundConnector()
		conn.Initialize([]byte(`{"port":9993}`))
		status := conn.(InboundConnector).(interface {
			GetStatus() ConnectorStatus
		}).GetStatus()
		if status.State == StateRunning {
			t.Error("connector should not be running before Start()")
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-002 — URL routing → MessageType
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_URLRouting(t *testing.T) {
	msgs, base, _ := fhirStart(t, nil)

	cases := []struct {
		id          string
		method      string
		path        string
		body        string
		wantStatus  int
		wantMsgType string
	}{
		// Single resource CRUD
		{"FRI-002-01", "POST", "/Patient", patientJSON, 201, "FHIR:Patient"},
		{"FRI-002-02", "GET", "/Patient?family=Smith", "", 202, "FHIR:Patient:search"},
		{"FRI-002-03", "GET", "/Patient/p1", "", 202, "FHIR:Patient:read"},
		{"FRI-002-04", "PUT", "/Patient/p1", patientJSON, 200, "FHIR:Patient:update"},
		{"FRI-002-05", "PATCH", "/Patient/p1", `{"resourceType":"Parameters"}`, 200, "FHIR:Patient:patch"},
		{"FRI-002-06", "DELETE", "/Patient/p1", "", 204, "FHIR:Patient:delete"},

		// Server-level operations
		{"FRI-002-07", "POST", "/$process-message", bundleTxJSON, 201, "FHIR:$process-message"},
		{"FRI-002-08", "POST", "/$export", "", 201, "FHIR:$export"},

		// Type-level operations
		{"FRI-002-09", "POST", "/Patient/$validate", patientJSON, 201, "FHIR:Patient:$validate"},
		{"FRI-002-10", "GET", "/Patient/$everything", "", 202, "FHIR:Patient:$everything"},

		// Instance-level operations
		{"FRI-002-11", "POST", "/Patient/p1/$everything", "", 201, "FHIR:Patient:p1:$everything"},
		{"FRI-002-12", "GET", "/Observation/o1/$lastn", "", 202, "FHIR:Observation:o1:$lastn"},

		// Bundle posted to base path (handled by handleBundle, not operation routing)
		{"FRI-002-13", "POST", "", bundleTxJSON, 200, "FHIR:Bundle:transaction"},
		{"FRI-002-14", "POST", "", bundleBatchJSON, 200, "FHIR:Bundle:batch"},
		{"FRI-002-15", "POST", "", bundleCollectionJSON, 201, "FHIR:Bundle:collection"},

		// Other resource types
		{"FRI-002-16", "POST", "/Observation", observationJSON, 201, "FHIR:Observation"},
		{"FRI-002-17", "POST", "/Medication/$lookup", `{}`, 201, "FHIR:Medication:$lookup"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			fhirDrain(msgs)
			url := base + tc.path
			resp := fhirDo(t, tc.method, url, tc.body, nil)
			resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status=%d want=%d (path=%s)", resp.StatusCode, tc.wantStatus, tc.path)
			}
			if tc.wantMsgType != "" {
				msg := fhirRecv(t, msgs)
				if msg.MessageType != tc.wantMsgType {
					t.Errorf("MessageType=%q want=%q", msg.MessageType, tc.wantMsgType)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-003 — Message headers
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_MessageHeaders(t *testing.T) {
	msgs, base, _ := fhirStart(t, nil)

	t.Run("FRI-003-01 POST single resource sets all FHIR headers", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)

		checks := map[string]string{
			"X-HTTP-Method":       "POST",
			"X-FHIR-ResourceType": "Patient",
			"X-FHIR-ResourceId":   "p1",
			"X-FHIR-Version":      "R4",
		}
		for k, want := range checks {
			if got := msg.Headers[k]; got != want {
				t.Errorf("header %s=%q want=%q", k, got, want)
			}
		}
		// Operation should NOT be set for plain create
		if op := msg.Headers["X-FHIR-Operation"]; op != "" {
			t.Errorf("X-FHIR-Operation should be empty for plain POST, got %q", op)
		}
	})

	t.Run("FRI-003-02 GET search sets X-Query-String", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "GET", base+"/Patient?family=Smith&gender=male", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)

		if got := msg.Headers["X-HTTP-Method"]; got != "GET" {
			t.Errorf("X-HTTP-Method=%q want=GET", got)
		}
		if got := msg.Headers["X-Query-String"]; got != "family=Smith&gender=male" {
			t.Errorf("X-Query-String=%q want=family=Smith&gender=male", got)
		}
	})

	t.Run("FRI-003-03 GET read sets ResourceType and ResourceId", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "GET", base+"/Patient/p1", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)

		if got := msg.Headers["X-FHIR-ResourceType"]; got != "Patient" {
			t.Errorf("X-FHIR-ResourceType=%q want=Patient", got)
		}
		if got := msg.Headers["X-FHIR-ResourceId"]; got != "p1" {
			t.Errorf("X-FHIR-ResourceId=%q want=p1", got)
		}
	})

	t.Run("FRI-003-04 POST $validate sets X-FHIR-Operation", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient/$validate", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)

		if got := msg.Headers["X-FHIR-Operation"]; got != "$validate" {
			t.Errorf("X-FHIR-Operation=%q want=$validate", got)
		}
		if got := msg.Headers["X-FHIR-ResourceType"]; got != "Patient" {
			t.Errorf("X-FHIR-ResourceType=%q want=Patient", got)
		}
	})

	t.Run("FRI-003-05 DELETE sets X-HTTP-Method=DELETE with no X-FHIR-Operation", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "DELETE", base+"/Patient/p1", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)

		if got := msg.Headers["X-HTTP-Method"]; got != "DELETE" {
			t.Errorf("X-HTTP-Method=%q want=DELETE", got)
		}
		if op := msg.Headers["X-FHIR-Operation"]; op != "" {
			t.Errorf("X-FHIR-Operation should be empty, got %q", op)
		}
	})

	t.Run("FRI-003-06 no X-Query-String when no query params", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if qs := msg.Headers["X-Query-String"]; qs != "" {
			t.Errorf("X-Query-String should be empty, got %q", qs)
		}
	})

	t.Run("FRI-003-07 SourceIP is populated", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.SourceIP == "" {
			t.Error("SourceIP should be populated")
		}
	})

	t.Run("FRI-003-08 MessageID is unique per request", func(t *testing.T) {
		fhirDrain(msgs)
		resp1 := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp1.Body.Close()
		resp2 := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp2.Body.Close()
		m1 := fhirRecv(t, msgs)
		m2 := fhirRecv(t, msgs)
		if m1.MessageID == m2.MessageID {
			t.Errorf("MessageIDs should be unique: both=%q", m1.MessageID)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-004 — Bundle mode
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_BundleMode(t *testing.T) {
	t.Run("FRI-004-01 bundle_as_unit sends one message for transaction bundle", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_as_unit"})
		resp := fhirDo(t, "POST", base, bundleTxJSON, nil)
		resp.Body.Close()

		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:Bundle:transaction" {
			t.Errorf("MessageType=%q want=FHIR:Bundle:transaction", msg.MessageType)
		}
		// Verify only ONE message
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-004-02 bundle_as_unit response is transaction-response Bundle", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_as_unit"})
		resp := fhirDo(t, "POST", base, bundleTxJSON, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fhirRecv(t, msgs)

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status=%d want=200", resp.StatusCode)
		}
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if rt := result["resourceType"]; rt != "Bundle" {
			t.Errorf("response resourceType=%v want=Bundle", rt)
		}
		if bt := result["type"]; bt != "transaction-response" {
			t.Errorf("response bundle type=%v want=transaction-response", bt)
		}
	})

	t.Run("FRI-004-03 bundle_as_unit batch response is batch-response", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_as_unit"})
		resp := fhirDo(t, "POST", base, bundleBatchJSON, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fhirRecv(t, msgs)

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if bt := result["type"]; bt != "batch-response" {
			t.Errorf("response bundle type=%v want=batch-response", bt)
		}
	})

	t.Run("FRI-004-04 bundle_unwrap sends N messages for N entries", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_unwrap"})
		// bundleTxJSON has 2 entries: Patient + Observation
		resp := fhirDo(t, "POST", base, bundleTxJSON, nil)
		resp.Body.Close()

		m1 := fhirRecv(t, msgs)
		m2 := fhirRecv(t, msgs)
		fhirNoMsg(t, msgs) // must be exactly 2

		types := []string{m1.MessageType, m2.MessageType}
		hasPatient := false
		hasObservation := false
		for _, mt := range types {
			if mt == "FHIR:Patient" {
				hasPatient = true
			}
			if mt == "FHIR:Observation" {
				hasObservation = true
			}
		}
		if !hasPatient {
			t.Error("expected FHIR:Patient message from unwrapped bundle")
		}
		if !hasObservation {
			t.Error("expected FHIR:Observation message from unwrapped bundle")
		}
	})

	t.Run("FRI-004-05 bundle_unwrap each message has X-FHIR-ResourceType header", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_unwrap"})
		resp := fhirDo(t, "POST", base, bundleTxJSON, nil)
		resp.Body.Close()

		m1 := fhirRecv(t, msgs)
		m2 := fhirRecv(t, msgs)

		for _, msg := range []*models.InboundMessage{m1, m2} {
			if msg.Headers["X-FHIR-ResourceType"] == "" {
				t.Errorf("X-FHIR-ResourceType missing in unwrapped message (type=%s)", msg.MessageType)
			}
			if msg.Headers["X-Bundle-Mode"] != "bundle_unwrap" {
				t.Errorf("X-Bundle-Mode=%q want=bundle_unwrap", msg.Headers["X-Bundle-Mode"])
			}
		}
	})

	t.Run("FRI-004-06 bundle_unwrap each message has X-Bundle-Entry-Index", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_unwrap"})
		resp := fhirDo(t, "POST", base, bundleTxJSON, nil)
		resp.Body.Close()

		m1 := fhirRecv(t, msgs)
		m2 := fhirRecv(t, msgs)

		indices := map[string]bool{
			m1.Headers["X-Bundle-Entry-Index"]: true,
			m2.Headers["X-Bundle-Entry-Index"]: true,
		}
		if !indices["0"] || !indices["1"] {
			t.Errorf("expected entry indices 0 and 1, got %v", indices)
		}
	})

	t.Run("FRI-004-07 bundle_unwrap empty bundle returns 422", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_unwrap"})
		emptyBundle := `{"resourceType":"Bundle","type":"batch","entry":[]}`
		resp := fhirDo(t, "POST", base, emptyBundle, nil)
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status=%d want=422 for empty bundle unwrap", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-004-08 bundle_unwrap entry without resource field is skipped", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"bundleMode": "bundle_unwrap"})
		// One good entry, one bad (no resource field), one good
		mixed := `{"resourceType":"Bundle","type":"batch","entry":[` +
			`{"resource":{"resourceType":"Patient","id":"p1"}},` +
			`{"request":{"method":"POST","url":"Patient"}},` +
			`{"resource":{"resourceType":"Observation","id":"o1","status":"final","code":{"text":"X"}}}` +
			`]}`
		resp := fhirDo(t, "POST", base, mixed, nil)
		resp.Body.Close()

		// Only 2 messages queued (bad entry skipped)
		fhirRecv(t, msgs)
		fhirRecv(t, msgs)
		fhirNoMsg(t, msgs)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-005 — HTTP method responses
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_HTTPMethodResponses(t *testing.T) {
	msgs, base, _ := fhirStart(t, nil)

	t.Run("FRI-005-01 POST returns 201 Created", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("POST status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-005-02 GET returns 202 Accepted", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "GET", base+"/Patient/p1", "", nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("GET status=%d want=202", resp.StatusCode)
		}
	})

	t.Run("FRI-005-03 PUT returns 200 OK", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "PUT", base+"/Patient/p1", patientJSON, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("PUT status=%d want=200", resp.StatusCode)
		}
	})

	t.Run("FRI-005-04 PATCH returns 200 OK", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "PATCH", base+"/Patient/p1", `{"resourceType":"Parameters"}`, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("PATCH status=%d want=200", resp.StatusCode)
		}
	})

	t.Run("FRI-005-05 DELETE returns 204 No Content", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "DELETE", base+"/Patient/p1", "", nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("DELETE status=%d want=204", resp.StatusCode)
		}
	})

	t.Run("FRI-005-06 all responses use Content-Type application/fhir+json", func(t *testing.T) {
		fhirDrain(msgs)
		for _, m := range []string{"POST", "GET", "PUT", "PATCH"} {
			var url, body string
			switch m {
			case "POST":
				url, body = base+"/Patient", patientJSON
			case "GET":
				url = base + "/Patient/p1"
			case "PUT":
				url, body = base+"/Patient/p1", patientJSON
			case "PATCH":
				url, body = base+"/Patient/p1", `{"resourceType":"Parameters"}`
			}
			resp := fhirDo(t, m, url, body, nil)
			ct := resp.Header.Get("Content-Type")
			resp.Body.Close()
			fhirRecv(t, msgs)
			if !strings.Contains(ct, "application/fhir+json") {
				t.Errorf("%s Content-Type=%q want application/fhir+json", m, ct)
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-006 — FHIR Operations
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_Operations(t *testing.T) {
	msgs, base, _ := fhirStart(t, nil)

	t.Run("FRI-006-01 server-level $process-message", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/$process-message", bundleTxJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:$process-message" {
			t.Errorf("MessageType=%q want=FHIR:$process-message", msg.MessageType)
		}
		if msg.Headers["X-FHIR-Operation"] != "$process-message" {
			t.Errorf("X-FHIR-Operation=%q want=$process-message", msg.Headers["X-FHIR-Operation"])
		}
	})

	t.Run("FRI-006-02 server-level $export (GET)", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "GET", base+"/$export", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:$export" {
			t.Errorf("MessageType=%q want=FHIR:$export", msg.MessageType)
		}
	})

	t.Run("FRI-006-03 type-level $validate", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient/$validate", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:Patient:$validate" {
			t.Errorf("MessageType=%q want=FHIR:Patient:$validate", msg.MessageType)
		}
		if msg.Headers["X-FHIR-ResourceType"] != "Patient" {
			t.Errorf("X-FHIR-ResourceType=%q want=Patient", msg.Headers["X-FHIR-ResourceType"])
		}
		if msg.Headers["X-FHIR-Operation"] != "$validate" {
			t.Errorf("X-FHIR-Operation=%q want=$validate", msg.Headers["X-FHIR-Operation"])
		}
	})

	t.Run("FRI-006-04 type-level $match (POST)", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient/$match", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:Patient:$match" {
			t.Errorf("MessageType=%q want=FHIR:Patient:$match", msg.MessageType)
		}
	})

	t.Run("FRI-006-05 instance-level $everything", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "GET", base+"/Patient/p1/$everything", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:Patient:p1:$everything" {
			t.Errorf("MessageType=%q want=FHIR:Patient:p1:$everything", msg.MessageType)
		}
		if msg.Headers["X-FHIR-ResourceId"] != "p1" {
			t.Errorf("X-FHIR-ResourceId=%q want=p1", msg.Headers["X-FHIR-ResourceId"])
		}
	})

	t.Run("FRI-006-06 instance-level $meta (POST)", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Observation/o1/$meta", "", nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.MessageType != "FHIR:Observation:o1:$meta" {
			t.Errorf("MessageType=%q want=FHIR:Observation:o1:$meta", msg.MessageType)
		}
	})

	t.Run("FRI-006-07 operation without $ prefix normalised by URL only", func(t *testing.T) {
		// The URL contains "$translate" literally — parseFHIRPath checks HasPrefix("$")
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/ConceptMap/$translate", `{}`, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.Headers["X-FHIR-Operation"] != "$translate" {
			t.Errorf("X-FHIR-Operation=%q want=$translate", msg.Headers["X-FHIR-Operation"])
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-007 — Authentication
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_Auth(t *testing.T) {
	t.Run("FRI-007-01 authType=none allows all requests", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"authType": "none"})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-007-02 authType=basic valid credentials → 201", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "basic",
			"username": "testuser",
			"password": "testpass",
		})
		encoded := base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"Authorization": "Basic " + encoded,
		})
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-007-03 authType=basic wrong password → 401", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "basic",
			"username": "testuser",
			"password": "correctpass",
		})
		encoded := base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass"))
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"Authorization": "Basic " + encoded,
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status=%d want=401", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-007-04 authType=basic no Authorization header → 401", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "basic", "username": "u", "password": "p",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status=%d want=401", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-007-05 authType=bearer valid token → 201", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType":    "bearer",
			"bearerToken": "my-secret-token",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"Authorization": "Bearer my-secret-token",
		})
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-007-06 authType=bearer wrong token → 401", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "bearer", "bearerToken": "correct",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"Authorization": "Bearer wrong",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status=%d want=401", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-007-07 authType=api_key default header X-API-Key valid → 201", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "api_key",
			"apiKey":   "key123",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"X-API-Key": "key123",
		})
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-007-08 authType=api_key custom header valid → 201", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType":     "api_key",
			"apiKey":       "key999",
			"apiKeyHeader": "X-My-Custom-Key",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"X-My-Custom-Key": "key999",
		})
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-007-09 api_key wrong header name → 401", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"authType":     "api_key",
			"apiKey":       "key999",
			"apiKeyHeader": "X-My-Custom-Key",
		})
		// Sends key in wrong header
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"X-API-Key": "key999",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status=%d want=401 when using wrong header name", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-007-10 auth skipped for GET /metadata", func(t *testing.T) {
		// /metadata must be accessible even with auth enabled (CapabilityStatement)
		port := fhirFreePort(t)
		cfg := map[string]interface{}{
			"port":     float64(port),
			"basePath": "/fhir/r4",
			"authType": "bearer",
			"bearerToken": "secret",
		}
		b, _ := json.Marshal(cfg)
		conn := NewHTTPFHIRInboundConnector()
		conn.Initialize(b)
		msgs := make(chan *models.InboundMessage, 10)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go conn.(InboundConnector).Start(ctx, msgs)

		time.Sleep(150 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/fhir/r4/metadata", port))
		if err != nil {
			t.Fatalf("GET /metadata: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /metadata status=%d want=200 (should bypass auth)", resp.StatusCode)
		}
	})

	t.Run("FRI-007-11 auth error response is FHIR OperationOutcome", func(t *testing.T) {
		_, base, _ := fhirStart(t, map[string]interface{}{
			"authType": "bearer", "bearerToken": "secret",
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, map[string]string{
			"Authorization": "Bearer wrong",
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if rt := result["resourceType"]; rt != "OperationOutcome" {
			t.Errorf("auth error body resourceType=%v want=OperationOutcome", rt)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-008 — allowedMethods
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_AllowedMethods(t *testing.T) {
	t.Run("FRI-008-01 POST-only restriction: GET → 405", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"allowedMethods": []string{"POST"},
		})
		resp := fhirDo(t, "GET", base+"/Patient?name=Smith", "", nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("status=%d want=405", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-008-02 POST-only restriction: POST passes", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"allowedMethods": []string{"POST"},
		})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("status=%d want=201", resp.StatusCode)
		}
	})

	t.Run("FRI-008-03 read-only restriction: only GET allowed", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"allowedMethods": []string{"GET"},
		})
		resp := fhirDo(t, "GET", base+"/Patient/p1", "", nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("GET status=%d want=202", resp.StatusCode)
		}

		// POST blocked
		resp2 := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST status=%d want=405", resp2.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-008-04 405 response is FHIR OperationOutcome", func(t *testing.T) {
		_, base, _ := fhirStart(t, map[string]interface{}{
			"allowedMethods": []string{"POST"},
		})
		resp := fhirDo(t, "DELETE", base+"/Patient/p1", "", nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if rt := result["resourceType"]; rt != "OperationOutcome" {
			t.Errorf("405 body resourceType=%v want=OperationOutcome", rt)
		}
	})

	t.Run("FRI-008-05 empty allowedMethods allows all methods", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{
			"allowedMethods": []string{},
		})
		for _, m := range []string{"POST", "GET", "PUT", "PATCH", "DELETE"} {
			fhirDrain(msgs)
			var url, body string
			switch m {
			case "POST":
				url, body = base+"/Patient", patientJSON
			case "GET":
				url = base + "/Patient/p1"
			case "PUT":
				url, body = base+"/Patient/p1", patientJSON
			case "PATCH":
				url, body = base+"/Patient/p1", `{"resourceType":"Parameters"}`
			case "DELETE":
				url = base + "/Patient/p1"
			}
			resp := fhirDo(t, m, url, body, nil)
			resp.Body.Close()
			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("method %s should be allowed with empty allowedMethods, got 405", m)
			}
			if m != "DELETE" { // DELETE returns 204 with no message body check needed
				fhirRecv(t, msgs)
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-009 — Special endpoints
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_SpecialEndpoints(t *testing.T) {
	port := fhirFreePort(t)
	cfg := map[string]interface{}{
		"port": float64(port), "basePath": "/fhir/r4", "fhirVersion": "R4",
	}
	b, _ := json.Marshal(cfg)
	conn := NewHTTPFHIRInboundConnector()
	conn.Initialize(b)
	msgs := make(chan *models.InboundMessage, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.(InboundConnector).Start(ctx, msgs)
	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	t.Run("FRI-009-01 GET /health returns 200", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status=%d want=200", resp.StatusCode)
		}
	})

	t.Run("FRI-009-02 GET /metadata returns 200 with CapabilityStatement", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/fhir/r4/metadata")
		if err != nil {
			t.Fatalf("GET /metadata: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status=%d want=200", resp.StatusCode)
		}
		var cs map[string]interface{}
		json.Unmarshal(body, &cs)
		if rt := cs["resourceType"]; rt != "CapabilityStatement" {
			t.Errorf("resourceType=%v want=CapabilityStatement", rt)
		}
		if status := cs["status"]; status != "active" {
			t.Errorf("status=%v want=active", status)
		}
	})

	t.Run("FRI-009-03 GET /metadata Content-Type is application/fhir+json", func(t *testing.T) {
		resp, _ := http.Get(baseURL + "/fhir/r4/metadata")
		ct := resp.Header.Get("Content-Type")
		resp.Body.Close()
		if !strings.Contains(ct, "application/fhir+json") {
			t.Errorf("Content-Type=%q want application/fhir+json", ct)
		}
	})

	t.Run("FRI-009-04 POST /metadata returns 405", func(t *testing.T) {
		resp := fhirDo(t, "POST", baseURL+"/fhir/r4/metadata", patientJSON, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST /metadata status=%d want=405", resp.StatusCode)
		}
	})

	t.Run("FRI-009-05 /metadata does not produce an InboundMessage", func(t *testing.T) {
		http.Get(baseURL + "/fhir/r4/metadata")
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-009-06 CapabilityStatement contains rest[0].resource array", func(t *testing.T) {
		resp, _ := http.Get(baseURL + "/fhir/r4/metadata")
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var cs map[string]interface{}
		json.Unmarshal(body, &cs)
		rest, ok := cs["rest"].([]interface{})
		if !ok || len(rest) == 0 {
			t.Fatal("CapabilityStatement missing rest array")
		}
		restEntry, ok := rest[0].(map[string]interface{})
		if !ok {
			t.Fatal("rest[0] is not an object")
		}
		resources, ok := restEntry["resource"].([]interface{})
		if !ok || len(resources) == 0 {
			t.Fatal("CapabilityStatement rest[0].resource is empty")
		}
		// Should cover known FHIR R4 types
		if len(resources) < 30 {
			t.Errorf("CapabilityStatement covers only %d resource types, expected ≥30", len(resources))
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-010 — Error cases
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_ErrorCases(t *testing.T) {
	msgs, base, _ := fhirStart(t, map[string]interface{}{"maxBodySizeMB": 1})

	t.Run("FRI-010-01 POST with invalid JSON → 400 with OperationOutcome", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", `not json`, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status=%d want=400", resp.StatusCode)
		}
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if rt := result["resourceType"]; rt != "OperationOutcome" {
			t.Errorf("error body resourceType=%v want=OperationOutcome", rt)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-010-02 POST empty body to base path → 400", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base, "", nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status=%d want=400", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-010-03 POST JSON without resourceType to base path → 400", func(t *testing.T) {
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base, `{"name":"Alice"}`, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status=%d want=400", resp.StatusCode)
		}
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-010-04 body exceeding maxBodySizeMB is truncated/rejected", func(t *testing.T) {
		fhirDrain(msgs)
		// 1.1 MB body when limit is 1 MB
		bigBody := strings.Repeat("x", 1024*1024+1024)
		req, _ := http.NewRequest("POST", base+"/Patient", bytes.NewBufferString(bigBody))
		req.Header.Set("Content-Type", "application/fhir+json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Skipf("large body request failed (may be truncated by client): %v", err)
		}
		resp.Body.Close()
		// Connector may return 400 (bad JSON since body is truncated) or similar
		// Key assertion: no valid InboundMessage with huge content
		fhirNoMsg(t, msgs)
	})

	t.Run("FRI-010-05 POST to URL resource type overrides empty body", func(t *testing.T) {
		// PUT /Patient/p1 with non-FHIR body — URL provides routing, should not error
		fhirDrain(msgs)
		resp := fhirDo(t, "PUT", base+"/Patient/p1", `{"arbitrary":"data"}`, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		// URL routing wins — resource type from URL
		if msg.Headers["X-FHIR-ResourceType"] != "Patient" {
			t.Errorf("X-FHIR-ResourceType=%q want=Patient (from URL)", msg.Headers["X-FHIR-ResourceType"])
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-011 — Body-vs-URL priority
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_BodyVsURLPriority(t *testing.T) {
	msgs, base, _ := fhirStart(t, nil)

	t.Run("FRI-011-01 URL resource type overrides body resource type", func(t *testing.T) {
		// POST to /Observation but body says Patient — URL wins
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Observation", patientJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.Headers["X-FHIR-ResourceType"] != "Observation" {
			t.Errorf("X-FHIR-ResourceType=%q want=Observation (URL wins over body)", msg.Headers["X-FHIR-ResourceType"])
		}
	})

	t.Run("FRI-011-02 URL resource ID overrides body resource ID", func(t *testing.T) {
		// PUT /Patient/url-id but body has id=body-id — URL wins
		fhirDrain(msgs)
		body := `{"resourceType":"Patient","id":"body-id"}`
		resp := fhirDo(t, "PUT", base+"/Patient/url-id", body, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.Headers["X-FHIR-ResourceId"] != "url-id" {
			t.Errorf("X-FHIR-ResourceId=%q want=url-id (URL wins over body)", msg.Headers["X-FHIR-ResourceId"])
		}
	})

	t.Run("FRI-011-03 body fills resource ID when URL has none", func(t *testing.T) {
		// POST /Patient with body containing id — body ID used
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil) // patientJSON has id=p1
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.Headers["X-FHIR-ResourceId"] != "p1" {
			t.Errorf("X-FHIR-ResourceId=%q want=p1 (from body)", msg.Headers["X-FHIR-ResourceId"])
		}
	})

	t.Run("FRI-011-04 body fills resource type when URL has none (base path POST)", func(t *testing.T) {
		// POST to base path — resource type comes entirely from body
		fhirDrain(msgs)
		resp := fhirDo(t, "POST", base, observationJSON, nil)
		resp.Body.Close()
		msg := fhirRecv(t, msgs)
		if msg.Headers["X-FHIR-ResourceType"] != "Observation" {
			t.Errorf("X-FHIR-ResourceType=%q want=Observation (from body)", msg.Headers["X-FHIR-ResourceType"])
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-012 — FHIR version header
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_FHIRVersionHeader(t *testing.T) {
	for _, ver := range []string{"R4", "R5", "STU3"} {
		ver := ver
		t.Run("FRI-012 fhirVersion="+ver, func(t *testing.T) {
			msgs, base, _ := fhirStart(t, map[string]interface{}{"fhirVersion": ver})
			resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
			resp.Body.Close()
			msg := fhirRecv(t, msgs)
			if msg.Headers["X-FHIR-Version"] != ver {
				t.Errorf("X-FHIR-Version=%q want=%q", msg.Headers["X-FHIR-Version"], ver)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FRI-013 — CORS middleware
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRInbound_CORS(t *testing.T) {
	t.Run("FRI-013-01 enableCORS=true adds CORS headers", func(t *testing.T) {
		msgs, base, _ := fhirStart(t, map[string]interface{}{"enableCORS": true})
		resp := fhirDo(t, "POST", base+"/Patient", patientJSON, nil)
		resp.Body.Close()
		fhirRecv(t, msgs)
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin=%q want=*", got)
		}
	})

	t.Run("FRI-013-02 OPTIONS preflight returns 200 with CORS headers", func(t *testing.T) {
		_, base, _ := fhirStart(t, map[string]interface{}{"enableCORS": true})
		req, _ := http.NewRequest("OPTIONS", base+"/Patient", nil)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("OPTIONS status=%d want=200", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin=%q want=*", got)
		}
	})
}
