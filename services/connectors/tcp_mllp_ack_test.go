// services/connectors/tcp_mllp_ack_test.go
// Comprehensive ACK/NACK tests for TCPMLLPInboundConnector.
//
// Test Groups:
//   TC-GO-001..005  getStringFromMap helper
//   TC-GO-006..013  generateACKMessage correctness (MSH/MSA structure)
//   TC-GO-014..024  runACKScript (valid, error, fallback scenarios)
//   TC-GO-025..026  ACKConfig defaults via Initialize
//   TC-GO-027..035  Integration — real TCP listener, full MLLP round-trip
//
// Run:
//   go test ./services/connectors/ -v -run TestACK
//   go test ./services/connectors/ -v -run TestACKIntegration  (requires open ports)

package connectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers shared across ACK tests
// ─────────────────────────────────────────────────────────────────────────────

// buildMLLPFrame wraps raw HL7 text in MLLP framing bytes.
func buildMLLPFrame(hl7 string) []byte {
	return append([]byte{MLLPStartByte}, append([]byte(hl7), MLLPEndByte1, MLLPEndByte2)...)
}

// readMLLPResponse reads one MLLP frame from conn (with a short deadline).
func readMLLPResponse(t *testing.T, conn net.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	msg, err := readMLLPFrame(reader)
	if err != nil {
		t.Fatalf("failed to read MLLP ACK: %v", err)
	}
	return msg
}

