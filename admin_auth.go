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
// When no proxy secret is configured (neither INTERNAL_PROXY_SECRET nor its
// JWT_SECRET fallback) every proxied request is REJECTED — fail closed. Before
// 2026-10-20 this mode let every request through, which on a standalone
// install (Go runs as its own service) would have exposed the whole API to
// anything that could reach the Go port. Node refuses to start without
// JWT_SECRET, so a working deployment always has a secret.
//
// The Go API port (default 8080) is configurable via API_PORT. All traffic
// to this port is expected to arrive through the Node.js proxy; the port
// should not be exposed on public network interfaces in production.
package main

import (
	"crypto/subtle"
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
		log.Printf("❌ SECURITY: no proxy secret is configured (INTERNAL_PROXY_SECRET / JWT_SECRET).")
		log.Printf("❌ All proxied /api requests will be REJECTED until one is set in .env.")
	} else if len(internalProxySecret) < 32 {
		log.Printf("⚠️  SECURITY WARNING: INTERNAL_PROXY_SECRET is shorter than 32 characters.")
		log.Printf("⚠️  Use a randomly generated secret of at least 32 characters.")
	}
}

// verifyProxySecret checks that the request arrived through the authenticated
// Node.js proxy. Fails closed: with no secret configured nothing is accepted.
// Exact, constant-time comparison (the header is a shared secret).
func verifyProxySecret(c *gin.Context) bool {
	if internalProxySecret == "" {
		return false
	}
	got := c.GetHeader("X-Internal-Proxy-Secret")
	return subtle.ConstantTimeCompare([]byte(got), []byte(internalProxySecret)) == 1
}

// clientOriginHeader is set by app.js's forwardToGo on every request it proxies
// from an outside client (browser, API caller, partner system). Node's own
// server-to-server calls (services/goBackendClient.js) never set it. A client
// cannot add or remove it: forwardToGo builds Go-bound headers from scratch.
const clientOriginHeader = "X-Proxy-Origin"

// publicClientRoute is a route a client may reach WITHOUT a verified app user.
// Either an exact method+path, or a path prefix whose routes enforce their own,
// non-session authentication.
type publicClientRoute struct {
	method     string // "" = any method
	path       string
	prefixOnly bool
	reason     string
}

// publicClientRoutes is the complete allow-list of anonymous-client access to
// the Go API. Everything else requires the verified user Node forwards as
// X-User-ID. Adding an entry here is a deliberate security decision.
var publicClientRoutes = []publicClientRoute{
	{method: http.MethodGet, path: "/api/system/health", reason: "liveness probe"},
	{path: "/api/eligibility/", prefixOnly: true, reason: "partner endpoint — per-interface auth (requireSyncEndpointAuth)"},
	{path: "/api/claim-status/", prefixOnly: true, reason: "partner endpoint — per-interface auth (requireSyncEndpointAuth)"},
	{path: "/api/prior-auth/", prefixOnly: true, reason: "partner endpoint — per-interface auth (requireSyncEndpointAuth)"},
	{path: "/api/pharmacy-claim/", prefixOnly: true, reason: "partner endpoint — per-interface auth (requireSyncEndpointAuth)"},
}

// isPublicClientRoute reports whether method+path is on the anonymous allow-list.
func isPublicClientRoute(method, path string) bool {
	for _, r := range publicClientRoutes {
		if r.method != "" && r.method != method {
			continue
		}
		if r.prefixOnly && strings.HasPrefix(path, r.path) {
			return true
		}
		if !r.prefixOnly && path == r.path {
			return true
		}
	}
	return false
}

// requireProxiedRequest is the base middleware applied to ALL /api routes.
// It verifies that the request passed through the Node.js proxy
// (X-Internal-Proxy-Secret) and — for client-originated requests — that Node
// actually authenticated a user (X-User-ID), unless the route is on
// publicClientRoutes. That second check is defense in depth: the proxy secret
// alone proves only that a request came through Node, which attaches it to
// every forwarded request, authenticated or not (the 2026-10-19 regression
// found whole route groups forwarded without auth for exactly this reason).
// Role enforcement is left to requireProxiedAdmin()/requireProxiedSuperAdmin().
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
		if strings.EqualFold(c.GetHeader(clientOriginHeader), "client") &&
			strings.TrimSpace(c.GetHeader("X-User-ID")) == "" &&
			!isPublicClientRoute(c.Request.Method, c.Request.URL.Path) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "authentication required",
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
