// internal/connectivity/tcp_connector.go
// TCP and MLLP connectivity handlers

package connectivity

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
)

// TCPInputConnector handles TCP/MLLP message reception
type TCPInputConnector struct {
	*BaseConnector
	listener     net.Listener
	connections  map[string]*TCPConnection
	connMutex    sync.RWMutex
	messageChan  chan<- *pkg.UniversalMessage
	useMLLP      bool
	maxConnections int
}

// TCPOutputConnector handles TCP/MLLP message transmission
type TCPOutputConnector struct {
	*BaseConnector
	connection   *TCPConnection
	useMLLP      bool
	pool         *ConnectionPool
}

// TCPConnection represents a TCP connection with MLLP support
type TCPConnection struct {
	ID         string
	Conn       net.Conn
	Reader     *bufio.Reader
	Writer     *bufio.Writer
	RemoteAddr string
	Connected  bool
	LastUsed   time.Time
	mutex      sync.RWMutex
}

// ConnectionPool manages a pool of TCP connections for output
type ConnectionPool struct {
	connections map[string]*TCPConnection
	mutex       sync.RWMutex
	maxSize     int
	maxIdle     time.Duration
}

// MLLP protocol constants
const (
	MLLPStartByte byte = 0x0B  // Vertical Tab
	MLLPEndByte1  byte = 0x1C  // File Separator
	MLLPEndByte2  byte = 0x0D  // Carriage Return
)

// NewTCPInputConnector creates a new TCP input connector
func NewTCPInputConnector(config pkg.ConnectorConfig) (*TCPInputConnector, error) {
	base := NewBaseConnector(config, "tcp_input")

	useMLLP := config.Protocol == pkg.ProtocolMLLP
	maxConnections := 100 // Default max connections

	if maxConn, exists := config.Settings["max_connections"]; exists {
		if mc, ok := maxConn.(float64); ok {
			maxConnections = int(mc)
		}
	}

	connector := &TCPInputConnector{
		BaseConnector:  base,
		connections:    make(map[string]*TCPConnection),
		useMLLP:        useMLLP,
		maxConnections: maxConnections,
	}

	return connector, nil
}

// NewTCPOutputConnector creates a new TCP output connector
func NewTCPOutputConnector(config pkg.ConnectorConfig) (*TCPOutputConnector, error) {
	base := NewBaseConnector(config, "tcp_output")

	useMLLP := config.Protocol == pkg.ProtocolMLLP

	connector := &TCPOutputConnector{
		BaseConnector: base,
		useMLLP:       useMLLP,
		pool:          NewConnectionPool(10, 5*time.Minute), // 10 connections, 5min idle
	}

	return connector, nil
}

// Start begins listening for TCP connections
func (tc *TCPInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := tc.Start(tc.ctx); err != nil {
		return err
	}

	tc.messageChan = messageChan

	// Start listening on the specified port
	address := fmt.Sprintf(":%d", tc.Config.Port)
	if tc.Config.Endpoint != "" && tc.Config.Endpoint != "0.0.0.0" {
		address = fmt.Sprintf("%s:%d", tc.Config.Endpoint, tc.Config.Port)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to start TCP listener: %w", err)
	}

	tc.listener = listener
	tc.SetConnected(true)

	// Start accepting connections
	go tc.acceptConnections()

	return nil
}

// StopListening stops the TCP listener
func (tc *TCPInputConnector) StopListening() error {
	if tc.listener != nil {
		tc.listener.Close()
	}

	// Close all connections
	tc.connMutex.Lock()
	for _, conn := range tc.connections {
		conn.Close()
	}
	tc.connections = make(map[string]*TCPConnection)
	tc.connMutex.Unlock()

	return tc.Stop()
}

// Connect establishes a TCP connection (for output connector)
func (tc *TCPOutputConnector) Connect() error {
	address := fmt.Sprintf("%s:%d", tc.Config.Endpoint, tc.Config.Port)

	conn, err := net.DialTimeout("tcp", address, tc.Config.Timeout)
	if err != nil {
		tc.RecordError(fmt.Errorf("failed to connect to %s: %w", address, err))
		return err
	}

	tcpConn := &TCPConnection{
		ID:         uuid.New().String(),
		Conn:       conn,
		Reader:     bufio.NewReader(conn),
		Writer:     bufio.NewWriter(conn),
		RemoteAddr: conn.RemoteAddr().String(),
		Connected:  true,
		LastUsed:   time.Now(),
	}

	tc.connection = tcpConn
	tc.SetConnected(true)

	return nil
}

// Disconnect closes the TCP connection
func (tc *TCPOutputConnector) Disconnect() error {
	if tc.connection != nil {
		tc.connection.Close()
		tc.connection = nil
	}

	tc.SetConnected(false)
	return nil
}

