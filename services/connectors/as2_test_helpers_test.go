// services/connectors/as2_test_helpers_test.go
// Shared test-only helper for generating a self-signed RSA cert/key pair —
// AS2 test doubles ("Acme Payer", "Acme Provider") never touch a real CA,
// matching this project's own "no live trading partner to test against"
// constraint (see as2_smime.go's own header comment on scope).
package connectors

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// generateTestIdentity creates a fresh, self-signed RSA certificate + key
// pair for commonName — enough to exercise sign/verify/encrypt/decrypt
// without any real CA or fixture files.
func generateTestIdentity(t *testing.T, commonName string) smimeIdentity {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key for %s: %v", commonName, err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating self-signed certificate for %s: %v", commonName, err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parsing generated certificate for %s: %v", commonName, err)
	}

	return smimeIdentity{cert: cert, key: key}
}

// pemCert PEM-encodes an identity's own certificate — for building
// Initialize()-style JSON config in connector-level tests.
func pemCert(t *testing.T, id smimeIdentity) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: id.cert.Raw}))
}

// pemKey PEM-encodes an identity's own RSA private key (PKCS#1 form).
func pemKey(t *testing.T, id smimeIdentity) string {
	t.Helper()
	rsaKey, ok := id.key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("pemKey: identity's key is not an *rsa.PrivateKey")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)}))
}
