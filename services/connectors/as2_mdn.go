// services/connectors/as2_mdn.go
// AS2 MDN (Message Disposition Notification) construction and parsing —
// the signed receipt an AS2 receiver returns proving what happened to a
// message it received. Scope: SYNCHRONOUS MDN only (the RFC 4130 mode where
// the MDN is the HTTP response body to the same POST that delivered the
// message) — async MDN (partner posts the MDN back later to a separate URL)
// is a named, deferred item, not attempted here.
//
// The signed CMS content is a COMPLETE MIME entity (a "Content-Type: ..."
// header line, a blank line, then the multipart/report body) rather than
// just the bare multipart body — this is what lets the boundary parameter
// (needed to parse the multipart body back apart) survive the sign/verify
// round trip, since CMS SignedData's own .Content is opaque bytes with no
// side channel for a boundary string otherwise.
package connectors

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

const mdnReportingUA = "ezHealthKonnect AS2"

// mdnDisposition is the parsed, machine-readable content of an MDN's own
// message/disposition-notification part — the fields this project's AS2
// connectors actually need to correlate a receipt with the message it
// answers and decide success/failure.
type mdnDisposition struct {
	OriginalMessageID string
	Disposition       string // e.g. "automatic-action/MDN-sent-automatically; processed"
}

// Processed reports whether Disposition indicates the message was accepted
// — RFC 8098's disposition-type token is "processed" for success; anything
// else ("failed", "processed/error", "processed/warning" is still success
// with a caveat this project treats as success — only a bare "failed" is
// treated as delivery failure, matching RFC 8098's own 3-state model:
// processed / processed-with-warning-or-error / failed).
func (d mdnDisposition) Processed() bool {
	return strings.Contains(d.Disposition, "processed")
}

// buildMDNEntity constructs the UNSIGNED, complete MIME entity for an MDN —
// "Content-Type: multipart/report...\r\n\r\n" followed by a human-readable
// text/plain part and a machine-readable message/disposition-notification
// part (RFC 8098's own two-part shape).
func buildMDNEntity(originalMessageID, as2To, disposition, humanText string) ([]byte, error) {
	partsBuf := &bytes.Buffer{}
	w := multipart.NewWriter(partsBuf)

	textPart, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain"}})
	if err != nil {
		return nil, fmt.Errorf("as2: creating MDN text part: %w", err)
	}
	if _, err := textPart.Write([]byte(humanText)); err != nil {
		return nil, fmt.Errorf("as2: writing MDN text part: %w", err)
	}

	mdnPart, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {"message/disposition-notification"}})
	if err != nil {
		return nil, fmt.Errorf("as2: creating MDN disposition part: %w", err)
	}
	fields := fmt.Sprintf(
		"Reporting-UA: %s\r\n"+
			"Original-Recipient: rfc822; %s\r\n"+
			"Final-Recipient: rfc822; %s\r\n"+
			"Original-Message-ID: %s\r\n"+
			"Disposition: automatic-action/MDN-sent-automatically; %s\r\n",
		mdnReportingUA, as2To, as2To, originalMessageID, disposition,
	)
	if _, err := mdnPart.Write([]byte(fields)); err != nil {
		return nil, fmt.Errorf("as2: writing MDN disposition part: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("as2: closing MDN multipart writer: %w", err)
	}

	header := fmt.Sprintf("Content-Type: multipart/report; report-type=disposition-notification; boundary=%q\r\n\r\n", w.Boundary())
	return append([]byte(header), partsBuf.Bytes()...), nil
}

// buildSignedMDN builds a complete, CMS-signed MDN ready to be written as
// the synchronous HTTP response body, plus the HTTP Content-Type header
// value the caller should send it with.
func buildSignedMDN(originalMessageID, as2To, disposition, humanText string, own smimeIdentity) (signedBody []byte, contentType string, err error) {
	entity, err := buildMDNEntity(originalMessageID, as2To, disposition, humanText)
	if err != nil {
		return nil, "", err
	}
	signed, err := signContent(entity, own)
	if err != nil {
		return nil, "", err
	}
	return signed, `application/pkcs7-mime; smime-type=signed-data; name="smime.p7m"`, nil
}

// parseAndVerifyMDN verifies a signed MDN entity (CMS SignedData, DER)
// against trustedSigner — the SAME direct-trust equality check
// verifySignedContent already enforces for inbound AS2 messages, since an
// MDN's whole purpose is proof of delivery and accepting one "signed by
// somebody" rather than specifically the partner we sent to would defeat
// that — then extracts the machine-readable disposition fields.
func parseAndVerifyMDN(signedDER []byte, trustedSigner *x509.Certificate) (*mdnDisposition, error) {
	entity, err := verifySignedContent(signedDER, trustedSigner)
	if err != nil {
		return nil, fmt.Errorf("as2: MDN verification failed: %w", err)
	}
	return parseMDNEntity(entity)
}

// parseMDNEntity splits a complete MIME entity ("Content-Type: ...\r\n\r\n"
// + multipart body) back into its own headers + body, extracts the
// boundary, and reads the message/disposition-notification part's own
// fields.
func parseMDNEntity(entity []byte) (*mdnDisposition, error) {
	headerEnd := bytes.Index(entity, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		return nil, fmt.Errorf("as2: MDN entity has no header/body separator")
	}
	headerLine := string(entity[:headerEnd])
	body := entity[headerEnd+4:]

	const prefix = "Content-Type:"
	idx := strings.Index(headerLine, prefix)
	if idx == -1 {
		return nil, fmt.Errorf("as2: MDN entity missing Content-Type header")
	}
	_, params, err := mime.ParseMediaType(strings.TrimSpace(headerLine[idx+len(prefix):]))
	if err != nil {
		return nil, fmt.Errorf("as2: parsing MDN Content-Type: %w", err)
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, fmt.Errorf("as2: MDN Content-Type has no boundary parameter")
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	disp := &mdnDisposition{}
	for {
		part, err := mr.NextPart()
		if err != nil {
			break // io.EOF (normal end) or a malformed trailing part either way stop reading
		}
		partBody := &bytes.Buffer{}
		if _, err := partBody.ReadFrom(part); err != nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(part.Header.Get("Content-Type")), "message/disposition-notification") {
			for _, line := range strings.Split(partBody.String(), "\n") {
				line = strings.TrimRight(line, "\r")
				if v, ok := strings.CutPrefix(line, "Original-Message-ID:"); ok {
					disp.OriginalMessageID = strings.TrimSpace(v)
				} else if v, ok := strings.CutPrefix(line, "Disposition:"); ok {
					disp.Disposition = strings.TrimSpace(v)
				}
			}
		}
	}

	if disp.Disposition == "" {
		return nil, fmt.Errorf("as2: MDN has no message/disposition-notification part with a Disposition field")
	}
	return disp, nil
}
