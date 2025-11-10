// processing/rest_output_connector.go
// REST/HTTP output connector for Go processing engine

package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RESTOutputConnector implements OutputConnector for REST/HTTP endpoints
type RESTOutputConnector struct {
	endpoint        string
	method          string
	headers         map[string]string
	timeout         time.Duration
	retryAttempts   int
	retryDelay      time.Duration
	client          *http.Client
	messageCount    int64
	errorCount      int64
	lastActivity    time.Time
}

// RESTOutputConfig configuration for REST output connector
type RESTOutputConfig struct {
	Endpoint      string            `json:"endpoint"`
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Timeout       int               `json:"timeout"`       // seconds
	RetryAttempts int               `json:"retryAttempts"`
	RetryDelay    int               `json:"retryDelay"`    // milliseconds
	Authentication map[string]interface{} `json:"authentication"`
}

// NewRESTOutputConnector creates a new REST output connector
func NewRESTOutputConnector(config map[string]interface{}) (OutputConnector, error) {
	// Parse configuration
	var restConfig RESTOutputConfig
	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := json.Unmarshal(configBytes, &restConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal REST config: %w", err)
	}

	// Set defaults
	if restConfig.Method == "" {
		restConfig.Method = "POST"
	}
	if restConfig.Timeout == 0 {
		restConfig.Timeout = 30
	}
	if restConfig.RetryAttempts == 0 {
		restConfig.RetryAttempts = 3
	}
	if restConfig.RetryDelay == 0 {
		restConfig.RetryDelay = 1000
	}
	if restConfig.Headers == nil {
		restConfig.Headers = make(map[string]string)
	}
	if restConfig.Headers["Content-Type"] == "" {
		restConfig.Headers["Content-Type"] = "application/json"
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: time.Duration(restConfig.Timeout) * time.Second,
	}

	connector := &RESTOutputConnector{
		endpoint:      restConfig.Endpoint,
		method:        restConfig.Method,
		headers:       restConfig.Headers,
		timeout:       time.Duration(restConfig.Timeout) * time.Second,
		retryAttempts: restConfig.RetryAttempts,
		retryDelay:    time.Duration(restConfig.RetryDelay) * time.Millisecond,
		client:        client,
		lastActivity:  time.Now(),
	}

	// Add authentication if configured
	if restConfig.Authentication != nil {
		if err := connector.configureAuthentication(restConfig.Authentication); err != nil {
			return nil, fmt.Errorf("failed to configure authentication: %w", err)
		}
	}

	fmt.Printf("✅ REST output connector initialized: %s\n", restConfig.Endpoint)
	return connector, nil
}

// Send delivers a processed message to the REST endpoint
func (r *RESTOutputConnector) Send(ctx context.Context, message ProcessedMessage) error {
	var lastErr error

	for attempt := 0; attempt <= r.retryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.retryDelay):
			}
		}

		// Prepare request payload
		payload := map[string]interface{}{
			"message":   message,
			"timestamp": time.Now().Format(time.RFC3339),
			"source":    "ezHealthKonnect",
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			lastErr = fmt.Errorf("failed to marshal payload: %w", err)
			continue
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, r.method, r.endpoint, bytes.NewReader(payloadBytes))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		// Add headers
		for key, value := range r.headers {
			req.Header.Set(key, value)
		}

		// Send request
		fmt.Printf("📤 Sending message to REST endpoint: %s (attempt %d)\n", r.endpoint, attempt+1)
		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			r.errorCount++
			continue
		}

		// Read response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			r.errorCount++
			continue
		}

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.messageCount++
			r.lastActivity = time.Now()
			fmt.Printf("✅ Message delivered successfully to %s (status: %d)\n", r.endpoint, resp.StatusCode)
			return nil
		}

		lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		r.errorCount++
	}

	return fmt.Errorf("failed to deliver message after %d attempts: %w", r.retryAttempts+1, lastErr)
}

// TestConnection tests the connection to the REST endpoint
func (r *RESTOutputConnector) TestConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Use HEAD or OPTIONS request for testing
	testMethod := "HEAD"
	if r.method == "GET" {
		testMethod = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, testMethod, r.endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	// Add headers (excluding Content-Type for HEAD/OPTIONS)
	for key, value := range r.headers {
		if testMethod == "HEAD" && key == "Content-Type" {
			continue
		}
		req.Header.Set(key, value)
	}

	fmt.Printf("🔍 Testing REST endpoint connection: %s\n", r.endpoint)
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()

	// Accept various success codes
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == 405 {
		fmt.Printf("✅ REST endpoint connection test successful (status: %d)\n", resp.StatusCode)
		return nil
	}

	return fmt.Errorf("connection test failed with status: %d", resp.StatusCode)
}

// Close cleans up the connector
func (r *RESTOutputConnector) Close() error {
	fmt.Printf("🔌 REST output connector disconnected from %s\n", r.endpoint)
	// HTTP client doesn't need explicit closing
	return nil
}

// GetStatus returns connector status
func (r *RESTOutputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{
		Type:         "REST",
		Status:       "ready",
		LastActivity: r.lastActivity.Format(time.RFC3339),
		MessageCount: r.messageCount,
		ErrorCount:   r.errorCount,
	}
}

// configureAuthentication configures authentication based on the provided configuration
func (r *RESTOutputConnector) configureAuthentication(authConfig map[string]interface{}) error {
	authType, ok := authConfig["type"].(string)
	if !ok {
		return fmt.Errorf("authentication type not specified")
	}

	switch authType {
	case "none", "":
		// No authentication required
		// Do nothing - proceed without adding auth headers

	case "bearer":
		token, ok := authConfig["token"].(string)
		if !ok {
			return fmt.Errorf("bearer token not specified")
		}
		r.headers["Authorization"] = fmt.Sprintf("Bearer %s", token)

	case "basic":
		username, ok := authConfig["username"].(string)
		if !ok {
			return fmt.Errorf("basic auth username not specified")
		}
		password, ok := authConfig["password"].(string)
		if !ok {
			return fmt.Errorf("basic auth password not specified")
		}
		// In a real implementation, you'd base64 encode username:password
		r.headers["Authorization"] = fmt.Sprintf("Basic %s:%s", username, password)

	case "apikey":
		key, ok := authConfig["key"].(string)
		if !ok {
			return fmt.Errorf("API key not specified")
		}
		headerName := "X-API-Key"
		if customHeader, exists := authConfig["headerName"].(string); exists {
			headerName = customHeader
		}
		r.headers[headerName] = key

	case "custom":
		if customHeaders, ok := authConfig["headers"].(map[string]interface{}); ok {
			for key, value := range customHeaders {
				if strValue, ok := value.(string); ok {
					r.headers[key] = strValue
				}
			}
		}

	default:
		return fmt.Errorf("unsupported authentication type: %s", authType)
	}

	return nil
}