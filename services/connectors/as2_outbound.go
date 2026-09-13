// services/connectors/as2_outbound.go
// AS2 Outbound Connector — signs+encrypts an outgoing EDI payload as S/MIME
// (CMS SignedData wrapped in CMS EnvelopedData — see as2_smime.go's own
// header comment for why this form and not multipart/signed), POSTs it to
// a trading partner's AS2 endpoint over HTTPS, and — when an MDN was
// requested — verifies the signed synchronous MDN response actually proves
// THIS message was received (Original-Message-ID must match what was sent;
// a validly-signed MDN for a DIFFERENT message is not acceptable proof of
// delivery for this one).
//
// Like edi_x12_outbound.go, this stays a transport-only "dumb byte
// shipper": it signs/encrypts/delivers whatever content it's given (built
// by the edi.build pipeline step) — no X12 envelope/business logic lives
// here.
//
// Configuration:
//
//	partner_url        string  Partner's AS2 receiving endpoint (https://...)
//	as2_from            string  This station's own AS2 identifier
//	as2_to              string  Partner's AS2 identifier
//	own_cert_pem        string  PEM certificate used to SIGN outgoing messages
//	own_key_pem         string  PEM private key matching own_cert_pem
//	partner_cert_pem    string  Partner's PEM certificate — used to ENCRYPT
//	                            outgoing messages and to VERIFY their MDN
//	request_mdn         bool    Request a signed synchronous MDN (default true)
//	timeout_seconds     int     HTTP timeout (default 60)
package connectors

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"ezhealthkonnect/models"
	httpservice "ezhealthkonnect/services/http"

	"github.com/google/uuid"
)

// AS2OutboundConnector signs, encrypts, and delivers EDI content to a
// trading partner's AS2 endpoint.
type AS2OutboundConnector struct {
	*BaseOutboundConnector

	partnerURL     string
	as2From        string
	as2To          string
	ownCertPEM     string
	ownKeyPEM      string
	partnerCertPEM string
	requestMDN     bool
	timeout        time.Duration

	own         smimeIdentity
	partnerCert *x509.Certificate
	httpClient  *httpservice.HTTPClientService
}

// NewAS2OutboundConnector creates a production AS2 outbound connector.
func NewAS2OutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "as2_outbound",
		DisplayName:        "AS2 Outbound",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_smime": true,
			"supports_mdn":   true, // synchronous MDN only — async MDN is a named future item
		},
	}
	return &AS2OutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, false),
	}
}

// Initialize parses configuration and pre-parses the certificates/key so
// Send() never re-parses PEM material per call.
func (c *AS2OutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.partnerURL = cfg.GetString("partner_url")
	c.as2From = cfg.GetString("as2_from")
	c.as2To = cfg.GetString("as2_to")
	c.ownCertPEM = cfg.GetString("own_cert_pem")
	c.ownKeyPEM = cfg.GetString("own_key_pem")
	c.partnerCertPEM = cfg.GetString("partner_cert_pem")
	c.requestMDN = cfg.GetBoolDefault("request_mdn", true)

	timeoutSec := cfg.GetInt("timeout_seconds")
	if timeoutSec == 0 {
		timeoutSec = 60
	}
	c.timeout = time.Duration(timeoutSec) * time.Second
	c.httpClient = httpservice.NewHTTPClientService(c.timeout)

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

	c.SetMetadata("partner_url", c.partnerURL)
	c.SetMetadata("as2_from", c.as2From)
	c.SetMetadata("as2_to", c.as2To)
	return nil
}

// Validate checks configuration completeness.
func (c *AS2OutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	typeName := c.GetMetadata().TypeName
	if c.partnerURL == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("partner_url is required"), false)
	}
	if c.as2From == "" || c.as2To == "" {
		return NewConnectorError(typeName, "validate", fmt.Errorf("as2_from and as2_to are required"), false)
	}
	if c.own.cert == nil || c.own.key == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("own_cert_pem and own_key_pem are required"), false)
	}
	if c.partnerCert == nil {
		return NewConnectorError(typeName, "validate", fmt.Errorf("partner_cert_pem is required"), false)
	}
	return nil
}

