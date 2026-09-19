// services/connectors/websocket_outbound.go
// WebSocket Outbound Connector — dials a remote websocket server, sends one
// frame, and (by default) reads back whatever response the server sends over
// the same connection so a downstream pipeline step can use it. Mirrors
// tcp_mllp_outbound.go's shape: retry-with-delay handled inside Send()
// itself, persistent-or-per-message connection modes, reconnect-after-drop
// via a connDead flag.
package connectors

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/gorilla/websocket"
)

// WebSocketOutboundConnector dials a remote websocket server and sends messages.
type WebSocketOutboundConnector struct {
	*BaseOutboundConnector

	url             string
	subProtocol     string
	headers         map[string]string
	connectionMode  string // "persistent" | "per-message"
	connectTimeout  time.Duration
	writeTimeout    time.Duration
	waitForResponse bool
	responseTimeout time.Duration
	tlsSkipVerify   bool
	maxRetries      int
	retryDelayMs    int

	mu       sync.Mutex
	conn     *websocket.Conn
	connDead bool
}

// NewWebSocketOutboundConnector replaces the (nonexistent) stub — this
// connector type is 100% new, confirmed via a full grep of
// connector_stubs.go before writing this file.
func NewWebSocketOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "websocket_outbound",
		DisplayName:        "WebSocket Client",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_auth":  true, // via custom handshake headers
			"supports_retry": true,
		},
	}
	return &WebSocketOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration.
func (c *WebSocketOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.url = cfg.GetString("url")
	if c.url == "" {
		return fmt.Errorf("url is required")
	}

	c.subProtocol = cfg.GetString("sub_protocol")

	if hdrs := cfg.GetMap("headers"); hdrs != nil {
		c.headers = make(map[string]string, len(hdrs))
		for k, v := range hdrs {
			if s, ok := v.(string); ok {
				c.headers[k] = s
			}
		}
	}

	c.connectionMode = cfg.GetString("connection_mode")
	if c.connectionMode == "" {
		c.connectionMode = "persistent"
	}

	connectSec := cfg.GetInt("connect_timeout_seconds")
	if connectSec <= 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	writeSec := cfg.GetInt("write_timeout_seconds")
	if writeSec <= 0 {
		writeSec = 10
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	c.waitForResponse = cfg.GetBoolDefault("wait_for_response", true)

	respSec := cfg.GetInt("response_timeout_seconds")
	if respSec <= 0 {
		respSec = 10
	}
	c.responseTimeout = time.Duration(respSec) * time.Second

	c.tlsSkipVerify = cfg.GetBool("tls_skip_verify")

	c.maxRetries = cfg.GetInt("max_retries")
	c.retryDelayMs = cfg.GetInt("retry_delay_ms")
	if c.retryDelayMs == 0 {
		c.retryDelayMs = 500
	}

	c.SetMetadata("url", c.url)
	c.SetMetadata("connection_mode", c.connectionMode)

	log.Printf("✅ WebSocket Outbound initialized: url=%s mode=%s wait_for_response=%t",
		c.url, c.connectionMode, c.waitForResponse)
	return nil
}

// Validate checks configuration validity.
func (c *WebSocketOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	if c.url == "" {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("url is required"), false)
	}
	parsed, err := url.Parse(c.url)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("url must be a ws:// or wss:// address, got %q", c.url), false)
	}
	if c.connectionMode != "persistent" && c.connectionMode != "per-message" {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("connection_mode must be 'persistent' or 'per-message'"), false)
	}
	return nil
}

// TestConnection dials the remote endpoint without sending data.
func (c *WebSocketOutboundConnector) TestConnection(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return NewConnectorError(c.metadata.TypeName, "test_connection", err, true)
	}
	return conn.Close()
}

// wsSendResult holds what sendOnce learned about the response (if any).
type wsSendResult struct {
	responseReceived bool
	responseText     string
	responseBinary   []byte
	isBinary         bool
}

