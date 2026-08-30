// services/connectors/http_rest_inbound_test.go
package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

func basicAuthHeaderValue(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func newTestHTTPRestInbound(t *testing.T, cfg map[string]interface{}) *HTTPRestInboundConnector {
	t.Helper()
	c := NewHTTPRestInboundConnector().(*HTTPRestInboundConnector)
	b, _ := json.Marshal(cfg)
	if err := c.Initialize(b); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	return c
}

func TestHTTPRestInbound_Initialize_Defaults(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18080, "authentication_type": "none"})
	if c.endpointPath != "/api/hl7/receive" {
		t.Errorf("expected default endpoint_path, got: %s", c.endpointPath)
	}
	if !c.httpMethods["POST"] {
		t.Error("expected default http_methods to include POST")
	}
}

func TestHTTPRestInbound_Initialize_RequiresPort(t *testing.T) {
	c := NewHTTPRestInboundConnector().(*HTTPRestInboundConnector)
	b, _ := json.Marshal(map[string]interface{}{"authentication_type": "none"})
	if err := c.Initialize(b); err == nil {
		t.Error("expected Initialize to reject config without port")
	}
}

func TestHTTPRestInbound_Initialize_RejectsInvalidAuthType(t *testing.T) {
	c := NewHTTPRestInboundConnector().(*HTTPRestInboundConnector)
	b, _ := json.Marshal(map[string]interface{}{"port": 18081, "authentication_type": "oauth2"})
	if err := c.Initialize(b); err == nil {
		t.Error("expected Initialize to reject an unrecognized authentication_type")
	}
}

func TestHTTPRestInbound_Validate_ApiKeyRequiresValue(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18082, "authentication_type": "api_key"})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject api_key auth without an api_key value")
	}
}

func TestHTTPRestInbound_Validate_BasicAuthRequiresCreds(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18083, "authentication_type": "basic_auth"})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject basic_auth without username/password")
	}
}

func TestHTTPRestInbound_Validate_BearerRequiresToken(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18084, "authentication_type": "bearer_token"})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject bearer_token auth without a token value")
	}
}

func TestHTTPRestInbound_Validate_NoneRequiresNothing(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18085, "authentication_type": "none"})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass for auth_type none, got: %v", err)
	}
}

func TestHTTPRestInbound_CheckAuth_ApiKey(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": 18086, "authentication_type": "api_key", "api_key_header": "X-API-Key", "api_key": "secret123",
	})
	good := &http.Request{Header: http.Header{"X-Api-Key": []string{"secret123"}}}
	bad := &http.Request{Header: http.Header{"X-Api-Key": []string{"wrong"}}}
	if !c.checkAuth(good) {
		t.Error("expected correct API key to pass auth")
	}
	if c.checkAuth(bad) {
		t.Error("expected incorrect API key to fail auth")
	}
}

func TestHTTPRestInbound_CheckAuth_BasicAuth(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": 18087, "authentication_type": "basic_auth", "username": "user1", "password": "pass1",
	})
	good := &http.Request{Header: http.Header{"Authorization": []string{basicAuthHeaderValue("user1", "pass1")}}}
	bad := &http.Request{Header: http.Header{"Authorization": []string{basicAuthHeaderValue("user1", "wrong")}}}
	if !c.checkAuth(good) {
		t.Error("expected correct basic auth credentials to pass")
	}
	if c.checkAuth(bad) {
		t.Error("expected incorrect basic auth credentials to fail")
	}
}

func TestHTTPRestInbound_CheckAuth_BearerToken(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": 18088, "authentication_type": "bearer_token", "bearer_token": "tok-abc",
	})
	good := &http.Request{Header: http.Header{"Authorization": []string{"Bearer tok-abc"}}}
	bad := &http.Request{Header: http.Header{"Authorization": []string{"Bearer wrong"}}}
	if !c.checkAuth(good) {
		t.Error("expected correct bearer token to pass")
	}
	if c.checkAuth(bad) {
		t.Error("expected incorrect bearer token to fail")
	}
}

func TestHTTPRestInbound_CheckAuth_None(t *testing.T) {
	c := newTestHTTPRestInbound(t, map[string]interface{}{"port": 18089, "authentication_type": "none"})
	req := &http.Request{Header: http.Header{}}
	if !c.checkAuth(req) {
		t.Error("expected auth_type none to always pass")
	}
}

// TestHTTPRestInbound_LiveRequest_EndToEnd starts a real HTTP server on a real
// port and sends a real HTTP request, proving the whole receive path (auth,
// method check, body passthrough, message-channel delivery) works together —
// not just each piece in isolation.
func TestHTTPRestInbound_LiveRequest_EndToEnd(t *testing.T) {
	port := 19090
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": port, "endpoint_path": "/intake", "authentication_type": "api_key",
		"api_key_header": "X-API-Key", "api_key": "topsecret",
	})

	msgChan := make(chan *models.InboundMessage, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.Start(ctx, msgChan)
	time.Sleep(200 * time.Millisecond) // let the listener come up

	body := []byte(`MSH|^~\&|TEST|TEST|EHK|EHK|20260830000000||ADT^A01|MSG001|P|2.5`)
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/intake", port), bytes.NewReader(body))
	req.Header.Set("X-API-Key", "topsecret")
	req.Header.Set("Content-Type", "application/hl7-v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}

	select {
	case msg := <-msgChan:
		if msg.Content != string(body) {
			t.Errorf("message content = %q, want %q", msg.Content, string(body))
		}
		if msg.ContentType != "application/hl7-v2" {
			t.Errorf("expected content type from request header to be preserved, got: %s", msg.ContentType)
		}
		if msg.SourceType != "http_rest" {
			t.Errorf("expected SourceType 'http_rest', got: %s", msg.SourceType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a message on the channel, got none")
	}
}

func TestHTTPRestInbound_LiveRequest_RejectsWrongMethod(t *testing.T) {
	port := 19091
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": port, "endpoint_path": "/intake", "authentication_type": "none",
	})
	msgChan := make(chan *models.InboundMessage, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Start(ctx, msgChan)
	time.Sleep(200 * time.Millisecond)

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/intake", port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET when only POST is allowed, got %d", resp.StatusCode)
	}
}

func TestHTTPRestInbound_LiveRequest_RejectsBadAuth(t *testing.T) {
	port := 19092
	c := newTestHTTPRestInbound(t, map[string]interface{}{
		"port": port, "endpoint_path": "/intake", "authentication_type": "api_key",
		"api_key_header": "X-API-Key", "api_key": "correct",
	})
	msgChan := make(chan *models.InboundMessage, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Start(ctx, msgChan)
	time.Sleep(200 * time.Millisecond)

	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/intake", port), bytes.NewReader([]byte("data")))
	req.Header.Set("X-API-Key", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong API key, got %d", resp.StatusCode)
	}
}
