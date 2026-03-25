// services/connectors/http_fhir_outbound.go
// HTTP FHIR Outbound Connector — FHIR-aware delivery to FHIR REST endpoints.
//
// Reuses services/http.HTTPClientService for HTTP execution and auth (same
// shared layer as http_outbound.go).  FHIR-specific logic (URL routing,
// OperationOutcome parsing, resource-type detection) lives in fhir_utils.go
// so that all FHIR connectors share identical protocol behaviour.
//
// URL routing (via fhir_utils.BuildFHIRURL):
//   Bundle / transaction  → POST  <baseUrl>/$transaction
//   Bundle / batch        → POST  <baseUrl>/$batch
//   Resource with id      → PUT   <baseUrl>/<resourceType>/<id>
//   Resource without id   → POST  <baseUrl>/<resourceType>
//   No resource type      → POST  <baseUrl>              (passthrough)
//
// Config keys (matches V78 DB config_schema):
//   baseUrl         string  required  e.g. "https://fhir.example.com/r4"
//   resourceType    string  optional  e.g. "Patient" — config-level override
//   resourceId      string  optional  e.g. "123"     — config-level override; triggers PUT
//   method          string  optional  POST (default) | PUT | PATCH
//   fhirVersion     string  optional  "R4" (default) | "R5" | "STU3"
//   authType        string  optional  none | basic | bearer | smart | api_key
//   username        string  basic auth
//   password        string  basic auth
//   bearerToken     string  bearer / SMART auth token
//   apiKey          string  api_key auth
//   apiKeyHeader    string  api_key header name (default: X-API-Key)
//   timeout         number  seconds (default 30)
//   retryAttempts   number  (default 3)
//   retryDelay      number  seconds (default 1)

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
	httpservice "ezhealthkonnect/services/http"
)

// ─────────────────────────────────────────────────────────────────────────────
// Connector struct
// ─────────────────────────────────────────────────────────────────────────────

// HTTPFHIROutboundConnector delivers FHIR resources to a FHIR REST endpoint.
// HTTP execution and auth are delegated to the shared HTTPClientService.
// FHIR-specific URL routing and error parsing are delegated to fhir_utils.go.
type HTTPFHIROutboundConnector struct {
	*BaseOutboundConnector
	// Static config (set in Initialize, read-only afterwards)
	baseURL      string
	resourceType string // optional config-level override; "" = auto-detect from body
	resourceID   string // optional config-level override; triggers PUT when set
	operation    string // optional FHIR operation, e.g. "$validate", "$everything"
	queryParams  string // optional raw query string, e.g. "_format=json&_summary=count"
	configMethod string // explicit HTTP verb override, e.g. "PATCH", "GET"
	fhirVersion  string
	authConfig   *httpservice.AuthConfig
	retryAttempts int
	retryDelay    time.Duration
	// Shared HTTP service — same layer as http_outbound.go
	httpService  *httpservice.HTTPClientService
	mu           sync.RWMutex
}

// NewHTTPFHIROutboundConnector creates a new HTTP FHIR outbound connector.
func NewHTTPFHIROutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:    "http_fhir_outbound",
		DisplayName: "HTTP FHIR Sender",
		Version:     "2.0.0",
		Category:    "outbound",
		Mode:        "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_auth":  true,
			"supports_retry": true,
		},
	}
	return &HTTPFHIROutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
		fhirVersion:           "R4",
		retryAttempts:         3,
		retryDelay:            1 * time.Second,
		httpService:           httpservice.NewHTTPClientService(30 * time.Second),
		authConfig:            &httpservice.AuthConfig{Type: httpservice.AuthTypeNone},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Initialize
// ─────────────────────────────────────────────────────────────────────────────

