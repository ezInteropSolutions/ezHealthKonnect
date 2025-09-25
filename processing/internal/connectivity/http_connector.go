// internal/connectivity/http_connector.go
// HTTP and FHIR connectivity handlers

package connectivity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
)

// HTTPInputConnector handles HTTP message reception (webhook server)
type HTTPInputConnector struct {
	*BaseConnector
	server      *http.Server
	messageChan chan<- *pkg.UniversalMessage
	mux         *http.ServeMux
	middleware  []HTTPMiddleware
}

// HTTPOutputConnector handles HTTP message transmission
type HTTPOutputConnector struct {
	*BaseConnector
	client     *http.Client
	authToken  string
	headers    map[string]string
	retryDelay time.Duration
}

// FHIROutputConnector handles FHIR server communication
type FHIROutputConnector struct {
	*HTTPOutputConnector
	fhirVersion  string
	serverURL    string
	capabilities *FHIRCapabilities
	capMutex     sync.RWMutex
}

// HTTPMiddleware defines middleware for HTTP processing
type HTTPMiddleware func(http.Handler) http.Handler

// FHIRCapabilities represents FHIR server capabilities
type FHIRCapabilities struct {
	FHIRVersion     string                       `json:"fhirVersion"`
	SupportedFormats []string                    `json:"format"`
	Resources       []FHIRResourceCapability     `json:"rest"`
	LastUpdated     time.Time                    `json:"lastUpdated"`
}

// FHIRResourceCapability represents capabilities for a FHIR resource
type FHIRResourceCapability struct {
	Type         string   `json:"type"`
	Interactions []string `json:"interaction"`
	SearchParams []string `json:"searchParam,omitempty"`
}

// NewHTTPInputConnector creates a new HTTP input connector
func NewHTTPInputConnector(config pkg.ConnectorConfig) (*HTTPInputConnector, error) {
	base := NewBaseConnector(config, "http_input")

	mux := http.NewServeMux()

	connector := &HTTPInputConnector{
		BaseConnector: base,
		mux:          mux,
		middleware:   []HTTPMiddleware{},
	}

	// Set up default routes
	connector.setupRoutes()

	return connector, nil
}

// NewHTTPOutputConnector creates a new HTTP output connector
func NewHTTPOutputConnector(config pkg.ConnectorConfig) (*HTTPOutputConnector, error) {
	base := NewBaseConnector(config, "http_output")

	// Create HTTP client with timeout and retry settings
	client := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
			MaxConnsPerHost:     5,
		},
	}

	headers := make(map[string]string)
	if contentType, exists := config.Settings["content_type"]; exists {
		if ct, ok := contentType.(string); ok {
			headers["Content-Type"] = ct
		}
	} else {
		headers["Content-Type"] = "application/json"
	}

	retryDelay := 1 * time.Second
	if delay, exists := config.Settings["retry_delay"]; exists {
		if d, ok := delay.(float64); ok {
			retryDelay = time.Duration(d) * time.Second
		}
	}

	connector := &HTTPOutputConnector{
		BaseConnector: base,
		client:        client,
		authToken:     config.Token,
		headers:       headers,
		retryDelay:    retryDelay,
	}

	return connector, nil
}

// NewFHIROutputConnector creates a new FHIR output connector
func NewFHIROutputConnector(config pkg.ConnectorConfig) (*FHIROutputConnector, error) {
	httpConnector, err := NewHTTPOutputConnector(config)
	if err != nil {
		return nil, err
	}

	// Override type and set FHIR-specific headers
	httpConnector.Type = "fhir_output"
	httpConnector.headers["Content-Type"] = "application/fhir+json"
	httpConnector.headers["Accept"] = "application/fhir+json"

	fhirVersion := "R4"
	if version, exists := config.Settings["fhir_version"]; exists {
		if v, ok := version.(string); ok {
			fhirVersion = v
		}
	}

	serverURL := fmt.Sprintf("%s://%s:%d", config.Protocol, config.Endpoint, config.Port)
	if config.Path != "" {
		serverURL += config.Path
	}

	connector := &FHIROutputConnector{
		HTTPOutputConnector: httpConnector,
		fhirVersion:         fhirVersion,
		serverURL:           serverURL,
	}

	return connector, nil
}

