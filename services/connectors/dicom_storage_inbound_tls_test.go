package connectors

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// TestDICOMStorageInbound_TLS_RealHandshakeSucceeds verifies the connector's
// TLSEnabled/TLSCertFile/TLSKeyFile wiring actually produces a real TLS
// listener — go-dicom v1.6.0's own high-level SCU API has no working TLS
// integration (SCUConfigTLS is declared but has zero real consumers,
// confirmed directly in network/scu.go and network/tls.go), so a full
// TLS-wrapped DIMSE association can't be driven through that library's own
// client. This test instead proves what this connector itself is
// responsible for: a stdlib tls.Dial against the configured port completes
// a real TLS handshake and presents exactly the configured certificate —
// the actual, in-scope claim "tls_enabled: true" makes.
func TestDICOMStorageInbound_TLS_RealHandshakeSucceeds(t *testing.T) {
	identity := generateTestIdentity(t, "EZHEALTHKONNECT-DICOM-TEST")
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, []byte(pemCert(t, identity)), 0o600); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte(pemKey(t, identity)), 0o600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}

	port := findFreePort(t)
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{
		"ae_title": "TESTSCP", "port": port,
		"tls_enabled": true, "tls_cert_file": certFile, "tls_key_file": keyFile,
	})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 1)
	if err := cIface.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cIface.Stop()
	time.Sleep(100 * time.Millisecond)

	conn, err := tls.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("real TLS handshake against the configured port failed: %v", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("server presented no certificate during the TLS handshake")
	}
	if !state.PeerCertificates[0].Equal(identity.cert) {
		t.Error("server presented a different certificate than the one configured via tls_cert_file")
	}
}

// TestDICOMStorageInbound_TLS_MissingFilesFailsAtStart confirms a
// misconfigured TLS setup (cert/key files that don't exist on disk) fails
// loudly at Start(), not silently falling back to plaintext.
func TestDICOMStorageInbound_TLS_MissingFilesFailsAtStart(t *testing.T) {
	port := findFreePort(t)
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{
		"ae_title": "TESTSCP", "port": port,
		"tls_enabled": true, "tls_cert_file": "/nonexistent/cert.pem", "tls_key_file": "/nonexistent/key.pem",
	})
	cIface.Initialize(cfg)

	msgChan := make(chan *models.InboundMessage, 1)
	if err := cIface.Start(context.Background(), msgChan); err == nil {
		t.Error("expected Start to fail when tls_enabled but the cert/key files do not exist")
	}
}