// Send wraps the retry loop around sendOnce, mirroring
// tcp_mllp_outbound.go's Send() shape.
func (c *WebSocketOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	content := message.Content
	if content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	var (
		result *wsSendResult
		err    error
	)

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(c.retryDelayMs) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		result, err = c.sendOnce(ctx, content)
		if err == nil {
			break
		}
		log.Printf("[websocket_outbound] attempt %d/%d failed: %v", attempt+1, c.maxRetries+1, err)
		if c.connectionMode == "persistent" {
			c.mu.Lock()
			c.connDead = true
			if c.conn != nil {
				_ = c.conn.Close()
				c.conn = nil
			}
			c.mu.Unlock()
		}
	}

	if err != nil {
		c.RecordError(err)
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: err.Error(),
			RetryCount:   c.maxRetries,
			DurationMs:   time.Since(start).Milliseconds(),
		}, err
	}

	c.IncrementMessagesSent()

	deliveryResult := &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata: map[string]interface{}{
			"response_received": result.responseReceived,
		},
	}
	if result.responseReceived {
		if result.isBinary {
			// A binary reply is never silently dropped — base64-encoded into
			// its own key so it stays valid UTF-8/JSON-safe as it flows
			// through step_output, distinct from the text-only response_body
			// key every other connector already uses.
			deliveryResult.Metadata["response_binary_base64"] = base64.StdEncoding.EncodeToString(result.responseBinary)
			deliveryResult.Metadata["response_frame_type"] = "binary"
		} else {
			deliveryResult.Metadata["response_body"] = result.responseText
			deliveryResult.Metadata["response_frame_type"] = "text"
		}
	}

	return deliveryResult, nil
}

// sendOnce writes one frame and, if configured, reads back one response
// frame within the response timeout.
func (c *WebSocketOutboundConnector) sendOnce(ctx context.Context, content string) (*wsSendResult, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	if err := conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(content)); err != nil {
		c.mu.Lock()
		c.connDead = true
		c.mu.Unlock()
		return nil, fmt.Errorf("write failed: %w", err)
	}

	result := &wsSendResult{}

	if !c.waitForResponse {
		if c.connectionMode == "per-message" {
			_ = conn.Close()
		}
		return result, nil
	}

	if err := conn.SetReadDeadline(time.Now().Add(c.responseTimeout)); err != nil {
		return result, nil
	}
	msgType, payload, err := conn.ReadMessage()
	if err != nil {
		// A read timeout or connection issue here is NOT a Send() failure —
		// the write already succeeded, so the message was genuinely
		// delivered; many real websocket sends are fire-and-forget with no
		// inline reply. Mark the persistent connection dead so the next
		// Send() redials rather than reusing a possibly-broken connection.
		c.mu.Lock()
		c.connDead = true
		c.mu.Unlock()
		if c.connectionMode == "per-message" {
			_ = conn.Close()
		}
		return result, nil
	}

	switch msgType {
	case websocket.TextMessage:
		result.responseReceived = true
		result.responseText = string(payload)
	case websocket.BinaryMessage:
		result.responseReceived = true
		result.isBinary = true
		result.responseBinary = payload
	}

	if c.connectionMode == "per-message" {
		_ = conn.Close()
	}
	return result, nil
}

// getConn returns a connection: reuses persistent conn or dials per-message.
func (c *WebSocketOutboundConnector) getConn(ctx context.Context) (*websocket.Conn, error) {
	if c.connectionMode == "per-message" {
		return c.dial(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.connDead {
		return c.conn, nil
	}
	conn, err := c.dialLocked(ctx)
	if err != nil {
		return nil, err
	}
	c.conn = conn
	c.connDead = false
	return conn, nil
}

// dial creates a new websocket connection.
func (c *WebSocketOutboundConnector) dial(ctx context.Context) (*websocket.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dialLocked(ctx)
}

// dialLocked must be called with c.mu held.
func (c *WebSocketOutboundConnector) dialLocked(ctx context.Context) (*websocket.Conn, error) {
	conn, _, err := c.dialer().DialContext(ctx, c.url, c.requestHeader())
	return conn, err
}

func (c *WebSocketOutboundConnector) dialer() *websocket.Dialer {
	d := &websocket.Dialer{
		HandshakeTimeout: c.connectTimeout,
	}
	if c.subProtocol != "" {
		d.Subprotocols = []string{c.subProtocol}
	}
	if c.tlsSkipVerify {
		d.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return d
}

func (c *WebSocketOutboundConnector) requestHeader() http.Header {
	h := http.Header{}
	for k, v := range c.headers {
		h.Set(k, v)
	}
	return h
}

// Close closes the persistent connection if open.
func (c *WebSocketOutboundConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	return c.BaseOutboundConnector.Close()
}
