// services/connectors/http_rest_inbound.go
// HTTP/REST Inbound Connector — a generic HTTP receiver with NO FHIR-specific
// behavior: no resource-type routing, no Bundle handling, no FHIR versioning.
// It accepts a request at a configured path/method(s), applies simple
// authentication, and forwards the raw request body as an InboundMessage's
// content, unparsed. Use this for a plain webhook-style intake (e.g. raw HL7
// v2 posted over HTTP, a generic JSON payload, or any other format your own
// pipeline steps parse downstream).
//
// This is deliberately a separate implementation from
// http_fhir_inbound.go's HTTPFHIRInboundConnector — for a while, this
// connector type (registered as "http_rest"/"http_rest_inbound"/"http") was
// silently aliased to the FHIR connector in connector_factory.go, and this
// session's V222 migration went further and overwrote this type's
// connectivity_types schema to match the FHIR connector's schema, treating
// the two as interchangeable. Both were mistakes: this connector and the
// FHIR one are meant to serve genuinely different use cases (FHIR REST API
// semantics vs. a plain generic HTTP intake) and were always meant to have
// their own distinct settings screens. This file is the real implementation
// that should have existed for "http_rest_inbound" all along; a follow-up
// migration restores its own schema (with actual credential-value fields
// added, which the original schema was missing entirely).
//
// Configuration:
//
//	port                  int      Listen port (required)
//	endpoint_path         string   URL path to accept requests on (default: "/api/hl7/receive")
//	http_methods          []string Allowed HTTP methods (default: ["POST"])
//	content_type          string   Fallback Content-Type when the request has none (default: "text/plain")
//	max_body_size_mb      int      Maximum accepted request body size (default: 10)
//	authentication_type   string   "none" | "api_key" | "basic_auth" | "bearer_token" (default: "api_key")
//	api_key_header        string   Header name the caller sends the API key in (default: "X-API-Key")
//	api_key               string   Expected API key value (for api_key auth)
//	username              string   Expected username (for basic_auth)
//	password              string   Expected password (for basic_auth)
//	bearer_token          string   Expected bearer token value (for bearer_token auth)
package connectors

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// HTTPRestInboundConnector implements a generic (non-FHIR) HTTP request receiver.
type HTTPRestInboundConnector struct {
	*BaseInboundConnector

	port           int
	endpointPath   string
	httpMethods    map[string]bool
	contentType    string
	maxBodySize    int64
	authType       string
	apiKeyHeader   string
	apiKey         string
	username       string
	password       string
	bearerToken    string
	requestTimeout time.Duration

	server      *http.Server
	messageChan chan<- *models.InboundMessage
	mu          sync.RWMutex
}

// NewHTTPRestInboundConnector creates a production generic HTTP/REST inbound connector.
func NewHTTPRestInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "http_rest_inbound",
		DisplayName:        "HTTP/REST API Receiver",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  false,
			"supports_auth":  true,
			"supports_batch": false,
		},
	}
	return &HTTPRestInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		endpointPath:         "/api/hl7/receive",
		httpMethods:          map[string]bool{"POST": true},
		contentType:          "text/plain",
		maxBodySize:          10 * 1024 * 1024,
		authType:             "api_key",
		apiKeyHeader:         "X-API-Key",
		requestTimeout:       30 * time.Second,
	}
}

// Initialize parses configuration.
func (h *HTTPRestInboundConnector) Initialize(config []byte) error {
	if err := h.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := h.GetConfig()

	h.port = cfg.GetInt("port")
	if h.port == 0 {
		return fmt.Errorf("port is required")
	}

	if v := cfg.GetString("endpoint_path"); v != "" {
		h.endpointPath = v
	}
	if !strings.HasPrefix(h.endpointPath, "/") {
		h.endpointPath = "/" + h.endpointPath
	}

	if methods := cfg.GetStringSlice("http_methods"); len(methods) > 0 {
		h.httpMethods = make(map[string]bool, len(methods))
		for _, m := range methods {
			h.httpMethods[strings.ToUpper(m)] = true
		}
	}

	if v := cfg.GetString("content_type"); v != "" {
		h.contentType = v
	}

	if v := cfg.GetInt("max_body_size_mb"); v > 0 {
		h.maxBodySize = int64(v) * 1024 * 1024
	}

	if v := cfg.GetString("authentication_type"); v != "" {
		h.authType = v
	}
	switch h.authType {
	case "none", "api_key", "basic_auth", "bearer_token":
	default:
		return fmt.Errorf("authentication_type must be 'none', 'api_key', 'basic_auth', or 'bearer_token', got %q", h.authType)
	}
	if v := cfg.GetString("api_key_header"); v != "" {
		h.apiKeyHeader = v
	}
	h.apiKey = cfg.GetString("api_key")
	h.username = cfg.GetString("username")
	h.password = cfg.GetString("password")
	h.bearerToken = cfg.GetString("bearer_token")

	log.Printf("✅ HTTP/REST Inbound initialized: port=%d path=%s methods=%v auth=%s",
		h.port, h.endpointPath, h.httpMethods, h.authType)
	return nil
}

