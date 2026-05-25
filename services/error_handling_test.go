package services

import (
	"context"
	"ezhealthkonnect/services/executors"
	"testing"
)

// ===============================================================
// ERROR HANDLING BACKEND TESTS
// ===============================================================
// Tests the error handling logic using the reusable executors.ErrorHandlingConfig,
// executors.ParseErrorHandlingConfig, and executors.ApplyErrorHandling utilities.

// --- Test: ErrorHandlingConfig parsing ---

func TestParseErrorHandlingConfig_Enabled(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled":      true,
			"onError":      "catch",
			"defaultField": "patient_status",
			"defaultValue": "unknown",
		},
	}

	eh := executors.ParseErrorHandlingConfig(config)
	if eh == nil {
		t.Fatal("expected non-nil config")
	}
	if eh.OnError != "catch" {
		t.Errorf("expected onError='catch', got '%s'", eh.OnError)
	}
	if eh.DefaultField != "patient_status" {
		t.Errorf("expected defaultField='patient_status', got '%s'", eh.DefaultField)
	}
	if eh.DefaultValue != "unknown" {
		t.Errorf("expected defaultValue='unknown', got '%s'", eh.DefaultValue)
	}
}

func TestParseErrorHandlingConfig_Disabled(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": false,
			"onError": "catch",
		},
	}

	eh := executors.ParseErrorHandlingConfig(config)
	if eh != nil {
		t.Errorf("expected nil when disabled, got %+v", eh)
	}
}

func TestParseErrorHandlingConfig_Missing(t *testing.T) {
	config := map[string]interface{}{
		"someOtherConfig": "value",
	}

	eh := executors.ParseErrorHandlingConfig(config)
	if eh != nil {
		t.Errorf("expected nil when missing, got %+v", eh)
	}
}

func TestParseErrorHandlingConfig_DefaultsToOnErrorCatch(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": true,
			// No onError specified — should default to "catch"
		},
	}

	eh := executors.ParseErrorHandlingConfig(config)
	if eh == nil {
		t.Fatal("expected non-nil config")
	}
	if eh.OnError != "catch" {
		t.Errorf("expected default onError='catch', got '%s'", eh.OnError)
	}
}

func TestParseErrorHandlingConfig_NoDefaultValue(t *testing.T) {
	config := map[string]interface{}{
		"errorHandling": map[string]interface{}{
			"enabled": true,
			"onError": "suppress",
			// No defaultField or defaultValue
		},
	}

	eh := executors.ParseErrorHandlingConfig(config)
	if eh == nil {
		t.Fatal("expected non-nil config")
	}
	if eh.DefaultField != "" {
		t.Error("defaultField should be empty when not provided")
	}
	if eh.DefaultValue != "" {
		t.Error("defaultValue should be empty when not provided")
	}
}

// --- Test: Error handling behavior using ApplyErrorHandling ---

func TestErrorHandling_CatchClearsError(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "catch"}
	inputMsg := map[string]interface{}{"data": "test"}

	output, err := executors.ApplyErrorHandling(eh, &testError{"step failed"}, nil, inputMsg, "test_step")

	if err != nil {
		t.Error("catch should clear the error")
	}
	if output["_lastError"] != "step failed" {
		t.Error("_lastError should be set even when caught")
	}
	if output["_lastErrorStep"] != "test_step" {
		t.Error("_lastErrorStep should be set")
	}
}

func TestErrorHandling_SuppressClearsError(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "suppress"}

	output, err := executors.ApplyErrorHandling(eh, &testError{"step failed"}, nil, map[string]interface{}{}, "Step1")

	if err != nil {
		t.Error("suppress should clear the error")
	}
	if output["_lastError"] != "step failed" {
		t.Error("_lastError should be set")
	}
}

func TestErrorHandling_RethrowKeepsError(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "rethrow"}

	output, err := executors.ApplyErrorHandling(eh, &testError{"step failed"}, nil, map[string]interface{}{}, "Step1")

	if err == nil {
		t.Error("rethrow should keep the error")
	}
	if err.Error() != "step failed" {
		t.Errorf("expected 'step failed', got '%s'", err.Error())
	}
	if output["_lastError"] != "step failed" {
		t.Error("_lastError should still be set on rethrow")
	}
}

