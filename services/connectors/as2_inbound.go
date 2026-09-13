// services/connectors/as2_inbound.go
// AS2 Inbound Connector — a persistent HTTPS listener that receives a
// trading partner's signed+encrypted EDI message, decrypts and verifies it,
// enqueues the plaintext content the same way every other inbound connector
// does, and returns a signed synchronous MDN in the SAME HTTP response —
// see as2_smime.go/as2_mdn.go for the shared crypto/MDN mechanics this
// connector is built on.
//
// Structurally mirrors http_fhir_inbound.go's own HTTP-listener lifecycle
// (own http.Server, TLS config, ListenAndServeTLS-in-goroutine-then-block-
// on-ctx.Done()) rather than edi_x12_inbound.go's SFTP-poller shape — see
// this project's own Phase 4 planning note for why AS2 is a new connector
// type, not a `transport` branch on the SFTP-only edi_x12_inbound.
//
// Every real outcome (success, decryption failure, signature failure, queue
// full) is communicated via the signed MDN's own Disposition field, with
// the HTTP status staying 200 — matching real-world AS2 server convention,
// where the transport layer succeeding (bytes were exchanged) is distinct
// from the application-layer outcome (was the message actually accepted).
//
// Configuration:
//
//	port                    int     Listener port (required)
//	base_path               string  URL path this connector listens on (default "/as2")
//	as2_from                string  Expected partner AS2 identifier (for MDN fields/logging)
//	as2_to                  string  This station's own AS2 identifier
//	own_cert_pem            string  PEM certificate used to sign the MDN + as the encryption recipient
//	own_key_pem             string  PEM private key matching own_cert_pem (decrypts incoming messages)
//	partner_cert_pem        string  Partner's PEM certificate — verifies their signature
//	tls_enabled             bool    Serve HTTPS (default false — real partner traffic should enable this)
//	tls_cert_file           string  Server TLS certificate path
//	tls_key_file            string  Server TLS key path
//	request_timeout_seconds int     Server read/write timeout (default 30)
package connectors

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// AS2InboundConnector implements an AS2 HTTPS receiver.
type AS2InboundConnector struct {
	*BaseInboundConnector

	port           int
	basePath       string
	as2From        string
	as2To          string
	ownCertPEM     string
	ownKeyPEM      string
	partnerCertPEM string
	tlsEnabled     bool
	tlsCertFile    string
	tlsKeyFile     string
	requestTimeout time.Duration

	own         smimeIdentity
	partnerCert *x509.Certificate

	server      *http.Server
	messageChan chan<- *models.InboundMessage
	mu          sync.RWMutex
}

// NewAS2InboundConnector creates a production AS2 inbound connector.
func NewAS2InboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "as2_inbound",
		DisplayName:        "AS2 Inbound",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_tls":   true,
			"supports_smime": true,
			"supports_mdn":   true, // synchronous MDN only — async MDN is a named future item
		},
	}
	return &AS2InboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
		basePath:             "/as2",
		requestTimeout:       30 * time.Second,
	}
}

// Initialize parses configuration and pre-parses certificates/key.
func (c *AS2InboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.port = cfg.GetInt("port")
	if v := cfg.GetString("base_path"); v != "" {
		c.basePath = v
	}
	c.as2From = cfg.GetString("as2_from")
	c.as2To = cfg.GetString("as2_to")
	c.ownCertPEM = cfg.GetString("own_cert_pem")
	c.ownKeyPEM = cfg.GetString("own_key_pem")
	c.partnerCertPEM = cfg.GetString("partner_cert_pem")
	c.tlsEnabled = cfg.GetBool("tls_enabled")
	c.tlsCertFile = cfg.GetString("tls_cert_file")
	c.tlsKeyFile = cfg.GetString("tls_key_file")

	timeoutSec := cfg.GetInt("request_timeout_seconds")
	if timeoutSec == 0 {
		timeoutSec = 30
	}
	c.requestTimeout = time.Duration(timeoutSec) * time.Second

	if c.ownCertPEM != "" && c.ownKeyPEM != "" {
		cert, err := parsePEMCertificate(c.ownCertPEM)
		if err != nil {
			return NewConnectorError(c.GetMetadata().TypeName, "initialize", fmt.Errorf("own_cert_pem: %w", err), false)
		}
		key, err := parsePEMPrivateKey(c.ownKeyPEM)
		if err != nil {
			return NewConnectorError(c.GetMetadata().TypeName, "initialize", fmt.Errorf("own_key_pem: %w", err), false)
		}
		c.own = smimeIdentity{cert: cert, key: key}
	}
	if c.partnerCertPEM != "" {
		cert, err := parsePEMCertificate(c.partnerCertPEM)
		if err != nil {
			return NewConnectorError(c.GetMetadata().TypeName, "initialize", fmt.Errorf("partner_cert_pem: %w", err), false)
		}
		c.partnerCert = cert
	}

	if c.tlsEnabled && (c.tlsCertFile == "" || c.tlsKeyFile == "") {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize", fmt.Errorf("tls_enabled but tls_cert_file and/or tls_key_file not specified"), false)
	}

	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("base_path", c.basePath)
	c.SetMetadata("as2_to", c.as2To)
	return nil
}

