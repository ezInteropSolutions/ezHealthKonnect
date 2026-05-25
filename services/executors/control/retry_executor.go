package control

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"math"
	"time"
)

// RetryExecutor implements retry logic for child steps
type RetryExecutor struct {
	*executors.BaseExecutor
	childExecutor ChildStepExecutor
}

func NewRetryExecutor() *RetryExecutor {
	return &RetryExecutor{
		BaseExecutor: executors.NewBaseExecutor("control.retry", models.ExecutorMetadata{
			Name:        "Retry Logic",
			Description: "Retries child steps on failure with configurable backoff",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Control Flow",
		}),
	}
}

func (e *RetryExecutor) SetChildExecutor(executor ChildStepExecutor) {
	e.childExecutor = executor
}

type retryConfig struct {
	ChildSteps     []string `json:"childSteps"`     // Step IDs to execute (and retry on failure)
	MaxRetries     int      `json:"maxRetries"`     // Max retry attempts (default: 3)
	DelayMs        int      `json:"delayMs"`        // Initial delay between retries in ms (default: 1000)
	BackoffType    string   `json:"backoffType"`    // "fixed", "exponential", "linear" (default: "fixed")
	MaxDelayMs     int      `json:"maxDelayMs"`     // Max delay cap for exponential backoff (default: 30000)
	RetryOnErrors  []string `json:"retryOnErrors"`  // Only retry on specific error substrings (empty = retry all)
}

func (e *RetryExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := retryConfig{
		MaxRetries:  3,
		DelayMs:     1000,
		BackoffType: "fixed",
		MaxDelayMs:  30000,
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}
	if config.MaxRetries < 1 {
		config.MaxRetries = 3
	}
	if config.DelayMs < 0 {
		config.DelayMs = 1000
	}

	log.Printf("  🔄 Retry starting: %d child steps, max %d retries, %s backoff",
		len(config.ChildSteps), config.MaxRetries, config.BackoffType)

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	var lastErr error
	var lastErrMessage string
	attemptErrors := []string{}
	success := false
	totalAttempts := 0

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		totalAttempts = attempt + 1

		if attempt > 0 {
			delay := e.calculateDelay(config, attempt)
			log.Printf("  ⏳ Retry attempt %d/%d (waiting %dms...)", attempt, config.MaxRetries, delay)
			select {
			case <-ctx.Done():
				return outputData, ctx.Err()
			case <-time.After(time.Duration(delay) * time.Millisecond):
			}
		}

		// Execute all child steps
		currentInput := inputData
		attemptSuccess := true
		var attemptErr error

		if e.childExecutor != nil {
			for i, childStepID := range config.ChildSteps {
				// Inject retry context
				retryInput := make(map[string]interface{})
				for k, v := range currentInput {
					retryInput[k] = v
				}
				retryInput["_retry"] = map[string]interface{}{
					"attempt":     attempt + 1,
					"maxRetries":  config.MaxRetries,
					"isRetry":     attempt > 0,
				}

				childOutput, stepName, err := e.childExecutor(ctx, childStepID, retryInput)
				if err != nil {
					attemptErr = err
					attemptSuccess = false
					log.Printf("  ❌ Attempt %d: step %d/%d (%s) failed: %v",
						attempt+1, i+1, len(config.ChildSteps), stepName, err)
					break
				}
				if childOutput != nil {
					currentInput = childOutput
				}
			}
		}

		if attemptSuccess {
			success = true
			// Apply successful output
			for k, v := range currentInput {
				outputData[k] = v
			}
			log.Printf("  ✅ Attempt %d succeeded", attempt+1)
			break
		}

		lastErr = attemptErr
		lastErrMessage = attemptErr.Error()
		attemptErrors = append(attemptErrors, fmt.Sprintf("Attempt %d: %s", attempt+1, lastErrMessage))

		// Check if this error is retryable
		if len(config.RetryOnErrors) > 0 && !e.isRetryableError(lastErrMessage, config.RetryOnErrors) {
			log.Printf("  ⚠️ Error not retryable, stopping: %s", lastErrMessage)
			break
		}
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"success":        success,
		"total_attempts": totalAttempts,
		"error_message":  lastErrMessage,
		"attempt_errors": attemptErrors,
	}
	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"success":        true,
		"total_attempts": totalAttempts,
		"max_retries":    config.MaxRetries,
		"backoff_type":   config.BackoffType,
		"final_success":  success,
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	if !success {
		log.Printf("  ❌ Retry exhausted after %d attempts", totalAttempts)
		return outputData, fmt.Errorf("retry exhausted after %d attempts: %w", totalAttempts, lastErr)
	}

	log.Printf("  ✅ Retry completed successfully after %d attempt(s)", totalAttempts)
	return outputData, nil
}

func (e *RetryExecutor) calculateDelay(config retryConfig, attempt int) int {
	var delay int
	switch config.BackoffType {
	case "exponential":
		delay = config.DelayMs * int(math.Pow(2, float64(attempt-1)))
	case "linear":
		delay = config.DelayMs * attempt
	default: // "fixed"
		delay = config.DelayMs
	}
	if config.MaxDelayMs > 0 && delay > config.MaxDelayMs {
		delay = config.MaxDelayMs
	}
	return delay
}

func (e *RetryExecutor) isRetryableError(errMsg string, retryOnErrors []string) bool {
	for _, pattern := range retryOnErrors {
		if len(pattern) > 0 && contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (e *RetryExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("retry requires config with childSteps")
	}
	return nil
}
