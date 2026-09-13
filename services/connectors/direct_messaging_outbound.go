// services/connectors/direct_messaging_outbound.go
// Direct Messaging (DirectTrust) Outbound Connector — signs+encrypts an
// outgoing clinical document as S/MIME (the exact same CMS SignedData
// wrapped in CMS EnvelopedData shape as2_outbound.go uses — see
// smime_shared.go's own header comment: DirectTrust's own Applicability
// Statement, like AS2's RFC 4130, specifies the identical "sign then
// encrypt" CMS layering) and delivers it as an email via SMTP.
//
// Unlike AS2 (raw binary over HTTP, which is fully binary-safe), the S/MIME
// envelope here is base64-encoded before being placed in the email body —
// SMTP has no such binary-safety guarantee across arbitrary relays, and
// base64 is the universal, real-world S/MIME-over-email convention every
// Direct Trust HISP actually uses.
//
// Like as2_outbound.go and edi_x12_outbound.go, this stays a transport-only
// "dumb byte shipper" for the CONTENT itself: it signs/encrypts/delivers
// whatever `message.Content` already is (a C-CDA document, built upstream by
// the pipeline) — no CDA/FHIR knowledge lives here. MDN-over-email
// (DirectTrust delivery/read receipts) is a named, deferred item — this
// connector proves delivery only by the SMTP transaction succeeding, the
// same base guarantee any SMTP sender has.
//
// Configuration:
//
//	smtp_host          string  SMTP host
//	smtp_port          int     SMTP port (default 587)
//	use_tls            bool    Opportunistic STARTTLS when the server offers
//	                           it (default true) — this path also performs
//	                           real SMTP AUTH. false forces a plain-text send
//	                           AND SKIPS AUTH ENTIRELY (stdlib smtp.PlainAuth
//	                           itself refuses to send credentials over a
//	                           non-TLS, non-localhost connection — a correct
//	                           security guard, not a bug to route around) —
//	                           for a controlled test server only; every real
//	                           Direct Trust deployment must leave this true.
//	username           string  Sender's own Direct Address (also the SMTP
//	                           AUTH username and the email's own From:)
//	password           string  SMTP AUTH password
//	cert_content       string  PEM certificate used to SIGN outgoing messages
//	private_key        string  PEM private key matching cert_content
//	partner_cert_pem   string  Recipient's PEM certificate — used to ENCRYPT
//	                           outgoing messages (direct-trust model, same as
//	                           AS2 — no DNS CERT-record/LDAP auto-discovery)
//	recipient_address  string  Default recipient Direct Address — a per-
//	                           message override may be supplied via
//	                           message.DestinationConfig["recipient_address"]
package connectors

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"ezhealthkonnect/models"

	"github.com/google/uuid"
)

// DirectMessagingOutboundConnector signs, encrypts, and emails a clinical
// document to a Direct Trust recipient over SMTP.
type DirectMessagingOutboundConnector struct {
	*BaseOutboundConnector

	smtpHost          string
	smtpPort          int
	useTLS            bool
	username          string
	password          string
	certPEM           string
	keyPEM            string
	partnerCertPEM    string
	recipientAddress  string

	own         smimeIdentity
	partnerCert *x509.Certificate
}

// NewDirectMessagingOutboundConnector creates a production Direct Messaging
// outbound connector.
func NewDirectMessagingOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "direct_messaging_outbound",
		DisplayName:        "Direct Messaging Outbound",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_smime": true,
			"supports_mdn":   false, // MDN-over-email is a named, deferred item
		},
	}
	return &DirectMessagingOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration and pre-parses the certificates/key so
// Send() never re-parses PEM material per call.
func (c *DirectMessagingOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.smtpHost = cfg.GetString("smtp_host")
	c.smtpPort = cfg.GetInt("smtp_port")
	if c.smtpPort == 0 {
		c.smtpPort = 587
	}
	c.useTLS = cfg.GetBoolDefault("use_tls", true)
	c.username = cfg.GetString("username")
	c.password = cfg.GetString("password")
	c.certPEM = cfg.GetString("cert_content")
	c.keyPEM = cfg.GetString("private_key")
	c.partnerCertPEM = cfg.GetString("partner_cert_pem")
	c.recipientAddress = cfg.GetString("recipient_address")

	own, partnerCert, err := parseSMIMEConfig(c.certPEM, c.keyPEM, c.partnerCertPEM)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize", err, false)
	}
	c.own = own
	c.partnerCert = partnerCert

	c.SetMetadata("smtp_host", c.smtpHost)
	c.SetMetadata("username", c.username)
	return nil
}

// Validate checks configuration completeness.
func (c *DirectMessagingOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	typeName := c.GetMetadata().TypeName
	if c.smtpHost == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("smtp_host is required"), false)
	}
	if c.username == "" || c.password == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("username and password are required"), false)
	}
	if c.own.cert == nil || c.own.key == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("cert_content and private_key are required"), false)
	}
	if c.partnerCert == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("partner_cert_pem is required"), false)
	}
	if c.recipientAddress == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("recipient_address is required"), false)
	}
	return nil
}

