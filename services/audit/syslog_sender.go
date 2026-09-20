// services/audit/syslog_sender.go
//
// RFC 5424 ("The Syslog Protocol") message framing + UDP/TCP(+TLS) delivery
// for ATNA audit export — the transport IHE ATNA's own Audit Trail
// transaction (ITI-20) is conventionally carried over, delivering the
// AuditMessage XML (atna_message.go) to an external Audit Record Repository
// (ARR). Configured entirely via environment variables (see
// syslogConfigFromEnv), disabled by default — ATNA_SYSLOG_HOST unset means
// no exporter is constructed at all (see syslog_audit_logger.go).
package audit

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// SyslogConfig configures the ATNA syslog exporter.
type SyslogConfig struct {
	Host                  string
	Port                  int    // default: 514 (udp/tcp) or 6514 (tls)
	Protocol              string // "udp" | "tcp" | "tls" — default "udp"
	Facility              int    // RFC 5424 facility number 0-23 — default 10 (security/authorization messages)
	AppName               string // RFC 5424 APP-NAME — default "ezHealthKonnect"
	AuditSourceID         string // DICOM AuditSourceIdentification/AuditSourceID — default = AppName
	EnterpriseSiteID      string // DICOM AuditSourceIdentification/AuditEnterpriseSiteID — optional
	TLSInsecureSkipVerify bool
	DialTimeout           time.Duration
	WriteTimeout          time.Duration
}

// syslogConfigFromEnv builds a SyslogConfig from ATNA_SYSLOG_* environment
// variables. The second return value is false (config zero-valued) whenever
// ATNA_SYSLOG_HOST is unset — the exporter is opt-in, never active by
// accident in a deployment that never configured an ARR target.
func syslogConfigFromEnv() (SyslogConfig, bool) {
	host := strings.TrimSpace(os.Getenv("ATNA_SYSLOG_HOST"))
	if host == "" {
		return SyslogConfig{}, false
	}

	protocol := strings.ToLower(strings.TrimSpace(os.Getenv("ATNA_SYSLOG_PROTOCOL")))
	if protocol == "" {
		protocol = "udp"
	}

	port := 514
	if protocol == "tls" {
		port = 6514
	}
	if p := strings.TrimSpace(os.Getenv("ATNA_SYSLOG_PORT")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n <= 65535 {
			port = n
		}
	}

	facility := 10 // security/authorization messages (RFC 5424 Table 1)
	if f := strings.TrimSpace(os.Getenv("ATNA_SYSLOG_FACILITY")); f != "" {
		if n, err := strconv.Atoi(f); err == nil && n >= 0 && n <= 23 {
			facility = n
		}
	}

	appName := strings.TrimSpace(os.Getenv("ATNA_SYSLOG_APP_NAME"))
	if appName == "" {
		appName = "ezHealthKonnect"
	}

	sourceID := strings.TrimSpace(os.Getenv("ATNA_AUDIT_SOURCE_ID"))
	if sourceID == "" {
		sourceID = appName
	}

	return SyslogConfig{
		Host:                  host,
		Port:                  port,
		Protocol:              protocol,
		Facility:              facility,
		AppName:               appName,
		AuditSourceID:         sourceID,
		EnterpriseSiteID:      strings.TrimSpace(os.Getenv("ATNA_AUDIT_ENTERPRISE_SITE_ID")),
		TLSInsecureSkipVerify: strings.EqualFold(strings.TrimSpace(os.Getenv("ATNA_SYSLOG_TLS_INSECURE_SKIP_VERIFY")), "true"),
		DialTimeout:           5 * time.Second,
		WriteTimeout:          5 * time.Second,
	}, true
}

// SettingsProvider, when set, supplies the LIVE, admin-UI-editable ATNA
// syslog config (services.AppSettingsCache.GetATNASyslogSettings(), read
// fresh on every export attempt) — set exactly once, from main.go, which can
// import both this leaf package and services/app_settings.go without a
// cycle (this package's own doc comment explains why it can't import
// "services" directly: several services/*.go files already import
// services/audit for AuditLogger, so the reverse import would cycle).
//
// Left nil by default (e.g. every unit test in this package, which
// constructs SyslogAuditLogger directly with no main.go wiring) — in that
// case resolveSyslogConfig falls back to syslogConfigFromEnv unconditionally.
// When set, the provider itself decides whether to fall back to env vars
// (its own second return value false) — see main.go's own wiring comment.
var SettingsProvider func() (SyslogConfig, bool)

// resolveSyslogConfig is what SyslogAuditLogger.Log calls on every audit
// event — never cached beyond the provider's own caching (AppSettingsCache's
// 5-minute TTL, invalidated immediately on an admin-UI save), so enabling,
// disabling, or reconfiguring ATNA export via the admin UI takes effect
// without a process restart.
func resolveSyslogConfig() (SyslogConfig, bool) {
	if SettingsProvider != nil {
		if cfg, enabled := SettingsProvider(); enabled {
			return cfg, true
		}
	}
	return syslogConfigFromEnv()
}

