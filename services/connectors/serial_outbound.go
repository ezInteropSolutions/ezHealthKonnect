// services/connectors/serial_outbound.go
// RS232 Serial Outbound Connector — opens the configured COM port fresh for
// each Send() call and plays ASTM's INITIATOR role via astm_framing.SendMessage.
// Deliberately NOT a persistent, reused connection (unlike
// websocket_outbound.go's own precedent) — a host pushing data DOWN to an
// instrument (orders, a host-query reply built outside the inbound
// connection that originally asked) is comparatively low-volume, so the
// simplicity of open/send/close avoids persistent-connection-state
// complexity (reconnect-on-drop, concurrent-access guarding) this phase
// doesn't need yet.
//
// Configuration: identical shape to serial_inbound.go's own port/baud/data/
// parity/stop-bits fields, plus:
//
//	write_timeout_sec  int  Overall Send() deadline (default: 30)
package connectors

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"

	"go.bug.st/serial"
)

// SerialOutboundConnector sends one ASTM message per Send() call over a
// freshly-opened COM port.
type SerialOutboundConnector struct {
	*BaseOutboundConnector

	portName      string
	mode          *serial.Mode
	framing       ASTMFramingConfig
	writeTimeout  time.Duration
}

// NewSerialOutboundConnector creates a production RS232 serial outbound
// connector.
func NewSerialOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "serial_outbound",
		DisplayName:        "RS232 Serial (ASTM) Outbound",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities:       map[string]bool{},
	}
	return &SerialOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration and builds the serial port mode.
func (c *SerialOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.portName = cfg.GetString("port_name")

	baudRate := cfg.GetInt("baud_rate")
	if baudRate == 0 {
		baudRate = 9600
	}
	dataBits := cfg.GetInt("data_bits")
	if dataBits == 0 {
		dataBits = 8
	}

	parity, err := parseSerialParity(cfg.GetString("parity"))
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize", err, false)
	}
	stopBits, err := parseSerialStopBits(cfg.GetString("stop_bits"))
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize", err, false)
	}

	c.mode = &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
	}

	c.framing = ASTMFramingConfig{
		ChecksumSeverity: cfg.GetString("checksum_severity"),
	}

	writeSec := cfg.GetInt("write_timeout_sec")
	if writeSec == 0 {
		writeSec = 30
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	c.SetMetadata("port_name", c.portName)
	c.SetMetadata("baud_rate", fmt.Sprintf("%d", baudRate))
	return nil
}

// Validate checks configuration validity.
func (c *SerialOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	if c.portName == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("port_name is required"), false)
	}
	return nil
}

// TestConnection opens and immediately closes the configured port.
func (c *SerialOutboundConnector) TestConnection(ctx context.Context) error {
	port, err := serial.Open(c.portName, c.mode)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return port.Close()
}

// Send opens the configured COM port, plays ASTM's initiator role to
// deliver message.Content, then closes the port.
func (c *SerialOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()

	port, err := serial.Open(c.portName, c.mode)
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
	defer port.Close()

	if err := port.SetReadTimeout(c.writeTimeout); err != nil {
		log.Printf("⚠️ Serial Outbound (%s): could not set read timeout: %v", c.portName, err)
	}

	if err := SendMessage(bufio.NewReader(port), port, message.Content, c.framing); err != nil {
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
	log.Printf("✅ Serial Outbound (%s): delivered message %s (%d bytes) in %dms", c.portName, message.MessageID, len(message.Content), durationMs)

	return &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: durationMs,
	}, nil
}
