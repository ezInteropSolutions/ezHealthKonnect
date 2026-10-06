// services/connectors/tcp_mllp_inbound.go
// TCP/MLLP Inbound Connector - HL7 v2.x Message Listener

package connectors

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"ezhealthkonnect/models"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// MLLP protocol constants
const (
	MLLPStartByte byte = 0x0B // Vertical Tab (VT)
	MLLPEndByte1  byte = 0x1C // File Separator (FS)
	MLLPEndByte2  byte = 0x0D // Carriage Return (CR)

	// defaultMaxMLLPMessageBytes bounds a single MLLP frame's size. HL7 v2 messages are
	// almost always a few KB, even with embedded base64 (e.g. OBX image data) staying
	// under a few MB; 10 MB gives generous headroom while still bounding memory use
	// against a sender that never emits the end-of-frame marker.
	defaultMaxMLLPMessageBytes = 10 * 1024 * 1024
	// hardCapMaxMLLPMessageBytes is the ceiling max_message_size_mb can be configured to.
	hardCapMaxMLLPMessageBytes = 100 * 1024 * 1024
)

// errMLLPFrameTooLarge is returned by readMLLPFrame when a frame exceeds the configured
// size limit before the end-of-frame marker is found. handleConnection treats this as
// fatal for the connection (not merely a bad message) because the reader's position is
// left mid-frame with no way to safely resynchronise to the next start byte.
var errMLLPFrameTooLarge = errors.New("mllp frame exceeds configured maximum message size")

// ConnectionInfo stores connection details for validation feedback
type ConnectionInfo struct {
	Conn          net.Conn
	OriginalMsg   string
	ConnID        string
	ReceivedAt    time.Time
}

// ACKConfig holds acknowledgment behavior configuration.
//
// MLLP is a request/response protocol: every received message ALWAYS gets an ACK or NACK.
// There is no option to suppress the response — doing so would leave the sender blocking.
type ACKConfig struct {
	// Mode controls the acknowledgment code sent:
	//   "immediate" (default) — AA sent as soon as the message is accepted onto the queue
	// Note: "none" is no longer supported; omitting the ACK violates the MLLP protocol.
	Mode            string
	OnError         string // "suppress" (default) → always AA; "nack" → AE on queue-full
	SendingApp      string // MSH-3 in ACK response
	SendingFacility string // MSH-4 in ACK response
	TextSuccess     string // MSA-3 text on success
	TextError       string // MSA-3 text on error/nack
	Script          string // Optional JS: function buildACK(msg) { return {ackCode, textMessage} }
}

// HostQueryConfig configures synchronous host-query handling — e.g. Mindray
// lab analyzers whose LIS Interface Manual documents them sending a live
// QRY^Q02 asking "what's ordered for this sample" and expecting a real
// DSR^Q03 reply on the SAME connection, not just an MSA ack. Disabled by
// default: a connector with no "host_query" config behaves EXACTLY as
// before (async enqueue + fixed ACK) — zero behavior change for every
// existing deployment unless explicitly opted in.
type HostQueryConfig struct {
	Enabled bool
	// MessageTypes are MSH.9.1 values (e.g. "QRY") treated as a live query
	// this connector must answer synchronously, rather than enqueue.
	MessageTypes []string
	// PipelineKey is the message_type a matching query resolves its
	// answering pipeline by (PipelineExecutor.GetPipeline's own 3rd arg) —
	// configurable rather than hardcoded, since a future device's own query
	// message type may differ from Mindray's "QRY".
	PipelineKey string
	// Timeout bounds how long this connection's goroutine blocks waiting for
	// the live pipeline round trip (e.g. an outbound REST call into the
	// HIS) before giving up and NACKing — see handleHostQuery's own doc
	// comment for the same "genuinely hung work keeps running in its own
	// goroutine" limitation controllers/sync_eligibility_controller.go
	// already names for its own synchronous pipeline call.
	Timeout time.Duration
	// OnFailureNACK sends a NACK when the pipeline can't be resolved, times
	// out, or fails — true by default, matching this connector's own
	// standing "MLLP is request/response; never leave a sender without a
	// response" rule.
	OnFailureNACK bool
}

// TCPMLLPInboundConnector implements HL7 MLLP protocol listener
type TCPMLLPInboundConnector struct {
	*BaseInboundConnector
	listener         net.Listener
	port             int
	enableTLS        bool
	tlsConfig        *tls.Config
	maxConnections   int
	connectionCount  int
	connectionMutex  sync.RWMutex
	activeConns      map[string]net.Conn
	maxMessageBytes  int
	readTimeout      time.Duration
	writeTimeout     time.Duration
	keepAlive        bool
	keepAlivePeriod  time.Duration
	authType         string
	authUsername     string
	authPassword     string
	validateChecksum bool
	ackConfig        ACKConfig

	// interfaceID is read from config's own "interface_id" key (injected by
	// processing/engine.go before Initialize is called, the same key
	// processing/connectors.go's extractInterfaceContext already reads for
	// SetInterfaceContext) — kept here, not just handed to the logger, so
	// handleHostQuery can resolve this connector's own pipeline by
	// (interfaceID, messageType) without any new plumbing beyond a config read.
	interfaceID      string
	pipelineExecutor PipelineExecutor
	messageParser    MessageParser
	hostQueryConfig  HostQueryConfig

	// Validation feedback support
	messageConns      map[string]*ConnectionInfo // messageID -> connection info (for validation feedback)
	messageConnsMutex sync.RWMutex
}

