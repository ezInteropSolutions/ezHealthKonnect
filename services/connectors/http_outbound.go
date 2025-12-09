// services/connectors/http_outbound.go
// HTTP Outbound Connector - Delivers messages via HTTP/HTTPS POST

package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPOutboundConnector implements HTTP/HTTPS message delivery
type HTTPOutboundConnector struct {
	*BaseOutboundConnector
	endpoint       string
	method         string // POST, PUT, PATCH
	authType       string
	authUsername   string
	authPassword   string
	bearerToken    string
	apiKey         string
	apiKeyHeader   string
	timeout        time.Duration
	retryAttempts  int
	retryDelay     time.Duration
	client         *http.Client
	mu             sync.RWMutex
}

// NewHTTPOutboundConnector creates a new HTTP outbound connector
func NewHTTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "http_outbound",
		DisplayName:        "HTTP/HTTPS Endpoint",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_auth":  true,
			"supports_retry": true,
		},
	}
	return &HTTPOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
		method:                "POST",
		timeout:               30 * time.Second,
		retryAttempts:         3,
		retryDelay:            1 * time.Second,
		apiKeyHeader:          "X-API-Key", // Default
	}
}

// Initialize configures the HTTP outbound connector
func (h *HTTPOutboundConnector) Initialize(config []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Printf("🔍 HTTP Outbound Initialize called")

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Endpoint (required)
	if endpoint, ok := cfg["endpoint"].(string); ok && endpoint != "" {
		h.endpoint = endpoint
		log.Printf("🔍 Endpoint set to: %s", h.endpoint)
	} else {
		return fmt.Errorf("endpoint is required")
	}

	// HTTP method (optional, defaults to POST)
	if method, ok := cfg["method"].(string); ok && method != "" {
		h.method = strings.ToUpper(method)
		log.Printf("🔍 Method set to: %s", h.method)
	}

	// Timeout (optional)
	if timeout, ok := cfg["timeout"].(float64); ok && timeout > 0 {
		h.timeout = time.Duration(timeout) * time.Second
		log.Printf("🔍 Timeout set to: %v", h.timeout)
	}

	// Retry settings (optional)
	if retryAttempts, ok := cfg["retryAttempts"].(float64); ok {
		h.retryAttempts = int(retryAttempts)
		log.Printf("🔍 Retry attempts set to: %d", h.retryAttempts)
	}
	if retryDelay, ok := cfg["retryDelay"].(float64); ok {
		h.retryDelay = time.Duration(retryDelay) * time.Second
		log.Printf("🔍 Retry delay set to: %v", h.retryDelay)
	}

	// Authentication
	if authType, ok := cfg["authType"].(string); ok {
		h.authType = authType
		log.Printf("🔍 Auth type set to: %s", h.authType)

		switch authType {
		case "basic":
			h.authUsername, _ = cfg["username"].(string)
			h.authPassword, _ = cfg["password"].(string)
			log.Printf("🔐 Basic Auth configured: username='%s'", h.authUsername)

		case "bearer":
			h.bearerToken, _ = cfg["bearerToken"].(string)
			log.Printf("🔐 Bearer Token configured")

		case "api_key":
			h.apiKey, _ = cfg["apiKey"].(string)
			if header, ok := cfg["apiKeyHeader"].(string); ok && header != "" {
				h.apiKeyHeader = header
			}
			log.Printf("🔐 API Key configured (header: %s)", h.apiKeyHeader)
		}
	}

	// Create HTTP client with timeout
	h.client = &http.Client{
		Timeout: h.timeout,
	}

	log.Printf("✅ HTTP Outbound initialized: endpoint=%s, method=%s, auth=%s, timeout=%v",
		h.endpoint, h.method, h.authType, h.timeout)

	return nil
}

