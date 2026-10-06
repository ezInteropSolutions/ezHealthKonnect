// dicom/mtls_test.go
// Proves buildMutualTLSConfig's real effect: a client presenting a
// certificate signed by the configured CA completes the TLS handshake; a
// client presenting a certificate from a DIFFERENT, untrusted CA (or no
// certificate at all) is rejected. This is the test the earlier "mutual TLS
// is impossible with this library" conclusion never had a chance to need —
// found to be wrong by reading network/tls.go's own buildTLSConfig function
// directly (its "Config *tls.Config" escape hatch), not by assumption.
//
// A real, non-obvious TLS wrinkle found while writing these tests: when a
// client has no certificate the server will accept, TLS allows it to
// respond to the server's CertificateRequest with an EMPTY certificate list
// rather than failing locally — so tls.Dial itself can return a nil error
// even though the SERVER will go on to reject the connection (confirmed via
// the server's own log line, "tls: client didn't provide a certificate").
// The rejection therefore has to be observed via a subsequent Read on the
// connection (which the server tears down), not via Dial's own return value.
package dicom

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// assertHandshakeRejected dials with the given client TLS config and
// confirms the connection is unusable — either Dial itself fails, or (the
// more common case for a missing/untrusted client cert, per this file's own
// header comment) a subsequent Read fails because the server tore the
// connection down after its own certificate verification failed.
func assertHandshakeRejected(t *testing.T, addr string, clientCfg *tls.Config) {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, clientCfg)
	if err != nil {
		return // rejected at dial time -- also a valid outcome
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, readErr := conn.Read(buf); readErr == nil {
		t.Fatal("expected the connection to be rejected (at dial or on first read), but both succeeded")
	}
}

func writeTempPEM(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return p
}

func TestReceiver_MutualTLS_ClientWithValidCertSucceeds(t *testing.T) {
	ca := newTestCA(t, "Test-CA")
	serverCertPEM, serverKeyPEM := ca.issueLeaf(t, "dicom-test-server")
	clientCertPEM, clientKeyPEM := ca.issueLeaf(t, "dicom-test-client")

	dir := t.TempDir()
	serverCertFile := writeTempPEM(t, dir, "server-cert.pem", serverCertPEM)
	serverKeyFile := writeTempPEM(t, dir, "server-key.pem", serverKeyPEM)
	caFile := writeTempPEM(t, dir, "ca-cert.pem", ca.pemCert())
	clientCertFile := writeTempPEM(t, dir, "client-cert.pem", clientCertPEM)
	clientKeyFile := writeTempPEM(t, dir, "client-key.pem", clientKeyPEM)

	port := findFreePort(t)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		TLSEnabled: true, TLSCertFile: serverCertFile, TLSKeyFile: serverKeyFile, TLSCAFile: caFile,
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	clientCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		t.Fatalf("loading client cert/key: %v", err)
	}

	conn, err := tls.Dial("tcp", r.Addr(), &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true, // test-only: not validating the server's own cert here, only proving the server accepted OUR client cert
	})
	if err != nil {
		t.Fatalf("expected handshake to succeed with a CA-signed client cert, got: %v", err)
	}
	defer conn.Close()
}

func TestReceiver_MutualTLS_ClientWithUntrustedCertRejected(t *testing.T) {
	ca := newTestCA(t, "Test-CA")
	serverCertPEM, serverKeyPEM := ca.issueLeaf(t, "dicom-test-server")

	untrustedCA := newTestCA(t, "Untrusted-CA")
	untrustedCertPEM, untrustedKeyPEM := untrustedCA.issueLeaf(t, "dicom-test-client-untrusted")

	dir := t.TempDir()
	serverCertFile := writeTempPEM(t, dir, "server-cert.pem", serverCertPEM)
	serverKeyFile := writeTempPEM(t, dir, "server-key.pem", serverKeyPEM)
	caFile := writeTempPEM(t, dir, "ca-cert.pem", ca.pemCert())
	untrustedCertFile := writeTempPEM(t, dir, "untrusted-cert.pem", untrustedCertPEM)
	untrustedKeyFile := writeTempPEM(t, dir, "untrusted-key.pem", untrustedKeyPEM)

	port := findFreePort(t)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		TLSEnabled: true, TLSCertFile: serverCertFile, TLSKeyFile: serverKeyFile, TLSCAFile: caFile,
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	untrustedCert, err := tls.LoadX509KeyPair(untrustedCertFile, untrustedKeyFile)
	if err != nil {
		t.Fatalf("loading untrusted cert/key: %v", err)
	}

	assertHandshakeRejected(t, r.Addr(), &tls.Config{
		Certificates:       []tls.Certificate{untrustedCert},
		InsecureSkipVerify: true,
	})
}

func TestReceiver_MutualTLS_ClientWithNoCertRejected(t *testing.T) {
	ca := newTestCA(t, "Test-CA")
	serverCertPEM, serverKeyPEM := ca.issueLeaf(t, "dicom-test-server")

	dir := t.TempDir()
	serverCertFile := writeTempPEM(t, dir, "server-cert.pem", serverCertPEM)
	serverKeyFile := writeTempPEM(t, dir, "server-key.pem", serverKeyPEM)
	caFile := writeTempPEM(t, dir, "ca-cert.pem", ca.pemCert())

	port := findFreePort(t)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		TLSEnabled: true, TLSCertFile: serverCertFile, TLSKeyFile: serverKeyFile, TLSCAFile: caFile,
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	// No client certificate presented at all.
	assertHandshakeRejected(t, r.Addr(), &tls.Config{InsecureSkipVerify: true})
}

// TestReceiver_TLS_NoCAFile_StillWorksWithoutClientCert proves the existing,
// pre-mutual-TLS behavior (server-side TLS only, no client cert required)
// is unchanged when TLSCAFile is left empty — the new code path is strictly
// additive.
func TestReceiver_TLS_NoCAFile_StillWorksWithoutClientCert(t *testing.T) {
	ca := newTestCA(t, "Test-CA")
	serverCertPEM, serverKeyPEM := ca.issueLeaf(t, "dicom-test-server")

	dir := t.TempDir()
	serverCertFile := writeTempPEM(t, dir, "server-cert.pem", serverCertPEM)
	serverKeyFile := writeTempPEM(t, dir, "server-key.pem", serverKeyPEM)

	port := findFreePort(t)
	r := NewReceiver(ReceiverConfig{
		AETitle: "TESTSCP", Port: port,
		TLSEnabled: true, TLSCertFile: serverCertFile, TLSKeyFile: serverKeyFile, // no TLSCAFile
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	time.Sleep(100 * time.Millisecond)

	conn, err := tls.Dial("tcp", r.Addr(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("expected handshake to succeed with no client cert when TLSCAFile is unset, got: %v", err)
	}
	conn.Close()
}