// syslogSender sends pre-formatted RFC 5424 messages to the configured ARR.
// UDP is connectionless (each Send dials fresh — cheap, no persistent state
// to leak on a network blip). TCP/TLS keep one persistent connection,
// reconnecting lazily on the next Send after a failure — mirroring this
// codebase's own established "outbound connector" reconnect-on-next-use
// convention (e.g. websocket_outbound.go) rather than a bespoke pattern.
type syslogSender struct {
	cfg  SyslogConfig
	conn net.Conn // only used for tcp/tls; nil for udp
}

func newSyslogSender(cfg SyslogConfig) *syslogSender {
	return &syslogSender{cfg: cfg}
}

func (s *syslogSender) addr() string {
	return net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
}

// Send delivers one already-framed RFC 5424 message (see buildSyslogMessage).
// Errors are returned for the caller to log — see syslog_audit_logger.go's
// own "never let export failure affect the compliance-critical DB write"
// handling; this function itself has no such opinion.
func (s *syslogSender) Send(message []byte) error {
	switch s.cfg.Protocol {
	case "udp":
		return s.sendUDP(message)
	case "tcp", "tls":
		return s.sendStream(message)
	default:
		return fmt.Errorf("atna syslog: unsupported protocol %q (expected udp, tcp, or tls)", s.cfg.Protocol)
	}
}

func (s *syslogSender) sendUDP(message []byte) error {
	conn, err := net.DialTimeout("udp", s.addr(), s.cfg.DialTimeout)
	if err != nil {
		return fmt.Errorf("atna syslog: udp dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
	_, err = conn.Write(message)
	if err != nil {
		return fmt.Errorf("atna syslog: udp write: %w", err)
	}
	return nil
}

// sendStream handles both tcp and tls — RFC 5425 octet-counted framing
// ("MSG-LEN SP SYSLOG-MSG") is used for both, since a plain newline-
// delimited framing is ambiguous if the XML payload itself ever contains a
// literal newline. Reuses s.conn across calls; reconnects transparently if
// the previous connection is gone or the write fails.
func (s *syslogSender) sendStream(message []byte) error {
	framed := fmt.Appendf(nil, "%d %s", len(message), message)

	if s.conn != nil {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
		if _, err := s.conn.Write(framed); err == nil {
			return nil
		}
		_ = s.conn.Close()
		s.conn = nil
	}

	conn, err := s.dialStream()
	if err != nil {
		return fmt.Errorf("atna syslog: %s dial: %w", s.cfg.Protocol, err)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
	if _, err := conn.Write(framed); err != nil {
		_ = conn.Close()
		return fmt.Errorf("atna syslog: %s write: %w", s.cfg.Protocol, err)
	}
	s.conn = conn
	return nil
}

func (s *syslogSender) dialStream() (net.Conn, error) {
	if s.cfg.Protocol == "tls" {
		dialer := &net.Dialer{Timeout: s.cfg.DialTimeout}
		return tls.DialWithDialer(dialer, "tcp", s.addr(), &tls.Config{
			InsecureSkipVerify: s.cfg.TLSInsecureSkipVerify,
		})
	}
	return net.DialTimeout("tcp", s.addr(), s.cfg.DialTimeout)
}

func (s *syslogSender) Close() error {
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

// syslogSeverity maps this codebase's risk_level vocabulary onto RFC 5424
// severity (0=Emergency..7=Debug) — a coarse but standard, well-defined
// mapping any syslog-consuming SIEM/ARR already understands.
func syslogSeverity(riskLevel string) int {
	switch riskLevel {
	case "critical":
		return 2 // Critical
	case "high":
		return 3 // Error
	case "medium":
		return 5 // Notice
	case "low":
		return 6 // Informational
	default:
		return 6
	}
}

// buildSyslogMessage frames body (the AuditMessage XML) as one RFC 5424
// message: "<PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID
// STRUCTURED-DATA BOM+MSG". STRUCTURED-DATA is "-" (NILVALUE) — every
// structured detail already lives in the XML MSG body itself, so a second,
// parallel SD-ID encoding would just duplicate it.
func buildSyslogMessage(cfg SyslogConfig, riskLevel string, atnaEventID string, body []byte) []byte {
	pri := cfg.Facility*8 + syslogSeverity(riskLevel)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")
	hostname := syslogHostname()
	procID := strconv.Itoa(os.Getpid())
	msgID := atnaEventID
	if msgID == "" {
		msgID = "-"
	}

	header := fmt.Sprintf("<%d>1 %s %s %s %s %s - ", pri, timestamp, hostname, cfg.AppName, procID, msgID)

	// RFC 5424 §6.4: a UTF-8 MSG SHOULD be preceded by the UTF-8 BOM so a
	// receiver can distinguish it from other encodings.
	bom := []byte{0xEF, 0xBB, 0xBF}

	out := make([]byte, 0, len(header)+len(bom)+len(body))
	out = append(out, []byte(header)...)
	out = append(out, bom...)
	out = append(out, body...)
	return out
}

func syslogHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "-"
	}
	return h
}
