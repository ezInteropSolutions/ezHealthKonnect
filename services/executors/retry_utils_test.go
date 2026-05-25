package executors

import (
	"context"
	"errors"
	"ezhealthkonnect/models"
	"fmt"
	"testing"
	"time"
)

// ===============================================================
// RETRY CONFIG PARSING TESTS
// ===============================================================

func TestParseRetryConfig_Nil_WhenNoRetryKey(t *testing.T) {
	config := map[string]interface{}{
		"someOtherKey": "value",
	}
	result := ParseRetryConfig(config)
	if result != nil {
		t.Fatalf("Expected nil, got %+v", result)
	}
}

func TestParseRetryConfig_Nil_WhenNotEnabled(t *testing.T) {
	config := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled": false,
		},
	}
	result := ParseRetryConfig(config)
	if result != nil {
		t.Fatalf("Expected nil when not enabled, got %+v", result)
	}
}

func TestParseRetryConfig_Defaults_WhenEnabledNoParams(t *testing.T) {
	config := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled": true,
		},
	}
	result := ParseRetryConfig(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", result.MaxRetries)
	}
	if result.DelayMs != 1000 {
		t.Errorf("Expected DelayMs=1000, got %d", result.DelayMs)
	}
	if result.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier=2.0, got %f", result.BackoffMultiplier)
	}
	if result.MaxDelayMs != 60000 {
		t.Errorf("Expected MaxDelayMs=60000, got %d", result.MaxDelayMs)
	}
}

func TestParseRetryConfig_CustomValues(t *testing.T) {
	config := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled":           true,
			"maxRetries":        float64(5),
			"delayMs":           float64(500),
			"backoffMultiplier": float64(3.0),
			"maxDelayMs":        float64(30000),
		},
	}
	result := ParseRetryConfig(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", result.MaxRetries)
	}
	if result.DelayMs != 500 {
		t.Errorf("Expected DelayMs=500, got %d", result.DelayMs)
	}
	if result.BackoffMultiplier != 3.0 {
		t.Errorf("Expected BackoffMultiplier=3.0, got %f", result.BackoffMultiplier)
	}
	if result.MaxDelayMs != 30000 {
		t.Errorf("Expected MaxDelayMs=30000, got %d", result.MaxDelayMs)
	}
}

func TestParseRetryConfig_Nil_WhenEmptyConfig(t *testing.T) {
	config := map[string]interface{}{}
	result := ParseRetryConfig(config)
	if result != nil {
		t.Fatalf("Expected nil for empty config, got %+v", result)
	}
}

// ===============================================================
// BACKOFF DELAY CALCULATION TESTS
// ===============================================================

func TestCalculateBackoffDelay_FirstRetry_BaseDelay(t *testing.T) {
	config := &RetryConfig{DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 60000}
	// attempt 2 = first retry = base delay
	delay := calculateBackoffDelay(config, 2)
	if delay != 1000 {
		t.Errorf("Expected 1000ms for first retry, got %d", delay)
	}
}

func TestCalculateBackoffDelay_ExponentialGrowth(t *testing.T) {
	config := &RetryConfig{DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 60000}

	// attempt 2 = 1000ms (base)
	// attempt 3 = 1000 * 2^1 = 2000ms
	// attempt 4 = 1000 * 2^2 = 4000ms
	expected := []int{1000, 2000, 4000}
	for i, exp := range expected {
		delay := calculateBackoffDelay(config, i+2)
		if delay != exp {
			t.Errorf("Attempt %d: expected %dms, got %dms", i+2, exp, delay)
		}
	}
}

func TestCalculateBackoffDelay_CappedAtMaxDelay(t *testing.T) {
	config := &RetryConfig{DelayMs: 10000, BackoffMultiplier: 3.0, MaxDelayMs: 15000}
	// attempt 3 = 10000 * 3^1 = 30000 -> capped at 15000
	delay := calculateBackoffDelay(config, 3)
	if delay != 15000 {
		t.Errorf("Expected delay capped at 15000, got %d", delay)
	}
}

func TestCalculateBackoffDelay_NoMultiplier(t *testing.T) {
	config := &RetryConfig{DelayMs: 500, BackoffMultiplier: 1.0, MaxDelayMs: 60000}
	// With multiplier=1.0, delay should stay constant
	for attempt := 2; attempt <= 5; attempt++ {
		delay := calculateBackoffDelay(config, attempt)
		if delay != 500 {
			t.Errorf("Attempt %d: expected fixed 500ms, got %d", attempt, delay)
		}
	}
}

