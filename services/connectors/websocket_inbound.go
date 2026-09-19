// services/connectors/websocket_inbound.go
// WebSocket Inbound Connector — a real-time server accepting websocket
// connections from external systems and enqueuing each received frame as an
// InboundMessage.
//
// Architecture note (why this mirrors tcp_mllp_inbound.go's Start() shape,
// NOT as2_inbound.go's/http_rest_inbound.go's): both of those connectors'
// Start() methods block on `<-ctx.Done()` before calling Stop(). The engine
// (processing/engine.go) always passes context.Background() to Start() and
// never cancels it, so that blocking goroutine leaks for the lifetime of the
// process — harmless only because DeactivateInterface calls connector.Stop()
// on a separate direct path, bypassing the blocked goroutine entirely. This
// connector avoids that leak by having Start() launch its server in a
// goroutine and return immediately, exactly like tcp_mllp_inbound.go does.
//
// Second subtlety: gorilla/websocket's Upgrader.Upgrade() hijacks the
// connection out of net/http's own tracking, so http.Server.Shutdown()
// (graceful drain) silently does NOT close or wait for any already-upgraded
// websocket connection. Stop() below therefore uses server.Close() (immediate)
// plus an explicit close of every tracked *websocket.Conn — the same
// two-step shutdown tcp_mllp_inbound.go uses for its own raw net.Conn map.
//
// Named, deferred scope decision: no pipeline-driven synchronous reply frame
// is sent back to the client after a message is enqueued. Doing so would
// require blocking this connection's goroutine on full async pipeline
// completion (processMessages runs asynchronously off the message channel,
// processing/engine.go) — a genuinely new blocking-inbound architecture no
// existing connector in this codebase has, even MLLP's ACK is only a
// queue-accept acknowledgment, not a post-pipeline result. A future phase
// could add this; not attempted here.
package connectors

import (
	"crypto/subtle"
	"crypto/tls"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/gorilla/websocket"
)

const (
	// defaultMaxWSMessageBytes/hardCapMaxWSMessageBytes mirror the same
	// default/cap convention tcp_mllp_inbound.go uses for its own
	// max_message_size_mb — generous headroom while still bounding memory
	// use against a sender that never stops streaming a single frame.
	defaultMaxWSMessageBytes = 10 * 1024 * 1024
	hardCapMaxWSMessageBytes = 100 * 1024 * 1024
)

// WebSocketInboundConnector implements a real-time websocket server listener.
type WebSocketInboundConnector struct {
	*BaseInboundConnector

	port            int
	basePath        string
	enableTLS       bool
	tlsCertFile     string
	tlsKeyFile      string
	maxMessageBytes int
	readTimeout     time.Duration
	writeTimeout    time.Duration
	pingInterval    time.Duration
	maxConnections  int
	authType        string // "none" | "bearer" | "basic"
	bearerToken     string
	username        string
	password        string

	upgrader websocket.Upgrader
	server   *http.Server

	connectionMutex sync.RWMutex
	activeConns     map[string]*websocket.Conn
	connectionCount int
}

// NewWebSocketInboundConnector replaces the (nonexistent) stub — this
// connector type is 100% new, confirmed via a full grep of
// connector_stubs.go before writing this file.
func NewWebSocketInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "websocket_inbound",
		DisplayName:        "WebSocket Server",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  false,
			"supports_tls":   true,
			"supports_auth":  true,
			"supports_batch": false,
			"generates_ack":  false, // websocket has no MLLP-style mandatory response
		},
	}
	return &WebSocketInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		activeConns:          make(map[string]*websocket.Conn),
	}
}

