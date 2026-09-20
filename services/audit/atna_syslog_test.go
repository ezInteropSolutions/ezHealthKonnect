// services/audit/atna_syslog_test.go
//
// Tests for the ATNA syslog exporter (atna_message.go, syslog_sender.go,
// syslog_audit_logger.go). The wire-level tests use real local UDP/TCP
// listeners (127.0.0.1:0, an OS-assigned ephemeral port) rather than mocks
// — this is genuine network I/O, not a simulation, giving real confidence
// the RFC 5424 framing and DICOM AuditMessage XML actually arrive intact
// over the wire, without needing external test infrastructure.
package audit

import (
	"context"
	"encoding/xml"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildAuditMessageXML_WellFormedAndPopulated(t *testing.T) {
	event := AuditEvent{
		Action:     "LOGIN_SUCCESS",
		UserID:     "user-123",
		EntityType: "User",
		EntityID:   "user-123",
		IPAddress:  "10.0.0.5",
	}
	resolved := resolveEvent(event)

	body, err := buildAuditMessageXML(event, resolved, "ezHealthKonnect", "site-1")
	if err != nil {
		t.Fatalf("buildAuditMessageXML failed: %v", err)
	}

	var parsed auditMessageXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("produced XML does not parse: %v\nXML: %s", err, body)
	}

	if parsed.EventIdentification.EventID.Code != "110114" {
		t.Errorf("expected EventID code 110114 (User Authentication) for LOGIN_SUCCESS, got %q", parsed.EventIdentification.EventID.Code)
	}
	if parsed.EventIdentification.EventOutcomeIndicator != "0" {
		t.Errorf("expected outcome indicator 0 (success), got %q", parsed.EventIdentification.EventOutcomeIndicator)
	}
	if parsed.ActiveParticipant.UserID != "user-123" {
		t.Errorf("expected UserID user-123, got %q", parsed.ActiveParticipant.UserID)
	}
	if parsed.ActiveParticipant.NetworkAccessPointID != "10.0.0.5" {
		t.Errorf("expected NetworkAccessPointID 10.0.0.5, got %q", parsed.ActiveParticipant.NetworkAccessPointID)
	}
	if parsed.AuditSourceIdentification.AuditSourceID != "ezHealthKonnect" {
		t.Errorf("expected AuditSourceID ezHealthKonnect, got %q", parsed.AuditSourceIdentification.AuditSourceID)
	}
	if parsed.AuditSourceIdentification.AuditEnterpriseSiteID != "site-1" {
		t.Errorf("expected AuditEnterpriseSiteID site-1, got %q", parsed.AuditSourceIdentification.AuditEnterpriseSiteID)
	}
	if parsed.ParticipantObjectIdentification == nil {
		t.Fatal("expected ParticipantObjectIdentification to be populated (EntityID was set)")
	}
	if parsed.ParticipantObjectIdentification.ParticipantObjectID != "user-123" {
		t.Errorf("expected ParticipantObjectID user-123, got %q", parsed.ParticipantObjectIdentification.ParticipantObjectID)
	}
	if parsed.ParticipantObjectIdentification.ParticipantObjectTypeCode != "1" {
		t.Errorf("expected ParticipantObjectTypeCode 1 (Person) for EntityType=User, got %q", parsed.ParticipantObjectIdentification.ParticipantObjectTypeCode)
	}
}

func TestBuildAuditMessageXML_NonUserEntity_UsesSystemObjectTypeCode(t *testing.T) {
	event := AuditEvent{Action: "INTERFACE_UPDATED", EntityType: "Interface", EntityID: "iface-1"}
	resolved := resolveEvent(event)
	body, err := buildAuditMessageXML(event, resolved, "app", "")
	if err != nil {
		t.Fatalf("buildAuditMessageXML failed: %v", err)
	}
	var parsed auditMessageXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("produced XML does not parse: %v", err)
	}
	if parsed.ParticipantObjectIdentification.ParticipantObjectTypeCode != "2" {
		t.Errorf("expected ParticipantObjectTypeCode 2 (System Object) for a non-User entity, got %q", parsed.ParticipantObjectIdentification.ParticipantObjectTypeCode)
	}
}

func TestBuildAuditMessageXML_NoEntityID_OmitsParticipantObject(t *testing.T) {
	event := AuditEvent{Action: "APPLICATION_STARTED"}
	resolved := resolveEvent(event)
	body, err := buildAuditMessageXML(event, resolved, "app", "")
	if err != nil {
		t.Fatalf("buildAuditMessageXML failed: %v", err)
	}
	if strings.Contains(string(body), "ParticipantObjectIdentification") {
		t.Errorf("expected no ParticipantObjectIdentification element when EntityID is empty, got: %s", body)
	}
}

