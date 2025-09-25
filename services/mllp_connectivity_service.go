// services/mllp_connectivity_service.go
// MLLP (Minimal Lower Layer Protocol) connectivity service for HL7 message handling

package services

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// MLLPConnectivityService handles MLLP protocol connectivity
type MLLPConnectivityService struct {
	db              *sql.DB
	activeListeners map[string]*MLLPListener
	mu              sync.RWMutex
}

// MLLPListener represents an active MLLP listener
type MLLPListener struct {
	ID           string
	Host         string
	Port         int
	Listener     net.Listener
	IsActive     bool
	StartTime    time.Time
	MessageCount int64
	Connections  map[string]*MLLPConnection
	ConnMutex    sync.RWMutex
	MessageChan  chan *HL7Message
	ctx          context.Context
	cancel       context.CancelFunc
}

// MLLPConnection represents a single MLLP connection
type MLLPConnection struct {
	ID           string
	Conn         net.Conn
	RemoteAddr   string
	StartTime    time.Time
	LastActivity time.Time
	MessageCount int64
	IsActive     bool
}

// HL7Message represents a received HL7 message
type HL7Message struct {
	ID           string                 `json:"id"`
	Content      string                 `json:"content"`
	Source       string                 `json:"source"`
	ReceivedAt   time.Time              `json:"receivedAt"`
	ConnectionID string                 `json:"connectionId"`
	ListenerID   string                 `json:"listenerId"`
	Headers      map[string]interface{} `json:"headers,omitempty"`
	Size         int                    `json:"size"`
}

// MLLPConfig represents MLLP configuration
type MLLPConfig struct {
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	MaxConnections    int           `json:"maxConnections"`
	ReadTimeout       time.Duration `json:"readTimeout"`
	WriteTimeout      time.Duration `json:"writeTimeout"`
	MaxMessageSize    int           `json:"maxMessageSize"`
	EnableKeepAlive   bool          `json:"enableKeepAlive"`
	KeepAliveInterval time.Duration `json:"keepAliveInterval"`
}

// MLLP protocol constants
const (
	MLLPStartByte = 0x0B // VT (Vertical Tab)
	MLLPEndByte1  = 0x1C // FS (File Separator)
	MLLPEndByte2  = 0x0D // CR (Carriage Return)

	DefaultMLLPPort           = 2575
	DefaultMaxConnections     = 100
	DefaultReadTimeout        = 30 * time.Second
	DefaultWriteTimeout       = 10 * time.Second
	DefaultMaxMessageSize     = 10 * 1024 * 1024 // 10MB
	DefaultKeepAliveInterval  = 60 * time.Second
)

// NewMLLPConnectivityService creates a new MLLP connectivity service
func NewMLLPConnectivityService(db *sql.DB) *MLLPConnectivityService {
	return &MLLPConnectivityService{
		db:              db,
		activeListeners: make(map[string]*MLLPListener),
	}
}

// StartListener starts an MLLP listener on the specified configuration
func (service *MLLPConnectivityService) StartListener(ctx context.Context, config *MLLPConfig) (*MLLPListener, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	// Validate configuration
	if err := service.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	listenerID := fmt.Sprintf("mllp_%s_%d", config.Host, config.Port)

	// Check if already listening
	if existing, exists := service.activeListeners[listenerID]; exists && existing.IsActive {
		return existing, nil
	}

	// Create TCP listener
	address := fmt.Sprintf("%s:%d", config.Host, config.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener on %s: %w", address, err)
	}

	// Create context for the listener
	listenerCtx, cancel := context.WithCancel(ctx)

	// Create MLLP listener
	mllpListener := &MLLPListener{
		ID:          listenerID,
		Host:        config.Host,
		Port:        config.Port,
		Listener:    listener,
		IsActive:    true,
		StartTime:   time.Now(),
		Connections: make(map[string]*MLLPConnection),
		MessageChan: make(chan *HL7Message, 1000),
		ctx:         listenerCtx,
		cancel:      cancel,
	}

	// Store listener
	service.activeListeners[listenerID] = mllpListener

	// Start accepting connections
	go service.acceptConnections(mllpListener, config)

	return mllpListener, nil
}

// StopListener stops an active MLLP listener
func (service *MLLPConnectivityService) StopListener(listenerID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	listener, exists := service.activeListeners[listenerID]
	if !exists {
		return fmt.Errorf("listener %s not found", listenerID)
	}

	// Cancel context
	listener.cancel()

	// Close all connections
	listener.ConnMutex.Lock()
	for _, conn := range listener.Connections {
		conn.Conn.Close()
	}
	listener.ConnMutex.Unlock()

	// Close listener
	listener.Listener.Close()
	listener.IsActive = false

	// Remove from active listeners
	delete(service.activeListeners, listenerID)

	return nil
}