// StartListening starts the HTTP server
func (hc *HTTPInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := hc.Start(hc.ctx); err != nil {
		return err
	}

	hc.messageChan = messageChan

	// Apply middleware
	var handler http.Handler = hc.mux
	for i := len(hc.middleware) - 1; i >= 0; i-- {
		handler = hc.middleware[i](handler)
	}

	// Create HTTP server
	address := fmt.Sprintf(":%d", hc.Config.Port)
	if hc.Config.Endpoint != "" && hc.Config.Endpoint != "0.0.0.0" {
		address = fmt.Sprintf("%s:%d", hc.Config.Endpoint, hc.Config.Port)
	}

	hc.server = &http.Server{
		Addr:         address,
		Handler:      handler,
		ReadTimeout:  hc.Config.Timeout,
		WriteTimeout: hc.Config.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if err := hc.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hc.RecordError(err)
		}
	}()

	hc.SetConnected(true)
	return nil
}

// StopListening stops the HTTP server
func (hc *HTTPInputConnector) StopListening() error {
	if hc.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hc.server.Shutdown(ctx)
	}

	return hc.Stop()
}

// TestConnection tests HTTP connectivity
func (hc *HTTPInputConnector) TestConnection() error {
	// Test if we can bind to the port
	address := fmt.Sprintf(":%d", hc.Config.Port)
	if hc.Config.Endpoint != "" && hc.Config.Endpoint != "0.0.0.0" {
		address = fmt.Sprintf("%s:%d", hc.Config.Endpoint, hc.Config.Port)
	}

	server := &http.Server{Addr: address}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("cannot bind to %s: %w", address, err)
	}
	listener.Close()

	return nil
}

// Connect establishes HTTP client connectivity
func (hc *HTTPOutputConnector) Connect() error {
	// Test connectivity with a simple request
	testURL := hc.GetConnectionString()
	if !strings.HasSuffix(testURL, "/") {
		testURL += "/"
	}

	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	hc.addAuthHeaders(req)

	ctx, cancel := context.WithTimeout(context.Background(), hc.Config.Timeout)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := hc.client.Do(req)
	if err != nil {
		hc.RecordError(err)
		return fmt.Errorf("failed to connect to %s: %w", testURL, err)
	}
	defer resp.Body.Close()

	hc.SetConnected(true)
	return nil
}

// Disconnect closes HTTP client connections
func (hc *HTTPOutputConnector) Disconnect() error {
	// Close idle connections
	hc.client.CloseIdleConnections()
	hc.SetConnected(false)
	return nil
}

// TestConnection tests HTTP connectivity (output)
func (hc *HTTPOutputConnector) TestConnection() error {
	return hc.Connect()
}

// SendMessage sends a message via HTTP
func (hc *HTTPOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	startTime := time.Now()

	url := hc.GetConnectionString()
	if hc.Config.Path != "" && !strings.HasSuffix(url, hc.Config.Path) {
		url += hc.Config.Path
	}

	// Create request
	req, err := http.NewRequest("POST", url, strings.NewReader(message.Content))
	if err != nil {
		hc.RecordError(err)
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req = req.WithContext(ctx)

	// Add headers
	for key, value := range hc.headers {
		req.Header.Set(key, value)
	}
	hc.addAuthHeaders(req)

	// Add message metadata as headers
	req.Header.Set("X-Message-ID", message.ID)
	req.Header.Set("X-Correlation-ID", message.CorrelationID)
	if message.SourceInterface != "" {
		req.Header.Set("X-Source-Interface", message.SourceInterface)
	}

	// Send request with retries
	var resp *http.Response
	for attempt := 0; attempt <= hc.Config.Retries; attempt++ {
		resp, err = hc.client.Do(req)
		if err == nil {
			break
		}

		if attempt < hc.Config.Retries {
			time.Sleep(hc.retryDelay * time.Duration(attempt+1))
		}
	}

	if err != nil {
		hc.RecordError(err)
		return fmt.Errorf("HTTP request failed after %d attempts: %w", hc.Config.Retries+1, err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		hc.RecordError(err)
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(respBody))
		hc.RecordError(err)
		return err
	}

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	// Store response in metadata
	message.Metadata["http_response_status"] = resp.StatusCode
	message.Metadata["http_response_headers"] = resp.Header
	if len(respBody) > 0 {
		message.Metadata["http_response_body"] = string(respBody)
	}

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	hc.RecordMessage(int64(len(message.Content)), latency)

	return nil
}

// SendMessage sends a FHIR resource
func (fc *FHIROutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	// Ensure we have FHIR server capabilities
	if err := fc.ensureCapabilities(ctx); err != nil {
		return fmt.Errorf("failed to get FHIR capabilities: %w", err)
	}

	// Parse FHIR resource to determine endpoint
	var resource map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &resource); err != nil {
		return fmt.Errorf("failed to parse FHIR resource: %w", err)
	}

	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid resourceType in FHIR resource")
	}

	// Check if resource type is supported
	if !fc.isResourceSupported(resourceType) {
		return fmt.Errorf("resource type %s not supported by FHIR server", resourceType)
	}

	// Build FHIR endpoint URL
	fhirURL := fmt.Sprintf("%s/%s", fc.serverURL, resourceType)

	// Check if resource has an ID (update vs create)
	if resourceID, exists := resource["id"]; exists && resourceID != "" {
		fhirURL += fmt.Sprintf("/%v", resourceID)
		// Use PUT for updates
		return fc.sendFHIRResource(ctx, "PUT", fhirURL, message)
	}

	// Use POST for creation
	return fc.sendFHIRResource(ctx, "POST", fhirURL, message)
}

