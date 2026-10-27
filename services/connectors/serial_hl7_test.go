package connectors

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

const serialTestORU = "MSH|^~\\&|VENT|ICU|EHK|HOSP|20261019120000||ORU^R01|CTRL123|P|2.6\rPID|||12345\rOBX|1|NM|RR^Resp Rate||18|/min\r"

// serialHL7Harness drives an hl7MLLPSerialProtocol over net.Pipe, which is a
// synchronous in-memory duplex stream — the same io.Reader/io.Writer shape a
// real serial.Port presents.
type serialHL7Harness struct {
	t        *testing.T
	protocol *hl7MLLPSerialProtocol
	device   net.Conn
	devRead  *bufio.Reader
	port     net.Conn
	portRead *bufio.Reader
}

func newSerialHL7Harness(t *testing.T, ack map[string]interface{}) *serialHL7Harness {
	t.Helper()
	cfgMap := map[string]interface{}{"protocol": "hl7_mllp"}
	if ack != nil {
		cfgMap["ack"] = ack
	}
	cfg := &ConnectorConfig{Config: cfgMap}
	p, err := newSerialProtocol("hl7_mllp", cfg, serialPortInfo{PortName: "COM9", BaudRate: 9600})
	if err != nil {
		t.Fatalf("newSerialProtocol: %v", err)
	}
	device, port := net.Pipe()
	t.Cleanup(func() { device.Close(); port.Close() })
	return &serialHL7Harness{
		t: t, protocol: p.(*hl7MLLPSerialProtocol),
		device: device, devRead: bufio.NewReader(device),
		port: port, portRead: bufio.NewReader(port),
	}
}

type receiveResult struct {
	delivered bool
	err       error
}

// exchange sends raw bytes from the device side and returns the ACK the
// connector wrote back plus ReceiveOne's own result.
func (h *serialHL7Harness) exchange(raw []byte, out chan *models.InboundMessage) (string, receiveResult) {
	h.t.Helper()
	done := make(chan receiveResult, 1)
	go func() {
		d, err := h.protocol.ReceiveOne(h.portRead, h.port, out)
		done <- receiveResult{d, err}
	}()
	go func() { _, _ = h.device.Write(raw) }()

	_ = h.device.SetReadDeadline(time.Now().Add(10 * time.Second))
	ack, err := ReadMLLPFrameFrom(h.devRead, 0, false)
	if err != nil {
		h.t.Fatalf("reading ACK from connector: %v", err)
	}
	select {
	case r := <-done:
		return ack, r
	case <-time.After(10 * time.Second):
		h.t.Fatal("ReceiveOne did not return")
		return "", receiveResult{}
	}
}

func msaCode(ack string) string { return parseMLLPACKCode(ack) }

func TestSerialHL7_ValidMessage_QueuedAndAA(t *testing.T) {
	h := newSerialHL7Harness(t, nil)
	out := make(chan *models.InboundMessage, 1)

	ack, res := h.exchange(mllpFrame(serialTestORU), out)
	if res.err != nil || !res.delivered {
		t.Fatalf("expected delivered without error, got %+v", res)
	}
	if code := msaCode(ack); code != "AA" {
		t.Fatalf("ACK code = %q, want AA (ack=%q)", code, ack)
	}
	if !strings.Contains(ack, "|CTRL123|") {
		t.Errorf("ACK does not echo MSH-10 control id: %q", ack)
	}
	msg := <-out
	if msg.Content != serialTestORU || msg.MessageType != "ORU^R01" || msg.CorrelationID != "CTRL123" {
		t.Errorf("unexpected message: type=%q corr=%q", msg.MessageType, msg.CorrelationID)
	}
	if msg.SourceType != "serial" || msg.SourceEndpoint != "COM9" || msg.Encoding != "HL7" {
		t.Errorf("unexpected source fields: %+v", msg)
	}
	if msg.SourceMetadata["serial_protocol"] != "hl7_mllp" {
		t.Errorf("serial_protocol metadata = %q", msg.SourceMetadata["serial_protocol"])
	}
}

func TestSerialHL7_LineNoiseBeforeFrame_IsSkipped(t *testing.T) {
	h := newSerialHL7Harness(t, nil)
	out := make(chan *models.InboundMessage, 1)
	raw := append([]byte{0x00, 0xFF, '\r', '\n'}, mllpFrame(serialTestORU)...)

	ack, res := h.exchange(raw, out)
	if !res.delivered || msaCode(ack) != "AA" {
		t.Fatalf("noise before the start byte must be skipped; got delivered=%v ack=%q", res.delivered, ack)
	}
}

func TestSerialHL7_NonHL7Frame_RejectedWithAR_NotQueued(t *testing.T) {
	h := newSerialHL7Harness(t, nil)
	out := make(chan *models.InboundMessage, 1)

	ack, res := h.exchange(mllpFrame("GARBAGE|not|hl7\r"), out)
	if res.delivered || res.err != nil {
		t.Fatalf("non-HL7 frame must not be delivered: %+v", res)
	}
	if code := msaCode(ack); code != "AR" {
		t.Fatalf("ACK code = %q, want AR", code)
	}
	if len(out) != 0 {
		t.Fatal("non-HL7 frame was queued")
	}
}

func TestSerialHL7_QueueFull_DefaultSendsAE(t *testing.T) {
	h := newSerialHL7Harness(t, nil)
	h.protocol.enqueueTimeout = 50 * time.Millisecond
	full := make(chan *models.InboundMessage) // unbuffered, nobody reading

	ack, res := h.exchange(mllpFrame(serialTestORU), full)
	if res.delivered {
		t.Fatal("message reported delivered while the queue was full")
	}
	if code := msaCode(ack); code != "AE" {
		t.Fatalf("ACK code = %q, want AE (device must resend)", code)
	}
}

