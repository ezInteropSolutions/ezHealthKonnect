package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newProxiedRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	internalProxySecret = secret
	r := gin.New()
	api := r.Group("/api", requireProxiedRequest())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	api.GET("/system/health", ok)
	api.POST("/system/health", ok)
	api.POST("/processing/interfaces/:id/deactivate", ok)
	api.POST("/eligibility/:id/check", ok)
	api.POST("/pharmacy-claim/:id/submit", ok)
	api.GET("/cda/documents", ok)
	return r
}

func TestRequireProxiedRequest(t *testing.T) {
	secret := strings.Repeat("s", 32) // built in code so secret scanners do not flag a fixture
	t.Cleanup(func() { internalProxySecret = "" })

	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		want    int
	}{
		{"no proxy secret", http.MethodGet, "/api/cda/documents", nil, 401},
		{"internal server-to-server call (no client origin) passes", http.MethodPost, "/api/processing/interfaces/x/deactivate",
			map[string]string{"X-Internal-Proxy-Secret": secret}, 200},
		{"anonymous client blocked on mutation", http.MethodPost, "/api/processing/interfaces/x/deactivate",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 401},
		{"anonymous client blocked on PHI read", http.MethodGet, "/api/cda/documents",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 401},
		{"blank user id treated as anonymous", http.MethodGet, "/api/cda/documents",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client", "X-User-ID": "  "}, 401},
		{"authenticated client passes", http.MethodGet, "/api/cda/documents",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client", "X-User-ID": "u-1"}, 200},
		{"anonymous health check allowed", http.MethodGet, "/api/system/health",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 200},
		{"health allow-list is GET only", http.MethodPost, "/api/system/health",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 401},
		{"partner endpoint passes to its own auth", http.MethodPost, "/api/eligibility/x/check",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 200},
		{"partner endpoint (pharmacy) passes to its own auth", http.MethodPost, "/api/pharmacy-claim/x/submit",
			map[string]string{"X-Internal-Proxy-Secret": secret, "X-Proxy-Origin": "client"}, 200},
	}
	r := newProxiedRouter(secret)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestIsPublicClientRoute_PrefixRequiresSlashBoundary(t *testing.T) {
	// "/api/eligibility-admin" must not ride on the "/api/eligibility/" prefix.
	if isPublicClientRoute(http.MethodPost, "/api/eligibility-admin/x") {
		t.Error("prefix match must not cross a path-segment boundary")
	}
}

// Fail closed: with no proxy secret configured nothing is accepted — before
// 2026-10-20 an unset secret let every request through.
func TestVerifyProxySecret_FailsClosedWithoutSecret(t *testing.T) {
	r := newProxiedRouter("")
	t.Cleanup(func() { internalProxySecret = "" })
	for _, h := range []map[string]string{nil, {"X-Internal-Proxy-Secret": ""}, {"X-Internal-Proxy-Secret": "anything"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/cda/documents", nil)
		for k, v := range h {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("headers %v: status %d, want 401 when no secret is configured", h, w.Code)
		}
	}
}

// The proxy secret is compared exactly — a value that differs only in case is rejected.
func TestVerifyProxySecret_IsExactMatch(t *testing.T) {
	secret := strings.Repeat("a", 16) + strings.Repeat("B", 16)
	r := newProxiedRouter(secret)
	t.Cleanup(func() { internalProxySecret = "" })
	req := httptest.NewRequest(http.MethodGet, "/api/cda/documents", nil)
	req.Header.Set("X-Internal-Proxy-Secret", strings.ToUpper(secret))
	req.Header.Set("X-User-ID", "u1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("case-altered secret accepted: status %d", w.Code)
	}
}
