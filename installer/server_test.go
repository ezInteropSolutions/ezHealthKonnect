package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func newTestServer() *UIServer {
	web := fstest.MapFS{"index.html": {Data: []byte("<html>")}}
	return NewUIServer(&fakePlatform{elevated: true}, nil, web, 41234)
}

func call(s *UIServer, method, target, host, token, contentType, origin, body string) int {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = host
	if token != "" {
		req.Header.Set("X-Installer-Token", token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestUIServer_RequiresTokenHostAndJSON(t *testing.T) {
	s := newTestServer()
	host := "127.0.0.1:41234"
	ok := `{"installDir":"/tmp/x","confirm":false}`

	cases := []struct {
		name                                 string
		method, target, host, token, ct, org string
		want                                 int
	}{
		{"valid read", "GET", "/api/defaults", host, s.Token, "", "", http.StatusOK},
		{"valid read on localhost", "GET", "/api/defaults", "localhost:41234", s.Token, "", "", http.StatusOK},
		{"missing token", "GET", "/api/defaults", host, "", "", "", http.StatusForbidden},
		{"wrong token", "GET", "/api/defaults", host, "nope", "", "", http.StatusForbidden},
		{"DNS rebinding host", "GET", "/api/defaults", "evil.example:41234", s.Token, "", "", http.StatusForbidden},
		{"foreign origin", "POST", "/api/uninstall", host, s.Token, "application/json", "http://evil.example", http.StatusForbidden},
		{"simple-request CSRF (text/plain)", "POST", "/api/uninstall", host, s.Token, "text/plain", "", http.StatusUnsupportedMediaType},
		{"unconfirmed uninstall", "POST", "/api/uninstall", host, s.Token, "application/json", "http://" + host, http.StatusBadRequest},
		{"static page wrong host", "GET", "/", "evil.example:41234", "", "", "", http.StatusForbidden},
		{"static page", "GET", "/", host, "", "", "", http.StatusOK},
	}
	for _, c := range cases {
		if got := call(s, c.method, c.target, c.host, c.token, c.ct, c.org, ok); got != c.want {
			t.Errorf("%s: HTTP %d, want %d", c.name, got, c.want)
		}
	}
	// EventSource cannot set headers, so a GET may carry the token in the URL.
	if got := call(s, "GET", "/api/defaults?t="+s.Token, host, "", "", "", ""); got != http.StatusOK {
		t.Errorf("token in query for GET: HTTP %d", got)
	}
	// ...but never for state-changing POSTs.
	if got := call(s, "POST", "/api/uninstall?t="+s.Token, host, "", "application/json", "", ok); got != http.StatusForbidden {
		t.Errorf("token in query for POST: HTTP %d", got)
	}
}

func TestUIServer_TokensAreUnique(t *testing.T) {
	a, b := newTestServer(), newTestServer()
	if a.Token == b.Token || len(a.Token) != 64 {
		t.Fatal("each run needs a fresh 256-bit token")
	}
	if !strings.Contains(a.URL(""), "t="+a.Token) || !strings.HasPrefix(a.URL(""), "http://127.0.0.1:") {
		t.Fatalf("URL %s", a.URL(""))
	}
}

func TestUIServer_PingRequiresToken(t *testing.T) {
	s := newTestServer()
	host := "127.0.0.1:41234"
	if code := call(s, http.MethodGet, "/api/ping", host, "", "", "", ""); code != http.StatusForbidden {
		t.Errorf("ping without token: got %d, want 403", code)
	}
	if code := call(s, http.MethodGet, "/api/ping", host, s.Token, "", "", ""); code != http.StatusNoContent {
		t.Errorf("ping with token: got %d, want 204", code)
	}
}

func TestUIServer_IdleOnlyAfterWizardLeavesAndNothingRuns(t *testing.T) {
	s := newTestServer()
	host := "127.0.0.1:41234"
	limit := 2 * time.Minute
	now := time.Now()

	if s.Idle(now.Add(time.Hour), limit) {
		t.Fatal("idle before the wizard ever connected (a slow first visit must not end setup)")
	}

	call(s, http.MethodGet, "/api/ping", host, s.Token, "", "", "")
	if s.Idle(time.Now().Add(limit/2), limit) {
		t.Error("idle while the wizard is still within the limit")
	}
	if !s.Idle(time.Now().Add(limit+time.Second), limit) {
		t.Error("not idle after the wizard stopped responding")
	}

	// Rejected requests (no token) are not wizard activity.
	s.mu.Lock()
	s.lastSeen = time.Now().Add(-time.Hour)
	s.mu.Unlock()
	call(s, http.MethodGet, "/api/ping", host, "", "", "", "")
	if !s.Idle(time.Now(), limit) {
		t.Error("an unauthorized request kept setup alive")
	}

	s.mu.Lock()
	s.busy = true
	s.mu.Unlock()
	if s.Idle(time.Now().Add(time.Hour), limit) {
		t.Error("idle while an operation is running (an install must always finish)")
	}

	s.mu.Lock()
	s.busy, s.streams = false, 1
	s.mu.Unlock()
	if s.Idle(time.Now().Add(time.Hour), limit) {
		t.Error("idle while a progress stream is open")
	}
}

func TestUIServer_ExitWhenIdleClosesDone(t *testing.T) {
	s := newTestServer()
	s.touch()
	go s.ExitWhenIdle(10*time.Millisecond, 5*time.Millisecond)
	select {
	case <-s.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("setup did not exit after the wizard went idle")
	}
}

// A stray </style> once closed the stylesheet early, and the rules after it
// were shown as text on the Welcome page.
func TestWizardPage_StylesheetIsWellFormed(t *testing.T) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	open, closing := strings.Count(page, "<style"), strings.Count(page, "</style>")
	if open != closing {
		t.Fatalf("index.html has %d <style> and %d </style>", open, closing)
	}
	head := page[strings.LastIndex(page, "</style>")+len("</style>"):]
	if between := strings.TrimSpace(head[:strings.Index(head, "</head>")]); between != "" {
		t.Errorf("text between the stylesheet and </head> would render on the page: %.80q", between)
	}
}
