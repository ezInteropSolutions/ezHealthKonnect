// models/parser_models.go
// Data models for message parsing

package models

import "time"

// MessageFormat represents detected message format
type MessageFormat string

const (
	FormatHL7v2    MessageFormat = "hl7v2"
	FormatHL7v3    MessageFormat = "hl7v3"
	FormatFHIR     MessageFormat = "fhir"
	FormatCCDA     MessageFormat = "ccda"
	FormatXML      MessageFormat = "xml"
	FormatJSON     MessageFormat = "json"
	FormatEDI      MessageFormat = "edi"
	FormatCSV      MessageFormat = "csv"
	FormatUnknown  MessageFormat = "unknown"
)

// ParserResult represents the output of parsing
type ParserResult struct {
	Success          bool                   `json:"success"`
	Format           MessageFormat          `json:"format"`
	ParsedJSON       map[string]interface{} `json:"parsed_json"`
	Metadata         ParserMetadata         `json:"metadata"`
	ValidationResult ValidationResult       `json:"validation_result"`
	Error            string                 `json:"error,omitempty"`
	ParsingTime      time.Duration          `json:"parsing_time"`
}

// ParserMetadata contains parsing metadata
type ParserMetadata struct {
	ParserVersion    string    `json:"parser_version"`
	DetectedVersion  string    `json:"detected_version"`  // e.g., "HL7 v2.5"
	MessageType      string    `json:"message_type"`      // e.g., "ADT^A01"
	MessageControlID string    `json:"message_control_id"`
	SegmentCount     int       `json:"segment_count"`
	FieldCount       int       `json:"field_count"`
	ParsedAt         time.Time `json:"parsed_at"`
}

// ValidationResult contains validation results
type ValidationResult struct {
	IsValid      bool     `json:"is_valid"`
	Warnings     []string `json:"warnings,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	WarningCount int      `json:"warning_count"`
	ErrorCount   int      `json:"error_count"`
}

// FormatDetectionResult contains format detection results
type FormatDetectionResult struct {
	DetectedFormat MessageFormat `json:"detected_format"`
	Confidence     float64       `json:"confidence"` // 0.0 to 1.0
	Indicators     []string      `json:"indicators"` // Why this format was detected
}
