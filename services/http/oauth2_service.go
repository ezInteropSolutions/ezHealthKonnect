package http

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ===============================================================
// OAUTH 2.0 SERVICE
// ===============================================================
// Handles OAuth 2.0 Client Credentials flow with token caching

// OAuth2GrantType defines OAuth 2.0 grant types
type OAuth2GrantType string

const (
	GrantTypeClientCredentials OAuth2GrantType = "client_credentials"
	GrantTypePassword          OAuth2GrantType = "password"
	GrantTypeRefreshToken      OAuth2GrantType = "refresh_token"
)

// OAuth2Config contains OAuth 2.0 configuration
type OAuth2Config struct {
	TokenURL     string          // OAuth 2.0 token endpoint
	ClientID     string          // OAuth 2.0 client ID
	ClientSecret string          // OAuth 2.0 client secret
	GrantType    OAuth2GrantType // OAuth 2.0 grant type
	Scope        string          // Optional: OAuth 2.0 scope
	Audience     string          // Optional: OAuth 2.0 audience (required by Auth0 and some providers)
	Username     string          // For password grant type
	Password     string          // For password grant type
	RefreshToken string          // For refresh token grant
}

// OAuth2Token represents an OAuth 2.0 access token
type OAuth2Token struct {
	AccessToken  string
	TokenType    string
	ExpiresIn    int       // Seconds until expiration
	RefreshToken string    // Optional refresh token
	Scope        string    // Granted scopes
	ExpiresAt    time.Time // Calculated expiration time
}

// IsExpired checks if the token is expired (with 5 minute buffer)
func (t *OAuth2Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false // Never expires if not set
	}
	// Expire 5 minutes early to avoid edge cases
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// OAuth2Service handles OAuth 2.0 token management with caching
type OAuth2Service struct {
	httpService *HTTPClientService
	tokenCache  map[string]*OAuth2Token // Cache key -> token
	mu          sync.RWMutex
}

// NewOAuth2Service creates a new OAuth 2.0 service
func NewOAuth2Service(httpService *HTTPClientService) *OAuth2Service {
	return &OAuth2Service{
		httpService: httpService,
		tokenCache:  make(map[string]*OAuth2Token),
	}
}

// GetToken retrieves a valid OAuth 2.0 token (from cache or by requesting new one)
func (o *OAuth2Service) GetToken(ctx context.Context, config *OAuth2Config) (*OAuth2Token, error) {
	// Generate cache key from config
	cacheKey := o.generateCacheKey(config)

	// Check cache for valid token
	o.mu.RLock()
	cachedToken, exists := o.tokenCache[cacheKey]
	o.mu.RUnlock()

	if exists && !cachedToken.IsExpired() {
		log.Printf("🔑 [OAuth2] Using cached token (expires in %v)", time.Until(cachedToken.ExpiresAt))
		return cachedToken, nil
	}

	// Token expired or not cached - request new token
	log.Printf("🔑 [OAuth2] Requesting new access token from %s", config.TokenURL)

	token, err := o.requestToken(ctx, config)
	if err != nil {
		return nil, err
	}

	// Cache the new token
	o.mu.Lock()
	o.tokenCache[cacheKey] = token
	o.mu.Unlock()

	log.Printf("✅ [OAuth2] Access token obtained (expires in %d seconds)", token.ExpiresIn)

	return token, nil
}

