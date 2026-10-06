// services/connectors/astm_tcp_inbound.go
// ASTM over TCP/IP Inbound Connector — a genuinely new connector type, not a
// protocol-mode switch on tcp_mllp_inbound (mirrors the AS2 Phase 4
// precedent cited in this feature's own plan doc): MLLP's framing has no
// connection-establishment handshake before data; ASTM's does (ENQ/ACK),
// which would force two incompatible state machines behind one config flag.
//
// Same LISTEN pattern as tcp_mllp_inbound.go — net.Listen + accept loop +
// per-connection goroutine, with the real shutdown mechanism being a direct
// Close() of the listener and every active connection (confirmed by reading
// tcp_mllp_inbound.go's own Stop() directly — the base class's stopCh is
// effectively vestigial for this connector shape since Start() never calls
// the base's own Start()). Reuses Phase A's own astm_framing.go handshake
// UNCHANGED, now applied over the real net.Conn from Accept() instead of the
// serial port's io.ReadWriteCloser — the whole reason that helper was
// written against a plain io.ReadWriter in Phase A.
//
// Configuration:
//
//	port               int     Listener port (required)
//	max_connections    int     (default: 10)
//	read_timeout_sec    int     Per-connection idle read timeout (default: 300)
//	checksum_severity  string  "error" | "warning" (default: "error") — see ASTMFramingConfig
//	initiate_role      string  "receiver" (default, the only mode implemented —
//	                           the instrument connects in and sends ENQ first,
//	                           the common real-world case) | "initiator" — named,
//	                           explicitly NOT implemented this phase (see Validate);
//	                           a config value accepted now so a future phase can
//	                           add it without a config-schema migration
package connectors

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// ASTMTCPInboundConnector listens for TCP connections and plays ASTM's
// receiver role on each one.
type ASTMTCPInboundConnector struct {
	*BaseInboundConnector

	port           int
	maxConnections int
	readTimeout    time.Duration
	framing        ASTMFramingConfig
	initiateRole   string

	listener        net.Listener
	connectionMutex sync.RWMutex
	activeConns     map[string]net.Conn
	connectionCount int
}

// NewASTMTCPInboundConnector creates a production ASTM-over-TCP/IP inbound
// connector.
func NewASTMTCPInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "astm_tcp_inbound",
		DisplayName:        "ASTM over TCP/IP",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron": false,
		},
	}
	return &ASTMTCPInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		activeConns:          make(map[string]net.Conn),
	}
}

// Initialize parses configuration.
func (c *ASTMTCPInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.port = cfg.GetInt("port")

	c.maxConnections = cfg.GetInt("max_connections")
	if c.maxConnections == 0 {
		c.maxConnections = 10
	}

	readSec := cfg.GetInt("read_timeout_sec")
	if readSec == 0 {
		readSec = 300
	}
	c.readTimeout = time.Duration(readSec) * time.Second

	c.framing = ASTMFramingConfig{
		ChecksumSeverity: cfg.GetString("checksum_severity"),
	}

	c.initiateRole = cfg.GetString("initiate_role")
	if c.initiateRole == "" {
		c.initiateRole = "receiver"
	}

	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	return nil
}

// Validate checks configuration validity.
func (c *ASTMTCPInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if c.port == 0 {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("port is required"), false)
	}
	if c.initiateRole != "receiver" {
		// "initiator" (the host sends ENQ first, pulling data from a
		// listening instrument) is a named, deliberately NOT-yet-implemented
		// mode — see this file's own header comment. Rejecting it clearly
		// here is the same "never silently fabricate behavior" discipline
		// this codebase applies everywhere else (e.g. AS2's own deferred
		// async-MDN item), rather than accepting the config and quietly
		// behaving like "receiver" anyway.
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("initiate_role %q is not yet implemented — only \"receiver\" (the instrument connects in and sends ENQ first) is supported today", c.initiateRole), false)
	}
	return nil
}

// TestConnection opens and immediately closes a listener on the configured
// port — proves the port was free to bind a moment ago, nothing about
// whether a real instrument has ever reached it (the same caveat every
// other listener-style connector's own TestConnection carries in this
// codebase).
func (c *ASTMTCPInboundConnector) TestConnection(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return listener.Close()
}

