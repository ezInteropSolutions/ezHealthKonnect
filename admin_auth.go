// admin_auth.go
// Go-side auth middleware for admin-only endpoints.
//
// Architecture: the Node.js proxy (app.js) is the primary auth gate. It
// verifies the session/JWT before forwarding to Go and injects two trusted
// headers:
//
//   X-Internal-Proxy-Secret  — shared INTERNAL_PROXY_SECRET env var; proves
//                              the request passed through the authenticated proxy.
//   X-User-Role              — the caller's role from the verified session.
//
// Go checks both. Direct calls to port 8080 that lack the proxy secret are
// rejected even if they forge the role header.
package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// internalProxySecret is loaded once at startup from the environment.
// Falls back to JWT_SECRET so existing deployments work without a new var.
var internalProxySecret = func() string {
	if s := os.Getenv("INTERNAL_PROXY_SECRET"); s != "" {
		return s
	}
	return os.Getenv("JWT_SECRET")
}()

// verifyProxySecret checks that the request arrived through the authenticated
// Node.js proxy. Shared by both requireProxiedAdmin and requireProxiedSuperAdmin.
func verifyProxySecret(c *gin.Context) bool {
	if internalProxySecret == "" {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Internal-Proxy-Secret"), internalProxySecret)
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
