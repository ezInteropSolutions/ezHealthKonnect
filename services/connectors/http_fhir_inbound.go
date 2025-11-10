// services/connectors/http_fhir_inbound.go
// HTTP FHIR Inbound Connector - FHIR Resource Receiver

package connectors

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPFHIRInboundConnector implements HTTP FHIR REST API listener
type HTTPFHIRInboundConnector struct {
	*BaseInboundConnector
	server          *http.Server
	port            int
	basePath        string
	authType        string
	authUsername    string
	authPassword    string
	apiKey          string
	bearerToken     string
	fhirVersion     string
	enableCORS      bool
	maxBodySize     int64
	requestTimeout  time.Duration
	messageChan     chan<- *models.InboundMessage
	mu              sync.RWMutex
}

// NewHTTPFHIRInboundConnector creates a new HTTP FHIR inbound connector
func NewHTTPFHIRInboundConnector() InboundConnector {
	return &HTTPFHIRInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector("http_fhir_inbound", "HTTP FHIR Receiver"),
		basePath:             "/fhir/r4",
		fhirVersion:          "R4",
		enableCORS:           true,
		maxBodySize:          10 * 1024 * 1024, // 10MB
		requestTimeout:       30 * time.Second,
	}
}

// Initialize configures the HTTP FHIR receiver
func (h *HTTPFHIRInboundConnector) Initialize(config []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Port (required)
	if port, ok := cfg["port"].(float64); ok {
		h.port = int(port)
	} else {
		return fmt.Errorf("port is required")
	}

	// Base path
	if basePath, ok := cfg["basePath"].(string); ok && basePath != "" {
		h.basePath = basePath
	}

	// FHIR version
	if version, ok := cfg["fhirVersion"].(string); ok && version != "" {
		h.fhirVersion = version
	}

	// Authentication
	if authType, ok := cfg["authType"].(string); ok {
		h.authType = authType
		switch authType {
		case "basic":
			h.authUsername, _ = cfg["username"].(string)
			h.authPassword, _ = cfg["password"].(string)
		case "bearer":
			h.bearerToken, _ = cfg["bearerToken"].(string)
		case "api_key":
			h.apiKey, _ = cfg["apiKey"].(string)
		}
	}

	log.Printf("✅ HTTP FHIR Inbound initialized: port=%d, path=%s, version=%s, auth=%s",
		h.port, h.basePath, h.fhirVersion, h.authType)

	return nil
}

// Start begins listening for HTTP FHIR requests
func (h *HTTPFHIRInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	h.mu.Lock()
	h.messageChan = messageChan
	h.mu.Unlock()

	// Create HTTP server
	mux := http.NewServeMux()

	// Register FHIR endpoint
	mux.HandleFunc(h.basePath, h.handleFHIRRequest)
	mux.HandleFunc(h.basePath+"/", h.handleFHIRRequest)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      h.loggingMiddleware(h.authMiddleware(mux)),
		ReadTimeout:  h.requestTimeout,
		WriteTimeout: h.requestTimeout,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("🌐 HTTP FHIR Inbound: Listening on port %d (path: %s, FHIR %s)",
			h.port, h.basePath, h.fhirVersion)

		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP FHIR Inbound: Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	return h.Stop()
}

// Stop gracefully shuts down the HTTP server
func (h *HTTPFHIRInboundConnector) Stop() error {
	if h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Printf("🛑 HTTP FHIR Inbound: Shutting down server on port %d", h.port)
		return h.server.Shutdown(ctx)
	}
	return nil
}

