package services

import (
	"database/sql"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
)

// ErrorCaptureService handles all error capture and retrieval operations
// Follows MVC Service pattern - contains business logic for error handling
// OOB Pattern: Auto-initialized, self-contained, works out of the box
type ErrorCaptureService struct {
	db *sql.DB
}

// NewErrorCaptureService creates a new error capture service
// OOB: Ready to use immediately after creation
func NewErrorCaptureService(db *sql.DB) *ErrorCaptureService {
	log.Println("✅ ErrorCaptureService initialized - ready for error tracking")
	return &ErrorCaptureService{
		db: db,
	}
}

// CaptureError stores an error in the message's error stack
// This is the primary method for recording any error, warning, or exception
func (ecs *ErrorCaptureService) CaptureError(
	interfaceID string,
	messageID string,
	errorContext *models.ErrorContext,
) error {
	// Convert error context to JSON
	errorJSON, err := errorContext.ToJSON()
	if err != nil {
		log.Printf("❌ Failed to marshal error context: %v", err)
		return fmt.Errorf("failed to marshal error context: %w", err)
	}

	// Get table name for this interface
	tableName := ecs.getMessageTableName(interfaceID)

	// Call PostgreSQL function to add error to stack
	query := `SELECT add_error_to_message_stack($1, $2, $3::jsonb)`

	_, err = ecs.db.Exec(query, tableName, messageID, string(errorJSON))
	if err != nil {
		log.Printf("❌ Failed to capture error for message %s: %v", messageID, err)
		return fmt.Errorf("failed to capture error: %w", err)
	}

	// Log based on severity
	ecs.logError(messageID, errorContext)

	return nil
}

// CaptureSimpleError is a convenience method for quick error capture
// OOB: Simplified API for common use cases
func (ecs *ErrorCaptureService) CaptureSimpleError(
	interfaceID string,
	messageID string,
	stage string,
	severity string,
	message string,
) error {
	errorContext := models.NewErrorContext(
		stage,
		severity,
		models.ErrorTypeUnknown,
		message,
		"",
		"",
		models.RecoveryMarkedFailed,
	)

	return ecs.CaptureError(interfaceID, messageID, errorContext)
}

// CapturePanic captures a panic with full stack trace
// This should be called from defer/recover blocks
func (ecs *ErrorCaptureService) CapturePanic(
	interfaceID string,
	messageID string,
	stage string,
	recoveredValue interface{},
) error {
	// Get stack trace
	stackTrace := string(debug.Stack())

	// Create panic info
	panicInfo := models.NewPanicInfo(messageID, interfaceID, stage, recoveredValue, stackTrace)

	// Convert to error context
	errorContext := panicInfo.ToErrorContext()
	errorContext.Details = fmt.Sprintf("Panic value: %v", recoveredValue)

	// Capture the error
	err := ecs.CaptureError(interfaceID, messageID, errorContext)

	// Always log panics to console for immediate visibility
	log.Printf("🚨 PANIC RECOVERED in %s for message %s: %v\n%s",
		stage, messageID, recoveredValue, stackTrace)

	return err
}

// CaptureWarning captures a warning (non-critical issue)
func (ecs *ErrorCaptureService) CaptureWarning(
	interfaceID string,
	messageID string,
	stage string,
	errorType string,
	message string,
	details string,
) error {
	errorContext := models.NewErrorContext(
		stage,
		models.SeverityWarning,
		errorType,
		message,
		details,
		"",
		models.RecoveryNone,
	)

	return ecs.CaptureError(interfaceID, messageID, errorContext)
}

// GetMessageErrors retrieves all errors for a specific message
// Returns errors sorted by timestamp (newest first)
func (ecs *ErrorCaptureService) GetMessageErrors(
	interfaceID string,
	messageID string,
) ([]models.ErrorContext, error) {
	tableName := ecs.getMessageTableName(interfaceID)

	query := `
		SELECT * FROM get_message_errors($1, $2)
		ORDER BY error_timestamp DESC
	`

	rows, err := ecs.db.Query(query, tableName, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to query errors: %w", err)
	}
	defer rows.Close()

	var errors []models.ErrorContext

	for rows.Next() {
		var ec models.ErrorContext
		var timestamp string

		err := rows.Scan(
			&timestamp,
			&ec.Stage,
			&ec.Severity,
			&ec.ErrorType,
			&ec.Message,
			&ec.Details,
			&ec.StackTrace,
			&ec.RecoveryAction,
		)

		if err != nil {
			log.Printf("⚠️ Error scanning error row: %v", err)
			continue
		}

		// Parse timestamp
		// Note: PostgreSQL returns timestamp as string, parse it
		// For now, we'll use the raw string format
		errors = append(errors, ec)
	}

	return errors, nil
}

