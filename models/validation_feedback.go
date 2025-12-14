package models

import "time"

// ValidationFeedback represents the result of validation that needs to be sent back to the connector
type ValidationFeedback struct {
	MessageID       string                 `json:"message_id"`
	InterfaceID     string                 `json:"interface_id"`
	ValidationMode  ValidationACKMode      `json:"validation_mode"`
	Status          string                 `json:"status"` // "passed", "rejected", "warning"
	Errors          []FieldValidationError `json:"errors,omitempty"`
	ProcessingTime  int64                  `json:"processing_time_ms"`
	ConnectorType   string                 `json:"connector_type"` // "tcp_mllp_inbound", "http_rest_inbound", etc.
	Timestamp       time.Time              `json:"timestamp"`
}

// NewValidationFeedback creates a new validation feedback instance
func NewValidationFeedback(messageID, interfaceID string, mode ValidationACKMode, status string, errors []FieldValidationError, processingTime int64, connectorType string) *ValidationFeedback {
	return &ValidationFeedback{
		MessageID:      messageID,
		InterfaceID:    interfaceID,
		ValidationMode: mode,
		Status:         status,
		Errors:         errors,
		ProcessingTime: processingTime,
		ConnectorType:  connectorType,
		Timestamp:      time.Now(),
	}
}

// GetErrorMessage returns a formatted error message from validation errors
func (vf *ValidationFeedback) GetErrorMessage() string {
	if len(vf.Errors) == 0 {
		return ""
	}

	var messages []string
	for _, err := range vf.Errors {
		messages = append(messages, err.Message)
	}

	if len(messages) == 1 {
		return messages[0]
	}

	// Join multiple errors
	result := ""
	for i, msg := range messages {
		if i > 0 {
			result += "; "
		}
		result += msg
	}
	return result
}

// IsRejected returns true if validation was rejected
func (vf *ValidationFeedback) IsRejected() bool {
	return vf.Status == "rejected"
}

// HasWarnings returns true if validation passed with warnings
func (vf *ValidationFeedback) HasWarnings() bool {
	return vf.Status == "warning"
}

// IsPassed returns true if validation passed without issues
func (vf *ValidationFeedback) IsPassed() bool {
	return vf.Status == "passed"
}
