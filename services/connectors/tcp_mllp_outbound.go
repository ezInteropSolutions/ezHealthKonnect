// services/connectors/tcp_mllp_outbound.go
// TCP/MLLP Outbound Connector — sends HL7 v2.x messages to a remote MLLP endpoint.
//
// Protocol (RFC-4346 / IHE ITI-8 Appendix C.4):
//
//	Sender wraps each message: VT + content + FS + CR
//	Receiver responds with an ACK (MSH+MSA) framed the same way.
//
// Connection modes:
//
//	persistent — single TCP connection reused for all messages (low latency)
//	per-message — new connection per message (simpler recovery, higher overhead)
package connectors

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// TCPMLLPOutboundConnector dials a remote HL7 MLLP server and sends messages.
type TCPMLLPOutboundConnector struct {
	*BaseOutboundConnector

	// config
	host             string
	port             int
	enableTLS        bool
	tlsConfig        *tls.Config
	connectionMode   string // "persistent" | "per-message"
	connectTimeout   time.Duration
	writeTimeout     time.Duration
	readTimeout      time.Duration
	expectACK        bool
	ackTimeout       time.Duration
	maxRetries       int
	retryDelayMs     int

	// persistent connection state
	mu       sync.Mutex
	conn     net.Conn
	connDead bool // set when we detect the persistent conn is broken
}

// NewTCPMLLPOutboundConnector replaces the stub in connector_stubs.go.
func NewTCPMLLPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "tcp_mllp_outbound",
		DisplayName:        "TCP/MLLP (HL7 v2.x) Client",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  false,
			"supports_tls":    true,
			"supports_auth":   false, // MLLP has no standard auth; use TLS client certs
			"supports_retry":  true,
			"validates_ack":   true,
		},
	}
	return &TCPMLLPOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration.
func (c *TCPMLLPOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.host = cfg.GetString("host")
	if c.host == "" {
		c.host = "localhost"
	}
	c.port = cfg.GetInt("port")
	if c.port == 0 {
		c.port = 2575
	}

	c.connectionMode = cfg.GetString("connection_mode")
	if c.connectionMode == "" {
		c.connectionMode = "persistent"
	}

	connectSec := cfg.GetInt("connect_timeout_seconds")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	writeSec := cfg.GetInt("write_timeout_seconds")
	if writeSec == 0 {
		writeSec = 30
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	readSec := cfg.GetInt("read_timeout_seconds")
	if readSec == 0 {
		readSec = 30
	}
	c.readTimeout = time.Duration(readSec) * time.Second

	ackTimeoutSec := cfg.GetInt("ack_timeout_seconds")
	if ackTimeoutSec == 0 {
		ackTimeoutSec = 30
	}
	c.ackTimeout = time.Duration(ackTimeoutSec) * time.Second

	c.expectACK = true
	if cfg.Has("expect_ack") {
		c.expectACK = cfg.GetBool("expect_ack")
	}

	c.maxRetries = cfg.GetInt("max_retries")
	c.retryDelayMs = cfg.GetInt("retry_delay_ms")
	if c.retryDelayMs == 0 {
		c.retryDelayMs = 500
	}

	c.enableTLS = cfg.GetBool("enable_tls")
	if c.enableTLS {
		c.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		switch cfg.GetString("tls_version") {
		case "TLS_1_3":
			c.tlsConfig.MinVersion = tls.VersionTLS13
		}
		certFile := cfg.GetString("certificate_file")
		keyFile := cfg.GetString("key_file")
		if certFile != "" && keyFile != "" {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return NewConnectorError(c.GetMetadata().TypeName, "tls_setup", err, false)
			}
			c.tlsConfig.Certificates = []tls.Certificate{cert}
		}
		skipVerify := cfg.GetBool("tls_skip_verify")
		c.tlsConfig.InsecureSkipVerify = skipVerify //nolint:gosec
	}

	c.SetMetadata("host", c.host)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("connection_mode", c.connectionMode)
	c.SetMetadata("tls_enabled", fmt.Sprintf("%t", c.enableTLS))
	return nil
}

// Validate checks configuration validity.
func (c *TCPMLLPOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	if c.host == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("host is required"), false)
	}
	if c.port <= 0 || c.port > 65535 {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("invalid port: %d", c.port), false)
	}
	if c.connectionMode != "persistent" && c.connectionMode != "per-message" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("connection_mode must be 'persistent' or 'per-message'"), false)
	}
	return nil
}

// TestConnection dials the remote endpoint without sending data.
func (c *TCPMLLPOutboundConnector) TestConnection(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	_ = conn.Close()
	return nil
}