// NewTCPMLLPInboundConnector creates a new TCP/MLLP inbound connector
func NewTCPMLLPInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "tcp_mllp_inbound",
		DisplayName:        "TCP/MLLP (HL7 v2.x) Listener",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  false,
			"supports_tls":   true,
			"supports_auth":  true,
			"supports_batch": false,
			"generates_ack":  true,
		},
	}

	base := NewBaseInboundConnector(metadata)
	return &TCPMLLPInboundConnector{
		BaseInboundConnector: base,
		activeConns:          make(map[string]net.Conn),
		messageConns:         make(map[string]*ConnectionInfo),
	}
}

// Initialize prepares the connector with its configuration
func (c *TCPMLLPInboundConnector) Initialize(config []byte) error {
	// Call base initialization
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}

	// Parse connector-specific config
	cfg := c.GetConfig()
	c.port = cfg.GetInt("port")
	if c.port == 0 {
		c.port = 2575 // Default MLLP port
	}

	// interface_id is injected into this same config blob by
	// processing/engine.go before Initialize runs (see
	// processing/connectors.go's extractInterfaceContext, which reads the
	// identical key for SetInterfaceContext) — read directly rather than
	// waiting on SetInterfaceContext, which only ever threads it into the
	// logger, not a retrievable field.
	c.interfaceID = cfg.GetString("interface_id")

	// TLS defaults to disabled. Set enable_tls: true with certificate_file + key_file
	// to enable TLS. A warning is printed when TLS is off to remind operators.
	c.enableTLS = cfg.GetBoolDefault("enable_tls", false)
	if !c.enableTLS {
		log.Printf("⚠️  SECURITY NOTE: TLS is DISABLED on MLLP listener (port %d). Set enable_tls: true in production.", c.port)
	}

	c.maxConnections = cfg.GetInt("max_connections")
	if c.maxConnections == 0 {
		c.maxConnections = 100
	}

	// Max MLLP frame size — bounds memory when a sender never emits the end-of-frame
	// marker (DoS via unbounded stream). Configurable in MB; 0/unset uses the default.
	maxMessageSizeMB := cfg.GetInt("max_message_size_mb")
	if maxMessageSizeMB <= 0 {
		c.maxMessageBytes = defaultMaxMLLPMessageBytes
	} else if maxMessageSizeMB*1024*1024 > hardCapMaxMLLPMessageBytes {
		c.maxMessageBytes = hardCapMaxMLLPMessageBytes
	} else {
		c.maxMessageBytes = maxMessageSizeMB * 1024 * 1024
	}

	// Timeout configurations
	readTimeoutSec := cfg.GetInt("read_timeout_seconds")
	if readTimeoutSec == 0 {
		readTimeoutSec = 300 // 5 minutes default
	}
	c.readTimeout = time.Duration(readTimeoutSec) * time.Second

	writeTimeoutSec := cfg.GetInt("write_timeout_seconds")
	if writeTimeoutSec == 0 {
		writeTimeoutSec = 30
	}
	c.writeTimeout = time.Duration(writeTimeoutSec) * time.Second

	// Keep-alive configuration
	c.keepAlive = cfg.GetBool("keep_alive")
	keepAliveSec := cfg.GetInt("keep_alive_period_seconds")
	if keepAliveSec == 0 {
		keepAliveSec = 60
	}
	c.keepAlivePeriod = time.Duration(keepAliveSec) * time.Second

	// Authentication
	c.authType = cfg.GetString("authentication_type")
	if c.authType == "" {
		c.authType = "none"
	}
	c.authUsername = cfg.GetString("username")
	c.authPassword = cfg.GetString("password")

	c.validateChecksum = cfg.GetBool("validate_checksum")

	// ACK behaviour — read from nested "ack" sub-object in config.
	// "mode: none" is no longer valid; MLLP requires every message to receive a response.
	// Old configs that set mode=none are silently upgraded to mode=immediate.
	ackMap := cfg.GetMap("ack")
	ackMode := getStringFromMap(ackMap, "mode", "immediate")
	if ackMode == "none" {
		log.Printf("⚠️  TCP/MLLP Inbound: ack.mode='none' is not valid — MLLP requires a response for every message. Defaulting to 'immediate'.")
		ackMode = "immediate"
	}
	c.ackConfig = ACKConfig{
		Mode:            ackMode,
		OnError:         getStringFromMap(ackMap, "on_error", "suppress"),
		SendingApp:      getStringFromMap(ackMap, "sending_app", "ezHealthKonnect"),
		SendingFacility: getStringFromMap(ackMap, "sending_facility", "EHK"),
		TextSuccess:     getStringFromMap(ackMap, "text_success", "Message received successfully"),
		TextError:       getStringFromMap(ackMap, "text_error", "Message processing error"),
		Script:          getStringFromMap(ackMap, "script", ""),
	}

	// Host-query behaviour — read from nested "host_query" sub-object,
	// mirroring "ack"'s own nested-object convention. Disabled unless
	// explicitly turned on (see HostQueryConfig's own doc comment).
	hostQueryMap := cfg.GetMap("host_query")
	timeoutSeconds := getIntFromMap(hostQueryMap, "timeout_seconds", 10)
	c.hostQueryConfig = HostQueryConfig{
		Enabled:       getBoolFromMap(hostQueryMap, "enabled", false),
		MessageTypes:  getStringSliceFromMap(hostQueryMap, "message_types", []string{"QRY"}),
		PipelineKey:   getStringFromMap(hostQueryMap, "pipeline_message_type", "QRY"),
		Timeout:       time.Duration(timeoutSeconds) * time.Second,
		OnFailureNACK: getBoolFromMap(hostQueryMap, "on_failure_nack", true),
	}

	// Setup TLS if enabled
	if c.enableTLS {
		tlsVersion := cfg.GetString("tls_version")
		c.tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		switch tlsVersion {
		case "TLS_1_3":
			c.tlsConfig.MinVersion = tls.VersionTLS13
		case "TLS_1_2":
			c.tlsConfig.MinVersion = tls.VersionTLS12
		}

		// Certificate configuration
		certFile := cfg.GetString("certificate_file")
		keyFile := cfg.GetString("key_file")
		if certFile != "" && keyFile != "" {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return NewConnectorError(c.metadata.TypeName, "tls_setup", err, false)
			}
			c.tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("tls_enabled", fmt.Sprintf("%t", c.enableTLS))
	c.SetMetadata("max_connections", fmt.Sprintf("%d", c.maxConnections))

	return nil
}