// TestConnection performs a bare reachability check against the partner
// URL — a real AS2 handshake only happens on an actual Send(), since a
// partner endpoint typically rejects anything that isn't a correctly
// signed/encrypted POST, often with a 4xx/5xx response to a plain HEAD.
// That's still proof the endpoint is reachable, so this deliberately uses a
// plain net/http client rather than httpservice.HTTPClientService.Execute,
// which treats ANY non-2xx response as a hard error (correct for callers
// expecting a real API success, wrong for a bare "is this host up" probe —
// found as a real bug via full-stack testing: this method's own dead
// `resp.StatusCode >= 500` branch was unreachable, since Execute already
// turns a 404/405 into a Go error before this method ever sees the status
// code at all). Only a genuine transport-level failure (DNS, refused
// connection, timeout) is treated as "not reachable" here.
func (c *AS2OutboundConnector) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.partnerURL, nil)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", fmt.Errorf("invalid partner_url: %w", err), false)
	}
	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	resp.Body.Close()
	return nil
}

// Send signs+encrypts message.Content and POSTs it to the partner's AS2
// endpoint, then (when request_mdn is set) verifies the signed synchronous
// MDN response actually proves THIS message — not some other one — was
// received.
func (c *AS2OutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	if message.Content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	envelope, err := buildAS2Envelope([]byte(message.Content), c.own, c.partnerCert)
	if err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}

	as2MessageID := fmt.Sprintf("<%s@%s>", uuid.New().String(), c.as2From)
	headers := map[string]string{
		"Content-Type":               `application/pkcs7-mime; smime-type=enveloped-data; name="smime.p7m"`,
		"AS2-Version":                "1.2",
		"AS2-From":                   c.as2From,
		"AS2-To":                     c.as2To,
		"Message-ID":                 as2MessageID,
		"Subject":                    "EDI Transaction",
		"Content-Transfer-Encoding":  "binary",
	}
	if c.requestMDN {
		headers["Disposition-Notification-To"] = c.as2From
		headers["Disposition-Notification-Options"] = "signed-receipt-protocol=required,pkcs7-signature; signed-receipt-micalg=required,sha256"
	}

	resp, err := c.httpClient.Execute(ctx, &httpservice.RequestConfig{
		Method:  "POST",
		URL:     c.partnerURL,
		Headers: headers,
		Body:    envelope,
		Timeout: c.timeout,
	}, nil)
	if err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sendErr := fmt.Errorf("partner endpoint returned HTTP %d", resp.StatusCode)
		c.RecordError(sendErr)
		return failResult(message.MessageID, start, sendErr), sendErr
	}

	result := &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata: map[string]interface{}{
			"as2_message_id": as2MessageID,
			"bytes_sent":      len(envelope),
		},
	}

	if c.requestMDN {
		disp, err := parseAndVerifyMDN(resp.Body, c.partnerCert)
		if err != nil {
			// A missing/invalid MDN does not un-send the message (the HTTP
			// POST itself already succeeded) but it DOES mean delivery is
			// unconfirmed — surfaced as a failed result, matching AS2's own
			// real-world meaning of a requested-but-unverifiable receipt.
			c.RecordError(err)
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("MDN verification failed: %v", err)
			return result, nil
		}
		if disp.OriginalMessageID != as2MessageID {
			mismatchErr := fmt.Errorf("MDN Original-Message-ID %q does not match sent Message-ID %q — not acceptable proof of delivery", disp.OriginalMessageID, as2MessageID)
			c.RecordError(mismatchErr)
			result.Success = false
			result.ErrorMessage = mismatchErr.Error()
			return result, nil
		}
		result.Acknowledgment = disp.Disposition
		if !disp.Processed() {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("partner MDN reported non-success disposition: %s", disp.Disposition)
			return result, nil
		}
	}

	c.IncrementMessagesSent()
	return result, nil
}
