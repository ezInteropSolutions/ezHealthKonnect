// services/mllp_connectivity_service.go
// MLLP (Minimal Lower Layer Protocol) connectivity service for HL7 message handling

package services

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
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
	hybridStorage   *HybridMessageStorage
	parserService   *MessageParserService
}

// MLLPListener represents an active MLLP listener
type MLLPListener struct {
	ID           string
	InterfaceID  string // Interface UUID from database
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
	InterfaceID       string        `json:"interfaceId"`       // Interface UUID from database
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
		hybridStorage:   nil, // Set via SetHybridStorage
		parserService:   nil, // Set via SetParserService
	}
}

// SetHybridStorage sets the hybrid storage service for message persistence
func (service *MLLPConnectivityService) SetHybridStorage(storage *HybridMessageStorage) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.hybridStorage = storage
	log.Printf("✅ MLLP Service: Hybrid storage configured")
}

// SetParserService sets the parser service for JSON conversion
func (service *MLLPConnectivityService) SetParserService(parser *MessageParserService) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.parserService = parser
	log.Printf("✅ MLLP Service: Parser service configured")
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

	// Create context for the listener - use Background() not request context
	// Request context will be cancelled when HTTP call completes
	listenerCtx, cancel := context.WithCancel(context.Background())

	// Create MLLP listener
	mllpListener := &MLLPListener{
		ID:          listenerID,
		InterfaceID: config.InterfaceID, // Associate with interface
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
	log.Printf("🔄 MLLP acceptConnections started for %s:%d", listener.Host, listener.Port)
	for listener.IsActive {
		select {
		case <-listener.ctx.Done():
			log.Printf("⏹️ MLLP listener context cancelled for %s:%d", listener.Host, listener.Port)
			return
		default:
			log.Printf("⏳ Waiting for connection on %s:%d...", listener.Host, listener.Port)
			conn, err := listener.Listener.Accept()
			if err != nil {
				log.Printf("❌ Accept error on %s:%d: %v", listener.Host, listener.Port, err)
				if listener.IsActive {
					// Log error but continue
					continue
				}
				return
			}
			log.Printf("✅ Connection accepted from %s", conn.RemoteAddr().String())

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

	// Add panic recovery to always send NACK on any unexpected error
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ PANIC in MLLP connection handler: %v", r)

			// Create emergency NACK message
			emergencyMsg := &HL7Message{
				ID:           service.generateMessageID(),
				Content:      "",
				Source:       conn.RemoteAddr,
				ReceivedAt:   time.Now(),
				ConnectionID: conn.ID,
				ListenerID:   listener.ID,
				Size:         0,
			}

			nack := service.generateAcknowledgmentWithError(emergencyMsg, "AE", fmt.Sprintf("Internal error: %v", r))
			service.sendAcknowledgment(conn, nack, config)
		}
	}()

	for conn.IsActive && listener.IsActive {
		// Set read timeout
		if err := conn.Conn.SetReadDeadline(time.Now().Add(config.ReadTimeout)); err != nil {
			// Send NACK for timeout error
			errorMsg := &HL7Message{
				ID:           service.generateMessageID(),
				Content:      "",
				Source:       conn.RemoteAddr,
				ReceivedAt:   time.Now(),
				ConnectionID: conn.ID,
				ListenerID:   listener.ID,
				Size:         0,
			}
			nack := service.generateAcknowledgmentWithError(errorMsg, "AE", fmt.Sprintf("Timeout error: %v", err))
			service.sendAcknowledgment(conn, nack, config)
			break
		}

		// Read MLLP message
		messageData, err := service.readMLLPMessage(conn.Conn, config.MaxMessageSize)

		// Always attempt to send ACK/NACK, even on error
		var hl7Message *HL7Message
		var ackCode string = "AA" // Application Accept (success)
		var errorText string = ""

		if err != nil {
			// Create minimal message structure for NACK
			ackCode = "AE" // Application Error
			errorText = fmt.Sprintf("Error reading message: %v", err)

			hl7Message = &HL7Message{
				ID:           service.generateMessageID(),
				Content:      string(messageData),
				Source:       conn.RemoteAddr,
				ReceivedAt:   time.Now(),
				ConnectionID: conn.ID,
				ListenerID:   listener.ID,
				Size:         len(messageData),
			}

			// Send NACK and log error
			log.Printf("❌ Error reading MLLP message from %s: %v", conn.RemoteAddr, err)
			nack := service.generateAcknowledgmentWithError(hl7Message, ackCode, errorText)
			if sendErr := service.sendAcknowledgment(conn, nack, config); sendErr != nil {
				log.Printf("❌ Failed to send NACK (connection likely closed by client): %v", sendErr)
			}

			if err == io.EOF {
				log.Printf("ℹ️  Client %s disconnected (EOF) - connection closed before ACK could be sent", conn.RemoteAddr)
				break // Connection closed cleanly
			}
			continue // Continue to next message
		}

		// Update connection activity
		conn.LastActivity = time.Now()
		conn.MessageCount++
		listener.MessageCount++

		// Create HL7 message
		hl7Message = &HL7Message{
			ID:           service.generateMessageID(),
			Content:      string(messageData),
			Source:       conn.RemoteAddr,
			ReceivedAt:   time.Now(),
			ConnectionID: conn.ID,
			ListenerID:   listener.ID,
			Size:         len(messageData),
		}

		// Validate message format
		if !service.isValidHL7Message(hl7Message.Content) {
			ackCode = "AR" // Application Reject
			errorText = "Invalid HL7 message format"
			log.Printf("⚠️  Invalid HL7 message from %s", conn.RemoteAddr)
		}

		// Send to message channel (non-blocking)
		if ackCode == "AA" {
			select {
			case listener.MessageChan <- hl7Message:
				// Message sent successfully
			default:
				// Channel is full
				ackCode = "AE"
				errorText = "Message queue full"
				log.Printf("⚠️  Message channel full, cannot process message from %s", conn.RemoteAddr)
			}
		}

		// Always send acknowledgment (ACK or NACK)
		var ack string
		if ackCode == "AA" {
			ack = service.generateAcknowledgment(hl7Message)
		} else {
			ack = service.generateAcknowledgmentWithError(hl7Message, ackCode, errorText)
		}

		if err := service.sendAcknowledgment(conn, ack, config); err != nil {
			log.Printf("❌ Failed to send ACK/NACK to %s: %v", conn.RemoteAddr, err)
			break // Only break if we can't send the acknowledgment
		}

		// Store message in hybrid storage (asynchronously) only if accepted
		if ackCode == "AA" {
			go service.processAndStoreMessage(hl7Message, listener.InterfaceID)
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
	// Generate a basic HL7 ACK message (AA - Application Accept)
	timestamp := time.Now().Format("20060102150405")
	messageControlId := service.extractMessageControlId(message.Content)

	ack := fmt.Sprintf("MSH|^~\\&|EZHEALTHKONNECT|SYSTEM|%s|%s|%s||ACK|%s|P|2.5\r",
		"SENDER", "RECEIVER", timestamp, messageControlId)
	ack += fmt.Sprintf("MSA|AA|%s\r", messageControlId)

	return ack
}

func (service *MLLPConnectivityService) generateAcknowledgmentWithError(message *HL7Message, ackCode string, errorText string) string {
	// Generate HL7 NACK message with error details
	// ackCode: AA (Accept), AE (Error), AR (Reject)
	timestamp := time.Now().Format("20060102150405")
	messageControlId := service.extractMessageControlId(message.Content)

	ack := fmt.Sprintf("MSH|^~\\&|EZHEALTHKONNECT|SYSTEM|%s|%s|%s||ACK|%s|P|2.5\r",
		"SENDER", "RECEIVER", timestamp, messageControlId)

	// MSA segment with acknowledgment code and optional error text
	if errorText != "" {
		ack += fmt.Sprintf("MSA|%s|%s|%s\r", ackCode, messageControlId, errorText)
	} else {
		ack += fmt.Sprintf("MSA|%s|%s\r", ackCode, messageControlId)
	}

	// Add ERR segment for detailed error information if error exists
	if errorText != "" && (ackCode == "AE" || ackCode == "AR") {
		ack += fmt.Sprintf("ERR|||%s|E\r", errorText)
	}

	return ack
}

func (service *MLLPConnectivityService) isValidHL7Message(content string) bool {
	// Basic HL7 validation - check for MSH segment
	if !strings.HasPrefix(content, "MSH") {
		return false
	}

	// Check for minimum required fields in MSH
	lines := strings.Split(content, "\r")
	if len(lines) == 0 {
		return false
	}

	mshFields := strings.Split(lines[0], "|")
	// MSH should have at least 12 fields
	return len(mshFields) >= 12
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

// processAndStoreMessage handles message storage and triggers JSON conversion + transformation pipeline
func (service *MLLPConnectivityService) processAndStoreMessage(hl7Message *HL7Message, interfaceID string) {
	ctx := context.Background()

	// Extract message type from HL7 message
	messageType := service.extractMessageControlId(hl7Message.Content) // Reuse existing parsing logic

	// Check if hybrid storage is available
	if service.hybridStorage == nil {
		log.Printf("⚠️ Hybrid storage not configured, message not persisted: %s", hl7Message.ID)
		return
	}

	// Prepare hybrid message data
	hybridData := &HybridMessageData{
		MessageID:       hl7Message.ID,
		CorrelationID:   hl7Message.ID,
		InterfaceID:     interfaceID, // Use interface UUID from database
		Status:          "received",
		Priority:        5, // Default medium priority
		ReceivedAt:      hl7Message.ReceivedAt,
		SourceType:      "mllp",
		SourceEndpoint:  hl7Message.Source,
		SourceIP:        hl7Message.Source,
		MessageType:     messageType,
		MessageSize:     hl7Message.Size,
		MessageEncoding: "UTF-8",
		RawContent:      hl7Message.Content, // Raw HL7 → MongoDB
		ParsedContent:   nil,                 // Will be populated by parser
		Metadata:        make(map[string]interface{}),
	}

	// Store message in hybrid storage (MongoDB raw + PostgreSQL metadata)
	log.Printf("💾 Storing message %s in hybrid storage...", hl7Message.ID)
	err := service.hybridStorage.StoreMessage(ctx, hybridData)
	if err != nil {
		log.Printf("❌ Failed to store message %s: %v", hl7Message.ID, err)
		return
	}

	log.Printf("✅ Message %s stored in hybrid storage (MongoDB + PostgreSQL)", hl7Message.ID)

	// Trigger JSON conversion asynchronously (if parser service is configured)
	if service.parserService != nil {
		go service.convertMessageToJSON(hl7Message.ID, hybridData.InterfaceID, hl7Message.Content)
	}
}

// convertMessageToJSON converts raw HL7 message to JSON asynchronously
func (service *MLLPConnectivityService) convertMessageToJSON(messageID, interfaceID, rawContent string) {
	ctx := context.Background()

	log.Printf("🔄 Starting JSON conversion for message %s", messageID)

	// Parse HL7 to JSON (messageID, interfaceID, rawContent)
	result, err := service.parserService.ParseToJSON(ctx, messageID, interfaceID, rawContent)
	if err != nil {
		log.Printf("❌ JSON conversion failed for message %s: %v", messageID, err)
		return
	}

	log.Printf("✅ JSON conversion completed for message %s (format: %s, time: %v)",
		messageID, result.Format, result.ParsingTime)

	// TODO: Trigger transformation pipeline here
	// transformationService.ExecuteTransformation(ctx, interfaceID, result.ParsedJSON)
}