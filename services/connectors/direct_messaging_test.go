// services/connectors/direct_messaging_test.go
// Direct Messaging inbound/outbound tests — reuses as2_test_helpers_test.go's
// own generateTestIdentity/pemCert/pemKey helpers directly (already generic,
// not AS2-specific despite the file name) since Direct Messaging shares the
// exact same S/MIME primitives (smime_shared.go). No live DirectTrust
// partner exists to test against — matching AS2's own precedent, round-trip
// crypto proof comes from self-generated test certs, not a real HISP.
package connectors

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/mail"
	"strings"
	"testing"
)

func directMessagingOutboundConfig(t *testing.T, own, partner smimeIdentity, recipient string) []byte {
	t.Helper()
	cfg := map[string]interface{}{
		"smtp_host":         "smtp.example.org",
		"smtp_port":         587,
		"username":          "sender@direct.example.org",
		"password":          "secret",
		"cert_content":      pemCert(t, own),
		"private_key":       pemKey(t, own),
		"partner_cert_pem":  pemCert(t, partner),
		"recipient_address": recipient,
	}
	return marshalJSON(t, cfg)
}

func directMessagingInboundConfig(t *testing.T, own, partner smimeIdentity) []byte {
	t.Helper()
	cfg := map[string]interface{}{
		"imap_host":                 "imap.example.org",
		"imap_port":                 993,
		"username":                  "recipient@direct.example.org",
		"password":                  "secret",
		"cert_content":              pemCert(t, own),
		"private_key":               pemKey(t, own),
		"partner_cert_pem":          pemCert(t, partner),
		"polling_interval_seconds":  60,
	}
	return marshalJSON(t, cfg)
}

func marshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling test config: %v", err)
	}
	return b
}

func TestDirectMessagingOutbound_Initialize_ParsesConfigAndCerts(t *testing.T) {
	sender := generateTestIdentity(t, "Sender Direct Address")
	recipient := generateTestIdentity(t, "Recipient Direct Address")

	c := NewDirectMessagingOutboundConnector().(*DirectMessagingOutboundConnector)
	if err := c.Initialize(directMessagingOutboundConfig(t, sender, recipient, "recipient@direct.example.org")); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if c.smtpHost != "smtp.example.org" || c.smtpPort != 587 {
		t.Errorf("smtp_host/smtp_port not parsed correctly: %q %d", c.smtpHost, c.smtpPort)
	}
	if c.own.cert == nil || c.own.key == nil {
		t.Error("own identity not parsed")
	}
	if c.partnerCert == nil {
		t.Error("partner certificate not parsed")
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass with a complete config: %v", err)
	}
}

func TestDirectMessagingOutbound_Validate_RequiresRecipientAddress(t *testing.T) {
	sender := generateTestIdentity(t, "Sender")
	recipient := generateTestIdentity(t, "Recipient")

	c := NewDirectMessagingOutboundConnector().(*DirectMessagingOutboundConnector)
	if err := c.Initialize(directMessagingOutboundConfig(t, sender, recipient, "")); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when recipient_address is empty")
	}
}

func TestDirectMessagingOutbound_Validate_RequiresPartnerCert(t *testing.T) {
	sender := generateTestIdentity(t, "Sender")

	c := NewDirectMessagingOutboundConnector().(*DirectMessagingOutboundConnector)
	cfg := map[string]interface{}{
		"smtp_host":         "smtp.example.org",
		"username":          "sender@direct.example.org",
		"password":          "secret",
		"cert_content":      pemCert(t, sender),
		"private_key":       pemKey(t, sender),
		"recipient_address": "recipient@direct.example.org",
	}
	if err := c.Initialize(marshalJSON(t, cfg)); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when partner_cert_pem is missing")
	}
}

// TestDirectMessagingRoundTrip_BuildThenProcess proves the full, real,
// wire-level round trip: outbound's own buildDirectEmailBody produces a MIME
// email that inbound's own processEmail can parse, base64-decode,
// decrypt, AND verify — recovering the exact original clinical document.
// This is the direct proof that the two connectors' own email format
// assumptions (base64 S/MIME body, application/pkcs7-mime content type)
// genuinely agree with each other, not just individually plausible.
func TestDirectMessagingRoundTrip_BuildThenProcess(t *testing.T) {
	sender := generateTestIdentity(t, "Sender Direct Address")
	recipient := generateTestIdentity(t, "Recipient Direct Address")
	clinicalDoc := `<?xml version="1.0"?><ClinicalDocument>real C-CDA content</ClinicalDocument>`

	envelope, err := buildAS2Envelope([]byte(clinicalDoc), sender, recipient.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope failed: %v", err)
	}
	rawEmail := buildDirectEmailBody(
		"sender@direct.example.org", "recipient@direct.example.org",
		"Clinical Document", "<abc123@direct.example.org>", envelope,
	)

	// Prepend the extra headers a real mail transport adds so processEmail's
	// own net/mail.ReadMessage parse is exercised the same way it would be
	// against a real fetched message (Received:, Return-Path:, etc. before
	// the sender's own headers) — proves header-order robustness, not just
	// the happy path of parsing exactly what was built.
	fullRaw := append([]byte("Received: from mx.example.org\r\n"), rawEmail...)

	inbound := &DirectMessagingInboundConnector{own: recipient, partnerCert: sender.cert}
	msg, err := inbound.processEmail(fullRaw)
	if err != nil {
		t.Fatalf("processEmail failed: %v", err)
	}
	if msg.Content != clinicalDoc {
		t.Errorf("recovered content = %q, want %q", msg.Content, clinicalDoc)
	}
	if msg.SourceType != "direct_messaging_inbound" {
		t.Errorf("SourceType = %q, want direct_messaging_inbound", msg.SourceType)
	}
	if msg.Headers["subject"] != "Clinical Document" {
		t.Errorf("Headers[subject] = %q, want %q", msg.Headers["subject"], "Clinical Document")
	}
}

