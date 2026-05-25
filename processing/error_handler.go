package processing

import (
	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
	"fmt"
	"log"
	"runtime/debug"
)

// ErrorHandler provides panic recovery and error capture wrapper methods
// OOB Pattern: Automatic error handling and recovery for message processing
type ErrorHandler struct {
	errorService *services.ErrorCaptureService
}

// NewErrorHandler creates a new error handler instance
func NewErrorHandler(errorService *services.ErrorCaptureService) *ErrorHandler {
	return &ErrorHandler{
		errorService: errorService,
	}
}

// ExecuteWithPanicRecovery wraps a function with panic recovery
// This ensures one bad message never crashes the entire engine
// OOB: Message isolation - each message processes independently
func (eh *ErrorHandler) ExecuteWithPanicRecovery(
	interfaceID string,
	messageID string,
	stage string,
	fn func() error,
) (err error) {
	// Defer panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Panic occurred - capture it
			stack := string(debug.Stack())

			log.Printf("🚨 PANIC RECOVERED in %s for message %s", stage, messageID)
			log.Printf("   Panic value: %v", r)
			log.Printf("   Stack trace:\n%s", stack)

			// Capture panic in database
			captureErr := eh.errorService.CapturePanic(interfaceID, messageID, stage, r)
			if captureErr != nil {
				log.Printf("❌ Failed to capture panic: %v", captureErr)
			}

			// Return panic as error
			err = fmt.Errorf("PANIC RECOVERED: %v", r)
		}
	}()

	// Execute the function
	err = fn()

	// If function returned an error, capture it
	if err != nil {
		eh.errorService.ExecuteWithErrorCapture(interfaceID, messageID, stage, func() error {
			return err
		})
	}

	return err
}

// ExecuteWithErrorCapture wraps a function with automatic error capture
// Simpler version without panic recovery - just error logging
func (eh *ErrorHandler) ExecuteWithErrorCapture(
	interfaceID string,
	messageID string,
	stage string,
	fn func() error,
) error {
	return eh.errorService.ExecuteWithErrorCapture(interfaceID, messageID, stage, fn)
}

// CaptureAndContinue captures an error but allows processing to continue
// Used for non-critical errors where we want to log but not fail
func (eh *ErrorHandler) CaptureAndContinue(
	interfaceID string,
	messageID string,
	stage string,
	err error,
	message string,
) {
	if err == nil {
		return
	}

	errorContext := models.NewErrorContext(
		stage,
		models.SeverityWarning,
		models.ErrorTypeUnknown,
		message,
		err.Error(),
		"",
		models.RecoveryNone,
	)

	captureErr := eh.errorService.CaptureError(interfaceID, messageID, errorContext)
	if captureErr != nil {
		log.Printf("⚠️ Failed to capture warning: %v", captureErr)
	}
}

// SafeExecuteAsync executes a function in a goroutine with panic recovery
// OOB: Message isolation - each message in its own goroutine
func (eh *ErrorHandler) SafeExecuteAsync(
	interfaceID string,
	messageID string,
	stage string,
	fn func() error,
) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())

				log.Printf("🚨 ASYNC PANIC RECOVERED in %s for message %s: %v", stage, messageID, r)
				log.Printf("   Stack trace:\n%s", stack)

				// Capture panic
				eh.errorService.CapturePanic(interfaceID, messageID, stage, r)
			}
		}()

		// Execute function
		err := fn()
		if err != nil {
			// Capture error
			eh.errorService.ExecuteWithErrorCapture(interfaceID, messageID, stage, func() error {
				return err
			})
		}
	}()
}

// ValidateAndCaptureErrors runs validation and captures all errors
// Returns true if validation passed, false if errors found
func (eh *ErrorHandler) ValidateAndCaptureErrors(
	interfaceID string,
	messageID string,
	stage string,
	validationErrors []error,
) bool {
	if len(validationErrors) == 0 {
		return true
	}

	// Create error contexts for all validation errors
	var errorContexts []*models.ErrorContext

	for _, valErr := range validationErrors {
		errorContext := models.NewErrorContext(
			stage,
			models.SeverityError,
			models.ErrorTypeValidation,
			valErr.Error(),
			"Validation failed",
			"",
			models.RecoveryMarkedFailed,
		)
		errorContexts = append(errorContexts, errorContext)
	}

	// Batch capture all errors
	errors := eh.errorService.BatchCaptureErrors(interfaceID, messageID, errorContexts)
	if len(errors) > 0 {
		log.Printf("⚠️ Failed to capture %d validation errors", len(errors))
	}

	return false
}

// HandleCriticalError handles system-level critical errors (OOM, etc.)
func (eh *ErrorHandler) HandleCriticalError(
	interfaceID string,
	messageID string,
	stage string,
	errorType string,
	message string,
	details string,
) {
	errorContext := models.NewErrorContext(
		stage,
		models.SeverityCritical,
		errorType,
		message,
		details,
		string(debug.Stack()),
		models.RecoveryMarkedFailed,
	)

	err := eh.errorService.CaptureError(interfaceID, messageID, errorContext)
	if err != nil {
		log.Printf("❌ CRITICAL: Failed to capture critical error: %v", err)
	}

	log.Printf("🔴 CRITICAL ERROR in %s for message %s: %s", stage, messageID, message)
	if details != "" {
		log.Printf("   Details: %s", details)
	}
}

// LogRecoveryAction logs a successful recovery action
func (eh *ErrorHandler) LogRecoveryAction(
	interfaceID string,
	messageID string,
	stage string,
	action string,
	details string,
) {
	log.Printf("✅ Recovery action [%s] for message %s in %s: %s",
		action, messageID, stage, details)

	// Optionally capture as a warning for audit trail
	errorContext := models.NewErrorContext(
		stage,
		models.SeverityWarning,
		"RecoveryAction",
		fmt.Sprintf("Recovery action: %s", action),
		details,
		"",
		action,
	)

	eh.errorService.CaptureError(interfaceID, messageID, errorContext)
}

// IsRecoverableError determines if an error is recoverable
func (eh *ErrorHandler) IsRecoverableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Network errors are recoverable (retry)
	if contains(errMsg, "network") || contains(errMsg, "connection") || contains(errMsg, "timeout") {
		return true
	}

	// Database temporary errors are recoverable
	if contains(errMsg, "deadlock") || contains(errMsg, "lock timeout") {
		return true
	}

	// HTTP 5xx errors are recoverable
	if contains(errMsg, "500") || contains(errMsg, "502") || contains(errMsg, "503") || contains(errMsg, "504") {
		return true
	}

	return false
}

// ShouldRetry determines if an operation should be retried based on error
func (eh *ErrorHandler) ShouldRetry(err error, attemptCount int, maxAttempts int) bool {
	if err == nil {
		return false
	}

	if attemptCount >= maxAttempts {
		return false
	}

	return eh.IsRecoverableError(err)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
