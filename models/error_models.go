package models

import (
	"encoding/json"
	"time"
)

// ErrorContext represents a single error/warning captured during message processing
// This follows the MVC Model pattern - pure data structure with no business logic
type ErrorContext struct {
	Timestamp      time.Time `json:"timestamp"`       // When the error occurred
	Stage          string    `json:"stage"`           // Processing stage: json_conversion, transformation, output_delivery
	Severity       string    `json:"severity"`        // warning, error, critical
	ErrorType      string    `json:"error_type"`      // HL7ParseError, ValidationError, NetworkError, Panic, etc.
	Message        string    `json:"message"`         // Short error description
	Details        string    `json:"details"`         // Detailed error information
	StackTrace     string    `json:"stack_trace"`     // Full stack trace for debugging (only for critical errors)
	RecoveryAction string    `json:"recovery_action"` // What was done: marked_failed, auto_corrected, retried, panic_recovered
}

// ErrorSeverity constants - standardized severity levels
const (
	SeverityWarning  = "warning"  // Non-critical issue, processing continued
	SeverityError    = "error"    // Error that caused processing to fail
	SeverityCritical = "critical" // System-level error (panic, OOM, etc.)
)

// ProcessingStage constants - standardized stage names
const (
	StageReception          = "reception"           // TCP/HTTP message reception
	StageJSONConversion     = "json_conversion"     // HL7 to JSON parsing
	StageTransformation     = "transformation"      // Transformation pipeline execution
	StageValidation         = "validation"          // Pre/post validation
	StageOutputDelivery     = "output_delivery"     // HTTP/FHIR delivery
	StageDatabase           = "database"            // Database operations
	StageUnknown            = "unknown"             // Fallback for uncategorized errors
)

// ErrorType constants - standardized error types
const (
	ErrorTypeHL7Parse       = "HL7ParseError"
	ErrorTypeValidation     = "ValidationError"
	ErrorTypeMapping        = "MappingError"
	ErrorTypeTransformation = "TransformationError"
	ErrorTypeNetwork        = "NetworkError"
	ErrorTypeDatabase       = "DatabaseError"
	ErrorTypeTimeout        = "TimeoutError"
	ErrorTypePanic          = "Panic"
	ErrorTypeOOM            = "OutOfMemory"
	ErrorTypeConfiguration  = "ConfigurationError"
	ErrorTypeUnknown        = "UnknownError"
)

// RecoveryAction constants - what the system did after error
const (
	RecoveryMarkedFailed    = "marked_failed"      // Message marked as failed
	RecoveryAutoCorrected   = "auto_corrected"     // System auto-corrected the issue
	RecoveryRetried         = "retried"            // Attempted retry
	RecoveryPanicRecovered  = "panic_recovered"    // Panic was caught and recovered
	RecoveryCircuitOpened   = "circuit_opened"     // Circuit breaker opened
	RecoverySkipped         = "skipped"            // Step skipped due to error
	RecoveryNone            = "none"               // No recovery action taken
)

// NewErrorContext creates a new error context with timestamp
// Factory pattern for consistent error creation
func NewErrorContext(stage, severity, errorType, message, details, stackTrace, recoveryAction string) *ErrorContext {
	return &ErrorContext{
		Timestamp:      time.Now().UTC(),
		Stage:          stage,
		Severity:       severity,
		ErrorType:      errorType,
		Message:        message,
		Details:        details,
		StackTrace:     stackTrace,
		RecoveryAction: recoveryAction,
	}
}

// ToJSON converts ErrorContext to JSON for PostgreSQL JSONB storage
func (ec *ErrorContext) ToJSON() ([]byte, error) {
	return json.Marshal(ec)
}

// IsCritical returns true if error severity is critical
func (ec *ErrorContext) IsCritical() bool {
	return ec.Severity == SeverityCritical
}

// IsError returns true if error severity is error or critical
func (ec *ErrorContext) IsError() bool {
	return ec.Severity == SeverityError || ec.Severity == SeverityCritical
}

// IsWarning returns true if error severity is warning
func (ec *ErrorContext) IsWarning() bool {
	return ec.Severity == SeverityWarning
}

// MessageErrorSummary represents error summary for a message
// Used for quick error status retrieval
type MessageErrorSummary struct {
	MessageID         string    `json:"message_id"`
	ErrorCount        int       `json:"error_count"`
	LastErrorMessage  string    `json:"last_error_message"`
	LastErrorTime     time.Time `json:"last_error_time"`
	LastErrorStage    string    `json:"last_error_stage"`
	ErrorSeverity     string    `json:"error_severity"`
	HasCriticalError  bool      `json:"has_critical_error"`
	HasError          bool      `json:"has_error"`
	HasWarning        bool      `json:"has_warning"`
}

// ErrorStatistics represents aggregated error statistics
// Used for error analytics and monitoring
type ErrorStatistics struct {
	TotalErrors          int64   `json:"total_errors"`
	CriticalErrors       int64   `json:"critical_errors"`
	RegularErrors        int64   `json:"regular_errors"`
	Warnings             int64   `json:"warnings"`
	MostCommonErrorType  string  `json:"most_common_error_type"`
	MostCommonErrorStage string  `json:"most_common_error_stage"`
	ErrorRatePercent     float64 `json:"error_rate_percent"`
	TimeWindowHours      int     `json:"time_window_hours"`
}

// ErrorFilter represents filtering criteria for error queries
type ErrorFilter struct {
	Severity       string    // Filter by severity
	Stage          string    // Filter by stage
	ErrorType      string    // Filter by error type
	StartTime      time.Time // Filter by time range start
	EndTime        time.Time // Filter by time range end
	Limit          int       // Limit number of results
}

// PanicInfo captures panic information for logging and error reporting
type PanicInfo struct {
	RecoveredValue interface{} `json:"recovered_value"` // The panic value
	StackTrace     string      `json:"stack_trace"`     // Full stack trace
	MessageID      string      `json:"message_id"`      // Message being processed
	InterfaceID    string      `json:"interface_id"`    // Interface ID
	Stage          string      `json:"stage"`           // Stage where panic occurred
	Timestamp      time.Time   `json:"timestamp"`       // When panic occurred
}

// NewPanicInfo creates a new PanicInfo instance
func NewPanicInfo(messageID, interfaceID, stage string, recoveredValue interface{}, stackTrace string) *PanicInfo {
	return &PanicInfo{
		RecoveredValue: recoveredValue,
		StackTrace:     stackTrace,
		MessageID:      messageID,
		InterfaceID:    interfaceID,
		Stage:          stage,
		Timestamp:      time.Now().UTC(),
	}
}

// ToErrorContext converts PanicInfo to ErrorContext for storage
func (pi *PanicInfo) ToErrorContext() *ErrorContext {
	return NewErrorContext(
		pi.Stage,
		SeverityCritical,
		ErrorTypePanic,
		"PANIC: System panic recovered",
		pi.RecoveredValue.(string),
		pi.StackTrace,
		RecoveryPanicRecovered,
	)
}
