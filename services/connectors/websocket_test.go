// services/connectors/websocket_test.go
// Tests for WebSocketInboundConnector / WebSocketOutboundConnector.
//
// Run:
//   go test ./services/connectors/ -v -run TestWebSocket
package connectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"

	"github.com/gorilla/websocket"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// startEchoWSServer starts a real local websocket server that echoes back
// whatever frame (text or binary) it receives, unchanged. Returns the ws://
// URL to connect to and a stop function.
func startEchoWSServer(t *testing.T) (wsURL string, stop func()) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(msgType, payload); err != nil {
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	wsURL = "ws" + strings.TrimPrefix(server.URL, "http") + "/echo"
	return wsURL, server.Close
}

// ─────────────────────────────────────────────────────────────────────────────
// Inbound: Initialize/Validate field checks
// ─────────────────────────────────────────────────────────────────────────────

func TestWebSocketInbound_Initialize_PortRequired(t *testing.T) {
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{})
	if err := c.Initialize(raw); err == nil {
		t.Fatalf("expected Initialize to fail when port is missing")
	}
}

func TestWebSocketInbound_Validate_TLSRequiresCertAndKey(t *testing.T) {
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	port := findFreePort(t)
	raw, _ := json.Marshal(map[string]interface{}{"port": port, "tls_enabled": true})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected Validate to fail when tls_enabled=true with no cert/key configured")
	}
}

func TestWebSocketInbound_Validate_RejectsUnknownAuthType(t *testing.T) {
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	port := findFreePort(t)
	raw, _ := json.Marshal(map[string]interface{}{"port": port, "authentication_type": "totally-bogus"})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected Validate to reject an unknown authentication_type")
	}
}

func TestWebSocketInbound_Validate_PortRange(t *testing.T) {
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{"port": 999999})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected Validate to reject an out-of-range port")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Outbound: Initialize/Validate field checks
// ─────────────────────────────────────────────────────────────────────────────

func TestWebSocketOutbound_Initialize_URLRequired(t *testing.T) {
	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{})
	if err := c.Initialize(raw); err == nil {
		t.Fatalf("expected Initialize to fail when url is missing")
	}
}

func TestWebSocketOutbound_Validate_RejectsNonWSScheme(t *testing.T) {
	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{"url": "http://example.invalid/ws"})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected Validate to reject a non-ws(s):// URL")
	}
}

func TestWebSocketOutbound_Validate_RejectsInvalidConnectionMode(t *testing.T) {
	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{"url": "ws://example.invalid/ws", "connection_mode": "bogus"})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected Validate to reject an invalid connection_mode")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Outbound: real local echo-server round trips
// ─────────────────────────────────────────────────────────────────────────────

func TestWebSocketOutbound_Send_EchoServer_CapturesTextResponse(t *testing.T) {
	wsURL, stop := startEchoWSServer(t)
	defer stop()

	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{
		"url":                      wsURL,
		"connection_mode":          "per-message",
		"response_timeout_seconds": 5,
	})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer c.Close()

	result, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "m1", Content: "hello websocket"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true, got false (err=%s)", result.ErrorMessage)
	}
	if got := result.Metadata["response_body"]; got != "hello websocket" {
		t.Errorf("expected echoed response_body='hello websocket', got %v", got)
	}
	if got := result.Metadata["response_received"]; got != true {
		t.Errorf("expected response_received=true, got %v", got)
	}
	if got := result.Metadata["response_frame_type"]; got != "text" {
		t.Errorf("expected response_frame_type=text, got %v", got)
	}
}

func TestWebSocketOutbound_Send_BinaryResponse_Base64Encoded(t *testing.T) {
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	binaryPayload := []byte{0x01, 0x02, 0x03, 0xFF}
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, binaryPayload)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/echo"

	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{
		"url":                      wsURL,
		"connection_mode":          "per-message",
		"response_timeout_seconds": 5,
	})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer c.Close()

	result, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "m2", Content: "trigger"})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if got := result.Metadata["response_frame_type"]; got != "binary" {
		t.Errorf("expected response_frame_type=binary, got %v", got)
	}
	expected := base64.StdEncoding.EncodeToString(binaryPayload)
	if got := result.Metadata["response_binary_base64"]; got != expected {
		t.Errorf("expected response_binary_base64=%s, got %v", expected, got)
	}
	if _, ok := result.Metadata["response_body"]; ok {
		t.Errorf("expected response_body to be absent for a binary response, a binary reply must never be silently coerced to text")
	}
}

