// controllers/sync_endpoint_auth.go
//
// SyncEndpointAuth — per-interface authentication for the synchronous
// partner-facing endpoints (/api/eligibility, /api/claim-status,
// /api/prior-auth, /api/pharmacy-claim). These are called machine-to-machine
// by payer / pharmacy / provider systems that have no app session, so they
// cannot use the app login. Instead each interface carries its own endpoint
// credentials, configured by the integration engineer on the interface:
//
//	interfaces.source_config.endpoint_auth = {
//	    "authentication_type": "api_key" | "basic_auth" | "bearer_token",
//	    "api_key_header": "X-API-Key", "api_key": "...",
//	    "username": "...", "password": "...",
//	    "bearer_token": "..."
//	}
//
// Secrets are encrypted at rest by the interface save path (key-name based,
// see interfacesController.js / credential_store.go) and decrypted here.
// Verification reuses connectors.InboundAuthenticator — the same constant-time
// implementation http_rest_inbound uses — so there is one credential check in
// the codebase, not two.
//
// Secure by default: an interface with no endpoint_auth (or authentication_type
// "none") is refused. A request carrying a verified app user (X-User-ID, which
// only Node's proxy can set — see admin_auth.go) is allowed, so in-app testing
// keeps working; such a user can already run pipelines via Test Pipeline.
//
// Found during the 2026-10-19 release regression: these endpoints executed an
// interface's pipeline for any anonymous caller who knew its id.
package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"ezhealthkonnect/services"
	"ezhealthkonnect/services/audit"
	"ezhealthkonnect/services/authguard"
	"ezhealthkonnect/services/connectors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EndpointAuthConfigSource returns an interface's decrypted endpoint_auth
// config. found=false means the interface does not exist (or is deleted);
// a nil config with found=true means the interface has no endpoint auth set.
type EndpointAuthConfigSource interface {
	EndpointAuthConfig(ctx context.Context, interfaceID string) (cfg map[string]interface{}, found bool, err error)
}

// dbEndpointAuthConfigSource reads endpoint_auth from interfaces.source_config.
type dbEndpointAuthConfigSource struct {
	db    *sql.DB
	creds *services.CredentialStore // nil = passthrough (no encryption key configured)
}

// NewDBEndpointAuthConfigSource constructs the production config source.
func NewDBEndpointAuthConfigSource(db *sql.DB, creds *services.CredentialStore) EndpointAuthConfigSource {
	return &dbEndpointAuthConfigSource{db: db, creds: creds}
}

