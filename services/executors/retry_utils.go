package executors

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries         int     // Number of retry attempts (0 = no retry)
	DelayMs            int     // Initial delay between retries in milliseconds
	BackoffMultiplier  float64 // Multiplier for exponential backoff (e.g., 2.0 = double each time)
	MaxDelayMs         int     // Maximum delay cap in milliseconds (0 = no cap)
}

// RetryResult holds the outcome of a retried operation
type RetryResult struct {
	Output       map[string]interface{}
	Err          error
	Attempts     int   // Total attempts made (1 = no retries needed)
	TotalDelayMs int64 // Total time spent waiting between retries
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		DelayMs:           1000,
		BackoffMultiplier: 2.0,
		MaxDelayMs:        60000,
	}
}

// ParseRetryConfig extracts RetryConfig from a step's config map.
// Returns nil if retry is not enabled.
func ParseRetryConfig(config map[string]interface{}) *RetryConfig {
	retryRaw, ok := config["retry"].(map[string]interface{})
	if !ok {
		return nil
	}
	enabled, ok := retryRaw["enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	rc := DefaultRetryConfig()

	if mr, ok := retryRaw["maxRetries"].(float64); ok {
		rc.MaxRetries = int(mr)
	}
	if d, ok := retryRaw["delayMs"].(float64); ok {
		rc.DelayMs = int(d)
	}
	if bm, ok := retryRaw["backoffMultiplier"].(float64); ok {
		rc.BackoffMultiplier = bm
	}
	if md, ok := retryRaw["maxDelayMs"].(float64); ok {
		rc.MaxDelayMs = int(md)
	}

	return &rc
}

// ExecuteWithRetry wraps any operation with retry logic and exponential backoff.
// The operation function is called repeatedly until it succeeds or retries are exhausted.
//
// Usage:
//
//	result := ExecuteWithRetry(ctx, retryConfig, func(attempt int) (map[string]interface{}, error) {
//	    return executor.Execute(ctx, step, input)
//	})
func ExecuteWithRetry(ctx context.Context, config *RetryConfig, operation func(attempt int) (map[string]interface{}, error)) RetryResult {
	// No retry configured — single execution
	if config == nil || config.MaxRetries <= 0 {
		output, err := operation(1)
		return RetryResult{Output: output, Err: err, Attempts: 1}
	}

	totalAttempts := config.MaxRetries + 1 // 1 initial + N retries
	var result RetryResult
	var totalDelay int64

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		// Check context cancellation before each attempt
		if ctx.Err() != nil {
			result.Err = fmt.Errorf("retry cancelled: %w", ctx.Err())
			result.Attempts = attempt
			result.TotalDelayMs = totalDelay
			return result
		}

		// Wait before retry (not on first attempt)
		if attempt > 1 {
			delay := calculateBackoffDelay(config, attempt)
			totalDelay += int64(delay)

			fmt.Printf("🔄 Retry %d/%d (waiting %dms)\n", attempt-1, config.MaxRetries, delay)

			select {
			case <-time.After(time.Duration(delay) * time.Millisecond):
				// Delay completed
			case <-ctx.Done():
				result.Err = fmt.Errorf("retry cancelled during backoff: %w", ctx.Err())
				result.Attempts = attempt
				result.TotalDelayMs = totalDelay
				return result
			}
		}

		output, err := operation(attempt)
		if err == nil {
			if attempt > 1 {
				fmt.Printf("🔄 Succeeded on attempt %d\n", attempt)
			}
			return RetryResult{Output: output, Err: nil, Attempts: attempt, TotalDelayMs: totalDelay}
		}

		result.Output = output
		result.Err = err

		// Don't retry non-retryable errors (client errors, auth failures, bad config)
		// Only transient errors (5xx, timeout, connection refused) are worth retrying
		if !isRetryableError(err) {
			fmt.Printf("🔄 Non-retryable error, skipping remaining retries: %s\n", err.Error())
			result.Attempts = attempt
			result.TotalDelayMs = totalDelay
			return result
		}

		if attempt < totalAttempts {
			fmt.Printf("🔄 Attempt %d/%d failed: %s\n", attempt, totalAttempts, err.Error())
		}
	}

	result.Attempts = totalAttempts
	result.TotalDelayMs = totalDelay
	return result
}

// ErrorHandlingConfig holds error handling configuration
type ErrorHandlingConfig struct {
	OnError      string // "catch", "suppress", "rethrow"
	DefaultField string // Optional field to set a default value on error
	DefaultValue string // The default value to set
}