// Validate checks required configuration.
func (h *HTTPRestInboundConnector) Validate() error {
	if err := h.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if h.port < 1 || h.port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	switch h.authType {
	case "api_key":
		if h.apiKey == "" {
			return fmt.Errorf("api_key is required when authentication_type is 'api_key'")
		}
	case "basic_auth":
		if h.username == "" || h.password == "" {
			return fmt.Errorf("username and password are required when authentication_type is 'basic_auth'")
		}
	case "bearer_token":
		if h.bearerToken == "" {
			return fmt.Errorf("bearer_token is required when authentication_type is 'bearer_token'")
		}
	}
	return nil
}

// TestConnection verifies the configured port is available.
func (h *HTTPRestInboundConnector) TestConnection(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", h.port, err)
	}
	return ln.Close()
}

// SupportsCron returns false — this is a push (event-driven) receiver.
func (h *HTTPRestInboundConnector) SupportsCron() bool { return false }

// Start launches the HTTP server.
func (h *HTTPRestInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	h.mu.Lock()
	h.messageChan = messageChan
	h.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(h.endpointPath, h.handleRequest)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      mux,
		ReadTimeout:  h.requestTimeout,
		WriteTimeout: h.requestTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🌐 HTTP/REST Inbound: listening on :%d%s (methods=%v)", h.port, h.endpointPath, h.httpMethods)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ HTTP/REST Inbound: server error: %v", err)
		}
	}()

	<-ctx.Done()
	return h.Stop()
}

// Stop shuts down the HTTP server.
func (h *HTTPRestInboundConnector) Stop() error {
	if h.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Printf("🛑 HTTP/REST Inbound: shutting down on port %d", h.port)
	return h.server.Shutdown(ctx)
}

// GetStatus returns connector status.
func (h *HTTPRestInboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st := ConnectorStatus{
		Connected:    h.server != nil,
		LastActivity: time.Now(),
		Metadata: map[string]string{
			"port":          fmt.Sprintf("%d", h.port),
			"endpoint_path": h.endpointPath,
			"auth":          h.authType,
		},
	}
	if h.server != nil {
		st.State = StateRunning
	} else {
		st.State = StateStopped
	}
	return st
}

// --------------------------------------------------------------------------
// Request handling
// --------------------------------------------------------------------------

func (h *HTTPRestInboundConnector) handleRequest(w http.ResponseWriter, r *http.Request) {
	if !h.httpMethods[r.Method] {
		http.Error(w, fmt.Sprintf(`{"error":"method %s not allowed"}`, r.Method), http.StatusMethodNotAllowed)
		return
	}
	if !h.checkAuth(r) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"authentication failed"}`, http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodySize))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to read body: %v"}`, err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = h.contentType
	}

	msg := &models.InboundMessage{
		MessageID:      fmt.Sprintf("http_rest_%d", time.Now().UnixNano()),
		Content:        string(body),
		ContentType:    contentType,
		SourceType:     "http_rest",
		SourceEndpoint: r.RequestURI,
		MessageSize:    len(body),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"X-HTTP-Method": r.Method,
			"Content-Type":  contentType,
			"User-Agent":    r.Header.Get("User-Agent"),
		},
	}

	h.mu.RLock()
	messageChan := h.messageChan
	h.mu.RUnlock()

	select {
	case messageChan <- msg:
		log.Printf("📥 HTTP/REST Inbound: %s %s → %d bytes accepted from %s", r.Method, r.URL.Path, len(body), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "message_id": msg.MessageID})
	default:
		http.Error(w, `{"error":"message queue full, please retry"}`, http.StatusServiceUnavailable)
	}
}

func (h *HTTPRestInboundConnector) checkAuth(r *http.Request) bool {
	switch h.authType {
	case "none":
		return true
	case "api_key":
		return subtle.ConstantTimeCompare([]byte(r.Header.Get(h.apiKeyHeader)), []byte(h.apiKey)) == 1
	case "basic_auth":
		user, pass, ok := r.BasicAuth()
		if !ok {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(user), []byte(h.username)) == 1 &&
			subtle.ConstantTimeCompare([]byte(pass), []byte(h.password)) == 1
	case "bearer_token":
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(h.bearerToken)) == 1
	default:
		return false
	}
}