// TestConnection dials the SMTP host and issues a real EHLO/QUIT — proof the
// server is reachable and speaking SMTP, without sending a real message.
func (c *DirectMessagingOutboundConnector) TestConnection(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", c.smtpHost, c.smtpPort)
	client, err := smtp.Dial(addr)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	defer client.Close()
	if err := client.Hello("ezhealthkonnect"); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return client.Quit()
}

// Send signs+encrypts message.Content as an S/MIME envelope, wraps it as a
// base64 email body, and delivers it via SMTP.
func (c *DirectMessagingOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	if message.Content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	recipient := c.recipientAddress
	if message.DestinationConfig != nil {
		if v, ok := message.DestinationConfig["recipient_address"].(string); ok && v != "" {
			recipient = v
		}
	}
	if recipient == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("no recipient_address configured or provided"), false)
	}

	envelope, err := buildAS2Envelope([]byte(message.Content), c.own, c.partnerCert)
	if err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}

	subject := "Clinical Document"
	if message.MessageType != "" {
		subject = message.MessageType
	}
	directMessageID := fmt.Sprintf("<%s@%s>", uuid.New().String(), emailDomain(c.username))
	body := buildDirectEmailBody(c.username, recipient, subject, directMessageID, envelope)

	addr := fmt.Sprintf("%s:%d", c.smtpHost, c.smtpPort)
	if c.useTLS {
		auth := smtp.PlainAuth("", c.username, c.password, c.smtpHost)
		err = smtp.SendMail(addr, auth, c.username, []string{recipient}, body)
	} else {
		err = sendSMTPNoStartTLS(addr, c.username, recipient, body)
	}
	if err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}

	c.IncrementMessagesSent()
	return &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata: map[string]interface{}{
			"direct_message_id": directMessageID,
			"recipient":         recipient,
			"bytes_sent":        len(envelope),
		},
	}, nil
}

// buildDirectEmailBody assembles the complete, ready-to-send MIME email —
// headers plus a base64-wrapped S/MIME body — extracted as its own function
// so a test can verify the exact wire shape without a live SMTP send.
func buildDirectEmailBody(from, to, subject, messageID string, envelope []byte) []byte {
	var body bytes.Buffer
	fmt.Fprintf(&body, "From: %s\r\n", from)
	fmt.Fprintf(&body, "To: %s\r\n", to)
	fmt.Fprintf(&body, "Subject: %s\r\n", subject)
	fmt.Fprintf(&body, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&body, "Message-ID: %s\r\n", messageID)
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: application/pkcs7-mime; smime-type=enveloped-data; name=\"smime.p7m\"\r\n")
	body.WriteString("Content-Transfer-Encoding: base64\r\n")
	body.WriteString("Content-Disposition: attachment; filename=\"smime.p7m\"\r\n")
	body.WriteString("\r\n")
	body.WriteString(wrapBase64(envelope))
	return body.Bytes()
}

// emailDomain extracts the part after "@" in a Direct Address, falling back
// to a fixed placeholder domain for a Message-ID when the address has none
// (defensive only — a real Direct Address always has one).
func emailDomain(address string) string {
	if idx := strings.LastIndex(address, "@"); idx != -1 && idx < len(address)-1 {
		return address[idx+1:]
	}
	return "direct.local"
}

// wrapBase64 base64-encodes data and wraps it at 76 characters per line
// (RFC 2045 §6.8) — the standard MIME base64-body convention every real mail
// client/server expects.
func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var out strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		out.WriteString(encoded[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

// sendSMTPNoStartTLS sends msg via a plain (never-upgraded, never-
// authenticated) SMTP session — for a controlled test server only (see
// use_tls's own doc comment above; every real Direct Trust deployment must
// use smtp.SendMail's opportunistic-STARTTLS path instead, which does
// authenticate).
//
// Deliberately never attempts SMTP AUTH here, even when the server
// advertises it: stdlib smtp.PlainAuth's own Start() refuses to send
// credentials whenever the connection isn't TLS AND the server isn't
// literally "localhost" (net/smtp/auth.go's isLocalhost check) — a correct
// security guard against leaking a real password in the clear, found by
// this exact scenario (a real test mail server reached by container
// hostname, not "localhost", over a deliberately-plain connection) failing
// with "unencrypted connection" during full-stack verification. Rather than
// route around that stdlib protection, this path accepts the honest
// trade-off it names: use_tls=false means auth is skipped entirely, matching
// its own "test server only" scope — a real deployment needing real
// authentication has no reason to ever set use_tls=false in the first place.
func sendSMTPNoStartTLS(addr, from, to string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer client.Close()

	if err := client.Hello("ezhealthkonnect"); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing message: %w", err)
	}
	return client.Quit()
}
