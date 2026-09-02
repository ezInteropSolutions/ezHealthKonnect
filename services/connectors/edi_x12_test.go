// services/connectors/edi_x12_test.go
// Coverage for edi_x12_inbound/edi_x12_outbound's pure logic (config default
// resolution, Validate()'s transport-lock + required-field checks, outbound
// filename resolution) -- no live SFTP server in this environment, same
// documented limitation as sftp_outbound_test.go and the cloud warehouse
// connectors.
package connectors

import (
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func newTestEDIInbound(t *testing.T, config map[string]interface{}) *EDIX12InboundConnector {
	t.Helper()
	c := NewEDIX12InboundConnector().(*EDIX12InboundConnector)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	return c
}

func newTestEDIOutbound(t *testing.T, config map[string]interface{}) *EDIX12OutboundConnector {
	t.Helper()
	c := NewEDIX12OutboundConnector().(*EDIX12OutboundConnector)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := c.Initialize(raw); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	return c
}

func validInboundConfig() map[string]interface{} {
	return map[string]interface{}{
		"transport": "sftp", "host": "sftp.example.com", "username": "user", "password": "pw",
	}
}

func validOutboundConfig() map[string]interface{} {
	return map[string]interface{}{
		"transport": "sftp", "host": "sftp.example.com", "username": "user", "password": "pw",
	}
}

// --- Inbound: Initialize() default resolution ---

func TestEDIInbound_InitializeAppliesDefaults(t *testing.T) {
	c := newTestEDIInbound(t, map[string]interface{}{"host": "sftp.example.com", "username": "user", "password": "pw"})

	if c.transport != "sftp" {
		t.Errorf("transport = %q, want %q", c.transport, "sftp")
	}
	if c.port != 22 {
		t.Errorf("port = %d, want 22", c.port)
	}
	if c.remotePath != "/incoming" {
		t.Errorf("remotePath = %q, want %q", c.remotePath, "/incoming")
	}
	if c.filePattern != "*.edi" {
		t.Errorf("filePattern = %q, want %q", c.filePattern, "*.edi")
	}
	if c.afterProcessing != "archive" {
		t.Errorf("afterProcessing = %q, want %q", c.afterProcessing, "archive")
	}
	if c.archiveDir != "/incoming/processed" {
		t.Errorf("archiveDir = %q, want %q (default derived from remote_path)", c.archiveDir, "/incoming/processed")
	}
	if c.maxFilesPerRun != 100 {
		t.Errorf("maxFilesPerRun = %d, want 100", c.maxFilesPerRun)
	}
}

func TestEDIInbound_InitializeHonorsExplicitArchiveDir(t *testing.T) {
	cfg := validInboundConfig()
	cfg["remote_path"] = "/inbound/835"
	cfg["archive_dir"] = "/archive/edi"
	c := newTestEDIInbound(t, cfg)

	if c.archiveDir != "/archive/edi" {
		t.Errorf("archiveDir = %q, want explicit %q (not remote_path-derived default)", c.archiveDir, "/archive/edi")
	}
}

// --- Inbound: Validate() ---

func TestEDIInbound_Validate_RejectsNonSFTPTransport(t *testing.T) {
	cfg := validInboundConfig()
	cfg["transport"] = "http"
	c := newTestEDIInbound(t, cfg)

	err := c.Validate()
	if err == nil {
		t.Fatal("expected Validate() to reject transport=http in phase 1, got nil error")
	}
	if !strings.Contains(err.Error(), "not implemented in phase 1") {
		t.Errorf("expected a clear phase-1 message, got: %v", err)
	}
}

func TestEDIInbound_Validate_AcceptsSFTPWithRequiredFields(t *testing.T) {
	c := newTestEDIInbound(t, validInboundConfig())
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid sftp config to pass, got: %v", err)
	}
}

func TestEDIInbound_Validate_RequiresHost(t *testing.T) {
	cfg := validInboundConfig()
	delete(cfg, "host")
	c := newTestEDIInbound(t, cfg)

	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate() to reject missing host, got nil error")
	}
}

func TestEDIInbound_Validate_RequiresUsername(t *testing.T) {
	cfg := validInboundConfig()
	delete(cfg, "username")
	c := newTestEDIInbound(t, cfg)

	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate() to reject missing username, got nil error")
	}
}

