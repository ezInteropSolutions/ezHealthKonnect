// services/connectors/direct_messaging_inbound.go
// Direct Messaging (DirectTrust) Inbound Connector — polls a mailbox over
// IMAP for new S/MIME-encrypted emails, decrypts+verifies each one against
// the configured partner certificate (the exact same direct-trust CMS
// decrypt+verify smime_shared.go already implements for AS2 — see that
// file's own header comment), and enqueues the recovered clinical document
// as a real InboundMessage.
//
// Shape: mode "pull" (poll, not a listening inbound SMTP server) — matches
// this connector's own pre-existing connectivity_types config schema
// (V69/V221), and mirrors postgresql_inbound.go's own polling-loop shape
// (SupportsCron true, a ticker-driven pollLoop with reconnect-with-backoff)
// far more closely than any listener-style connector — an IMAP session is
// connection-oriented and stateful (a persisted watermark, not a one-shot
// directory scan).
//
// Watermark: "highest UID seen so far" (an IMAP UID is a per-mailbox,
// permanently-increasing identifier — RFC 3501 §2.3.1.1), persisted via
// SetMetadata("last_uid", ...) after each successful poll, mirroring
// edi_x12_inbound.go's own SFTP-side "remember what's already been
// processed" discipline but keyed on UID instead of filename.
//
// Content-Transfer-Encoding scope: only base64 is decoded (the universal,
// real-world S/MIME-over-email convention — also what
// direct_messaging_outbound.go itself always sends) — a message using a
// different encoding fails to base64-decode and is skipped with a logged
// warning rather than silently corrupted. MDN-over-email (DirectTrust
// delivery/read receipts) is a named, deferred item — this connector
// enqueues every successfully decrypted message as a new clinical document,
// with no attempt to distinguish an MDN receipt from one.
//
// Configuration:
//
//	imap_host                  string  IMAP host
//	imap_port                  int     IMAP port (default 993)
//	use_tls                    bool    Implicit TLS / IMAPS (default true) —
//	                                   false connects in plain text, for a
//	                                   controlled test mailbox only.
//	username                   string  This mailbox's own Direct Address
//	password                   string  IMAP password
//	cert_content               string  PEM certificate — the encryption
//	                                   recipient for incoming messages
//	private_key                string  PEM private key matching cert_content
//	                                   — decrypts incoming messages
//	partner_cert_pem           string  Sender's PEM certificate — verifies
//	                                   the signature on incoming messages
//	                                   (direct-trust model, same as AS2)
//	polling_interval_seconds   int     Seconds between polls (default 60)
package connectors

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/mail"
	"strings"
	"time"

	"ezhealthkonnect/models"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/google/uuid"
)

const directMessagingMaxReconnectDelay = 5 * time.Minute

// DirectMessagingInboundConnector polls an IMAP mailbox for new
// S/MIME-encrypted Direct Trust messages.
type DirectMessagingInboundConnector struct {
	*BaseInboundConnector

	imapHost         string
	imapPort         int
	useTLS           bool
	username         string
	password         string
	certPEM          string
	keyPEM           string
	partnerCertPEM   string
	pollingInterval  int

	own         smimeIdentity
	partnerCert *x509.Certificate

	lastUID uint32
}

// NewDirectMessagingInboundConnector creates a production Direct Messaging
// inbound connector.
func NewDirectMessagingInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "direct_messaging_inbound",
		DisplayName:        "Direct Messaging Inbound",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  true,
			"supports_tls":   true,
			"supports_smime": true,
			"supports_mdn":   false, // MDN-over-email is a named, deferred item
		},
	}
	return &DirectMessagingInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses configuration and pre-parses the certificates/key so