// Validate checks configuration validity
func (c *TCPMLLPInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}

	// Validate port
	if c.port <= 0 || c.port > 65535 {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("invalid port: %d (must be 1-65535)", c.port), false)
	}

	// Validate TLS configuration: TLS is on by default. When enabled, cert and
	// key files are required — self-signed certs are not provisioned automatically
	// because they are not acceptable in production healthcare environments.
	// Operators must obtain a cert from their CA (or use their EHR vendor's cert)
	// and set certificate_file + key_file in the connector config.
	if c.enableTLS && (c.tlsConfig == nil || len(c.tlsConfig.Certificates) == 0) {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf(
				"TLS is enabled but no certificate is configured. "+
					"Set certificate_file and key_file in the connector config, "+
					"or set enable_tls: false to disable TLS (not recommended for production)",
			), false)
	}

	// Validate authentication
	if c.authType != "none" && c.authType != "basic" && c.authType != "token" {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("invalid authentication_type: %s", c.authType), false)
	}

	if c.authType == "basic" && (c.authUsername == "" || c.authPassword == "") {
		return NewConnectorError(c.metadata.TypeName, "validate",
			fmt.Errorf("basic authentication requires username and password"), false)
	}

	return nil
}

// TestConnection verifies the port can be opened
func (c *TCPMLLPInboundConnector) TestConnection(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", c.port)

	var listener net.Listener
	var err error

	if c.enableTLS {
		listener, err = tls.Listen("tcp", addr, c.tlsConfig)
	} else {
		listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return NewConnectorError(c.metadata.TypeName, "test_connection", err, true)
	}

	listener.Close()
	return nil
}

// Start begins listening for MLLP connections
func (c *TCPMLLPInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	// Check if already running
	if c.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	// Validate before starting
	if err := c.Validate(); err != nil {
		return err
	}

	// Open listener
	addr := fmt.Sprintf(":%d", c.port)
	var err error

	if c.enableTLS {
		c.listener, err = tls.Listen("tcp", addr, c.tlsConfig)
		log.Printf("✅ TCP/MLLP Inbound: Listening on port %d (TLS enabled)", c.port)
	} else {
		c.listener, err = net.Listen("tcp", addr)
		log.Printf("✅ TCP/MLLP Inbound: Listening on port %d (TLS disabled)", c.port)
	}

	if err != nil {
		c.RecordError(err)
		return NewConnectorError(c.metadata.TypeName, "start", err, true)
	}

	c.SetState(StateRunning)
	c.SetConnected(true)

	// Accept connections in goroutine
	go c.acceptConnections(ctx, messageChan)

	return nil
}

