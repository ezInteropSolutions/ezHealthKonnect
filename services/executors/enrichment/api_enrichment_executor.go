package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	httpservice "ezhealthkonnect/services/http"
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

	if err != nil {
		if config.FailOnError {
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

		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Store response in target path
	targetPath := e.getTargetPath(config)
	executors.SetNestedValue(inputData, targetPath, resp.JSON)

	// Count enriched fields if response is a map
	enrichedFields := 0
	if jsonMap, ok := resp.JSON.(map[string]interface{}); ok {
		enrichedFields = e.countFields(jsonMap)
	}

	log.Printf("✅ [API Enrichment] Response stored at: %s (%d fields, %dms)",
		targetPath, enrichedFields, apiDuration)
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// countFields recursively counts the number of fields in a map
func (e *APIEnrichmentExecutor) countFields(data map[string]interface{}) int {
	count := 0
	for _, value := range data {
		count++
		if nested, ok := value.(map[string]interface{}); ok {
			count += e.countFields(nested)
		}
	}
	return count
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