// poll() never re-parses PEM material per cycle.
func (d *DirectMessagingInboundConnector) Initialize(config []byte) error {
	if err := d.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := d.GetConfig()

	d.imapHost = cfg.GetString("imap_host")
	d.imapPort = cfg.GetInt("imap_port")
	if d.imapPort == 0 {
		d.imapPort = 993
	}
	d.useTLS = cfg.GetBoolDefault("use_tls", true)
	d.username = cfg.GetString("username")
	d.password = cfg.GetString("password")
	d.certPEM = cfg.GetString("cert_content")
	d.keyPEM = cfg.GetString("private_key")
	d.partnerCertPEM = cfg.GetString("partner_cert_pem")
	d.pollingInterval = cfg.GetInt("polling_interval_seconds")
	if d.pollingInterval == 0 {
		d.pollingInterval = 60
	}

	own, partnerCert, err := parseSMIMEConfig(d.certPEM, d.keyPEM, d.partnerCertPEM)
	if err != nil {
		return NewConnectorError(d.GetMetadata().TypeName, "initialize", err, false)
	}
	d.own = own
	d.partnerCert = partnerCert

	d.SetMetadata("imap_host", d.imapHost)
	d.SetMetadata("username", d.username)
	d.BaseConnector.initialized = true
	d.SetState(StateReady)
	return nil
}

// Validate checks configuration completeness.
func (d *DirectMessagingInboundConnector) Validate() error {
	if !d.BaseConnector.initialized {
		return NewConnectorError(d.GetMetadata().TypeName, "validate", fmt.Errorf("connector not initialized"), false)
	}
	typeName := d.GetMetadata().TypeName
	if d.imapHost == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("imap_host is required"), false)
	}
	if d.username == "" || d.password == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("username and password are required"), false)
	}
	if d.own.cert == nil || d.own.key == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("cert_content and private_key are required"), false)
	}
	if d.partnerCert == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("partner_cert_pem is required"), false)
	}
	return nil
}

// connect dials and logs into the IMAP server, returning a ready client.
func (d *DirectMessagingInboundConnector) connect() (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", d.imapHost, d.imapPort)
	var c *client.Client
	var err error
	if d.useTLS {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: d.imapHost})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	if err := c.Login(d.username, d.password); err != nil {
		c.Logout()
		return nil, fmt.Errorf("login: %w", err)
	}
	return c, nil
}

// TestConnection dials and logs in, then immediately logs out — proof the
// mailbox is reachable and the credentials work, without leaving a session
// open or consuming any messages.
func (d *DirectMessagingInboundConnector) TestConnection(ctx context.Context) error {
	c, err := d.connect()
	if err != nil {
		return NewConnectorError(d.GetMetadata().TypeName, "test_connection", err, true)
	}
	return c.Logout()
}

