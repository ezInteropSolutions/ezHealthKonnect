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

	// Format-agnostic schema-enriched fields — same structure regardless of format.
	// HL7:  keyed as "SEG.n"        e.g. "PID.5"
	// FHIR: keyed as "Type.element" e.g. "Patient.name"
	EnhancedFields  map[string]*EnhancedField `json:"enhanced_fields,omitempty"`
	FieldOrder      []string                  `json:"field_order,omitempty"`
	TypeName        string                    `json:"type_name,omitempty"`        // human-readable schema name
	TypeDescription string                    `json:"type_description,omitempty"` // schema description
}

// EnhancedField is a format-agnostic, schema-annotated message field.
// Field names mirror both hl7.FieldInfo and FHIR element definitions so
// consumers handle all formats through a single code path.
type EnhancedField struct {
	Path            string           `json:"path"`
	Value           interface{}      `json:"value,omitempty"`
	Name            string           `json:"name,omitempty"`
	Description     string           `json:"description,omitempty"`
	DataType        string           `json:"dataType,omitempty"`
	Cardinality     string           `json:"cardinality,omitempty"`
	Required        bool             `json:"required"`
	HasValue        bool             `json:"hasValue"`
	SchemaFound     bool             `json:"schemaFound"`
	MustSupport     bool             `json:"mustSupport,omitempty"`
	IsModifier      bool             `json:"isModifier,omitempty"`
	IsSummary       bool             `json:"isSummary,omitempty"`
	ValueSet        string           `json:"valueSet,omitempty"`
	BindingStrength string           `json:"bindingStrength,omitempty"`
	SubFields       []*EnhancedField `json:"subFields,omitempty"` // HL7 components / FHIR nested elements
}

// ParseError is a format-agnostic parse/validation issue.
type ParseError struct {
	Severity   string `json:"severity"` // "error" | "warning" | "info"
	Code       string `json:"code,omitempty"`
	Path       string `json:"path,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
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
