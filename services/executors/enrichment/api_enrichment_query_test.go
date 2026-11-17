// services/executors/enrichment/api_enrichment_query_test.go
// Tests for api_enrichment's statusOutputField and bodyRefFallbacks, and
// the HTTP client's pre-encoded form body. Synthetic values only.
//
//   TC-AEQ-001  statusOutputField adds the status to the output; accepted
//               statuses stay distinguishable; off by default
//   TC-AEQ-002  bodyRefFallbacks: the first path with a value is the body
//   TC-AEQ-003  a form body given as an encoded string is sent as it is
package enrichment

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAPIEnrichment_StatusOutputField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
		}
		_, _ = w.Write([]byte(`{"resourceType":"OperationOutcome"}`))
	}))
	t.Cleanup(srv.Close)

	for path, want := range map[string]int{"/found": 200, "/missing": 404} {
		out, err := runAPIStep(t, map[string]interface{}{
			"baseUrl": srv.URL, "endpoint": path, "acceptStatuses": []interface{}{404}, "statusOutputField": "http_status",
		}, map[string]interface{}{})
		if err != nil {
			t.Fatal(err)
		}
		so := out["_stepOutput"].(map[string]interface{})
		if so["http_status"] != want || so["resourceType"] != "OperationOutcome" {
			t.Fatalf("%s: %v", path, so)
		}
	}
	out, err := runAPIStep(t, map[string]interface{}{"baseUrl": srv.URL, "endpoint": "/found"}, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := out["_stepOutput"].(map[string]interface{})["http_status"]; has {
		t.Fatal("status added without statusOutputField")
	}
}

// bodyRecorder answers 200 and records each request's body and content type.
func bodyRecorder(t *testing.T) (*httptest.Server, func() (string, string)) {
	t.Helper()
	var mu sync.Mutex
	var body, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body, ctype = string(b), r.Header.Get("Content-Type")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() (string, string) {
		mu.Lock()
		defer mu.Unlock()
		return body, ctype
	}
}

func TestAPIEnrichment_BodyRefFallbacksAndFormString(t *testing.T) {
	srv, last := bodyRecorder(t)
	cfg := map[string]interface{}{
		"baseUrl": srv.URL, "endpoint": "/fhir/r4/Patient/_search", "method": "POST",
		"headers":          map[string]interface{}{"Content-Type": "application/x-www-form-urlencoded"},
		"bodyRef":          "page.search",
		"bodyRefFallbacks": []interface{}{"query.search"},
	}
	for _, c := range []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{"fallback", map[string]interface{}{"page": map[string]interface{}{}, "query": map[string]interface{}{"search": "family=SYNTHWELL&_count=5"}}, "family=SYNTHWELL&_count=5"},
		{"empty first", map[string]interface{}{"page": map[string]interface{}{"search": ""}, "query": map[string]interface{}{"search": "given=Ann"}}, "given=Ann"},
		{"first", map[string]interface{}{"page": map[string]interface{}{"search": "family=X&_cursor=5"}, "query": map[string]interface{}{"search": "family=X"}}, "family=X&_cursor=5"},
	} {
		if _, err := runAPIStep(t, cfg, c.data); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		body, ctype := last()
		if body != c.want || ctype != "application/x-www-form-urlencoded" {
			t.Fatalf("%s: body %q type %q", c.name, body, ctype)
		}
	}
}