func TestBuildAuditMessageXML_UnregisteredAction_FallsBackToGenericEventID(t *testing.T) {
	event := AuditEvent{Action: "SOME_TOTALLY_UNKNOWN_ACTION"}
	resolved := resolveEvent(event)
	body, err := buildAuditMessageXML(event, resolved, "app", "")
	if err != nil {
		t.Fatalf("buildAuditMessageXML failed: %v", err)
	}
	var parsed auditMessageXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("produced XML does not parse: %v", err)
	}
	if parsed.EventIdentification.EventID.Code != "110100" {
		t.Errorf("expected fallback EventID 110100 (Application Activity), got %q", parsed.EventIdentification.EventID.Code)
	}
}

func TestBuildAuditMessageXML_NoUserID_DefaultsToSystem(t *testing.T) {
	event := AuditEvent{Action: "APPLICATION_STARTED"}
	resolved := resolveEvent(event)
	body, _ := buildAuditMessageXML(event, resolved, "app", "")
	var parsed auditMessageXML
	_ = xml.Unmarshal(body, &parsed)
	if parsed.ActiveParticipant.UserID != "system" {
		t.Errorf("expected UserID to default to 'system' for a userless event, got %q", parsed.ActiveParticipant.UserID)
	}
}

func TestSyslogSeverity_MapsRiskLevelsCorrectly(t *testing.T) {
	cases := map[string]int{
		"critical": 2,
		"high":     3,
		"medium":   5,
		"low":      6,
		"":         6,
		"unknown":  6,
	}
	for risk, want := range cases {
		if got := syslogSeverity(risk); got != want {
			t.Errorf("syslogSeverity(%q) = %d, want %d", risk, got, want)
		}
	}
}

func TestBuildSyslogMessage_RFC5424Framing(t *testing.T) {
	cfg := SyslogConfig{Facility: 10, AppName: "ezHealthKonnect"}
	body := []byte(`<AuditMessage/>`)
	msg := buildSyslogMessage(cfg, "high", "110100", body)
	s := string(msg)

	// PRI = facility*8 + severity = 10*8 + 3 (high->error) = 83
	if !strings.HasPrefix(s, "<83>1 ") {
		t.Errorf("expected PRI <83>1 prefix (facility=10, severity=3 for high risk), got prefix: %q", s[:min(20, len(s))])
	}
	if !strings.Contains(s, "ezHealthKonnect") {
		t.Errorf("expected APP-NAME ezHealthKonnect in message, got: %s", s)
	}
	if !strings.Contains(s, "110100") {
		t.Errorf("expected MSGID 110100 in message, got: %s", s)
	}
	if !strings.Contains(s, "<AuditMessage/>") {
		t.Errorf("expected the AuditMessage XML body present in message, got: %s", s)
	}
	// UTF-8 BOM (EF BB BF) must precede the MSG body per RFC 5424 §6.4.
	bomIdx := strings.Index(s, "\xEF\xBB\xBF")
	bodyIdx := strings.Index(s, "<AuditMessage/>")
	if bomIdx == -1 || bomIdx+3 != bodyIdx {
		t.Errorf("expected UTF-8 BOM immediately before the MSG body, got message: %q", s)
	}
}

func TestSyslogConfigFromEnv_DisabledWhenHostUnset(t *testing.T) {
	os.Unsetenv("ATNA_SYSLOG_HOST")
	_, enabled := syslogConfigFromEnv()
	if enabled {
		t.Error("expected syslogConfigFromEnv to report disabled when ATNA_SYSLOG_HOST is unset")
	}
}

func TestSyslogConfigFromEnv_DefaultsAndOverrides(t *testing.T) {
	t.Setenv("ATNA_SYSLOG_HOST", "arr.example.internal")
	t.Setenv("ATNA_SYSLOG_PROTOCOL", "")
	t.Setenv("ATNA_SYSLOG_PORT", "")
	t.Setenv("ATNA_SYSLOG_FACILITY", "")
	t.Setenv("ATNA_SYSLOG_APP_NAME", "")
	t.Setenv("ATNA_AUDIT_SOURCE_ID", "")

	cfg, enabled := syslogConfigFromEnv()
	if !enabled {
		t.Fatal("expected enabled=true when ATNA_SYSLOG_HOST is set")
	}
	if cfg.Protocol != "udp" {
		t.Errorf("expected default protocol udp, got %q", cfg.Protocol)
	}
	if cfg.Port != 514 {
		t.Errorf("expected default port 514 for udp, got %d", cfg.Port)
	}
	if cfg.Facility != 10 {
		t.Errorf("expected default facility 10, got %d", cfg.Facility)
	}
	if cfg.AppName != "ezHealthKonnect" {
		t.Errorf("expected default app name ezHealthKonnect, got %q", cfg.AppName)
	}
	if cfg.AuditSourceID != "ezHealthKonnect" {
		t.Errorf("expected AuditSourceID to default to AppName, got %q", cfg.AuditSourceID)
	}

	t.Setenv("ATNA_SYSLOG_PROTOCOL", "tls")
	cfg2, _ := syslogConfigFromEnv()
	if cfg2.Port != 6514 {
		t.Errorf("expected default port 6514 for tls, got %d", cfg2.Port)
	}

	t.Setenv("ATNA_SYSLOG_PORT", "1514")
	cfg3, _ := syslogConfigFromEnv()
	if cfg3.Port != 1514 {
		t.Errorf("expected explicit port override 1514, got %d", cfg3.Port)
	}
}

