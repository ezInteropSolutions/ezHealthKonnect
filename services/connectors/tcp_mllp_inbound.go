// services/connectors/tcp_mllp_inbound.go
// TCP/MLLP Inbound Connector - HL7 v2.x Message Listener

package connectors

import (
	"bufio"
	"context"
	"crypto/tls"
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
)

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
	readTimeout      time.Duration
	writeTimeout     time.Duration
	keepAlive        bool
	keepAlivePeriod  time.Duration
	authType         string
	authUsername     string
	authPassword     string
	validateChecksum bool
	ackConfig        ACKConfig

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
			log.Printf("❌ TCP/MLLP Inbound: Read error from %s: %v", connID, err)
			c.sendNACK(conn, "Error reading message")
			continue
		}

		if len(hl7Message) == 0 {
			continue
		}

		log.Printf("📨 TCP/MLLP Inbound: Received %d bytes from %s", len(hl7Message), connID)

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

// sendACK sends an ACK message
func (c *TCPMLLPInboundConnector) sendACK(conn net.Conn, ack string) error {
	conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))

	// MLLP framing
	frame := fmt.Sprintf("%c%s%c%c", MLLPStartByte, ack, MLLPEndByte1, MLLPEndByte2)

	_, err := conn.Write([]byte(frame))
	if err != nil {
		log.Printf("❌ TCP/MLLP Inbound: Failed to send ACK: %v", err)
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