// Send wraps the content in an MLLP frame and delivers it.
func (c *TCPMLLPOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	content := message.Content
	if content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	var (
		ackMsg string
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
		ackMsg, err = c.sendOnce(ctx, content)
		if err == nil {
			break
		}
		log.Printf("[tcp_mllp_outbound] attempt %d/%d failed: %v", attempt+1, c.maxRetries+1, err)
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

	// Parse ACK code from MSA segment (AA = Accept, AE = Error, AR = Reject)
	ackCode := parseMLLPACKCode(ackMsg)
	success := ackCode == "" || ackCode == "AA" || !c.expectACK

	result := &DeliveryResult{
		Success:        success,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: ackMsg,
		DurationMs:     time.Since(start).Milliseconds(),
		Metadata:       map[string]interface{}{"ack_code": ackCode},
	}

	if !success {
		result.ErrorMessage = fmt.Sprintf("MLLP NACK received: %s", ackCode)
	}

	return result, nil
}

// Close closes the persistent connection if open.
func (c *TCPMLLPOutboundConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	return c.BaseOutboundConnector.Close()
}

// sendOnce performs a single send attempt (dial → write → read ACK).
func (c *TCPMLLPOutboundConnector) sendOnce(ctx context.Context, content string) (string, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return "", err
	}

	// Write MLLP frame
	frame := mllpFrame(content)
	if err := conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
		return "", err
	}
	if _, err := conn.Write(frame); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}

	if !c.expectACK {
		if c.connectionMode == "per-message" {
			_ = conn.Close()
		}
		return "", nil
	}

	// Read ACK frame
	if err := conn.SetReadDeadline(time.Now().Add(c.ackTimeout)); err != nil {
		return "", err
	}
	ack, err := readMLLPFrame(conn)
	if err != nil {
		return "", fmt.Errorf("ACK read failed: %w", err)
	}

	if c.connectionMode == "per-message" {
		_ = conn.Close()
	}

	return ack, nil
}

// getConn returns a connection: reuses persistent conn or dials per-message.
func (c *TCPMLLPOutboundConnector) getConn(ctx context.Context) (net.Conn, error) {
	if c.connectionMode == "per-message" {
		return c.dial(ctx)
	}
	// Persistent mode
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.connDead {
		return c.conn, nil
	}
	// (Re)dial
	conn, err := c.dialLocked(ctx)
	if err != nil {
		return nil, err
	}
	c.conn = conn
	c.connDead = false
	return conn, nil
}

// dial creates a new TCP (or TLS) connection.
func (c *TCPMLLPOutboundConnector) dial(ctx context.Context) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dialLocked(ctx)
}

// dialLocked must be called with c.mu held.
func (c *TCPMLLPOutboundConnector) dialLocked(ctx context.Context) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	d := &net.Dialer{Timeout: c.connectTimeout}

	if c.enableTLS {
		tlsDialer := &tls.Dialer{NetDialer: d, Config: c.tlsConfig}
		return tlsDialer.DialContext(ctx, "tcp", addr)
	}
	return d.DialContext(ctx, "tcp", addr)
}

// mllpFrame wraps content in MLLP framing bytes.
func mllpFrame(content string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(MLLPStartByte)
	buf.WriteString(content)
	buf.WriteByte(MLLPEndByte1)
	buf.WriteByte(MLLPEndByte2)
	return buf.Bytes()
}

// readMLLPFrame reads one MLLP frame from a connection (VT...FS+CR).
func readMLLPFrame(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)

	// Wait for start byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading MLLP start: %w", err)
		}
		if b == MLLPStartByte {
			break
		}
	}

	// Read until FS+CR
	var payload bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading MLLP payload: %w", err)
		}
		if b == MLLPEndByte1 {
			// Consume trailing CR
			_, _ = reader.ReadByte()
			break
		}
		payload.WriteByte(b)
	}
	return payload.String(), nil
}

// parseMLLPACKCode extracts the MSA-1 acknowledgment code from an HL7 ACK message.
// Returns "AA", "AE", or "AR". Empty string means no MSA found.
// Normalises both \r and \r\n segment separators before splitting.
func parseMLLPACKCode(ackMsg string) string {
	// Normalise \r\n → \r so strings.Split(…, "\r") handles both terminators
	normalised := strings.ReplaceAll(ackMsg, "\r\n", "\r")
	for _, line := range strings.Split(normalised, "\r") {
		line = strings.TrimLeft(line, "\n") // strip any stray leading newline
		if strings.HasPrefix(line, "MSA") {
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