// Start opens the listener and accepts connections in a goroutine.
func (c *ASTMTCPInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if c.IsRunning() {
		return ErrConnectorAlreadyRunning
	}
	if err := c.Validate(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		c.RecordError(err)
		return NewConnectorError(c.GetMetadata().TypeName, "start", err, true)
	}
	c.listener = listener

	c.SetState(StateRunning)
	c.SetConnected(true)
	log.Printf("📟 ASTM/TCP Inbound: Listening on port %d", c.port)

	go c.acceptConnections(messageChan)
	return nil
}

func (c *ASTMTCPInboundConnector) acceptConnections(messageChan chan<- *models.InboundMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ASTM/TCP Inbound: panic in acceptConnections: %v", r)
			c.SetState(StateError)
		}
	}()

	for {
		c.connectionMutex.RLock()
		current := c.connectionCount
		c.connectionMutex.RUnlock()
		if current >= c.maxConnections {
			time.Sleep(time.Second)
			continue
		}

		conn, err := c.listener.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			c.RecordError(err)
			log.Printf("❌ ASTM/TCP Inbound: accept error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		connID := conn.RemoteAddr().String()
		c.connectionMutex.Lock()
		c.activeConns[connID] = conn
		c.connectionCount++
		total := c.connectionCount
		c.connectionMutex.Unlock()

		log.Printf("📥 ASTM/TCP Inbound: new connection from %s (total: %d)", connID, total)
		go c.handleConnection(conn, connID, messageChan)
	}
}

func (c *ASTMTCPInboundConnector) handleConnection(conn net.Conn, connID string, messageChan chan<- *models.InboundMessage) {
	defer func() {
		conn.Close()
		c.connectionMutex.Lock()
		delete(c.activeConns, connID)
		c.connectionCount--
		c.connectionMutex.Unlock()
		log.Printf("📤 ASTM/TCP Inbound: connection closed %s", connID)
		if r := recover(); r != nil {
			log.Printf("❌ ASTM/TCP Inbound: panic in handleConnection: %v", r)
		}
	}()

	// One connection may carry multiple ENQ...EOT message exchanges in
	// sequence (the same "keep the session open across messages" convention
	// MLLP uses) — loop until the peer closes the connection or a read error
	// occurs. bufReader is constructed ONCE for this connection's entire
	// lifetime, never per-call — see ReceiveMessage's own doc comment for
	// why a fresh bufio.Reader on every call would silently lose bytes the
	// instrument already sent for the NEXT message (a real, found-and-fixed
	// deadlock, not theoretical — this exact bug reproduced in this file's
	// own multi-exchange test before the fix).
	bufReader := bufio.NewReader(conn)
	for {
		conn.SetReadDeadline(time.Now().Add(c.readTimeout))
		content, err := ReceiveMessage(bufReader, conn, c.framing)
		if err != nil {
			return // connection closed or errored — handled by the defer above
		}

		msg := &models.InboundMessage{
			MessageID:      generateASTMMessageID("astm_tcp"),
			Content:        content,
			SourceType:     "astm_tcp",
			SourceEndpoint: fmt.Sprintf("%d", c.port),
			ReceivedAt:     time.Now(),
			SourceMetadata: map[string]string{
				"remote_addr": connID,
				"port":        fmt.Sprintf("%d", c.port),
			},
		}
		messageChan <- msg
		c.IncrementMessagesReceived()
	}
}

// Stop closes the listener and every active connection — the real shutdown
// mechanism (mirrors tcp_mllp_inbound.go's own Stop() exactly).
func (c *ASTMTCPInboundConnector) Stop() error {
	if c.listener != nil {
		c.listener.Close()
	}

	c.connectionMutex.Lock()
	for connID, conn := range c.activeConns {
		conn.Close()
		log.Printf("🔌 ASTM/TCP Inbound: closing connection %s", connID)
	}
	c.activeConns = make(map[string]net.Conn)
	c.connectionCount = 0
	c.connectionMutex.Unlock()

	c.SetState(StateStopped)
	c.SetConnected(false)
	return c.BaseInboundConnector.Stop()
}

// Close releases all resources.
func (c *ASTMTCPInboundConnector) Close() error {
	c.Stop()
	return c.BaseInboundConnector.Close()
}
