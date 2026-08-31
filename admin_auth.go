// admin_auth.go
// Go-side auth middleware for all /api routes and admin-only sub-groups.
//
// Architecture: the Node.js proxy (app.js) is the primary auth gate. It
// verifies the session/JWT before forwarding to Go and injects two trusted
// headers:
//
//   X-Internal-Proxy-Secret  — shared INTERNAL_PROXY_SECRET env var; proves
//                              the request passed through the authenticated proxy.
//   X-User-Role              — the caller's role from the verified session.
//
// Three tiers of protection:
//
//   requireProxiedRequest()   — applied to ALL /api routes. Verifies the
//                               proxy secret only. No role check.
//   requireProxiedAdmin()     — verifies secret + requires admin/super_admin role.
//   requireProxiedSuperAdmin() — verifies secret + requires super_admin role only.
//
// When INTERNAL_PROXY_SECRET is not configured the middleware passes all
// requests (permissive mode). A startup warning is emitted in main() so
// operators cannot miss a misconfigured deployment.
//
// The Go API port (default 8080) is configurable via API_PORT. All traffic
// to this port is expected to arrive through the Node.js proxy; the port
// should not be exposed on public network interfaces in production.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// internalProxySecret holds the resolved secret used by verifyProxySecret.
//
// This is intentionally NOT a package-level initializer (it used to be:
// `var internalProxySecret = func() string { ... os.Getenv(...) ... }()`).
// Package-level variable initializers run before main() executes a single
// statement — including before main()'s own godotenv.Load() call — so on any
// deployment where INTERNAL_PROXY_SECRET/JWT_SECRET exist only inside .env
// (true for the standalone Windows/Linux installers, which never set these as
// real OS/service environment variables), an initializer here would always
// resolve to "" regardless of .env's contents, silently falling back to
// permissive mode. Worse, if the process happens to inherit an unrelated,
// stale value for JWT_SECRET from the OS environment, this var would latch
// onto that forever — no amount of editing .env or restarting the process
// would change it, since a package initializer only ever runs once, before
// .env is even read.
//
// InitInternalProxySecret must be called explicitly from main(), after
// godotenv.Load(), so this reads the same environment Node.js's proxy client
// resolves its header from.
var internalProxySecret string

// InitInternalProxySecret resolves internalProxySecret from the environment.
// Falls back to JWT_SECRET so existing deployments work without a new var.
// Call once from main(), after godotenv.Load() and before the HTTP server
// starts accepting requests.
func InitInternalProxySecret() {
	if s := os.Getenv("INTERNAL_PROXY_SECRET"); s != "" {
		internalProxySecret = s
		return
	}
	internalProxySecret = os.Getenv("JWT_SECRET")
}

// WarnIfProxySecretMissing logs a prominent warning when no proxy secret is
// configured. Call once from main() after all services are initialised.
func WarnIfProxySecretMissing() {
	if internalProxySecret == "" {
		log.Printf("⚠️  SECURITY WARNING: INTERNAL_PROXY_SECRET is not set.")
		log.Printf("⚠️  All /api routes are accessible without authentication.")
		log.Printf("⚠️  Set INTERNAL_PROXY_SECRET in your .env file before production deployment.")
	} else if len(internalProxySecret) < 32 {
		log.Printf("⚠️  SECURITY WARNING: INTERNAL_PROXY_SECRET is shorter than 32 characters.")
		log.Printf("⚠️  Use a randomly generated secret of at least 32 characters.")
	}
}

// verifyProxySecret checks that the request arrived through the authenticated
// Node.js proxy. Returns true (permissive) when no secret is configured so
// local development works out of the box.
func verifyProxySecret(c *gin.Context) bool {
	if internalProxySecret == "" {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Internal-Proxy-Secret"), internalProxySecret)
}

// requireProxiedRequest is the base middleware applied to ALL /api routes.
// It verifies only that the request passed through the authenticated Node.js
// proxy (X-Internal-Proxy-Secret). No role check is performed here — role
// enforcement is left to requireProxiedAdmin() and requireProxiedSuperAdmin()
// on sensitive sub-groups.
func requireProxiedRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !verifyProxySecret(c) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "direct access to internal API not permitted",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// requireProxiedAdmin rejects requests that either:
//   - did not come through the Node.js proxy (missing / wrong X-Internal-Proxy-Secret), or
//   - were not made by a user with the "admin" or "super_admin" role (X-User-Role header).
//
// Apply to any endpoint that should be admin-only and never callable directly
// against the Go port.
func requireProxiedAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !verifyProxySecret(c) {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "direct access to internal API not permitted"})
			c.Abort()
			return
		}
		role := c.GetHeader("X-User-Role")
		if !strings.EqualFold(role, "admin") && !strings.EqualFold(role, "super_admin") {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// requireProxiedSuperAdmin is like requireProxiedAdmin but restricted to the
// "super_admin" role only. Used for vendor-only operations (e.g. OOB template
// rebuild) that client admins should not be able to trigger.
func requireProxiedSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !verifyProxySecret(c) {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "direct access to internal API not permitted"})
			c.Abort()
			return
		}
		role := c.GetHeader("X-User-Role")
		if !strings.EqualFold(role, "super_admin") {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "super_admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
