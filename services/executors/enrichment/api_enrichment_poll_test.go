package enrichment

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// jobServer answers a job's state from the list, one per request, then the last.
func jobServer(t *testing.T, states ...string) (*httptest.Server, *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&n, 1)) - 1
		if i >= len(states) {
			i = len(states) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"state": states[i], "rows": i})
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func pollCfg(url string, until []interface{}, timeout int) map[string]interface{} {
	return map[string]interface{}{"endpoint": url + "/jobs/1", "poll": map[string]interface{}{
		"untilField": "state", "untilValues": until, "intervalSeconds": 1, "timeoutSeconds": timeout}}
}

func TestAPIEnrichment_PollWaitsForTheWantedState(t *testing.T) {
	srv, hits := jobServer(t, "queued", "running", "Completed")
	out, err := runAPIStep(t, pollCfg(srv.URL, []interface{}{"completed", "failed"}, 30), map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if *hits != 3 {
		t.Errorf("%d requests, want 3", *hits)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if so["state"] != "Completed" || so["rows"] != float64(2) {
		t.Errorf("step output is not the last response: %v", so)
	}
}

func TestAPIEnrichment_PollAnsweredAtOnceMakesOneRequest(t *testing.T) {
	srv, hits := jobServer(t, "completed")
	if _, err := runAPIStep(t, pollCfg(srv.URL, []interface{}{"completed"}, 30), map[string]interface{}{}); err != nil || *hits != 1 {
		t.Fatalf("err %v, %d requests", err, *hits)
	}
}

func TestAPIEnrichment_PollTimesOutNamingTheLastState(t *testing.T) {
	srv, _ := jobServer(t, "running")
	_, err := runAPIStep(t, pollCfg(srv.URL, []interface{}{"completed"}, 2), map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "running") {
		t.Fatalf("err: %v", err)
	}
}

func TestAPIEnrichment_PollNeedsAFieldAndValues(t *testing.T) {
	if _, err := runAPIStep(t, map[string]interface{}{"endpoint": "http://x", "poll": map[string]interface{}{"untilField": "state"}},
		map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "poll needs") {
		t.Fatalf("err: %v", err)
	}
}

func TestAPIEnrichment_PollFailsAtOnceOnARequestError(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			_, _ = w.Write([]byte(`{"state":"running"}`))
			return
		}
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	if _, err := runAPIStep(t, pollCfg(srv.URL, []interface{}{"completed"}, 30), map[string]interface{}{}); err == nil ||
		!strings.Contains(err.Error(), "500") {
		t.Fatalf("err: %v", err)
	}
}