// handleFHIRRequest processes incoming FHIR resources
func (h *HTTPFHIRInboundConnector) handleFHIRRequest(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	if h.enableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Only accept POST for now (receiving FHIR resources)
	if r.Method != http.MethodPost {
		h.sendOperationOutcome(w, http.StatusMethodNotAllowed, "error", "invalid",
			fmt.Sprintf("Method %s not allowed. Use POST to submit FHIR resources.", r.Method))
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodySize))
	if err != nil {
		h.sendOperationOutcome(w, http.StatusBadRequest, "error", "invalid",
			fmt.Sprintf("Failed to read request body: %v", err))
		return
	}
	defer r.Body.Close()

	// Validate it's JSON
	var fhirResource map[string]interface{}
	if err := json.Unmarshal(body, &fhirResource); err != nil {
		h.sendOperationOutcome(w, http.StatusBadRequest, "error", "structure",
			fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Check for resourceType
	resourceType, ok := fhirResource["resourceType"].(string)
	if !ok || resourceType == "" {
		h.sendOperationOutcome(w, http.StatusBadRequest, "error", "required",
			"Missing required field: resourceType")
		return
	}

	// Create inbound message
	msg := &models.InboundMessage{
		MessageID:   fmt.Sprintf("http_%s_%d", r.RemoteAddr, time.Now().UnixNano()),
		Content:     body,
		ContentType: "application/fhir+json",
		Protocol:    "http",
		SourceIP:    strings.Split(r.RemoteAddr, ":")[0],
		Headers: map[string]string{
			"Content-Type": r.Header.Get("Content-Type"),
			"User-Agent":   r.Header.Get("User-Agent"),
		},
		ReceivedAt:  time.Now(),
		Format:      "fhir",
		MessageType: resourceType,
	}

	// Send to processing engine
	select {
	case h.messageChan <- msg:
		log.Printf("📥 HTTP FHIR Inbound: Received %s resource (%d bytes) from %s",
			resourceType, len(body), r.RemoteAddr)

		// Send success response
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resourceType": "OperationOutcome",
			"issue": []map[string]interface{}{
				{
					"severity":    "information",
					"code":        "informational",
					"diagnostics": fmt.Sprintf("%s resource accepted for processing", resourceType),
				},
			},
		})

	default:
		h.sendOperationOutcome(w, http.StatusServiceUnavailable, "error", "transient",
			"Message queue full, please retry")
	}
}

// authMiddleware validates authentication
func (h *HTTPFHIRInboundConnector) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip if no auth configured
		if h.authType == "" || h.authType == "none" {
			next.ServeHTTP(w, r)
			return
		}

		switch h.authType {
		case "basic":
			if !h.validateBasicAuth(r) {
				w.Header().Set("WWW-Authenticate", `Basic realm="FHIR API"`)
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security",
					"Authentication required")
				return
			}
		case "bearer":
			if !h.validateBearerToken(r) {
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security",
					"Invalid or missing bearer token")
				return
			}
		case "api_key":
			if !h.validateAPIKey(r) {
				h.sendOperationOutcome(w, http.StatusUnauthorized, "error", "security",
					"Invalid or missing API key")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// validateBasicAuth checks HTTP Basic Authentication
func (h *HTTPFHIRInboundConnector) validateBasicAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}

	payload, err := base64.StdEncoding.DecodeString(auth[6:])
	if err != nil {
		return false
	}

	parts := strings.SplitN(string(payload), ":", 2)
	if len(parts) != 2 {
		return false
	}

	username, password := parts[0], parts[1]
	return subtle.ConstantTimeCompare([]byte(username), []byte(h.authUsername)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(h.authPassword)) == 1
}

// validateBearerToken checks Bearer token
func (h *HTTPFHIRInboundConnector) validateBearerToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := auth[7:]
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.bearerToken)) == 1
}

// validateAPIKey checks API key
func (h *HTTPFHIRInboundConnector) validateAPIKey(r *http.Request) bool {
	apiKey := r.Header.Get("X-API-Key")
	return subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) == 1
}

// loggingMiddleware logs HTTP requests
func (h *HTTPFHIRInboundConnector) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("📊 HTTP FHIR: %s %s from %s (%v)",
			r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

// sendOperationOutcome sends a FHIR OperationOutcome response
func (h *HTTPFHIRInboundConnector) sendOperationOutcome(w http.ResponseWriter, status int, severity, code, message string) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{
			{
				"severity":    severity,
				"code":        code,
				"diagnostics": message,
			},
		},
	})
}

// SupportsCron returns false (HTTP receivers don't use cron)
func (h *HTTPFHIRInboundConnector) SupportsCron() bool {
	return false
}

// Validate checks the configuration
func (h *HTTPFHIRInboundConnector) Validate() error {
	if h.port == 0 {
		return fmt.Errorf("port must be configured")
	}
	if h.port < 1 || h.port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// TestConnection checks if the port is available
func (h *HTTPFHIRInboundConnector) TestConnection(ctx context.Context) error {
	// Try to bind to the port temporarily
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", h.port, err)
	}
	ln.Close()
	return nil
}

// GetStatus returns the current connector status
func (h *HTTPFHIRInboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := ConnectorStatus{
		Name:      h.metadata.Name,
		Type:      h.metadata.Type,
		State:     "unknown",
		Timestamp: time.Now(),
	}

	if h.server != nil {
		status.State = "running"
		status.Details = map[string]interface{}{
			"port":     h.port,
			"basePath": h.basePath,
			"version":  h.fhirVersion,
			"auth":     h.authType,
		}
	} else {
		status.State = "stopped"
	}

	return status
}
