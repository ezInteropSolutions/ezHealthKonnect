package connectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// findFreePort is already defined in tcp_mllp_ack_test.go (same package) —
// reused directly here rather than duplicated.

func TestASTMTCPInbound_Initialize_PortRequired(t *testing.T) {
	c := NewASTMTCPInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize should not itself fail on missing port: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when port is missing")
	}
}

func TestASTMTCPInbound_Validate_RejectsUnimplementedInitiatorRole(t *testing.T) {
	c := NewASTMTCPInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port": 19999, "initiate_role": "initiator"})
	c.Initialize(cfg)
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject initiate_role=initiator (not yet implemented)")
	}
}

func TestASTMTCPInboundOutbound_RealRoundTrip(t *testing.T) {
	port := findFreePort(t)

	inbound := NewASTMTCPInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port": port})
	if err := inbound.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := inbound.Start(ctx, msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer inbound.Stop()

	time.Sleep(100 * time.Millisecond) // let the listener actually bind

	outbound := NewASTMTCPOutboundConnector()
	outCfg, _ := json.Marshal(map[string]interface{}{"host": "127.0.0.1", "port": port})
	if err := outbound.Initialize(outCfg); err != nil {
		t.Fatalf("outbound Initialize: %v", err)
	}

	content := "H|\\^&|MSGCTRL001||ANALYZER-1^1.0^SN12345|||||LIS||P|E1394-97|20231004120000\r" +
		"P|1||MRN12345||Doe^Jane||19800101|F\r" +
		"L|1|N\r"

	result, err := outbound.Send(context.Background(), &models.OutboundMessage{
		MessageID: "test-1",
		Content:   content,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected successful delivery, got: %+v", result)
	}

	select {
	case msg := <-msgChan:
		if msg.Content != content {
			t.Errorf("received content mismatch:\n got:  %q\n want: %q", msg.Content, content)
		}
		if msg.SourceType != "astm_tcp" {
			t.Errorf("SourceType = %q, want astm_tcp", msg.SourceType)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message on channel")
	}
}

func TestASTMTCPOutbound_Validate_RequiresHostAndPort(t *testing.T) {
	c := NewASTMTCPOutboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{})
	c.Initialize(cfg)
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when host/port are missing")
	}
}

func TestASTMTCPInbound_Stop_ClosesListenerAndActiveConnections(t *testing.T) {
	port := findFreePort(t)

	inboundIface := NewASTMTCPInboundConnector()
	inbound := inboundIface.(*ASTMTCPInboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port": port})
	if err := inbound.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := inbound.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Hold a real, still-open connection — never completes the ASTM
	// handshake, mirroring an instrument that connected but hasn't sent
	// anything yet.
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()
	time.Sleep(100 * time.Millisecond) // let the server-side accept loop register it

	if err := inbound.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The held connection must be closed server-side — a Read on the client
	// end should now return EOF/closed, not hang or keep succeeding.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, readErr := conn.Read(buf)
	if readErr == nil {
		t.Error("expected the held connection to be closed by Stop(), but Read succeeded")
	}

	// A fresh dial to the same port should fail — the listener itself is closed.
	if _, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second); err == nil {
		t.Error("expected the listener to be closed after Stop(), but a new connection succeeded")
	}
}

func TestASTMTCPInbound_MaxConnections_LimitsConcurrentConnections(t *testing.T) {
	port := findFreePort(t)

	inboundIface := NewASTMTCPInboundConnector()
	inbound := inboundIface.(*ASTMTCPInboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port": port, "max_connections": 1})
	if err := inbound.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := inbound.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer inbound.Stop()
	time.Sleep(100 * time.Millisecond)

	conn1, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer conn1.Close()
	time.Sleep(200 * time.Millisecond) // let the accept loop register connection 1

	inbound.connectionMutex.RLock()
	count := inbound.connectionCount
	inbound.connectionMutex.RUnlock()
	if count != 1 {
		t.Fatalf("expected connectionCount=1 after the first connection, got %d", count)
	}

	// A second TCP dial still succeeds at the OS level (the kernel accepts
	// into the listen backlog regardless of application-level limits), but
	// the connector's own accept loop must not register it as tracked while
	// at the configured max_connections.
	conn2, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err == nil {
		defer conn2.Close()
	}
	time.Sleep(300 * time.Millisecond) // well under the accept loop's own 1s backoff-and-recheck

	inbound.connectionMutex.RLock()
	countAfter := inbound.connectionCount
	inbound.connectionMutex.RUnlock()
	if countAfter > 1 {
		t.Errorf("expected connectionCount to stay at the configured max_connections=1, got %d", countAfter)
	}
}

func TestASTMTCP_OneConnection_MultipleSequentialExchanges(t *testing.T) {
	port := findFreePort(t)

	inboundIface := NewASTMTCPInboundConnector()
	inbound := inboundIface.(*ASTMTCPInboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port": port})
	if err := inbound.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 10)
	if err := inbound.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer inbound.Stop()
	time.Sleep(100 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	// Two full ENQ...EOT exchanges over the SAME connection — a real
	// instrument keeping its session open across multiple messages, the
	// scenario this connector's own handleConnection loop exists for. ONE
	// bufio.Reader reused across both SendMessage calls — this is the exact
	// shape that caught a real deadlock before ReceiveMessage/SendMessage
	// were fixed to take a caller-owned, persistent reader.
	bufReader := bufio.NewReader(conn)
	if err := SendMessage(bufReader, conn, "H|\\^&|M1\rL|1|N\r", ASTMFramingConfig{}); err != nil {
		t.Fatalf("first SendMessage on the shared connection: %v", err)
	}
	if err := SendMessage(bufReader, conn, "H|\\^&|M2\rL|1|N\r", ASTMFramingConfig{}); err != nil {
		t.Fatalf("second SendMessage on the SAME, still-open connection: %v", err)
	}

	var received []string
	for i := 0; i < 2; i++ {
		select {
		case msg := <-msgChan:
			received = append(received, msg.Content)
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for message %d of 2", i+1)
		}
	}

	// Each received message is the FULL content sent in that exchange (both
	// its H and L records), not just the H record alone — the assertion
	// below matches what was actually sent, not an under-specified guess.
	if len(received) != 2 || received[0] != "H|\\^&|M1\rL|1|N\r" || received[1] != "H|\\^&|M2\rL|1|N\r" {
		t.Errorf("expected 2 distinct messages from the same connection, got %#v", received)
	}
}

// TestASTMTCP_ConcurrentConnections_EachGetsAUniqueMessageID is the
// permanent regression guard for a real, found-and-fixed bug:
// generateASTMMessageID (astm_framing.go) used to increment a shared
// package-level int64 counter with a plain `++` — an unguarded data race
// the moment two ASTM connections received content concurrently, which
// could silently produce two real InboundMessages with the SAME MessageID
// (message_id has no uniqueness constraint in messages_intf_*, confirmed
// via InterfaceTableManager.js — so a collision wouldn't even fail loudly,
// just corrupt correlation). Fixed by switching to uuid.New(), which has no
// shared mutable state to race on. This test drives N real, concurrent TCP
// connections — not just a single connection's own sequential messages
// (already covered by TestASTMTCP_OneConnection_MultipleSequentialExchanges)
// — and asserts every received MessageID is genuinely distinct.
func TestASTMTCP_ConcurrentConnections_EachGetsAUniqueMessageID(t *testing.T) {
	port := findFreePort(t)

	inboundIface := NewASTMTCPInboundConnector()
	inbound := inboundIface.(*ASTMTCPInboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port": port, "max_connections": 20})
	if err := inbound.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 50)
	if err := inbound.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer inbound.Stop()
	time.Sleep(100 * time.Millisecond)

	const concurrency = 10
	done := make(chan struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			outbound := NewASTMTCPOutboundConnector()
			outCfg, _ := json.Marshal(map[string]interface{}{"host": "127.0.0.1", "port": port})
			outbound.Initialize(outCfg)
			outbound.Send(context.Background(), &models.OutboundMessage{
				MessageID: fmt.Sprintf("test-%d", n),
				Content:   fmt.Sprintf("H|\\^&|M%d\rL|1|N\r", n),
			})
		}(i)
	}
	for i := 0; i < concurrency; i++ {
		<-done
	}

	seen := make(map[string]bool, concurrency)
	for i := 0; i < concurrency; i++ {
		select {
		case msg := <-msgChan:
			if seen[msg.MessageID] {
				t.Fatalf("duplicate MessageID %q received — the generateASTMMessageID race regressed", msg.MessageID)
			}
			seen[msg.MessageID] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for message %d of %d", i+1, concurrency)
		}
	}
	if len(seen) != concurrency {
		t.Errorf("expected %d distinct message IDs, got %d", concurrency, len(seen))
	}
}

func TestASTMTCPOutbound_Send_ConnectionRefused_ReturnsFailureNotPanic(t *testing.T) {
	c := NewASTMTCPOutboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"host": "127.0.0.1", "port": 1}) // port 1 — nothing listening
	c.Initialize(cfg)

	result, err := c.Send(context.Background(), &models.OutboundMessage{MessageID: "x", Content: "H|\\^&|\rL|1|N\r"})
	if err == nil {
		t.Error("expected an error when nothing is listening")
	}
	if result == nil || result.Success {
		t.Errorf("expected a failed DeliveryResult, got %+v", result)
	}
}
