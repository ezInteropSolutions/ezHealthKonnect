// services/executors/enrichment/api_enrichment_options_test.go
// Tests for api_enrichment's baseUrl, encodePlaceholders and acceptStatuses
// options, and the HTTP client's body rewind on retry.
//
//   TC-AEO-001  baseUrl joins a relative endpoint; an absolute one ignores it
//   TC-AEO-002  encodePlaceholders keeps hostile values inside one segment
//   TC-AEO-003  without encodePlaceholders values are substituted raw (compat)
//   TC-AEO-004  an accepted 404 succeeds with the response body as output
//   TC-AEO-005  a status not accepted still fails the step
//   TC-AEO-006  a retried PUT resends its body
package enrichment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// pathRecorder answers 200 {"ok":true} and records each request's escaped path.
func pathRecorder(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.EscapedPath())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func runAPIStep(t *testing.T, cfg map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	t.Helper()
	step := &models.TransformationStep{ID: t.Name(), StepName: "api", StepType: "enrichment.api", Enabled: true, Config: cfg}
	return NewAPIEnrichmentExecutor().Execute(context.Background(), step, input)
}

func TestAPIEnrichment_BaseURL(t *testing.T) {
	srv, paths := pathRecorder(t)
	if _, err := runAPIStep(t, map[string]interface{}{
		"baseUrl": srv.URL + "/", "endpoint": "/fhir/r4/Patient/{eid}", "fieldMappings": map[string]interface{}{"eid": "subject"},
	}, map[string]interface{}{"subject": "EID-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runAPIStep(t, map[string]interface{}{
		"baseUrl": "https://ignored.invalid", "endpoint": srv.URL + "/abs",
	}, map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths(), ","); got != "/fhir/r4/Patient/EID-1,/abs" {
		t.Fatalf("paths %s", got)
	}
}

func TestAPIEnrichment_EncodePlaceholders(t *testing.T) {
	srv, paths := pathRecorder(t)
	for _, v := range []string{"000/123?x=1#f", "..", "Zoë 1"} {
		if _, err := runAPIStep(t, map[string]interface{}{
			"baseUrl": srv.URL, "endpoint": "/v1/source-records/EHR/{id}", "encodePlaceholders": true,
			"fieldMappings": map[string]interface{}{"id": "localId"},
		}, map[string]interface{}{"localId": v}); err != nil {
			t.Fatal(err)
		}
	}
	got := paths()
	want := []string{"/v1/source-records/EHR/000%2F123%3Fx=1%23f", "/v1/source-records/EHR/%2E%2E", "/v1/source-records/EHR/Zo%C3%AB%201"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths\n %v\nwant\n %v", got, want)
	}
}

func TestAPIEnrichment_PlaceholdersRawByDefault(t *testing.T) {
	srv, paths := pathRecorder(t)
	if _, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/{frag}", "fieldMappings": map[string]interface{}{"frag": "f"},
	}, map[string]interface{}{"f": "a/b"}); err != nil {
		t.Fatal(err)
	}
	if got := paths(); len(got) != 1 || got[0] != "/a/b" {
		t.Fatalf("paths %v", got)
	}
}

func notFoundServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"not-found"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPIEnrichment_AcceptStatuses(t *testing.T) {
	srv := notFoundServer(t, http.StatusNotFound)
	out, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/Patient/x", "acceptStatuses": []interface{}{404, 410},
	}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("accepted 404 failed the step: %v", err)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if so == nil {
		raw, _ := json.Marshal(out)
		t.Fatalf("no _stepOutput in %s", raw)
	}
	if got := executors.GetNestedValue(so, "variables.resourceType"); got != "OperationOutcome" {
		if got2 := so["resourceType"]; got2 != "OperationOutcome" {
			raw, _ := json.Marshal(so)
			t.Fatalf("step output doesn't carry the response body: %s", raw)
		}
	}
}

func TestAPIEnrichment_UnacceptedStatusFails(t *testing.T) {
	srv := notFoundServer(t, http.StatusInternalServerError)
	if _, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/Patient/x", "acceptStatuses": []interface{}{404},
	}, map[string]interface{}{}); err == nil {
		t.Fatal("a 500 not in acceptStatuses succeeded")
	}
}

func TestAPIEnrichment_RetryResendsBody(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if _, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL + "/r", "method": "PUT", "retryCount": 1, "retryDelayMs": 1,
		"requestBody": map[string]interface{}{"a": "b"},
	}, map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || bodies[0] != bodies[1] || bodies[1] == "" {
		t.Fatalf("bodies per attempt: %q", bodies)
	}
}

func TestAPIEnrichment_QueryPlaceholdersAndPredicates(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	input := map[string]interface{}{"message": map[string]interface{}{"fhirResources": []interface{}{
		map[string]interface{}{"resourceType": "MessageHeader"},
		map[string]interface{}{"resourceType": "Patient", "identifier": []interface{}{
			map[string]interface{}{"system": "urn:oid:9.9", "value": "X"},
			map[string]interface{}{"system": "urn:oid:1.2.3", "value": "A&B=C#D"},
		}},
	}}}
	if _, err := runAPIStep(t, map[string]interface{}{
		"baseUrl": srv.URL, "endpoint": "/fhir/r4/Patient?identifier=urn:oid:1.2.3|{id}", "method": "GET", "encodePlaceholders": true,
		"fieldMappings": map[string]interface{}{"id": "message.fhirResources[resourceType=Patient].identifier[system=urn:oid:1.2.3].value"},
	}, input); err != nil {
		t.Fatal(err)
	}
	if got != "identifier=urn:oid:1.2.3|A%26B%3DC%23D" {
		t.Fatalf("query %q", got)
	}
}

func TestAPIEnrichment_RequestBodyPlaceholdersInArrays(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	input := map[string]interface{}{"message": map[string]interface{}{"old": "OLD-1", "items": []interface{}{
		map[string]interface{}{"k": "a", "v": "A"}, map[string]interface{}{"k": "b", "v": "B"}}}}
	if _, err := runAPIStep(t, map[string]interface{}{
		"endpoint": srv.URL, "method": "PUT",
		"requestBody": map[string]interface{}{"active": false, "identifier": []interface{}{map[string]interface{}{"value": "oldId"}},
			"link": []interface{}{map[string]interface{}{"other": map[string]interface{}{"value": "newId"}}}},
		"fieldMappings": map[string]interface{}{"oldId": "old", "newId": "message.items[k=b].v"},
	}, input); err != nil {
		t.Fatal(err)
	}
	ids := body["identifier"].([]interface{})
	link := body["link"].([]interface{})[0].(map[string]interface{})["other"].(map[string]interface{})
	if body["active"] != false || ids[0].(map[string]interface{})["value"] != "OLD-1" || link["value"] != "B" {
		t.Fatalf("body %v", body)
	}
}