func TestCalculateBackoffDelay_DefaultMaxDelay_WhenZero(t *testing.T) {
	config := &RetryConfig{DelayMs: 50000, BackoffMultiplier: 2.0, MaxDelayMs: 0}
	// attempt 3 = 50000 * 2 = 100000 -> capped at default 60000
	delay := calculateBackoffDelay(config, 3)
	if delay != 60000 {
		t.Errorf("Expected default cap at 60000, got %d", delay)
	}
}

// ===============================================================
// EXECUTE WITH RETRY TESTS
// ===============================================================

func TestExecuteWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 3, DelayMs: 10}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"status": "ok"}, nil
	})

	if result.Err != nil {
		t.Fatalf("Expected no error, got: %v", result.Err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}
	if result.Output["status"] != "ok" {
		t.Errorf("Expected output status=ok, got %v", result.Output["status"])
	}
}

func TestExecuteWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 3, DelayMs: 10, BackoffMultiplier: 1.0}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		if callCount == 1 {
			return nil, fmt.Errorf("transient error")
		}
		return map[string]interface{}{"recovered": true}, nil
	})

	if result.Err != nil {
		t.Fatalf("Expected no error after recovery, got: %v", result.Err)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}
	if result.Attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", result.Attempts)
	}
}

func TestExecuteWithRetry_AllRetriesExhausted(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 2, DelayMs: 10, BackoffMultiplier: 1.0}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return nil, fmt.Errorf("persistent error")
	})

	if result.Err == nil {
		t.Fatal("Expected error after all retries exhausted")
	}
	if callCount != 3 { // 1 initial + 2 retries
		t.Errorf("Expected 3 calls (1+2 retries), got %d", callCount)
	}
	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
	if result.Err.Error() != "persistent error" {
		t.Errorf("Expected 'persistent error', got '%s'", result.Err.Error())
	}
}

func TestExecuteWithRetry_NilConfig_SingleExecution(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), nil, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"single": true}, nil
	})

	if result.Err != nil {
		t.Fatalf("Expected no error, got: %v", result.Err)
	}
	if callCount != 1 {
		t.Errorf("Expected exactly 1 call with nil config, got %d", callCount)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}
}

func TestExecuteWithRetry_ZeroRetries_SingleExecution(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 0}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return nil, fmt.Errorf("should fail once")
	})

	if result.Err == nil {
		t.Fatal("Expected error with 0 retries")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call with 0 retries, got %d", callCount)
	}
}

func TestExecuteWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	result := ExecuteWithRetry(ctx, &RetryConfig{MaxRetries: 5, DelayMs: 500, BackoffMultiplier: 1.0}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		if callCount == 1 {
			cancel() // Cancel after first failure
			return nil, fmt.Errorf("first failure")
		}
		return map[string]interface{}{"should": "not reach"}, nil
	})

	if result.Err == nil {
		t.Fatal("Expected error from context cancellation")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call before cancellation, got %d", callCount)
	}
}

func TestExecuteWithRetry_AttemptNumberPassedCorrectly(t *testing.T) {
	attempts := []int{}
	ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 2, DelayMs: 10, BackoffMultiplier: 1.0}, func(attempt int) (map[string]interface{}, error) {
		attempts = append(attempts, attempt)
		return nil, fmt.Errorf("fail")
	})

	expected := []int{1, 2, 3}
	if len(attempts) != len(expected) {
		t.Fatalf("Expected %d attempts, got %d", len(expected), len(attempts))
	}
	for i, exp := range expected {
		if attempts[i] != exp {
			t.Errorf("Attempt %d: expected %d, got %d", i, exp, attempts[i])
		}
	}
}

func TestExecuteWithRetry_TotalDelayAccumulated(t *testing.T) {
	start := time.Now()
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 2, DelayMs: 50, BackoffMultiplier: 1.0, MaxDelayMs: 60000}, func(attempt int) (map[string]interface{}, error) {
		return nil, fmt.Errorf("fail")
	})

	elapsed := time.Since(start).Milliseconds()

	// Should have waited ~100ms (50ms * 2 retries)
	if result.TotalDelayMs < 80 {
		t.Errorf("Expected TotalDelayMs >= 80, got %d", result.TotalDelayMs)
	}
	// Actual elapsed should be >= the total delay
	if elapsed < 80 {
		t.Errorf("Expected elapsed >= 80ms, got %dms", elapsed)
	}
}

