// services/connectors/http_outbound.go
// HTTP Outbound Connector - Delivers messages via HTTP/HTTPS POST
// Uses shared HTTPClientService for OOP code reuse

package connectors

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	httpservice "ezhealthkonnect/services/http"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// HTTPOutboundConnector implements HTTP/HTTPS message delivery
type HTTPOutboundConnector struct {
	*BaseOutboundConnector
	endpoint      string
	method        string // POST, PUT, PATCH
	authConfig    *httpservice.AuthConfig
	timeout       time.Duration
	retryAttempts int
	retryDelay    time.Duration
	httpService   *httpservice.HTTPClientService
	mu            sync.RWMutex
}

// NewHTTPOutboundConnector creates a new HTTP outbound connector
func NewHTTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "http_outbound",
		DisplayName:        "HTTP/HTTPS Endpoint",
		Version:            "2.0.0", // Upgraded to use shared HTTPClientService
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
		httpService:           httpservice.NewHTTPClientService(30 * time.Second),
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

	// Endpoint (required) — schema field is "url", legacy field is "endpoint"
	endpoint, _ := cfg["url"].(string)
	if endpoint == "" {
		endpoint, _ = cfg["endpoint"].(string)
	}
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	h.endpoint = endpoint
	log.Printf("🔍 Endpoint set to: %s", h.endpoint)

	// HTTP method (optional, defaults to POST)
	if method, ok := cfg["method"].(string); ok && method != "" {
		h.method = strings.ToUpper(method)
		log.Printf("🔍 Method set to: %s", h.method)
	}

	// Timeout — schema field is "timeout_seconds", legacy field is "timeout"
	if timeout, ok := cfg["timeout_seconds"].(float64); ok && timeout > 0 {
		h.timeout = time.Duration(timeout) * time.Second
		log.Printf("🔍 Timeout set to: %v", h.timeout)
	} else if timeout, ok := cfg["timeout"].(float64); ok && timeout > 0 {
		h.timeout = time.Duration(timeout) * time.Second
		log.Printf("🔍 Timeout set to: %v", h.timeout)
	}

	// Retry settings — schema fields are "retry_attempts"/"retry_delay_seconds"
	if retryAttempts, ok := cfg["retry_attempts"].(float64); ok {
		h.retryAttempts = int(retryAttempts)
	} else if retryAttempts, ok := cfg["retryAttempts"].(float64); ok {
		h.retryAttempts = int(retryAttempts)
	}
	if retryDelay, ok := cfg["retry_delay_seconds"].(float64); ok {
		h.retryDelay = time.Duration(retryDelay) * time.Second
	} else if retryDelay, ok := cfg["retryDelay"].(float64); ok {
		h.retryDelay = time.Duration(retryDelay) * time.Second
	}

	// Authentication — schema field is "authentication_type", values: basic_auth/bearer_token/api_key
	authType, _ := cfg["authentication_type"].(string)
	if authType == "" {
		authType, _ = cfg["authType"].(string)
	}
	if authType != "" && authType != "none" {
		log.Printf("🔍 Auth type set to: %s", authType)

		h.authConfig = &httpservice.AuthConfig{}

		switch authType {
		case "basic_auth", "basic":
			h.authConfig.Type = httpservice.AuthTypeBasic
			h.authConfig.Username, _ = cfg["username"].(string)
			h.authConfig.Password, _ = cfg["password"].(string)
			log.Printf("🔐 Basic Auth configured: username='%s'", h.authConfig.Username)

		case "bearer_token", "bearer":
			h.authConfig.Type = httpservice.AuthTypeBearer
			// Schema field is "bearer_token", legacy is "bearerToken"
			token, _ := cfg["bearer_token"].(string)
			if token == "" {
				token, _ = cfg["bearerToken"].(string)
			}
			h.authConfig.BearerToken = token
			log.Printf("🔐 Bearer Token configured")

		case "api_key":
			h.authConfig.Type = httpservice.AuthTypeAPIKey
			// Schema field is "api_key", legacy is "apiKey"
			apiKey, _ := cfg["api_key"].(string)
			if apiKey == "" {
				apiKey, _ = cfg["apiKey"].(string)
			}
			h.authConfig.APIKey = apiKey
			header, _ := cfg["api_key_header"].(string)
			if header == "" {
				header, _ = cfg["apiKeyHeader"].(string)
			}
			if header == "" {
				header = "X-API-Key"
			}
			h.authConfig.APIKeyHeader = header
			log.Printf("🔐 API Key configured (header: %s)", h.authConfig.APIKeyHeader)
		}
	}

	// Update HTTP service timeout
	h.httpService.SetTimeout(h.timeout)

	authTypeStr := "none"
	if h.authConfig != nil {
		authTypeStr = string(h.authConfig.Type)
	}
	log.Printf("✅ HTTP Outbound initialized: endpoint=%s, method=%s, auth=%s, timeout=%v",
		h.endpoint, h.method, authTypeStr, h.timeout)

	return nil
}

