package enrichment

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A refused status fails the step, and its answer stays in the step's
// execution details for a relay that passes it on.
func TestAPIEnrichment_RefusedStatusKeepsItsAnswer(t *testing.T) {
	srv := notFoundServer(t, http.StatusUnprocessableEntity)
	out, err := runAPIStep(t, map[string]interface{}{"endpoint": srv.URL + "/Patient/x"}, map[string]interface{}{})
	if err == nil {
		t.Fatal("a 422 succeeded")
	}
	details, _ := out["_executionDetails"].(map[string]interface{})
	resp, _ := details["http_response"].(map[string]interface{})
	if resp == nil {
		raw, _ := json.Marshal(out)
		t.Fatalf("no http_response in %s", raw)
	}
	if resp["status_code"] != http.StatusUnprocessableEntity || !strings.Contains(resp["body_raw"].(string), "OperationOutcome") {
		t.Errorf("response details: %v", resp)
	}
}