func TestErrorHandling_DefaultValueApplied(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "fallback",
	}
	output, err := executors.ApplyErrorHandling(
		eh,
		&testError{"step failed"},
		map[string]interface{}{"status": "active", "name": "test"},
		nil,
		"Step1",
	)

	if err != nil {
		t.Error("catch should clear the error")
	}
	if output["status"] != "fallback" {
		t.Errorf("expected status='fallback', got '%v'", output["status"])
	}
	if output["name"] != "test" {
		t.Error("other fields should not be modified")
	}
}

func TestErrorHandling_DefaultValueOnlyWhenBothProvided(t *testing.T) {
	// Only field, no value
	eh := &executors.ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "",
	}
	output, _ := executors.ApplyErrorHandling(
		eh,
		&testError{"step failed"},
		map[string]interface{}{"status": "active"},
		nil,
		"Step1",
	)

	if output["status"] != "active" {
		t.Error("status should not change when defaultValue is empty")
	}
}

func TestErrorHandling_NilOutputCopiesInputMessage(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "catch"}
	inputMsg := map[string]interface{}{
		"patient": "John",
		"age":     30,
	}
	output, _ := executors.ApplyErrorHandling(eh, &testError{"step failed"}, nil, inputMsg, "Step1")

	if output["patient"] != "John" {
		t.Error("output should contain copied input data")
	}
	if output["age"] != 30 {
		t.Error("output should contain all input fields")
	}
}

func TestErrorHandling_NilConfig_LeavesErrorIntact(t *testing.T) {
	output, err := executors.ApplyErrorHandling(
		nil,
		&testError{"step failed"},
		map[string]interface{}{"data": "test"},
		nil,
		"Step1",
	)

	if err == nil {
		t.Error("error should not be cleared without a handler")
	}
	if output["_lastError"] != nil {
		t.Error("_lastError should not be set without a handler")
	}
}

func TestErrorHandling_NilError_NoOp(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "fallback",
	}
	output, err := executors.ApplyErrorHandling(
		eh,
		nil, // No error
		map[string]interface{}{"status": "active"},
		nil,
		"Step1",
	)

	if err != nil {
		t.Error("should have no error")
	}
	if output["status"] != "active" {
		t.Error("status should not change when there's no error")
	}
	if output["_lastError"] != nil {
		t.Error("_lastError should not be set when no error occurred")
	}
}

// --- Test: Retry + Error Handling integration ---

func TestRetryThenErrorHandling_CatchAfterRetryExhausted(t *testing.T) {
	retryConfig := &executors.RetryConfig{
		MaxRetries:        2,
		DelayMs:           1,
		BackoffMultiplier: 1,
		MaxDelayMs:        100,
	}

	attempts := 0
	retryResult := executors.ExecuteWithRetry(context.Background(), retryConfig, func(attempt int) (map[string]interface{}, error) {
		attempts++
		return nil, &testError{"always fails"}
	})

	if retryResult.Err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if retryResult.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", retryResult.Attempts)
	}

	// Now apply error handling
	eh := &executors.ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "error_fallback",
	}
	output, err := executors.ApplyErrorHandling(
		eh,
		retryResult.Err,
		retryResult.Output,
		map[string]interface{}{"status": "pending"},
		"Step1",
	)

	if err != nil {
		t.Error("catch should clear the error after retry exhaustion")
	}
	if output["status"] != "error_fallback" {
		t.Errorf("expected default value 'error_fallback', got '%v'", output["status"])
	}
	if output["_lastError"] != "always fails" {
		t.Error("_lastError should record the last error")
	}
}

