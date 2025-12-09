package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
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

// LogEntry represents a single log entry
type LogEntry struct {
	MessageID     string                 `bson:"message_id"`
	CorrelationID string                 `bson:"correlation_id"`
	LogLevel      LogLevel               `bson:"log_level"`
	Category      LogCategory            `bson:"category"`
	Timestamp     time.Time              `bson:"timestamp"`
	Message       string                 `bson:"message"`
	Details       map[string]interface{} `bson:"details,omitempty"`
	Context       map[string]interface{} `bson:"context,omitempty"`
	StackTrace    string                 `bson:"stack_trace,omitempty"`
	ErrorCode     string                 `bson:"error_code,omitempty"`
}

// ProcessingLogger handles logging for message processing
type ProcessingLogger struct {
	interfaceID     string
	interfaceName   string
	messageID       string
	correlationID   string
	messageType     string
	debugMode       bool
	mongoClient     *mongo.Client
	collectionName  string
	context         map[string]interface{}
}

// NewProcessingLogger creates a new logger instance
func NewProcessingLogger(
	interfaceID, interfaceName, messageID, correlationID, messageType string,
	debugMode bool,
	mongoClient *mongo.Client,
) *ProcessingLogger {
	return &ProcessingLogger{
		interfaceID:    interfaceID,
		interfaceName:  interfaceName,
		messageID:      messageID,
		correlationID:  correlationID,
		messageType:    messageType,
		debugMode:      debugMode,
		mongoClient:    mongoClient,
		collectionName: fmt.Sprintf("processing_logs_intf_%s", strings.ReplaceAll(interfaceID, "-", "_")),
		context: map[string]interface{}{
			"interface_id":   interfaceID,
			"interface_name": interfaceName,
			"message_type":   messageType,
		},
	}
}

// Error logs an error (always captured)
func (l *ProcessingLogger) Error(category LogCategory, message string, details map[string]interface{}) {
	l.log(LogLevelError, category, message, details)
}

// Warning logs a warning (always captured)
func (l *ProcessingLogger) Warning(category LogCategory, message string, details map[string]interface{}) {
	l.log(LogLevelWarning, category, message, details)
}

// Info logs an informational message (only in debug mode)
func (l *ProcessingLogger) Info(category LogCategory, message string, details map[string]interface{}) {
	if l.debugMode {
		l.log(LogLevelInfo, category, message, details)
	}
}

// Debug logs detailed debug information (only in debug mode)
func (l *ProcessingLogger) Debug(category LogCategory, message string, details map[string]interface{}) {
	if l.debugMode {
		l.log(LogLevelDebug, category, message, details)
	}
}

// log writes a log entry to MongoDB (async)
func (l *ProcessingLogger) log(level LogLevel, category LogCategory, message string, details map[string]interface{}) {
	// Skip if MongoDB client not available
	if l.mongoClient == nil {
		fmt.Printf("[%s] %s: %s - %s\n", level, category, l.messageID, message)
		return
	}

	// Create log entry
	entry := LogEntry{
		MessageID:     l.messageID,
		CorrelationID: l.correlationID,
		LogLevel:      level,
		Category:      category,
		Timestamp:     time.Now(),
		Message:       message,
		Details:       details,
		Context:       l.context,
	}

	// Write to MongoDB asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		collection := l.mongoClient.Database("ezhealthkonnect").Collection(l.collectionName)
		_, err := collection.InsertOne(ctx, entry)
		if err != nil {
			fmt.Printf("❌ Failed to write log to MongoDB: %v\n", err)
		}
	}()

	// Also print to console for immediate visibility
	fmt.Printf("[%s] %s: %s - %s\n", level, category, l.messageID, message)
}

// AddContext adds contextual information to all future logs
func (l *ProcessingLogger) AddContext(key string, value interface{}) {
	l.context[key] = value
}

// LogConnectionAttempt logs a connection attempt
func (l *ProcessingLogger) LogConnectionAttempt(endpoint, protocol string) {
	l.Info(LogCategoryConnection, "Connecting to destination", map[string]interface{}{
		"endpoint": endpoint,
		"protocol": protocol,
	})
}

