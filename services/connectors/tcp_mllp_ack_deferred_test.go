package connectors

// Deferred MLLP acknowledgements (ack.mode after_store / after_processing).
// A small fake engine stands in for the processing engine: it reads the
// connector's channel and reports feedback / outcomes the way the engine does.

import (
	"context"
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// mllpFakeEngine consumes the connector's channel and calls react for every message.
func mllpFakeEngine(msgChan chan *models.InboundMessage, react func(m *models.InboundMessage)) {
	go func() {
		for m := range msgChan {
			react(m)
		}
	}()
}

func dialMLLP(t *testing.T, port int) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// ackOf reads one ACK and returns MSA-1 and MSA-2.
func ackOf(t *testing.T, conn net.Conn, within time.Duration) (code, controlID string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(within))
	msg, err := ReadMLLPFrameFrom(bufio.NewReader(conn), 1<<20, false)
	if err != nil {
		t.Fatalf("no ACK within %s: %v", within, err)
	}
	_, msa := parseACKSegments(msg)
	if len(msa) < 3 {
		t.Fatalf("bad ACK: %q", msg)
	}
	return msa[1], msa[2]
}

// deferredConnector starts a connector in the given mode.
func deferredConnector(t *testing.T, mode string, extraAck map[string]interface{}) (*TCPMLLPInboundConnector, int, chan *models.InboundMessage) {
	t.Helper()
	port := findFreePort(t)
	ack := map[string]interface{}{"mode": mode}
	for k, v := range extraAck {
		ack[k] = v
	}
	c := newConnectorOnPort(t, port, map[string]interface{}{"ack": ack})
	msgChan, stop := startConnector(t, c)
	t.Cleanup(stop)
	return c, port, msgChan
}

func feedback(c *TCPMLLPInboundConnector, id, status string) {
	var errs []models.FieldValidationError
	if status == "rejected" {
		errs = []models.FieldValidationError{{Field: "MSH.9", Type: "X", Message: "no"}}
	}
	_ = c.SendValidationResponse(context.Background(), models.NewValidationFeedback(id, "i", models.ValidationModeStrictReject, status, errs, 0, ""))
}

func TestDeferredACK_DefaultsAreAfterStoreAnd30s(t *testing.T) {
	c := NewTCPMLLPInboundConnector().(*TCPMLLPInboundConnector)
	if err := c.Initialize([]byte(`{"port": 0}`)); err != nil {
		t.Fatal(err)
	}
	if c.ackConfig.Mode != ACKModeAfterStore || c.ackConfig.Wait != 30*time.Second {
		t.Fatalf("defaults: mode=%q wait=%s", c.ackConfig.Mode, c.ackConfig.Wait)
	}
}

func TestDeferredACK_ConfigNormalisation(t *testing.T) {
	for in, want := range map[string]string{
		"": ACKModeAfterStore, "none": ACKModeAfterStore, "bogus": ACKModeAfterStore,
		"immediate": ACKModeImmediate, "after_store": ACKModeAfterStore, "after_processing": ACKModeAfterProcessing,
	} {
		if got := normalizeMLLPACKMode(in); got != want {
			t.Errorf("normalizeMLLPACKMode(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[int]time.Duration{0: 30 * time.Second, -5: 30 * time.Second, 5: 5 * time.Second, 99999: 10 * time.Minute} {
		if got := clampACKWait(in); got != want {
			t.Errorf("clampACKWait(%d)=%s want %s", in, got, want)
		}
	}
	c := newConnectorOnPort(t, 0, map[string]interface{}{"ack": map[string]interface{}{"mode": "after_processing", "wait_seconds": 7}})
	if c.ackConfig.Mode != ACKModeAfterProcessing || c.ackConfig.Wait != 7*time.Second {
		t.Fatalf("configured: mode=%q wait=%s", c.ackConfig.Mode, c.ackConfig.Wait)
	}
}

func TestDeferredACK_AfterStore_AcceptedRejectedAndStoreFailure(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterStore, nil)
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		switch {
		case strings.HasPrefix(m.CorrelationID, "OK"):
			feedback(c, m.MessageID, "accepted")
			// the pipeline outcome that follows must change nothing
			c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeFailed, Error: "late failure"})
		case strings.HasPrefix(m.CorrelationID, "REJ"):
			feedback(c, m.MessageID, "rejected")
			c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeRejected})
		case strings.HasPrefix(m.CorrelationID, "STORE"):
			// the engine reports a store failure only as an outcome
			c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeFailed, Error: "db down"})
		}
	})
	conn := dialMLLP(t, port)
	for _, tc := range []struct{ id, want string }{{"OK1", "AA"}, {"REJ1", "AE"}, {"STORE1", "AE"}, {"OK2", "AA"}} {
		conn.Write(buildMLLPFrame(sampleHL7(tc.id, "ADT^A01")))
		code, ctrl := ackOf(t, conn, 3*time.Second)
		if code != tc.want || ctrl != tc.id {
			t.Errorf("%s: got %s/%s want %s", tc.id, code, ctrl, tc.want)
		}
	}
	if n := c.tracker().size(); n != 0 {
		t.Errorf("tracker leaked %d entries", n)
	}
}