// SupportsCron returns true — IMAP poll mode supports cron scheduling.
func (d *DirectMessagingInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (d *DirectMessagingInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !d.BaseConnector.initialized {
		return NewConnectorError(d.GetMetadata().TypeName, "start", fmt.Errorf("connector not initialized"), false)
	}
	d.SetState(StateRunning)
	log.Printf("📧 Direct Messaging Inbound: Starting (interval=%ds)", d.pollingInterval)
	go d.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop polls immediately on start, then on each ticker interval, with
// exponential backoff reconnect on connection errors — same shape as
// postgresql_inbound.go's own pollLoop.
func (d *DirectMessagingInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer d.SetState(StateReady)

	ticker := time.NewTicker(time.Duration(d.pollingInterval) * time.Second)
	defer ticker.Stop()

	reconnectAttempts := 0

	runOnce := func() {
		if err := d.poll(messageChan); err != nil {
			d.RecordError(err)
			log.Printf("❌ Direct Messaging Inbound: Poll error: %v", err)
			if IsConnectionError(err) {
				reconnectAttempts++
				delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
				if delay > directMessagingMaxReconnectDelay {
					delay = directMessagingMaxReconnectDelay
				}
				log.Printf("🔄 Direct Messaging Inbound: Connection lost — retrying in %v (attempt %d)", delay, reconnectAttempts)
				d.SetState(StateError)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
				case <-d.stopCh:
				}
			}
		} else {
			d.ClearError()
			d.SetState(StateRunning)
			reconnectAttempts = 0
		}
	}

	runOnce()

	for {
		select {
		case <-ctx.Done():
			log.Printf("📧 Direct Messaging Inbound: Context cancelled")
			return
		case <-d.stopCh:
			log.Printf("📧 Direct Messaging Inbound: Stop signal received")
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

// poll connects, fetches every message with a UID greater than the last
// watermark, decrypts+verifies each one, and enqueues it as an
// InboundMessage — advancing the watermark only after a successful pass.
func (d *DirectMessagingInboundConnector) poll(messageChan chan<- *models.InboundMessage) error {
	c, err := d.connect()
	if err != nil {
		return err
	}
	defer c.Logout()

	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("select INBOX: %w", err)
	}
	if mbox.Messages == 0 {
		return nil
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(d.lastUID+1, 0) // 0 means "*" (through the highest UID)

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, section.FetchItem()}
	messages := make(chan *imap.Message, 10)
	fetchDone := make(chan error, 1)
	go func() { fetchDone <- c.UidFetch(seqSet, items, messages) }()

	maxUID := d.lastUID
	for msg := range messages {
		// Defensive re-check: IMAP's "N:*" range can, on some servers,
		// still surface the mailbox's single highest-UID message even when
		// its own UID is below N (RFC 3501's own documented "*" ambiguity
		// when N exceeds every real UID) — never re-process an already-seen
		// message regardless of what the range technically returned.
		if msg.Uid <= d.lastUID {
			continue
		}
		if msg.Uid > maxUID {
			maxUID = msg.Uid
		}

		literal := msg.GetBody(section)
		if literal == nil {
			log.Printf("⚠️  Direct Messaging Inbound: message UID %d has no body — skipping", msg.Uid)
			continue
		}
		raw, err := io.ReadAll(literal)
		if err != nil {
			log.Printf("⚠️  Direct Messaging Inbound: reading body for UID %d failed: %v", msg.Uid, err)
			continue
		}

		inboundMsg, err := d.processEmail(raw)
		if err != nil {
			log.Printf("⚠️  Direct Messaging Inbound: processing UID %d failed: %v", msg.Uid, err)
			continue
		}

		messageChan <- inboundMsg
		d.IncrementMessagesReceived()
	}
	if err := <-fetchDone; err != nil {
		return fmt.Errorf("uid fetch: %w", err)
	}

	if maxUID > d.lastUID {
		d.lastUID = maxUID
		d.SetMetadata("last_uid", fmt.Sprintf("%d", maxUID))
	}
	return nil
}

// processEmail parses a raw MIME email, base64-decodes its body, and
// decrypts+verifies the recovered S/MIME envelope against the configured
// partner certificate, returning the original clinical document as an
// InboundMessage.
func (d *DirectMessagingInboundConnector) processEmail(raw []byte) (*models.InboundMessage, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing MIME message: %w", err)
	}
	bodyBytes, err := io.ReadAll(m.Body)
	if err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}

	envelope, err := base64.StdEncoding.DecodeString(stripWhitespace(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("base64-decoding message body: %w", err)
	}

	content, err := openAS2Envelope(envelope, d.own, d.partnerCert)
	if err != nil {
		return nil, fmt.Errorf("decrypt/verify failed: %w", err)
	}

	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("direct_%s", uuid.New().String()),
		Content:        string(content),
		ContentType:    "application/xml",
		SourceType:     "direct_messaging_inbound",
		SourceEndpoint: fmt.Sprintf("%s:%d", d.imapHost, d.imapPort),
		ReceivedAt:     time.Now(),
		MessageSize:    len(content),
		Priority:       5,
		Headers: map[string]string{
			"from":       m.Header.Get("From"),
			"subject":    m.Header.Get("Subject"),
			"message_id": m.Header.Get("Message-ID"),
		},
	}, nil
}

// stripWhitespace removes every space/CR/LF/tab from s — RFC 2045 base64
// bodies are wrapped at 76 chars/line, which encoding/base64 will not accept
// unless the line breaks are removed first.
func stripWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\r', '\n', '\t':
			return -1
		}
		return r
	}, s)
}

// Stop halts the connector.
func (d *DirectMessagingInboundConnector) Stop() error {
	log.Printf("📧 Direct Messaging Inbound: Stopping")
	return d.BaseInboundConnector.Stop()
}

// Close is an alias for Stop.
func (d *DirectMessagingInboundConnector) Close() error { return d.Stop() }

// GetStatus returns connector status with Direct Messaging-specific metadata.
func (d *DirectMessagingInboundConnector) GetStatus() ConnectorStatus {
	status := d.BaseInboundConnector.GetStatus()
	status.Metadata["imap_host"] = d.imapHost
	status.Metadata["username"] = d.username
	status.Metadata["last_uid"] = fmt.Sprintf("%d", d.lastUID)
	return status
}