// newConnectorOnPort creates and initialises a TCPMLLPInboundConnector on the
// given port with the supplied extra config fields merged into defaults.
func newConnectorOnPort(t *testing.T, port int, extra map[string]interface{}) *TCPMLLPInboundConnector {
	t.Helper()
	cfg := map[string]interface{}{
		"port": port,
	}
	for k, v := range extra {
		cfg[k] = v
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	c := NewTCPMLLPInboundConnector().(*TCPMLLPInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// startConnector starts the connector in a goroutine and returns the message
// channel plus a stop function.
func startConnector(t *testing.T, c *TCPMLLPInboundConnector) (chan *models.InboundMessage, func()) {
	t.Helper()
	msgChan := make(chan *models.InboundMessage, 100)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { c.Start(ctx, msgChan) }()
	time.Sleep(120 * time.Millisecond) // wait for listener to bind
	stop := func() {
		cancel()
		c.Stop()
	}
	return msgChan, stop
}

// sampleHL7 returns a minimal valid HL7 ADT^A01 message with the given control ID.
func sampleHL7(controlID, msgType string) string {
	if msgType == "" {
		msgType = "ADT^A01"
	}
	ts := time.Now().Format("20060102150405")
	return fmt.Sprintf(
		"MSH|^~\\&|SENDER|SFAC|RECV|RFAC|%s||%s|%s|P|2.5\r\nPID|||MRN001^^^MRN||Doe^Jane||19900101|F\r\n",
		ts, msgType, controlID,
	)
}

// parseACKSegments splits a raw ACK string into MSH and MSA field slices.
// HL7 segments are delimited by CRLF (\r\n). Bare \r is also accepted for
// robustness when receiving from third-party systems.
// After splitting by |:
//   MSH: [0]=MSH [1]=encoding [2]=SendingApp [3]=SendingFacility [4]=RecvApp [5]=RecvFac ...
//   MSA: [0]=MSA [1]=AckCode  [2]=ControlID  [3]=TextMessage
func parseACKSegments(ack string) (msh []string, msa []string) {
	// Normalise bare CR to CRLF, then split on CRLF
	ack = strings.ReplaceAll(ack, "\r\n", "\r")
	ack = strings.ReplaceAll(ack, "\r", "\r\n")
	lines := strings.Split(ack, "\r\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MSH") {
			msh = strings.Split(line, "|")
		} else if strings.HasPrefix(line, "MSA") {
			msa = strings.Split(line, "|")
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-001..005  getStringFromMap
// ─────────────────────────────────────────────────────────────────────────────

func TestACK_GetStringFromMap_NilMapReturnsDefault(t *testing.T) {
	// TC-GO-001
	got := getStringFromMap(nil, "mode", "immediate")
	if got != "immediate" {
		t.Errorf("expected 'immediate', got %q", got)
	}
}

func TestACK_GetStringFromMap_MissingKeyReturnsDefault(t *testing.T) {
	// TC-GO-002
	m := map[string]interface{}{"other": "value"}
	got := getStringFromMap(m, "mode", "immediate")
	if got != "immediate" {
		t.Errorf("expected 'immediate', got %q", got)
	}
}

func TestACK_GetStringFromMap_EmptyStringReturnsDefault(t *testing.T) {
	// TC-GO-003: empty string is treated as "not set", returns default
	m := map[string]interface{}{"mode": ""}
	got := getStringFromMap(m, "mode", "immediate")
	if got != "immediate" {
		t.Errorf("expected 'immediate', got %q", got)
	}
}

func TestACK_GetStringFromMap_ValidValueReturned(t *testing.T) {
	// TC-GO-004: getStringFromMap returns the stored value verbatim.
	// Note: "none" is no longer a valid ack mode; Initialize() rejects it at a higher level.
	m := map[string]interface{}{"mode": "immediate"}
	got := getStringFromMap(m, "mode", "fallback")
	if got != "immediate" {
		t.Errorf("expected 'immediate', got %q", got)
	}
}

func TestACK_GetStringFromMap_NonStringTypeReturnsDefault(t *testing.T) {
	// TC-GO-005: integer value for a string field → fallback
	m := map[string]interface{}{"mode": 42}
	got := getStringFromMap(m, "mode", "immediate")
	if got != "immediate" {
		t.Errorf("expected 'immediate', got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-006..013  generateACKMessage
// ─────────────────────────────────────────────────────────────────────────────

func newConnectorWithACK(t *testing.T, ack ACKConfig) *TCPMLLPInboundConnector {
	t.Helper()
	c := &TCPMLLPInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{TypeName: "tcp_mllp_inbound"}),
		ackConfig:            ack,
	}
	return c
}

func TestACK_GenerateACKMessage_DefaultSenderIdentity(t *testing.T) {
	// TC-GO-006: empty ackConfig → MSH-3=ezHealthKonnect, MSH-4=EHK
	// ACK MSH format: MSH|^~\&|sendingApp|sendingFacility|RecvApp|RecvFac|ts||ACK|ctrl|P|2.5
	//   split by |: [0]=MSH [1]=encoding [2]=sendingApp [3]=sendingFacility ...
	c := newConnectorWithACK(t, ACKConfig{})
	hl7 := sampleHL7("CTRL001", "")
	ack := c.generateACKMessage(hl7, "AA", "OK")

	msh, _ := parseACKSegments(ack)
	if len(msh) < 4 {
		t.Fatalf("MSH too short: %v | raw ACK: %q", msh, ack)
	}
	if msh[2] != "ezHealthKonnect" {
		t.Errorf("MSH-3 (index 2) expected 'ezHealthKonnect', got %q", msh[2])
	}
	if msh[3] != "EHK" {
		t.Errorf("MSH-4 (index 3) expected 'EHK', got %q", msh[3])
	}
}

func TestACK_GenerateACKMessage_CustomSendingApp(t *testing.T) {
	// TC-GO-007
	c := newConnectorWithACK(t, ACKConfig{SendingApp: "MySystem"})
	ack := c.generateACKMessage(sampleHL7("CTRL002", ""), "AA", "OK")
	msh, _ := parseACKSegments(ack)
	if len(msh) < 3 {
		t.Fatalf("MSH too short: %v | raw ACK: %q", msh, ack)
	}
	if msh[2] != "MySystem" {
		t.Errorf("MSH-3 (index 2) expected 'MySystem', got %q", msh[2])
	}
}

func TestACK_GenerateACKMessage_CustomSendingFacility(t *testing.T) {
	// TC-GO-008
	c := newConnectorWithACK(t, ACKConfig{SendingFacility: "HOSPITAL_A"})
	ack := c.generateACKMessage(sampleHL7("CTRL003", ""), "AA", "OK")
	msh, _ := parseACKSegments(ack)
	if len(msh) < 4 {
		t.Fatalf("MSH too short: %v | raw ACK: %q", msh, ack)
	}
	if msh[3] != "HOSPITAL_A" {
		t.Errorf("MSH-4 (index 3) expected 'HOSPITAL_A', got %q", msh[3])
	}
}

func TestACK_GenerateACKMessage_ControlIDEchoedInMSA2(t *testing.T) {
	// TC-GO-009: MSH-10 from original → MSA-2 in ACK
	c := newConnectorWithACK(t, ACKConfig{})
	hl7 := sampleHL7("MYCTRL999", "")
	ack := c.generateACKMessage(hl7, "AA", "OK")
	_, msa := parseACKSegments(ack)
	if len(msa) < 3 {
		t.Fatalf("MSA too short: %v", msa)
	}
	if msa[2] != "MYCTRL999" {
		t.Errorf("MSA-2 expected 'MYCTRL999', got %q", msa[2])
	}
}

func TestACK_GenerateACKMessage_AACodeInMSA1(t *testing.T) {
	// TC-GO-010
	c := newConnectorWithACK(t, ACKConfig{})
	ack := c.generateACKMessage(sampleHL7("C1", ""), "AA", "Accepted")
	_, msa := parseACKSegments(ack)
	if msa[1] != "AA" {
		t.Errorf("MSA-1 expected 'AA', got %q", msa[1])
	}
}

func TestACK_GenerateACKMessage_AECodeInMSA1(t *testing.T) {
	// TC-GO-011
	c := newConnectorWithACK(t, ACKConfig{})
	ack := c.generateACKMessage(sampleHL7("C2", ""), "AE", "Error")
	_, msa := parseACKSegments(ack)
	if msa[1] != "AE" {
		t.Errorf("MSA-1 expected 'AE', got %q", msa[1])
	}
}

func TestACK_GenerateACKMessage_ARCodeInMSA1(t *testing.T) {
	// TC-GO-012
	c := newConnectorWithACK(t, ACKConfig{})
	ack := c.generateACKMessage(sampleHL7("C3", ""), "AR", "Rejected")
	_, msa := parseACKSegments(ack)
	if msa[1] != "AR" {
		t.Errorf("MSA-1 expected 'AR', got %q", msa[1])
	}
}

func TestACK_GenerateACKMessage_CustomTextInMSA3(t *testing.T) {
	// TC-GO-013
	c := newConnectorWithACK(t, ACKConfig{})
	ack := c.generateACKMessage(sampleHL7("C4", ""), "AA", "Routing to Epic ADT feed")
	_, msa := parseACKSegments(ack)
	if len(msa) < 4 {
		t.Fatalf("MSA too short: %v", msa)
	}
	if msa[3] != "Routing to Epic ADT feed" {
		t.Errorf("MSA-3 expected custom text, got %q", msa[3])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-014..024  runACKScript
// ─────────────────────────────────────────────────────────────────────────────

func TestACK_Script_ValidAAScript(t *testing.T) {
	// TC-GO-014: well-formed script returning AA
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) { return { ackCode: "AA", textMessage: "Accepted" }; }`,
	})
	code, text, err := c.runACKScript(sampleHL7("SC001", "ADT^A01"), "AA", "default text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "AA" {
		t.Errorf("expected 'AA', got %q", code)
	}
	if text != "Accepted" {
		t.Errorf("expected 'Accepted', got %q", text)
	}
}

func TestACK_Script_ConditionalOnMessageType_SendsAE(t *testing.T) {
	// TC-GO-015: script inspects messageType to return AE for ORU^R01
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			if (msg.messageType === "ORU^R01") {
				return { ackCode: "AE", textMessage: "Lab results not accepted" };
			}
			return { ackCode: "AA", textMessage: "OK" };
		}`,
	})
	code, text, err := c.runACKScript(sampleHL7("SC002", "ORU^R01"), "AA", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "AE" {
		t.Errorf("expected 'AE', got %q", code)
	}
	if text != "Lab results not accepted" {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestACK_Script_UnknownMessageTypeReturnsAR(t *testing.T) {
	// TC-GO-016: script rejects unknown message types
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			var allowed = ["ADT^A01", "ADT^A08", "ORU^R01"];
			for (var i = 0; i < allowed.length; i++) {
				if (allowed[i] === msg.messageType) return { ackCode: "AA", textMessage: "OK" };
			}
			return { ackCode: "AR", textMessage: "Message type not supported" };
		}`,
	})
	code, text, err := c.runACKScript(sampleHL7("SC003", "SIU^S12"), "AA", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "AR" {
		t.Errorf("expected 'AR', got %q", code)
	}
	if !strings.Contains(text, "not supported") {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestACK_Script_SyntaxErrorFallsBackToDefault(t *testing.T) {
	// TC-GO-017: JS syntax error → error returned, caller uses defaults
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg { return { ackCode: "AA" }; }`, // missing )
	})
	code, text, err := c.runACKScript(sampleHL7("SC004", "ADT^A01"), "AA", "default text")
	if err == nil {
		t.Error("expected error for syntax-invalid script")
	}
	// Defaults must be returned when error occurs
	if code != "AA" || text != "default text" {
		t.Errorf("expected defaults on error, got code=%q text=%q", code, text)
	}
}

func TestACK_Script_MissingBuildACKFunction(t *testing.T) {
	// TC-GO-018: script doesn't define buildACK
	c := newConnectorWithACK(t, ACKConfig{
		Script: `var x = 1;`,
	})
	_, _, err := c.runACKScript(sampleHL7("SC005", "ADT^A01"), "AA", "default")
	if err == nil {
		t.Error("expected error when buildACK is not defined")
	}
}

func TestACK_Script_BuildACKNotAFunction(t *testing.T) {
	// TC-GO-019: buildACK is a string, not a function
	c := newConnectorWithACK(t, ACKConfig{
		Script: `var buildACK = "not a function";`,
	})
	_, _, err := c.runACKScript(sampleHL7("SC006", "ADT^A01"), "AA", "default")
	if err == nil {
		t.Error("expected error when buildACK is not callable")
	}
}

func TestACK_Script_UndefinedAckCodeUsesDefault(t *testing.T) {
	// TC-GO-020: script returns object without ackCode → fallback to defaultCode
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) { return { textMessage: "Custom text only" }; }`,
	})
	code, text, err := c.runACKScript(sampleHL7("SC007", "ADT^A01"), "AA", "default text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "AA" {
		t.Errorf("expected fallback 'AA', got %q", code)
	}
	if text != "Custom text only" {
		t.Errorf("expected 'Custom text only', got %q", text)
	}
}

func TestACK_Script_AccessesControlID(t *testing.T) {
	// TC-GO-021: script can read msg.controlID
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			return { ackCode: "AA", textMessage: "Accepted: " + msg.controlID };
		}`,
	})
	_, text, err := c.runACKScript(sampleHL7("MYID12345", "ADT^A01"), "AA", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "MYID12345") {
		t.Errorf("expected controlID in text, got %q", text)
	}
}

func TestACK_Script_AccessesMessageType(t *testing.T) {
	// TC-GO-022: script can read msg.messageType
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			return { ackCode: "AA", textMessage: "Type=" + msg.messageType };
		}`,
	})
	_, text, err := c.runACKScript(sampleHL7("X1", "ORU^R01"), "AA", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "ORU^R01") {
		t.Errorf("expected message type in text, got %q", text)
	}
}

func TestACK_Script_AccessesSendingApp(t *testing.T) {
	// TC-GO-023: script can read msg.sendingApp (MSH-3 of inbound)
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			return { ackCode: "AA", textMessage: "From=" + msg.sendingApp };
		}`,
	})
	// sampleHL7 uses "SENDER" as MSH-3
	_, text, err := c.runACKScript(sampleHL7("X2", "ADT^A01"), "AA", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "SENDER") {
		t.Errorf("expected 'SENDER' in text, got %q", text)
	}
}