// acceptConnections handles incoming connections
func (c *TCPMLLPInboundConnector) acceptConnections(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ TCP/MLLP Inbound: Panic in acceptConnections: %v", r)
			c.SetState(StateError)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("🛑 TCP/MLLP Inbound: Context cancelled, stopping listener")
			return
		case <-c.GetStopChannel():
			log.Printf("🛑 TCP/MLLP Inbound: Stop signal received")
			return
		default:
			// Check connection limit
			c.connectionMutex.RLock()
			currentCount := c.connectionCount
			c.connectionMutex.RUnlock()

			if currentCount >= c.maxConnections {
				log.Printf("⚠️ TCP/MLLP Inbound: Max connections reached (%d), rejecting new connections", c.maxConnections)
				time.Sleep(1 * time.Second)
				continue
			}

			// Accept connection
			conn, err := c.listener.Accept()
			if err != nil {
				// Check if listener was closed
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				c.RecordError(err)
				log.Printf("❌ TCP/MLLP Inbound: Accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Configure connection
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if c.keepAlive {
					tcpConn.SetKeepAlive(true)
					tcpConn.SetKeepAlivePeriod(c.keepAlivePeriod)
				}
			}

			// Track connection
			connID := conn.RemoteAddr().String()
			c.connectionMutex.Lock()
			c.activeConns[connID] = conn
			c.connectionCount++
			c.connectionMutex.Unlock()

			log.Printf("📥 TCP/MLLP Inbound: New connection from %s (total: %d)", connID, c.connectionCount)

			// Handle connection in separate goroutine
			go c.handleConnection(conn, connID, messageChan)
		}
	}
}

// handleConnection processes a single MLLP connection
func (c *TCPMLLPInboundConnector) handleConnection(conn net.Conn, connID string, messageChan chan<- *models.InboundMessage) {
	defer func() {
		conn.Close()
		c.connectionMutex.Lock()
		delete(c.activeConns, connID)
		c.connectionCount--
		c.connectionMutex.Unlock()
		log.Printf("📤 TCP/MLLP Inbound: Connection closed %s (total: %d)", connID, c.connectionCount)

		if r := recover(); r != nil {
			log.Printf("❌ TCP/MLLP Inbound: Panic in handleConnection: %v", r)
		}
	}()

	reader := bufio.NewReader(conn)

	for {
		// Set read timeout
		conn.SetReadDeadline(time.Now().Add(c.readTimeout))

		// Read MLLP frame
		hl7Message, err := c.readMLLPFrame(reader)
		if err != nil {
			if err == io.EOF {
				log.Printf("🔌 TCP/MLLP Inbound: Client disconnected: %s", connID)
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("⏱️ TCP/MLLP Inbound: Read timeout for %s", connID)
				return
			}
			if errors.Is(err, errMLLPFrameTooLarge) {
				// Reader position is now mid-frame with no safe way to resynchronise
				// to the next start byte — close the connection rather than risk
				// misinterpreting the remaining bytes of the oversized frame as a
				// new message.
				log.Printf("🚨 TCP/MLLP Inbound: Frame from %s exceeded max size (%d bytes) — closing connection",
					connID, c.maxMessageBytes)
				c.sendNACK(conn, "Message exceeds maximum allowed size")
				return
			}
			log.Printf("❌ TCP/MLLP Inbound: Read error from %s: %v", connID, err)
			c.sendNACK(conn, "Error reading message")
			continue
		}

		if len(hl7Message) == 0 {
			continue
		}

		log.Printf("📨 TCP/MLLP Inbound: Received %d bytes from %s", len(hl7Message), connID)

		// A configured host-query message type (e.g. Mindray's QRY^Q02) is
		// answered synchronously on THIS connection instead of the normal
		// async enqueue+ACK path below — see handleHostQuery's own doc
		// comment. Disabled by default, so every existing deployment's
		// behavior is unchanged unless host_query.enabled is explicitly set.
		if c.hostQueryConfig.Enabled && c.isHostQueryMessage(hl7Message) {
			c.handleHostQuery(conn, hl7Message)
			continue
		}

		// Create inbound message
		inboundMsg := &models.InboundMessage{
			MessageID:      fmt.Sprintf("tcp_%s_%d", connID, time.Now().UnixNano()),
			CorrelationID:  c.extractMessageControlID(hl7Message),
			Content:        hl7Message,
			SourceType:     "tcp_mllp",
			SourceEndpoint: fmt.Sprintf("tcp://0.0.0.0:%d", c.port),
			SourceIP:       strings.Split(connID, ":")[0],
			ReceivedAt:     time.Now(),
			MessageType:    c.extractMessageType(hl7Message),
			MessageSize:    len(hl7Message),
			Encoding:       "HL7",
		}

		// Send to processing pipeline.
		// ACK/NACK is ALWAYS sent — MLLP is request/response; leaving the sender
		// without a response would cause it to block indefinitely.
		select {
		case messageChan <- inboundMsg:
			c.IncrementMessagesReceived()
			log.Printf("✅ TCP/MLLP Inbound: Message queued for processing: %s", inboundMsg.MessageID)
			c.sendConfiguredACK(conn, hl7Message, "AA", c.ackConfig.TextSuccess)

		case <-time.After(5 * time.Second):
			log.Printf("⚠️ TCP/MLLP Inbound: Message channel full, sending NACK")
			if c.ackConfig.OnError == "nack" {
				c.sendConfiguredACK(conn, hl7Message, "AE", c.ackConfig.TextError)
			} else {
				// "suppress" mode: send AA anyway — caller's backpressure handles retries
				c.sendConfiguredACK(conn, hl7Message, "AA", c.ackConfig.TextSuccess)
			}
		}
	}
}

