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
// UNIT TESTS FOR OAUTH 2.0 SERVICE
// ===============================================================

func TestOAuth2Service_ClientCredentials(t *testing.T) {
	// Mock OAuth 2.0 server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Verify Basic Auth
		username, password, ok := r.BasicAuth()
		if !ok || username != "test-client" || password != "test-secret" {
			t.Error("Invalid client credentials")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse form: %v", err)
		}

		// Verify grant type
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("Expected grant_type=client_credentials, got %s", r.FormValue("grant_type"))
		}

		// Return token response
		response := map[string]interface{}{
			"access_token": "test-access-token-12345",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create OAuth2 service
	httpService := NewHTTPClientService(5 * time.Second)
	oauth2Service := NewOAuth2Service(httpService)

	// Configure OAuth2
	config := &OAuth2Config{
		TokenURL:     mockServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypeClientCredentials,
		Scope:        "read write",
	}

	// Get token
	token, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// Verify token
	if token.AccessToken != "test-access-token-12345" {
		t.Errorf("Expected access token test-access-token-12345, got %s", token.AccessToken)
	}

	if token.TokenType != "Bearer" {
		t.Errorf("Expected token type Bearer, got %s", token.TokenType)
	}

	if token.ExpiresIn != 3600 {
		t.Errorf("Expected expires_in 3600, got %d", token.ExpiresIn)
	}

	if token.Scope != "read write" {
		t.Errorf("Expected scope 'read write', got %s", token.Scope)
	}
}

func TestOAuth2Service_TokenCaching(t *testing.T) {
	requestCount := 0

	// Mock OAuth 2.0 server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		response := map[string]interface{}{
			"access_token": "cached-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create OAuth2 service
	httpService := NewHTTPClientService(5 * time.Second)
	oauth2Service := NewOAuth2Service(httpService)

	config := &OAuth2Config{
		TokenURL:     mockServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypeClientCredentials,
	}

	// First request - should call server
	token1, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("First GetToken failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 server request, got %d", requestCount)
	}

	// Second request - should use cache
	token2, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("Expected cache to be used (1 request), got %d requests", requestCount)
	}

	// Verify same token
	if token1.AccessToken != token2.AccessToken {
		t.Error("Expected cached token to be returned")
	}
}

func TestOAuth2Service_TokenExpiration(t *testing.T) {
	requestCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		response := map[string]interface{}{
			"access_token": "expiring-token",
			"token_type":   "Bearer",
			"expires_in":   1, // 1 second expiration
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	httpService := NewHTTPClientService(5 * time.Second)
	oauth2Service := NewOAuth2Service(httpService)

	config := &OAuth2Config{
		TokenURL:     mockServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypeClientCredentials,
	}

	// Get first token
	_, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount)
	}

	// Wait for token to expire (1 second + buffer)
	time.Sleep(2 * time.Second)

	// Get token again - should request new token
	_, err = oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected token refresh (2 requests), got %d", requestCount)
	}
}

func TestOAuth2Service_PasswordGrant(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse form: %v", err)
		}

		// Verify password grant
		if r.FormValue("grant_type") != "password" {
			t.Errorf("Expected grant_type=password, got %s", r.FormValue("grant_type"))
		}

		if r.FormValue("username") != "testuser" {
			t.Errorf("Expected username=testuser, got %s", r.FormValue("username"))
		}

		if r.FormValue("password") != "testpass" {
			t.Errorf("Expected password=testpass, got %s", r.FormValue("password"))
		}

		response := map[string]interface{}{
			"access_token": "password-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	httpService := NewHTTPClientService(5 * time.Second)
	oauth2Service := NewOAuth2Service(httpService)

	config := &OAuth2Config{
		TokenURL:     mockServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypePassword,
		Username:     "testuser",
		Password:     "testpass",
	}

	token, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if token.AccessToken != "password-token" {
		t.Errorf("Expected password-token, got %s", token.AccessToken)
	}
}

func TestOAuth2Service_ClearCache(t *testing.T) {
	requestCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		response := map[string]interface{}{
			"access_token": "clear-cache-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	httpService := NewHTTPClientService(5 * time.Second)
	oauth2Service := NewOAuth2Service(httpService)

	config := &OAuth2Config{
		TokenURL:     mockServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypeClientCredentials,
	}

	// Get token (1st request)
	_, err := oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// Clear cache
	oauth2Service.ClearCache()

	// Get token again (should request new token - 2nd request)
	_, err = oauth2Service.GetToken(context.Background(), config)
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests after cache clear, got %d", requestCount)
	}
}

func TestHTTPClientService_ExecuteWithOAuth2(t *testing.T) {
	// Mock OAuth 2.0 server
	mockOAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"access_token": "oauth-test-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockOAuthServer.Close()

	// Mock API server that requires OAuth
	mockAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Bearer token
		auth := r.Header.Get("Authorization")
		if auth != "Bearer oauth-test-token" {
			t.Errorf("Expected Bearer oauth-test-token, got %s", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		response := map[string]interface{}{
			"data": "protected resource",
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer mockAPIServer.Close()

	// Create HTTP service
	httpService := NewHTTPClientService(5 * time.Second)

	// Configure OAuth 2.0
	oauth2Config := &OAuth2Config{
		TokenURL:     mockOAuthServer.URL + "/oauth/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    GrantTypeClientCredentials,
	}

	// Configure API request
	requestConfig := &RequestConfig{
		Method: "GET",
		URL:    mockAPIServer.URL + "/api/data",
	}

	// Execute with OAuth 2.0
	resp, err := httpService.ExecuteWithOAuth2(context.Background(), requestConfig, oauth2Config)
	if err != nil {
		t.Fatalf("ExecuteWithOAuth2 failed: %v", err)
	}

	// Verify response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	jsonResp := resp.JSON.(map[string]interface{})
	if jsonResp["data"] != "protected resource" {
		t.Error("Failed to access protected resource")
	}
}