// TestConnection tests the TCP connection
func (tc *TCPInputConnector) TestConnection() error {
	// For input connectors, test if we can bind to the port
	address := fmt.Sprintf(":%d", tc.Config.Port)
	if tc.Config.Endpoint != "" && tc.Config.Endpoint != "0.0.0.0" {
		address = fmt.Sprintf("%s:%d", tc.Config.Endpoint, tc.Config.Port)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("cannot bind to %s: %w", address, err)
	}
	listener.Close()

	return nil
}

// TestConnection tests the TCP connection (output)
func (tc *TCPOutputConnector) TestConnection() error {
	address := fmt.Sprintf("%s:%d", tc.Config.Endpoint, tc.Config.Port)

	conn, err := net.DialTimeout("tcp", address, tc.Config.Timeout)
	if err != nil {
		return fmt.Errorf("cannot connect to %s: %w", address, err)
	}
	conn.Close()

	return nil
}

// SendMessage sends a message via TCP/MLLP
func (tc *TCPOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	startTime := time.Now()

	// Get or create connection
	conn, err := tc.pool.GetConnection(tc.Config)
	if err != nil {
		// Try direct connection if pool fails
		if tc.connection == nil || !tc.connection.Connected {
			if err := tc.Connect(); err != nil {
				tc.RecordError(err)
				return err
			}
		}
		conn = tc.connection
	}

	// Prepare message content
	content := message.Content
	if tc.useMLLP {
		content = tc.wrapMLLP(content)
	}

	// Send message
	if err := conn.WriteMessage(content); err != nil {
		tc.RecordError(err)
		return err
	}

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	tc.RecordMessage(int64(len(content)), latency)

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	return nil
}

// SendBatch sends multiple messages in a batch
func (tc *TCPOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	for _, message := range messages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := tc.SendMessage(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

// SupportsAcknowledgment returns whether TCP supports acknowledgments
func (tc *TCPOutputConnector) SupportsAcknowledgment() bool {
	return tc.useMLLP // MLLP supports ACK/NAK
}

// WaitForAcknowledgment waits for message acknowledgment
func (tc *TCPOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	if !tc.useMLLP {
		return fmt.Errorf("acknowledgments not supported for plain TCP")
	}

	// Read acknowledgment from connection
	if tc.connection == nil {
		return fmt.Errorf("no active connection")
	}

	tc.connection.Conn.SetReadDeadline(time.Now().Add(timeout))
	defer tc.connection.Conn.SetReadDeadline(time.Time{})

	ackData, err := tc.connection.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read acknowledgment: %w", err)
	}

	// Parse ACK/NAK
	if strings.Contains(ackData, "ACK") {
		return nil
	} else if strings.Contains(ackData, "NAK") {
		return fmt.Errorf("message rejected (NAK received)")
	}

	return fmt.Errorf("unexpected acknowledgment: %s", ackData)
}

// acceptConnections handles incoming TCP connections
func (tc *TCPInputConnector) acceptConnections() {
	for {
		select {
		case <-tc.Context().Done():
			return
		default:
			conn, err := tc.listener.Accept()
			if err != nil {
				if !strings.Contains(err.Error(), "use of closed network connection") {
					tc.RecordError(err)
				}
				return
			}

			// Check connection limit
			tc.connMutex.RLock()
			connCount := len(tc.connections)
			tc.connMutex.RUnlock()

			if connCount >= tc.maxConnections {
				conn.Close()
				continue
			}

			// Create connection wrapper
			tcpConn := &TCPConnection{
				ID:         uuid.New().String(),
				Conn:       conn,
				Reader:     bufio.NewReader(conn),
				Writer:     bufio.NewWriter(conn),
				RemoteAddr: conn.RemoteAddr().String(),
				Connected:  true,
				LastUsed:   time.Now(),
			}

			// Add to connections map
			tc.connMutex.Lock()
			tc.connections[tcpConn.ID] = tcpConn
			tc.connMutex.Unlock()

			// Handle connection in goroutine
			go tc.handleConnection(tcpConn)
		}
	}
}

// handleConnection processes messages from a TCP connection
func (tc *TCPInputConnector) handleConnection(conn *TCPConnection) {
	defer func() {
		conn.Close()
		tc.connMutex.Lock()
		delete(tc.connections, conn.ID)
		tc.connMutex.Unlock()
	}()

	for {
		select {
		case <-tc.Context().Done():
			return
		default:
			// Set read timeout
			conn.Conn.SetReadDeadline(time.Now().Add(tc.Config.Timeout))

			// Read message
			messageData, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "timeout") {
					tc.RecordError(err)
				}
				return
			}

			conn.LastUsed = time.Now()

			// Create universal message
			message := pkg.NewUniversalMessage()
			message.Content = messageData
			message.ContentType = "HL7" // Default for TCP/MLLP
			message.SourceProtocol = string(tc.Protocol)
			message.SourceEndpoint = conn.RemoteAddr
			message.SourceIP = strings.Split(conn.RemoteAddr, ":")[0]
			message.Size = int64(len(messageData))

			// Detect format if MLLP wrapper is present
			if tc.useMLLP {
				if unwrapped, err := tc.unwrapMLLP(messageData); err == nil {
					message.Content = unwrapped
				}
			}

			// Send to processing channel
			select {
			case tc.messageChan <- message:
				tc.RecordMessage(message.Size, 0)
			case <-tc.Context().Done():
				return
			}

			// Send acknowledgment for MLLP
			if tc.useMLLP {
				ack := tc.createACK(message)
				conn.WriteMessage(ack)
			}
		}
	}
}