// readMLLPFrame reads an MLLP-framed message from the connection
func (c *TCPMLLPInboundConnector) readMLLPFrame(reader *bufio.Reader) (string, error) {
	// Read start byte
	startByte, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	if startByte != MLLPStartByte {
		return "", fmt.Errorf("invalid MLLP start byte: 0x%02X (expected 0x0B)", startByte)
	}

	// Read until end bytes
	var message strings.Builder
	for {
		if message.Len() >= c.maxMessageBytes {
			return "", errMLLPFrameTooLarge
		}

		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}

		if b == MLLPEndByte1 {
			// Check for CR
			nextByte, err := reader.ReadByte()
			if err != nil {
				return "", err
			}
			if nextByte == MLLPEndByte2 {
				// Valid MLLP end sequence
				break
			}
			// Not end sequence, add bytes to message
			message.WriteByte(b)
			message.WriteByte(nextByte)
		} else {
			message.WriteByte(b)
		}
	}

	return message.String(), nil
}

// extractMessageType extracts message type from HL7 MSH segment
func (c *TCPMLLPInboundConnector) extractMessageType(hl7Message string) string {
	lines := strings.Split(hl7Message, "\r")
	if len(lines) == 0 {
		return "UNKNOWN"
	}

	mshSegment := lines[0]
	if !strings.HasPrefix(mshSegment, "MSH") {
		return "UNKNOWN"
	}

	fields := strings.Split(mshSegment, "|")
	if len(fields) > 8 {
		return fields[8] // Message type field (MSH.9)
	}

	return "UNKNOWN"
}

// extractMessageControlID extracts message control ID from HL7 MSH segment
func (c *TCPMLLPInboundConnector) extractMessageControlID(hl7Message string) string {
	lines := strings.Split(hl7Message, "\r")
	if len(lines) == 0 {
		return ""
	}

	mshSegment := lines[0]
	if !strings.HasPrefix(mshSegment, "MSH") {
		return ""
	}

	fields := strings.Split(mshSegment, "|")
	if len(fields) > 9 {
		return fields[9] // Message Control ID field (MSH.10)
	}

	return ""
}

// generateACKMessage generates an HL7 ACK message using configured sender identity
func (c *TCPMLLPInboundConnector) generateACKMessage(originalMessage string, ackCode string, textMessage string) string {
	messageControlID := c.extractMessageControlID(originalMessage)
	timestamp := time.Now().Format("20060102150405")

	sendingApp := c.ackConfig.SendingApp
	if sendingApp == "" {
		sendingApp = "ezHealthKonnect"
	}
	sendingFacility := c.ackConfig.SendingFacility
	if sendingFacility == "" {
		sendingFacility = "EHK"
	}

	ack := fmt.Sprintf("MSH|^~\\&|%s|%s|SENDER|SENDER|%s||ACK|%s|P|2.5\r\n",
		sendingApp, sendingFacility, timestamp, messageControlID)
	ack += fmt.Sprintf("MSA|%s|%s|%s\r\n", ackCode, messageControlID, textMessage)

	return ack
}

// sendConfiguredACK applies ackConfig (script override, custom text, sender identity) then sends
func (c *TCPMLLPInboundConnector) sendConfiguredACK(conn net.Conn, originalMessage string, defaultCode string, defaultText string) {
	ackCode := defaultCode
	textMessage := defaultText

	// Run custom script if provided — script can override ackCode and textMessage
	if c.ackConfig.Script != "" {
		if code, text, err := c.runACKScript(originalMessage, defaultCode, defaultText); err == nil {
			ackCode = code
			textMessage = text
		} else {
			log.Printf("⚠️ TCP/MLLP ACK script error: %v — falling back to default", err)
		}
	}

	ack := c.generateACKMessage(originalMessage, ackCode, textMessage)
	if err := c.sendACK(conn, ack); err != nil {
		log.Printf("⚠️ TCP/MLLP Inbound: Failed to send ACK (%s): %v", ackCode, err)
	} else {
		log.Printf("✅ TCP/MLLP Inbound: ACK sent (%s)", ackCode)
	}
}

// runACKScript executes a user-supplied JS function:
//
//	function buildACK(msg) { return { ackCode: "AA", textMessage: "OK" }; }
//
// msg is an object with: controlID, messageType, sendingApp, sendingFacility, raw
func (c *TCPMLLPInboundConnector) runACKScript(originalMessage string, defaultCode string, defaultText string) (string, string, error) {
	vm := goja.New()

	// Expose message context to the script
	msgObj := map[string]interface{}{
		"raw":             originalMessage,
		"controlID":       c.extractMessageControlID(originalMessage),
		"messageType":     c.extractMessageType(originalMessage),
		"sendingApp":      c.extractMSHField(originalMessage, 2),
		"sendingFacility": c.extractMSHField(originalMessage, 3),
		"defaultCode":     defaultCode,
		"defaultText":     defaultText,
	}
	if err := vm.Set("msg", msgObj); err != nil {
		return defaultCode, defaultText, err
	}

	// Execute the script
	if _, err := vm.RunString(c.ackConfig.Script); err != nil {
		return defaultCode, defaultText, fmt.Errorf("script parse error: %w", err)
	}

	// Call buildACK(msg)
	buildACK, ok := goja.AssertFunction(vm.Get("buildACK"))
	if !ok {
		return defaultCode, defaultText, fmt.Errorf("script must define function buildACK(msg)")
	}

	result, err := buildACK(goja.Undefined(), vm.ToValue(msgObj))
	if err != nil {
		return defaultCode, defaultText, fmt.Errorf("script execution error: %w", err)
	}

	// Extract ackCode and textMessage from result object
	obj := result.ToObject(vm)
	if obj == nil {
		return defaultCode, defaultText, fmt.Errorf("buildACK must return an object")
	}

	ackCode := defaultCode
	textMessage := defaultText

	if v := obj.Get("ackCode"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		ackCode = v.String()
	}
	if v := obj.Get("textMessage"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		textMessage = v.String()
	}

	return ackCode, textMessage, nil
}