func TestDeferredACK_AfterProcessing_WaitsForTheOutcome(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterProcessing, nil)
	release := make(chan struct{})
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		feedback(c, m.MessageID, "accepted") // storing is not enough in this mode
		<-release
		status := models.OutcomeProcessed
		switch m.CorrelationID {
		case "FAIL":
			status = models.OutcomeFailed
		case "SKIP":
			status = models.OutcomeSkipped
		}
		c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: status})
	})
	conn := dialMLLP(t, port)
	conn.Write(buildMLLPFrame(sampleHL7("P1", "ADT^A01")))

	// Stored but not processed: no ACK yet.
	conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatal("ACK arrived before the pipeline finished")
	}
	release <- struct{}{}
	if code, ctrl := ackOf(t, conn, 3*time.Second); code != "AA" || ctrl != "P1" {
		t.Errorf("processed: %s/%s", code, ctrl)
	}
	for _, tc := range []struct{ id, want string }{{"SKIP", "AA"}, {"FAIL", "AE"}} {
		go func() { release <- struct{}{} }()
		conn.Write(buildMLLPFrame(sampleHL7(tc.id, "ADT^A01")))
		if code, ctrl := ackOf(t, conn, 3*time.Second); code != tc.want || ctrl != tc.id {
			t.Errorf("%s: got %s/%s want %s", tc.id, code, ctrl, tc.want)
		}
	}
}

func TestDeferredACK_SignalBeforeTheWaitBeginsIsNotLost(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterProcessing, nil)
	// Answers inside the same instant the message leaves the channel.
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeProcessed})
	})
	conn := dialMLLP(t, port)
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("Q%d", i)
		conn.Write(buildMLLPFrame(sampleHL7(id, "ADT^A01")))
		if code, ctrl := ackOf(t, conn, 3*time.Second); code != "AA" || ctrl != id {
			t.Fatalf("%s: %s/%s", id, code, ctrl)
		}
	}
}

func TestDeferredACK_NoVerdictAnswersAEAfterTheWait(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterStore, map[string]interface{}{"wait_seconds": 1})
	mllpFakeEngine(ch, func(m *models.InboundMessage) {}) // engine never answers
	conn := dialMLLP(t, port)
	start := time.Now()
	conn.Write(buildMLLPFrame(sampleHL7("SLOW", "ADT^A01")))
	code, ctrl := ackOf(t, conn, 4*time.Second)
	if code != "AE" || ctrl != "SLOW" {
		t.Fatalf("got %s/%s", code, ctrl)
	}
	if d := time.Since(start); d < 900*time.Millisecond || d > 2500*time.Millisecond {
		t.Errorf("AE after %s, want about 1s", d)
	}
	if n := c.tracker().size(); n != 0 {
		t.Errorf("tracker leaked %d entries", n)
	}
	// A verdict that arrives afterwards is ignored quietly.
	c.OnMessageOutcome(models.MessageOutcome{MessageID: "gone", Status: models.OutcomeProcessed})
}