// Initialize parses configuration.
func (c *WebSocketInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.port = cfg.GetInt("port")
	if c.port == 0 {
		return fmt.Errorf("port is required")
	}

	c.basePath = cfg.GetString("base_path")
	if c.basePath == "" {
		c.basePath = "/ws"
	}
	if !strings.HasPrefix(c.basePath, "/") {
		c.basePath = "/" + c.basePath
	}

	c.enableTLS = cfg.GetBool("tls_enabled")
	c.tlsCertFile = cfg.GetString("tls_cert_file")
	c.tlsKeyFile = cfg.GetString("tls_key_file")

	maxMB := cfg.GetInt("max_message_size_mb")
	if maxMB <= 0 {
		c.maxMessageBytes = defaultMaxWSMessageBytes
	} else {
		c.maxMessageBytes = maxMB * 1024 * 1024
		if c.maxMessageBytes > hardCapMaxWSMessageBytes {
			c.maxMessageBytes = hardCapMaxWSMessageBytes
		}
	}

	readSec := cfg.GetInt("read_timeout_seconds")
	if readSec <= 0 {
		readSec = 60
	}
	c.readTimeout = time.Duration(readSec) * time.Second

	writeSec := cfg.GetInt("write_timeout_seconds")
	if writeSec <= 0 {
		writeSec = 10
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	pingSec := cfg.GetInt("ping_interval_seconds")
	if pingSec <= 0 {
		pingSec = 30
	}
	c.pingInterval = time.Duration(pingSec) * time.Second

	c.maxConnections = cfg.GetInt("max_connections")
	if c.maxConnections <= 0 {
		c.maxConnections = 100
	}

	c.authType = cfg.GetString("authentication_type")
	if c.authType == "" {
		c.authType = "none"
	}
	c.bearerToken = cfg.GetString("bearer_token")
	c.username = cfg.GetString("username")
	c.password = cfg.GetString("password")

	c.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		// Same-network HL7/FHIR integration traffic, not browser CORS —
		// this project's other listeners (tcp_mllp, http_rest) have no
		// Origin concept at all, so this matches the existing trust model
		// rather than introducing a new one.
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("base_path", c.basePath)
	c.SetMetadata("tls_enabled", fmt.Sprintf("%t", c.enableTLS))

	log.Printf("✅ WebSocket Inbound initialized: port=%d path=%s auth=%s", c.port, c.basePath, c.authType)
	return nil
}

// Validate checks configuration validity.
func (c *WebSocketInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if c.port <= 0 || c.port > 65535 {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("invalid port: %d (must be 1-65535)", c.port), false)
	}
	if c.enableTLS && (c.tlsCertFile == "" || c.tlsKeyFile == "") {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("tls_enabled is true but tls_cert_file/tls_key_file are not set"), false)
	}
	switch c.authType {
	case "none", "bearer", "basic":
	default:
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("invalid authentication_type: %s", c.authType), false)
	}
	if c.authType == "bearer" && c.bearerToken == "" {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("bearer authentication requires bearer_token"), false)
	}
	if c.authType == "basic" && (c.username == "" || c.password == "") {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("basic authentication requires username and password"), false)
	}
	return nil
}

// TestConnection verifies the configured port can be bound.
func (c *WebSocketInboundConnector) TestConnection(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", c.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return NewConnectorError(c.metadata.TypeName, "test_connection", err, true)
	}
	return ln.Close()
}

// SupportsCron returns false — this is a push (event-driven) listener.
func (c *WebSocketInboundConnector) SupportsCron() bool { return false }

// Start launches the websocket server. Returns immediately once the server
// goroutine is launched — does NOT block on ctx.Done() (see file header).
func (c *WebSocketInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if err := c.Validate(); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(c.basePath, func(w http.ResponseWriter, r *http.Request) {
		c.handleUpgrade(w, r, messageChan)
	})

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.port),
		Handler: mux,
	}
	if c.enableTLS {
		c.server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	go func() {
		var err error
		if c.enableTLS {
			log.Printf("✅ WebSocket Inbound: Listening on port %d%s (TLS enabled)", c.port, c.basePath)
			err = c.server.ListenAndServeTLS(c.tlsCertFile, c.tlsKeyFile)
		} else {
			log.Printf("✅ WebSocket Inbound: Listening on port %d%s (TLS disabled)", c.port, c.basePath)
			err = c.server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("❌ WebSocket Inbound: server error: %v", err)
			c.RecordError(err)
		}
	}()

	c.SetState(StateRunning)
	c.SetConnected(true)
	return nil
}

// checkAuth validates the upgrade request's Authorization header (when
// authentication_type is not "none"). The trust decision here is a shared
// secret over the connection, same as tcp_mllp_inbound's basic-auth option —
// TLS is the transport confidentiality layer, this is the credential check.
func (c *WebSocketInboundConnector) checkAuth(r *http.Request) bool {
	switch c.authType {
	case "bearer":
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		return strings.HasPrefix(auth, "Bearer ") &&
			subtle.ConstantTimeCompare([]byte(token), []byte(c.bearerToken)) == 1
	case "basic":
		u, p, ok := r.BasicAuth()
		return ok &&
			subtle.ConstantTimeCompare([]byte(u), []byte(c.username)) == 1 &&
			subtle.ConstantTimeCompare([]byte(p), []byte(c.password)) == 1
	default: // "none"
		return true
	}
}

