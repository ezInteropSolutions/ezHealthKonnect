// services/connectors/as2_smime_test.go
// Round-trip and negative tests for the CMS sign/verify/encrypt/decrypt
// primitives — no live AS2 trading partner exists to test against, so this
// proves the crypto layer itself: sign+encrypt a synthetic payload with a
// self-generated test cert/key, decrypt+verify it, assert byte-for-byte
// equality; plus the negative cases a security-sensitive layer like this
// needs (wrong signer, wrong decryption key, tampered ciphertext).
package connectors

import (
	"bytes"
	"testing"
)

func TestAS2Envelope_RoundTrip(t *testing.T) {
	acmeProvider := generateTestIdentity(t, "Acme Provider")
	acmePayer := generateTestIdentity(t, "Acme Payer")

	original := []byte("ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260912*1421*^*00501*889860169*0*P*:~\nST*270*0001~\nSE*2*0001~\n")

	envelope, err := buildAS2Envelope(original, acmeProvider, acmePayer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}
	if len(envelope) == 0 {
		t.Fatal("buildAS2Envelope produced empty output")
	}
	if bytes.Contains(envelope, original) {
		t.Error("the envelope should NOT contain the plaintext content in the clear — it must be encrypted")
	}

	recovered, err := openAS2Envelope(envelope, acmePayer, acmeProvider.cert)
	if err != nil {
		t.Fatalf("openAS2Envelope: %v", err)
	}
	if !bytes.Equal(recovered, original) {
		t.Errorf("recovered content = %q, want %q", recovered, original)
	}
}

func TestAS2Envelope_WrongDecryptionKey_Fails(t *testing.T) {
	acmeProvider := generateTestIdentity(t, "Acme Provider")
	acmePayer := generateTestIdentity(t, "Acme Payer")
	imposter := generateTestIdentity(t, "Imposter")

	envelope, err := buildAS2Envelope([]byte("secret content"), acmeProvider, acmePayer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	// imposter has no idea what acmePayer's real private key is — decrypting
	// with the wrong identity must fail, not silently return garbage.
	if _, err := openAS2Envelope(envelope, imposter, acmeProvider.cert); err == nil {
		t.Error("expected decryption to fail with the wrong recipient identity, got nil error")
	}
}

func TestAS2Envelope_WrongTrustedSigner_Rejected(t *testing.T) {
	acmeProvider := generateTestIdentity(t, "Acme Provider")
	acmePayer := generateTestIdentity(t, "Acme Payer")
	imposter := generateTestIdentity(t, "Imposter")

	envelope, err := buildAS2Envelope([]byte("real content"), acmeProvider, acmePayer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	// The message decrypts fine (acmePayer's own key), but was actually
	// signed by acmeProvider, not imposter — verifying against the WRONG
	// expected signer must be rejected. This is the core direct-trust check
	// AS2 relies on (see verifySignedContent's own doc comment).
	if _, err := openAS2Envelope(envelope, acmePayer, imposter.cert); err == nil {
		t.Error("expected verification to fail when the configured trusted signer doesn't match who actually signed it, got nil error")
	}
}

func TestAS2Envelope_TamperedCiphertext_Rejected(t *testing.T) {
	acmeProvider := generateTestIdentity(t, "Acme Provider")
	acmePayer := generateTestIdentity(t, "Acme Payer")

	envelope, err := buildAS2Envelope([]byte("do not tamper with me"), acmeProvider, acmePayer.cert)
	if err != nil {
		t.Fatalf("buildAS2Envelope: %v", err)
	}

	tampered := append([]byte(nil), envelope...)
	// Flip a byte roughly in the middle of the DER structure — this should
	// break either DER parsing or the decrypted plaintext's own signature.
	mid := len(tampered) / 2
	tampered[mid] ^= 0xFF

	if _, err := openAS2Envelope(tampered, acmePayer, acmeProvider.cert); err == nil {
		t.Error("expected tampered ciphertext to fail decryption/verification, got nil error")
	}
}

func TestAS2Envelope_ContentNeverAppearsInClearInEncryptedForm(t *testing.T) {
	acmeProvider := generateTestIdentity(t, "Acme Provider")
	acmePayer := generateTestIdentity(t, "Acme Payer")

	secret := []byte("HIGHLY SENSITIVE ELIGIBILITY DATA — SUBSCRIBER SSN 123-45-6789")
	signed, err := signContent(secret, acmeProvider)
	if err != nil {
		t.Fatalf("signContent: %v", err)
	}
	encrypted, err := encryptContent(signed, acmePayer.cert)
	if err != nil {
		t.Fatalf("encryptContent: %v", err)
	}
	if bytes.Contains(encrypted, secret) {
		t.Fatal("encrypted output contains the plaintext secret in the clear — encryption is not actually happening")
	}
}