// LogConnectionSuccess logs successful connection
func (l *ProcessingLogger) LogConnectionSuccess(endpoint string, durationMs int64) {
	l.Info(LogCategoryConnection, "Connected successfully", map[string]interface{}{
		"endpoint":    endpoint,
		"duration_ms": durationMs,
	})
}

// LogConnectionFailure logs connection failure
func (l *ProcessingLogger) LogConnectionFailure(endpoint, errorMsg string, retryAttempt int) {
	l.Error(LogCategoryConnection, "Connection failed", map[string]interface{}{
		"endpoint":      endpoint,
		"error":         errorMsg,
		"retry_attempt": retryAttempt,
	})
}

// LogParsingStart logs the start of parsing
func (l *ProcessingLogger) LogParsingStart(format string) {
	l.Debug(LogCategoryParsing, fmt.Sprintf("Starting %s parsing", format), nil)
}

// LogParsingComplete logs successful parsing
func (l *ProcessingLogger) LogParsingComplete(format string, durationMs int64, segmentCount int) {
	l.Info(LogCategoryParsing, "Parsing completed", map[string]interface{}{
		"format":        format,
		"duration_ms":   durationMs,
		"segment_count": segmentCount,
	})
}

// LogParsingError logs parsing error
func (l *ProcessingLogger) LogParsingError(format, errorMsg string, lineNumber int) {
	l.Error(LogCategoryParsing, "Parsing failed", map[string]interface{}{
		"format":      format,
		"error":       errorMsg,
		"line_number": lineNumber,
	})
}

// LogFieldMapping logs a field transformation (debug only)
func (l *ProcessingLogger) LogFieldMapping(sourceField, targetField, value string) {
	l.Debug(LogCategoryTransformation, "Field mapped", map[string]interface{}{
		"source": sourceField,
		"target": targetField,
		"value":  value,
	})
}

// LogTransformationComplete logs transformation completion
func (l *ProcessingLogger) LogTransformationComplete(fieldsMapped int, durationMs int64) {
	l.Info(LogCategoryTransformation, "Transformation completed", map[string]interface{}{
		"fields_mapped": fieldsMapped,
		"duration_ms":   durationMs,
	})
}

// LogTransformationError logs transformation error
func (l *ProcessingLogger) LogTransformationError(step, errorMsg string) {
	l.Error(LogCategoryTransformation, "Transformation failed", map[string]interface{}{
		"step":  step,
		"error": errorMsg,
	})
}

// LogDeliveryAttempt logs delivery attempt
func (l *ProcessingLogger) LogDeliveryAttempt(endpoint, method string, attempt int) {
	l.Info(LogCategoryDelivery, "Attempting delivery", map[string]interface{}{
		"endpoint": endpoint,
		"method":   method,
		"attempt":  attempt,
	})
}

// LogDeliverySuccess logs successful delivery
func (l *ProcessingLogger) LogDeliverySuccess(endpoint string, statusCode int, durationMs int64) {
	l.Info(LogCategoryDelivery, "Delivery successful", map[string]interface{}{
		"endpoint":    endpoint,
		"status_code": statusCode,
		"duration_ms": durationMs,
	})
}

// LogDeliveryFailure logs delivery failure
func (l *ProcessingLogger) LogDeliveryFailure(endpoint, errorMsg string, statusCode, attempt int) {
	l.Error(LogCategoryDelivery, "Delivery failed", map[string]interface{}{
		"endpoint":    endpoint,
		"error":       errorMsg,
		"status_code": statusCode,
		"attempt":     attempt,
	})
}

// LogValidationWarning logs a validation warning
func (l *ProcessingLogger) LogValidationWarning(field, issue string) {
	l.Warning(LogCategoryValidation, "Validation warning", map[string]interface{}{
		"field": field,
		"issue": issue,
	})
}

// GetLogs retrieves logs for the current message
func GetMessageLogs(mongoClient *mongo.Client, interfaceID, messageID string, levelFilter LogLevel) ([]LogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collectionName := fmt.Sprintf("processing_logs_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
	collection := mongoClient.Database("ezhealthkonnect").Collection(collectionName)

	// Build filter
	filter := bson.M{"message_id": messageID}
	if levelFilter != "" {
		filter["log_level"] = levelFilter
	}

	// Query logs
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	// Decode results
	var logs []LogEntry
	if err = cursor.All(ctx, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}
