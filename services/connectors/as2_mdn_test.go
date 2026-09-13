// services/connectors/as2_mdn_test.go
// Round-trip and negative tests for MDN construction/verification — proves
// a signed MDN can be built and correctly parsed back, and that the
// security-relevant rejections work: a tampered MDN, one signed by the
// wrong key, and (at the caller level, mirrored by as2_outbound_test.go)
// an Original-Message-ID mismatch — a validly-signed MDN for a DIFFERENT
// message must never be treated as proof of delivery for this one.
package connectors

import (
	"testing"
)

func TestMDN_BuildAndVerify_RoundTrip(t *testing.T) {
	responder := generateTestIdentity(t, "Acme Payer")

	signed, contentType, err := buildSignedMDN("<msg-123@provider>", "acme-payer", "processed", "The message was received and processed successfully.", responder)
	if err != nil {
		t.Fatalf("buildSignedMDN: %v", err)
	}
	if contentType == "" {
		t.Error("expected a non-empty content type for the signed MDN")
	}

	disp, err := parseAndVerifyMDN(signed, responder.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN: %v", err)
	}
	if disp.OriginalMessageID != "<msg-123@provider>" {
		t.Errorf("OriginalMessageID = %q, want <msg-123@provider>", disp.OriginalMessageID)
	}
	if !disp.Processed() {
		t.Errorf("Disposition = %q, want it to report Processed() = true", disp.Disposition)
	}
}

func TestMDN_FailedDisposition_NotProcessed(t *testing.T) {
	responder := generateTestIdentity(t, "Acme Payer")

	signed, _, err := buildSignedMDN("<msg-456@provider>", "acme-payer", "failed/error: decryption-failed", "Could not decrypt the message.", responder)
	if err != nil {
		t.Fatalf("buildSignedMDN: %v", err)
	}

	disp, err := parseAndVerifyMDN(signed, responder.cert)
	if err != nil {
		t.Fatalf("parseAndVerifyMDN: %v", err)
	}
	if disp.Processed() {
		t.Errorf("Disposition = %q, want Processed() = false for a failed disposition", disp.Disposition)
	}
}

func TestMDN_WrongSigner_Rejected(t *testing.T) {
	responder := generateTestIdentity(t, "Acme Payer")
	imposter := generateTestIdentity(t, "Imposter")

	signed, _, err := buildSignedMDN("<msg-789@provider>", "acme-payer", "processed", "ok", responder)
	if err != nil {
		t.Fatalf("buildSignedMDN: %v", err)
	}

	// The MDN was really signed by "responder" — verifying it against
	// "imposter" as the expected signer must fail, exactly like a real
	// forged/substituted MDN would.
	if _, err := parseAndVerifyMDN(signed, imposter.cert); err == nil {
		t.Error("expected verification to fail against the wrong expected signer, got nil error")
	}
}

func TestMDN_TamperedSignature_Rejected(t *testing.T) {
	responder := generateTestIdentity(t, "Acme Payer")

	signed, _, err := buildSignedMDN("<msg-999@provider>", "acme-payer", "processed", "ok", responder)
	if err != nil {
		t.Fatalf("buildSignedMDN: %v", err)
	}

	tampered := append([]byte(nil), signed...)
	mid := len(tampered) / 2
	tampered[mid] ^= 0xFF

	if _, err := parseAndVerifyMDN(tampered, responder.cert); err == nil {
		t.Error("expected a tampered signed MDN to fail verification, got nil error")
	}
}