// TestDirectMessagingRoundTrip_WrongSigner_Rejected proves the same
// direct-trust enforcement AS2 already has: a message genuinely encrypted
// for the real recipient, but SIGNED by an unrelated third party, must be
// rejected — accepting "signed by somebody" instead of specifically the
// configured partner would defeat the whole point of verifying it.
func TestDirectMessagingRoundTrip_WrongSigner_Rejected(t *testing.T) {
	impostor := generateTestIdentity(t, "Impostor")
	recipient := generateTestIdentity(t, "Recipient Direct Address")
	realPartner := generateTestIdentity(t, "Real Partner")

	envelope, err := buildAS2Envelope([]byte("forged content"), impostor, recipient.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope failed: %v", err)
	}
	rawEmail := buildDirectEmailBody("impostor@evil.example", "recipient@direct.example.org", "Subject", "<x@evil.example>", envelope)

	inbound := &DirectMessagingInboundConnector{own: recipient, partnerCert: realPartner.cert}
	if _, err := inbound.processEmail(rawEmail); err == nil {
		t.Error("expected processEmail to reject a message signed by an untrusted party")
	}
}

func TestDirectMessagingInbound_Initialize_ParsesConfigAndCerts(t *testing.T) {
	own := generateTestIdentity(t, "Recipient")
	partner := generateTestIdentity(t, "Sender")

	c := NewDirectMessagingInboundConnector().(*DirectMessagingInboundConnector)
	if err := c.Initialize(directMessagingInboundConfig(t, own, partner)); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if c.imapHost != "imap.example.org" || c.imapPort != 993 {
		t.Errorf("imap_host/imap_port not parsed correctly: %q %d", c.imapHost, c.imapPort)
	}
	if c.own.cert == nil || c.own.key == nil {
		t.Error("own identity not parsed")
	}
	if c.partnerCert == nil {
		t.Error("partner certificate not parsed")
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass with a complete config: %v", err)
	}
	if !c.SupportsCron() {
		t.Error("SupportsCron should be true — this is a poll-mode connector")
	}
}

func TestDirectMessagingInbound_Validate_RequiresPartnerCert(t *testing.T) {
	own := generateTestIdentity(t, "Recipient")

	c := NewDirectMessagingInboundConnector().(*DirectMessagingInboundConnector)
	cfg := map[string]interface{}{
		"imap_host":    "imap.example.org",
		"username":     "recipient@direct.example.org",
		"password":     "secret",
		"cert_content": pemCert(t, own),
		"private_key":  pemKey(t, own),
	}
	if err := c.Initialize(marshalJSON(t, cfg)); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when partner_cert_pem is missing")
	}
}

func TestWrapBase64_RoundTripsAndWrapsAt76Chars(t *testing.T) {
	data := []byte(strings.Repeat("healthcare interop ", 20))
	wrapped := wrapBase64(data)
	for _, line := range strings.Split(strings.TrimSuffix(wrapped, "\r\n"), "\r\n") {
		if len(line) > 76 {
			t.Errorf("line exceeds 76 chars: %d", len(line))
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(wrapped))
	if err != nil {
		t.Fatalf("decoding wrapped base64 failed: %v", err)
	}
	if string(decoded) != string(data) {
		t.Errorf("round-tripped data mismatch: got %q, want %q", decoded, data)
	}
}

func TestStripWhitespace_RemovesSpacesTabsAndNewlines(t *testing.T) {
	got := stripWhitespace("ab\r\ncd \t ef\r\n")
	if got != "abcdef" {
		t.Errorf("stripWhitespace = %q, want %q", got, "abcdef")
	}
}

// TestDirectMessagingInbound_ProcessEmail_MalformedBody_ReturnsError proves a
// non-base64 body degrades gracefully (a logged skip in the real poll loop)
// rather than panicking.
func TestDirectMessagingInbound_ProcessEmail_MalformedBody_ReturnsError(t *testing.T) {
	own := generateTestIdentity(t, "Recipient")
	partner := generateTestIdentity(t, "Sender")
	raw := []byte("From: sender@example.org\r\nTo: recipient@example.org\r\n\r\nnot valid base64!!!")

	inbound := &DirectMessagingInboundConnector{own: own, partnerCert: partner.cert}
	if _, err := inbound.processEmail(raw); err == nil {
		t.Error("expected processEmail to return an error for a non-base64 body")
	}
}

// verify net/mail actually parses what buildDirectEmailBody produces, as a
// direct check on the header-construction format itself (independent of the
// S/MIME crypto layer already proven above).
func TestBuildDirectEmailBody_ProducesParsableMIMEHeaders(t *testing.T) {
	body := buildDirectEmailBody("a@example.org", "b@example.org", "Test Subject", "<mid@example.org>", []byte("envelope-bytes"))
	m, err := mail.ReadMessage(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("net/mail failed to parse buildDirectEmailBody's own output: %v", err)
	}
	if got := m.Header.Get("Subject"); got != "Test Subject" {
		t.Errorf("Subject header = %q, want %q", got, "Test Subject")
	}
	if got := m.Header.Get("Content-Type"); !strings.Contains(got, "application/pkcs7-mime") {
		t.Errorf("Content-Type header = %q, want it to contain application/pkcs7-mime", got)
	}
}
