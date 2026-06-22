package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===============================================================
// UNIT TESTS FOR API ENRICHMENT EXECUTOR
// ===============================================================

func TestAPIEnrichmentExecutor_BasicGET(t *testing.T) {
	// Setup mock API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Return mock patient data
		response := map[string]interface{}{
			"patientId": "12345",
			"name":      "John Doe",
			"dob":       "1985-06-15",
			"status":    "active",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create executor
	executor := NewAPIEnrichmentExecutor()

	// Create test step configuration
	step := &models.TransformationStep{
		StepName: "Test API Enrichment",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":   mockServer.URL + "/patients/12345",
			"method":     "GET",
			"targetPath": "enriched.patient",
		},
	}

	// Create input data
	inputData := map[string]interface{}{
		"messageType": "ADT^A01",
	}

	// Execute enrichment
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.patient
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput field not found or not a map")
	}

	// Verify patient data (flat in step output)
	if stepOutput["patientId"] != "12345" {
		t.Errorf("Expected patientId 12345, got %v", stepOutput["patientId"])
	}
	if stepOutput["name"] != "John Doe" {
		t.Errorf("Expected name John Doe, got %v", stepOutput["name"])
	}
}

func TestAPIEnrichmentExecutor_FieldMapping(t *testing.T) {
	// Setup mock API server that expects patient ID in URL
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract patient ID from URL path
		expectedPath := "/patients/MRN-9999"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		response := map[string]interface{}{
			"found": true,
			"mrn":   "MRN-9999",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Patient Lookup",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint": mockServer.URL + "/patients/{patientId}",
			"method":   "GET",
			"fieldMappings": map[string]interface{}{
				"patientId": "enhancedSegments.PID.fields[2].value",
			},
			"targetPath": "enriched.lookup",
		},
	}

	// Input data with nested patient ID
	inputData := map[string]interface{}{
		"enhancedSegments": map[string]interface{}{
			"PID": map[string]interface{}{
				"fields": []interface{}{
					map[string]interface{}{"value": "1"},
					map[string]interface{}{"value": "2"},
					map[string]interface{}{"value": "MRN-9999"}, // Index 2
				},
			},
		},
	}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.lookup
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["found"] != true {
		t.Error("Expected found=true in lookup result")
	}
}

func TestAPIEnrichmentExecutor_BasicAuth(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Basic auth header
		auth := r.Header.Get("Authorization")
		expectedAuth := "Basic dGVzdHVzZXI6dGVzdHBhc3M=" // testuser:testpass in base64

		if auth != expectedAuth {
			t.Errorf("Expected auth %s, got %s", expectedAuth, auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		response := map[string]interface{}{"authenticated": true}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Authenticated API Call",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":   mockServer.URL + "/secure",
			"method":     "GET",
			"authType":   "basic",
			"username":   "testuser",
			"password":   "testpass",
			"targetPath": "enriched.auth",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.auth
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["authenticated"] != true {
		t.Error("Expected authenticated=true")
	}
}

func TestAPIEnrichmentExecutor_BearerToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expectedAuth := "Bearer test-token-123"

		if auth != expectedAuth {
			t.Errorf("Expected auth %s, got %s", expectedAuth, auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		response := map[string]interface{}{"valid": true}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Bearer Token API",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":    mockServer.URL + "/api",
			"method":      "GET",
			"authType":    "bearer",
			"bearerToken": "test-token-123",
			"targetPath":  "enriched.token",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.token
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["valid"] != true {
		t.Error("Expected valid=true")
	}
}

func TestAPIEnrichmentExecutor_RetryOnFailure(t *testing.T) {
	attemptCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		// Fail first 2 attempts, succeed on 3rd
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{"success": true, "attempts": attemptCount}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Retry Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":      mockServer.URL + "/flaky",
			"method":        "GET",
			"retryCount":    3,
			"retryDelayMs":  100,
			"targetPath":    "enriched.retry",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}

	// P7: data is now in _stepOutput (flat), not enriched.retry
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["success"] != true {
		t.Error("Expected success=true after retries")
	}
}

func TestAPIEnrichmentExecutor_FailOnError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Fail Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":    mockServer.URL + "/missing",
			"method":      "GET",
			"failOnError": true,
			"targetPath":  "enriched.fail",
		},
	}

	inputData := map[string]interface{}{}

	_, err := executor.Execute(context.Background(), step, inputData)
	if err == nil {
		t.Fatal("Expected error when API fails with failOnError=true")
	}
}