// Send delivers a message to the HTTP endpoint
func (h *HTTPOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	startTime := time.Now()
	result := &DeliveryResult{
		MessageID:  message.MessageID,
		Timestamp:  startTime,
		RetryCount: 0,
		Success:    false,
		Metadata:   make(map[string]interface{}),
	}

	log.Printf("🚀 [HTTP OUT] Sending message %s to %s", message.MessageID, h.endpoint)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, h.method, h.endpoint, bytes.NewReader([]byte(message.Content)))
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to create request: %v", err)
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, err
	}

	// Add headers from message
	for key, value := range message.Headers {
		req.Header.Set(key, value)
	}

	// Set Content-Type if provided
	if message.ContentType != "" {
		req.Header.Set("Content-Type", message.ContentType)
	}

	// Add authentication
	h.addAuthentication(req)

	// Execute with retries
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= h.retryAttempts; attempt++ {
		if attempt > 0 {
			log.Printf("🔄 [HTTP OUT] Retry attempt %d/%d for message %s", attempt, h.retryAttempts, message.MessageID)
			time.Sleep(h.retryDelay)
			result.RetryCount = attempt
		}

		resp, lastErr = h.client.Do(req)
		if lastErr == nil {
			break
		}

		log.Printf("⚠️ [HTTP OUT] Attempt %d failed: %v", attempt+1, lastErr)
	}

	result.DurationMs = time.Since(startTime).Milliseconds()

	if lastErr != nil {
		result.ErrorMessage = fmt.Sprintf("HTTP request failed after %d retries: %v", result.RetryCount, lastErr)
		log.Printf("❌ [HTTP OUT] Message %s delivery failed: %s", message.MessageID, result.ErrorMessage)
		return result, lastErr
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("⚠️ [HTTP OUT] Failed to read response body: %v", err)
		respBody = []byte{}
	}

	// Store response details in metadata
	result.Metadata["status_code"] = resp.StatusCode
	result.Metadata["response_body"] = string(respBody)
	result.Metadata["response_headers"] = resp.Header

	// Check response status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		result.Acknowledgment = fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Printf("✅ [HTTP OUT] Message %s delivered successfully (status: %d, duration: %dms)",
			message.MessageID, resp.StatusCode, result.DurationMs)
		log.Printf("📥 [HTTP OUT] Response: %s", truncateString(string(respBody), 200))
	} else {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
		log.Printf("❌ [HTTP OUT] Message %s delivery failed with HTTP %d (duration: %dms)",
			message.MessageID, resp.StatusCode, result.DurationMs)
		log.Printf("📥 [HTTP OUT] Error response: %s", truncateString(string(respBody), 200))
	}

	return result, nil
}

// SendBatch delivers multiple messages (not supported, falls back to individual sends)
func (h *HTTPOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	results := make([]*DeliveryResult, len(messages))
	for i, msg := range messages {
		result, err := h.Send(ctx, msg)
		if err != nil {
			result.ErrorMessage = err.Error()
		}
		results[i] = result
	}
	return results, nil
}

// SupportsBatch returns false (HTTP outbound doesn't support native batching)
func (h *HTTPOutboundConnector) SupportsBatch() bool {
	return false
}

// Validate checks the configuration
func (h *HTTPOutboundConnector) Validate() error {
	if h.endpoint == "" {
		return fmt.Errorf("endpoint must be configured")
	}
	if !strings.HasPrefix(h.endpoint, "http://") && !strings.HasPrefix(h.endpoint, "https://") {
		return fmt.Errorf("endpoint must start with http:// or https://")
	}
	return nil
}

// TestConnection checks if the endpoint is reachable
func (h *HTTPOutboundConnector) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "HEAD", h.endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	h.addAuthentication(req)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("endpoint not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("endpoint returned server error: %d", resp.StatusCode)
	}

	return nil
}

// GetStatus returns the current connector status
func (h *HTTPOutboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := ConnectorStatus{
		Connected:    h.client != nil,
		LastActivity: time.Now(),
		State:        StateReady,
		Metadata: map[string]string{
			"endpoint": h.endpoint,
			"method":   h.method,
			"auth":     h.authType,
			"timeout":  h.timeout.String(),
		},
	}

	return status
}

// addAuthentication adds authentication headers to the request
func (h *HTTPOutboundConnector) addAuthentication(req *http.Request) {
	switch h.authType {
	case "basic":
		if h.authUsername != "" && h.authPassword != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(h.authUsername + ":" + h.authPassword))
			req.Header.Set("Authorization", "Basic "+auth)
			log.Printf("🔐 [HTTP OUT] Added Basic Auth for user: %s", h.authUsername)
		}

	case "bearer":
		if h.bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+h.bearerToken)
			log.Printf("🔐 [HTTP OUT] Added Bearer Token")
		}

	case "api_key":
		if h.apiKey != "" {
			req.Header.Set(h.apiKeyHeader, h.apiKey)
			log.Printf("🔐 [HTTP OUT] Added API Key header: %s", h.apiKeyHeader)
		}
	}
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
