package controllers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/services/audit"
	"ezhealthkonnect/services/authguard"

	"github.com/gin-gonic/gin"
)

const testIfaceID = "11111111-2222-3333-4444-555555555555"

type fakeEndpointAuthSource struct {
	cfg   map[string]interface{}
	found bool
	err   error
}

func (f fakeEndpointAuthSource) EndpointAuthConfig(_ context.Context, _ string) (map[string]interface{}, bool, error) {
	return f.cfg, f.found, f.err
}

type recordingAuditLogger struct {
	mu     sync.Mutex
	events []audit.AuditEvent
}

func (r *recordingAuditLogger) Log(_ context.Context, e audit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func newSyncAuthRouter(src EndpointAuthConfigSource, al audit.AuditLogger) (*gin.Engine, *bool) {
	gin.SetMode(gin.TestMode)
	reached := false
	r := gin.New()
	g := r.Group("/api/eligibility", NewSyncEndpointAuth(src, al, nil).Middleware())
	g.POST("/:interfaceId/check", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })
	return r, &reached
}

func doSyncReq(r *gin.Engine, id string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/"+id+"/check", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSyncEndpointAuth(t *testing.T) {
	apiKeyCfg := map[string]interface{}{"authentication_type": "api_key", "api_key": "partner-secret"}
	cases := []struct {
		name        string
		src         fakeEndpointAuthSource
		id          string
		headers     map[string]string
		wantStatus  int
		wantReached bool
		wantAudit   bool
	}{
		{"correct api key", fakeEndpointAuthSource{cfg: apiKeyCfg, found: true}, testIfaceID, map[string]string{"X-API-Key": "partner-secret"}, 200, true, false},
		{"wrong api key", fakeEndpointAuthSource{cfg: apiKeyCfg, found: true}, testIfaceID, map[string]string{"X-API-Key": "nope"}, 401, false, true},
		{"no credentials", fakeEndpointAuthSource{cfg: apiKeyCfg, found: true}, testIfaceID, nil, 401, false, true},
		{"verified app user bypasses partner auth", fakeEndpointAuthSource{cfg: nil, found: true}, testIfaceID, map[string]string{"X-User-ID": "u-1"}, 200, true, false},
		{"endpoint_auth not configured", fakeEndpointAuthSource{cfg: nil, found: true}, testIfaceID, map[string]string{"X-API-Key": "x"}, 401, false, true},
		{"type none refused", fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "none"}, found: true}, testIfaceID, nil, 401, false, true},
		{"configured type but missing secret refused", fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "bearer_token"}, found: true}, testIfaceID, map[string]string{"Authorization": "Bearer "}, 401, false, true},
		{"unknown type refused", fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "magic"}, found: true}, testIfaceID, nil, 401, false, true},
		{"unknown interface", fakeEndpointAuthSource{found: false}, testIfaceID, map[string]string{"X-API-Key": "partner-secret"}, 401, false, true},
		{"non-uuid interface id", fakeEndpointAuthSource{cfg: apiKeyCfg, found: true}, "x", map[string]string{"X-API-Key": "partner-secret"}, 401, false, true},
		{"config source error", fakeEndpointAuthSource{err: errors.New("db down")}, testIfaceID, map[string]string{"X-API-Key": "partner-secret"}, 500, false, false},
		{"bearer ok", fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "bearer_token", "bearer_token": "tok"}, found: true}, testIfaceID, map[string]string{"Authorization": "Bearer tok"}, 200, true, false},
		{"basic ok", fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, found: true}, testIfaceID,
			map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))}, 200, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := &recordingAuditLogger{}
			r, reached := newSyncAuthRouter(tc.src, al)
			w := doSyncReq(r, tc.id, tc.headers)
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if *reached != tc.wantReached {
				t.Errorf("handler reached = %v, want %v", *reached, tc.wantReached)
			}
			if gotAudit := len(al.events) > 0; gotAudit != tc.wantAudit {
				t.Errorf("audit written = %v, want %v", gotAudit, tc.wantAudit)
			}
			if tc.wantAudit && al.events[0].Action != "SYNC_ENDPOINT_AUTH_FAILED" {
				t.Errorf("audit action = %q", al.events[0].Action)
			}
		})
	}
}

func TestSyncEndpointAuth_AuditNeverStoresGarbageEntityID(t *testing.T) {
	al := &recordingAuditLogger{}
	r, _ := newSyncAuthRouter(fakeEndpointAuthSource{found: true}, al)
	doSyncReq(r, "not-a-uuid", nil)
	if len(al.events) != 1 || al.events[0].EntityID != "" {
		t.Fatalf("expected one audit event with empty EntityID, got %+v", al.events)
	}
}

func TestSyncEndpointAuth_NilAuditLoggerIsSafe(t *testing.T) {
	r, _ := newSyncAuthRouter(fakeEndpointAuthSource{found: true}, nil)
	if w := doSyncReq(r, testIfaceID, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func newLimitedRouter(src EndpointAuthConfigSource, al audit.AuditLogger) *gin.Engine {
	gin.SetMode(gin.TestMode)
	limiter := authguard.NewInMemoryFailureLimiter(authguard.Policy{MaxFailures: 3, Window: time.Minute, Lockout: time.Minute}, nil)
	r := gin.New()
	g := r.Group("/api/eligibility", NewSyncEndpointAuth(src, al, limiter).Middleware())
	g.POST("/:interfaceId/check", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func doFromIP(r *gin.Engine, ip string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/"+testIfaceID+"/check", nil)
	req.RemoteAddr = ip + ":40000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSyncEndpointAuth_LocksOutAfterRepeatedFailures(t *testing.T) {
	src := fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "api_key", "api_key": "partner-secret"}, found: true}
	al := &recordingAuditLogger{}
	r := newLimitedRouter(src, al)

	for i := 0; i < 3; i++ {
		if w := doFromIP(r, "203.0.113.9", map[string]string{"X-API-Key": "guess"}); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i+1, w.Code)
		}
	}
	// Locked: even the CORRECT key is refused from this client until the lockout expires.
	w := doFromIP(r, "203.0.113.9", map[string]string{"X-API-Key": "partner-secret"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429 once locked out", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("a 429 must carry Retry-After")
	}
	// A different client (the real partner) is unaffected.
	if w := doFromIP(r, "198.51.100.7", map[string]string{"X-API-Key": "partner-secret"}); w.Code != http.StatusOK {
		t.Fatalf("other client status %d, want 200 — an attacker must not lock out the partner", w.Code)
	}
	last := al.events[len(al.events)-1]
	if last.Action != "SYNC_ENDPOINT_AUTH_FAILED" || !strings.Contains(fmt.Sprint(last.Metadata["reason"]), "locked out") {
		t.Errorf("lockout must be audited, last event: %+v", last)
	}
}

func TestSyncEndpointAuth_SuccessResetsFailureCount(t *testing.T) {
	src := fakeEndpointAuthSource{cfg: map[string]interface{}{"authentication_type": "api_key", "api_key": "partner-secret"}, found: true}
	r := newLimitedRouter(src, nil)
	ip := "203.0.113.10"
	doFromIP(r, ip, map[string]string{"X-API-Key": "typo"})
	doFromIP(r, ip, map[string]string{"X-API-Key": "typo"})
	if w := doFromIP(r, ip, map[string]string{"X-API-Key": "partner-secret"}); w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	// Two more typos after a success must not lock (count was reset).
	doFromIP(r, ip, map[string]string{"X-API-Key": "typo"})
	if w := doFromIP(r, ip, map[string]string{"X-API-Key": "typo"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401 (not locked) after a reset", w.Code)
	}
}