func TestACK_Script_RuntimeErrorFallsBackToDefault(t *testing.T) {
	// TC-GO-024: script throws at runtime → fallback to defaults
	c := newConnectorWithACK(t, ACKConfig{
		Script: `function buildACK(msg) {
			throw new Error("intentional runtime error");
		}`,
	})
	code, text, err := c.runACKScript(sampleHL7("X3", "ADT^A01"), "AE", "error default")
	if err == nil {
		t.Error("expected error from script throw")
	}
	if code != "AE" || text != "error default" {
		t.Errorf("expected defaults on runtime error, got code=%q text=%q", code, text)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-025..026  ACKConfig defaults via Initialize
// ─────────────────────────────────────────────────────────────────────────────

func TestACK_Initialize_EmptyAckMapGivesDefaults(t *testing.T) {
	// TC-GO-025: connector with no ack config gets sensible defaults
	c := newConnectorOnPort(t, 0, nil) // port 0 = ephemeral, never started
	if c.ackConfig.Mode != "immediate" {
		t.Errorf("default Mode expected 'immediate', got %q", c.ackConfig.Mode)
	}
	if c.ackConfig.OnError != "suppress" {
		t.Errorf("default OnError expected 'suppress', got %q", c.ackConfig.OnError)
	}
	if c.ackConfig.SendingApp != "ezHealthKonnect" {
		t.Errorf("default SendingApp expected 'ezHealthKonnect', got %q", c.ackConfig.SendingApp)
	}
	if c.ackConfig.SendingFacility != "EHK" {
		t.Errorf("default SendingFacility expected 'EHK', got %q", c.ackConfig.SendingFacility)
	}
	if c.ackConfig.TextSuccess != "Message received successfully" {
		t.Errorf("default TextSuccess unexpected: %q", c.ackConfig.TextSuccess)
	}
	if c.ackConfig.TextError != "Message processing error" {
		t.Errorf("default TextError unexpected: %q", c.ackConfig.TextError)
	}
}

func TestACK_Initialize_PartialAckMapMergesWithDefaults(t *testing.T) {
	// TC-GO-026: only sending_app overridden, rest stay at defaults
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"ack": map[string]interface{}{
			"sending_app": "MY_SYSTEM",
		},
	})
	if c.ackConfig.SendingApp != "MY_SYSTEM" {
		t.Errorf("expected 'MY_SYSTEM', got %q", c.ackConfig.SendingApp)
	}
	if c.ackConfig.SendingFacility != "EHK" {
		t.Errorf("SendingFacility should still be 'EHK', got %q", c.ackConfig.SendingFacility)
	}
	if c.ackConfig.Mode != "immediate" {
		t.Errorf("Mode should still be 'immediate', got %q", c.ackConfig.Mode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-027..035  Integration — real TCP listener, full MLLP round-trip
// ─────────────────────────────────────────────────────────────────────────────
// These tests start an actual TCP listener on a random port and send real
// MLLP-framed HL7 messages to verify end-to-end ACK behaviour.

// findFreePort asks the OS for an available TCP port.
func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestACKIntegration_DefaultConfig_SendsAA(t *testing.T) {
	// TC-GO-027: default config → AA ACK received by sender
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, nil)
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("INT001", "ADT^A01")))
	ack := readMLLPResponse(t, conn)

	_, msa := parseACKSegments(ack)
	if len(msa) < 2 || msa[1] != "AA" {
		t.Errorf("expected MSA-1=AA, got: %v | raw: %q", msa, ack)
	}
}