func TestAPIEnrichmentExecutor_DefaultValue(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	defaultVal := map[string]interface{}{
		"status": "not_found",
		"reason": "Patient not in EMPI",
	}

	step := &models.TransformationStep{
		StepName: "Default Value Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":     mockServer.URL + "/missing",
			"method":       "GET",
			"failOnError":  false,
			"defaultValue": defaultVal,
			"targetPath":   "enriched.default",
		},
	}

	inputData := map[string]interface{}{}

	// The executor always returns the underlying error (see its own comment:
	// "Always return error — pipeline service decides retry/catch behavior").
	// defaultValue's role is to populate fallback *data* in _stepOutput for a
	// pipeline-level errorHandling config to use, not to suppress the error.
	output, err := executor.Execute(context.Background(), step, inputData)
	if err == nil {
		t.Fatal("Expected the 404 to be returned as an error")
	}

	// P7: default value is in _stepOutput["value"], not enriched.default
	stepOutput := output["_stepOutput"].(map[string]interface{})
	result := stepOutput["value"].(map[string]interface{})

	if result["status"] != "not_found" {
		t.Errorf("Expected default value, got %v", result)
	}
}

func TestAPIEnrichmentExecutor_Timeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay response longer than timeout
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{"delayed": true})
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Timeout Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":    mockServer.URL + "/slow",
			"method":      "GET",
			"timeoutMs":   100, // Timeout before server responds
			"failOnError": true,
			"targetPath":  "enriched.timeout",
		},
	}

	inputData := map[string]interface{}{}

	_, err := executor.Execute(context.Background(), step, inputData)
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}

func TestAPIEnrichmentExecutor_CustomHeaders(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify custom headers
		if r.Header.Get("X-Custom-Header") != "test-value" {
			t.Error("Expected X-Custom-Header=test-value")
		}
		if r.Header.Get("Accept") != "application/fhir+json" {
			t.Error("Expected Accept=application/fhir+json")
		}

		response := map[string]interface{}{"headers": "verified"}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Custom Headers Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint": mockServer.URL + "/headers",
			"method":   "GET",
			"headers": map[string]interface{}{
				"X-Custom-Header": "test-value",
				"Accept":          "application/fhir+json",
			},
			"targetPath": "enriched.headers",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.headers
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["headers"] != "verified" {
		t.Error("Headers not verified correctly")
	}
}

func TestAPIEnrichmentExecutor_POST_WithBody(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Parse request body
		var requestBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&requestBody)

		if requestBody["query"] != "patient_lookup" {
			t.Errorf("Expected query=patient_lookup, got %v", requestBody["query"])
		}

		response := map[string]interface{}{"result": "success"}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "POST Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint": mockServer.URL + "/search",
			"method":   "POST",
			"requestBody": map[string]interface{}{
				"query": "patient_lookup",
				"limit": 10,
			},
			"targetPath": "enriched.post",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.post
	stepOutput := output["_stepOutput"].(map[string]interface{})

	if stepOutput["result"] != "success" {
		t.Error("POST request failed")
	}
}

func TestAPIEnrichmentExecutor_ConfigValidation(t *testing.T) {
	executor := NewAPIEnrichmentExecutor()

	// Test missing endpoint
	step := &models.TransformationStep{
		StepName: "Invalid Config",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method": "GET",
		},
	}

	err := executor.Validate(step)
	if err == nil {
		t.Fatal("Expected validation error for missing endpoint")
	}

	// Test nil config
	step.Config = nil
	err = executor.Validate(step)
	if err == nil {
		t.Fatal("Expected validation error for nil config")
	}
}