// SupportsAcknowledgment returns whether HTTP supports acknowledgments
func (hc *HTTPOutputConnector) SupportsAcknowledgment() bool {
	return true // HTTP responses serve as acknowledgments
}

// WaitForAcknowledgment waits for HTTP response (already handled in SendMessage)
func (hc *HTTPOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	// HTTP responses are synchronous, so this is effectively a no-op
	return nil
}

// SendBatch sends multiple messages
func (hc *HTTPOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	for _, message := range messages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := hc.SendMessage(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

// setupRoutes sets up default HTTP routes
func (hc *HTTPInputConnector) setupRoutes() {
	// Health check endpoint
	hc.mux.HandleFunc("/health", hc.handleHealth)

	// Generic message endpoint
	hc.mux.HandleFunc("/message", hc.handleMessage)

	// FHIR endpoints
	hc.mux.HandleFunc("/fhir/", hc.handleFHIR)

	// Webhook endpoint
	hc.mux.HandleFunc("/webhook", hc.handleWebhook)

	// Catch-all for other endpoints
	hc.mux.HandleFunc("/", hc.handleDefault)
}

// HTTP handlers

func (hc *HTTPInputConnector) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := hc.GetStatus()
	response := map[string]interface{}{
		"status":     "healthy",
		"connector":  status,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (hc *HTTPInputConnector) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hc.processHTTPMessage(w, r, "HTTP")
}

func (hc *HTTPInputConnector) handleFHIR(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hc.processHTTPMessage(w, r, "FHIR")
}

func (hc *HTTPInputConnector) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hc.processHTTPMessage(w, r, "WEBHOOK")
}

func (hc *HTTPInputConnector) handleDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" || r.Method == "PUT" {
		hc.processHTTPMessage(w, r, "UNKNOWN")
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// processHTTPMessage processes an incoming HTTP message
func (hc *HTTPInputConnector) processHTTPMessage(w http.ResponseWriter, r *http.Request, messageType string) {
	startTime := time.Now()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		hc.RecordError(err)
		return
	}
	defer r.Body.Close()

	// Create universal message
	message := pkg.NewUniversalMessage()
	message.Content = string(body)
	message.ContentType = messageType
	message.SourceProtocol = string(hc.Protocol)
	message.SourceEndpoint = r.Host
	message.SourceIP = r.RemoteAddr
	message.Size = int64(len(body))

	// Extract headers into metadata
	headers := make(map[string]interface{})
	for key, values := range r.Header {
		if len(values) == 1 {
			headers[key] = values[0]
		} else {
			headers[key] = values
		}
	}
	message.Metadata["http_headers"] = headers
	message.Metadata["http_method"] = r.Method
	message.Metadata["http_url"] = r.URL.String()
	message.Metadata["http_user_agent"] = r.UserAgent()

	// Extract message ID from headers if present
	if msgID := r.Header.Get("X-Message-ID"); msgID != "" {
		message.ID = msgID
	}
	if corrID := r.Header.Get("X-Correlation-ID"); corrID != "" {
		message.CorrelationID = corrID
	}

	// Determine content type from header
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		if strings.Contains(contentType, "fhir") {
			message.ContentType = "FHIR"
		} else if strings.Contains(contentType, "hl7") {
			message.ContentType = "HL7"
		} else if strings.Contains(contentType, "xml") {
			message.ContentType = "XML"
		} else if strings.Contains(contentType, "json") {
			message.ContentType = "JSON"
		}
	}

	// Send to processing channel
	select {
	case hc.messageChan <- message:
		// Success response
		response := map[string]interface{}{
			"status":        "accepted",
			"message_id":    message.ID,
			"correlation_id": message.CorrelationID,
			"timestamp":     time.Now().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)

		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		hc.RecordMessage(message.Size, latency)

	case <-time.After(5 * time.Second):
		http.Error(w, "Processing queue full", http.StatusServiceUnavailable)
	}
}