func TestACKIntegration_CustomSenderIdentityInACK(t *testing.T) {
	// TC-GO-028: sending_app/facility appear in MSH-3/MSH-4 of returned ACK
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"ack": map[string]interface{}{
			"sending_app":      "HOSPITAL_HIS",
			"sending_facility": "WARD_7B",
		},
	})
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("INT002", "ADT^A01")))
	ack := readMLLPResponse(t, conn)

	msh, _ := parseACKSegments(ack)
	if len(msh) < 4 {
		t.Fatalf("MSH too short in ACK: %q", ack)
	}
	if msh[2] != "HOSPITAL_HIS" {
		t.Errorf("MSH-3 (index 2) expected 'HOSPITAL_HIS', got %q", msh[2])
	}
	if msh[3] != "WARD_7B" {
		t.Errorf("MSH-4 (index 3) expected 'WARD_7B', got %q", msh[3])
	}
}

func TestACKIntegration_CustomSuccessTextInMSA3(t *testing.T) {
	// TC-GO-029: text_success appears in MSA-3
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"ack": map[string]interface{}{
			"text_success": "Routed to Epic ADT",
		},
	})
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("INT003", "ADT^A01")))
	ack := readMLLPResponse(t, conn)

	_, msa := parseACKSegments(ack)
	if len(msa) < 4 || msa[3] != "Routed to Epic ADT" {
		t.Errorf("expected custom MSA-3, got: %v | raw: %q", msa, ack)
	}
}