// --- Real wire-level tests: actual local UDP/TCP listeners, not mocks ---

func TestSyslogSender_UDP_RealDelivery(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test UDP listener: %v", err)
	}
	defer pc.Close()

	host, portStr, _ := net.SplitHostPort(pc.LocalAddr().String())
	cfg := SyslogConfig{Host: host, Port: mustAtoi(t, portStr), Protocol: "udp", Facility: 10, AppName: "test", DialTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second}

	sender := newSyslogSender(cfg)
	testMsg := buildSyslogMessage(cfg, "low", "110100", []byte(`<AuditMessage>test-udp</AuditMessage>`))

	if err := sender.Send(testMsg); err != nil {
		t.Fatalf("Send over UDP failed: %v", err)
	}

	buf := make([]byte, 4096)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("did not receive UDP packet: %v", err)
	}
	received := string(buf[:n])
	if !strings.Contains(received, "test-udp") {
		t.Errorf("received UDP packet did not contain expected content: %s", received)
	}
}

func TestSyslogSender_TCP_RealDelivery_OctetCountedFraming(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test TCP listener: %v", err)
	}
	defer ln.Close()

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	cfg := SyslogConfig{Host: host, Port: mustAtoi(t, portStr), Protocol: "tcp", Facility: 10, AppName: "test", DialTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second}

	sender := newSyslogSender(cfg)
	defer sender.Close()
	testMsg := buildSyslogMessage(cfg, "low", "110100", []byte(`<AuditMessage>test-tcp</AuditMessage>`))

	if err := sender.Send(testMsg); err != nil {
		t.Fatalf("Send over TCP failed: %v", err)
	}

	select {
	case got := <-received:
		if !strings.Contains(got, "test-tcp") {
			t.Errorf("received TCP data did not contain expected content: %s", got)
		}
		// Verify octet-counted framing: "<len> <message...>"
		spaceIdx := strings.Index(got, " ")
		if spaceIdx == -1 {
			t.Fatalf("expected octet-counted framing 'LEN MSG', got: %s", got)
		}
		declaredLen := mustAtoi(t, got[:spaceIdx])
		actualMsgLen := len(got) - spaceIdx - 1
		if declaredLen != actualMsgLen {
			t.Errorf("octet-count mismatch: declared %d, actual message length %d", declaredLen, actualMsgLen)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TCP delivery")
	}
}

func TestSyslogAuditLogger_Log_CallsInnerAndExportsAsync(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test UDP listener: %v", err)
	}
	defer pc.Close()

	host, portStr, _ := net.SplitHostPort(pc.LocalAddr().String())
	cfg := SyslogConfig{Host: host, Port: mustAtoi(t, portStr), Protocol: "udp", Facility: 10, AppName: "test", AuditSourceID: "test-src", DialTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second}

	inner := &fakeInnerLogger{}
	wrapped := NewSyslogAuditLogger(inner, cfg)

	event := AuditEvent{Action: "LOGIN_SUCCESS", UserID: "u1"}
	if err := wrapped.Log(context.Background(), event); err != nil {
		t.Fatalf("Log returned an error: %v", err)
	}
	if len(inner.events) != 1 || inner.events[0].Action != "LOGIN_SUCCESS" {
		t.Errorf("expected the inner logger to receive the event synchronously, got: %+v", inner.events)
	}

	buf := make([]byte, 4096)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("expected an async syslog export to arrive, got error: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "110114") { // LOGIN_SUCCESS's real ATNA EventID
		t.Errorf("expected exported message to carry EventID 110114, got: %s", buf[:n])
	}
}

func TestSyslogAuditLogger_Log_InnerErrorStillReturnedEvenIfExportFails(t *testing.T) {
	// Point at a port nothing is listening on (deliberately unreachable for
	// a TCP dial) — the export should fail silently in the background while
	// the inner logger's own error is still what Log returns.
	cfg := SyslogConfig{Host: "127.0.0.1", Port: 1, Protocol: "tcp", DialTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond}
	inner := &fakeInnerLogger{err: context.DeadlineExceeded}
	wrapped := NewSyslogAuditLogger(inner, cfg)

	err := wrapped.Log(context.Background(), AuditEvent{Action: "LOGIN_SUCCESS"})
	if err != context.DeadlineExceeded {
		t.Errorf("expected Log to return the inner logger's own error unchanged, got: %v", err)
	}
	// Give the background export goroutine a moment to fail-and-log, just to
	// confirm it doesn't panic or hang the test process.
	time.Sleep(300 * time.Millisecond)
}

type fakeInnerLogger struct {
	events []AuditEvent
	err    error
}

func (f *fakeInnerLogger) Log(ctx context.Context, event AuditEvent) error {
	f.events = append(f.events, event)
	return f.err
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("mustAtoi: %q is not numeric", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}
