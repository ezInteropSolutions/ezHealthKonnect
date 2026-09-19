package transform

import (
	"context"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// startEchoWSServer starts a real local websocket server that echoes back
// whatever text frame it receives. Mirrors services/connectors/websocket_test.go's
// own helper of the same shape — duplicated here (rather than exported from
// package connectors) since this test lives in package transform and only
// needs this one small piece, not the whole connectors test surface.
func startEchoWSServer(t *testing.T) (wsURL string, stop func()) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(msgType, payload); err != nil {
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	wsURL = "ws" + strings.TrimPrefix(server.URL, "http") + "/echo"
	return wsURL, server.Close
}

// TestOutboundConnector_WebSocket_SurfacesResponseBody proves the generalized
// metadata-surfacing loop in outbound_connector_executor.go (added alongside
// the websocket_outbound connector) correctly forwards a websocket response
// into step_output through the real OutboundConnectorExecutor — the same
// proof outbound_connector_ack_code_test.go already gives for MLLP's
// ack_code, now for a 3rd connector type with zero executor code changes
// needed for this specific key.
func TestOutboundConnector_WebSocket_SurfacesResponseBody(t *testing.T) {
	wsURL, stop := startEchoWSServer(t)
	defer stop()

	exec := NewOutboundConnectorExecutor()
	step := &models.TransformationStep{
		StepName: "Send To Partner System",
		StepType: "connector.outbound",
		Enabled:  true,
		Config: map[string]interface{}{
			"connectorType": "websocket_outbound",
			"config": map[string]interface{}{
				"url":                      wsURL,
				"connection_mode":          "per-message",
				"response_timeout_seconds": 5,
			},
			"contentField": "raw",
		},
	}
	input := map[string]interface{}{
		"raw": "hello from the pipeline",
	}

	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("expected successful delivery, got error: %v", err)
	}

	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be set, got: %+v", result)
	}

	if got := out["response_body"]; got != "hello from the pipeline" {
		t.Errorf("expected echoed response_body surfaced in step_output, got %v (full step output: %+v)", got, out)
	}
	if got := out["response_received"]; got != true {
		t.Errorf("expected response_received=true surfaced in step_output, got %v", got)
	}
	if got := out["response_frame_type"]; got != "text" {
		t.Errorf("expected response_frame_type=text surfaced in step_output, got %v", got)
	}
}