func TestACKIntegration_ScriptRejectsWrongMessageType(t *testing.T) {
	// TC-GO-030: script returns AR for SIU messages; sender gets AR back
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"ack": map[string]interface{}{
			"script": `function buildACK(msg) {
				if (msg.messageType.indexOf("ADT") !== 0) {
					return { ackCode: "AR", textMessage: "Only ADT messages accepted" };
				}
				return { ackCode: "AA", textMessage: "Accepted" };
			}`,
		},
	})
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("INT004", "SIU^S12")))
	ack := readMLLPResponse(t, conn)

	_, msa := parseACKSegments(ack)
	if len(msa) < 2 || msa[1] != "AR" {
		t.Errorf("expected AR for SIU message, got: %v | raw: %q", msa, ack)
	}
	if len(msa) >= 4 && !strings.Contains(msa[3], "Only ADT") {
		t.Errorf("expected rejection text in MSA-3, got %q", msa[3])
	}
}

func TestACKIntegration_ModeNoneUpgradedToImmediate(t *testing.T) {
	// TC-GO-031: mode=none is no longer valid — MLLP requires a response for every message.
	// Initialize() silently upgrades mode=none to mode=immediate, so the sender always
	// receives an AA. This test verifies that behavior.
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"ack": map[string]interface{}{"mode": "none"},
	})
	msgChan, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("INT005", "ADT^A01")))

	// ACK must always be sent — mode=none is upgraded to immediate
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		t.Errorf("expected ACK (mode=none upgraded to immediate), got n=%d err=%v", n, err)
	}
	_, msa := parseACKSegments(string(buf[:n]))
	if len(msa) < 2 || msa[1] != "AA" {
		t.Errorf("expected AA after mode=none upgrade, got: %v", msa)
	}

	// Message must also be queued
	select {
	case msg := <-msgChan:
		if msg == nil {
			t.Error("expected non-nil message")
		}
	case <-time.After(1 * time.Second):
		t.Error("message not queued within 1s")
	}
}

