package models

import (
	"encoding/json"
)

// ValidationRule represents a single validation rule
type ValidationRule struct {
	Field        string                 `json:"field"`        // Field path (e.g., "enhancedSegments.PID.fields[4].subfields[1].value")
	Type         string                 `json:"type"`         // Validation type: required, format, length, pattern
	ErrorMessage string                 `json:"errorMessage"` // Custom error message
	Options      map[string]interface{} `json:"-"` // Type-specific options (populated from extra fields)
}

// UnmarshalJSON custom unmarshaling to capture extra fields as options
func (vr *ValidationRule) UnmarshalJSON(data []byte) error {
	// First unmarshal into a generic map
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract known fields
	if field, ok := raw["field"].(string); ok {
		vr.Field = field
	}
	if typ, ok := raw["type"].(string); ok {
		vr.Type = typ
	}
	if errMsg, ok := raw["errorMessage"].(string); ok {
		vr.ErrorMessage = errMsg
	}

	// All other fields go into Options
	vr.Options = make(map[string]interface{})
	for key, value := range raw {
		if key != "field" && key != "type" && key != "errorMessage" {
			vr.Options[key] = value
		}
	}

	return nil
}

// ValidationACKMode defines how the system responds to validation failures
type ValidationACKMode string

const (
	// ValidationModeStrictReject sends NACK on validation failure, stops processing
	ValidationModeStrictReject ValidationACKMode = "strict_reject"

	// ValidationModeAcceptAndFlag sends ACK, continues processing with warnings
	ValidationModeAcceptAndFlag ValidationACKMode = "accept_and_flag"

	// ValidationModeNoValidation always sends ACK, skips validation
	ValidationModeNoValidation ValidationACKMode = "no_validation"
)

// ValidationConfig represents the configuration for field validation step
type ValidationConfig struct {
	Rules            []ValidationRule  `json:"rules"`
	ValidationMode   ValidationACKMode `json:"validation_mode,omitempty"`  // How to handle validation failures
	StopOnFirstError bool              `json:"stopOnFirstError,omitempty"` // Stop validation on first failure
	AddFieldNames    bool              `json:"addFieldNames,omitempty"`    // Auto-add field names to error messages
	DetailedOutput   bool              `json:"detailedOutput,omitempty"`   // Include field-by-field validation results in output
}

// FieldValidationResult represents the result of a validation
type FieldValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []FieldValidationError `json:"errors,omitempty"`
}

// FieldValidationError represents a single validation error
type FieldValidationError struct {
	Field        string `json:"field"`        // Field path that failed
	FieldName    string `json:"fieldName"`    // Human-readable field name
	Type         string `json:"type"`         // Validation type that failed
	Message      string `json:"message"`      // Error message
	ActualValue  string `json:"actualValue"`  // The value that failed validation
	ExpectedFormat string `json:"expectedFormat,omitempty"` // Expected format/pattern
}

// FormatOptions represents options for format validation
type FormatOptions struct {
	Format string `json:"format"` // Preset: email, phone, ssn, date, datetime, mrn, zip
	Regex  string `json:"regex,omitempty"`  // Custom regex pattern
}

// LengthOptions represents options for length validation
type LengthOptions struct {
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
	Exact int `json:"exact,omitempty"`
}

// PatternOptions represents options for pattern validation
type PatternOptions struct {
	Regex string `json:"regex"` // Custom regex pattern
}
