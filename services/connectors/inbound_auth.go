// services/connectors/inbound_auth.go
//
// InboundAuthenticator — the one implementation of "authenticate an inbound
// HTTP caller against credentials an engineer configured", shared by every
// HTTP surface that accepts data from an outside system:
//
//   - http_rest_inbound (its own listener port)
//   - the synchronous partner endpoints on the main API (/api/eligibility,
//     /api/claim-status, /api/prior-auth, /api/pharmacy-claim), enforced per
//     interface by controllers/sync_endpoint_auth.go
//
// Extracted from http_rest_inbound.go's original checkAuth so both use the
// same constant-time comparisons and the same config vocabulary:
//
//	authentication_type  "none" | "api_key" | "basic_auth" | "bearer_token"
//	api_key_header       header the caller sends the key in (default "X-API-Key")
//	api_key              expected API key            (api_key)
//	username / password  expected Basic credentials  (basic_auth)
//	bearer_token         expected bearer token       (bearer_token)
package connectors

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

// Inbound authentication types.
const (
	InboundAuthNone        = "none"
	InboundAuthAPIKey      = "api_key"
	InboundAuthBasic       = "basic_auth"
	InboundAuthBearerToken = "bearer_token"

	defaultAPIKeyHeader = "X-API-Key"
)

// InboundAuthenticator verifies an inbound request's credentials.
type InboundAuthenticator struct {
	authType     string
	apiKeyHeader string
	apiKey       string
	username     string
	password     string
	bearerToken  string
}

// NewInboundAuthenticator parses an authenticator from a config map using the
// vocabulary above. defaultType applies when authentication_type is absent.
// It validates the type name only; call Validate to require the matching
// credential to be present.
func NewInboundAuthenticator(cfg map[string]interface{}, defaultType string) (*InboundAuthenticator, error) {
	get := func(key string) string {
		if v, ok := cfg[key].(string); ok {
			return v
		}
		return ""
	}
	a := &InboundAuthenticator{
		authType:     defaultType,
		apiKeyHeader: defaultAPIKeyHeader,
		apiKey:       get("api_key"),
		username:     get("username"),
		password:     get("password"),
		bearerToken:  get("bearer_token"),
	}
	if v := strings.TrimSpace(get("authentication_type")); v != "" {
		a.authType = v
	}
	if v := strings.TrimSpace(get("api_key_header")); v != "" {
		a.apiKeyHeader = v
	}
	switch a.authType {
	case InboundAuthNone, InboundAuthAPIKey, InboundAuthBasic, InboundAuthBearerToken:
	default:
		return nil, fmt.Errorf("authentication_type must be 'none', 'api_key', 'basic_auth', or 'bearer_token', got %q", a.authType)
	}
	return a, nil
}

// Type returns the configured authentication type.
func (a *InboundAuthenticator) Type() string { return a.authType }

// Validate requires the credential matching the configured type to be set.
func (a *InboundAuthenticator) Validate() error {
	switch a.authType {
	case InboundAuthAPIKey:
		if a.apiKey == "" {
			return fmt.Errorf("api_key is required when authentication_type is 'api_key'")
		}
	case InboundAuthBasic:
		if a.username == "" || a.password == "" {
			return fmt.Errorf("username and password are required when authentication_type is 'basic_auth'")
		}
	case InboundAuthBearerToken:
		if a.bearerToken == "" {
			return fmt.Errorf("bearer_token is required when authentication_type is 'bearer_token'")
		}
	}
	return nil
}

// Authenticate reports whether r carries the configured credentials. An empty
// configured secret never matches (an absent header must not equal an unset
// key), regardless of whether Validate was called.
func (a *InboundAuthenticator) Authenticate(r *http.Request) bool {
	switch a.authType {
	case InboundAuthNone:
		return true
	case InboundAuthAPIKey:
		return secretEquals(r.Header.Get(a.apiKeyHeader), a.apiKey)
	case InboundAuthBasic:
		user, pass, ok := r.BasicAuth()
		if !ok {
			return false
		}
		// Evaluate both comparisons so timing does not reveal which one failed.
		userOK := secretEquals(user, a.username)
		passOK := secretEquals(pass, a.password)
		return userOK && passOK
	case InboundAuthBearerToken:
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return false
		}
		return secretEquals(auth[len("Bearer "):], a.bearerToken)
	default:
		return false
	}
}

// secretEquals is a constant-time comparison that never matches an empty expected value.
func secretEquals(got, expected string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
