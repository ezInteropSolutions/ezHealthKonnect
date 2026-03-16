// services/processing_logger.go
// ProcessingLogger — writes structured log entries to object storage (S3/MinIO/local)
// as NDJSON files, one file per message.
//
// Public API is backward-compatible: callers that previously passed a *mongo.Client
// now pass a *storage.ObjectStorageService (or nil for console-only logging).

package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/services/storage"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelError   LogLevel = "error"
	LogLevelWarning LogLevel = "warning"
	LogLevelInfo    LogLevel = "info"
	LogLevelDebug   LogLevel = "debug"
)

// LogCategory represents the processing stage
type LogCategory string

const (
	LogCategoryConnection     LogCategory = "connection"
	LogCategoryParsing        LogCategory = "parsing"
	LogCategoryValidation     LogCategory = "validation"
	LogCategoryTransformation LogCategory = "transformation"
	LogCategoryDelivery       LogCategory = "delivery"
)

// LogEntry represents a single log entry (kept for API compatibility).
type LogEntry struct {
	MessageID     string                 `json:"message_id"`
	CorrelationID string                 `json:"correlation_id"`
	LogLevel      LogLevel               `json:"log_level"`
	Category      LogCategory            `json:"category"`
	Timestamp     time.Time              `json:"timestamp"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
	Context       map[string]interface{} `json:"context,omitempty"`
	StackTrace    string                 `json:"stack_trace,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
}

// ProcessingLogger handles logging for message processing.
type ProcessingLogger struct {
	interfaceID   string
	interfaceName string
	messageID     string
	correlationID string
	messageType   string
	debugMode     bool
	objStorage    *storage.ObjectStorageService
	context       map[string]interface{}
}

// NewProcessingLogger creates a new logger instance backed by object storage.
// Pass nil for objStorage to fall back to console-only logging.
func NewProcessingLogger(
	interfaceID, interfaceName, messageID, correlationID, messageType string,
	debugMode bool,
	objStorage *storage.ObjectStorageService,
) *ProcessingLogger {
	return &ProcessingLogger{
		interfaceID:   interfaceID,
		interfaceName: interfaceName,
		messageID:     messageID,
		correlationID: correlationID,
		messageType:   messageType,
		debugMode:     debugMode,
		objStorage:    objStorage,
		context: map[string]interface{}{
			"interface_id":   interfaceID,
			"interface_name": interfaceName,
			"message_type":   messageType,
		},
	}
}

// Error logs an error (always captured regardless of debugMode).
func (l *ProcessingLogger) Error(category LogCategory, message string, details map[string]interface{}) {
	l.log(LogLevelError, category, message, details)
}

// Warning logs a warning (always captured regardless of debugMode).
func (l *ProcessingLogger) Warning(category LogCategory, message string, details map[string]interface{}) {
	l.log(LogLevelWarning, category, message, details)
}

// Info logs an informational message (only in debug mode).
func (l *ProcessingLogger) Info(category LogCategory, message string, details map[string]interface{}) {
	if l.debugMode {
		l.log(LogLevelInfo, category, message, details)
	}
}

// Debug logs detailed debug information (only in debug mode).
func (l *ProcessingLogger) Debug(category LogCategory, message string, details map[string]interface{}) {
	if l.debugMode {
		l.log(LogLevelDebug, category, message, details)
	}
}

// log writes a log entry to object storage as NDJSON (async) and to console.
func (l *ProcessingLogger) log(level LogLevel, category LogCategory, message string, details map[string]interface{}) {
	// Always print to console for immediate visibility
	fmt.Printf("[%s] %s: %s - %s\n", level, category, l.messageID, message)

	if l.objStorage == nil {
		return
	}

	entry := storage.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     string(level),
		Stage:     string(category),
		Message:   message,
	}

	// Merge details and context into Fields
	if len(details) > 0 || len(l.context) > 0 {
		fields := make(map[string]interface{})
		for k, v := range l.context {
			fields[k] = v
		}
		for k, v := range details {
			fields[k] = v
		}
		entry.Fields = fields
	}

	go func() {
		ctx := context.Background()
		if err := l.objStorage.AppendLog(ctx, l.interfaceID, l.messageID, entry); err != nil {
			fmt.Printf("❌ Failed to write log to object storage: %v\n", err)
		}
	}()
}

// AddContext adds contextual information to all future logs.
func (l *ProcessingLogger) AddContext(key string, value interface{}) {
	l.context[key] = value
}

// LogConnectionAttempt logs a connection attempt.
func (l *ProcessingLogger) LogConnectionAttempt(endpoint, protocol string) {
	l.Info(LogCategoryConnection, "Connecting to destination", map[string]interface{}{
		"endpoint": endpoint,
		"protocol": protocol,
	})
}