// Validate checks configuration completeness.
func (c *AS2InboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	typeName := c.GetMetadata().TypeName
	if c.port == 0 {
		return NewConnectorError(typeName, "validate", fmt.Errorf("port is required"), false)
	}
	if c.as2To == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("as2_to is required"), false)
	}
	if c.own.cert == nil || c.own.key == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("own_cert_pem and own_key_pem are required"), false)
	}
	if c.partnerCert == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("partner_cert_pem is required"), false)
	}
	return nil
}

// TestConnection has nothing meaningful to check for a listener that isn't
// running yet — mirrors http_fhir_inbound.go's own posture (a real
// connectivity test only happens once a partner actually POSTs to it).
func (c *AS2InboundConnector) TestConnection(ctx context.Context) error {
	return nil
}

// Start begins listening for AS2 POSTs.
func (c *AS2InboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.mu.Lock()
	c.messageChan = messageChan
	c.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(c.basePath, c.handleAS2Request)

	c.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", c.port),
		Handler:      mux,
		ReadTimeout:  c.requestTimeout,
		WriteTimeout: c.requestTimeout,
		IdleTimeout:  60 * time.Second,
	}

	c.SetState(StateRunning)

	go func() {
		scheme := "HTTP"
		if c.tlsEnabled {
			scheme = "HTTPS"
		}
		log.Printf("🔐 AS2 Inbound: listening on :%d%s (%s)", c.port, c.basePath, scheme)
		var err error
		if c.tlsEnabled {
			err = c.server.ListenAndServeTLS(c.tlsCertFile, c.tlsKeyFile)
		} else {
			err = c.server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("❌ AS2 Inbound: server error: %v", err)
		}
	}()

	<-ctx.Done()
	return c.Stop()
}

// Stop gracefully shuts down the listener.
func (c *AS2InboundConnector) Stop() error {
	if c.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Printf("🛑 AS2 Inbound: shutting down on port %d", c.port)
	return c.server.Shutdown(ctx)
}

// handleAS2Request decrypts+verifies an incoming AS2 message, enqueues it,
// and always responds with a signed MDN describing the real outcome — the
// HTTP status stays 200 regardless (the transport succeeded; the
// application-level result lives in the MDN body), matching real AS2
// server convention.
func (c *AS2InboundConnector) handleAS2Request(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "AS2 requires POST", http.StatusMethodNotAllowed)
		return
	}

	messageID := r.Header.Get("Message-ID")
	if messageID == "" {
		messageID = fmt.Sprintf("<unknown-%d>", time.Now().UnixNano())
	}
	partnerFrom := r.Header.Get("AS2-From")

	body, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024))
	if err != nil {
		c.respondMDN(w, messageID, partnerFrom, "failed/error: unexpected-processing-error", fmt.Sprintf("could not read request body: %v", err))
		return
	}

	content, err := openAS2Envelope(body, c.own, c.partnerCert)
	if err != nil {
		c.RecordError(err)
		log.Printf("⚠️  AS2 Inbound: decrypt/verify failed for %s from %s: %v", messageID, partnerFrom, err)
		c.respondMDN(w, messageID, partnerFrom, "failed/error: decryption-failed", fmt.Sprintf("could not decrypt or verify message: %v", err))
		return
	}

	msg := &models.InboundMessage{
		MessageID:      messageID,
		Content:        string(content),
		ContentType:    "application/edi-x12",
		SourceType:     "as2_inbound",
		SourceEndpoint: r.RemoteAddr,
		SourceIP:       r.RemoteAddr,
		ReceivedAt:     time.Now(),
		MessageSize:    len(content),
		Headers: map[string]string{
			"AS2-From":   partnerFrom,
			"AS2-To":     r.Header.Get("AS2-To"),
			"Message-ID": messageID,
		},
	}

	c.mu.RLock()
	ch := c.messageChan
	c.mu.RUnlock()

	select {
	case ch <- msg:
		c.IncrementMessagesReceived()
		c.respondMDN(w, messageID, partnerFrom, "processed", "The message was received and processed successfully.")
	default:
		c.RecordError(fmt.Errorf("message queue full"))
		c.respondMDN(w, messageID, partnerFrom, "failed/error: unsupported-processing-mode", "Message queue full — please retry.")
	}
}

// respondMDN builds+signs an MDN for messageID and writes it as the HTTP
// response body with the correct AS2 content-type, always returning 200 —
// see this file's own header comment for why the HTTP status doesn't vary
// with the application-level outcome. respondingTo is the original
// sender's own AS2-From value (this response's AS2-To — we're now the one
// sending, they're now the recipient).
func (c *AS2InboundConnector) respondMDN(w http.ResponseWriter, originalMessageID, respondingTo, disposition, humanText string) {
	signed, contentType, err := buildSignedMDN(originalMessageID, c.as2To, disposition, humanText, c.own)
	if err != nil {
		log.Printf("❌ AS2 Inbound: failed to build MDN: %v", err)
		http.Error(w, "failed to build MDN", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("AS2-From", c.as2To)
	w.Header().Set("AS2-To", respondingTo)
	w.WriteHeader(http.StatusOK)
	w.Write(signed)
}