func TestEDIInbound_Validate_RejectsInvalidAfterProcessing(t *testing.T) {
	cfg := validInboundConfig()
	cfg["after_processing"] = "shred"
	c := newTestEDIInbound(t, cfg)

	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate() to reject after_processing='shred', got nil error")
	}
}

func TestEDIInbound_Validate_AcceptsAllThreeAfterProcessingValues(t *testing.T) {
	for _, v := range []string{"delete", "archive", "none"} {
		cfg := validInboundConfig()
		cfg["after_processing"] = v
		c := newTestEDIInbound(t, cfg)
		if err := c.Validate(); err != nil {
			t.Errorf("after_processing=%q: expected valid, got: %v", v, err)
		}
	}
}

// --- Outbound: Initialize() default resolution ---

func TestEDIOutbound_InitializeAppliesDefaults(t *testing.T) {
	c := newTestEDIOutbound(t, map[string]interface{}{"host": "sftp.example.com", "username": "user", "password": "pw"})

	if c.transport != "sftp" {
		t.Errorf("transport = %q, want %q", c.transport, "sftp")
	}
	if c.port != 22 {
		t.Errorf("port = %d, want 22", c.port)
	}
	if c.remotePath != "/outgoing" {
		t.Errorf("remotePath = %q, want %q", c.remotePath, "/outgoing")
	}
}

// --- Outbound: Validate() ---

func TestEDIOutbound_Validate_RejectsNonSFTPTransport(t *testing.T) {
	cfg := validOutboundConfig()
	cfg["transport"] = "as2"
	c := newTestEDIOutbound(t, cfg)

	err := c.Validate()
	if err == nil {
		t.Fatal("expected Validate() to reject transport=as2 in phase 1, got nil error")
	}
	if !strings.Contains(err.Error(), "not implemented in phase 1") {
		t.Errorf("expected a clear phase-1 message, got: %v", err)
	}
}

func TestEDIOutbound_Validate_RequiresPasswordWhenAuthTypePassword(t *testing.T) {
	cfg := validOutboundConfig()
	delete(cfg, "password")
	c := newTestEDIOutbound(t, cfg)

	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate() to reject missing password for auth_type=password, got nil error")
	}
}

// Missing key_content is caught during Initialize() itself, not Validate() --
// buildAuthMethods() (which Initialize calls to build the SSH client config)
// fails fast for auth_type=key with no key to parse, before Validate() would
// ever run. So this asserts against Initialize's own error, not the
// newTestEDIOutbound helper (which treats an Initialize error as fatal).
func TestEDIOutbound_Initialize_RejectsKeyAuthWithoutKeyContent(t *testing.T) {
	c := NewEDIX12OutboundConnector().(*EDIX12OutboundConnector)
	cfg := validOutboundConfig()
	delete(cfg, "password")
	cfg["auth_type"] = "key"
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	if err := c.Initialize(raw); err == nil {
		t.Fatal("expected Initialize() to reject auth_type=key without key_content, got nil error")
	}
}

// --- Outbound: resolveFilename (pure logic, no network) ---

func TestEDIOutbound_ResolveFilename_DefaultsToTimestampedEdiExtension(t *testing.T) {
	c := newTestEDIOutbound(t, validOutboundConfig())
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-12345678"})
	if !strings.HasSuffix(name, ".edi") {
		t.Errorf("expected default filename to end in .edi, got: %s", name)
	}
	if !strings.HasPrefix(name, "edi_") {
		t.Errorf("expected default filename to start with edi_, got: %s", name)
	}
}

func TestEDIOutbound_ResolveFilename_UsesPatternWhenSet(t *testing.T) {
	cfg := validOutboundConfig()
	cfg["filename_pattern"] = "{interface_id}/{message_id}.txt"
	c := newTestEDIOutbound(t, cfg)

	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-1", InterfaceID: "if-42"})
	if name != "if-42/msg-1.txt" {
		t.Errorf("resolveFilename() = %q, want %q", name, "if-42/msg-1.txt")
	}
}

func TestEDIOutbound_ResolveFilename_AppendsEdiExtensionWhenPatternHasNone(t *testing.T) {
	cfg := validOutboundConfig()
	cfg["filename_pattern"] = "{message_id}"
	c := newTestEDIOutbound(t, cfg)

	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-1"})
	if !strings.HasSuffix(name, ".edi") {
		t.Errorf("expected inferred .edi extension appended, got: %s", name)
	}
}
