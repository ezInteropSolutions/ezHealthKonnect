// services/connectors/connector_test.go
// Comprehensive unit tests for Sprint 7 Tier 1 connector implementations:
//   - MLLP Protocol (framing + ACK parsing)
//   - TCP/MLLP Outbound (init, validate, config defaults)
//   - SFTP Outbound (init, validate, resolveFilename, failResult)
//   - SFTP Inbound (init, validate, filterNonEmpty, generateSFTPMessageID)
//   - MongoDB Outbound (init, validate, parseContent)
//
// All tests use only unit-testable surface (no real network calls).
// Run: go test ./services/connectors/ -v -run TestMLLP|TestTCPMllp|TestSFTP|TestMongoDB

package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// marshalConfig converts a map to JSON bytes for Initialize calls.
func marshalConfig(t *testing.T, m map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}
	return b
}

// mllpConfig returns JSON config for TCPMLLPOutbound with overrides applied.
func mllpOutboundConfig(overrides map[string]interface{}) []byte {
	base := map[string]interface{}{
		"host": "localhost",
		"port": 2575,
	}
	for k, v := range overrides {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return b
}

// sftpOutboundCfg returns minimal valid SFTP outbound config with overrides.
func sftpOutboundCfg(overrides map[string]interface{}) []byte {
	base := map[string]interface{}{
		"host":      "sftp.example.com",
		"username":  "hl7user",
		"password":  "secret",
		"auth_type": "password",
	}
	for k, v := range overrides {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return b
}

// sftpInboundCfg returns minimal valid SFTP inbound config with overrides.
func sftpInboundCfg(overrides map[string]interface{}) []byte {
	base := map[string]interface{}{
		"host":     "sftp.example.com",
		"username": "hl7user",
		"password": "secret",
	}
	for k, v := range overrides {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return b
}

// mongodbCfg returns minimal valid MongoDB outbound config with overrides.
func mongodbCfg(overrides map[string]interface{}) []byte {
	base := map[string]interface{}{
		"uri": "mongodb://localhost:27017",
	}
	for k, v := range overrides {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// MLLP Protocol Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestMLLPFrame_CorrectFraming verifies that mllpFrame wraps content with
// the correct VT (0x0B), FS (0x1C), CR (0x0D) bytes.
func TestMLLPFrame_CorrectFraming(t *testing.T) {
	content := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240101||ADT^A01|MSG001|P|2.5"
	frame := mllpFrame(content)

	if len(frame) < 3 {
		t.Fatalf("frame too short: got %d bytes", len(frame))
	}
	if frame[0] != MLLPStartByte {
		t.Errorf("first byte: want 0x%02X (VT), got 0x%02X", MLLPStartByte, frame[0])
	}
	if frame[len(frame)-2] != MLLPEndByte1 {
		t.Errorf("second-to-last byte: want 0x%02X (FS), got 0x%02X", MLLPEndByte1, frame[len(frame)-2])
	}
	if frame[len(frame)-1] != MLLPEndByte2 {
		t.Errorf("last byte: want 0x%02X (CR), got 0x%02X", MLLPEndByte2, frame[len(frame)-1])
	}
	middle := string(frame[1 : len(frame)-2])
	if middle != content {
		t.Errorf("payload mismatch: want %q, got %q", content, middle)
	}
}

// TestMLLPFrame_EmptyContent ensures an empty HL7 message is still wrapped.
func TestMLLPFrame_EmptyContent(t *testing.T) {
	frame := mllpFrame("")
	// Should still have VT + FS + CR = 3 bytes
	if len(frame) != 3 {
		t.Errorf("empty content frame: want 3 bytes, got %d", len(frame))
	}
	if frame[0] != MLLPStartByte || frame[1] != MLLPEndByte1 || frame[2] != MLLPEndByte2 {
		t.Errorf("empty frame bytes wrong: %v", frame)
	}
}

// TestMLLPFrame_RoundTrip verifies that a framed message can be read back
// exactly via readMLLPFrame.
func TestMLLPFrame_RoundTrip(t *testing.T) {
	content := "MSH|^~\\&|A|B|C|D|20240315120000||ORU^R01|M123|P|2.5\rPID|||12345"
	frame := mllpFrame(content)

	r := bytes.NewReader(frame)
	got, err := readMLLPFrame(r)
	if err != nil {
		t.Fatalf("readMLLPFrame error: %v", err)
	}
	if got != content {
		t.Errorf("round-trip mismatch:\nwant: %q\ngot:  %q", content, got)
	}
}

// TestReadMLLPFrame_SkipsPreambleBytes verifies the reader skips garbage bytes
// before the MLLP start byte (common in real TCP streams).
func TestReadMLLPFrame_SkipsPreambleBytes(t *testing.T) {
	content := "MSH|^~\\&|SEND|FAC|||20240101||ADT^A04|CTRL|P|2.5"
	// Insert preamble garbage before VT
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0xFF, 0x0A}) // garbage
	buf.Write(mllpFrame(content))

	got, err := readMLLPFrame(&buf)
	if err != nil {
		t.Fatalf("readMLLPFrame with preamble error: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
}

// TestParseMLLPACKCode_AcceptACK verifies AA extraction.
func TestParseMLLPACKCode_AcceptACK(t *testing.T) {
	ack := "MSH|^~\\&|RECV|FAC|SEND|FAC|20240101||ACK^A01|ACK001|P|2.5\rMSA|AA|MSG001|Message accepted"
	code := parseMLLPACKCode(ack)
	if code != "AA" {
		t.Errorf("want AA, got %q", code)
	}
}

// TestParseMLLPACKCode_ErrorACK verifies AE extraction.
func TestParseMLLPACKCode_ErrorACK(t *testing.T) {
	ack := "MSH|^~\\&|RECV|FAC|SEND|FAC|20240101||ACK|ACK002|P|2.5\rMSA|AE|MSG001|Application error"
	code := parseMLLPACKCode(ack)
	if code != "AE" {
		t.Errorf("want AE, got %q", code)
	}
}

// TestParseMLLPACKCode_RejectACK verifies AR extraction.
func TestParseMLLPACKCode_RejectACK(t *testing.T) {
	ack := "MSH|^~\\&|X|Y|Z|W|20240101||ACK|ACK003|P|2.5\rMSA|AR|MSG001|Message rejected"
	code := parseMLLPACKCode(ack)
	if code != "AR" {
		t.Errorf("want AR, got %q", code)
	}
}

// TestParseMLLPACKCode_NoMSASegment returns empty string when no MSA present.
func TestParseMLLPACKCode_NoMSASegment(t *testing.T) {
	msg := "MSH|^~\\&|A|B|C|D|20240101||ADT^A01|MSG|P|2.5\rPID|||12345"
	code := parseMLLPACKCode(msg)
	if code != "" {
		t.Errorf("expected empty string for no-MSA message, got %q", code)
	}
}

// TestParseMLLPACKCode_MultilineWithCRLF ensures parsing works with
// both \r and \n line separators.
func TestParseMLLPACKCode_MultilineWithCRLF(t *testing.T) {
	ack := "MSH|^~\\&|R|F|S|E|20240101||ACK|ID|P|2.5\r\nMSA|AA|ID|OK"
	code := parseMLLPACKCode(ack)
	// \r\n split will produce "MSA|AA|ID|OK" and "\nMSA|AA|ID|OK" — check it's found
	if code == "" {
		t.Error("expected non-empty ACK code, got empty string")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TCP/MLLP Outbound Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestTCPMLLPOutbound_DefaultsApplied verifies that defaults are set when
// no values are provided in the config.
func TestTCPMLLPOutbound_DefaultsApplied(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	cfg := mllpOutboundConfig(nil) // only host + port
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}
	if c.connectionMode != "persistent" {
		t.Errorf("default connection_mode: want persistent, got %q", c.connectionMode)
	}
	if c.retryDelayMs != 500 {
		t.Errorf("default retry_delay_ms: want 500, got %d", c.retryDelayMs)
	}
	if !c.expectACK {
		t.Error("default expect_ack: want true, got false")
	}
}

// TestTCPMLLPOutbound_ValidConfig succeeds with a fully specified config.
func TestTCPMLLPOutbound_ValidConfig(t *testing.T) {
	c := NewTCPMLLPOutboundConnector()
	cfg := mllpOutboundConfig(map[string]interface{}{
		"connection_mode": "per-message",
		"max_retries":     3,
		"retry_delay_ms":  1000,
	})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate error: %v", err)
	}
}

// TestTCPMLLPOutbound_Validate_MissingHost returns an error when host is empty.
func TestTCPMLLPOutbound_Validate_MissingHost(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	// Initialize with empty host — it defaults to "localhost" in code,
	// but let's test Validate with a manually cleared host.
	cfg := mllpOutboundConfig(map[string]interface{}{"host": "localhost", "port": 2575})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}
	c.host = "" // Force-clear for unit test
	err := c.Validate()
	if err == nil {
		t.Error("expected error for empty host, got nil")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention 'host': %v", err)
	}
}

// TestTCPMLLPOutbound_Validate_InvalidPort_TooHigh returns error for port > 65535.
func TestTCPMLLPOutbound_Validate_InvalidPort_TooHigh(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	cfg := mllpOutboundConfig(nil)
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}
	c.port = 99999
	err := c.Validate()
	if err == nil {
		t.Error("expected error for port > 65535, got nil")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error should mention 'port': %v", err)
	}
}

// TestTCPMLLPOutbound_Validate_InvalidPort_Zero returns error for port = 0.
func TestTCPMLLPOutbound_Validate_InvalidPort_Zero(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	cfg := mllpOutboundConfig(nil)
	_ = c.Initialize(cfg)
	c.port = 0
	if err := c.Validate(); err == nil {
		t.Error("expected error for port 0, got nil")
	}
}

// TestTCPMLLPOutbound_Validate_InvalidConnectionMode returns error for bad mode.
func TestTCPMLLPOutbound_Validate_InvalidConnectionMode(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	cfg := mllpOutboundConfig(nil)
	_ = c.Initialize(cfg)
	c.connectionMode = "streaming" // invalid
	err := c.Validate()
	if err == nil {
		t.Error("expected error for invalid connection_mode, got nil")
	}
	if !strings.Contains(err.Error(), "connection_mode") {
		t.Errorf("error should mention connection_mode: %v", err)
	}
}

// TestTCPMLLPOutbound_Validate_PersistentMode validates the persistent connection mode.
func TestTCPMLLPOutbound_Validate_PersistentMode(t *testing.T) {
	c := NewTCPMLLPOutboundConnector()
	cfg := mllpOutboundConfig(map[string]interface{}{"connection_mode": "persistent"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for persistent mode: %v", err)
	}
}

// TestTCPMLLPOutbound_Validate_PerMessageMode validates the per-message mode.
func TestTCPMLLPOutbound_Validate_PerMessageMode(t *testing.T) {
	c := NewTCPMLLPOutboundConnector()
	cfg := mllpOutboundConfig(map[string]interface{}{"connection_mode": "per-message"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for per-message mode: %v", err)
	}
}

// TestTCPMLLPOutbound_TLSConfigApplied verifies TLS config is set when enabled.
func TestTCPMLLPOutbound_TLSConfigApplied(t *testing.T) {
	c := NewTCPMLLPOutboundConnector().(*TCPMLLPOutboundConnector)
	cfg := mllpOutboundConfig(map[string]interface{}{
		"enable_tls":      true,
		"tls_skip_verify": true,
	})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !c.enableTLS {
		t.Error("enableTLS should be true")
	}
	if c.tlsConfig == nil {
		t.Error("tlsConfig should not be nil when TLS is enabled")
	}
}

// TestTCPMLLPOutbound_MetadataTypeName verifies the connector reports the correct type name.
func TestTCPMLLPOutbound_MetadataTypeName(t *testing.T) {
	c := NewTCPMLLPOutboundConnector()
	if meta := c.GetMetadata(); meta.TypeName != "tcp_mllp_outbound" {
		t.Errorf("TypeName: want tcp_mllp_outbound, got %q", meta.TypeName)
	}
}

// TestTCPMLLPOutbound_SupportsBatch verifies batch is not supported (MLLP is per-message).
func TestTCPMLLPOutbound_SupportsBatch(t *testing.T) {
	c := NewTCPMLLPOutboundConnector()
	if c.SupportsBatch() {
		t.Error("MLLP outbound should NOT support batch (one ACK per message)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SFTP Outbound Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestSFTPOutbound_Initialize_PasswordAuth succeeds with password authentication.
func TestSFTPOutbound_Initialize_PasswordAuth(t *testing.T) {
	c := NewSFTPOutboundConnector()
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize with password auth failed: %v", err)
	}
}

// TestSFTPOutbound_Initialize_KeyAuth_NoKeyContent returns error when key_content
// is missing and key auth is requested (key_file reading is not supported in this env).
func TestSFTPOutbound_Initialize_KeyAuth_NoKeyContent(t *testing.T) {
	c := NewSFTPOutboundConnector()
	cfg := sftpOutboundCfg(map[string]interface{}{
		"auth_type": "key",
		"key_file":  "/some/path/id_rsa", // key_file without key_content triggers error
	})
	err := c.Initialize(cfg)
	if err == nil {
		t.Error("expected error when auth_type=key and no key_content, got nil")
	}
	if !strings.Contains(err.Error(), "key_content") {
		t.Errorf("error should mention key_content: %v", err)
	}
}

// TestSFTPOutbound_DefaultValues verifies defaults are set correctly.
func TestSFTPOutbound_DefaultValues(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.port != 22 {
		t.Errorf("default port: want 22, got %d", c.port)
	}
	if c.remoteDir != "/upload" {
		t.Errorf("default remoteDir: want /upload, got %q", c.remoteDir)
	}
	if c.filenamePrefix != "msg_" {
		t.Errorf("default filenamePrefix: want msg_, got %q", c.filenamePrefix)
	}
	if c.fileExtension != ".dat" {
		t.Errorf("default fileExtension: want .dat, got %q", c.fileExtension)
	}
}

// TestSFTPOutbound_Validate_MissingHost returns error when host is empty.
func TestSFTPOutbound_Validate_MissingHost(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.host = "" // Force-clear
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing host, got nil")
	}
}

// TestSFTPOutbound_Validate_MissingUsername returns error when username is empty.
func TestSFTPOutbound_Validate_MissingUsername(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.username = "" // Force-clear
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing username, got nil")
	}
}

// TestSFTPOutbound_Validate_PasswordAuthMissingPassword returns error when password is empty.
func TestSFTPOutbound_Validate_PasswordAuthMissingPassword(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.password = "" // Clear password
	c.authType = "password"
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing password with password auth, got nil")
	}
}

// TestSFTPOutbound_Validate_KeyAuthMissingKey returns error when no key is provided.
func TestSFTPOutbound_Validate_KeyAuthMissingKey(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.authType = "key"
	c.keyFile = ""
	c.keyContent = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for key auth with no key material, got nil")
	}
}

// TestSFTPOutbound_ResolveFilename_FromMetadata uses message metadata as filename.
func TestSFTPOutbound_ResolveFilename_FromMetadata(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.filenameField = "original_filename"
	msg := &models.OutboundMessage{
		MessageID: "test-001",
		Metadata:  map[string]string{"original_filename": "patient_adta01.hl7"},
	}
	name := c.resolveFilename(msg)
	if name != "patient_adta01.hl7" {
		t.Errorf("want patient_adta01.hl7, got %q", name)
	}
}

// TestSFTPOutbound_ResolveFilename_GeneratedWhenNoMetadata generates a timestamped name.
func TestSFTPOutbound_ResolveFilename_GeneratedWhenNoMetadata(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	if err := c.Initialize(sftpOutboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	msg := &models.OutboundMessage{MessageID: "abcdefgh1234"}
	name := c.resolveFilename(msg)
	// Should be: msg_YYYYMMDD_HHMMSS_abcdefgh.dat
	if !strings.HasPrefix(name, "msg_") {
		t.Errorf("filename should start with 'msg_': got %q", name)
	}
	if !strings.HasSuffix(name, ".dat") {
		t.Errorf("filename should end with '.dat': got %q", name)
	}
}

// TestSFTPOutbound_ResolveFilename_PrefixAndExtension respects configured prefix/extension.
func TestSFTPOutbound_ResolveFilename_PrefixAndExtension(t *testing.T) {
	c := NewSFTPOutboundConnector().(*SFTPOutboundConnector)
	cfg := sftpOutboundCfg(map[string]interface{}{
		"filename_prefix": "hl7_",
		"file_extension":  ".hl7",
	})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	msg := &models.OutboundMessage{MessageID: "msg001"}
	name := c.resolveFilename(msg)
	if !strings.HasPrefix(name, "hl7_") {
		t.Errorf("want prefix hl7_, got %q", name)
	}
	if !strings.HasSuffix(name, ".hl7") {
		t.Errorf("want extension .hl7, got %q", name)
	}
}

// TestSFTPOutbound_FailResult verifies the structure of a failed delivery result.
func TestSFTPOutbound_FailResult(t *testing.T) {
	start := time.Now()
	result := failResult("msg-xyz-001", start, fmt.Errorf("connection refused"))
	if result == nil {
		t.Fatal("failResult returned nil")
	}
	if result.Success {
		t.Error("failed result should have Success=false")
	}
	if result.MessageID != "msg-xyz-001" {
		t.Errorf("MessageID: want msg-xyz-001, got %q", result.MessageID)
	}
	if !strings.Contains(result.ErrorMessage, "connection refused") {
		t.Errorf("ErrorMessage should contain 'connection refused': %q", result.ErrorMessage)
	}
	if result.DurationMs < 0 {
		t.Error("DurationMs should be >= 0")
	}
}

// TestSFTPOutbound_MetadataTypeName verifies the connector type name.
func TestSFTPOutbound_MetadataTypeName(t *testing.T) {
	c := NewSFTPOutboundConnector()
	if meta := c.GetMetadata(); meta.TypeName != "sftp_outbound" {
		t.Errorf("TypeName: want sftp_outbound, got %q", meta.TypeName)
	}
}

// TestSFTPOutbound_SupportsBatch verifies batch is supported (SFTP can upload multiple files).
func TestSFTPOutbound_SupportsBatch(t *testing.T) {
	c := NewSFTPOutboundConnector()
	if !c.SupportsBatch() {
		t.Error("SFTP outbound should support batch uploads")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SFTP Inbound Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestSFTPInbound_Initialize_Defaults verifies all default values are set.
func TestSFTPInbound_Initialize_Defaults(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.port != 22 {
		t.Errorf("default port: want 22, got %d", c.port)
	}
	if c.remoteDir != "/inbox" {
		t.Errorf("default remoteDir: want /inbox, got %q", c.remoteDir)
	}
	if c.filePattern != "*.hl7" {
		t.Errorf("default filePattern: want *.hl7, got %q", c.filePattern)
	}
	if c.afterProcessing != "archive" {
		t.Errorf("default afterProcessing: want archive, got %q", c.afterProcessing)
	}
	if c.maxFilesPerRun != 100 {
		t.Errorf("default maxFilesPerRun: want 100, got %d", c.maxFilesPerRun)
	}
	if c.pollInterval != 30*time.Second {
		t.Errorf("default pollInterval: want 30s, got %v", c.pollInterval)
	}
}

// TestSFTPInbound_Validate_InvalidAfterProcessing returns error for unknown action.
func TestSFTPInbound_Validate_InvalidAfterProcessing(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.afterProcessing = "move" // invalid value
	err := c.Validate()
	if err == nil {
		t.Error("expected error for invalid after_processing value, got nil")
	}
	if !strings.Contains(err.Error(), "after_processing") {
		t.Errorf("error should mention after_processing: %v", err)
	}
}

// TestSFTPInbound_Validate_Delete verifies "delete" is accepted.
func TestSFTPInbound_Validate_Delete(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.afterProcessing = "delete"
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for after_processing=delete: %v", err)
	}
}

// TestSFTPInbound_Validate_Archive verifies "archive" is accepted.
func TestSFTPInbound_Validate_Archive(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.afterProcessing = "archive"
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for after_processing=archive: %v", err)
	}
}

// TestSFTPInbound_Validate_None verifies "none" is accepted.
func TestSFTPInbound_Validate_None(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.afterProcessing = "none"
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for after_processing=none: %v", err)
	}
}

// TestSFTPInbound_Validate_MissingHost returns error when host is absent.
func TestSFTPInbound_Validate_MissingHost(t *testing.T) {
	c := NewSFTPInboundConnector().(*SFTPInboundConnector)
	if err := c.Initialize(sftpInboundCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.host = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing host, got nil")
	}
}

// TestSFTPInbound_SupportsCron verifies the connector advertises cron support.
func TestSFTPInbound_SupportsCron(t *testing.T) {
	c := NewSFTPInboundConnector()
	if !c.SupportsCron() {
		t.Error("SFTP inbound should support cron (polling)")
	}
}

// TestFilterNonEmpty_RemovesBlankAndWhitespace verifies empty string filtering.
func TestFilterNonEmpty_RemovesBlankAndWhitespace(t *testing.T) {
	input := []string{"a", "", "b", "  ", "c", "\t"}
	got := filterNonEmpty(input)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len: want %d, got %d: %v", len(want), len(got), got)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("index %d: want %q, got %q", i, v, got[i])
		}
	}
}

// TestFilterNonEmpty_AllEmpty returns empty slice.
func TestFilterNonEmpty_AllEmpty(t *testing.T) {
	got := filterNonEmpty([]string{"", "  ", "\t", "\n"})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// TestFilterNonEmpty_AllValid returns all elements unchanged.
func TestFilterNonEmpty_AllValid(t *testing.T) {
	input := []string{"/inbox/file1.hl7", "/inbox/file2.hl7"}
	got := filterNonEmpty(input)
	if len(got) != 2 {
		t.Errorf("expected 2 elements, got %d", len(got))
	}
}

// TestGenerateSFTPMessageID_Format verifies the ID has the expected prefix and filename.
func TestGenerateSFTPMessageID_Format(t *testing.T) {
	id := generateSFTPMessageID("/inbox/ADT_A01_20240315.hl7")
	if !strings.HasPrefix(id, "sftp_") {
		t.Errorf("ID should start with sftp_: got %q", id)
	}
	if !strings.HasSuffix(id, "_ADT_A01_20240315.hl7") {
		t.Errorf("ID should end with file basename: got %q", id)
	}
}

// TestGenerateSFTPMessageID_Uniqueness two calls produce different IDs (timestamp-based).
// Note: This test uses time.Sleep to ensure timestamp difference.
func TestGenerateSFTPMessageID_DifferentFiles(t *testing.T) {
	id1 := generateSFTPMessageID("/inbox/file1.hl7")
	id2 := generateSFTPMessageID("/inbox/file2.hl7")
	if id1 == id2 {
		t.Error("different filenames should produce different IDs")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MongoDB Outbound Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestMongoDBOutbound_Initialize_WithURI succeeds with uri field.
func TestMongoDBOutbound_Initialize_WithURI(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	if err := c.Initialize(mongodbCfg(nil)); err != nil {
		t.Fatalf("Initialize with uri failed: %v", err)
	}
}

// TestMongoDBOutbound_Initialize_WithConnectionString supports legacy field name.
func TestMongoDBOutbound_Initialize_WithConnectionString(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	cfg := marshalConfig(t, map[string]interface{}{
		"connection_string": "mongodb://mongo:27017",
	})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize with connection_string failed: %v", err)
	}
}

// TestMongoDBOutbound_Initialize_NoURI returns error when neither uri nor connection_string given.
func TestMongoDBOutbound_Initialize_NoURI(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	cfg := marshalConfig(t, map[string]interface{}{
		"database":   "health",
		"collection": "fhir_bundles",
	})
	err := c.Initialize(cfg)
	if err == nil {
		t.Error("expected error for missing uri, got nil")
	}
	if !strings.Contains(err.Error(), "uri") {
		t.Errorf("error should mention 'uri': %v", err)
	}
}

// TestMongoDBOutbound_DefaultValues verifies defaults are applied.
func TestMongoDBOutbound_DefaultValues(t *testing.T) {
	c := NewMongoDBOutboundConnector().(*MongoDBOutboundConnector)
	if err := c.Initialize(mongodbCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.database != "ezhealthkonnect" {
		t.Errorf("default database: want ezhealthkonnect, got %q", c.database)
	}
	if c.collection != "messages" {
		t.Errorf("default collection: want messages, got %q", c.collection)
	}
	if c.writeMode != "insert" {
		t.Errorf("default writeMode: want insert, got %q", c.writeMode)
	}
	if c.upsertField != "_id" {
		t.Errorf("default upsertField: want _id, got %q", c.upsertField)
	}
}

// TestMongoDBOutbound_Validate_InsertMode passes validation.
func TestMongoDBOutbound_Validate_InsertMode(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	if err := c.Initialize(mongodbCfg(map[string]interface{}{"write_mode": "insert"})); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for insert mode: %v", err)
	}
}

// TestMongoDBOutbound_Validate_UpsertMode passes validation.
func TestMongoDBOutbound_Validate_UpsertMode(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	if err := c.Initialize(mongodbCfg(map[string]interface{}{"write_mode": "upsert"})); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate should pass for upsert mode: %v", err)
	}
}

// TestMongoDBOutbound_Validate_InvalidWriteMode returns error for unknown mode.
func TestMongoDBOutbound_Validate_InvalidWriteMode(t *testing.T) {
	c := NewMongoDBOutboundConnector().(*MongoDBOutboundConnector)
	if err := c.Initialize(mongodbCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c.writeMode = "replace" // invalid
	err := c.Validate()
	if err == nil {
		t.Error("expected error for invalid write_mode, got nil")
	}
	if !strings.Contains(err.Error(), "write_mode") {
		t.Errorf("error should mention write_mode: %v", err)
	}
}

// TestMongoDBOutbound_ParseContent_ValidJSON parses a JSON document correctly.
func TestMongoDBOutbound_ParseContent_ValidJSON(t *testing.T) {
	c := NewMongoDBOutboundConnector().(*MongoDBOutboundConnector)
	if err := c.Initialize(mongodbCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	fhirBundle := `{"resourceType":"Bundle","type":"transaction","entry":[]}`
	msg := &models.OutboundMessage{
		MessageID:   "fhir-001",
		Content:     fhirBundle,
		ContentType: "application/fhir+json",
	}
	doc, err := c.parseContent(msg)
	if err != nil {
		t.Fatalf("parseContent error: %v", err)
	}
	if doc["resourceType"] != "Bundle" {
		t.Errorf("want resourceType=Bundle, got %v", doc["resourceType"])
	}
}

// TestMongoDBOutbound_ParseContent_InvalidJSON_WrapsAsString stores plain string when JSON fails.
func TestMongoDBOutbound_ParseContent_InvalidJSON_WrapsAsString(t *testing.T) {
	c := NewMongoDBOutboundConnector().(*MongoDBOutboundConnector)
	if err := c.Initialize(mongodbCfg(nil)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	hl7Msg := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315120000||ADT^A01|MSG001|P|2.5"
	msg := &models.OutboundMessage{
		MessageID:   "hl7-001",
		Content:     hl7Msg,
		ContentType: "application/hl7-v2",
	}
	doc, err := c.parseContent(msg)
	if err != nil {
		t.Fatalf("parseContent for HL7 should not error: %v", err)
	}
	if doc["content"] != hl7Msg {
		t.Errorf("expected content key with HL7 string, got %v", doc["content"])
	}
}

// TestMongoDBOutbound_ParseContent_ContentField_Extracts sub-document at path.
func TestMongoDBOutbound_ParseContent_ContentField_Extracts(t *testing.T) {
	c := NewMongoDBOutboundConnector().(*MongoDBOutboundConnector)
	if err := c.Initialize(mongodbCfg(map[string]interface{}{
		"content_field": "fhir_bundle",
	})); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	payload := `{"fhir_bundle":{"resourceType":"Bundle","id":"b001"},"meta":{"source":"hl7"}}`
	msg := &models.OutboundMessage{Content: payload}
	doc, err := c.parseContent(msg)
	if err != nil {
		t.Fatalf("parseContent with content_field error: %v", err)
	}
	if doc["resourceType"] != "Bundle" {
		t.Errorf("expected extracted sub-doc with resourceType=Bundle, got %v", doc["resourceType"])
	}
	if _, ok := doc["meta"]; ok {
		t.Error("extracted doc should not contain outer 'meta' key")
	}
}

// TestMongoDBOutbound_MetadataTypeName verifies the connector type name.
func TestMongoDBOutbound_MetadataTypeName(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	if meta := c.GetMetadata(); meta.TypeName != "mongodb_outbound" {
		t.Errorf("TypeName: want mongodb_outbound, got %q", meta.TypeName)
	}
}

// TestMongoDBOutbound_SupportsBatch verifies batch is supported.
func TestMongoDBOutbound_SupportsBatch(t *testing.T) {
	c := NewMongoDBOutboundConnector()
	if !c.SupportsBatch() {
		t.Error("MongoDB outbound should support batch")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Connector Factory Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestFactory_CreateOutbound_AllTier1Types verifies all Sprint 7 Tier 1 outbound
// connectors can be created from the global factory.
func TestFactory_CreateOutbound_AllTier1Types(t *testing.T) {
	factory := GetFactory()
	tier1 := []string{
		"tcp_mllp_outbound",
		"sftp_outbound",
		"mongodb_outbound",
		"http_outbound",
		"file_writer",
	}
	for _, typeName := range tier1 {
		c, err := factory.CreateOutbound(typeName)
		if err != nil {
			t.Errorf("CreateOutbound(%q) failed: %v", typeName, err)
			continue
		}
		if c == nil {
			t.Errorf("CreateOutbound(%q) returned nil", typeName)
		}
	}
}

// TestFactory_CreateInbound_AllTier1Types verifies all Tier 1 inbound connectors.
func TestFactory_CreateInbound_AllTier1Types(t *testing.T) {
	factory := GetFactory()
	tier1 := []string{
		"tcp_mllp_inbound",
		"sftp_inbound",
		"http_rest_inbound",
		"file_listener",
	}
	for _, typeName := range tier1 {
		c, err := factory.CreateInbound(typeName)
		if err != nil {
			t.Errorf("CreateInbound(%q) failed: %v", typeName, err)
			continue
		}
		if c == nil {
			t.Errorf("CreateInbound(%q) returned nil", typeName)
		}
	}
}

// TestFactory_CreateOutbound_UnknownType returns error for unregistered type.
func TestFactory_CreateOutbound_UnknownType(t *testing.T) {
	factory := GetFactory()
	_, err := factory.CreateOutbound("nonexistent_outbound_xyz")
	if err == nil {
		t.Error("expected error for unknown connector type, got nil")
	}
}

// TestFactory_GetSupportedTypes includes all 32 connectors.
func TestFactory_GetSupportedTypes(t *testing.T) {
	factory := GetFactory()
	types := factory.GetSupportedTypes()
	if len(types) < 32 {
		t.Errorf("expected at least 32 connector types, got %d", len(types))
	}
	// Spot-check key types
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	required := []string{
		"tcp_mllp_inbound", "tcp_mllp_outbound",
		"sftp_inbound", "sftp_outbound",
		"mongodb_outbound",
		"http_outbound", "http_rest_inbound",
	}
	for _, r := range required {
		if !typeSet[r] {
			t.Errorf("factory should support %q", r)
		}
	}
}
