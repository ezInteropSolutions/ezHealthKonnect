package connectors

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func newAuthReq(headers map[string]string) *http.Request {
	r := &http.Request{Header: http.Header{}}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestInboundAuthenticator_Authenticate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     map[string]interface{}
		headers map[string]string
		want    bool
	}{
		{"api key default header ok", map[string]interface{}{"authentication_type": "api_key", "api_key": "k1"}, map[string]string{"X-API-Key": "k1"}, true},
		{"api key wrong", map[string]interface{}{"authentication_type": "api_key", "api_key": "k1"}, map[string]string{"X-API-Key": "k2"}, false},
		{"api key missing header", map[string]interface{}{"authentication_type": "api_key", "api_key": "k1"}, nil, false},
		{"api key custom header", map[string]interface{}{"authentication_type": "api_key", "api_key": "k1", "api_key_header": "X-Partner-Key"}, map[string]string{"X-Partner-Key": "k1"}, true},
		{"api key custom header, sent on default header", map[string]interface{}{"authentication_type": "api_key", "api_key": "k1", "api_key_header": "X-Partner-Key"}, map[string]string{"X-API-Key": "k1"}, false},
		{"empty configured api key never matches absent header", map[string]interface{}{"authentication_type": "api_key"}, nil, false},
		{"empty configured api key never matches empty header", map[string]interface{}{"authentication_type": "api_key"}, map[string]string{"X-API-Key": ""}, false},
		{"basic ok", map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, map[string]string{"Authorization": basicHeader("u", "p")}, true},
		{"basic wrong user", map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, map[string]string{"Authorization": basicHeader("x", "p")}, false},
		{"basic wrong pass", map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, map[string]string{"Authorization": basicHeader("u", "x")}, false},
		{"basic with bearer header", map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, map[string]string{"Authorization": "Bearer p"}, false},
		{"basic unset credentials never match empty basic", map[string]interface{}{"authentication_type": "basic_auth"}, map[string]string{"Authorization": basicHeader("", "")}, false},
		{"bearer ok", map[string]interface{}{"authentication_type": "bearer_token", "bearer_token": "t"}, map[string]string{"Authorization": "Bearer t"}, true},
		{"bearer no prefix", map[string]interface{}{"authentication_type": "bearer_token", "bearer_token": "t"}, map[string]string{"Authorization": "t"}, false},
		{"bearer wrong", map[string]interface{}{"authentication_type": "bearer_token", "bearer_token": "t"}, map[string]string{"Authorization": "Bearer x"}, false},
		{"none always passes", map[string]interface{}{"authentication_type": "none"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewInboundAuthenticator(tc.cfg, InboundAuthAPIKey)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if got := a.Authenticate(newAuthReq(tc.headers)); got != tc.want {
				t.Errorf("Authenticate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInboundAuthenticator_DefaultTypeApplies(t *testing.T) {
	a, err := NewInboundAuthenticator(map[string]interface{}{"bearer_token": "t"}, InboundAuthBearerToken)
	if err != nil {
		t.Fatal(err)
	}
	if a.Type() != InboundAuthBearerToken {
		t.Errorf("Type() = %q, want default %q", a.Type(), InboundAuthBearerToken)
	}
	if !a.Authenticate(newAuthReq(map[string]string{"Authorization": "Bearer t"})) {
		t.Error("expected default bearer type to authenticate")
	}
}

func TestInboundAuthenticator_RejectsUnknownType(t *testing.T) {
	if _, err := NewInboundAuthenticator(map[string]interface{}{"authentication_type": "oauth_magic"}, InboundAuthAPIKey); err == nil {
		t.Error("expected an error for an unknown authentication_type")
	}
}

func TestInboundAuthenticator_Validate(t *testing.T) {
	cases := []struct {
		cfg     map[string]interface{}
		wantErr bool
	}{
		{map[string]interface{}{"authentication_type": "api_key"}, true},
		{map[string]interface{}{"authentication_type": "api_key", "api_key": "k"}, false},
		{map[string]interface{}{"authentication_type": "basic_auth", "username": "u"}, true},
		{map[string]interface{}{"authentication_type": "basic_auth", "username": "u", "password": "p"}, false},
		{map[string]interface{}{"authentication_type": "bearer_token"}, true},
		{map[string]interface{}{"authentication_type": "bearer_token", "bearer_token": "t"}, false},
		{map[string]interface{}{"authentication_type": "none"}, false},
	}
	for _, tc := range cases {
		a, err := NewInboundAuthenticator(tc.cfg, InboundAuthAPIKey)
		if err != nil {
			t.Fatalf("parse %v: %v", tc.cfg, err)
		}
		if err := a.Validate(); (err != nil) != tc.wantErr {
			t.Errorf("Validate(%v) err=%v, wantErr=%v", tc.cfg, err, tc.wantErr)
		}
	}
}

func TestInboundAuthenticator_NilConfigIsSafe(t *testing.T) {
	a, err := NewInboundAuthenticator(nil, InboundAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if a.Authenticate(newAuthReq(nil)) {
		t.Error("nil config with api_key default must not authenticate anything")
	}
}