func TestDeferredACK_ImmediateModeIsUnchanged(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeImmediate, nil)
	_ = ch // nobody consumes: the AA must not depend on the engine
	conn := dialMLLP(t, port)
	conn.Write(buildMLLPFrame(sampleHL7("IMM", "ADT^A01")))
	if code, ctrl := ackOf(t, conn, time.Second); code != "AA" || ctrl != "IMM" {
		t.Fatalf("got %s/%s", code, ctrl)
	}
	// Late engine signals are no-ops and no longer errors.
	var m *models.InboundMessage = <-ch
	if err := c.SendValidationResponse(context.Background(), models.NewValidationFeedback(m.MessageID, "i", models.ValidationModeStrictReject, "rejected", nil, 0, "")); err != nil {
		t.Errorf("feedback in immediate mode: %v", err)
	}
	c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeFailed})
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Error("a second reply was sent")
	}
	if n := c.tracker().size(); n != 0 {
		t.Errorf("immediate mode tracked %d frames", n)
	}
}

func TestDeferredACK_SplitFrame(t *testing.T) {
	frame := func(a, b string) string { return sampleHL7(a, "ADT^A01") + sampleHL7(b, "ADT^A01") }
	if ExpectedHL7Parts(frame("A", "B")) != 2 || ExpectedHL7Parts(sampleHL7("A", "")) != 1 {
		t.Fatal("ExpectedHL7Parts")
	}
	c, port, ch := deferredConnector(t, ACKModeAfterStore, map[string]interface{}{"wait_seconds": 2})
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		// The engine names the parts <frame id>-part1, -part2.
		switch m.CorrelationID {
		case "S1": // both parts stored
			feedback(c, m.MessageID+"-part1", "accepted")
			feedback(c, m.MessageID+"-part2", "accepted")
		case "S2": // second part fails
			feedback(c, m.MessageID+"-part1", "accepted")
			c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID + "-part2", Status: models.OutcomeFailed})
		case "S3": // only one part ever reported
			feedback(c, m.MessageID+"-part1", "accepted")
		}
	})
	conn := dialMLLP(t, port)
	for _, tc := range []struct{ id, want string }{{"S1", "AA"}, {"S2", "AE"}, {"S3", "AE"}} {
		conn.Write(buildMLLPFrame(frame(tc.id, tc.id+"b")))
		if code, ctrl := ackOf(t, conn, 4*time.Second); code != tc.want || ctrl != tc.id {
			t.Errorf("%s: got %s/%s want %s", tc.id, code, ctrl, tc.want)
		}
	}
}

func TestDeferredACK_ConnectionDroppedWhileWaiting(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterProcessing, nil)
	got := make(chan *models.InboundMessage, 1)
	mllpFakeEngine(ch, func(m *models.InboundMessage) { got <- m })
	conn := dialMLLP(t, port)
	conn.Write(buildMLLPFrame(sampleHL7("DROP", "ADT^A01")))
	m := <-got
	conn.Close() // the sender gives up
	c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeProcessed})
	time.Sleep(200 * time.Millisecond)
	if n := c.tracker().size(); n != 0 {
		t.Errorf("tracker leaked %d entries", n)
	}
	// The listener still serves other senders.
	conn2 := dialMLLP(t, port)
	conn2.Write(buildMLLPFrame(sampleHL7("NEXT", "ADT^A01")))
	m2 := <-got
	c.OnMessageOutcome(models.MessageOutcome{MessageID: m2.MessageID, Status: models.OutcomeProcessed})
	if code, ctrl := ackOf(t, conn2, 2*time.Second); code != "AA" || ctrl != "NEXT" {
		t.Errorf("%s/%s", code, ctrl)
	}
}

func TestDeferredACK_StopReleasesWaiters(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterStore, nil)
	got := make(chan struct{}, 1)
	mllpFakeEngine(ch, func(m *models.InboundMessage) { got <- struct{}{} })
	conn := dialMLLP(t, port)
	conn.Write(buildMLLPFrame(sampleHL7("STOP", "ADT^A01")))
	<-got
	if c.tracker().size() != 1 {
		t.Fatal("frame should be waiting")
	}
	c.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for c.tracker().size() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := c.tracker().size(); n != 0 {
		t.Errorf("tracker leaked %d entries after Stop", n)
	}
}