// requestToken requests a new OAuth 2.0 token
func (o *OAuth2Service) requestToken(ctx context.Context, config *OAuth2Config) (*OAuth2Token, error) {
	// Build request body based on grant type
	requestBody := map[string]string{
		"grant_type": string(config.GrantType),
	}

	// Add audience if provided (required by Auth0 and some providers)
	if config.Audience != "" {
		requestBody["audience"] = config.Audience
	}

	switch config.GrantType {
	case GrantTypeClientCredentials:
		// Client credentials flow
		if config.Scope != "" {
			requestBody["scope"] = config.Scope
		}

	case GrantTypePassword:
		// Resource owner password credentials flow
		requestBody["username"] = config.Username
		requestBody["password"] = config.Password
		if config.Scope != "" {
			requestBody["scope"] = config.Scope
		}

	case GrantTypeRefreshToken:
		// Refresh token flow
		requestBody["refresh_token"] = config.RefreshToken
		if config.Scope != "" {
			requestBody["scope"] = config.Scope
		}
	}

	// Create request configuration
	requestConfig := &RequestConfig{
		Method:     "POST",
		URL:        config.TokenURL,
		Body:       requestBody,
		Headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Timeout:    10 * time.Second,
		RetryCount: 2,
		RetryDelay: 1 * time.Second,
	}

	// Create auth config for client credentials
	authConfig := &AuthConfig{
		Type:     AuthTypeBasic,
		Username: config.ClientID,
		Password: config.ClientSecret,
	}

	// Execute token request
	resp, err := o.httpService.Execute(ctx, requestConfig, authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to request OAuth2 token: %w", err)
	}

	// Parse token response
	tokenResp, ok := resp.JSON.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid OAuth2 token response format")
	}

	// Extract token fields
	accessToken, _ := tokenResp["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("OAuth2 response missing access_token")
	}

	tokenType, _ := tokenResp["token_type"].(string)
	if tokenType == "" {
		tokenType = "Bearer" // Default to Bearer
	}

	expiresIn, _ := tokenResp["expires_in"].(float64)
	if expiresIn == 0 {
		expiresIn = 3600 // Default to 1 hour
	}

	refreshToken, _ := tokenResp["refresh_token"].(string)
	scope, _ := tokenResp["scope"].(string)

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	return &OAuth2Token{
		AccessToken:  accessToken,
		TokenType:    tokenType,
		ExpiresIn:    int(expiresIn),
		RefreshToken: refreshToken,
		Scope:        scope,
		ExpiresAt:    expiresAt,
	}, nil
}

// generateCacheKey generates a unique cache key for OAuth2 config
func (o *OAuth2Service) generateCacheKey(config *OAuth2Config) string {
	// Simple cache key - could be enhanced with hashing for security
	return fmt.Sprintf("%s|%s|%s|%s",
		config.TokenURL,
		config.ClientID,
		config.GrantType,
		config.Scope,
	)
}

// ClearCache clears all cached tokens
func (o *OAuth2Service) ClearCache() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.tokenCache = make(map[string]*OAuth2Token)
	log.Printf("🔑 [OAuth2] Token cache cleared")
}

// ClearToken removes a specific cached token
func (o *OAuth2Service) ClearToken(config *OAuth2Config) {
	o.mu.Lock()
	defer o.mu.Unlock()

	cacheKey := o.generateCacheKey(config)
	delete(o.tokenCache, cacheKey)
	log.Printf("🔑 [OAuth2] Cleared cached token for %s", cacheKey)
}

// ===============================================================
// OAUTH 2.0 INTEGRATION WITH HTTP CLIENT SERVICE
// ===============================================================

// ExecuteWithOAuth2 executes an HTTP request with automatic OAuth 2.0 token handling
func (h *HTTPClientService) ExecuteWithOAuth2(
	ctx context.Context,
	config *RequestConfig,
	oauth2Config *OAuth2Config,
) (*Response, error) {
	// Create OAuth2 service if needed
	oauth2Service := NewOAuth2Service(h)

	// Get valid OAuth 2.0 token
	token, err := oauth2Service.GetToken(ctx, oauth2Config)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain OAuth2 token: %w", err)
	}

	// Create auth config with Bearer token
	authConfig := &AuthConfig{
		Type:        AuthTypeBearer,
		BearerToken: token.AccessToken,
	}

	// Execute request with OAuth 2.0 token
	return h.Execute(ctx, config, authConfig)
}

// GetOAuth2Token retrieves an OAuth 2.0 token (useful for debugging/testing)
func (h *HTTPClientService) GetOAuth2Token(
	ctx context.Context,
	oauth2Config *OAuth2Config,
) (*OAuth2Token, error) {
	oauth2Service := NewOAuth2Service(h)
	return oauth2Service.GetToken(ctx, oauth2Config)
}