func (h *HTTPFHIROutboundConnector) Initialize(config []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Base URL (required) — also accept "url" or "endpoint" for UI compat
	for _, key := range []string{"baseUrl", "url", "endpoint"} {
		if v, ok := cfg[key].(string); ok && v != "" {
			h.baseURL = strings.TrimRight(v, "/")
			break
		}
	}
	if h.baseURL == "" {
		return fmt.Errorf("baseUrl is required")
	}
	if !strings.HasPrefix(h.baseURL, "http://") && !strings.HasPrefix(h.baseURL, "https://") {
		return fmt.Errorf("baseUrl must use http:// or https:// scheme, got: %s", h.baseURL)
	}

	// Optional resource routing (config-level overrides; runtime auto-detect is the default)
	h.resourceType, _ = cfg["resourceType"].(string)
	h.resourceID, _ = cfg["resourceId"].(string)
	h.operation, _   = cfg["operation"].(string)
	h.queryParams, _  = cfg["queryParams"].(string)

	if v, ok := cfg["method"].(string); ok {
		h.configMethod = strings.ToUpper(v)
	}

	// FHIR version
	if v, ok := cfg["fhirVersion"].(string); ok && v != "" {
		h.fhirVersion = strings.ToUpper(v)
	}

	// Timeout — create a fresh httpService with the configured timeout
	timeout := 30 * time.Second
	if v, ok := cfg["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}
	h.httpService = httpservice.NewHTTPClientService(timeout)

	// Retry
	if v, ok := cfg["retryAttempts"].(float64); ok {
		h.retryAttempts = int(v)
	}
	if v, ok := cfg["retryDelay"].(float64); ok {
		h.retryDelay = time.Duration(v) * time.Second
	}

	// Auth — delegated to fhir_utils which maps to services/http.AuthConfig
	authType, _ := cfg["authType"].(string)
	username, _      := cfg["username"].(string)
	password, _      := cfg["password"].(string)
	bearerToken, _   := cfg["bearerToken"].(string)
	apiKey, _        := cfg["apiKey"].(string)
	apiKeyHeader, _  := cfg["apiKeyHeader"].(string)
	h.authConfig = ToHTTPAuthConfig(authType, username, password, bearerToken, apiKey, apiKeyHeader)

	log.Printf("✅ [FHIR OUT] Initialized: baseUrl=%s resourceType=%q method=%q auth=%s retries=%d",
		h.baseURL, h.resourceType, h.configMethod, authType, h.retryAttempts)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Send
// ─────────────────────────────────────────────────────────────────────────────

func (h *HTTPFHIROutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	startTime := time.Now()
	result := &DeliveryResult{
		MessageID: message.MessageID,
		Timestamp: startTime,
		Success:   false,
		Metadata:  make(map[string]interface{}),
	}

	// Resolve effective URL and method — config overrides take priority,
	// then runtime auto-detect from message body / headers.
	effectiveURL, effectiveMethod := h.resolveURLAndMethod(message)
	log.Printf("🚀 [FHIR OUT] %s %s (%d bytes)", effectiveMethod, effectiveURL, len(message.Content))

	// Build an authenticated HTTP request via the shared service.
	// Auth headers (Basic, Bearer, API Key) are applied by BuildRequest.
	reqCfg := &httpservice.RequestConfig{
		Method: effectiveMethod,
		URL:    effectiveURL,
		Headers: map[string]string{
			"Content-Type": "application/fhir+json",
			"Accept":       "application/fhir+json",
			"User-Agent":   "ezHealthKonnect/2.0 FHIR-Outbound",
		},
		Body: message.Content, // passed as string → BuildRequest sends as-is, no re-marshaling
	}
	httpClient := h.httpService.GetClient()

	// FHIR retry loop — unlike generic HTTP, we SKIP retrying 4xx client errors
	// (except 429 Too Many Requests) because they indicate a permanent problem
	// that won't be fixed by retrying (e.g. 400 Bad Request, 401, 404).
	var lastErr error
	for attempt := 0; attempt <= h.retryAttempts; attempt++ {
		if attempt > 0 {
			log.Printf("  🔄 [FHIR OUT] Retry %d/%d after %v", attempt, h.retryAttempts, h.retryDelay)
			select {
			case <-ctx.Done():
				result.ErrorMessage = "context cancelled during retry"
				return result, ctx.Err()
			case <-time.After(h.retryDelay):
			}
		}

		req, buildErr := h.httpService.BuildRequest(ctx, reqCfg, h.authConfig)
		if buildErr != nil {
			result.ErrorMessage = fmt.Sprintf("build request: %v", buildErr)
			return result, buildErr
		}

		httpResp, doErr := httpClient.Do(req)
		if doErr != nil {
			lastErr = doErr
			log.Printf("  ❌ [FHIR OUT] attempt %d network error: %v", attempt+1, doErr)
			continue
		}

		bodyBytes, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		bodyStr := string(bodyBytes)

		result.DurationMs = time.Since(startTime).Milliseconds()
		result.Metadata["status_code"] = httpResp.StatusCode
		result.Metadata["response_body"] = TruncateFHIR(bodyStr, 500)

		if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
			result.Success = true
			result.Acknowledgment = fmt.Sprintf("HTTP %d", httpResp.StatusCode)
			if loc := httpResp.Header.Get("Location"); loc != "" {
				result.Metadata["location"] = loc
				result.Acknowledgment += " Location: " + loc
			}
			log.Printf("  ✅ [FHIR OUT] Delivered: HTTP %d (%dms)", httpResp.StatusCode, result.DurationMs)
			return result, nil
		}

		// Extract OperationOutcome diagnostics for meaningful error messages
		diagMsg := ExtractOpOutcomeDiag(bodyStr)
		if diagMsg != "" {
			lastErr = fmt.Errorf("FHIR server rejected (HTTP %d): %s", httpResp.StatusCode, diagMsg)
		} else {
			lastErr = fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, TruncateFHIR(bodyStr, 300))
		}
		log.Printf("  ❌ [FHIR OUT] attempt %d: %v", attempt+1, lastErr)

		// Do not retry 4xx client errors (permanent failures) except 429
		if httpResp.StatusCode >= 400 && httpResp.StatusCode < 500 && httpResp.StatusCode != 429 {
			break
		}
	}

	result.DurationMs = time.Since(startTime).Milliseconds()
	result.ErrorMessage = lastErr.Error()
	return result, lastErr
}