// GetErrorSummary retrieves error summary for a message
// Quick overview without fetching full error stack
func (ecs *ErrorCaptureService) GetErrorSummary(
	interfaceID string,
	messageID string,
) (*models.MessageErrorSummary, error) {
	tableName := ecs.getMessageTableName(interfaceID)

	query := fmt.Sprintf(`
		SELECT
			message_id,
			COALESCE(error_count, 0),
			COALESCE(last_error_message, ''),
			last_error_timestamp,
			COALESCE(last_error_stage, ''),
			COALESCE(error_severity, '')
		FROM %s
		WHERE message_id = $1
	`, tableName)

	var summary models.MessageErrorSummary
	var lastErrorTime sql.NullTime

	err := ecs.db.QueryRow(query, messageID).Scan(
		&summary.MessageID,
		&summary.ErrorCount,
		&summary.LastErrorMessage,
		&lastErrorTime,
		&summary.LastErrorStage,
		&summary.ErrorSeverity,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get error summary: %w", err)
	}

	if lastErrorTime.Valid {
		summary.LastErrorTime = lastErrorTime.Time
	}

	// Set boolean flags
	summary.HasCriticalError = summary.ErrorSeverity == models.SeverityCritical
	summary.HasError = summary.ErrorSeverity == models.SeverityError || summary.HasCriticalError
	summary.HasWarning = summary.ErrorSeverity == models.SeverityWarning

	return &summary, nil
}

// GetErrorStatistics retrieves error statistics for an interface
// Useful for monitoring and analytics
func (ecs *ErrorCaptureService) GetErrorStatistics(
	interfaceID string,
	hours int,
) (*models.ErrorStatistics, error) {
	tableName := ecs.getMessageTableName(interfaceID)

	query := `SELECT * FROM get_error_statistics($1, $2)`

	var stats models.ErrorStatistics
	var mostCommonErrorType, mostCommonErrorStage sql.NullString

	err := ecs.db.QueryRow(query, tableName, hours).Scan(
		&stats.TotalErrors,
		&stats.CriticalErrors,
		&stats.RegularErrors,
		&stats.Warnings,
		&mostCommonErrorType,
		&mostCommonErrorStage,
		&stats.ErrorRatePercent,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get error statistics: %w", err)
	}

	if mostCommonErrorType.Valid {
		stats.MostCommonErrorType = mostCommonErrorType.String
	}
	if mostCommonErrorStage.Valid {
		stats.MostCommonErrorStage = mostCommonErrorStage.String
	}

	stats.TimeWindowHours = hours

	return &stats, nil
}

// ClearMessageErrors clears all errors for a message (used for retry/reprocess)
func (ecs *ErrorCaptureService) ClearMessageErrors(
	interfaceID string,
	messageID string,
) error {
	tableName := ecs.getMessageTableName(interfaceID)

	query := fmt.Sprintf(`
		UPDATE %s
		SET
			error_stack = '[]'::jsonb,
			error_count = 0,
			last_error_message = NULL,
			last_error_timestamp = NULL,
			last_error_stage = NULL,
			error_severity = NULL,
			updated_at = NOW()
		WHERE message_id = $1
	`, tableName)

	_, err := ecs.db.Exec(query, messageID)
	if err != nil {
		return fmt.Errorf("failed to clear errors: %w", err)
	}

	log.Printf("✅ Cleared all errors for message %s (retry/reprocess)", messageID)

	return nil
}

// Helper methods

func (ecs *ErrorCaptureService) getMessageTableName(interfaceID string) string {
	// Replace hyphens with underscores for table name
	safeID := strings.ReplaceAll(interfaceID, "-", "_")
	return fmt.Sprintf("messages_intf_%s", safeID)
}

func (ecs *ErrorCaptureService) logError(messageID string, ec *models.ErrorContext) {
	switch ec.Severity {
	case models.SeverityCritical:
		log.Printf("🔴 CRITICAL ERROR [%s] in %s for message %s: %s",
			ec.ErrorType, ec.Stage, messageID, ec.Message)
	case models.SeverityError:
		log.Printf("❌ ERROR [%s] in %s for message %s: %s",
			ec.ErrorType, ec.Stage, messageID, ec.Message)
	case models.SeverityWarning:
		log.Printf("⚠️  WARNING [%s] in %s for message %s: %s",
			ec.ErrorType, ec.Stage, messageID, ec.Message)
	}

	// Log details if present
	if ec.Details != "" {
		log.Printf("   Details: %s", ec.Details)
	}
}

// ExecuteWithErrorCapture wraps a function with error capture
// OOB: Automatic error handling for any function
func (ecs *ErrorCaptureService) ExecuteWithErrorCapture(
	interfaceID string,
	messageID string,
	stage string,
	fn func() error,
) error {
	err := fn()
	if err != nil {
		// Determine error type from error message
		errorType := ecs.classifyError(err)

		// Create error context
		errorContext := models.NewErrorContext(
			stage,
			models.SeverityError,
			errorType,
			err.Error(),
			"",
			"",
			models.RecoveryMarkedFailed,
		)

		// Capture the error
		captureErr := ecs.CaptureError(interfaceID, messageID, errorContext)
		if captureErr != nil {
			log.Printf("⚠️ Failed to capture error: %v", captureErr)
		}
	}

	return err
}