func TestExecuteWithRetry_OutputPreservedFromLastAttempt(t *testing.T) {
	callCount := 0
	result := ExecuteWithRetry(context.Background(), &RetryConfig{MaxRetries: 2, DelayMs: 10, BackoffMultiplier: 1.0}, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"attempt": callCount}, fmt.Errorf("fail %d", callCount)
	})

	if result.Err == nil {
		t.Fatal("Expected error")
	}
	// Output should be from the last attempt
	if result.Output["attempt"] != 3 {
		t.Errorf("Expected output from attempt 3, got %v", result.Output["attempt"])
	}
}

// ===============================================================
// BASE EXECUTOR RETRY CONVENIENCE METHOD TEST
// ===============================================================

func TestBaseExecutor_ExecuteWithRetry(t *testing.T) {
	base := NewBaseExecutor("test.step", models.ExecutorMetadata{})

	config := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled":    true,
			"maxRetries": float64(2),
			"delayMs":    float64(10),
		},
	}

	callCount := 0
	result := base.ExecuteWithRetry(context.Background(), config, func(attempt int) (map[string]interface{}, error) {
		callCount++
		if callCount < 2 {
			return nil, fmt.Errorf("transient")
		}
		return map[string]interface{}{"ok": true}, nil
	})

	if result.Err != nil {
		t.Fatalf("Expected success, got: %v", result.Err)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}
}

func TestBaseExecutor_ExecuteWithRetry_NoRetryConfig(t *testing.T) {
	base := NewBaseExecutor("test.step", models.ExecutorMetadata{})

	config := map[string]interface{}{
		"someOtherKey": "value",
	}

	callCount := 0
	result := base.ExecuteWithRetry(context.Background(), config, func(attempt int) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"single": true}, nil
	})

	if result.Err != nil {
		t.Fatalf("Expected no error, got: %v", result.Err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call (no retry), got %d", callCount)
	}
}

// ===============================================================
// ERROR HANDLING CONFIG PARSING TESTS
// ===============================================================

func TestParseErrorHandlingConfig_Nil_WhenNoKey(t *testing.T) {
	config := map[string]interface{}{}
	result := ParseErrorHandlingConfig(config)
	if result != nil {
		t.Fatalf("Expected nil, got %+v", result)
	}
}

func TestParseErrorHandlingConfig_Nil_WhenNotEnabled(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": false,
		},
	}
	result := ParseErrorHandlingConfig(config)
	if result != nil {
		t.Fatalf("Expected nil when not enabled, got %+v", result)
	}
}

func TestParseErrorHandlingConfig_Defaults(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": true,
		},
	}
	result := ParseErrorHandlingConfig(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.OnError != "catch" {
		t.Errorf("Expected OnError=catch, got %s", result.OnError)
	}
}

func TestParseErrorHandlingConfig_CustomValues(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled":      true,
			"onError":      "rethrow",
			"defaultField": "PID.3",
			"defaultValue": "UNKNOWN",
		},
	}
	result := ParseErrorHandlingConfig(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.OnError != "rethrow" {
		t.Errorf("Expected OnError=rethrow, got %s", result.OnError)
	}
	if result.DefaultField != "PID.3" {
		t.Errorf("Expected DefaultField=PID.3, got %s", result.DefaultField)
	}
	if result.DefaultValue != "UNKNOWN" {
		t.Errorf("Expected DefaultValue=UNKNOWN, got %s", result.DefaultValue)
	}
}

// ===============================================================
// PIPELINE DEFAULTS PARSING TESTS
// ===============================================================

func TestParsePipelineRetryDefaults_Nil_WhenEmpty(t *testing.T) {
	result := ParsePipelineRetryDefaults(nil)
	if result != nil {
		t.Fatalf("Expected nil for nil config, got %+v", result)
	}
}

