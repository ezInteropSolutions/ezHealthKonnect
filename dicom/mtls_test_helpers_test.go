// dicom/mtls_test_helpers_test.go
// Test-only helpers for a real CA -> leaf certificate chain, needed to
// exercise mutual TLS (buildMutualTLSConfig in receiver.go) — unlike
// services/connectors/as2_test_helpers_test.go's generateTestIdentity
// (deliberately self-signed, correct for AS2's own S/MIME use case), a
// mutual-TLS test needs a real CA relationship: a server cert the client
// trusts because it chains to a known CA, and a client cert the server
// either accepts (same CA) or rejects (different, untrusted CA).
package dicom

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

type testCA struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

// newTestCA creates a fresh, self-signed CA certificate able to sign leaf
// certificates.
func newTestCA(t *testing.T, commonName string) testCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parsing CA certificate: %v", err)
	}
	return testCA{cert: cert, key: key}
}

// issueLeaf creates a leaf cert/key pair signed by ca, valid for both server
// and client auth (DICOM TLS peers play either role in this test).
func (ca testCA) issueLeaf(t *testing.T, commonName string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating leaf key for %s: %v", commonName, err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost", "127.0.0.1"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("creating leaf certificate for %s: %v", commonName, err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM
}

func (ca testCA) pemCert() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}))
}
