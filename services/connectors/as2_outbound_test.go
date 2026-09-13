// services/connectors/as2_outbound_test.go
// Connector-level tests for AS2OutboundConnector using httptest.Server to
// simulate a real trading partner endpoint — no live AS2 partner exists to
// test against (same documented limitation as sftp_outbound_test.go's own
// "no live SFTP server in this environment").
package connectors

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

// buildAS2OutboundConnector constructs+initializes a real AS2OutboundConnector
// against a test partner endpoint.
func buildAS2OutboundConnector(t *testing.T, partnerURL string, own smimeIdentity, partnerCert smimeIdentity, requestMDN bool) *AS2OutboundConnector {
	t.Helper()
	cfg := map[string]interface{}{
		"partner_url":      partnerURL,
		"as2_from":         "acme-provider",
		"as2_to":           "acme-payer",
		"own_cert_pem":     pemCert(t, own),
		"own_key_pem":      pemKey(t, own),
		"partner_cert_pem": pemCert(t, partnerCert),
		"request_mdn":      requestMDN,
		"timeout_seconds":  10,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	conn := NewAS2OutboundConnector().(*AS2OutboundConnector)
	if err := conn.Initialize(b); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := conn.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return conn
}

// newTestAS2PartnerServer simulates a real AS2 trading partner: decrypts +
// verifies the incoming request with partner's own identity, checks it was
// signed by expectedSender, and returns a signed MDN. onReceived (optional)
// lets a test inspect the recovered plaintext content.
func newTestAS2PartnerServer(t *testing.T, partner smimeIdentity, expectedSender smimeIdentity, onReceived func(content []byte)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		content, err := openAS2Envelope(full, partner, expectedSender.cert)
		messageID := r.Header.Get("Message-ID")
		if err != nil {
			signed, contentType, mdnErr := buildSignedMDN(messageID, "acme-provider", "failed/error: decryption-failed", err.Error(), partner)
			if mdnErr != nil {
				http.Error(w, mdnErr.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			w.Write(signed)
			return
		}

		if onReceived != nil {
			onReceived(content)
		}

		signed, contentType, err := buildSignedMDN(messageID, "acme-provider", "processed", "Received and processed.", partner)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(signed)
	}))
}

func TestAS2Outbound_Send_HappyPath_MDNVerified(t *testing.T) {
	provider := generateTestIdentity(t, "Acme Provider") // us
	payer := generateTestIdentity(t, "Acme Payer")       // the partner

	var received []byte
	server := newTestAS2PartnerServer(t, payer, provider, func(content []byte) { received = content })
	defer server.Close()

	conn := buildAS2OutboundConnector(t, server.URL, provider, payer, true)

	message := &models.OutboundMessage{
		MessageID: "test-msg-1",
		Content:   "ST*270*0001~SE*2*0001~",
	}
	result, err := conn.Send(context.Background(), message)
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true, got false with error %q", result.ErrorMessage)
	}
	if !strings.Contains(result.Acknowledgment, "processed") {
		t.Errorf("Acknowledgment = %q, want it to contain \"processed\"", result.Acknowledgment)
	}
	if string(received) != message.Content {
		t.Errorf("partner recovered content = %q, want %q", received, message.Content)
	}
}

func TestAS2Outbound_Send_MDNMessageIDMismatch_FailsResult(t *testing.T) {
	provider := generateTestIdentity(t, "Acme Provider")
	payer := generateTestIdentity(t, "Acme Payer")

	// A rogue/misbehaving partner returns a validly-signed MDN, but for a
	// DIFFERENT (wrong) Original-Message-ID — must not be accepted as proof
	// of delivery for the message we actually sent.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signed, contentType, err := buildSignedMDN("<some-other-message@elsewhere>", "acme-provider", "processed", "ok", payer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(signed)
	}))
	defer server.Close()

	conn := buildAS2OutboundConnector(t, server.URL, provider, payer, true)

	result, err := conn.Send(context.Background(), &models.OutboundMessage{MessageID: "test-msg-2", Content: "content"})
	if err != nil {
		t.Fatalf("Send returned unexpected transport error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false for a mismatched Original-Message-ID, got true")
	}
	if !strings.Contains(result.ErrorMessage, "does not match") {
		t.Errorf("ErrorMessage = %q, want it to mention the Message-ID mismatch", result.ErrorMessage)
	}
}

func TestAS2Outbound_Send_MDNSignedByWrongParty_FailsResult(t *testing.T) {
	provider := generateTestIdentity(t, "Acme Provider")
	payer := generateTestIdentity(t, "Acme Payer")
	imposter := generateTestIdentity(t, "Imposter")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		messageID := r.Header.Get("Message-ID")
		// Signed by "imposter", not the configured partner "payer".
		signed, contentType, err := buildSignedMDN(messageID, "acme-provider", "processed", "ok", imposter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(signed)
	}))
	defer server.Close()

	conn := buildAS2OutboundConnector(t, server.URL, provider, payer, true)

	result, err := conn.Send(context.Background(), &models.OutboundMessage{MessageID: "test-msg-3", Content: "content"})
	if err != nil {
		t.Fatalf("Send returned unexpected transport error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false for an MDN signed by an untrusted party, got true")
	}
}

// TestAS2Outbound_TestConnection_ReachableWith404IsSuccess is a regression
// test for a real bug found via full-stack testing against the live app:
// httpservice.HTTPClientService.Execute treats ANY non-2xx response as a Go
// error, which made TestConnection's own "only fail on 5xx" intent
// unreachable dead code — a real AS2 partner endpoint commonly rejects a
// bare HEAD probe with 404/405 (it only implements POST), which must still
// count as "reachable," not "connection failed."
func TestAS2Outbound_TestConnection_ReachableWith404IsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	own := generateTestIdentity(t, "Acme Provider")
	partner := generateTestIdentity(t, "Acme Payer")
	conn := buildAS2OutboundConnector(t, server.URL, own, partner, false)

	if err := conn.TestConnection(context.Background()); err != nil {
		t.Errorf("expected a 404 response to still count as reachable, got error: %v", err)
	}
}

func TestAS2Outbound_TestConnection_TrulyUnreachable_Fails(t *testing.T) {
	own := generateTestIdentity(t, "Acme Provider")
	partner := generateTestIdentity(t, "Acme Payer")
	// Port 1 is reserved/unlikely to have anything listening — a genuine
	// connection failure, distinct from a reachable-but-4xx endpoint above.
	conn := buildAS2OutboundConnector(t, "http://127.0.0.1:1/as2", own, partner, false)

	if err := conn.TestConnection(context.Background()); err == nil {
		t.Error("expected a genuinely unreachable endpoint to fail TestConnection, got nil error")
	}
}

func TestAS2Outbound_Validate_RequiresCertificates(t *testing.T) {
	conn := NewAS2OutboundConnector().(*AS2OutboundConnector)
	cfg := map[string]interface{}{
		"partner_url": "https://partner.example.com/as2",
		"as2_from":    "us",
		"as2_to":      "them",
	}
	b, _ := json.Marshal(cfg)
	if err := conn.Initialize(b); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := conn.Validate(); err == nil {
		t.Error("expected Validate to fail without own/partner certificates configured")
	}
}
