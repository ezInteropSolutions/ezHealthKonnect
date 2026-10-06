// services/connectors/tcp_mllp_hostquery_test.go
// Tests for TCPMLLPInboundConnector's synchronous host-query handling
// (host_query config + PipelineExecutor/PipelineAwareConnector) — the
// mechanism a device like a Mindray lab analyzer relies on when it sends a
// live HL7 query (e.g. QRY^Q02) on an open MLLP connection and expects a
// real reply message (e.g. DSR^Q03) back on that SAME connection, not just
// an MSA ack.
//
// Test Groups:
//   TC-HQ-001..006  Pure helpers: getBoolFromMap, getIntFromMap,
//                   getStringSliceFromMap, splitMessageType
//   TC-HQ-007..011  extractHL7BuildReply
//   TC-HQ-012..019  Integration — real TCP listener, real MLLP round-trip,
//                   fakePipelineExecutor
//
// Run:
//   go test ./services/connectors/ -v -run TestHostQuery
package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// dialLocal connects to a connector already listening on 127.0.0.1:port —
// shared by every integration test below (mirrors tcp_mllp_ack_test.go's own
// repeated net.DialTimeout call, factored out since this file uses it many
// more times).
func dialLocal(t *testing.T, port int) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return conn
}

// ─────────────────────────────────────────────────────────────────────────────
// fakePipelineExecutor — a minimal, configurable PipelineExecutor test double.
// ─────────────────────────────────────────────────────────────────────────────

type fakePipelineExecutor struct {
	// getPipelineErr, when set, makes GetPipeline fail.
	getPipelineErr error
	// pipeline is returned by a successful GetPipeline call.
	pipeline *models.TransformationPipeline
	// executeDelay simulates a slow live HIS round trip.
	executeDelay time.Duration
	// executeResult/executeErr are returned by ExecutePipeline.
	executeResult *models.TransformationExecutionResult
	executeErr    error

	// Captured call arguments, for assertions.
	gotInterfaceID string
	gotMessageType string
	gotInputData   map[string]interface{}
}

func (f *fakePipelineExecutor) GetPipeline(ctx context.Context, interfaceID, messageType string) (*models.TransformationPipeline, error) {
	f.gotInterfaceID = interfaceID
	f.gotMessageType = messageType
	if f.getPipelineErr != nil {
		return nil, f.getPipelineErr
	}
	if f.pipeline != nil {
		return f.pipeline, nil
	}
	return &models.TransformationPipeline{ID: "test-pipeline", InterfaceID: interfaceID, MessageType: messageType}, nil
}