func TestParsePipelineRetryDefaults_Nil_WhenNotEnabled(t *testing.T) {
	config := map[string]interface{}{
		"defaultRetry": map[string]interface{}{
			"enabled": false,
		},
	}
	result := ParsePipelineRetryDefaults(config)
	if result != nil {
		t.Fatalf("Expected nil when not enabled, got %+v", result)
	}
}

func TestParsePipelineRetryDefaults_ParsesValues(t *testing.T) {
	config := map[string]interface{}{
		"defaultRetry": map[string]interface{}{
			"enabled":           true,
			"maxRetries":        float64(5),
			"delayMs":           float64(2000),
			"backoffMultiplier": float64(1.5),
			"maxDelayMs":        float64(30000),
		},
	}
	result := ParsePipelineRetryDefaults(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", result.MaxRetries)
	}
	if result.DelayMs != 2000 {
		t.Errorf("Expected DelayMs=2000, got %d", result.DelayMs)
	}
}

func TestParsePipelineErrorHandlingDefaults_ParsesValues(t *testing.T) {
	config := map[string]interface{}{
		"defaultErrorHandling": map[string]interface{}{
			"enabled": true,
			"onError": "suppress",
		},
	}
	result := ParsePipelineErrorHandlingDefaults(config)
	if result == nil {
		t.Fatal("Expected non-nil config")
	}
	if result.OnError != "suppress" {
		t.Errorf("Expected OnError=suppress, got %s", result.OnError)
	}
}

// ===============================================================
// RESOLVE CONFIG TESTS (INHERITANCE CHAIN)
// ===============================================================

func TestResolveRetryConfig_InheritsPipelineDefault(t *testing.T) {
	stepConfig := map[string]interface{}{} // No retry config
	pipelineDefault := &RetryConfig{MaxRetries: 3, DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 60000}

	result := ResolveRetryConfig(stepConfig, pipelineDefault)
	if result == nil {
		t.Fatal("Expected pipeline default to be inherited")
	}
	if result.MaxRetries != 3 {
		t.Errorf("Expected inherited MaxRetries=3, got %d", result.MaxRetries)
	}
}

func TestResolveRetryConfig_StepOverridesPipeline(t *testing.T) {
	stepConfig := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled":    true,
			"maxRetries": float64(5),
			"delayMs":    float64(500),
		},
	}
	pipelineDefault := &RetryConfig{MaxRetries: 3, DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 60000}

	result := ResolveRetryConfig(stepConfig, pipelineDefault)
	if result == nil {
		t.Fatal("Expected step override")
	}
	if result.MaxRetries != 5 {
		t.Errorf("Expected step override MaxRetries=5, got %d", result.MaxRetries)
	}
	if result.DelayMs != 500 {
		t.Errorf("Expected step override DelayMs=500, got %d", result.DelayMs)
	}
}

func TestResolveRetryConfig_StepOptOut(t *testing.T) {
	stepConfig := map[string]interface{}{
		"retry": map[string]interface{}{
			"enabled": false,
		},
	}
	pipelineDefault := &RetryConfig{MaxRetries: 3, DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 60000}

	result := ResolveRetryConfig(stepConfig, pipelineDefault)
	if result != nil {
		t.Fatalf("Expected nil (step opt-out), got %+v", result)
	}
}

func TestResolveRetryConfig_NoPipelineDefault_NoStepConfig(t *testing.T) {
	stepConfig := map[string]interface{}{}
	result := ResolveRetryConfig(stepConfig, nil)
	if result != nil {
		t.Fatalf("Expected nil when no defaults and no step config, got %+v", result)
	}
}

func TestResolveErrorHandlingConfig_InheritsPipelineDefault(t *testing.T) {
	stepConfig := map[string]interface{}{} // No EH config
	pipelineDefault := &ErrorHandlingConfig{OnError: "catch"}

	result := ResolveErrorHandlingConfig(stepConfig, pipelineDefault)
	if result == nil {
		t.Fatal("Expected pipeline default to be inherited")
	}
	if result.OnError != "catch" {
		t.Errorf("Expected inherited OnError=catch, got %s", result.OnError)
	}
}

func TestResolveErrorHandlingConfig_StepOverride(t *testing.T) {
	stepConfig := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": true,
			"onError": "rethrow",
		},
	}
	pipelineDefault := &ErrorHandlingConfig{OnError: "catch"}

	result := ResolveErrorHandlingConfig(stepConfig, pipelineDefault)
	if result == nil {
		t.Fatal("Expected step override")
	}
	if result.OnError != "rethrow" {
		t.Errorf("Expected step override OnError=rethrow, got %s", result.OnError)
	}
}