func TestWebSocketOutbound_Send_NoResponseWithinTimeout_StillSucceeds(t *testing.T) {
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/silent", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(3 * time.Second) // never replies within the client's short timeout
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/silent"

	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{
		"url":                      wsURL,
		"connection_mode":          "per-message",
		"response_timeout_seconds": 1,
	})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer c.Close()

	start := time.Now()
	result, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "m3", Content: "ping"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected no error on a response timeout (the write already succeeded), got: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true even without a response, got false (err=%s)", result.ErrorMessage)
	}
	if got := result.Metadata["response_received"]; got != false {
		t.Errorf("expected response_received=false, got %v", got)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected Send to actually wait out the ~1s response timeout, took %v", elapsed)
	}
}

func TestWebSocketOutbound_PersistentMode_ReconnectsAfterConnectionDrop(t *testing.T) {
	wsURL, stop := startEchoWSServer(t)
	defer stop()

	c := NewWebSocketOutboundConnector().(*WebSocketOutboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{
		"url":                      wsURL,
		"connection_mode":          "persistent",
		"response_timeout_seconds": 3,
		"max_retries":              1,
	})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer c.Close()

	r1, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "a", Content: "one"})
	if err != nil {
		t.Fatalf("first Send error: %v", err)
	}
	if got := r1.Metadata["response_body"]; got != "one" {
		t.Fatalf("expected first response_body='one', got %v", got)
	}

	// Forcibly close the client-side of the cached persistent connection —
	// deterministically simulates a dropped connection without depending on
	// real-world TCP FIN/RST propagation timing. c.conn/c.mu are accessible
	// directly since this test lives in the same package.
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()

	r2, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "b", Content: "two"})
	if err != nil {
		t.Fatalf("second Send error (expected a transparent redial): %v", err)
	}
	if got := r2.Metadata["response_body"]; got != "two" {
		t.Fatalf("expected second response_body='two' after redial, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Inbound: real local listener round trips
// ─────────────────────────────────────────────────────────────────────────────

func TestWebSocketInbound_Start_ReceivesMessageOnChannel(t *testing.T) {
	port := findFreePort(t)
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{"port": port})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := c.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()
	time.Sleep(150 * time.Millisecond) // let the server actually bind

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("hello server")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case msg := <-msgChan:
		if msg.Content != "hello server" {
			t.Errorf("expected content 'hello server', got %q", msg.Content)
		}
		if msg.SourceType != "websocket" {
			t.Errorf("expected SourceType=websocket, got %q", msg.SourceType)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for message on channel")
	}
}

func TestWebSocketInbound_BearerAuth_RejectsMissingToken(t *testing.T) {
	port := findFreePort(t)
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{
		"port":                port,
		"authentication_type": "bearer",
		"bearer_token":        "secret123",
	})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := c.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()
	time.Sleep(150 * time.Millisecond)

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	_, resp, err := dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err == nil {
		t.Fatalf("expected upgrade to fail without a valid Authorization header")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestWebSocketInbound_Stop_UnblocksBlockedConnections(t *testing.T) {
	port := findFreePort(t)
	c := NewWebSocketInboundConnector().(*WebSocketInboundConnector)
	raw, _ := json.Marshal(map[string]interface{}{"port": port})
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := c.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer conn.Close()

	// The server's handleConnection goroutine is now blocked in
	// ReadMessage() (the client never sends anything). Stop() must force it
	// to unblock (via server.Close() + explicit activeConns closing) rather
	// than leaving it hanging — this is the test that would catch a
	// regression on that shutdown design.
	if err := c.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected the client-side read to observe the connection closing once Stop() force-closed it server-side")
	}
}
