// services/connectors/serial_protocols.go
// The message protocols serial_inbound.go can speak over one RS232 port.
//
// A serial port is only a byte pipe; what travels over it is decided by the
// device. Lab analyzers mostly use ASTM E1394-97 (ENQ/STX/ETX/EOT handshake);
// some point-of-care and lab devices send HL7 v2 framed exactly like MLLP over
// TCP (VT ... FS CR) and expect an HL7 ACK back on the same line. Each protocol
// is one serialProtocol implementation registered in serialProtocolFactories
// — adding a protocol is one new type plus one registry row, with no change to
// the connector's port handling.
//
// Decision record (2026-10-19): HL7 over RS232 was added because a prospect's
// ICU equipment advertises "HL7, LAN and RS232", and HL7 v2 over serial is a
// real (if uncommon, mostly older point-of-care / lab) deployment pattern.
// Most modern devices — and nearly all ventilators — reach HL7 through the
// network or a vendor gateway, so HL7 over TCP/IP stays the recommended path.
package connectors

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"ezhealthkonnect/models"
)

// serialProtocol receives exactly one message exchange from an open port.
// It returns delivered=true when a message was handed to out. A returned
// error is reported by the caller, which keeps reading.
type serialProtocol interface {
	// Name is the config value that selects this protocol.
	Name() string
	ReceiveOne(r *bufio.Reader, w io.Writer, out chan<- *models.InboundMessage) (delivered bool, err error)
}

// serialPortInfo is what a protocol needs to label the messages it produces.
type serialPortInfo struct {
	PortName string
	BaudRate int
}

// serialProtocolFactory builds a protocol from the connector's config.
type serialProtocolFactory func(cfg *ConnectorConfig, port serialPortInfo) (serialProtocol, error)

// serialProtocolFactories is the registry of supported serial protocols,
// keyed by the "protocol" config value.
var serialProtocolFactories = map[string]serialProtocolFactory{
	"astm":     newASTMSerialProtocol,
	"hl7_mllp": newHL7MLLPSerialProtocol,
}

// defaultSerialProtocol keeps every existing serial interface (configured
// before the protocol setting existed) on ASTM.
const defaultSerialProtocol = "astm"

// newSerialProtocol resolves the configured protocol name.
func newSerialProtocol(name string, cfg *ConnectorConfig, port serialPortInfo) (serialProtocol, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = defaultSerialProtocol
	}
	factory, ok := serialProtocolFactories[key]
	if !ok {
		return nil, fmt.Errorf("unrecognized protocol %q (expected %s)", name, strings.Join(supportedSerialProtocols(), "|"))
	}
	return factory(cfg, port)
}

