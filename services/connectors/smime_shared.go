// services/connectors/smime_shared.go
// Shared S/MIME (CMS/PKCS#7) sign+encrypt / decrypt+verify primitives —
// renamed from as2_smime.go once a SECOND, structurally similar S/MIME-based
// healthcare transport (Direct Messaging, direct_messaging_inbound.go/
// direct_messaging_outbound.go) started depending on the exact same
// functions. Used by: as2_inbound.go (decrypt+verify a partner's incoming
// message, then sign+encrypt the MDN response), as2_outbound.go (sign+encrypt
// an outgoing message, then verify the partner's incoming MDN response), and
// direct_messaging_inbound.go/outbound.go (decrypt+verify / sign+encrypt a
// DirectTrust email body — DirectTrust's own Applicability Statement, like
// AS2's RFC 4130, specifies the identical "sign then encrypt" CMS layering,
// so this file's own buildAS2Envelope/openAS2Envelope helpers — despite the
// "AS2" name, pure CMS with zero AS2-specific data — are directly reusable
// with no changes) — one implementation, not duplicated per transport.
//
// MIME approach (a deliberate, named scope choice — see CLAUDE.md's own
// Phase 4 section): this uses CMS-wrapped signing
// ("application/pkcs7-mime; smime-type=signed-data", content ATTACHED, not
// detached) layered under CMS enveloping ("smime-type=enveloped-data"),
// rather than RFC 4130's alternative "multipart/signed" MIME-boundary form.
// Both are valid, real AS2 message shapes (RFC 4130 §3.1 explicitly permits
// either "detached signature using... multipart/signed" or a "CMS... object
// [that] does not need the MIME multipart/signed wrapper"). The CMS-only
// form is structurally simpler Go code (one PKCS7 blob per layer, no
// hand-rolled MIME boundary parsing) — multipart/signed is a named, not-yet-
// built alternative if a specific real trading partner requires it.
//
// Crypto library: github.com/smallstep/pkcs7 (an actively maintained fork of
// the archived go.mozilla.org/pkcs7) — this project's go.mod had no PKCS7/
// S-MIME/CMS library before this; hand-rolling ASN.1 CMS structures was
// deliberately avoided (exactly the kind of low-level crypto-format code
// where a subtle DER/padding bug silently produces a signature that
// "verifies" when it shouldn't).
package connectors

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/smallstep/pkcs7"
)

// smimeIdentity bundles one S/MIME endpoint's own certificate + private key
// (used to sign outgoing content and decrypt incoming content) — shared
// shape for as2_inbound.go/as2_outbound.go and direct_messaging_inbound.go/
// direct_messaging_outbound.go's own config.
type smimeIdentity struct {
	cert *x509.Certificate
	key  crypto.PrivateKey
}

// parsePEMCertificate decodes a single PEM-encoded X.509 certificate.
func parsePEMCertificate(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expected a PEM-encoded CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert, nil
}

// parsePEMPrivateKey decodes a PEM-encoded RSA private key — PKCS#1
// ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") form, matching whichever a
// real partner's own key material happens to be in.
func parsePEMPrivateKey(pemStr string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("expected a PEM-encoded private key block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key (tried PKCS#1 and PKCS#8): %w", err)
	}
	return key, nil
}

// parseSMIMEConfig parses the 3-field own-cert/own-key/partner-cert PEM
// config shape both S/MIME-based connector pairs share (as2_inbound.go/
// as2_outbound.go inline this themselves, pre-dating this helper;
// direct_messaging_inbound.go/direct_messaging_outbound.go call it
// directly) into a ready-to-use identity + partner certificate. Either half
// may be left unset (empty own identity, nil partner cert) — the caller's
// own Validate() is responsible for rejecting a genuinely incomplete config;
// this just does the parsing.
func parseSMIMEConfig(ownCertPEM, ownKeyPEM, partnerCertPEM string) (smimeIdentity, *x509.Certificate, error) {
	var own smimeIdentity
	if ownCertPEM != "" && ownKeyPEM != "" {
		cert, err := parsePEMCertificate(ownCertPEM)
		if err != nil {
			return own, nil, fmt.Errorf("own certificate: %w", err)
		}
		key, err := parsePEMPrivateKey(ownKeyPEM)
		if err != nil {
			return own, nil, fmt.Errorf("own private key: %w", err)
		}
		own = smimeIdentity{cert: cert, key: key}
	}
	var partnerCert *x509.Certificate
	if partnerCertPEM != "" {
		cert, err := parsePEMCertificate(partnerCertPEM)
		if err != nil {
			return own, nil, fmt.Errorf("partner certificate: %w", err)
		}
		partnerCert = cert
	}
	return own, partnerCert, nil
}

