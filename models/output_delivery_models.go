package models

import "time"

// DeliveryRequest represents a request to deliver a transformed message to a destination
type DeliveryRequest struct {
	// Message identification
	OutputMessageID string                 `json:"output_message_id"` // UUID of output message in output_intf_* table
	InterfaceID     string                 `json:"interface_id"`      // UUID of interface
	InputMessageID  string                 `json:"input_message_id"`  // Original input message ID
	CorrelationID   string                 `json:"correlation_id"`    // For tracking message lineage

	// Message content
	TransformedData map[string]interface{} `json:"transformed_data"` // FHIR bundle, HL7 message, etc.
	MessageType     string                 `json:"message_type"`     // e.g., "ADT^A01", "Patient", etc.

	// Destination configuration
	TargetConfig DestinationConfig `json:"target_config"` // Where to send

	// Retry management
	RetryAttempt int `json:"retry_attempt"` // Current retry attempt (0 = first attempt)
	MaxRetries   int `json:"max_retries"`   // Maximum retry attempts
}

// LegacyDeliveryResult represents the result of a delivery attempt (legacy model)
// DEPRECATED: Use DeliveryResult from delivery_models.go instead
type LegacyDeliveryResult struct {
	// Overall status
	Success bool   `json:"success"` // Overall success/failure
	Status  string `json:"status"`  // "delivered", "failed", "retrying"

	// HTTP/Protocol response
	StatusCode int    `json:"status_code"` // HTTP status code or equivalent
	Response   string `json:"response"`    // Response body from destination

	// Timing
	DeliveryTimeMs int64 `json:"delivery_time_ms"` // Time taken for delivery in milliseconds

	// Error handling
	ErrorMessage string `json:"error_message,omitempty"` // Error details if failed
	ShouldRetry  bool   `json:"should_retry"`            // Whether to retry on failure

	// Retry scheduling
	NextRetryAt time.Time `json:"next_retry_at,omitempty"` // When to retry next (exponential backoff)
	RetryDelayS int       `json:"retry_delay_s,omitempty"` // Delay in seconds for next retry
}

// OutputMessage represents a transformed message ready for delivery
type OutputMessage struct {
	// Identification
	ID            string `json:"id"`
	InterfaceID   string `json:"interface_id"`
	CorrelationID string `json:"correlation_id"`
	MessageType   string `json:"message_type"`

	// Content
	Content     map[string]interface{} `json:"content"`      // Transformed message (FHIR bundle, etc.)
	ContentType string                 `json:"content_type"` // e.g., "application/fhir+json"
	Encoding    string                 `json:"encoding"`     // e.g., "utf-8"

	// Metadata
	SourceMessageID   string    `json:"source_message_id"`
	TransformationID  string    `json:"transformation_id"`
	TransformedAt     time.Time `json:"transformed_at"`
	TransformationTimeMs int64  `json:"transformation_time_ms"`

	// Delivery tracking
	DeliveryStatus   string    `json:"delivery_status"`    // "pending", "delivered", "failed"
	DeliveryAttempts int       `json:"delivery_attempts"`
	LastDeliveryAt   time.Time `json:"last_delivery_at,omitempty"`
}

// RetryPolicy defines retry behavior for delivery failures
type RetryPolicy struct {
	// Retry configuration
	MaxAttempts       int           `json:"max_attempts"`        // Default: 3
	InitialDelayMs    int           `json:"initial_delay_ms"`    // Default: 1000 (1s)
	MaxDelayMs        int           `json:"max_delay_ms"`        // Default: 60000 (60s)
	BackoffMultiplier float64       `json:"backoff_multiplier"`  // Default: 2.0 (exponential)

	// Retryable conditions
	RetryableStatusCodes []int    `json:"retryable_status_codes"` // e.g., [500, 502, 503, 504]
	RetryableErrors      []string `json:"retryable_errors"`       // e.g., ["timeout", "connection refused"]
}

// DefaultRetryPolicy returns a sensible default retry policy
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       3,
		InitialDelayMs:    1000,  // 1 second
		MaxDelayMs:        60000, // 60 seconds
		BackoffMultiplier: 2.0,   // Exponential: 1s, 2s, 4s, 8s, ...
		RetryableStatusCodes: []int{
			500, // Internal Server Error
			502, // Bad Gateway
			503, // Service Unavailable
			504, // Gateway Timeout
			429, // Too Many Requests
		},
		RetryableErrors: []string{
			"timeout",
			"connection refused",
			"connection reset",
			"network unreachable",
			"dns resolution",
			"i/o timeout",
		},
	}
}

// CalculateRetryDelay calculates the delay for the next retry using exponential backoff
func (rp *RetryPolicy) CalculateRetryDelay(attemptNumber int) int {
	if attemptNumber <= 0 {
		return rp.InitialDelayMs
	}

	// Calculate exponential backoff: initialDelay * (multiplier ^ attemptNumber)
	delayMs := float64(rp.InitialDelayMs)
	for i := 0; i < attemptNumber; i++ {
		delayMs *= rp.BackoffMultiplier
	}

	// Cap at max delay
	if int(delayMs) > rp.MaxDelayMs {
		return rp.MaxDelayMs
	}

	return int(delayMs)
}

// IsRetryableStatusCode checks if a status code is retryable
func (rp *RetryPolicy) IsRetryableStatusCode(statusCode int) bool {
	for _, code := range rp.RetryableStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// IsRetryableError checks if an error message indicates a retryable failure
func (rp *RetryPolicy) IsRetryableError(errorMsg string) bool {
	errorMsgLower := toLower(errorMsg)
	for _, retryableErr := range rp.RetryableErrors {
		if contains(errorMsgLower, toLower(retryableErr)) {
			return true
		}
	}
	return false
}

// Helper functions
func toLower(s string) string {
	// Simple lowercase conversion
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// ConnectorMetadata provides information about a destination connector
type ConnectorMetadata struct {
	Name        string   `json:"name"`        // e.g., "FHIR R4 HTTP Connector"
	Type        string   `json:"type"`        // e.g., "fhir", "hl7", "database"
	Version     string   `json:"version"`     // e.g., "1.0.0"
	Protocol    string   `json:"protocol"`    // e.g., "http", "tcp", "jdbc"
	Description string   `json:"description"` // Human-readable description
	Capabilities []string `json:"capabilities"` // e.g., ["retry", "authentication", "tls"]
}