func supportedSerialProtocols() []string {
	names := make([]string, 0, len(serialProtocolFactories))
	for n := range serialProtocolFactories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ─── ASTM E1394-97 ──────────────────────────────────────────────────────────

type astmSerialProtocol struct {
	framing ASTMFramingConfig
	port    serialPortInfo
}

func newASTMSerialProtocol(cfg *ConnectorConfig, port serialPortInfo) (serialProtocol, error) {
	return &astmSerialProtocol{
		framing: ASTMFramingConfig{ChecksumSeverity: cfg.GetString("checksum_severity")},
		port:    port,
	}, nil
}

func (p *astmSerialProtocol) Name() string { return "astm" }

func (p *astmSerialProtocol) ReceiveOne(r *bufio.Reader, w io.Writer, out chan<- *models.InboundMessage) (bool, error) {
	content, err := ReceiveMessage(r, w, p.framing)
	if err != nil {
		return false, err
	}
	out <- &models.InboundMessage{
		MessageID:      generateASTMMessageID("serial"),
		Content:        content,
		SourceType:     "serial",
		SourceEndpoint: p.port.PortName,
		ReceivedAt:     time.Now(),
		SourceMetadata: map[string]string{
			"serial_port":     p.port.PortName,
			"baud_rate":       fmt.Sprintf("%d", p.port.BaudRate),
			"serial_protocol": "astm",
		},
	}
	return true, nil
}

// ─── HL7 v2 with MLLP framing ───────────────────────────────────────────────

// hl7SerialEnqueueTimeout matches the TCP listener: how long a message waits
// for room in the processing queue before the device is told AE (retry).
const hl7SerialEnqueueTimeout = 5 * time.Second

type hl7MLLPSerialProtocol struct {
	ack            *HL7Acknowledger
	maxBytes       int
	port           serialPortInfo
	enqueueTimeout time.Duration
}

func newHL7MLLPSerialProtocol(cfg *ConnectorConfig, port serialPortInfo) (serialProtocol, error) {
	maxBytes := defaultMaxMLLPMessageBytes
	if mb := cfg.GetInt("max_message_size_mb"); mb > 0 {
		maxBytes = mb * 1024 * 1024
		if maxBytes > hardCapMaxMLLPMessageBytes {
			maxBytes = hardCapMaxMLLPMessageBytes
		}
	}
	return &hl7MLLPSerialProtocol{
		ack:            NewHL7Acknowledger(parseACKConfig(cfg.GetMap("ack"))),
		maxBytes:       maxBytes,
		port:           port,
		enqueueTimeout: hl7SerialEnqueueTimeout,
	}, nil
}

func (p *hl7MLLPSerialProtocol) Name() string { return "hl7_mllp" }

// ReceiveOne reads one MLLP frame and always answers it with an HL7 ACK on
// the same line: AR for a frame that is not HL7 v2, AA once queued, AE when
// the queue stays full (or AA with on_error "suppress", which drops it).
// Noise before the start byte is skipped — serial lines produce it.
func (p *hl7MLLPSerialProtocol) ReceiveOne(r *bufio.Reader, w io.Writer, out chan<- *models.InboundMessage) (bool, error) {
	frame, err := ReadMLLPFrameFrom(r, p.maxBytes, true)
	if err != nil {
		if errors.Is(err, errMLLPFrameTooLarge) {
			p.reply(w, "", "AE", "Message exceeds maximum allowed size")
		}
		return false, err
	}
	if strings.TrimSpace(frame) == "" {
		return false, nil
	}
	if !isHL7v2Message(frame) {
		log.Printf("🚫 Serial Inbound (%s): rejected non-HL7 frame (%d bytes) — no MSH segment", p.port.PortName, len(frame))
		p.reply(w, frame, "AR", "Message rejected: not a valid HL7 v2 message (missing MSH segment)")
		return false, nil
	}

	msg := &models.InboundMessage{
		MessageID:      generateASTMMessageID("serial"),
		CorrelationID:  HL7MessageControlID(frame),
		Content:        frame,
		SourceType:     "serial",
		SourceEndpoint: p.port.PortName,
		ReceivedAt:     time.Now(),
		MessageType:    HL7MessageType(frame),
		MessageSize:    len(frame),
		Encoding:       "HL7",
		SourceMetadata: map[string]string{
			"serial_port":     p.port.PortName,
			"baud_rate":       fmt.Sprintf("%d", p.port.BaudRate),
			"serial_protocol": "hl7_mllp",
		},
	}

	select {
	case out <- msg:
		p.reply(w, frame, "AA", p.ack.Config.TextSuccess)
		return true, nil
	case <-time.After(p.enqueueTimeout):
		if p.ack.Config.OnError == "suppress" {
			log.Printf("⚠️ Serial Inbound (%s): queue full — message DROPPED, AA sent (on_error=suppress)", p.port.PortName)
			p.reply(w, frame, "AA", p.ack.Config.TextSuccess)
		} else {
			log.Printf("⚠️ Serial Inbound (%s): queue full — AE sent so the device resends", p.port.PortName)
			p.reply(w, frame, "AE", p.ack.Config.TextError)
		}
		return false, nil
	}
}

func (p *hl7MLLPSerialProtocol) reply(w io.Writer, original, code, text string) {
	ack, sent, scriptErr := p.ack.Reply(original, code, text)
	if scriptErr != nil {
		log.Printf("⚠️ Serial Inbound (%s): ACK script error: %v — using default", p.port.PortName, scriptErr)
	}
	if err := WriteMLLPFrameTo(w, ack); err != nil {
		log.Printf("⚠️ Serial Inbound (%s): failed to send ACK (%s): %v", p.port.PortName, sent, err)
	}
}