// addAuthHeaders adds authentication headers to the request
func (hc *HTTPOutputConnector) addAuthHeaders(req *http.Request) {
	if hc.authToken != "" {
		if hc.Config.AuthType == "bearer" || hc.Config.AuthType == "" {
			req.Header.Set("Authorization", "Bearer "+hc.authToken)
		} else if hc.Config.AuthType == "basic" {
			req.Header.Set("Authorization", "Basic "+hc.authToken)
		} else {
			req.Header.Set("Authorization", hc.authToken)
		}
	}

	if hc.Config.Username != "" && hc.Config.Password != "" {
		req.SetBasicAuth(hc.Config.Username, hc.Config.Password)
	}
}

// FHIR-specific methods

// ensureCapabilities fetches FHIR server capabilities if not cached
func (fc *FHIROutputConnector) ensureCapabilities(ctx context.Context) error {
	fc.capMutex.RLock()
	if fc.capabilities != nil && time.Since(fc.capabilities.LastUpdated) < 1*time.Hour {
		fc.capMutex.RUnlock()
		return nil
	}
	fc.capMutex.RUnlock()

	// Fetch capabilities
	capURL := fc.serverURL + "/metadata"
	req, err := http.NewRequest("GET", capURL, nil)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/fhir+json")
	fc.addAuthHeaders(req)

	resp, err := fc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to fetch FHIR capabilities: status %d", resp.StatusCode)
	}

	var capStatement map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&capStatement); err != nil {
		return err
	}

	// Parse capabilities (simplified)
	capabilities := &FHIRCapabilities{
		FHIRVersion:     fc.fhirVersion,
		SupportedFormats: []string{"application/fhir+json"},
		Resources:       []FHIRResourceCapability{},
		LastUpdated:     time.Now(),
	}

	// TODO: Parse actual capability statement
	// For now, assume common resources are supported
	commonResources := []string{"Patient", "Observation", "DiagnosticReport", "Organization", "Practitioner"}
	for _, resourceType := range commonResources {
		capabilities.Resources = append(capabilities.Resources, FHIRResourceCapability{
			Type:         resourceType,
			Interactions: []string{"create", "read", "update", "delete"},
		})
	}

	fc.capMutex.Lock()
	fc.capabilities = capabilities
	fc.capMutex.Unlock()

	return nil
}

// isResourceSupported checks if a FHIR resource type is supported
func (fc *FHIROutputConnector) isResourceSupported(resourceType string) bool {
	fc.capMutex.RLock()
	defer fc.capMutex.RUnlock()

	if fc.capabilities == nil {
		return true // Assume supported if capabilities unknown
	}

	for _, resource := range fc.capabilities.Resources {
		if resource.Type == resourceType {
			return true
		}
	}

	return false
}

// sendFHIRResource sends a FHIR resource with specific method and URL
func (fc *FHIROutputConnector) sendFHIRResource(ctx context.Context, method, url string, message *pkg.UniversalMessage) error {
	req, err := http.NewRequest(method, url, strings.NewReader(message.Content))
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Accept", "application/fhir+json")
	fc.addAuthHeaders(req)

	resp, err := fc.client.Do(req)
	if err != nil {
		fc.RecordError(err)
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("FHIR request failed with status %d: %s", resp.StatusCode, string(respBody))
		fc.RecordError(err)
		return err
	}

	// Update message with FHIR response
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	message.Metadata["fhir_response_status"] = resp.StatusCode
	message.Metadata["fhir_response_headers"] = resp.Header
	if len(respBody) > 0 {
		message.Metadata["fhir_response_body"] = string(respBody)
	}

	return nil
}