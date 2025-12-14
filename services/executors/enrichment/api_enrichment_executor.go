package enrichment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ===============================================================
// API ENRICHMENT EXECUTOR
// ===============================================================
// Queries external REST APIs to enrich message data
// Implements Strategy Pattern - concrete strategy for API enrichment

type APIEnrichmentExecutor struct {
	*executors.BaseExecutor
	httpClient *http.Client
}

// NewAPIEnrichmentExecutor creates a new API enrichment executor
func NewAPIEnrichmentExecutor() *APIEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "API Enrichment",
		Description: "Enriches messages by querying external REST APIs (EMPI, EHR, LIMS, etc.)",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}

	base := executors.NewBaseExecutor("pre.enrichment.api", metadata)

	return &APIEnrichmentExecutor{
		BaseExecutor: base,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Global timeout, overridden per request
		},
	}
}

// Execute performs API enrichment
func (e *APIEnrichmentExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	log.Printf("🌐 [API Enrichment] Calling API: %s %s", config.Method, config.Endpoint)

	// Build request
	req, err := e.buildRequest(ctx, config, inputData)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Execute API call with retries
	responseData, err := e.executeWithRetry(ctx, req, config)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

		// Use default value if configured
		if config.DefaultValue != nil {
			log.Printf("⚠️  API call failed, using default value: %v", config.DefaultValue)
			responseData = config.DefaultValue
		} else {
			log.Printf("⚠️  API call failed, continuing without enrichment: %v", err)
			e.PostExecute(ctx, step, nil, time.Since(start))
			return inputData, nil
		}
	}

	// Store response in target path
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.api"
	}

	executors.SetNestedValue(inputData, targetPath, responseData)

	log.Printf("✅ [API Enrichment] Response stored at: %s", targetPath)
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// Validate checks if the step configuration is valid
func (e *APIEnrichmentExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses and validates the step configuration
func (e *APIEnrichmentExecutor) parseConfig(step *models.TransformationStep) (*models.APIEnrichmentConfig, error) {
	if step.Config == nil {
		return nil, fmt.Errorf("API enrichment requires configuration")
	}

	// Marshal to JSON then unmarshal to struct for type safety
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.APIEnrichmentConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required for API enrichment")
	}

	// Set defaults
	if config.Method == "" {
		config.Method = "GET"
	}
	if config.TimeoutMs == 0 {
		config.TimeoutMs = 5000
	}
	if config.RetryDelayMs == 0 {
		config.RetryDelayMs = 1000
	}

	return &config, nil
}

