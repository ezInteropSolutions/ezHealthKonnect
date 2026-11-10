package transform

import (
	"context"
	"ezhealthkonnect/models"
	"net"
	"sync"
	"testing"
)

// Regression (v26.10 UAT, message viewer): the outbound step persisted "the exact bytes sent" only AFTER a successful
// send, so a message whose delivery failed had no outbound payload and the viewer's Transformed tab could only say
// "not yet available". The payload that was prepared and attempted is the key evidence for diagnosing the failure and
// is what a redrive resends, so it is stored BEFORE delivery is attempted, whatever the outcome.

type storedPayload struct {
	interfaceID, messageID, content, contentType string
}

// recordingStoreCtx returns a context carrying a StoreOutboundFn that records every call.
func recordingStoreCtx() (context.Context, func() []storedPayload) {
	var mu sync.Mutex
	var calls []storedPayload
	ctx := context.WithValue(context.Background(), "interface_id", "iface-1")
	ctx = context.WithValue(ctx, "message_id", "msg-1")
	ctx = context.WithValue(ctx, "store_outbound_fn", models.StoreOutboundFn(func(ifaceID, msgID, content, ct string) string {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, storedPayload{ifaceID, msgID, content, ct})
		return "s3://test/outbound/" + msgID
	}))
	return ctx, func() []storedPayload {
		mu.Lock()
		defer mu.Unlock()
		return append([]storedPayload(nil), calls...)
	}
}

func mllpStep(host string, port int) *models.TransformationStep {
	return &models.TransformationStep{
		StepName: "Deliver ADT",
		StepType: "connector.outbound",
		Enabled:  true,
		Config: map[string]interface{}{
			"connectorType": "tcp_mllp_outbound",
			"config": map[string]interface{}{
				"host": host, "port": port, "connection_mode": "per-message",
				"ack_timeout_seconds": 2, "connect_timeout_seconds": 1, "max_retries": 0,
			},
			"contentField": "raw",
			"contentType":  "application/hl7-v2",
		},
	}
}

const adtRaw = "MSH|^~\\&|A|B|C|D|20260913||ADT^A01|1|P|2.5"

func TestOutboundConnector_FailedDelivery_StillStoresTheAttemptedPayload(t *testing.T) {
	// A port nothing listens on: the connection is refused, so delivery fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	ctx, stored := recordingStoreCtx()
	_, execErr := NewOutboundConnectorExecutor().Execute(ctx, mllpStep("127.0.0.1", port), map[string]interface{}{"raw": adtRaw})
	if execErr == nil {
		t.Fatal("expected the delivery to fail (nothing is listening)")
	}

	got := stored()
	if len(got) != 1 {
		t.Fatalf("the attempted payload must be stored once even though delivery failed; stored %d time(s)", len(got))
	}
	if got[0].content != adtRaw {
		t.Errorf("stored content = %q, want the exact payload that was attempted (%q)", got[0].content, adtRaw)
	}
	if got[0].interfaceID != "iface-1" || got[0].messageID != "msg-1" {
		t.Errorf("stored under %s/%s, want iface-1/msg-1", got[0].interfaceID, got[0].messageID)
	}
}

func TestOutboundConnector_SuccessfulDelivery_StoresThePayloadExactlyOnce(t *testing.T) {
	host, port := splitHostPort(t, fakeMLLPServer(t, "MSH|^~\\&|S|F|C|F|20260913||ACK|1|P|2.5\rMSA|AA|1\r"))

	ctx, stored := recordingStoreCtx()
	if _, err := NewOutboundConnectorExecutor().Execute(ctx, mllpStep(host, port), map[string]interface{}{"raw": adtRaw}); err != nil {
		t.Fatalf("expected successful delivery: %v", err)
	}
	if got := stored(); len(got) != 1 {
		t.Fatalf("a delivered payload must be stored exactly once (not before AND after the send); stored %d time(s)", len(got))
	}
}