// LogConnectionSuccess logs successful connection.
func (l *ProcessingLogger) LogConnectionSuccess(endpoint string, durationMs int64) {
	l.Info(LogCategoryConnection, "Connected successfully", map[string]interface{}{
		"endpoint":    endpoint,
		"duration_ms": durationMs,
	})
}

// LogConnectionFailure logs connection failure.
func (l *ProcessingLogger) LogConnectionFailure(endpoint, errorMsg string, retryAttempt int) {
	l.Error(LogCategoryConnection, "Connection failed", map[string]interface{}{
		"endpoint":      endpoint,
		"error":         errorMsg,
		"retry_attempt": retryAttempt,
	})
}

// LogParsingStart logs the start of parsing.
func (l *ProcessingLogger) LogParsingStart(format string) {
	l.Debug(LogCategoryParsing, fmt.Sprintf("Starting %s parsing", format), nil)
}

// LogParsingComplete logs successful parsing.
func (l *ProcessingLogger) LogParsingComplete(format string, durationMs int64, segmentCount int) {
	l.Info(LogCategoryParsing, "Parsing completed", map[string]interface{}{
		"format":        format,
		"duration_ms":   durationMs,
		"segment_count": segmentCount,
	})
}

// LogParsingError logs parsing error.
func (l *ProcessingLogger) LogParsingError(format, errorMsg string, lineNumber int) {
	l.Error(LogCategoryParsing, "Parsing failed", map[string]interface{}{
		"format":      format,
		"error":       errorMsg,
		"line_number": lineNumber,
	})
}

// LogFieldMapping logs a field transformation (debug only).
func (l *ProcessingLogger) LogFieldMapping(sourceField, targetField, value string) {
	l.Debug(LogCategoryTransformation, "Field mapped", map[string]interface{}{
		"source": sourceField,
		"target": targetField,
		"value":  value,
	})
}

// LogTransformationComplete logs transformation completion.
func (l *ProcessingLogger) LogTransformationComplete(fieldsMapped int, durationMs int64) {
	l.Info(LogCategoryTransformation, "Transformation completed", map[string]interface{}{
		"fields_mapped": fieldsMapped,
		"duration_ms":   durationMs,
	})
}

// LogTransformationError logs transformation error.
func (l *ProcessingLogger) LogTransformationError(step, errorMsg string) {
	l.Error(LogCategoryTransformation, "Transformation failed", map[string]interface{}{
		"step":  step,
		"error": errorMsg,
	})
}

// LogDeliveryAttempt logs delivery attempt.
func (l *ProcessingLogger) LogDeliveryAttempt(endpoint, method string, attempt int) {
	l.Info(LogCategoryDelivery, "Attempting delivery", map[string]interface{}{
		"endpoint": endpoint,
		"method":   method,
		"attempt":  attempt,
	})
}

// LogDeliverySuccess logs successful delivery.
func (l *ProcessingLogger) LogDeliverySuccess(endpoint string, statusCode int, durationMs int64) {
	l.Info(LogCategoryDelivery, "Delivery successful", map[string]interface{}{
		"endpoint":    endpoint,
		"status_code": statusCode,
		"duration_ms": durationMs,
	})
}

// LogDeliveryFailure logs delivery failure.
func (l *ProcessingLogger) LogDeliveryFailure(endpoint, errorMsg string, statusCode, attempt int) {
	l.Error(LogCategoryDelivery, "Delivery failed", map[string]interface{}{
		"endpoint":    endpoint,
		"error":       errorMsg,
		"status_code": statusCode,
		"attempt":     attempt,
	})
}

// LogValidationWarning logs a validation warning.
func (l *ProcessingLogger) LogValidationWarning(field, issue string) {
	l.Warning(LogCategoryValidation, "Validation warning", map[string]interface{}{
		"field": field,
		"issue": issue,
	})
}

// GetMessageLogs retrieves log entries for a message from object storage.
// objStorage may be nil — returns empty slice in that case.
func GetMessageLogs(objStorage *storage.ObjectStorageService, interfaceID, messageID string, levelFilter LogLevel) ([]LogEntry, error) {
	if objStorage == nil {
		return nil, nil
	}

	entries, err := objStorage.GetLogs(context.Background(), interfaceID, messageID)
	if err != nil {
		return nil, err
	}

	var result []LogEntry
	for _, e := range entries {
		if levelFilter != "" && LogLevel(e.Level) != levelFilter {
			continue
		}
		result = append(result, LogEntry{
			MessageID:     messageID,
			CorrelationID: strings.Join([]string{interfaceID, messageID}, "/"),
			LogLevel:      LogLevel(e.Level),
			Category:      LogCategory(e.Stage),
			Timestamp:     e.Timestamp,
			Message:       e.Message,
			Details:       e.Fields,
		})
	}
	return result, nil
}
