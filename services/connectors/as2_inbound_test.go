// services/connectors/as2_inbound_test.go
// Connector-level tests for AS2InboundConnector's own HTTP handler — driven
// directly via httptest.NewRecorder against handleAS2Request (same package,
// no live listener needed) rather than a real bound TCP port, matching this
// project's own httptest-based connector-test convention.
package connectors

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ezhealthkonnect/models"
)

func buildAS2InboundConnector(t *testing.T, own smimeIdentity, partnerCert smimeIdentity) *AS2InboundConnector {
	t.Helper()
	cfg := map[string]interface{}{
		"port":             19999,
		"as2_from":         "acme-payer",
		"as2_to":           "acme-payer",
		"own_cert_pem":     pemCert(t, own),
		"own_key_pem":      pemKey(t, own),
		"partner_cert_pem": pemCert(t, partnerCert),
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	conn := NewAS2InboundConnector().(*AS2InboundConnector)
	if err := conn.Initialize(b); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := conn.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return conn
}

func TestAS2Inbound_HandleRequest_HappyPath_EnqueuesAndReturnsProcessedMDN(t *testing.T) {
	payer := generateTestIdentity(t, "Acme Payer")    // us
	provider := generateTestIdentity(t, "Acme Provider") // the partner

	conn := buildAS2InboundConnector(t, payer, provider)
	ch := make(chan *models.InboundMessage, 1)
	conn.mu.Lock()
	conn.messageChan = ch
	conn.mu.Unlock()

	content := []byte("ST*270*0001~SE*2*0001~")
	envelope, err := buildAS2Envelope(content, provider, payer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/as2", bytes.NewReader(envelope))
	req.Header.Set("Message-ID", "<test-1@provider>")
	req.Header.Set("AS2-From", "acme-provider")
	req.Header.Set("AS2-To", "acme-payer")
	rec := httptest.NewRecorder()

	conn.handleAS2Request(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (AS2 always returns 200; outcome is in the MDN body)", rec.Code)
	}

	select {
	case msg := <-ch:
		if msg.Content != string(content) {
			t.Errorf("enqueued message content = %q, want %q", msg.Content, content)
		}
		if msg.SourceType != "as2_inbound" {
			t.Errorf("SourceType = %q, want as2_inbound", msg.SourceType)
		}
	default:
		t.Fatal("expected a message to be enqueued onto messageChan, got none")
	}

	disp, err := parseAndVerifyMDN(rec.Body.Bytes(), payer.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN on the response body: %v", err)
	}
	if disp.OriginalMessageID != "<test-1@provider>" {
		t.Errorf("MDN OriginalMessageID = %q, want <test-1@provider>", disp.OriginalMessageID)
	}
	if !disp.Processed() {
		t.Errorf("MDN Disposition = %q, want Processed() = true", disp.Disposition)
	}
}

func TestAS2Inbound_HandleRequest_GarbageBody_ReturnsFailedMDN_NoEnqueue(t *testing.T) {
	payer := generateTestIdentity(t, "Acme Payer")
	provider := generateTestIdentity(t, "Acme Provider")

	conn := buildAS2InboundConnector(t, payer, provider)
	ch := make(chan *models.InboundMessage, 1)
	conn.mu.Lock()
	conn.messageChan = ch
	conn.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/as2", bytes.NewReader([]byte("this is not a valid PKCS7 envelope at all")))
	req.Header.Set("Message-ID", "<garbage-1@provider>")
	rec := httptest.NewRecorder()

	conn.handleAS2Request(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even for a failed disposition", rec.Code)
	}

	select {
	case msg := <-ch:
		t.Fatalf("expected NO message enqueued for an undecryptable body, got: %+v", msg)
	default:
	}

	disp, err := parseAndVerifyMDN(rec.Body.Bytes(), payer.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN on the response body: %v", err)
	}
	if disp.Processed() {
		t.Error("expected Processed() = false for a garbage/undecryptable body")
	}
}

func TestAS2Inbound_HandleRequest_WrongSigner_Rejected(t *testing.T) {
	payer := generateTestIdentity(t, "Acme Payer")
	provider := generateTestIdentity(t, "Acme Provider")
	imposter := generateTestIdentity(t, "Imposter")

	conn := buildAS2InboundConnector(t, payer, provider) // trusts "provider", not "imposter"
	ch := make(chan *models.InboundMessage, 1)
	conn.mu.Lock()
	conn.messageChan = ch
	conn.mu.Unlock()

	envelope, err := buildAS2Envelope([]byte("forged content"), imposter, payer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/as2", bytes.NewReader(envelope))
	req.Header.Set("Message-ID", "<forged-1@imposter>")
	rec := httptest.NewRecorder()

	conn.handleAS2Request(rec, req)

	select {
	case msg := <-ch:
		t.Fatalf("expected NO message enqueued for a message signed by an untrusted party, got: %+v", msg)
	default:
	}

	disp, err := parseAndVerifyMDN(rec.Body.Bytes(), payer.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN on the response body: %v", err)
	}
	if disp.Processed() {
		t.Error("expected Processed() = false for a message signed by an untrusted party")
	}
}

func TestAS2Inbound_HandleRequest_QueueFull_ReturnsFailedMDN(t *testing.T) {
	payer := generateTestIdentity(t, "Acme Payer")
	provider := generateTestIdentity(t, "Acme Provider")

	conn := buildAS2InboundConnector(t, payer, provider)
	ch := make(chan *models.InboundMessage) // unbuffered, nobody reading — always full
	conn.mu.Lock()
	conn.messageChan = ch
	conn.mu.Unlock()

	envelope, err := buildAS2Envelope([]byte("content"), provider, payer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/as2", bytes.NewReader(envelope))
	req.Header.Set("Message-ID", "<full-queue-1@provider>")
	rec := httptest.NewRecorder()

	conn.handleAS2Request(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when the queue is full", rec.Code)
	}
	disp, err := parseAndVerifyMDN(rec.Body.Bytes(), payer.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN on the response body: %v", err)
	}
	if disp.Processed() {
		t.Error("expected Processed() = false when the message queue is full")
	}
}

func TestAS2Inbound_Validate_RequiresCertificates(t *testing.T) {
	conn := NewAS2InboundConnector().(*AS2InboundConnector)
	cfg := map[string]interface{}{
		"port":     19998,
		"as2_from": "them",
		"as2_to":   "us",
	}
	b, _ := json.Marshal(cfg)
	if err := conn.Initialize(b); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := conn.Validate(); err == nil {
		t.Error("expected Validate to fail without own/partner certificates configured")
	}
}
