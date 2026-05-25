package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===============================================================
// UNIT TESTS FOR HTTP CLIENT SERVICE
// ===============================================================

func TestHTTPClientService_BasicGET(t *testing.T) {
	// Setup mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		response := map[string]interface{}{
			"status": "success",
			"data":   "test",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create service
	service := NewHTTPClientService(5 * time.Second)

	// Execute request
	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/test",
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	jsonResp, ok := resp.JSON.(map[string]interface{})
	if !ok {
		t.Fatal("Expected JSON response")
	}

	if jsonResp["status"] != "success" {
		t.Errorf("Expected status=success, got %v", jsonResp["status"])
	}
}

func TestHTTPClientService_BasicAuth(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expectedAuth := "Basic dGVzdHVzZXI6dGVzdHBhc3M=" // testuser:testpass

		if auth != expectedAuth {
			t.Errorf("Expected auth %s, got %s", expectedAuth, auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"authenticated": true})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/secure",
	}

	auth := &AuthConfig{
		Type:     AuthTypeBasic,
		Username: "testuser",
		Password: "testpass",
	}

	resp, err := service.Execute(context.Background(), config, auth)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["authenticated"] != true {
		t.Error("Expected authenticated=true")
	}
}

func TestHTTPClientService_BearerToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expectedAuth := "Bearer test-token-123"

		if auth != expectedAuth {
			t.Errorf("Expected auth %s, got %s", expectedAuth, auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"valid": true})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/api",
	}

	auth := &AuthConfig{
		Type:        AuthTypeBearer,
		BearerToken: "test-token-123",
	}

	resp, err := service.Execute(context.Background(), config, auth)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["valid"] != true {
		t.Error("Expected valid=true")
	}
}

func TestHTTPClientService_APIKey(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")

		if apiKey != "test-key-456" {
			t.Errorf("Expected API key test-key-456, got %s", apiKey)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"valid": true})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/api",
	}

	auth := &AuthConfig{
		Type:   AuthTypeAPIKey,
		APIKey: "test-key-456",
	}

	resp, err := service.Execute(context.Background(), config, auth)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["valid"] != true {
		t.Error("Expected valid=true")
	}
}

func TestHTTPClientService_CustomAPIKeyHeader(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Custom-Key")

		if apiKey != "custom-key" {
			t.Errorf("Expected custom key in X-Custom-Key header, got %s", apiKey)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"valid": true})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/api",
	}

	auth := &AuthConfig{
		Type:         AuthTypeAPIKey,
		APIKey:       "custom-key",
		APIKeyHeader: "X-Custom-Key",
	}

	resp, err := service.Execute(context.Background(), config, auth)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["valid"] != true {
		t.Error("Expected valid=true")
	}
}

func TestHTTPClientService_POST_WithBody(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json")
		}

		var requestBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&requestBody)

		if requestBody["query"] != "test" {
			t.Errorf("Expected query=test, got %v", requestBody["query"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"result": "success"})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "POST",
		URL:    mockServer.URL + "/search",
		Body: map[string]interface{}{
			"query": "test",
			"limit": 10,
		},
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["result"] != "success" {
		t.Error("Expected result=success")
	}
}

func TestHTTPClientService_CustomHeaders(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "test-value" {
			t.Error("Expected X-Custom-Header=test-value")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("Expected Accept=application/json")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"headers": "verified"})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/test",
		Headers: map[string]string{
			"X-Custom-Header": "test-value",
			"Accept":          "application/json",
		},
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["headers"] != "verified" {
		t.Error("Headers not verified")
	}
}

func TestHTTPClientService_QueryParams(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("param1") != "value1" {
			t.Errorf("Expected param1=value1, got %s", query.Get("param1"))
		}
		if query.Get("param2") != "value2" {
			t.Errorf("Expected param2=value2, got %s", query.Get("param2"))
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"params": "verified"})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/test",
		QueryParams: map[string]string{
			"param1": "value1",
			"param2": "value2",
		},
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["params"] != "verified" {
		t.Error("Query params not verified")
	}
}

func TestHTTPClientService_RetryOnFailure(t *testing.T) {
	attemptCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		// Fail first 2 attempts
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "attempts": attemptCount})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method:     "GET",
		URL:        mockServer.URL + "/flaky",
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["success"] != true {
		t.Error("Expected success=true")
	}
}

func TestHTTPClientService_Timeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than timeout
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{"delayed": true})
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method:  "GET",
		URL:     mockServer.URL + "/slow",
		Timeout: 100 * time.Millisecond,
	}

	_, err := service.Execute(context.Background(), config, nil)
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}

func TestHTTPClientService_NonJSONResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Plain text response"))
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method: "GET",
		URL:    mockServer.URL + "/text",
	}

	resp, err := service.Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Non-JSON response should be returned as string
	if resp.JSON != "Plain text response" {
		t.Errorf("Expected plain text response, got %v", resp.JSON)
	}
}

func TestHTTPClientService_ErrorResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
	}))
	defer mockServer.Close()

	service := NewHTTPClientService(5 * time.Second)

	config := &RequestConfig{
		Method:     "GET",
		URL:        mockServer.URL + "/missing",
		RetryCount: 0, // No retries
	}

	_, err := service.Execute(context.Background(), config, nil)
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
}