// extractMSHField extracts a field from the MSH segment by 0-based position after the separator
func (c *TCPMLLPInboundConnector) extractMSHField(hl7Message string, position int) string {
	lines := strings.Split(hl7Message, "\r")
	if len(lines) == 0 {
		return ""
	}
	fields := strings.Split(lines[0], "|")
	if len(fields) > position {
		return fields[position]
	}
	return ""
}

// getStringFromMap safely reads a string value from a map with a fallback default
func getStringFromMap(m map[string]interface{}, key string, defaultVal string) string {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

// getBoolFromMap safely reads a bool value from a map with a fallback default —
// the nested-map counterpart to ConnectorConfig.GetBoolDefault, for sub-objects
// (like "host_query") that aren't the connector's own top-level config map.
func getBoolFromMap(m map[string]interface{}, key string, defaultVal bool) bool {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// getIntFromMap safely reads an int value from a map with a fallback default.
// Handles JSON number (float64) and Go int forms, matching
// ConnectorConfig.GetInt's own handling for its top-level config map.
func getIntFromMap(m map[string]interface{}, key string, defaultVal int) int {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// getStringSliceFromMap safely reads a []string value from a map with a
// fallback default — the nested-map counterpart to
// ConnectorConfig.GetStringSlice.
func getStringSliceFromMap(m map[string]interface{}, key string, defaultVal []string) []string {
	if m == nil {
		return defaultVal
	}
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	slice, ok := v.([]interface{})
	if !ok {
		return defaultVal
	}
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if s, ok := item.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return defaultVal
	}
	return result
}

// splitMessageType splits a raw MSH.9 value (e.g. "QRY^Q02") into its
// message type and trigger event components. Deliberately separate from
// extractMessageType, which returns the raw unsplit value used elsewhere
// (InboundMessage.MessageType, the ACK script's msg.messageType) — changing
// that shape would be a behavior change for existing consumers; this is a
// new, narrowly-scoped helper for the host-query branch only.
func splitMessageType(raw string) (messageType string, triggerEvent string) {
	parts := strings.SplitN(raw, "^", 2)
	messageType = parts[0]
	if len(parts) > 1 {
		triggerEvent = parts[1]
	}
	return messageType, triggerEvent
}

// writeMLLPFrame wraps message in MLLP framing (VT ... FS CR) and writes it
// to conn. This is the single low-level write path every reply this
// connector ever sends goes through — an ACK, a NACK, or a live host-query
// reply (e.g. DSR^Q03) alike.
func (c *TCPMLLPInboundConnector) writeMLLPFrame(conn net.Conn, message string) error {
	conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))

	frame := fmt.Sprintf("%c%s%c%c", MLLPStartByte, message, MLLPEndByte1, MLLPEndByte2)

	_, err := conn.Write([]byte(frame))
	if err != nil {
		log.Printf("❌ TCP/MLLP Inbound: Failed to write MLLP frame: %v", err)
		return err
	}
	return nil
}

// sendACK sends an ACK message
func (c *TCPMLLPInboundConnector) sendACK(conn net.Conn, ack string) error {
	if err := c.writeMLLPFrame(conn, ack); err != nil {
		return err
	}
	log.Printf("✅ TCP/MLLP Inbound: ACK sent")
	return nil
}

// sendNACK sends a NACK message
func (c *TCPMLLPInboundConnector) sendNACK(conn net.Conn, reason string) error {
	timestamp := time.Now().Format("20060102150405")
	controlID := fmt.Sprintf("NACK%d", time.Now().UnixNano())

	nack := fmt.Sprintf("MSH|^~\\&|ezHealthKonnect|EHK|SENDER|SENDER|%s||ACK|%s|P|2.5\r\n", timestamp, controlID)
	nack += fmt.Sprintf("MSA|AE|%s|%s\r\n", controlID, reason)

	return c.sendACK(conn, nack)
}

// SetPipelineExecutor implements PipelineAwareConnector — injected by the
// connector-management layer (processing/engine.go) after Start, mirroring
// SetInterfaceContext's own "setter after construction" shape.
func (c *TCPMLLPInboundConnector) SetPipelineExecutor(pe PipelineExecutor) {
	c.pipelineExecutor = pe
}

// SetMessageParser implements MessageParserAwareConnector — same injection
// shape as SetPipelineExecutor.
func (c *TCPMLLPInboundConnector) SetMessageParser(mp MessageParser) {
	c.messageParser = mp
}