func TestDeferredACK_QueueFullStillAnswersAE(t *testing.T) {
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{"ack": map[string]interface{}{"mode": ACKModeAfterStore}})
	ctxChan := make(chan *models.InboundMessage) // unbuffered, never read: always full
	go func() { c.Start(context.Background(), ctxChan) }()
	time.Sleep(150 * time.Millisecond)
	t.Cleanup(func() { c.Stop() })
	conn := dialMLLP(t, port)
	conn.Write(buildMLLPFrame(sampleHL7("FULL", "ADT^A01")))
	if code, _ := ackOf(t, conn, 8*time.Second); code != "AE" {
		t.Errorf("got %s", code)
	}
	if n := c.tracker().size(); n != 0 {
		t.Errorf("tracker leaked %d entries", n)
	}
}

// A burst on one connection: every message acknowledged once, in order.
func TestDeferredACK_BurstOnOneConnectionInOrder(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterProcessing, nil)
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		feedback(c, m.MessageID, "accepted")
		c.OnMessageOutcome(models.MessageOutcome{MessageID: m.MessageID, Status: models.OutcomeProcessed})
	})
	conn := dialMLLP(t, port)
	const n = 1000
	go func() {
		for i := 0; i < n; i++ {
			conn.Write(buildMLLPFrame(sampleHL7(fmt.Sprintf("B%04d", i), "ADT^A01")))
		}
	}()
	reader := bufio.NewReader(conn)
	for i := 0; i < n; i++ {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		msg, err := ReadMLLPFrameFrom(reader, 1<<20, false)
		if err != nil {
			t.Fatalf("ACK %d: %v", i, err)
		}
		_, msa := parseACKSegments(msg)
		if msa[1] != "AA" || msa[2] != fmt.Sprintf("B%04d", i) {
			t.Fatalf("ACK %d: %v", i, msa)
		}
	}
	if sz := c.tracker().size(); sz != 0 {
		t.Errorf("tracker leaked %d entries", sz)
	}
}

// Several senders at once, with the engine answering out of order.
func TestDeferredACK_ParallelSendersEngineAnswersOutOfOrder(t *testing.T) {
	c, port, ch := deferredConnector(t, ACKModeAfterStore, nil)
	var wg sync.WaitGroup
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		wg.Add(1)
		go func() { // each message finishes on its own schedule
			defer wg.Done()
			time.Sleep(time.Duration(len(m.CorrelationID)*7%13) * time.Millisecond)
			feedback(c, m.MessageID, "accepted")
		}()
	})
	const senders, per = 8, 125
	var senderWG sync.WaitGroup
	for s := 0; s < senders; s++ {
		senderWG.Add(1)
		go func(s int) {
			defer senderWG.Done()
			conn := dialMLLP(t, port)
			for i := 0; i < per; i++ {
				id := fmt.Sprintf("S%dM%d", s, i)
				conn.Write(buildMLLPFrame(sampleHL7(id, "ADT^A01")))
				if code, ctrl := ackOf(t, conn, 5*time.Second); code != "AA" || ctrl != id {
					t.Errorf("%s: %s/%s", id, code, ctrl)
					return
				}
			}
		}(s)
	}
	senderWG.Wait()
	wg.Wait()
	if sz := c.tracker().size(); sz != 0 {
		t.Errorf("tracker leaked %d entries", sz)
	}
}

// With a low connection limit and an engine that takes a few milliseconds per
// message, senders that close after their ACK must not each wait a full second
// for a free slot (the accept loop used to sleep a second at the limit).
func TestDeferredACK_ConnectionLimitDoesNotStallSenders(t *testing.T) {
	port := findFreePort(t)
	c := newConnectorOnPort(t, port, map[string]interface{}{
		"max_connections": 2,
		"ack":             map[string]interface{}{"mode": ACKModeAfterStore},
	})
	ch, stop := startConnector(t, c)
	t.Cleanup(stop)
	mllpFakeEngine(ch, func(m *models.InboundMessage) {
		time.Sleep(15 * time.Millisecond)
		feedback(c, m.MessageID, "accepted")
	})
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn := dialMLLP(t, port)
			id := fmt.Sprintf("L%d", i)
			conn.Write(buildMLLPFrame(sampleHL7(id, "ADT^A01")))
			if code, ctrl := ackOf(t, conn, 5*time.Second); code != "AA" || ctrl != id {
				t.Errorf("%s: %s/%s", id, code, ctrl)
			}
			conn.Close()
		}(i)
	}
	wg.Wait()
	if d := time.Since(start); d > 900*time.Millisecond {
		t.Errorf("10 senders through a limit of 2 took %s; they should not wait a second each", d)
	}
}
