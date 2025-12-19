package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ===============================================================
// HTTP CLIENT SERVICE
// ===============================================================
// Shared HTTP client with authentication, retry logic, and
// response parsing. Used by API enrichment executor and HTTP
// outbound connector to eliminate code duplication.

// AuthType defines supported authentication methods
type AuthType string

const (
	AuthTypeNone   AuthType = "none"
	AuthTypeBasic  AuthType = "basic"
	AuthTypeBearer AuthType = "bearer"
	AuthTypeAPIKey AuthType = "apikey"
)

// AuthConfig contains authentication configuration
type AuthConfig struct {
	Type        AuthType
	Username    string // For Basic auth
	Password    string // For Basic auth
	BearerToken string // For Bearer auth
	APIKey      string // For API Key auth
	APIKeyHeader string // Custom header name for API Key (default: X-API-Key)
}

// RequestConfig contains HTTP request configuration
type RequestConfig struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]string
	Body        interface{} // Will be JSON marshaled if not nil
	Timeout     time.Duration
	RetryCount  int
	RetryDelay  time.Duration
}

// Response contains HTTP response data
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	JSON       interface{} // Parsed JSON response if applicable
}

// HTTPClientService provides shared HTTP client functionality
type HTTPClientService struct {
	client *http.Client
	mu     sync.RWMutex
}

// NewHTTPClientService creates a new HTTP client service
func NewHTTPClientService(timeout time.Duration) *HTTPClientService {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &HTTPClientService{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// BuildRequest constructs an HTTP request from configuration
func (h *HTTPClientService) BuildRequest(
	ctx context.Context,
	config *RequestConfig,
	auth *AuthConfig,
) (*http.Request, error) {
	// Create request body if configured
	var bodyReader io.Reader
	var contentType string

	if config.Body != nil {
		// Check if Content-Type is already set in headers
		for key, value := range config.Headers {
			if strings.ToLower(key) == "content-type" {
				contentType = value
				break
			}
		}

		// Determine how to encode body based on Content-Type
		if contentType == "application/x-www-form-urlencoded" {
			// Form-encoded data (for OAuth 2.0)
			if formData, ok := config.Body.(map[string]string); ok {
				formValues := url.Values{}
				for key, value := range formData {
					formValues.Set(key, value)
				}
				bodyReader = strings.NewReader(formValues.Encode())
			} else {
				return nil, fmt.Errorf("form-encoded body must be map[string]string")
			}
		} else {
			// Default: JSON encoding
			bodyBytes, err := json.Marshal(config.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(bodyBytes)
			if contentType == "" {
				contentType = "application/json"
			}
		}
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Set content type if not already set
	if config.Body != nil && contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add query parameters
	if len(config.QueryParams) > 0 {
		q := req.URL.Query()
		for key, value := range config.QueryParams {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Add authentication
	if auth != nil {
		h.AddAuthentication(req, auth)
	}

	return req, nil
}

// AddAuthentication adds authentication headers to the request
func (h *HTTPClientService) AddAuthentication(req *http.Request, auth *AuthConfig) {
	switch auth.Type {
	case AuthTypeBasic:
		if auth.Username != "" && auth.Password != "" {
			credentials := auth.Username + ":" + auth.Password
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
			req.Header.Set("Authorization", "Basic "+encoded)
			log.Printf("   🔐 Added Basic auth for user: %s", auth.Username)
		}

	case AuthTypeBearer:
		if auth.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
			log.Printf("   🔐 Added Bearer token authentication")
		}

	case AuthTypeAPIKey:
		if auth.APIKey != "" {
			headerName := auth.APIKeyHeader
			if headerName == "" {
				headerName = "X-API-Key"
			}
			req.Header.Set(headerName, auth.APIKey)
			log.Printf("   🔐 Added API key authentication (header: %s)", headerName)
		}
	}
}

// ExecuteWithRetry executes the HTTP request with retry logic
func (h *HTTPClientService) ExecuteWithRetry(
	ctx context.Context,
	req *http.Request,
	retryCount int,
	retryDelay time.Duration,
) (*Response, error) {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryDelay == 0 {
		retryDelay = 1 * time.Second
	}

	var lastErr error

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			log.Printf("   🔄 Retry attempt %d/%d", attempt, retryCount)

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		// Execute request
		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			continue
		}

		defer resp.Body.Close()

		// Read response body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
			continue
		}

		// Try to parse JSON response
		var jsonData interface{}
		if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
			// Not JSON - return as string
			log.Printf("   ⚠️  Response is not JSON, returning raw body")
			jsonData = string(bodyBytes)
		}

		log.Printf("   ✅ HTTP request successful (status: %d)", resp.StatusCode)

		return &Response{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       bodyBytes,
			JSON:       jsonData,
		}, nil
	}

	return nil, lastErr
}

// Execute performs a complete HTTP request with authentication and retry
func (h *HTTPClientService) Execute(
	ctx context.Context,
	config *RequestConfig,
	auth *AuthConfig,
) (*Response, error) {
	// Build request
	req, err := h.BuildRequest(ctx, config, auth)
	if err != nil {
		return nil, err
	}

	// Set timeout for this specific request
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()

		// Update request context
		req = req.WithContext(ctx)
	}

	// Execute with retry
	return h.ExecuteWithRetry(ctx, req, config.RetryCount, config.RetryDelay)
}

// SetTimeout updates the global client timeout
func (h *HTTPClientService) SetTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.client.Timeout = timeout
}

// GetClient returns the underlying HTTP client (for advanced usage)
func (h *HTTPClientService) GetClient() *http.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.client
}