// ParseErrorHandlingConfig extracts ErrorHandlingConfig from a step's config map.
// Returns nil if error handling is not enabled.
func ParseErrorHandlingConfig(config map[string]interface{}) *ErrorHandlingConfig {
	ehRaw, ok := config["errorHandling"].(map[string]interface{})
	if !ok {
		return nil
	}
	enabled, ok := ehRaw["enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	eh := &ErrorHandlingConfig{
		OnError: "suppress", // Default (P2: "catch" was removed from UI; treat as "suppress")
	}
	if oe, ok := ehRaw["onError"].(string); ok {
		eh.OnError = oe
	}
	// P2: backward-compat alias — old pipelines saved with "catch" behave as "suppress".
	if eh.OnError == "catch" {
		eh.OnError = "suppress"
	}
	if df, ok := ehRaw["defaultField"].(string); ok {
		eh.DefaultField = df
	}
	if dv, ok := ehRaw["defaultValue"].(string); ok {
		eh.DefaultValue = dv
	}
	return eh
}

// ParsePipelineRetryDefaults extracts pipeline-level retry defaults from pipeline_config.
// Returns nil if no pipeline-level retry defaults are configured.
func ParsePipelineRetryDefaults(pipelineConfig map[string]interface{}) *RetryConfig {
	if pipelineConfig == nil {
		return nil
	}
	drRaw, ok := pipelineConfig["defaultRetry"].(map[string]interface{})
	if !ok {
		return nil
	}
	enabled, ok := drRaw["enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	rc := DefaultRetryConfig()
	if mr, ok := drRaw["maxRetries"].(float64); ok {
		rc.MaxRetries = int(mr)
	}
	if d, ok := drRaw["delayMs"].(float64); ok {
		rc.DelayMs = int(d)
	}
	if bm, ok := drRaw["backoffMultiplier"].(float64); ok {
		rc.BackoffMultiplier = bm
	}
	if md, ok := drRaw["maxDelayMs"].(float64); ok {
		rc.MaxDelayMs = int(md)
	}
	return &rc
}

// ParsePipelineErrorHandlingDefaults extracts pipeline-level error handling defaults from pipeline_config.
// Returns nil if no pipeline-level error handling defaults are configured.
func ParsePipelineErrorHandlingDefaults(pipelineConfig map[string]interface{}) *ErrorHandlingConfig {
	if pipelineConfig == nil {
		return nil
	}
	ehRaw, ok := pipelineConfig["defaultErrorHandling"].(map[string]interface{})
	if !ok {
		return nil
	}
	enabled, ok := ehRaw["enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	eh := &ErrorHandlingConfig{
		OnError: "suppress",
	}
	if oe, ok := ehRaw["onError"].(string); ok {
		eh.OnError = oe
	}
	if eh.OnError == "catch" {
		eh.OnError = "suppress"
	}
	if df, ok := ehRaw["defaultField"].(string); ok {
		eh.DefaultField = df
	}
	if dv, ok := ehRaw["defaultValue"].(string); ok {
		eh.DefaultValue = dv
	}
	return eh
}

// ResolveRetryConfig resolves the final retry config using the resolution chain:
//
//	Step Override > Pipeline Default > nil (no retry)
//
// Three-state logic:
//   - Step has retry config with enabled=true → use step config (override)
//   - Step has retry config with enabled=false → no retry (explicit opt-out)
//   - Step has no retry config at all → use pipeline default (inheritance)
func ResolveRetryConfig(stepConfig map[string]interface{}, pipelineDefault *RetryConfig) *RetryConfig {
	retryRaw, hasRetry := stepConfig["retry"].(map[string]interface{})
	if !hasRetry {
		// No step-level config → inherit pipeline default
		return pipelineDefault
	}

	// Step has explicit retry config — check if enabled
	enabled, ok := retryRaw["enabled"].(bool)
	if !ok {
		// "enabled" key missing → inherit pipeline default
		return pipelineDefault
	}
	if !enabled {
		// Explicitly disabled at step level → no retry (opt-out)
		return nil
	}

	// Step-level retry enabled → parse step config (overrides pipeline)
	return ParseRetryConfig(stepConfig)
}

// ResolveErrorHandlingConfig resolves the final error handling config using the resolution chain:
//
//	Step Override > Pipeline Default > nil (no error handling)
//
// Three-state logic:
//   - Step has errorHandling config with enabled=true → use step config (override)
//   - Step has errorHandling config with enabled=false → no error handling (explicit opt-out)
//   - Step has no errorHandling config at all → use pipeline default (inheritance)
func ResolveErrorHandlingConfig(stepConfig map[string]interface{}, pipelineDefault *ErrorHandlingConfig) *ErrorHandlingConfig {
	ehRaw, hasEH := stepConfig["errorHandling"].(map[string]interface{})
	if !hasEH {
		// No step-level config → inherit pipeline default
		return pipelineDefault
	}

	// Step has explicit error handling config — check if enabled
	enabled, ok := ehRaw["enabled"].(bool)
	if !ok {
		// "enabled" key missing → inherit pipeline default
		return pipelineDefault
	}
	if !enabled {
		// Explicitly disabled at step level → no error handling (opt-out)
		return nil
	}

	// Step-level error handling enabled → parse step config (overrides pipeline)
	return ParseErrorHandlingConfig(stepConfig)
}

// ApplyErrorHandling executes the error handling strategy on step failure.
// Centralizes catch/suppress/rethrow/default-value logic in one reusable function.
//
// Returns:
//   - Updated outputData (with _lastError, _lastErrorStep, and optional default value)
//   - nil error if caught/suppressed, original error if rethrown
func ApplyErrorHandling(
	config *ErrorHandlingConfig,
	stepErr error,
	outputData map[string]interface{},
	inputData map[string]interface{},
	stepName string,
) (map[string]interface{}, error) {
	if config == nil || stepErr == nil {
		return outputData, stepErr
	}

	fmt.Printf("🛡️ Error handling triggered for: %s (error: %s)\n", stepName, stepErr.Error())

	// Ensure outputData is non-nil; copy input if needed
	if outputData == nil {
		outputData = make(map[string]interface{})
		for k, v := range inputData {
			outputData[k] = v
		}
	}

	// Apply default value if configured
	if config.DefaultField != "" && config.DefaultValue != "" {
		UpdateFieldValue(outputData, config.DefaultField, config.DefaultValue)
		fmt.Printf("🛡️ Set default value: %s = %s\n", config.DefaultField, config.DefaultValue)
	}

	// Store error info for downstream steps
	outputData["_lastError"] = stepErr.Error()
	outputData["_lastErrorStep"] = stepName

	// Apply error handling strategy
	switch config.OnError {
	case "catch", "suppress":
		fmt.Printf("🛡️ Error caught by handler, continuing pipeline: %s\n", stepName)
		return outputData, nil // Clear the error — it's been handled
	case "rethrow":
		fmt.Printf("🛡️ Error rethrown, pipeline will handle: %s\n", stepName)
		return outputData, stepErr // Leave error intact
	default:
		// Unknown strategy — treat as catch (safe default)
		fmt.Printf("🛡️ Unknown onError strategy '%s', defaulting to catch: %s\n", config.OnError, stepName)
		return outputData, nil
	}
}

// isRetryableError determines if an error is transient and worth retrying.
// Client errors (400, 401, 403, 404, 422) are NOT retried — they won't succeed on retry.
// Server errors (5xx), timeouts, and connection issues ARE retried.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Non-retryable: HTTP 4xx client errors (bad request, auth, not found, validation)
	nonRetryablePatterns := []string{
		"status 400", "status 401", "status 403", "status 404", "status 405",
		"status 409", "status 422", "status 429",
		"bad request", "unauthorized", "forbidden", "not found",
		"invalid config", "missing required", "validation failed",
		"unsupported", "no such host",
	}
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(msg, pattern) {
			return false
		}
	}

	// Retryable: transient server/network errors
	retryablePatterns := []string{
		"status 500", "status 502", "status 503", "status 504",
		"timeout", "timed out", "connection refused", "connection reset",
		"eof", "broken pipe", "temporary failure", "no route to host",
		"i/o timeout", "network is unreachable",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	// Default: retry unknown errors (safer for transient failures)
	return true
}

// calculateBackoffDelay computes the delay for a given attempt using exponential backoff
func calculateBackoffDelay(config *RetryConfig, attempt int) int {
	delay := float64(config.DelayMs)

	// Apply exponential backoff: delay * multiplier^(attempt-2)
	// attempt 2 = first retry = base delay
	// attempt 3 = second retry = delay * multiplier
	// attempt 4 = third retry = delay * multiplier^2
	if config.BackoffMultiplier > 1 && attempt > 2 {
		delay *= math.Pow(config.BackoffMultiplier, float64(attempt-2))
	}

	// Apply max delay cap
	maxDelay := float64(config.MaxDelayMs)
	if maxDelay <= 0 {
		maxDelay = 60000 // Default 60s cap
	}
	if delay > maxDelay {
		delay = maxDelay
	}

	return int(delay)
}