func (h *HTTPFHIROutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	results := make([]*DeliveryResult, len(messages))
	for i, msg := range messages {
		r, err := h.Send(ctx, msg)
		if err != nil && r != nil {
			r.ErrorMessage = err.Error()
		}
		results[i] = r
	}
	return results, nil
}

func (h *HTTPFHIROutboundConnector) SupportsBatch() bool { return false }

// ─────────────────────────────────────────────────────────────────────────────
// URL resolution
// ─────────────────────────────────────────────────────────────────────────────

// resolveURLAndMethod builds the final FHIR URL and HTTP verb.
//
// Priority chain:
//  1. Config-level resourceType / resourceId (set by user in connector config)
//  2. Runtime auto-detect from message body (parseResourceTypeAndID)
//  3. X-FHIR-ResourceType / X-FHIR-ResourceId headers set by pipeline steps
//
// URL computation is delegated to fhir_utils.BuildFHIRURL.
func (h *HTTPFHIROutboundConnector) resolveURLAndMethod(message *models.OutboundMessage) (string, string) {
	resType := h.resourceType
	resID   := h.resourceID
	bundleType := ""

	// Auto-detect from message body when config doesn't override
	if resType == "" && message.Content != "" {
		if info := ParseFHIRResource(message.Content); info != nil {
			resType = info.ResourceType
			bundleType = info.BundleType
			if resID == "" {
				resID = info.ID
			}
		}
	}

	// Also honour pipeline-set FHIR headers (set e.g. by http_fhir_inbound unwrap)
	if resType == "" {
		resType, _ = message.Headers["X-FHIR-ResourceType"]
	}
	if resID == "" {
		resID, _ = message.Headers["X-FHIR-ResourceId"]
	}
	if bundleType == "" {
		bundleType, _ = message.Headers["X-Bundle-Type"]
	}

	// operation and queryParams are config-level only (not runtime-detected from body)
	return BuildFHIRURL(h.baseURL, resType, resID, bundleType, h.operation, h.queryParams, h.configMethod)
}

// ─────────────────────────────────────────────────────────────────────────────
// Validate / TestConnection / GetStatus / Close
// ─────────────────────────────────────────────────────────────────────────────

func (h *HTTPFHIROutboundConnector) Validate() error {
	if h.baseURL == "" {
		return fmt.Errorf("baseUrl must be configured")
	}
	if !strings.HasPrefix(h.baseURL, "http://") && !strings.HasPrefix(h.baseURL, "https://") {
		return fmt.Errorf("baseUrl must start with http:// or https://")
	}
	return nil
}

func (h *HTTPFHIROutboundConnector) TestConnection(ctx context.Context) error {
	// Every FHIR-compliant server must serve GET /metadata (CapabilityStatement).
	metadataURL := h.baseURL + "/metadata"
	reqCfg := &httpservice.RequestConfig{
		Method:  "GET",
		URL:     metadataURL,
		Headers: map[string]string{"Accept": "application/fhir+json"},
	}
	req, err := h.httpService.BuildRequest(ctx, reqCfg, h.authConfig)
	if err != nil {
		return fmt.Errorf("build metadata request: %w", err)
	}
	resp, err := h.httpService.GetClient().Do(req)
	if err != nil {
		return fmt.Errorf("FHIR server not reachable at %s: %w", metadataURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("FHIR server error %d from %s", resp.StatusCode, metadataURL)
	}
	// 401/403 is still "reachable" — auth may not be configured yet
	log.Printf("✅ [FHIR OUT] TestConnection OK: %s → HTTP %d", metadataURL, resp.StatusCode)
	return nil
}

func (h *HTTPFHIROutboundConnector) Close() error { return nil }

func (h *HTTPFHIROutboundConnector) GetStatus() ConnectorStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return ConnectorStatus{
		Connected:    h.httpService != nil,
		LastActivity: time.Now(),
		State:        StateReady,
		Metadata: map[string]string{
			"baseUrl":      h.baseURL,
			"resourceType": h.resourceType,
			"operation":    h.operation,
			"queryParams":  h.queryParams,
			"fhirVersion":  h.fhirVersion,
		},
	}
}