// wrapMLLP wraps a message in MLLP framing
func (tc *TCPOutputConnector) wrapMLLP(content string) string {
	return fmt.Sprintf("%c%s%c%c", MLLPStartByte, content, MLLPEndByte1, MLLPEndByte2)
}

// unwrapMLLP removes MLLP framing from a message
func (tc *TCPInputConnector) unwrapMLLP(content string) (string, error) {
	if len(content) < 3 {
		return "", fmt.Errorf("message too short for MLLP")
	}

	if content[0] != MLLPStartByte {
		return "", fmt.Errorf("missing MLLP start byte")
	}

	if content[len(content)-2] != MLLPEndByte1 || content[len(content)-1] != MLLPEndByte2 {
		return "", fmt.Errorf("missing MLLP end bytes")
	}

	return content[1 : len(content)-2], nil
}

// createACK creates an ACK message for MLLP
func (tc *TCPInputConnector) createACK(message *pkg.UniversalMessage) string {
	// Simple ACK - in practice this would be more sophisticated
	ack := "MSH|^~\\&|RECEIVER||SENDER||" + time.Now().Format("20060102150405") + "||ACK||P|2.5\rMSA|AA|" + message.ID + "\r"
	return tc.wrapMLLP(ack)
}

// TCPConnection methods

// ReadMessage reads a complete message from the connection
func (conn *TCPConnection) ReadMessage() (string, error) {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()

	if !conn.Connected {
		return "", fmt.Errorf("connection is closed")
	}

	// For MLLP, read until end sequence
	var message strings.Builder
	for {
		b, err := conn.Reader.ReadByte()
		if err != nil {
			return "", err
		}

		message.WriteByte(b)

		// Check for MLLP end sequence
		if b == MLLPEndByte2 {
			content := message.String()
			if len(content) >= 2 && content[len(content)-2] == MLLPEndByte1 {
				return content, nil
			}
		}

		// Prevent infinite reading
		if message.Len() > 1024*1024 { // 1MB limit
			return "", fmt.Errorf("message too large")
		}
	}
}

// WriteMessage writes a message to the connection
func (conn *TCPConnection) WriteMessage(content string) error {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if !conn.Connected {
		return fmt.Errorf("connection is closed")
	}

	if _, err := conn.Writer.WriteString(content); err != nil {
		return err
	}

	return conn.Writer.Flush()
}

// Close closes the TCP connection
func (conn *TCPConnection) Close() error {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.Connected {
		conn.Connected = false
		if conn.Conn != nil {
			return conn.Conn.Close()
		}
	}

	return nil
}

// ConnectionPool methods

// NewConnectionPool creates a new connection pool
func NewConnectionPool(maxSize int, maxIdle time.Duration) *ConnectionPool {
	return &ConnectionPool{
		connections: make(map[string]*TCPConnection),
		maxSize:     maxSize,
		maxIdle:     maxIdle,
	}
}

// GetConnection gets a connection from the pool or creates a new one
func (cp *ConnectionPool) GetConnection(config pkg.ConnectorConfig) (*TCPConnection, error) {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	address := fmt.Sprintf("%s:%d", config.Endpoint, config.Port)

	// Look for existing connection
	for _, conn := range cp.connections {
		if conn.Connected && time.Since(conn.LastUsed) < cp.maxIdle {
			conn.LastUsed = time.Now()
			return conn, nil
		}
	}

	// Create new connection if under limit
	if len(cp.connections) < cp.maxSize {
		conn, err := net.DialTimeout("tcp", address, config.Timeout)
		if err != nil {
			return nil, err
		}

		tcpConn := &TCPConnection{
			ID:         uuid.New().String(),
			Conn:       conn,
			Reader:     bufio.NewReader(conn),
			Writer:     bufio.NewWriter(conn),
			RemoteAddr: address,
			Connected:  true,
			LastUsed:   time.Now(),
		}

		cp.connections[tcpConn.ID] = tcpConn
		return tcpConn, nil
	}

	return nil, fmt.Errorf("connection pool exhausted")
}

// CleanupIdle removes idle connections from the pool
func (cp *ConnectionPool) CleanupIdle() {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	for id, conn := range cp.connections {
		if time.Since(conn.LastUsed) > cp.maxIdle {
			conn.Close()
			delete(cp.connections, id)
		}
	}
}