// Send delivers a message to the HTTP endpoint using shared HTTPClientService
func (h *HTTPOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	startTime := time.Now()
	result := &DeliveryResult{
		MessageID:  message.MessageID,
		Timestamp:  startTime,
		RetryCount: h.retryAttempts,
		Success:    false,
		Metadata:   make(map[string]interface{}),
	}

	log.Printf("🚀 [HTTP OUT] Sending message %s to %s", message.MessageID, h.endpoint)

	// Prepare headers
	headers := make(map[string]string)
	for key, value := range message.Headers {
		headers[key] = value
	}

	// Set Content-Type if provided
	if message.ContentType != "" {
		headers["Content-Type"] = message.ContentType
	}

	// Create request configuration
	requestConfig := &httpservice.RequestConfig{
		Method:     h.method,
		URL:        h.endpoint,
		Headers:    headers,
		Body:       message.Content, // Will be JSON marshaled if needed
		Timeout:    h.timeout,
		RetryCount: h.retryAttempts,
		RetryDelay: h.retryDelay,
	}

	// Execute request using shared HTTP service
	resp, err := h.httpService.Execute(ctx, requestConfig, h.authConfig)

	result.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("HTTP request failed: %v", err)
		log.Printf("❌ [HTTP OUT] Message %s delivery failed: %s", message.MessageID, result.ErrorMessage)
		return result, err
	}

	// Store response details in metadata
	result.Metadata["status_code"] = resp.StatusCode
	result.Metadata["response_body"] = string(resp.Body)
	result.Metadata["response_headers"] = resp.Headers

	// Check response status (already validated by HTTPClientService, but double-check)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		result.Acknowledgment = fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Printf("✅ [HTTP OUT] Message %s delivered successfully (status: %d, duration: %dms)",
			message.MessageID, resp.StatusCode, result.DurationMs)
		log.Printf("📥 [HTTP OUT] Response: %s", truncateString(string(resp.Body), 200))
	} else {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(resp.Body))
		log.Printf("❌ [HTTP OUT] Message %s delivery failed with HTTP %d (duration: %dms)",
			message.MessageID, resp.StatusCode, result.DurationMs)
		log.Printf("📥 [HTTP OUT] Error response: %s", truncateString(string(resp.Body), 200))
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
	requestConfig := &httpservice.RequestConfig{
		Method:  "HEAD",
		URL:     h.endpoint,
		Timeout: 5 * time.Second,
	}

	resp, err := h.httpService.Execute(ctx, requestConfig, h.authConfig)
	if err != nil {
		return fmt.Errorf("endpoint not reachable: %w", err)
	}

	if resp.StatusCode >= 500 {
		return fmt.Errorf("endpoint returned server error: %d", resp.StatusCode)
	}

	return nil
}

// GetStatus returns the current connector status
func (h *HTTPOutboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	authTypeStr := "none"
	if h.authConfig != nil {
		authTypeStr = string(h.authConfig.Type)
	}

	status := ConnectorStatus{
		Connected:    h.httpService != nil,
		LastActivity: time.Now(),
		State:        StateReady,
		Metadata: map[string]string{
			"endpoint": h.endpoint,
			"method":   h.method,
			"auth":     authTypeStr,
			"timeout":  h.timeout.String(),
		},
	}

	return status
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
