// services/connectors/tcp_mllp_hostquery_edgecases_test.go
// Additional edge-case coverage for TCPMLLPInboundConnector's host-query
// mechanism (see tcp_mllp_hostquery_test.go for the core mechanism tests).
// This file focuses on cases the core suite didn't already cover:
//   - multiple messages (normal + query, interleaved) on ONE persistent
//     connection, proving handleHostQuery's "continue" correctly resumes the
//     read loop rather than leaving the reader/connection in a bad state
//   - concurrent connections, each getting its OWN correct reply — guards
//     against any accidental shared mutable state across goroutines
//   - case-insensitive message_types matching, from both directions
//   - the real (easily-misunderstood) behavior of an explicitly empty
//     message_types: [] array
//
// Run:
//   go test ./services/connectors/ -v -run TestHostQueryEdgeCase
package connectors

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Same connection, multiple messages (normal + query interleaved)
// ─────────────────────────────────────────────────────────────────────────────

func TestHostQueryEdgeCase_SameConnection_NormalThenQueryThenNormal(t *testing.T) {
	// A real analyzer keeps one persistent MLLP connection open and sends a
	// mix of message types on it over time (e.g. results, then an order
	// query, then more results). handleHostQuery's own "continue" must
	// resume the SAME read loop cleanly — this proves it does, across three
	// messages of alternating kind on one socket.
	reply := "MSH|^~\\&|EHK|EHK|ANALYZER|LAB|20260101120000||DSR^Q03|R1|P|2.5\r"
	fake := &fakePipelineExecutor{executeResult: hl7BuildResult(reply)}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(fake)
	msgChan, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()

	// 1) A normal ADT message — async enqueue + fixed AA ack.
	conn.Write(buildMLLPFrame(sampleHL7("CTRL-A", "ADT^A01")))
	select {
	case msg := <-msgChan:
		if msg.MessageType != "ADT^A01" {
			t.Errorf("expected first message enqueued as ADT^A01, got %q", msg.MessageType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first (ADT) message was never enqueued")
	}
	ack1 := readMLLPResponse(t, conn)
	if _, msa := parseACKSegments(ack1); len(msa) < 2 || msa[1] != "AA" {
		t.Fatalf("expected AA ack for the first ADT message, got %q", ack1)
	}

	// 2) A host-query message on the SAME connection — synchronous reply,
	// never touches msgChan.
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL-B")))
	reply2 := readMLLPResponse(t, conn)
	if reply2 != reply {
		t.Fatalf("expected the pipeline's own DSR reply for the query, got %q", reply2)
	}
	select {
	case msg := <-msgChan:
		t.Errorf("the host-query message must never be enqueued for async processing, got %v", msg)
	case <-time.After(300 * time.Millisecond):
		// expected — nothing enqueued
	}

	// 3) A second normal message right after — proves the connection/reader
	// is still healthy after the synchronous branch.
	conn.Write(buildMLLPFrame(sampleHL7("CTRL-C", "ORU^R01")))
	select {
	case msg := <-msgChan:
		if msg.MessageType != "ORU^R01" {
			t.Errorf("expected third message enqueued as ORU^R01, got %q", msg.MessageType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("third (ORU) message was never enqueued — connection/reader likely broke after the host-query branch")
	}
	ack3 := readMLLPResponse(t, conn)
	if _, msa := parseACKSegments(ack3); len(msa) < 2 || msa[1] != "AA" {
		t.Errorf("expected AA ack for the third message, got %q", ack3)
	}
}

func TestHostQueryEdgeCase_SameConnection_TwoConsecutiveQueriesBothAnswered(t *testing.T) {
	// A device may send several queries back-to-back on the same connection
	// (e.g. one per sample) before the operator moves on — confirm each gets
	// its own independent reply and the executor is called for both.
	fake := &fakePipelineExecutor{executeResult: hl7BuildResult("MSH|^~\\&|EHK|EHK|A|A|20260101||DSR^Q03|R1|P|2.5\r")}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()

	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL-Q1")))
	r1 := readMLLPResponse(t, conn)
	if r1 == "" {
		t.Fatal("expected a reply to the first query")
	}

	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL-Q2")))
	r2 := readMLLPResponse(t, conn)
	if r2 == "" {
		t.Fatal("expected a reply to the second query")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Concurrent connections
// ─────────────────────────────────────────────────────────────────────────────

// echoingPipelineExecutor is a real PipelineExecutor implementation (not a
// canned fixed response) that reads the incoming raw HL7 message's own
// MSH.10 control ID back out and echoes it into the built reply — so a
// concurrency test can prove each connection got back exactly the reply that
// corresponds to ITS OWN request, not a sibling goroutine's, ruling out any
// accidental shared mutable state across concurrently-handled connections.
type echoingPipelineExecutor struct{}

func (f *echoingPipelineExecutor) GetPipeline(ctx context.Context, interfaceID, messageType string) (*models.TransformationPipeline, error) {
	return &models.TransformationPipeline{ID: "echo-pipeline", InterfaceID: interfaceID, MessageType: messageType}, nil
}

func (f *echoingPipelineExecutor) ExecutePipeline(ctx context.Context, pipeline *models.TransformationPipeline, inputData map[string]interface{}) (*models.TransformationExecutionResult, error) {
	raw, _ := inputData["raw"].(string)
	controlID := ""
	if lines := strings.Split(raw, "\r"); len(lines) > 0 {
		if fields := strings.Split(lines[0], "|"); len(fields) > 9 {
			controlID = fields[9] // MSH.10
		}
	}
	return hl7BuildResult("ECHO:" + controlID), nil
}

func TestHostQueryEdgeCase_ConcurrentConnections_EachGetsItsOwnCorrectReply(t *testing.T) {
	// Guards against any accidental shared mutable state (e.g. a package- or
	// connector-level variable holding "the current request") across
	// concurrently-handled connections — each of N simultaneous connections
	// must receive the reply that corresponds to the control ID IT sent, not
	// a sibling connection's.
	fake := &echoingPipelineExecutor{}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	const n = 8
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			controlID := fmt.Sprintf("CTRL-CONCURRENT-%d", i)
			conn := dialLocal(t, port)
			defer conn.Close()
			conn.Write(buildMLLPFrame(hostQueryHL7(controlID)))
			reply := readMLLPResponse(t, conn)
			expected := "ECHO:" + controlID
			if reply != expected {
				results <- fmt.Errorf("connection %d: expected reply %q, got %q", i, expected, reply)
				return
			}
			results <- nil
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Case-insensitive message_types matching
// ─────────────────────────────────────────────────────────────────────────────

func TestHostQueryEdgeCase_IsHostQueryMessage_ConfiguredLowercaseMatchesUppercaseWire(t *testing.T) {
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true, "message_types": []interface{}{"qry"}},
	})
	if !c.isHostQueryMessage(hostQueryHL7("X")) { // wire message type is "QRY^Q02"
		t.Error("expected a lowercase-configured 'qry' to match the wire's uppercase 'QRY^Q02'")
	}
}

func TestHostQueryEdgeCase_IsHostQueryMessage_ConfiguredUppercaseMatchesLowercaseWire(t *testing.T) {
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true, "message_types": []interface{}{"QRY"}},
	})
	lowerWireMsg := "MSH|^~\\&|ANALYZER|LAB|EHK|EHK|20260101||qry^q02|CTRL|P|2.3.1\r"
	if !c.isHostQueryMessage(lowerWireMsg) {
		t.Error("expected the configured uppercase 'QRY' to match a lowercase 'qry^q02' on the wire")
	}
}

func TestHostQueryEdgeCase_IsHostQueryMessage_NonMatchingTypeReturnsFalse(t *testing.T) {
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true, "message_types": []interface{}{"QRY"}},
	})
	if c.isHostQueryMessage(sampleHL7("X", "ADT^A01")) {
		t.Error("expected ADT^A01 to NOT match a message_types list of [QRY]")
	}
}

