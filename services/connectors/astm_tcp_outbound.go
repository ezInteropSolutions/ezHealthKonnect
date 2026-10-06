// services/connectors/astm_tcp_outbound.go
// ASTM over TCP/IP Outbound Connector — dials a remote ASTM receiver and
// plays ASTM's initiator role via astm_framing.SendMessage, reusing Phase
// A's own handshake unchanged over a real net.Conn instead of a serial
// port's io.ReadWriteCloser. Mirrors serial_outbound.go's own "open fresh
// per Send() call, no persistent connection" simplicity — unlike
// astm_tcp_inbound.go, there's no initiator-role ambiguity here: an
// OUTBOUND connector pushing data to a remote receiver is, by definition,
// the one that initiates (sends ENQ first) — the same unambiguous role
// serial_outbound.go already plays.
//
// Configuration:
//
//	host               string  Remote hostname or IP (required)
//	port               int     Remote port (required)
//	checksum_severity  string  "error" | "warning" (default: "error")
//	connect_timeout_sec int    (default: 10)
//	write_timeout_sec   int    Overall Send() deadline (default: 30)
package connectors

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"ezhealthkonnect/models"
)

// ASTMTCPOutboundConnector sends one ASTM message per Send() call over a
// freshly-dialed TCP connection.
type ASTMTCPOutboundConnector struct {
	*BaseOutboundConnector

	host           string
	port           int
	framing        ASTMFramingConfig
	connectTimeout time.Duration
	writeTimeout   time.Duration
}

// NewASTMTCPOutboundConnector creates a production ASTM-over-TCP/IP outbound
// connector.
func NewASTMTCPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "astm_tcp_outbound",
		DisplayName:        "ASTM over TCP/IP Outbound",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities:       map[string]bool{},
	}
	return &ASTMTCPOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration.
func (c *ASTMTCPOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.host = cfg.GetString("host")
	c.port = cfg.GetInt("port")

	c.framing = ASTMFramingConfig{
		ChecksumSeverity: cfg.GetString("checksum_severity"),
	}

	connectSec := cfg.GetInt("connect_timeout_sec")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	writeSec := cfg.GetInt("write_timeout_sec")
	if writeSec == 0 {
		writeSec = 30
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	c.SetMetadata("host", c.host)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	return nil
}

// Validate checks configuration validity.
func (c *ASTMTCPOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	if c.host == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("host is required"), false)
	}
	if c.port == 0 {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("port is required"), false)
	}
	return nil
}

// TestConnection dials and immediately closes a connection to the
// configured host:port.
func (c *ASTMTCPOutboundConnector) TestConnection(ctx context.Context) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.host, c.port), c.connectTimeout)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return conn.Close()
}

// Send dials the configured host:port, plays ASTM's initiator role to
// deliver message.Content, then closes the connection.
func (c *ASTMTCPOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.host, c.port), c.connectTimeout)
	if err != nil {
		c.RecordError(err)
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: err.Error(),
			DurationMs:   time.Since(start).Milliseconds(),
		}, NewConnectorError(c.GetMetadata().TypeName, "send", err, true)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(c.writeTimeout))

	if err := SendMessage(bufio.NewReader(conn), conn, message.Content, c.framing); err != nil {
		c.RecordError(err)
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: err.Error(),
			DurationMs:   time.Since(start).Milliseconds(),
		}, NewConnectorError(c.GetMetadata().TypeName, "send", err, true)
	}

	c.IncrementMessagesSent()
	durationMs := time.Since(start).Milliseconds()
	log.Printf("✅ ASTM/TCP Outbound (%s:%d): delivered message %s (%d bytes) in %dms", c.host, c.port, message.MessageID, len(message.Content), durationMs)

	return &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: durationMs,
	}, nil
}