func TestACKIntegration_ControlIDCorrelation(t *testing.T) {
	// TC-GO-033: MSA-2 in ACK must echo original MSH-10
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, nil)
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	const myControlID = "CORRELATION_TEST_42"
	conn.Write(buildMLLPFrame(sampleHL7(myControlID, "ADT^A01")))
	ack := readMLLPResponse(t, conn)

	_, msa := parseACKSegments(ack)
	if len(msa) < 3 || msa[2] != myControlID {
		t.Errorf("MSA-2 should echo %q, got: %v | raw: %q", myControlID, msa, ack)
	}
}

func TestACKIntegration_MultipleConcurrentConnections(t *testing.T) {
	// TC-GO-032: 5 concurrent senders each get their own AA ACK
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, nil)
	_, stop := startConnector(t, c)
	defer stop()

	const workers = 5
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func(idx int) {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
			if err != nil {
				errs <- fmt.Errorf("worker %d connect: %v", idx, err)
				return
			}
			defer conn.Close()

			controlID := fmt.Sprintf("CONCURRENT_%d", idx)
			conn.Write(buildMLLPFrame(sampleHL7(controlID, "ADT^A01")))

			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			reader := bufio.NewReader(conn)
			ack, err := readMLLPFrame(reader)
			if err != nil {
				errs <- fmt.Errorf("worker %d read ACK: %v", idx, err)
				return
			}

			_, msa := parseACKSegments(ack)
			if len(msa) < 3 || msa[2] != controlID {
				errs <- fmt.Errorf("worker %d: MSA-2 should be %q, got %v", idx, controlID, msa)
				return
			}
			errs <- nil
		}(i)
	}

	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestACKIntegration_MultipleMessagesOnSameConnection(t *testing.T) {
	// TC-GO-035: one connection sends two messages sequentially; each gets an ACK
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, nil)
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	for seq, ctrlID := range []string{"MULTI_MSG_1", "MULTI_MSG_2"} {
		conn.Write(buildMLLPFrame(sampleHL7(ctrlID, "ADT^A01")))
		ack := readMLLPResponse(t, conn)

		_, msa := parseACKSegments(ack)
		if len(msa) < 3 || msa[2] != ctrlID {
			t.Errorf("message %d: MSA-2 should be %q, got %v", seq+1, ctrlID, msa)
		}
		if msa[1] != "AA" {
			t.Errorf("message %d: expected AA, got %q", seq+1, msa[1])
		}
	}
}