func (ecs *ErrorCaptureService) classifyError(err error) string {
	errMsg := strings.ToLower(err.Error())

	if strings.Contains(errMsg, "parse") || strings.Contains(errMsg, "hl7") {
		return models.ErrorTypeHL7Parse
	}
	if strings.Contains(errMsg, "validation") || strings.Contains(errMsg, "valid") {
		return models.ErrorTypeValidation
	}
	if strings.Contains(errMsg, "map") || strings.Contains(errMsg, "transform") {
		return models.ErrorTypeMapping
	}
	if strings.Contains(errMsg, "network") || strings.Contains(errMsg, "connection") {
		return models.ErrorTypeNetwork
	}
	if strings.Contains(errMsg, "database") || strings.Contains(errMsg, "sql") {
		return models.ErrorTypeDatabase
	}
	if strings.Contains(errMsg, "timeout") {
		return models.ErrorTypeTimeout
	}
	if strings.Contains(errMsg, "config") {
		return models.ErrorTypeConfiguration
	}

	return models.ErrorTypeUnknown
}

// ErrorCaptureMethods provides a set of helper methods for common error scenarios
// OOB: Pre-built error capture patterns

func (ecs *ErrorCaptureService) CaptureHL7ParseError(interfaceID, messageID, message, details string) error {
	return ecs.CaptureError(interfaceID, messageID, models.NewErrorContext(
		models.StageJSONConversion,
		models.SeverityError,
		models.ErrorTypeHL7Parse,
		message,
		details,
		"",
		models.RecoveryMarkedFailed,
	))
}

func (ecs *ErrorCaptureService) CaptureValidationError(interfaceID, messageID, stage, message, details string) error {
	return ecs.CaptureError(interfaceID, messageID, models.NewErrorContext(
		stage,
		models.SeverityError,
		models.ErrorTypeValidation,
		message,
		details,
		"",
		models.RecoveryMarkedFailed,
	))
}

func (ecs *ErrorCaptureService) CaptureNetworkError(interfaceID, messageID, message, details string, retried bool) error {
	recovery := models.RecoveryMarkedFailed
	if retried {
		recovery = models.RecoveryRetried
	}

	return ecs.CaptureError(interfaceID, messageID, models.NewErrorContext(
		models.StageOutputDelivery,
		models.SeverityError,
		models.ErrorTypeNetwork,
		message,
		details,
		"",
		recovery,
	))
}

func (ecs *ErrorCaptureService) CaptureDatabaseError(interfaceID, messageID, stage, message, details string) error {
	return ecs.CaptureError(interfaceID, messageID, models.NewErrorContext(
		stage,
		models.SeverityError,
		models.ErrorTypeDatabase,
		message,
		details,
		"",
		models.RecoveryMarkedFailed,
	))
}

// BatchCaptureErrors captures multiple errors at once
// Useful for validation that produces multiple errors
func (ecs *ErrorCaptureService) BatchCaptureErrors(
	interfaceID string,
	messageID string,
	errorContexts []*models.ErrorContext,
) []error {
	var errors []error

	for _, ec := range errorContexts {
		err := ecs.CaptureError(interfaceID, messageID, ec)
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		log.Printf("⚠️ Failed to capture %d out of %d errors", len(errors), len(errorContexts))
	}

	return errors
}

// GetErrorStackAsJSON retrieves the raw error_stack JSONB for a message
// Useful for debugging or exporting
func (ecs *ErrorCaptureService) GetErrorStackAsJSON(
	interfaceID string,
	messageID string,
) (string, error) {
	tableName := ecs.getMessageTableName(interfaceID)

	query := fmt.Sprintf(`
		SELECT COALESCE(error_stack::text, '[]')
		FROM %s
		WHERE message_id = $1
	`, tableName)

	var errorStackJSON string
	err := ecs.db.QueryRow(query, messageID).Scan(&errorStackJSON)
	if err != nil {
		return "", fmt.Errorf("failed to get error stack: %w", err)
	}

	return errorStackJSON, nil
}

// IsMessageFailed checks if a message has failed (has critical or error severity)
func (ecs *ErrorCaptureService) IsMessageFailed(
	interfaceID string,
	messageID string,
) (bool, error) {
	summary, err := ecs.GetErrorSummary(interfaceID, messageID)
	if err != nil {
		return false, err
	}

	return summary.HasError || summary.HasCriticalError, nil
}

// GetFailedMessagesCount returns count of failed messages in last N hours
func (ecs *ErrorCaptureService) GetFailedMessagesCount(
	interfaceID string,
	hours int,
) (int64, error) {
	tableName := ecs.getMessageTableName(interfaceID)

	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM %s
		WHERE received_at >= NOW() - INTERVAL '%d hours'
		AND error_severity IN ('error', 'critical')
	`, tableName, hours)

	var count int64
	err := ecs.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get failed messages count: %w", err)
	}

	return count, nil
}
