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
//	authentication_type  "none" | "api_key" | "basic_auth" | "bearer_token" | "standard_webhooks"
//	api_key_header       header the caller sends the key in (default "X-API-Key")
//	api_key              expected API key            (api_key)
//	username / password  expected Basic credentials  (basic_auth)
//	bearer_token         expected bearer token       (bearer_token)
//	webhook_secret       "whsec_..." signing secret  (standard_webhooks)
//	webhook_tolerance_seconds  accepted signing-time skew (standard_webhooks, default 300)
//
// standard_webhooks signs the request body (standard_webhooks.go), so it is
// checked with AuthenticateRequest, which is given the body; Authenticate,
// which only sees headers, refuses it.
package connectors

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Inbound authentication types.
const (
	InboundAuthNone        = "none"
	InboundAuthAPIKey      = "api_key"
	InboundAuthBasic       = "basic_auth"
	InboundAuthBearerToken = "bearer_token"
	InboundAuthStandardWebhooks = "standard_webhooks"

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

	webhookSecret    string
	webhookTolerance time.Duration
	webhook          *standardWebhookVerifier // set when the secret parses
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
		webhookSecret: get("webhook_secret"),
	}
	if secs, ok := cfg["webhook_tolerance_seconds"].(float64); ok && secs > 0 {
		a.webhookTolerance = time.Duration(secs) * time.Second
	}
	if v := strings.TrimSpace(get("authentication_type")); v != "" {
		a.authType = v
	}
	if v := strings.TrimSpace(get("api_key_header")); v != "" {
		a.apiKeyHeader = v
	}
	switch a.authType {
	case InboundAuthNone, InboundAuthAPIKey, InboundAuthBasic, InboundAuthBearerToken:
	case InboundAuthStandardWebhooks:
		// A bad or missing secret is reported by Validate; until then the
		// verifier stays nil and every request is refused.
		if v, err := newStandardWebhookVerifier(a.webhookSecret, a.webhookTolerance); err == nil {
			a.webhook = v
		}
	default:
		return nil, fmt.Errorf("authentication_type must be 'none', 'api_key', 'basic_auth', 'bearer_token', or 'standard_webhooks', got %q", a.authType)
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
	case InboundAuthStandardWebhooks:
		if _, err := parseStandardWebhookSecret(a.webhookSecret); err != nil {
			return err
		}
	}
	return nil
}

// RequiresBody reports whether the configured type authenticates the request
// body, so callers must read it before calling AuthenticateRequest.
func (a *InboundAuthenticator) RequiresBody() bool {
	return a.authType == InboundAuthStandardWebhooks
}

// AuthenticateRequest is Authenticate for callers that have read the body:
// it also checks body signatures.
func (a *InboundAuthenticator) AuthenticateRequest(r *http.Request, body []byte) bool {
	if a.authType == InboundAuthStandardWebhooks {
		return a.webhook != nil && a.webhook.Verify(r.Header, body) == nil
	}
	return a.Authenticate(r)
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
	case InboundAuthStandardWebhooks:
		return false // signed bodies need AuthenticateRequest
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