// signContent wraps content as an ATTACHED CMS SignedData structure (the
// original content plus a detached-from-MIME's-perspective, but CMS-attached,
// signature) — the recipient recovers content directly from the parsed
// PKCS7's own .Content field, no separate MIME part needed.
func signContent(content []byte, identity smimeIdentity) ([]byte, error) {
	sd, err := pkcs7.NewSignedData(content)
	if err != nil {
		return nil, fmt.Errorf("as2: NewSignedData: %w", err)
	}
	if err := sd.AddSigner(identity.cert, identity.key, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, fmt.Errorf("as2: AddSigner: %w", err)
	}
	signed, err := sd.Finish()
	if err != nil {
		return nil, fmt.Errorf("as2: Finish signing: %w", err)
	}
	return signed, nil
}

// verifySignedContent parses a CMS SignedData structure, verifies its
// signature is cryptographically valid, AND confirms the embedded signer
// certificate is byte-for-byte the CONFIGURED trusted partner certificate —
// not just "some certificate whose signature checks out." AS2's trust model
// is direct certificate exchange between trading partners, not a CA
// hierarchy, so pkcs7.Verify()'s own "empty trust store" chain check alone
// would accept a signature from ANY self-consistent certificate; the
// explicit equality check here is what actually enforces "this really came
// from the partner we configured," which is the whole point of verifying an
// AS2 message or MDN at all.
func verifySignedContent(signedDER []byte, trustedSigner *x509.Certificate) ([]byte, error) {
	p7, err := pkcs7.Parse(signedDER)
	if err != nil {
		return nil, fmt.Errorf("as2: parsing signed data: %w", err)
	}
	if err := p7.Verify(); err != nil {
		return nil, fmt.Errorf("as2: signature verification failed: %w", err)
	}
	signer := p7.GetOnlySigner()
	if signer == nil {
		return nil, fmt.Errorf("as2: signed data does not have exactly one signer")
	}
	if !bytes.Equal(signer.Raw, trustedSigner.Raw) {
		return nil, fmt.Errorf("as2: signed by %q, not the configured trading partner certificate", signer.Subject.String())
	}
	return p7.Content, nil
}

// encryptContent wraps content as a CMS EnvelopedData structure, encrypted
// for recipientCert's own public key (RSA key transport, per pkcs7's own
// documented support).
func encryptContent(content []byte, recipientCert *x509.Certificate) ([]byte, error) {
	encrypted, err := pkcs7.Encrypt(content, []*x509.Certificate{recipientCert})
	if err != nil {
		return nil, fmt.Errorf("as2: encrypting: %w", err)
	}
	return encrypted, nil
}

// decryptContent parses a CMS EnvelopedData structure and decrypts it for
// identity's own certificate/private key.
func decryptContent(encryptedDER []byte, identity smimeIdentity) ([]byte, error) {
	p7, err := pkcs7.Parse(encryptedDER)
	if err != nil {
		return nil, fmt.Errorf("as2: parsing encrypted data: %w", err)
	}
	decrypted, err := p7.Decrypt(identity.cert, identity.key)
	if err != nil {
		return nil, fmt.Errorf("as2: decryption failed: %w", err)
	}
	return decrypted, nil
}

// buildAS2Envelope is the FULL outbound transform: sign content with own
// identity, then encrypt the signed result for the partner. The result is
// sent as-is (raw binary) over HTTP — this project's chosen transport
// encoding (RFC 4130 permits either binary or base64; HTTP POST bodies are
// fully binary-safe, so base64's extra encode/decode step buys nothing here
// and is skipped, unlike the email-derived S/MIME convention some AS2
// implementations still follow for historical 7-bit-transport reasons).
func buildAS2Envelope(content []byte, own smimeIdentity, partnerCert *x509.Certificate) ([]byte, error) {
	signed, err := signContent(content, own)
	if err != nil {
		return nil, err
	}
	return encryptContent(signed, partnerCert)
}

// openAS2Envelope is the FULL inbound transform: decrypt with own identity,
// verify the signature against the configured partner certificate,
// returning the original plaintext content.
func openAS2Envelope(wireBytes []byte, own smimeIdentity, partnerCert *x509.Certificate) ([]byte, error) {
	signed, err := decryptContent(wireBytes, own)
	if err != nil {
		return nil, err
	}
	return verifySignedContent(signed, partnerCert)
}