func (f *fakePipelineExecutor) ExecutePipeline(ctx context.Context, pipeline *models.TransformationPipeline, inputData map[string]interface{}) (*models.TransformationExecutionResult, error) {
	f.gotInputData = inputData
	if f.executeDelay > 0 {
		select {
		case <-time.After(f.executeDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.executeErr != nil {
		// Mirrors the real TransformationPipelineService.ExecutePipeline,
		// which returns its PARTIAL result alongside a non-nil error when a
		// step fails ("return result, fmt.Errorf(...)"), not nil — tests
		// that only set executeErr (and leave executeResult nil) keep
		// their existing "nil result + error" behavior unchanged.
		return f.executeResult, f.executeErr
	}
	if f.executeResult != nil {
		return f.executeResult, nil
	}
	return &models.TransformationExecutionResult{Status: "completed"}, nil
}

// hl7BuildResult builds a canned "completed" execution result whose final
// hl7.build step produced replyMessage — the shape extractHL7BuildReply and
// handleHostQuery both expect.
func hl7BuildResult(replyMessage string) *models.TransformationExecutionResult {
	return &models.TransformationExecutionResult{
		Status: "completed",
		ExecutionLog: []models.StepExecutionLog{
			{
				StepType: "hl7.build",
				Success:  true,
				StepOutput: &models.StepOutput{
					OutputData: map[string]interface{}{"hl7_message": replyMessage},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-HQ-001..006  Pure helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestHostQuery_GetBoolFromMap_NilMapReturnsDefault(t *testing.T) {
	if got := getBoolFromMap(nil, "enabled", true); got != true {
		t.Errorf("expected true, got %v", got)
	}
}

func TestHostQuery_GetBoolFromMap_ReadsRealValue(t *testing.T) {
	m := map[string]interface{}{"enabled": true}
	if got := getBoolFromMap(m, "enabled", false); got != true {
		t.Errorf("expected true, got %v", got)
	}
}

func TestHostQuery_GetIntFromMap_HandlesJSONFloat64(t *testing.T) {
	// JSON numbers unmarshal as float64 — this is the real shape Initialize
	// sees after json.Unmarshal on the connector's raw config bytes.
	m := map[string]interface{}{"timeout_seconds": float64(15)}
	if got := getIntFromMap(m, "timeout_seconds", 10); got != 15 {
		t.Errorf("expected 15, got %d", got)
	}
}

func TestHostQuery_GetIntFromMap_MissingKeyReturnsDefault(t *testing.T) {
	if got := getIntFromMap(map[string]interface{}{}, "timeout_seconds", 10); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestHostQuery_GetStringSliceFromMap_ReadsJSONArray(t *testing.T) {
	m := map[string]interface{}{"message_types": []interface{}{"QRY", "QBP"}}
	got := getStringSliceFromMap(m, "message_types", []string{"QRY"})
	if len(got) != 2 || got[0] != "QRY" || got[1] != "QBP" {
		t.Errorf("expected [QRY QBP], got %v", got)
	}
}

func TestHostQuery_GetStringSliceFromMap_EmptyOrMissingReturnsDefault(t *testing.T) {
	got := getStringSliceFromMap(nil, "message_types", []string{"QRY"})
	if len(got) != 1 || got[0] != "QRY" {
		t.Errorf("expected [QRY], got %v", got)
	}
}

func TestHostQuery_SplitMessageType_SplitsOnCaret(t *testing.T) {
	msgType, trigger := splitMessageType("QRY^Q02")
	if msgType != "QRY" || trigger != "Q02" {
		t.Errorf("expected (QRY, Q02), got (%s, %s)", msgType, trigger)
	}
}

func TestHostQuery_SplitMessageType_NoCaretLeavesTriggerEmpty(t *testing.T) {
	msgType, trigger := splitMessageType("UNKNOWN")
	if msgType != "UNKNOWN" || trigger != "" {
		t.Errorf("expected (UNKNOWN, \"\"), got (%s, %s)", msgType, trigger)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-HQ-007..011  extractHL7BuildReply
// ─────────────────────────────────────────────────────────────────────────────

func TestHostQuery_ExtractHL7BuildReply_NilResultReturnsEmpty(t *testing.T) {
	if got := extractHL7BuildReply(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHostQuery_ExtractHL7BuildReply_NoMatchingStepReturnsEmpty(t *testing.T) {
	result := &models.TransformationExecutionResult{
		Status: "completed",
		ExecutionLog: []models.StepExecutionLog{
			{StepType: "hl7.parse", Success: true, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"parsed": true}}},
		},
	}
	if got := extractHL7BuildReply(result); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHostQuery_ExtractHL7BuildReply_ReturnsHL7MessageField(t *testing.T) {
	result := hl7BuildResult("MSH|^~\\&|EHK|EHK|A|A|20260101120000||DSR^Q03|1|P|2.5\rQAK|1|OK\r")
	got := extractHL7BuildReply(result)
	if got == "" || got[:3] != "MSH" {
		t.Errorf("expected a real MSH-prefixed reply, got %q", got)
	}
}

func TestHostQuery_ExtractHL7BuildReply_UnsuccessfulStepSkipped(t *testing.T) {
	result := &models.TransformationExecutionResult{
		Status: "completed",
		ExecutionLog: []models.StepExecutionLog{
			{StepType: "hl7.build", Success: false, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"hl7_message": "SHOULD_NOT_APPEAR"}}},
		},
	}
	if got := extractHL7BuildReply(result); got != "" {
		t.Errorf("expected empty string for an unsuccessful step, got %q", got)
	}
}

func TestHostQuery_ExtractHL7BuildReply_LastMatchingStepWins(t *testing.T) {
	result := &models.TransformationExecutionResult{
		Status: "completed",
		ExecutionLog: []models.StepExecutionLog{
			{StepType: "hl7.build", Success: true, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"hl7_message": "FIRST"}}},
			{StepType: "hl7.build", Success: true, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"hl7_message": "SECOND"}}},
		},
	}
	if got := extractHL7BuildReply(result); got != "SECOND" {
		t.Errorf("expected the last matching step's output (SECOND), got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-HQ-012..019  Integration — real TCP listener, real MLLP round-trip
// ─────────────────────────────────────────────────────────────────────────────

// hostQueryHL7 returns a minimal QRY^Q02 message.
func hostQueryHL7(controlID string) string {
	ts := time.Now().Format("20060102150405")
	return "MSH|^~\\&|ANALYZER|LAB|EHK|EHK|" + ts + "||QRY^Q02|" + controlID + "|P|2.3.1\r" +
		"QRD|" + ts + "|R|I|Q001|||1^RD|SAMPLE001|OTH\r"
}

func TestHostQuery_IntegrationRegression_DisabledByDefaultBehavesUnchanged(t *testing.T) {
	// TC-HQ-012: with no "host_query" config at all, a QRY^Q02 message must
	// go through the EXACT same async-enqueue + fixed-ACK path every other
	// message type already does — zero behavior change for every existing
	// deployment that hasn't opted in.
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{"interface_id": "iface-1"})
	msgChan, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()

	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL1")))

	select {
	case msg := <-msgChan:
		if msg.MessageType != "QRY^Q02" {
			t.Errorf("expected message to be enqueued with type QRY^Q02, got %q", msg.MessageType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("message was not enqueued for async processing — host_query default should be disabled")
	}

	ack := readMLLPResponse(t, conn)
	_, msa := parseACKSegments(ack)
	if len(msa) < 2 || msa[1] != "AA" {
		t.Errorf("expected a standard AA ack, got MSA=%v", msa)
	}
}

func TestHostQuery_IntegrationRegression_NonMatchingTypeStillEnqueued(t *testing.T) {
	// TC-HQ-013: even with host_query enabled for "QRY", an unrelated
	// message type (e.g. an ADT result) still flows through the normal path.
	fake := &fakePipelineExecutor{}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true, "message_types": []interface{}{"QRY"}},
	})
	c.SetPipelineExecutor(fake)
	msgChan, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()

	conn.Write(buildMLLPFrame(sampleHL7("CTRL2", "ADT^A01")))

	select {
	case <-msgChan:
		// expected — ADT is not a configured host_query message type
	case <-time.After(2 * time.Second):
		t.Fatal("ADT message should still be enqueued for normal async processing")
	}
	readMLLPResponse(t, conn) // drain the standard ACK
}

func TestHostQuery_Integration_SuccessfulQueryReturnsLiveReply(t *testing.T) {
	// TC-HQ-014: the core happy path — a matching QRY^Q02 resolves the
	// connector's own interface pipeline, executes it, and writes the
	// pipeline's real hl7.build output back on the SAME connection.
	expectedReply := "MSH|^~\\&|EHK|EHK|ANALYZER|LAB|20260101120000||DSR^Q03|R1|P|2.5\rQAK|1|OK\rQRD|...\r"
	fake := &fakePipelineExecutor{executeResult: hl7BuildResult(expectedReply)}

	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-42",
		"host_query": map[string]interface{}{
			"enabled":               true,
			"message_types":         []interface{}{"QRY"},
			"pipeline_message_type": "QRY",
			"timeout_seconds":       5,
		},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()

	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL3")))

	reply := readMLLPResponse(t, conn)
	if reply != expectedReply {
		t.Errorf("expected the pipeline's own reply verbatim, got %q", reply)
	}
	if fake.gotInterfaceID != "iface-42" {
		t.Errorf("expected GetPipeline called with interfaceID iface-42, got %q", fake.gotInterfaceID)
	}
	if fake.gotMessageType != "QRY" {
		t.Errorf("expected GetPipeline called with messageType QRY, got %q", fake.gotMessageType)
	}
	if raw, ok := fake.gotInputData["raw"].(string); !ok || raw == "" {
		t.Errorf("expected ExecutePipeline's inputData to carry the raw HL7 message, got %v", fake.gotInputData)
	}
}

// fakeMessageParser is a minimal MessageParser test double.
type fakeMessageParser struct {
	parsedJSON     map[string]interface{}
	err            error
	gotInterfaceID string
	gotRawContent  string
}

func (f *fakeMessageParser) ParseToJSON(ctx context.Context, messageID, interfaceID, rawContent string) (*models.ParserResult, error) {
	f.gotInterfaceID = interfaceID
	f.gotRawContent = rawContent
	if f.err != nil {
		return nil, f.err
	}
	return &models.ParserResult{Success: true, ParsedJSON: f.parsedJSON}, nil
}

func TestHostQuery_Integration_WiredMessageParser_ExecutePipelineGetsParsedJSON(t *testing.T) {
	// TC-HQ-021: when a MessageParser is wired (the real deployment shape —
	// processing/engine.go injects the shared *services.MessageParserService),
	// the answering pipeline receives the SAME canonical enhancedSegments
	// shape every other HL7 pipeline in this codebase is authored against,
	// not a bare {"raw": ...} map — proving a real pipeline author could use
	// an ordinary "QRD.8" field-mapping step here.
	expectedReply := "MSH|^~\\&|EHK|EHK|ANALYZER|LAB|20260101120000||DSR^Q03|R1|P|2.5\r"
	pipelineFake := &fakePipelineExecutor{executeResult: hl7BuildResult(expectedReply)}
	parserFake := &fakeMessageParser{
		parsedJSON: map[string]interface{}{
			"enhancedSegments": map[string]interface{}{
				"QRD": map[string]interface{}{"fields": []interface{}{
					map[string]interface{}{"key": "QRD.8", "value": "SAMPLE001"},
				}},
			},
		},
	}

	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-77",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(pipelineFake)
	c.SetMessageParser(parserFake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL21")))

	reply := readMLLPResponse(t, conn)
	if reply != expectedReply {
		t.Errorf("expected the pipeline's own reply, got %q", reply)
	}

	if parserFake.gotInterfaceID != "iface-77" {
		t.Errorf("expected ParseToJSON called with interfaceID iface-77, got %q", parserFake.gotInterfaceID)
	}
	if parserFake.gotRawContent == "" {
		t.Error("expected ParseToJSON to receive the raw HL7 message")
	}
	// The critical assertion: ExecutePipeline must receive the PARSED shape
	// (enhancedSegments), not the bare {"raw": ...} fallback.
	if _, ok := pipelineFake.gotInputData["enhancedSegments"]; !ok {
		t.Errorf("expected ExecutePipeline's inputData to be the parsed JSON (enhancedSegments present), got %v", pipelineFake.gotInputData)
	}
}

func TestHostQuery_Integration_MessageParserFailure_FallsBackToRaw(t *testing.T) {
	// TC-HQ-022: a parse error must not crash or hang the connection — it
	// falls back to the bare {"raw": ...} shape so a pipeline that does its
	// own raw-string handling (e.g. an enrichment.script step) still works.
	expectedReply := "MSH|^~\\&|EHK|EHK|ANALYZER|LAB|20260101120000||DSR^Q03|R1|P|2.5\r"
	pipelineFake := &fakePipelineExecutor{executeResult: hl7BuildResult(expectedReply)}
	parserFake := &fakeMessageParser{err: errors.New("unsupported format")}

	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(pipelineFake)
	c.SetMessageParser(parserFake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL22")))

	reply := readMLLPResponse(t, conn)
	if reply != expectedReply {
		t.Errorf("expected the pipeline's own reply despite the parse failure, got %q", reply)
	}
	if raw, ok := pipelineFake.gotInputData["raw"].(string); !ok || raw == "" {
		t.Errorf("expected ExecutePipeline's inputData to fall back to {raw: ...}, got %v", pipelineFake.gotInputData)
	}
}

func TestHostQuery_Integration_NoPipelineExecutorWiredSendsNACK(t *testing.T) {
	// TC-HQ-015: host_query enabled but SetPipelineExecutor was never
	// called — must NACK, never hang or crash.
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true},
	})
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL4")))

	reply := readMLLPResponse(t, conn)
	_, msa := parseACKSegments(reply)
	if len(msa) < 2 || msa[1] != "AE" {
		t.Errorf("expected AE NACK when no pipeline executor is wired, got MSA=%v", msa)
	}
}

func TestHostQuery_Integration_MissingInterfaceIDSendsNACK(t *testing.T) {
	// TC-HQ-016: host_query enabled, executor wired, but interface_id was
	// never present in config — must NACK rather than call GetPipeline
	// with an empty interfaceID.
	fake := &fakePipelineExecutor{}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"host_query": map[string]interface{}{"enabled": true},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL5")))

	reply := readMLLPResponse(t, conn)
	_, msa := parseACKSegments(reply)
	if len(msa) < 2 || msa[1] != "AE" {
		t.Errorf("expected AE NACK when interface_id is unknown, got MSA=%v", msa)
	}
	if fake.gotInterfaceID != "" {
		t.Errorf("GetPipeline should never have been called, but got interfaceID=%q", fake.gotInterfaceID)
	}
}

func TestHostQuery_Integration_PipelineLookupFailureSendsNACK(t *testing.T) {
	// TC-HQ-017: GetPipeline itself errors (e.g. no "QRY" pipeline
	// configured for this interface) — must NACK, not crash or hang.
	fake := &fakePipelineExecutor{getPipelineErr: errors.New("no pipeline configured")}
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
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL6")))

	reply := readMLLPResponse(t, conn)
	_, msa := parseACKSegments(reply)
	if len(msa) < 2 || msa[1] != "AE" {
		t.Errorf("expected AE NACK when pipeline lookup fails, got MSA=%v", msa)
	}
}

func TestHostQuery_Integration_NoHL7BuildOutputSendsNACK(t *testing.T) {
	// TC-HQ-018: the pipeline runs to completion but never produced an
	// hl7.build output (misconfigured pipeline) — must NACK rather than
	// send an empty/garbage frame.
	fake := &fakePipelineExecutor{executeResult: &models.TransformationExecutionResult{Status: "completed"}}
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
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL7")))

	reply := readMLLPResponse(t, conn)
	_, msa := parseACKSegments(reply)
	if len(msa) < 2 || msa[1] != "AE" {
		t.Errorf("expected AE NACK when the pipeline produced no hl7.build output, got MSA=%v", msa)
	}
}

func TestHostQuery_Integration_LaterStepFailureAfterSuccessfulBuildStillAnswers(t *testing.T) {
	// TC-HQ-023: a REAL bug found this session via the device-connect wizard
	// — a pipeline that answers a host query AND (for a real ORU message)
	// delivers results to a HIS in the SAME run (the common "one pipeline,
	// GetPipeline's own message-type fallback" shape a device template
	// naturally produces) must not have a perfectly good, already-built
	// DSR^Q03 reply discarded just because some LATER, query-irrelevant
	// step (e.g. results delivery) failed afterward. ExecutePipeline
	// returns its PARTIAL result (ExecutionLog populated up to the
	// failure) alongside a non-nil error — ensure handleHostQuery actually
	// uses that partial result instead of NACKing on any non-nil error.
	expectedReply := "MSH|^~\\&|EHK|EHK|ANALYZER|LAB|20260101120000||DSR^Q03|R1|P|2.5\r"
	fake := &fakePipelineExecutor{
		// hl7.build (the reply) already succeeded; the overall run still
		// errors because a LATER step (e.g. "Deliver Results to HIS")
		// failed — exactly what a real failed outbound delivery produces.
		executeResult: hl7BuildResult(expectedReply),
		executeErr:    errors.New("pipeline failed at step Deliver Results to HIS: outbound delivery failed"),
	}
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
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL-LATERFAIL")))

	reply := readMLLPResponse(t, conn)
	if reply != expectedReply {
		t.Errorf("expected the already-built DSR reply despite the later delivery failure, got %q", reply)
	}
}

func TestHostQuery_Integration_TimeoutSendsNACK(t *testing.T) {
	// TC-HQ-019: the pipeline hangs (e.g. a slow HIS REST call) longer than
	// the configured timeout — the connection must NACK rather than block
	// the sender indefinitely.
	fake := &fakePipelineExecutor{executeDelay: 3 * time.Second, executeResult: hl7BuildResult("SHOULD_NOT_ARRIVE")}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true, "timeout_seconds": 1},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL8")))

	start := time.Now()
	reply := readMLLPResponse(t, conn)
	elapsed := time.Since(start)

	if elapsed > 2500*time.Millisecond {
		t.Errorf("expected the NACK within ~1s of the configured timeout, took %s", elapsed)
	}
	_, msa := parseACKSegments(reply)
	if len(msa) < 2 || msa[1] != "AE" {
		t.Errorf("expected AE NACK on timeout, got MSA=%v", msa)
	}
}

func TestHostQuery_Integration_OnFailureNackDisabledSendsNoResponse(t *testing.T) {
	// TC-HQ-020: on_failure_nack: false means a failed query gets NO
	// response at all — a deliberate opt-out, not a bug — verified by
	// confirming the read genuinely times out rather than receiving bytes.
	fake := &fakePipelineExecutor{getPipelineErr: errors.New("no pipeline configured")}
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"interface_id": "iface-1",
		"host_query":   map[string]interface{}{"enabled": true, "on_failure_nack": false},
	})
	c.SetPipelineExecutor(fake)
	_, stop := startConnector(t, c)
	defer stop()

	conn := dialLocal(t, port)
	defer conn.Close()
	conn.Write(buildMLLPFrame(hostQueryHL7("CTRL9")))

	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 16)
	if n, err := conn.Read(buf); err == nil {
		t.Errorf("expected no response (on_failure_nack disabled), but got %d bytes: %q", n, buf[:n])
	}
}

// ensure config marshalling round-trips a nested array the way Initialize expects
func TestHostQuery_ConfigJSONRoundTrip_MessageTypesArray(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"port": 0,
		"host_query": map[string]interface{}{
			"enabled":       true,
			"message_types": []string{"QRY", "QBP"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c := NewTCPMLLPInboundConnector().(*TCPMLLPInboundConnector)
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !c.hostQueryConfig.Enabled {
		t.Error("expected host_query.enabled to be true")
	}
	if len(c.hostQueryConfig.MessageTypes) != 2 || c.hostQueryConfig.MessageTypes[0] != "QRY" || c.hostQueryConfig.MessageTypes[1] != "QBP" {
		t.Errorf("expected [QRY QBP], got %v", c.hostQueryConfig.MessageTypes)
	}
}
