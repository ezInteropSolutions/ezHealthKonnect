package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	httpservice "ezhealthkonnect/services/http"
	"ezhealthkonnect/services/response"
	"fmt"
	"log"
	"strings"
	"time"
)

// ===============================================================
// API ENRICHMENT EXECUTOR
// ===============================================================
// Queries external REST APIs to enrich message data
// Implements Strategy Pattern - concrete strategy for API enrichment
// Uses shared HTTPClientService for OOP code reuse

type APIEnrichmentExecutor struct {
	*executors.BaseExecutor
	httpService *httpservice.HTTPClientService
}

// NewAPIEnrichmentExecutor creates a new API enrichment executor
func NewAPIEnrichmentExecutor() *APIEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "API Enrichment",
		Description: "Enriches messages by querying external REST APIs (EMPI, EHR, LIMS, etc.)",
		Version:     "2.0.0", // Upgraded to use shared HTTPClientService
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}

	base := executors.NewBaseExecutor("pre.enrichment.api", metadata)

	return &APIEnrichmentExecutor{
		BaseExecutor: base,
		httpService:  httpservice.NewHTTPClientService(30 * time.Second),
	}
}

// Execute performs API enrichment using shared HTTPClientService
// This method maintains backward compatibility but doesn't track step outputs
// For full step output tracking, use ExecuteWithContext from pipeline service
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

	// Build URL with field mappings
	url := e.buildURL(config.Endpoint, config.FieldMappings, inputData)

	// Build request body with field mappings
	var requestBody interface{}
	if config.RequestBody != nil {
		requestBody = e.replaceFieldMappings(config.RequestBody, config.FieldMappings, inputData)
	}

	// Build query params with field mappings
	queryParams := e.buildQueryParams(config.QueryParams, config.FieldMappings, inputData)

	// Create request configuration
	requestConfig := &httpservice.RequestConfig{
		Method:      config.Method,
		URL:         url,
		Headers:     config.Headers,
		QueryParams: queryParams,
		Body:        requestBody,
		Timeout:     time.Duration(config.TimeoutMs) * time.Millisecond,
		RetryCount:  config.RetryCount,
		RetryDelay:  time.Duration(config.RetryDelayMs) * time.Millisecond,
	}

	// Create authentication configuration
	authConfig := e.buildAuthConfig(config)

	// Execute API call using shared service
	apiStart := time.Now()
	resp, err := e.httpService.Execute(ctx, requestConfig, authConfig)
	apiDuration := time.Since(apiStart).Milliseconds()

	// Store comprehensive request/response details for step output tracking
	requestDetails := e.buildRequestDetails(requestConfig, authConfig, apiStart)
	var responseDetails map[string]interface{}

	if err != nil {
		// Store error response details
		responseDetails = map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": apiDuration,
		}

		if config.FailOnError {
			// Store failed API call details in output data for visibility
			inputData["_api_request"] = requestDetails
			inputData["_api_response"] = responseDetails
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

		// Use default value if configured
		if config.DefaultValue != nil {
			log.Printf("⚠️  API call failed, using default value: %v", config.DefaultValue)
			executors.SetNestedValue(inputData, e.getTargetPath(config), config.DefaultValue)
		} else {
			log.Printf("⚠️  API call failed, continuing without enrichment: %v", err)
		}

		// Still store request/response details for debugging
		inputData["_api_request"] = requestDetails
		inputData["_api_response"] = responseDetails
		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Check if response mapping is configured
	var enrichedFields int
	targetPath := e.getTargetPath(config)

	if responseMappingRaw, hasMappingConfig := step.Config["responseMapping"]; hasMappingConfig {
		// Apply response mapping to extract specific fields
		log.Printf("📋 [API Enrichment] Applying response mapping configuration")
		mappedData, mappingErr := e.applyResponseMapping(resp.JSON, responseMappingRaw)

		if mappingErr != nil {
			log.Printf("⚠️  Response mapping failed: %v, storing full response instead", mappingErr)
			// Fallback: Store full response at target path
			executors.SetNestedValue(inputData, targetPath, resp.JSON)
			enrichedFields = e.countFields(resp.JSON)
		} else {
			// Store mapped fields directly in message (not nested under targetPath)
			for field, value := range mappedData {
				executors.SetNestedValue(inputData, field, value)
			}
			enrichedFields = len(mappedData)
			log.Printf("✅ [API Enrichment] Extracted %d fields using response mapping", enrichedFields)
		}
	} else {
		// No response mapping - store full response at target path (existing behavior)
		executors.SetNestedValue(inputData, targetPath, resp.JSON)
		enrichedFields = e.countFields(resp.JSON)
		log.Printf("✅ [API Enrichment] Stored full response at: %s", targetPath)
	}

	// Build response details with full information
	responseDetails = e.buildResponseDetails(resp, apiDuration, enrichedFields)

	// Store request/response details for step output tracking
	inputData["_api_request"] = requestDetails
	inputData["_api_response"] = responseDetails
	inputData["_api_enriched_path"] = targetPath

	log.Printf("✅ [API Enrichment] Response stored at: %s (%d fields, %dms)",
		targetPath, enrichedFields, apiDuration)
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

// buildURL replaces field mappings in URL template
func (e *APIEnrichmentExecutor) buildURL(
	urlTemplate string,
	fieldMappings map[string]string,
	inputData map[string]interface{},
) string {
	url := urlTemplate
	for key, fieldPath := range fieldMappings {
		value := executors.GetNestedValue(inputData, fieldPath)
		if value != nil {
			url = strings.ReplaceAll(url, "{"+key+"}", fmt.Sprintf("%v", value))
		}
	}
	return url
}

// buildQueryParams creates query parameters with field mapping replacements
func (e *APIEnrichmentExecutor) buildQueryParams(
	queryParams map[string]string,
	fieldMappings map[string]string,
	inputData map[string]interface{},
) map[string]string {
	if len(queryParams) == 0 {
		return nil
	}

	result := make(map[string]string)
	for key, value := range queryParams {
		// Check if this query param should be replaced with a field value
		if fieldPath, exists := fieldMappings[key]; exists {
			fieldValue := executors.GetNestedValue(inputData, fieldPath)
			if fieldValue != nil {
				result[key] = fmt.Sprintf("%v", fieldValue)
				continue
			}
		}
		result[key] = value
	}
	return result
}

// buildAuthConfig creates authentication configuration from API enrichment config
func (e *APIEnrichmentExecutor) buildAuthConfig(config *models.APIEnrichmentConfig) *httpservice.AuthConfig {
	if config.AuthType == "" || config.AuthType == "none" {
		return nil
	}

	authConfig := &httpservice.AuthConfig{}

	switch config.AuthType {
	case "basic":
		authConfig.Type = httpservice.AuthTypeBasic
		authConfig.Username = config.Username
		authConfig.Password = config.Password

	case "bearer":
		authConfig.Type = httpservice.AuthTypeBearer
		authConfig.BearerToken = config.BearerToken

	case "apikey":
		authConfig.Type = httpservice.AuthTypeAPIKey
		authConfig.APIKey = config.APIKey

	case "oauth2":
		// OAuth 2.0 - full OAuth configuration with automatic token management
		authConfig.Type = httpservice.AuthTypeOAuth2
		authConfig.OAuth2Config = &httpservice.OAuth2Config{
			TokenURL:     config.OAuth2TokenURL,
			ClientID:     config.OAuth2ClientID,
			ClientSecret: config.OAuth2ClientSecret,
			GrantType:    httpservice.OAuth2GrantType(config.OAuth2GrantType),
			Scope:        config.OAuth2Scope,
			Username:     config.OAuth2Username, // For password grant
			Password:     config.OAuth2Password, // For password grant
		}
	}

	return authConfig
}

// getTargetPath returns the target path for storing API response
func (e *APIEnrichmentExecutor) getTargetPath(config *models.APIEnrichmentConfig) string {
	if config.TargetPath != "" {
		return config.TargetPath
	}
	return "enriched.api"
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

// buildRequestDetails creates comprehensive request details for step output
func (e *APIEnrichmentExecutor) buildRequestDetails(
	requestConfig *httpservice.RequestConfig,
	authConfig *httpservice.AuthConfig,
	sentAt time.Time,
) map[string]interface{} {
	// Mask sensitive headers
	maskedHeaders := e.maskSensitiveHeaders(requestConfig.Headers, authConfig)

	requestDetails := map[string]interface{}{
		"method":   requestConfig.Method,
		"url":      requestConfig.URL,
		"headers":  maskedHeaders,
		"sent_at":  sentAt.Format(time.RFC3339Nano),
	}

	// Add query params if present
	if len(requestConfig.QueryParams) > 0 {
		requestDetails["query_params"] = requestConfig.QueryParams
	}

	// Add request body if present
	if requestConfig.Body != nil {
		requestDetails["body"] = requestConfig.Body
	}

	// Add timeout info
	if requestConfig.Timeout > 0 {
		requestDetails["timeout_ms"] = int(requestConfig.Timeout.Milliseconds())
	}

	return requestDetails
}

// buildResponseDetails creates comprehensive response details for step output
func (e *APIEnrichmentExecutor) buildResponseDetails(
	resp *httpservice.Response,
	durationMs int64,
	enrichedFields int,
) map[string]interface{} {
	responseDetails := map[string]interface{}{
		"status_code":     resp.StatusCode,
		"status_text":     e.getStatusText(resp.StatusCode),
		"duration_ms":     durationMs,
		"enriched_fields": enrichedFields,
		"received_at":     time.Now().Format(time.RFC3339Nano),
	}

	// Add response headers (convert http.Header to map)
	if len(resp.Headers) > 0 {
		headers := make(map[string]string)
		for key, values := range resp.Headers {
			if len(values) > 0 {
				headers[key] = values[0] // Use first value
			}
		}
		responseDetails["headers"] = headers
	}

	// Add content type
	contentType := resp.Headers.Get("Content-Type")
	if contentType != "" {
		responseDetails["content_type"] = contentType
	}

	// Add body - both raw and parsed
	if len(resp.Body) > 0 {
		responseDetails["body_raw"] = string(resp.Body)
		responseDetails["body_size"] = len(resp.Body)
	}

	// Add parsed JSON if available
	if resp.JSON != nil {
		responseDetails["body_parsed"] = resp.JSON

		// Add field structure analysis for UI field picker
		responseDetails["field_structure"] = e.analyzeFieldStructure(resp.JSON, "")
	}

	return responseDetails
}

// maskSensitiveHeaders masks sensitive header values for security
func (e *APIEnrichmentExecutor) maskSensitiveHeaders(
	headers map[string]string,
	authConfig *httpservice.AuthConfig,
) map[string]string {
	masked := make(map[string]string)
	sensitiveKeys := map[string]bool{
		"authorization": true,
		"api-key":       true,
		"x-api-key":     true,
		"apikey":        true,
		"token":         true,
		"bearer":        true,
		"password":      true,
		"secret":        true,
	}

	for key, value := range headers {
		lowerKey := strings.ToLower(key)
		if sensitiveKeys[lowerKey] {
			masked[key] = "***MASKED***"
		} else {
			masked[key] = value
		}
	}

	// Add auth type if present (without credentials)
	if authConfig != nil {
		masked["X-Auth-Type"] = string(authConfig.Type)
	}

	return masked
}

// getStatusText returns human-readable HTTP status text
func (e *APIEnrichmentExecutor) getStatusText(statusCode int) string {
	statusTexts := map[int]string{
		200: "OK",
		201: "Created",
		204: "No Content",
		400: "Bad Request",
		401: "Unauthorized",
		403: "Forbidden",
		404: "Not Found",
		500: "Internal Server Error",
		502: "Bad Gateway",
		503: "Service Unavailable",
		504: "Gateway Timeout",
	}

	if text, ok := statusTexts[statusCode]; ok {
		return text
	}

	if statusCode >= 200 && statusCode < 300 {
		return "Success"
	} else if statusCode >= 400 && statusCode < 500 {
		return "Client Error"
	} else if statusCode >= 500 {
		return "Server Error"
	}

	return "Unknown"
}

// ================================================================
// RESPONSE MAPPING SUPPORT
// ================================================================

// applyResponseMapping applies response mapping configuration to extract specific fields
func (e *APIEnrichmentExecutor) applyResponseMapping(apiResponse interface{}, mappingConfigRaw interface{}) (map[string]interface{}, error) {
	// Import response extractor service
	extractorService := response.NewResponseExtractorService()

	// Parse step config to extract mapping rules
	configJSON, err := json.Marshal(mappingConfigRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response mapping config: %w", err)
	}

	var responseMappingConfig models.ResponseMappingConfig
	if err := json.Unmarshal(configJSON, &responseMappingConfig); err != nil {
		return nil, fmt.Errorf("failed to parse response mapping config: %w", err)
	}

	// Load mapping rules based on mode
	var mappingRules []models.ResponseMappingRule

	switch responseMappingConfig.Mode {
	case models.MappingModeTemplate:
		// Load template (requires DB access - will be handled by pipeline service)
		// For now, return error indicating DB dependency
		return nil, fmt.Errorf("template mode requires database access - use pipeline service LoadMappingRulesForStep")

	case models.MappingModeCustom:
		// Use custom extractors directly
		mappingRules = responseMappingConfig.Extractors

	case models.MappingModeExtend:
		// Load template + add custom extractors (requires DB)
		return nil, fmt.Errorf("extend mode requires database access - use pipeline service LoadMappingRulesForStep")

	case models.MappingModeOverride:
		// Load template with overrides (requires DB)
		return nil, fmt.Errorf("override mode requires database access - use pipeline service LoadMappingRulesForStep")

	default:
		return nil, fmt.Errorf("unknown mapping mode: %s", responseMappingConfig.Mode)
	}

	if len(mappingRules) == 0 {
		return nil, fmt.Errorf("no mapping rules configured")
	}

	// Apply mapping rules to extract fields
	log.Printf("📋 Applying %d response mapping rules", len(mappingRules))
	return extractorService.ApplyMappingRules(apiResponse, mappingRules)
}

// countFields recursively counts fields in any data structure
func (e *APIEnrichmentExecutor) countFields(data interface{}) int {
	switch v := data.(type) {
	case map[string]interface{}:
		count := 0
		for _, value := range v {
			count++
			if nested, ok := value.(map[string]interface{}); ok {
				count += e.countFields(nested)
			}
		}
		return count
	case []interface{}:
		count := 0
		for _, item := range v {
			count += e.countFields(item)
		}
		return count
	default:
		return 1
	}
}

// ================================================================
// FIELD STRUCTURE ANALYSIS (For UI Field Picker)
// ================================================================

// analyzeFieldStructure creates a flattened list of all available JSONPath expressions
// This enables the UI to show a visual field picker for response mapping configuration
func (e *APIEnrichmentExecutor) analyzeFieldStructure(data interface{}, prefix string) []map[string]interface{} {
	fields := []map[string]interface{}{}

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			path := prefix + "." + key
			if prefix == "" {
				path = "$" + path
			}

			fieldType := e.getJSONType(value)
			fields = append(fields, map[string]interface{}{
				"path":   path,
				"key":    key,
				"type":   fieldType,
				"sample": e.getSampleValue(value),
			})

			// Recursively analyze nested objects (but not arrays to keep list manageable)
			if nested, ok := value.(map[string]interface{}); ok {
				if nestedFields := e.analyzeFieldStructure(nested, path); len(nestedFields) > 0 {
					fields = append(fields, nestedFields...)
				}
			}
		}

	case []interface{}:
		if len(v) > 0 {
			// Analyze first array element as example
			path := prefix + "[0]"
			fields = append(fields, map[string]interface{}{
				"path":   prefix, // Use array path without [0] for filtering
				"key":    "[array]",
				"type":   "array",
				"sample": fmt.Sprintf("Array with %d items", len(v)),
			})

			// Analyze first element structure
			if nestedFields := e.analyzeFieldStructure(v[0], path); len(nestedFields) > 0 {
				fields = append(fields, nestedFields...)
			}
		}
	}

	return fields
}

// getJSONType returns the JSON type of a value
func (e *APIEnrichmentExecutor) getJSONType(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case float64, int, int64, int32:
		return "number"
	case bool:
		return "boolean"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

// getSampleValue returns a preview of the value for UI display
func (e *APIEnrichmentExecutor) getSampleValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return "{...}"
	case []interface{}:
		return fmt.Sprintf("[%d items]", len(v))
	case string:
		if len(v) > 50 {
			return v[:50] + "..."
		}
		return v
	default:
		return value
	}
}
