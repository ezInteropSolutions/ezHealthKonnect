package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
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
