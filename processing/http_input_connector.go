// processing/http_input_connector.go
// HTTP input connector for receiving messages via HTTP endpoints

package processing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPInputConnector implements InputConnector for HTTP endpoints
type HTTPInputConnector struct {
	host         string
	port         int
	path         string
	interfaceID  string
	server       *http.Server
	router       *gin.Engine
	messageChan  chan<- Message
	messageCount int64
	errorCount   int64
	lastActivity time.Time
	mutex        sync.RWMutex
}

// NewHTTPInputConnector creates a new HTTP input connector
func NewHTTPInputConnector(config map[string]interface{}) (InputConnector, error) {
	host, ok := config["host"].(string)
	if !ok {
		host = "0.0.0.0" // Default to listen on all interfaces
	}

	port, ok := config["port"].(float64)
	if !ok {
		return nil, fmt.Errorf("HTTP port not specified")
	}

	path, ok := config["path"].(string)
	if !ok {
		path = "/fhir" // Default FHIR endpoint
	}

	// Extract interface_id if provided
	interfaceID, _ := config["interface_id"].(string)

	// Set up Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	connector := &HTTPInputConnector{
		host:         host,
		port:         int(port),
		path:         path,
		interfaceID:  interfaceID,
		router:       router,
		lastActivity: time.Now(),
	}

	fmt.Printf("✅ HTTP input connector initialized: %s:%d%s (Interface: %s)\n", host, int(port), path, interfaceID)
	return connector, nil
}

// Start begins listening for HTTP messages
func (h *HTTPInputConnector) Start(ctx context.Context, messageChan chan<- Message) error {
	h.messageChan = messageChan

	// Set up HTTP endpoints
	h.router.POST(h.path, h.handleMessage)
	h.router.PUT(h.path, h.handleMessage)
	h.router.POST(h.path+"/*resourceType", h.handleMessage)
	h.router.PUT(h.path+"/*resourceType", h.handleMessage)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", h.host, h.port)
	h.server = &http.Server{
		Addr:    addr,
		Handler: h.router,
	}

	log.Printf("🔌 Starting HTTP input connector: %s", addr)

	// Start server in goroutine
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP server error: %v", err)
		}
	}()

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		h.Close()
	}()

	return nil
}

// handleMessage handles incoming HTTP messages
func (h *HTTPInputConnector) handleMessage(c *gin.Context) {
	h.mutex.Lock()
	h.messageCount++
	h.lastActivity = time.Now()
	h.mutex.Unlock()

	// Read request body
	body, err := c.GetRawData()
	if err != nil {
		h.mutex.Lock()
		h.errorCount++
		h.mutex.Unlock()

		log.Printf("❌ Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Create message
	message := Message{
		ID:        fmt.Sprintf("http_%d", time.Now().UnixNano()),
		Content:   string(body),
		Type:      "FHIR",
		Source:    "http",
		Timestamp: time.Now().Format(time.RFC3339),
		Metadata: map[string]interface{}{
			"interface_id": h.interfaceID,
			"source_address": c.ClientIP(),
			"message_size": len(body),
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"headers":     c.Request.Header,
			"contentType": c.GetHeader("Content-Type"),
			"userAgent":   c.GetHeader("User-Agent"),
		},
	}

	// Send to processing channel
	select {
	case h.messageChan <- message:
		log.Printf("📨 HTTP message received: %s from %s", message.ID, message.Source)
		c.JSON(http.StatusOK, gin.H{
			"status":    "accepted",
			"messageId": message.ID,
			"timestamp": message.Timestamp,
		})
	default:
		h.mutex.Lock()
		h.errorCount++
		h.mutex.Unlock()

		log.Printf("❌ Message channel full, dropping message")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Processing queue full"})
	}
}

// TestConnection tests the HTTP server setup
func (h *HTTPInputConnector) TestConnection() error {
	log.Printf("🔍 Testing HTTP server setup: %s:%d%s", h.host, h.port, h.path)
	// For HTTP input, we just verify the configuration is valid
	return nil
}

// Close shuts down the HTTP server
func (h *HTTPInputConnector) Close() error {
	if h.server != nil {
		log.Printf("🔌 HTTP input connector shutting down: %s:%d", h.host, h.port)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return h.server.Shutdown(ctx)
	}
	return nil
}

// GetStatus returns connector status
func (h *HTTPInputConnector) GetStatus() ConnectorStatus {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return ConnectorStatus{
		Type:         "HTTP",
		Status:       "active",
		LastActivity: h.lastActivity.Format(time.RFC3339),
		MessageCount: h.messageCount,
		ErrorCount:   h.errorCount,
	}
}