func (s *dbEndpointAuthConfigSource) EndpointAuthConfig(ctx context.Context, interfaceID string) (map[string]interface{}, bool, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT source_config::text FROM interfaces WHERE id = $1 AND deleted_at IS NULL`, interfaceID,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !raw.Valid || raw.String == "" {
		return nil, true, nil
	}
	var sourceConfig map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &sourceConfig); err != nil {
		return nil, true, fmt.Errorf("source_config is not valid JSON: %w", err)
	}
	endpointAuth, _ := sourceConfig["endpoint_auth"].(map[string]interface{})
	if endpointAuth == nil {
		return nil, true, nil
	}
	decrypted, err := s.creds.DecryptConfigFields(endpointAuth)
	if err != nil {
		return nil, true, fmt.Errorf("decrypting endpoint_auth: %w", err)
	}
	return decrypted, true, nil
}

// SyncEndpointAuth enforces per-interface endpoint authentication.
type SyncEndpointAuth struct {
	source  EndpointAuthConfigSource
	audit   audit.AuditLogger         // optional; nil disables audit rows
	limiter authguard.FailureLimiter // optional; nil disables brute-force lockout
}

// NewSyncEndpointAuth constructs the middleware provider. limiter blocks a
// client (IP + interface) after repeated wrong credentials; nil disables it.
func NewSyncEndpointAuth(source EndpointAuthConfigSource, auditLogger audit.AuditLogger, limiter authguard.FailureLimiter) *SyncEndpointAuth {
	return &SyncEndpointAuth{source: source, audit: auditLogger, limiter: limiter}
}

// Middleware returns the gin handler. It must be attached to a group whose
// routes carry an :interfaceId path parameter.
func (a *SyncEndpointAuth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("X-User-ID")) != "" {
			c.Next() // verified app user (set only by Node's authenticated proxy)
			return
		}

		interfaceID := c.Param("interfaceId")
		if _, err := uuid.Parse(interfaceID); err != nil {
			a.reject(c, interfaceID, "invalid interface id", http.StatusUnauthorized, "authentication failed")
			return
		}

		// Brute-force protection: keyed by client IP + interface, so an attacker
		// can only lock out their own address, never the legitimate partner.
		lockKey := c.ClientIP() + "|" + interfaceID
		if a.limiter != nil {
			if blocked, retryAfter := a.limiter.Blocked(lockKey); blocked {
				c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				a.reject(c, interfaceID, "locked out after repeated failed attempts", http.StatusTooManyRequests,
					"too many failed authentication attempts — try again later")
				return
			}
		}

		cfg, found, err := a.source.EndpointAuthConfig(c.Request.Context(), interfaceID)
		if err != nil {
			log.Printf("❌ sync endpoint auth: loading config for interface %s: %v", interfaceID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "endpoint authentication could not be evaluated"})
			c.Abort()
			return
		}
		if !found {
			a.reject(c, interfaceID, "unknown interface", http.StatusUnauthorized, "authentication failed")
			return
		}
		if cfg == nil {
			a.reject(c, interfaceID, "endpoint_auth not configured", http.StatusUnauthorized,
				"endpoint authentication is not configured for this interface")
			return
		}

		authenticator, err := connectors.NewInboundAuthenticator(cfg, connectors.InboundAuthAPIKey)
		if err == nil && authenticator.Type() == connectors.InboundAuthNone {
			err = fmt.Errorf("authentication_type 'none' is not permitted for partner endpoints")
		}
		if err == nil {
			err = authenticator.Validate()
		}
		if err != nil {
			a.reject(c, interfaceID, "endpoint_auth invalid: "+err.Error(), http.StatusUnauthorized,
				"endpoint authentication is not configured correctly for this interface")
			return
		}

		if !authenticator.Authenticate(c.Request) {
			reason := "credentials did not match (" + authenticator.Type() + ")"
			if a.limiter != nil && a.limiter.RecordFailure(lockKey) {
				reason += "; client now locked out"
			}
			a.reject(c, interfaceID, reason, http.StatusUnauthorized, "authentication failed")
			return
		}
		if a.limiter != nil {
			a.limiter.Reset(lockKey)
		}
		c.Next()
	}
}

// reject writes the HTTP error and an audit row. reason is internal detail
// (logged and audited, never sent to the caller beyond clientMsg).
func (a *SyncEndpointAuth) reject(c *gin.Context, interfaceID, reason string, status int, clientMsg string) {
	log.Printf("🔒 sync endpoint auth rejected %s %s (interface %s): %s", c.Request.Method, c.Request.URL.Path, interfaceID, reason)
	if a.audit != nil {
		_ = a.audit.Log(c.Request.Context(), audit.AuditEvent{
			Action:     "SYNC_ENDPOINT_AUTH_FAILED",
			EntityType: "interface",
			EntityID:   interfaceIDForAudit(interfaceID),
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			Metadata: map[string]interface{}{
				"path":   c.Request.URL.Path,
				"method": c.Request.Method,
				"reason": reason,
			},
		})
	}
	c.JSON(status, gin.H{"success": false, "error": clientMsg})
	c.Abort()
}

// interfaceIDForAudit returns the id only when it is a well-formed uuid, so a
// garbage path segment is never written into the audit entity_id column.
func interfaceIDForAudit(id string) string {
	if _, err := uuid.Parse(id); err != nil {
		return ""
	}
	return id
}
