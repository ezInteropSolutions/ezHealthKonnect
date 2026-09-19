package transform

import (
	"bufio"
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"net"
	"testing"
)

// fakeMLLPServer accepts one connection, reads a single MLLP frame
// (VT ... FS CR), and writes back a canned ACK framed the same way.
// Mirrors the real wire format tcp_mllp_outbound.go's mllpFrame/readMLLPFrame
// use (start=0x0B, end=0x1C 0x0D) without depending on those unexported
// helpers from another package.
func fakeMLLPServer(t *testing.T, ackSegment string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake MLLP server: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed during cleanup
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		// Discard bytes up to and including the start byte.
		if _, err := reader.ReadBytes(0x0B); err != nil {
			return
		}
		// Read until the FS (0x1C) framing byte; discard the trailing CR (0x0D) separately.
		if _, err := reader.ReadBytes(0x1C); err != nil {
			return
		}
		_, _ = reader.Discard(1) // trailing CR

		ack := "\x0b" + ackSegment + "\x1c\x0d"
		_, _ = conn.Write([]byte(ack))
	}()

	return ln.Addr().String()
}

func TestOutboundConnector_MLLP_SurfacesParsedAckCode(t *testing.T) {
	host, port := splitHostPort(t, fakeMLLPServer(t, "MSH|^~\\&|SERVER|FAC|CLIENT|FAC|20260913||ACK|1|P|2.5\rMSA|AA|1\r"))

	exec := NewOutboundConnectorExecutor()
	step := &models.TransformationStep{
		StepName: "Deliver ADT",
		StepType: "connector.outbound",
		Enabled:  true,
		Config: map[string]interface{}{
			"connectorType": "tcp_mllp_outbound",
			"config": map[string]interface{}{
				"host":                     host,
				"port":                     port,
				"connection_mode":          "per-message",
				"ack_timeout_seconds":      5,
				"connect_timeout_seconds":  5,
			},
			"contentField": "raw",
		},
	}
	input := map[string]interface{}{
		"raw": "MSH|^~\\&|A|B|C|D|20260913||ADT^A01|1|P|2.5",
	}

	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("expected successful delivery, got error: %v", err)
	}

	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be set, got: %+v", result)
	}

	if got := out["ack_code"]; got != "AA" {
		t.Errorf("expected ack_code=AA surfaced from result.Metadata, got %v (full step output: %+v)", got, out)
	}
	ack, _ := out["acknowledgment"].(string)
	if ack == "" {
		t.Errorf("expected raw acknowledgment text to still be surfaced, got empty")
	}
}

func TestOutboundConnector_MLLP_SurfacesNackCode(t *testing.T) {
	host, port := splitHostPort(t, fakeMLLPServer(t, "MSH|^~\\&|SERVER|FAC|CLIENT|FAC|20260913||ACK|1|P|2.5\rMSA|AE|1|Unknown message type\r"))

	exec := NewOutboundConnectorExecutor()
	step := &models.TransformationStep{
		StepName: "Deliver ADT",
		StepType: "connector.outbound",
		Enabled:  true,
		Config: map[string]interface{}{
			"connectorType": "tcp_mllp_outbound",
			"config": map[string]interface{}{
				"host":                    host,
				"port":                    port,
				"connection_mode":         "per-message",
				"ack_timeout_seconds":     5,
				"connect_timeout_seconds": 5,
			},
			"contentField": "raw",
		},
	}
	input := map[string]interface{}{
		"raw": "MSH|^~\\&|A|B|C|D|20260913||ADT^A01|1|P|2.5",
	}

	result, err := exec.Execute(context.Background(), step, input)
	// tcp_mllp_outbound's Send() reports a NACK as data (DeliveryResult.Success
	// = false), not as a Go error — the executor's own network-level "success"
	// stays true (the send itself didn't fail); "delivery_success" and
	// "ack_code" are what a downstream step must actually check.
	if err != nil {
		t.Fatalf("expected no Go-level error on a NACK (it's reported via DeliveryResult, not err), got: %v", err)
	}

	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be set, got: %+v", result)
	}
	if got := out["delivery_success"]; got != false {
		t.Errorf("expected delivery_success=false on NACK, got %v", got)
	}
	if got := out["ack_code"]; got != "AE" {
		t.Errorf("expected ack_code=AE surfaced on NACK, got %v (full step output: %+v)", got, out)
	}
}

// splitHostPort is a tiny helper since fakeMLLPServer returns "host:port" but
// the connector config wants them as separate fields.
func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split addr %q: %v", addr, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("failed to parse port %q: %v", portStr, err)
	}
	return host, port
}