func TestRetryThenErrorHandling_RethrowAfterRetryExhausted(t *testing.T) {
	retryConfig := &executors.RetryConfig{
		MaxRetries:        1,
		DelayMs:           1,
		BackoffMultiplier: 1,
		MaxDelayMs:        100,
	}

	retryResult := executors.ExecuteWithRetry(context.Background(), retryConfig, func(attempt int) (map[string]interface{}, error) {
		return nil, &testError{"critical failure"}
	})

	eh := &executors.ErrorHandlingConfig{OnError: "rethrow"}
	_, err := executors.ApplyErrorHandling(
		eh,
		retryResult.Err,
		retryResult.Output,
		map[string]interface{}{},
		"Step1",
	)

	if err == nil {
		t.Error("rethrow should keep the error even after retry")
	}
	if err.Error() != "critical failure" {
		t.Errorf("expected 'critical failure', got '%s'", err.Error())
	}
}

func TestRetrySucceeds_NoErrorHandlingTriggered(t *testing.T) {
	retryConfig := &executors.RetryConfig{
		MaxRetries:        3,
		DelayMs:           1,
		BackoffMultiplier: 1,
		MaxDelayMs:        100,
	}

	callCount := 0
	retryResult := executors.ExecuteWithRetry(context.Background(), retryConfig, func(attempt int) (map[string]interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, &testError{"temporary failure"}
		}
		return map[string]interface{}{"result": "success"}, nil
	})

	if retryResult.Err != nil {
		t.Error("should succeed on third attempt")
	}

	// Error handling should not be invoked since retry succeeded
	eh := &executors.ErrorHandlingConfig{
		OnError:      "catch",
		DefaultField: "status",
		DefaultValue: "fallback",
	}
	output, err := executors.ApplyErrorHandling(
		eh,
		retryResult.Err, // nil — no error
		retryResult.Output,
		nil,
		"Step1",
	)

	if err != nil {
		t.Error("no error expected")
	}
	if output["result"] != "success" {
		t.Error("output should have the successful result")
	}
	if output["_lastError"] != nil {
		t.Error("_lastError should not be set when retry succeeds")
	}
}

// --- Test: Default value with different field path formats ---

func TestDefaultValue_SimpleJSONPath(t *testing.T) {
	output := map[string]interface{}{
		"patient_status": "active",
	}
	executors.UpdateFieldValue(output, "patient_status", "unknown")
	if output["patient_status"] != "unknown" {
		t.Errorf("expected 'unknown', got '%v'", output["patient_status"])
	}
}

func TestDefaultValue_NestedJSONPath(t *testing.T) {
	output := map[string]interface{}{
		"data": map[string]interface{}{
			"patient": map[string]interface{}{
				"status": "active",
			},
		},
	}
	executors.UpdateFieldValue(output, "data.patient.status", "discharged")
	data := output["data"].(map[string]interface{})
	patient := data["patient"].(map[string]interface{})
	if patient["status"] != "discharged" {
		t.Errorf("expected 'discharged', got '%v'", patient["status"])
	}
}

// --- Test: Unknown onError value behavior ---

func TestErrorHandling_UnknownOnErrorDefaultsToCatch(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "invalid_strategy"}
	_, err := executors.ApplyErrorHandling(
		eh,
		&testError{"step failed"},
		map[string]interface{}{},
		nil,
		"Step1",
	)

	// ApplyErrorHandling treats unknown strategy as catch (safe default)
	if err != nil {
		t.Error("unknown onError strategy should default to catch (clear error)")
	}
}

// --- Test: _lastError and _lastErrorStep propagation ---

func TestErrorInfo_PropagatedToDownstreamSteps(t *testing.T) {
	eh := &executors.ErrorHandlingConfig{OnError: "catch"}
	output, _ := executors.ApplyErrorHandling(
		eh,
		&testError{"validation failed"},
		map[string]interface{}{},
		nil,
		"test_step",
	)

	lastError, ok := output["_lastError"].(string)
	if !ok || lastError != "validation failed" {
		t.Error("downstream step should be able to read _lastError")
	}
	lastStep, ok := output["_lastErrorStep"].(string)
	if !ok || lastStep != "test_step" {
		t.Error("downstream step should be able to read _lastErrorStep")
	}
}

// --- Helper: test error type ---

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