// buildRequest constructs the HTTP request from configuration
func (e *APIEnrichmentExecutor) buildRequest(
	ctx context.Context,
	config *models.APIEnrichmentConfig,
	inputData map[string]interface{},
) (*http.Request, error) {

	// Replace field mappings in URL
	url := config.Endpoint
	for key, fieldPath := range config.FieldMappings {
		value := executors.GetNestedValue(inputData, fieldPath)
		if value != nil {
			url = strings.ReplaceAll(url, "{"+key+"}", fmt.Sprintf("%v", value))
		}
	}

	// Create request body if configured
	var bodyReader io.Reader
	if config.Method == "POST" || config.Method == "PUT" || config.Method == "PATCH" {
		if config.RequestBody != nil {
			// Replace field mappings in request body
			body := e.replaceFieldMappings(config.RequestBody, config.FieldMappings, inputData)
			bodyBytes, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, config.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Add query parameters
	if len(config.QueryParams) > 0 {
		q := req.URL.Query()
		for key, value := range config.QueryParams {
			// Replace field mappings in query params
			if fieldPath, exists := config.FieldMappings[key]; exists {
				fieldValue := executors.GetNestedValue(inputData, fieldPath)
				if fieldValue != nil {
					value = fmt.Sprintf("%v", fieldValue)
				}
			}
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Add authentication
	e.addAuthentication(req, config)

	return req, nil
}

// addAuthentication adds authentication headers to the request
func (e *APIEnrichmentExecutor) addAuthentication(req *http.Request, config *models.APIEnrichmentConfig) {
	switch config.AuthType {
	case "basic":
		if config.Username != "" && config.Password != "" {
			auth := config.Username + ":" + config.Password
			encoded := base64.StdEncoding.EncodeToString([]byte(auth))
			req.Header.Set("Authorization", "Basic "+encoded)
			log.Printf("   🔐 Added Basic auth for user: %s", config.Username)
		}
	case "bearer":
		if config.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+config.BearerToken)
			log.Printf("   🔐 Added Bearer token authentication")
		}
	case "apikey":
		if config.APIKey != "" {
			req.Header.Set("X-API-Key", config.APIKey)
			log.Printf("   🔐 Added API key authentication")
		}
	}
}

// replaceFieldMappings replaces field mappings in a map
func (e *APIEnrichmentExecutor) replaceFieldMappings(
	data map[string]interface{},
	mappings map[string]string,
	inputData map[string]interface{},
) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range data {
		if strValue, ok := value.(string); ok {
			// Check if it's a field mapping placeholder
			if fieldPath, exists := mappings[strValue]; exists {
				fieldValue := executors.GetNestedValue(inputData, fieldPath)
				result[key] = fieldValue
			} else {
				result[key] = value
			}
		} else if mapValue, ok := value.(map[string]interface{}); ok {
			result[key] = e.replaceFieldMappings(mapValue, mappings, inputData)
		} else {
			result[key] = value
		}
	}
	return result
}

// executeWithRetry executes the HTTP request with retry logic
func (e *APIEnrichmentExecutor) executeWithRetry(
	ctx context.Context,
	req *http.Request,
	config *models.APIEnrichmentConfig,
) (interface{}, error) {

	// Set timeout for this specific request
	timeout := time.Duration(config.TimeoutMs) * time.Millisecond
	client := &http.Client{Timeout: timeout}

	var lastErr error
	retries := config.RetryCount
	if retries < 0 {
		retries = 0
	}

	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			log.Printf("   🔄 Retry attempt %d/%d", attempt, retries)
			time.Sleep(time.Duration(config.RetryDelayMs) * time.Millisecond)
		}

		// Execute request
		resp, err := client.Do(req)
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

		// Parse JSON response
		var responseData interface{}
		if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
			// If not JSON, return as string
			log.Printf("   ⚠️  Response is not JSON, storing as string")
			return string(bodyBytes), nil
		}

		log.Printf("   ✅ API call successful (status: %d)", resp.StatusCode)
		return responseData, nil
	}

	return nil, lastErr
}

// GetConfigSchema returns the JSON schema for configuration
func (e *APIEnrichmentExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"endpoint"},
		"properties": map[string]interface{}{
			"endpoint": map[string]interface{}{
				"type":        "string",
				"description": "API endpoint URL (supports {fieldName} placeholders)",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, PATCH, DELETE)",
				"default":     "GET",
			},
			"authType": map[string]interface{}{
				"type":        "string",
				"description": "Authentication type (none, basic, bearer, apikey)",
				"enum":        []string{"none", "basic", "bearer", "apikey"},
			},
			"fieldMappings": map[string]interface{}{
				"type":        "object",
				"description": "Map API parameters to message fields (e.g., {\"patientId\": \"PID.3\"})",
			},
			"targetPath": map[string]interface{}{
				"type":        "string",
				"description": "Where to store the API response in the message",
				"default":     "enriched.api",
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Request timeout in milliseconds",
				"default":     5000,
			},
			"retryCount": map[string]interface{}{
				"type":        "integer",
				"description": "Number of retry attempts on failure",
				"default":     0,
			},
			"failOnError": map[string]interface{}{
				"type":        "boolean",
				"description": "Stop pipeline if API call fails",
				"default":     false,
			},
		},
	}
}

// GetConfigExample returns an example configuration
func (e *APIEnrichmentExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"endpoint": "https://api.empi.hospital.org/patients/{patientId}",
		"method":   "GET",
		"authType": "bearer",
		"bearerToken": "your-api-token",
		"headers": map[string]string{
			"Accept": "application/json",
		},
		"fieldMappings": map[string]string{
			"patientId": "PID.3", // Use patient ID from HL7 message
		},
		"targetPath":  "enriched.empi",
		"timeoutMs":   5000,
		"retryCount":  2,
		"retryDelayMs": 1000,
		"failOnError": false,
		"defaultValue": map[string]interface{}{
			"status": "not_found",
		},
	}
}
