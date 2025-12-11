package models

// ValidationRule represents a single validation rule
type ValidationRule struct {
	Field        string                 `json:"field"`        // Field path (e.g., "enhancedSegments.PID.fields[4].subfields[1].value")
	Type         string                 `json:"type"`         // Validation type: required, format, length, pattern
	ErrorMessage string                 `json:"errorMessage"` // Custom error message
	Options      map[string]interface{} `json:"options,omitempty"` // Type-specific options
}

// ValidationConfig represents the configuration for field validation step
type ValidationConfig struct {
	Rules            []ValidationRule `json:"rules"`
	StopOnFirstError bool             `json:"stopOnFirstError,omitempty"` // Stop validation on first failure
	AddFieldNames    bool             `json:"addFieldNames,omitempty"`    // Auto-add field names to error messages
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