// isHostQueryMessage reports whether hl7Message's MSH.9.1 matches one of the
// configured host_query.message_types (case-insensitive — HL7 message type
// codes are conventionally uppercase, but real-world senders vary).
func (c *TCPMLLPInboundConnector) isHostQueryMessage(hl7Message string) bool {
	messageType, _ := splitMessageType(c.extractMessageType(hl7Message))
	for _, t := range c.hostQueryConfig.MessageTypes {
		if strings.EqualFold(messageType, t) {
			return true
		}
	}
	return false
}

// handleHostQuery answers a live host-query message (e.g. Mindray's
// QRY^Q02) synchronously on the SAME connection it arrived on: it resolves
// this connector's own interface pipeline keyed by host_query.pipeline_message_type,
// runs it inline with the parsed message as input, and writes back whatever
// that pipeline's own hl7.build step produced (e.g. a real DSR^Q03) — the
// same "call ExecutePipeline synchronously and return its own real output"
// pattern controllers/sync_eligibility_controller.go already uses from an
// HTTP handler, applied here from this connection's own goroutine instead.
//
// Named limitation, not silently papered over (mirrors
// sync_eligibility_controller.go's own documented caveat): ExecutePipeline
// has no internal cancellation checks in its own step loop, so a genuinely
// hung pipeline keeps running to completion in its own goroutine after this
// method's timeout gives up and NACKs — the timeout bounds how long THIS
// connection waits, not how long the pipeline itself runs.
func (c *TCPMLLPInboundConnector) handleHostQuery(conn net.Conn, hl7Message string) {
	if c.pipelineExecutor == nil {
		log.Printf("⚠️ TCP/MLLP Inbound: host_query enabled but no pipeline executor is wired — sending NACK")
		c.failHostQuery(conn, "Host query processing is not available")
		return
	}
	if c.interfaceID == "" {
		log.Printf("⚠️ TCP/MLLP Inbound: host_query enabled but interface_id is unknown — sending NACK")
		c.failHostQuery(conn, "Host query processing is not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.hostQueryConfig.Timeout)
	defer cancel()

	pipeline, err := c.pipelineExecutor.GetPipeline(ctx, c.interfaceID, c.hostQueryConfig.PipelineKey)
	if err != nil {
		log.Printf("❌ TCP/MLLP Inbound: host_query pipeline lookup failed for interface %s (%s): %v",
			c.interfaceID, c.hostQueryConfig.PipelineKey, err)
		c.failHostQuery(conn, "No pipeline configured to answer this query")
		return
	}

	// Parse to the SAME canonical JSON shape (enhancedSegments, etc.) the
	// normal async ingestion path already hands ExecutePipeline — so the
	// answering pipeline can use ordinary HL7-path field mapping (e.g.
	// "QRD.8" for the queried sample ID) exactly like any other HL7
	// pipeline in this codebase, not a bespoke raw-string parser. Falls back
	// to a bare {"raw": ...} shape when no MessageParser is wired (e.g.
	// tests) or parsing fails — a pipeline author who wants to hand-parse
	// the raw text themselves in an enrichment.script step still can.
	inputData := map[string]interface{}{"raw": hl7Message}
	if c.messageParser != nil {
		messageID := fmt.Sprintf("hostquery_%s_%d", c.interfaceID, time.Now().UnixNano())
		if parseResult, parseErr := c.messageParser.ParseToJSON(ctx, messageID, c.interfaceID, hl7Message); parseErr != nil {
			log.Printf("⚠️ TCP/MLLP Inbound: host_query message parse failed, falling back to raw input: %v", parseErr)
		} else if parseResult != nil && parseResult.ParsedJSON != nil {
			inputData = parseResult.ParsedJSON
		}
	}

	type outcome struct {
		result *models.TransformationExecutionResult
		err    error
	}
	outcomeCh := make(chan outcome, 1)
	go func() {
		result, execErr := c.pipelineExecutor.ExecutePipeline(ctx, pipeline, inputData)
		outcomeCh <- outcome{result, execErr}
	}()

	select {
	case <-ctx.Done():
		log.Printf("⏱️ TCP/MLLP Inbound: host_query pipeline timed out after %s", c.hostQueryConfig.Timeout)
		c.failHostQuery(conn, "Query processing timed out")

	case o := <-outcomeCh:
		// ExecutePipeline returns its PARTIAL result alongside a non-nil
		// error when a step fails (transformation_pipeline_helpers.go:
		// "return result, fmt.Errorf(...)"), not nil — so a later,
		// query-irrelevant step failing (e.g. a results-delivery step that
		// only matters for a real ORU result, not for answering a live
		// QRY) must not discard an hl7.build reply that ALREADY completed
		// successfully earlier in the SAME run. Check for a usable reply
		// first, regardless of o.err/o.result.Status, and only treat the
		// query as unanswerable if nothing usable was actually produced.
		var reply string
		if o.result != nil {
			reply = extractHL7BuildReply(o.result)
		}
		if reply == "" {
			log.Printf("❌ TCP/MLLP Inbound: host_query pipeline execution failed: %v", o.err)
			c.failHostQuery(conn, "Query processing failed")
			return
		}
		if o.err != nil {
			log.Printf("⚠️ TCP/MLLP Inbound: host_query pipeline reported a later error after already building a usable reply (answering anyway): %v", o.err)
		}
		if writeErr := c.writeMLLPFrame(conn, reply); writeErr != nil {
			log.Printf("⚠️ TCP/MLLP Inbound: failed to send host_query reply: %v", writeErr)
		} else {
			log.Printf("✅ TCP/MLLP Inbound: host_query reply sent (%d bytes)", len(reply))
		}
	}
}

// failHostQuery sends a NACK when host_query.on_failure_nack is enabled
// (the default) — a live query the pipeline can't honestly answer is still
// bound by this connector's own "never leave an MLLP sender without a
// response" rule.
func (c *TCPMLLPInboundConnector) failHostQuery(conn net.Conn, reason string) {
	if c.hostQueryConfig.OnFailureNACK {
		c.sendNACK(conn, reason)
	}
}

// extractHL7BuildReply pulls a completed pipeline's hl7.build step output
// (field "hl7_message" — see controllers/transformation_test_controller.go's
// own buildStepContentFields registry) so a host-query reply can be written
// back on the connection it arrived on. That registry lives in package
// controllers, which sits above services/connectors — importing it here
// would create a cycle — so this is a small, narrowly-scoped extraction
// specific to this connector's own single need, not a duplicate of that
// registry's general multi-step-type machinery. Returns the LAST matching
// step's output, matching that registry's own "final artifact wins"
// convention.
func extractHL7BuildReply(result *models.TransformationExecutionResult) string {
	if result == nil {
		return ""
	}
	normalizer := models.NewOutputNormalizer()
	var reply string
	for _, stepLog := range result.ExecutionLog {
		if !stepLog.Success || stepLog.StepType != "hl7.build" || stepLog.StepOutput == nil {
			continue
		}
		normalized := normalizer.NormalizeStepOutput(stepLog.StepOutput.OutputData)
		if msg, ok := normalized["hl7_message"].(string); ok && msg != "" {
			reply = msg
		}
	}
	return reply
}

// Stop gracefully stops the listener
func (c *TCPMLLPInboundConnector) Stop() error {
	// Always close the listener/connections even when not in StateRunning.
	// A connector can be in StateError (e.g. acceptConnections goroutine panicked)
	// while its net.Listener is still bound to the OS port.  Bailing out early
	// would leave the port occupied and cause false "address already in use" errors
	// on the next activation attempt.
	if c.IsRunning() {
		log.Printf("🛑 TCP/MLLP Inbound: Stopping listener...")
	} else {
		log.Printf("🛑 TCP/MLLP Inbound: Releasing resources (connector not running — cleaning up stale socket)...")
	}

	// Close listener
	if c.listener != nil {
		c.listener.Close()
	}

	// Close all active connections
	c.connectionMutex.Lock()
	for connID, conn := range c.activeConns {
		log.Printf("🔌 TCP/MLLP Inbound: Closing connection %s", connID)
		conn.Close()
	}
	c.activeConns = make(map[string]net.Conn)
	c.connectionCount = 0
	c.connectionMutex.Unlock()

	c.SetState(StateStopped)
	c.SetConnected(false)

	log.Printf("✅ TCP/MLLP Inbound: Stopped successfully")

	return c.BaseInboundConnector.Stop()
}

// Close releases all resources
func (c *TCPMLLPInboundConnector) Close() error {
	c.Stop()
	return c.BaseInboundConnector.Close()
}

// SendValidationResponse implements ValidationAwareConnector interface
func (c *TCPMLLPInboundConnector) SendValidationResponse(ctx context.Context, feedback *models.ValidationFeedback) error {
	c.messageConnsMutex.RLock()
	connInfo, exists := c.messageConns[feedback.MessageID]
	c.messageConnsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found for message %s", feedback.MessageID)
	}

	defer func() {
		c.messageConnsMutex.Lock()
		delete(c.messageConns, feedback.MessageID)
		c.messageConnsMutex.Unlock()
	}()

	var ackCode string
	var ackMessage string

	if feedback.IsRejected() {
		ackCode = "AE"
		ackMessage = feedback.GetErrorMessage()
		log.Printf("❌ TCP/MLLP: Sending NACK for %s: %s", feedback.MessageID, ackMessage)
	} else if feedback.HasWarnings() {
		ackCode = "AA"
		ackMessage = "Message accepted with warnings"
		log.Printf("⚠️  TCP/MLLP: Sending ACK with warnings for %s", feedback.MessageID)
	} else {
		ackCode = "AA"
		ackMessage = "Message accepted"
		log.Printf("✅ TCP/MLLP: Sending ACK for %s", feedback.MessageID)
	}

	ack := c.generateACKMessage(connInfo.OriginalMsg, ackCode, ackMessage)
	return c.sendACK(connInfo.Conn, ack)
}

// SupportsValidationFeedback implements ValidationAwareConnector interface
func (c *TCPMLLPInboundConnector) SupportsValidationFeedback() bool {
	return true
}