func TestACKIntegration_ScriptErrorFallsBackToDefaultAA(t *testing.T) {
	// TC-GO-034: broken script → fallback AA is still returned (connector doesn't crash)
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"ack": map[string]interface{}{
			"script": `function buildACK(msg) { throw "boom"; }`,
		},
	})
	_, stop := startConnector(t, c)
	defer stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("ERR001", "ADT^A01")))
	ack := readMLLPResponse(t, conn)

	_, msa := parseACKSegments(ack)
	// Should fall back to AA (default) not crash
	if len(msa) < 2 || msa[1] != "AA" {
		t.Errorf("expected fallback AA on script error, got: %v | raw: %q", msa, ack)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-GO-036  extractMSHField helper
// ─────────────────────────────────────────────────────────────────────────────

func TestACK_ExtractMSHField_SendingApp(t *testing.T) {
	c := &TCPMLLPInboundConnector{}
	hl7 := sampleHL7("X", "ADT^A01") // MSH-3 = "SENDER"
	got := c.extractMSHField(hl7, 2)  // position 2 = MSH-3 (after split by |)
	if got != "SENDER" {
		t.Errorf("expected 'SENDER', got %q", got)
	}
}

func TestACK_ExtractMSHField_OutOfRange(t *testing.T) {
	c := &TCPMLLPInboundConnector{}
	got := c.extractMSHField("MSH|only|two", 99)
	if got != "" {
		t.Errorf("expected empty string for out-of-range, got %q", got)
	}
}

// Suppress unused import warning from models
var _ = models.InboundMessage{}