func TestSerialHL7_QueueFull_SuppressSendsAA(t *testing.T) {
	h := newSerialHL7Harness(t, map[string]interface{}{"on_error": "suppress"})
	h.protocol.enqueueTimeout = 50 * time.Millisecond
	full := make(chan *models.InboundMessage)

	ack, res := h.exchange(mllpFrame(serialTestORU), full)
	if res.delivered {
		t.Fatal("suppress mode still drops the message — it must not report delivered")
	}
	if code := msaCode(ack); code != "AA" {
		t.Fatalf("ACK code = %q, want AA in explicit suppress mode", code)
	}
}

func TestSerialHL7_SequentialMessagesOnOnePort(t *testing.T) {
	h := newSerialHL7Harness(t, nil)
	out := make(chan *models.InboundMessage, 3)
	for i := 0; i < 3; i++ {
		ack, res := h.exchange(mllpFrame(serialTestORU), out)
		if !res.delivered || msaCode(ack) != "AA" {
			t.Fatalf("message %d: delivered=%v ack=%q", i+1, res.delivered, ack)
		}
	}
	if len(out) != 3 {
		t.Fatalf("queued %d messages, want 3", len(out))
	}
}

func TestSerialHL7_ConfiguredSenderIdentityAndScript(t *testing.T) {
	h := newSerialHL7Harness(t, map[string]interface{}{
		"sending_app":      "ICUGW",
		"sending_facility": "WARD7",
		"script":           "function buildACK(msg) { return { ackCode: 'AE', textMessage: 'type ' + msg.messageType }; }",
	})
	out := make(chan *models.InboundMessage, 1)

	ack, _ := h.exchange(mllpFrame(serialTestORU), out)
	if !strings.HasPrefix(ack, "MSH|^~\\&|ICUGW|WARD7|") {
		t.Errorf("sender identity not applied: %q", ack)
	}
	if msaCode(ack) != "AE" || !strings.Contains(ack, "type ORU^R01") {
		t.Errorf("script override not applied: %q", ack)
	}
}

func TestNewSerialProtocol_Registry(t *testing.T) {
	cfg := &ConnectorConfig{Config: map[string]interface{}{}}
	for _, tc := range []struct{ in, want string }{{"", "astm"}, {"astm", "astm"}, {"HL7_MLLP", "hl7_mllp"}} {
		p, err := newSerialProtocol(tc.in, cfg, serialPortInfo{})
		if err != nil || p.Name() != tc.want {
			t.Errorf("newSerialProtocol(%q) = %v, %v; want %s", tc.in, p, err, tc.want)
		}
	}
	if _, err := newSerialProtocol("hl7_raw", cfg, serialPortInfo{}); err == nil {
		t.Error("unknown protocol must be rejected")
	}
}

func TestSerialInbound_Initialize_ProtocolSelection(t *testing.T) {
	c := NewSerialInboundConnector().(*SerialInboundConnector)
	if err := c.Initialize([]byte(`{"port_name":"COM3","protocol":"hl7_mllp"}`)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.protocol.Name() != "hl7_mllp" {
		t.Errorf("protocol = %s, want hl7_mllp", c.protocol.Name())
	}

	legacy := NewSerialInboundConnector().(*SerialInboundConnector)
	if err := legacy.Initialize([]byte(`{"port_name":"COM3"}`)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if legacy.protocol.Name() != "astm" {
		t.Errorf("a config without protocol must stay ASTM, got %s", legacy.protocol.Name())
	}

	bad := NewSerialInboundConnector().(*SerialInboundConnector)
	if err := bad.Initialize([]byte(`{"port_name":"COM3","protocol":"nope"}`)); err == nil {
		t.Error("Initialize must reject an unknown protocol")
	}
}

func TestReadMLLPFrameFrom_Behaviors(t *testing.T) {
	// Without preamble skipping, a stray first byte is a framing error (TCP behavior).
	_, err := ReadMLLPFrameFrom(bufio.NewReader(bytes.NewReader(append([]byte{'x'}, mllpFrame("MSH|a")...))), 0, false)
	if err == nil {
		t.Error("expected a framing error without preamble skipping")
	}
	// FS not followed by CR stays in the payload.
	raw := []byte{MLLPStartByte, 'a', MLLPEndByte1, 'b', MLLPEndByte1, MLLPEndByte2}
	got, err := ReadMLLPFrameFrom(bufio.NewReader(bytes.NewReader(raw)), 0, false)
	if err != nil || got != "a\x1cb" {
		t.Errorf("got %q, %v", got, err)
	}
	// Size cap.
	big := mllpFrame(strings.Repeat("A", 64))
	if _, err := ReadMLLPFrameFrom(bufio.NewReader(bytes.NewReader(big)), 16, true); !errors.Is(err, errMLLPFrameTooLarge) {
		t.Errorf("expected errMLLPFrameTooLarge, got %v", err)
	}
}

func TestHL7Helpers_HandleCRLFTerminators(t *testing.T) {
	m := "MSH|^~\\&|A|B|C|D|20260101||ADT^A01|X9|P|2.5\r\nPID|1\r\n"
	if HL7MessageType(m) != "ADT^A01" || HL7MessageControlID(m) != "X9" {
		t.Errorf("type=%q control=%q", HL7MessageType(m), HL7MessageControlID(m))
	}
	if HL7MessageType("PID|1") != "UNKNOWN" {
		t.Error("non-MSH first segment must report UNKNOWN")
	}
}