func TestResolveErrorHandlingConfig_StepOptOut(t *testing.T) {
	stepConfig := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": false,
		},
	}
	pipelineDefault := &ErrorHandlingConfig{OnError: "catch"}

	result := ResolveErrorHandlingConfig(stepConfig, pipelineDefault)
	if result != nil {
		t.Fatalf("Expected nil (step opt-out), got %+v", result)
	}
}

// ===============================================================
// APPLY ERROR HANDLING TESTS
// ===============================================================

func TestApplyErrorHandling_CatchClearsError(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "catch"}
	input := map[string]interface{}{"data": "test"}
	output := map[string]interface{}{"data": "test"}

	resultOutput, resultErr := ApplyErrorHandling(config, errors.New("step failed"), output, input, "TestStep")

	if resultErr != nil {
		t.Fatalf("Expected nil error after catch, got: %v", resultErr)
	}
	if resultOutput["_lastError"] != "step failed" {
		t.Errorf("Expected _lastError to be set")
	}
	if resultOutput["_lastErrorStep"] != "TestStep" {
		t.Errorf("Expected _lastErrorStep=TestStep")
	}
}

func TestApplyErrorHandling_SuppressClearsError(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "suppress"}
	output := map[string]interface{}{}

	_, resultErr := ApplyErrorHandling(config, errors.New("err"), output, nil, "Step1")
	if resultErr != nil {
		t.Fatalf("Expected nil error after suppress, got: %v", resultErr)
	}
}

func TestApplyErrorHandling_RethrowKeepsError(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "rethrow"}
	output := map[string]interface{}{}
	originalErr := errors.New("critical failure")

	_, resultErr := ApplyErrorHandling(config, originalErr, output, nil, "Step1")
	if resultErr == nil {
		t.Fatal("Expected error to be rethrown")
	}
	if resultErr.Error() != "critical failure" {
		t.Errorf("Expected original error, got: %s", resultErr.Error())
	}
}

func TestApplyErrorHandling_DefaultValueApplied(t *testing.T) {
	config := &ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "FALLBACK",
	}
	output := map[string]interface{}{}
	input := map[string]interface{}{}

	resultOutput, _ := ApplyErrorHandling(config, errors.New("err"), output, input, "Step1")
	if resultOutput["status"] != "FALLBACK" {
		t.Errorf("Expected default value applied, got: %v", resultOutput["status"])
	}
}

func TestApplyErrorHandling_NilOutput_CopiesInput(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "catch"}
	input := map[string]interface{}{"key": "value"}

	resultOutput, _ := ApplyErrorHandling(config, errors.New("err"), nil, input, "Step1")
	if resultOutput["key"] != "value" {
		t.Errorf("Expected input to be copied to output, got: %v", resultOutput["key"])
	}
}

func TestApplyErrorHandling_NilConfig_ReturnsOriginal(t *testing.T) {
	output := map[string]interface{}{"data": "test"}
	err := errors.New("err")

	resultOutput, resultErr := ApplyErrorHandling(nil, err, output, nil, "Step1")
	if resultErr == nil {
		t.Fatal("Expected original error when config is nil")
	}
	if resultOutput["data"] != "test" {
		t.Errorf("Expected original output when config is nil")
	}
}

func TestApplyErrorHandling_NilError_NoOp(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "catch"}
	output := map[string]interface{}{"data": "test"}

	resultOutput, resultErr := ApplyErrorHandling(config, nil, output, nil, "Step1")
	if resultErr != nil {
		t.Fatal("Expected nil error to pass through")
	}
	if resultOutput["_lastError"] != nil {
		t.Errorf("Expected no _lastError when error is nil")
	}
}

func TestApplyErrorHandling_UnknownStrategy_DefaultsToCatch(t *testing.T) {
	config := &ErrorHandlingConfig{OnError: "invalid_strategy"}
	output := map[string]interface{}{}

	_, resultErr := ApplyErrorHandling(config, errors.New("err"), output, nil, "Step1")
	if resultErr != nil {
		t.Fatalf("Expected unknown strategy to default to catch, got error: %v", resultErr)
	}
}
