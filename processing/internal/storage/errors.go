// internal/storage/errors.go
// Storage-specific error definitions

package storage

import (
	"errors"
	"fmt"
)

// Common storage errors
var (
	ErrProviderNotFound     = errors.New("storage provider not found")
	ErrNoPrimaryProvider    = errors.New("no primary storage provider configured")
	ErrConnectionFailed     = errors.New("failed to connect to storage provider")
	ErrTransactionFailed    = errors.New("transaction failed")
	ErrMessageNotFound      = errors.New("message not found")
	ErrInterfaceNotFound    = errors.New("interface configuration not found")
	ErrRuleNotFound         = errors.New("routing rule not found")
	ErrTransformNotFound    = errors.New("transformation configuration not found")
	ErrInvalidFilter        = errors.New("invalid filter criteria")
	ErrTimeout              = errors.New("operation timed out")
	ErrDuplicateKey         = errors.New("duplicate key constraint violation")
	ErrDataCorruption       = errors.New("data corruption detected")
	ErrStorageFull          = errors.New("storage capacity exceeded")
	ErrProviderUnavailable  = errors.New("storage provider temporarily unavailable")
)

// ErrorType defines the type of storage error
type ErrorType string

const (
	ErrorTypeConnection   ErrorType = "connection"
	ErrorTypeRead         ErrorType = "read"
	ErrorTypeWrite        ErrorType = "write"
	ErrorTypeDelete       ErrorType = "delete"
	ErrorTypeQuery        ErrorType = "query"
	ErrorTypeTransaction  ErrorType = "transaction"
	ErrorTypeValidation   ErrorType = "validation"
	ErrorTypeTimeout      ErrorType = "timeout"
	ErrorTypeCapacity     ErrorType = "capacity"
	ErrorTypeCorruption   ErrorType = "corruption"
	ErrorTypeAuth         ErrorType = "authentication"
	ErrorTypePermission   ErrorType = "permission"
)

// StorageError represents a storage-specific error
type StorageError struct {
	Type          ErrorType     `json:"type"`
	Provider      ProviderType  `json:"provider"`
	Operation     string        `json:"operation"`
	Message       string        `json:"message"`
	OriginalError error         `json:"original_error,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
	Retryable     bool          `json:"retryable"`
}

// Error implements the error interface
func (se *StorageError) Error() string {
	if se.OriginalError != nil {
		return fmt.Sprintf("%s error in %s provider during %s: %s (original: %v)",
			se.Type, se.Provider, se.Operation, se.Message, se.OriginalError)
	}
	return fmt.Sprintf("%s error in %s provider during %s: %s",
		se.Type, se.Provider, se.Operation, se.Message)
}

// Unwrap returns the original error for error wrapping
func (se *StorageError) Unwrap() error {
	return se.OriginalError
}

// IsRetryable returns whether the error is retryable
func (se *StorageError) IsRetryable() bool {
	return se.Retryable
}

// NewStorageError creates a new storage error
func NewStorageError(errorType ErrorType, provider ProviderType, operation, message string, originalError error) *StorageError {
	return &StorageError{
		Type:          errorType,
		Provider:      provider,
		Operation:     operation,
		Message:       message,
		OriginalError: originalError,
		Details:       make(map[string]interface{}),
		Retryable:     isRetryableError(errorType, originalError),
	}
}

// isRetryableError determines if an error is retryable based on type and original error
func isRetryableError(errorType ErrorType, originalError error) bool {
	switch errorType {
	case ErrorTypeConnection, ErrorTypeTimeout:
		return true
	case ErrorTypeCapacity, ErrorTypeCorruption, ErrorTypeValidation:
		return false
	case ErrorTypeRead, ErrorTypeWrite, ErrorTypeDelete, ErrorTypeQuery:
		// Check original error for specific conditions
		if originalError != nil {
			errStr := originalError.Error()
			// Network-related errors are usually retryable
			if containsAny(errStr, []string{"timeout", "connection", "network", "temporary"}) {
				return true
			}
			// Constraint violations are not retryable
			if containsAny(errStr, []string{"constraint", "unique", "duplicate"}) {
				return false
			}
		}
		return true // Default to retryable for database operations
	default:
		return false
	}
}

// containsAny checks if a string contains any of the given substrings
func containsAny(str string, substrings []string) bool {
	for _, substr := range substrings {
		if len(str) >= len(substr) {
			for i := 0; i <= len(str)-len(substr); i++ {
				if str[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// ErrorHandler defines error handling strategies
type ErrorHandler struct {
	MaxRetries      int
	RetryDelay      func(attempt int) int // Returns delay in milliseconds
	RetryableTypes  []ErrorType
	OnRetryExceeded func(error) error
}

// DefaultErrorHandler returns a default error handler
func DefaultErrorHandler() *ErrorHandler {
	return &ErrorHandler{
		MaxRetries: 3,
		RetryDelay: func(attempt int) int {
			// Exponential backoff: 100ms, 200ms, 400ms, etc.
			return 100 * (1 << uint(attempt-1))
		},
		RetryableTypes: []ErrorType{
			ErrorTypeConnection,
			ErrorTypeTimeout,
			ErrorTypeRead,
			ErrorTypeWrite,
			ErrorTypeQuery,
		},
		OnRetryExceeded: func(err error) error {
			return fmt.Errorf("max retries exceeded: %w", err)
		},
	}
}

// ShouldRetry determines if an error should be retried
func (eh *ErrorHandler) ShouldRetry(err error, attempt int) bool {
	if attempt >= eh.MaxRetries {
		return false
	}

	storageErr, ok := err.(*StorageError)
	if !ok {
		return false
	}

	if !storageErr.IsRetryable() {
		return false
	}

	for _, retryableType := range eh.RetryableTypes {
		if storageErr.Type == retryableType {
			return true
		}
	}

	return false
}

// GetRetryDelay returns the delay for the next retry attempt
func (eh *ErrorHandler) GetRetryDelay(attempt int) int {
	if eh.RetryDelay != nil {
		return eh.RetryDelay(attempt)
	}
	return 1000 // Default 1 second
}