// GetActiveListeners returns all active MLLP listeners
func (service *MLLPConnectivityService) GetActiveListeners() []*MLLPListener {
	service.mu.RLock()
	defer service.mu.RUnlock()

	listeners := make([]*MLLPListener, 0, len(service.activeListeners))
	for _, listener := range service.activeListeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

// SendMessage sends an HL7 message via MLLP to a target endpoint
func (service *MLLPConnectivityService) SendMessage(ctx context.Context, message *HL7Message, targetEndpoint string, config *MLLPConfig) error {
	// Create connection to target
	conn, err := net.DialTimeout("tcp", targetEndpoint, config.WriteTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", targetEndpoint, err)
	}
	defer conn.Close()

	// Frame the message with MLLP
	framedMessage := service.frameMessage(message.Content)

	// Set write timeout
	if err := conn.SetWriteDeadline(time.Now().Add(config.WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Send the message
	if _, err := conn.Write(framedMessage); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Read acknowledgment
	if err := conn.SetReadDeadline(time.Now().Add(config.ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	ackMessage, err := service.readMLLPMessage(conn, config.MaxMessageSize)
	if err != nil {
		return fmt.Errorf("failed to read acknowledgment: %w", err)
	}

	// Validate acknowledgment
	if !service.isValidAcknowledgment(ackMessage) {
		return fmt.Errorf("received invalid acknowledgment: %s", string(ackMessage))
	}

	return nil
}

// GetListenerMessages returns the message channel for a listener
func (service *MLLPConnectivityService) GetListenerMessages(listenerID string) (<-chan *HL7Message, error) {
	service.mu.RLock()
	defer service.mu.RUnlock()

	listener, exists := service.activeListeners[listenerID]
	if !exists {
		return nil, fmt.Errorf("listener %s not found", listenerID)
	}

	return listener.MessageChan, nil
}

// Private methods

func (service *MLLPConnectivityService) validateConfig(config *MLLPConfig) error {
	if config.Host == "" {
		config.Host = "0.0.0.0"
	}
	if config.Port <= 0 {
		config.Port = DefaultMLLPPort
	}
	if config.MaxConnections <= 0 {
		config.MaxConnections = DefaultMaxConnections
	}
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = DefaultReadTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = DefaultWriteTimeout
	}
	if config.MaxMessageSize <= 0 {
		config.MaxMessageSize = DefaultMaxMessageSize
	}
	if config.KeepAliveInterval <= 0 {
		config.KeepAliveInterval = DefaultKeepAliveInterval
	}
	return nil
}

func (service *MLLPConnectivityService) acceptConnections(listener *MLLPListener, config *MLLPConfig) {
	for listener.IsActive {
		select {
		case <-listener.ctx.Done():
			return
		default:
			conn, err := listener.Listener.Accept()
			if err != nil {
				if listener.IsActive {
					// Log error but continue
					continue
				}
				return
			}

			// Check connection limit
			listener.ConnMutex.RLock()
			connectionCount := len(listener.Connections)
			listener.ConnMutex.RUnlock()

			if connectionCount >= config.MaxConnections {
				conn.Close()
				continue
			}

			// Create MLLP connection
			connID := fmt.Sprintf("%s_%d", conn.RemoteAddr().String(), time.Now().UnixNano())
			mllpConn := &MLLPConnection{
				ID:           connID,
				Conn:         conn,
				RemoteAddr:   conn.RemoteAddr().String(),
				StartTime:    time.Now(),
				LastActivity: time.Now(),
				IsActive:     true,
			}

			// Configure keep-alive
			if tcpConn, ok := conn.(*net.TCPConn); ok && config.EnableKeepAlive {
				tcpConn.SetKeepAlive(true)
				tcpConn.SetKeepAlivePeriod(config.KeepAliveInterval)
			}

			// Store connection
			listener.ConnMutex.Lock()
			listener.Connections[connID] = mllpConn
			listener.ConnMutex.Unlock()

			// Handle connection
			go service.handleConnection(listener, mllpConn, config)
		}
	}
}

func (service *MLLPConnectivityService) handleConnection(listener *MLLPListener, conn *MLLPConnection, config *MLLPConfig) {
	defer service.closeConnection(listener, conn)

	for conn.IsActive && listener.IsActive {
		// Set read timeout
		if err := conn.Conn.SetReadDeadline(time.Now().Add(config.ReadTimeout)); err != nil {
			break
		}

		// Read MLLP message
		messageData, err := service.readMLLPMessage(conn.Conn, config.MaxMessageSize)
		if err != nil {
			if err != io.EOF {
				// Log error
			}
			break
		}

		// Update connection activity
		conn.LastActivity = time.Now()
		conn.MessageCount++
		listener.MessageCount++

		// Create HL7 message
		hl7Message := &HL7Message{
			ID:           service.generateMessageID(),
			Content:      string(messageData),
			Source:       conn.RemoteAddr,
			ReceivedAt:   time.Now(),
			ConnectionID: conn.ID,
			ListenerID:   listener.ID,
			Size:         len(messageData),
		}

		// Send to message channel (non-blocking)
		select {
		case listener.MessageChan <- hl7Message:
			// Message sent successfully
		default:
			// Channel is full, skip message (could implement buffering)
		}

		// Send acknowledgment
		ack := service.generateAcknowledgment(hl7Message)
		if err := service.sendAcknowledgment(conn, ack, config); err != nil {
			break
		}
	}
}

func (service *MLLPConnectivityService) readMLLPMessage(conn net.Conn, maxSize int) ([]byte, error) {
	reader := bufio.NewReader(conn)

	// Read start byte
	startByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	if startByte != MLLPStartByte {
		return nil, fmt.Errorf("invalid MLLP start byte: expected 0x%02X, got 0x%02X", MLLPStartByte, startByte)
	}

	// Read message content until end sequence
	var message []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}

		if b == MLLPEndByte1 {
			// Check for second end byte
			nextByte, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if nextByte == MLLPEndByte2 {
				// Found complete end sequence
				break
			} else {
				// False positive, include both bytes
				message = append(message, b, nextByte)
			}
		} else {
			message = append(message, b)
		}

		// Check message size limit
		if len(message) > maxSize {
			return nil, fmt.Errorf("message exceeds maximum size of %d bytes", maxSize)
		}
	}

	return message, nil
}

func (service *MLLPConnectivityService) frameMessage(content string) []byte {
	// MLLP framing: <SB>message<EB><CR>
	framedMessage := make([]byte, 0, len(content)+3)
	framedMessage = append(framedMessage, MLLPStartByte)
	framedMessage = append(framedMessage, []byte(content)...)
	framedMessage = append(framedMessage, MLLPEndByte1)
	framedMessage = append(framedMessage, MLLPEndByte2)
	return framedMessage
}

func (service *MLLPConnectivityService) generateAcknowledgment(message *HL7Message) string {
	// Generate a basic HL7 ACK message
	timestamp := time.Now().Format("20060102150405")
	messageControlId := service.extractMessageControlId(message.Content)

	ack := fmt.Sprintf("MSH|^~\\&|EZHEALTHKONNECT|SYSTEM|%s|%s|%s||ACK|%s|P|2.5\r",
		"SENDER", "RECEIVER", timestamp, messageControlId)
	ack += fmt.Sprintf("MSA|AA|%s\r", messageControlId)

	return ack
}

func (service *MLLPConnectivityService) extractMessageControlId(hl7Content string) string {
	// Extract message control ID from MSH segment
	lines := strings.Split(hl7Content, "\r")
	for _, line := range lines {
		if strings.HasPrefix(line, "MSH") {
			fields := strings.Split(line, "|")
			if len(fields) >= 10 {
				return fields[9] // Message Control ID is field 10 (index 9)
			}
		}
	}
	return fmt.Sprintf("ACK%d", time.Now().Unix())
}

func (service *MLLPConnectivityService) sendAcknowledgment(conn *MLLPConnection, ack string, config *MLLPConfig) error {
	framedAck := service.frameMessage(ack)

	// Set write timeout
	if err := conn.Conn.SetWriteDeadline(time.Now().Add(config.WriteTimeout)); err != nil {
		return err
	}

	// Send acknowledgment
	writer := bufio.NewWriter(conn.Conn)
	if _, err := writer.Write(framedAck); err != nil {
		return err
	}

	return writer.Flush()
}

func (service *MLLPConnectivityService) isValidAcknowledgment(ackMessage []byte) bool {
	// Basic validation - check if it's an HL7 ACK message
	ackStr := string(ackMessage)
	return strings.Contains(ackStr, "MSH") && strings.Contains(ackStr, "MSA")
}

func (service *MLLPConnectivityService) closeConnection(listener *MLLPListener, conn *MLLPConnection) {
	if !conn.IsActive {
		return
	}

	conn.IsActive = false
	conn.Conn.Close()

	// Remove from connections map
	listener.ConnMutex.Lock()
	delete(listener.Connections, conn.ID)
	listener.ConnMutex.Unlock()
}

func (service *MLLPConnectivityService) generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}