// handleUpgrade authenticates, capacity-gates, and upgrades one incoming
// HTTP request to a websocket connection.
func (c *WebSocketInboundConnector) handleUpgrade(w http.ResponseWriter, r *http.Request, messageChan chan<- *models.InboundMessage) {
	if !c.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	c.connectionMutex.RLock()
	atCapacity := c.connectionCount >= c.maxConnections
	c.connectionMutex.RUnlock()
	if atCapacity {
		log.Printf("⚠️ WebSocket Inbound: Max connections reached (%d), rejecting upgrade from %s", c.maxConnections, r.RemoteAddr)
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket Inbound: Upgrade failed from %s: %v", r.RemoteAddr, err)
		return
	}
	conn.SetReadLimit(int64(c.maxMessageBytes))

	connID := fmt.Sprintf("%s-%d", r.RemoteAddr, time.Now().UnixNano())
	c.connectionMutex.Lock()
	c.activeConns[connID] = conn
	c.connectionCount++
	c.connectionMutex.Unlock()

	log.Printf("📥 WebSocket Inbound: New connection from %s (total: %d)", r.RemoteAddr, c.connectionCount)

	go c.handleConnection(conn, connID, r.RemoteAddr, messageChan)
}

// handleConnection reads frames from one websocket connection until it
// closes or Stop() force-closes it, enqueuing each as an InboundMessage.
func (c *WebSocketInboundConnector) handleConnection(conn *websocket.Conn, connID, remoteAddr string, messageChan chan<- *models.InboundMessage) {
	defer func() {
		conn.Close()
		c.connectionMutex.Lock()
		// Only decrement if this connection is still tracked — Stop() may
		// have already reset activeConns/connectionCount to empty/0 (it
		// force-closes every tracked conn, which is what unblocks this very
		// goroutine's ReadMessage() call), and decrementing again here would
		// otherwise race Stop() into a spurious negative count.
		if _, stillTracked := c.activeConns[connID]; stillTracked {
			delete(c.activeConns, connID)
			c.connectionCount--
		}
		c.connectionMutex.Unlock()
		log.Printf("📤 WebSocket Inbound: Connection closed %s (total: %d)", remoteAddr, c.connectionCount)

		if r := recover(); r != nil {
			log.Printf("❌ WebSocket Inbound: Panic in handleConnection: %v", r)
		}
	}()

	conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(c.readTimeout))
		return nil
	})

	stopPing := make(chan struct{})
	defer close(stopPing)
	if c.pingInterval > 0 {
		go func() {
			ticker := time.NewTicker(c.pingInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				case <-stopPing:
					return
				}
			}
		}()
	}

	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("⚠️ WebSocket Inbound: Unexpected close from %s: %v", remoteAddr, err)
			} else {
				log.Printf("🔌 WebSocket Inbound: Client disconnected: %s", remoteAddr)
			}
			return
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue // control frames (ping/pong/close) are handled by gorilla/the handlers above
		}

		log.Printf("📨 WebSocket Inbound: Received %d bytes from %s", len(payload), remoteAddr)

		inboundMsg := &models.InboundMessage{
			MessageID:      fmt.Sprintf("ws_%s_%d", connID, time.Now().UnixNano()),
			Content:        string(payload),
			SourceType:     "websocket",
			SourceEndpoint: fmt.Sprintf("ws://0.0.0.0:%d%s", c.port, c.basePath),
			SourceIP:       hostOnly(remoteAddr),
			ReceivedAt:     time.Now(),
			MessageSize:    len(payload),
		}

		select {
		case messageChan <- inboundMsg:
			c.IncrementMessagesReceived()
			log.Printf("✅ WebSocket Inbound: Message queued for processing: %s", inboundMsg.MessageID)
		case <-time.After(5 * time.Second):
			log.Printf("⚠️ WebSocket Inbound: Message channel full, dropping message from %s", remoteAddr)
		case <-c.GetStopChannel():
			return
		}
	}
}

// hostOnly strips the port from a "host:port" remote address string,
// tolerating a bare host (no port) if that's ever what's passed in.
func hostOnly(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// Stop closes the server and every tracked connection. server.Close() (not
// Shutdown()) is used deliberately — see the file header comment on why
// Shutdown() would silently skip already-upgraded (hijacked) connections.
func (c *WebSocketInboundConnector) Stop() error {
	if c.server != nil {
		log.Printf("🛑 WebSocket Inbound: Stopping server...")
		c.server.Close()
	}

	c.connectionMutex.Lock()
	for connID, conn := range c.activeConns {
		log.Printf("🔌 WebSocket Inbound: Closing connection %s", connID)
		conn.Close()
	}
	c.activeConns = make(map[string]*websocket.Conn)
	c.connectionCount = 0
	c.connectionMutex.Unlock()

	c.SetState(StateStopped)
	c.SetConnected(false)
	log.Printf("✅ WebSocket Inbound: Stopped successfully")

	return c.BaseInboundConnector.Stop()
}

// Close releases all resources.
func (c *WebSocketInboundConnector) Close() error {
	c.Stop()
	return c.BaseInboundConnector.Close()
}