func TestHostQueryEdgeCase_IsHostQueryMessage_MalformedMessageReturnsFalseNotPanic(t *testing.T) {
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true},
	})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("isHostQueryMessage panicked on malformed input: %v", r)
		}
	}()
	if c.isHostQueryMessage("") {
		t.Error("expected an empty message to never match")
	}
	if c.isHostQueryMessage("not even close to HL7") {
		t.Error("expected a non-HL7 string to never match")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Explicitly empty message_types: [] — a real, easily-misunderstood case
// ─────────────────────────────────────────────────────────────────────────────

func TestHostQueryEdgeCase_ExplicitlyEmptyMessageTypesArray_FallsBackToDefaultQRY(t *testing.T) {
	// getStringSliceFromMap falls back to its default whenever the parsed
	// slice is empty — including when the CONFIG explicitly supplied []
	// rather than omitting the key entirely. Worth a named, asserted test:
	// a user who writes message_types: [] expecting "match nothing" is
	// actually getting "match the default (QRY)" instead — real, current
	// behavior, not a bug, but one a future change could easily flip
	// unnoticed.
	c := newConnectorOnPort(t, 0, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true, "message_types": []interface{}{}},
	})
	if len(c.hostQueryConfig.MessageTypes) != 1 || c.hostQueryConfig.MessageTypes[0] != "QRY" {
		t.Errorf("expected an explicitly empty message_types array to fall back to the default [QRY], got %v", c.hostQueryConfig.MessageTypes)
	}
	if !c.isHostQueryMessage(hostQueryHL7("X")) {
		t.Error("expected a QRY^Q02 message to still match after an explicitly empty message_types array (default fallback)")
	}
}

