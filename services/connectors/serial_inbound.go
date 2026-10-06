// services/connectors/serial_inbound.go
// RS232 Serial Inbound Connector — opens a physical/virtual COM port ONCE
// (like one SSH session, mirroring sftp_inbound.go's DIAL pattern — NOT
// tcp_mllp_inbound.go's LISTEN pattern, since a serial port is a single
// resource, never something to bind/accept connections on) and continuously
// plays ASTM's RECEIVER role against it via astm_framing.ReceiveMessage: the
// instrument calls (sends ENQ) whenever it has data, this connector answers.
//
// Requires the confirmed "co-located process" deployment topology — this
// connector (or the whole ezHealthKonnect process) must run on a machine
// with a real, physical connection to the instrument's COM port.
//
// Configuration:
//
//	port_name          string  COM port name, e.g. "COM3" (Windows) or "/dev/ttyUSB0" (Linux)
//	baud_rate          int     (default: 9600)
//	data_bits          int     5-8 (default: 8)
//	parity             string  "none" | "odd" | "even" | "mark" | "space" (default: "none")
//	stop_bits          string  "1" | "1.5" | "2" (default: "1")
//	checksum_severity  string  "error" | "warning" (default: "error") — see
//	                           ASTMFramingConfig's own doc comment
package connectors

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"go.bug.st/serial"
)

// SerialInboundConnector continuously reads ASTM messages off one open COM
// port.
type SerialInboundConnector struct {
	*BaseInboundConnector

	portName string
	mode     *serial.Mode
	framing  ASTMFramingConfig

	mu       sync.Mutex
	port     serial.Port
	stopping bool
}

// NewSerialInboundConnector creates a production RS232 serial inbound
// connector.
func NewSerialInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "serial_inbound",
		DisplayName:        "RS232 Serial (ASTM) Inbound",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron": false, // a continuously-open port, not an interval poll — see sftp_inbound.go's own SupportsCron=true for the contrast
		},
	}
	return &SerialInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses configuration and builds the serial port mode.
func (c *SerialInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
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

	c.SetMetadata("port_name", c.portName)
	c.SetMetadata("baud_rate", fmt.Sprintf("%d", baudRate))
	return nil
}

func parseSerialParity(v string) (serial.Parity, error) {
	switch strings.ToLower(v) {
	case "", "none":
		return serial.NoParity, nil
	case "odd":
		return serial.OddParity, nil
	case "even":
		return serial.EvenParity, nil
	case "mark":
		return serial.MarkParity, nil
	case "space":
		return serial.SpaceParity, nil
	default:
		return serial.NoParity, fmt.Errorf("unrecognized parity %q (expected none|odd|even|mark|space)", v)
	}
}

func parseSerialStopBits(v string) (serial.StopBits, error) {
	switch v {
	case "", "1":
		return serial.OneStopBit, nil
	case "1.5":
		return serial.OnePointFiveStopBits, nil
	case "2":
		return serial.TwoStopBits, nil
	default:
		return serial.OneStopBit, fmt.Errorf("unrecognized stop_bits %q (expected 1|1.5|2)", v)
	}
}

// Validate checks configuration validity.
func (c *SerialInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if c.portName == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("port_name is required"), false)
	}
	return nil
}

// TestConnection opens and immediately closes the configured port — proves
// the port exists and is free to open right now, the same "proves
// reachability, not that a real device has ever talked to it" caveat every
// other listener-style connector's own TestConnection carries (see this
// feature's own plan doc).
func (c *SerialInboundConnector) TestConnection(ctx context.Context) error {
	port, err := serial.Open(c.portName, c.mode)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return port.Close()
}

// Start opens the configured COM port once and launches a goroutine that
// continuously waits for the instrument to initiate (ENQ), answering each
// message via astm_framing.ReceiveMessage.
func (c *SerialInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	port, err := serial.Open(c.portName, c.mode)
	if err != nil {
		c.RecordError(err)
		return NewConnectorError(c.GetMetadata().TypeName, "start", err, true)
	}

	c.mu.Lock()
	c.port = port
	c.stopping = false
	c.mu.Unlock()

	c.SetState(StateRunning)
	c.SetConnected(true)
	log.Printf("📟 Serial Inbound: Listening on %s (baud=%d)", c.portName, c.mode.BaudRate)

	go c.readLoop(port, messageChan)
	return nil
}

func (c *SerialInboundConnector) readLoop(port serial.Port, messageChan chan<- *models.InboundMessage) {
	// Constructed ONCE for this port's entire lifetime, never per-call —
	// see ReceiveMessage's own doc comment for why a fresh bufio.Reader on
	// every call would silently lose bytes the instrument already sent for
	// the NEXT message (a real, found-and-fixed deadlock, not theoretical).
	bufReader := bufio.NewReader(port)
	for {
		content, err := ReceiveMessage(bufReader, port, c.framing)
		if err != nil {
			c.mu.Lock()
			stopping := c.stopping
			c.mu.Unlock()
			if stopping {
				return // Stop() closed the port on purpose — not a real error
			}
			c.RecordError(fmt.Errorf("serial_inbound: %w", err))
			log.Printf("⚠️ Serial Inbound (%s): %v — retrying", c.portName, err)
			time.Sleep(time.Second) // avoid a hot error loop if the port itself is in a bad state
			continue
		}

		msg := &models.InboundMessage{
			MessageID:      generateASTMMessageID("serial"),
			Content:        content,
			SourceType:     "serial",
			SourceEndpoint: c.portName,
			ReceivedAt:     time.Now(),
			SourceMetadata: map[string]string{
				"serial_port": c.portName,
				"baud_rate":   fmt.Sprintf("%d", c.mode.BaudRate),
			},
		}
		messageChan <- msg
		c.IncrementMessagesReceived()
	}
}

// Stop closes the serial port, which unblocks the read loop's blocked Read
// call — the same "close the real OS resource directly, don't rely on the
// base class's stopCh" convention tcp_mllp_inbound.go's own Stop() already
// establishes for a connection-oriented (not interval-poll) connector.
func (c *SerialInboundConnector) Stop() error {
	c.mu.Lock()
	c.stopping = true
	port := c.port
	c.mu.Unlock()

	if port != nil {
		if err := port.Close(); err != nil {
			log.Printf("⚠️ Serial Inbound (%s): error closing port: %v", c.portName, err)
		}
	}
	c.SetConnected(false)
	c.SetState(StateStopped)
	return nil
}

