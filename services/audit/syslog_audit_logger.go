// services/audit/syslog_audit_logger.go
//
// SyslogAuditLogger decorates an existing AuditLogger with best-effort ATNA
// syslog export, added ADDITIVELY: every one of this codebase's ~9
// NewPostgresAuditLogger(db) call sites already stores the result in an
// audit.AuditLogger-typed field (never the concrete *PostgresAuditLogger —
// see that constructor's own DI-mandate comment), so wrapping happens
// entirely INSIDE NewPostgresAuditLogger itself rather than touching any of
// those 9 sites.
//
// NewPostgresAuditLogger now wraps UNCONDITIONALLY (not just when
// ATNA_SYSLOG_HOST was set at startup) — Log itself resolves the LIVE config
// (resolveSyslogConfig, admin-UI settings falling back to env vars) on every
// call and no-ops (a fast, synchronous early return — no goroutine, no
// network I/O) whenever export is disabled. This is what lets an admin
// enable/disable/reconfigure ATNA export at runtime without a restart.
package audit

import (
	"context"
	"log"
	"sync"
)

// SyslogAuditLogger wraps an inner AuditLogger (the compliance-critical,
// primary write — Postgres audit_logs) and additionally, best-effort,
// forwards every event to an external ATNA Audit Record Repository over
// syslog. The inner write's own success/failure is what Log returns; a
// syslog export failure is logged and never affects the caller — an
// unreachable/misconfigured ARR must never be able to break message
// processing, interface management, or anything else in this codebase that
// calls AuditLogger.Log.
type SyslogAuditLogger struct {
	inner AuditLogger

	// fixedCfg, when non-nil, pins Log to this exact config forever —
	// export is always enabled, never re-resolved. This is the LEGACY/TEST
	// mode: NewSyslogAuditLogger's own signature (below) is part of this
	// package's "exported for unit testing" surface (services/audit/
	// atna_syslog_test.go constructs it directly with a fixed config
	// pointed at a local test listener) and stays unchanged so those tests
	// keep working untouched. Real production wiring (NewPostgresAuditLogger)
	// uses newLiveSyslogAuditLogger instead, which leaves this nil so every
	// Log call re-resolves via resolveSyslogConfig().
	fixedCfg *SyslogConfig

	mu        sync.Mutex
	sender    *syslogSender
	senderCfg SyslogConfig
}

// NewSyslogAuditLogger wraps inner with ATNA syslog export permanently
// pinned to cfg. inner may be any AuditLogger (normally a
// *PostgresAuditLogger, but decorator composition doesn't care).
func NewSyslogAuditLogger(inner AuditLogger, cfg SyslogConfig) *SyslogAuditLogger {
	return &SyslogAuditLogger{inner: inner, fixedCfg: &cfg}
}

// newLiveSyslogAuditLogger wraps inner with ATNA syslog export that
// re-resolves its own config (admin settings, falling back to env vars) on
// every single Log call — see resolveConfig below. Unexported: only
// NewPostgresAuditLogger uses this; anything constructing a
// SyslogAuditLogger directly (tests, or a future caller that genuinely
// wants a fixed target) should use the exported NewSyslogAuditLogger above.
func newLiveSyslogAuditLogger(inner AuditLogger) *SyslogAuditLogger {
	return &SyslogAuditLogger{inner: inner}
}

func (l *SyslogAuditLogger) resolveConfig() (SyslogConfig, bool) {
	if l.fixedCfg != nil {
		return *l.fixedCfg, true
	}
	return resolveSyslogConfig()
}

// senderFor returns a syslogSender for cfg, rebuilding (and closing the old
// one) only when cfg has actually changed since the last call — the common
// case (a stable configuration) keeps reusing one persistent TCP/TLS
// connection instead of reconnecting on every audit event. SyslogConfig's
// fields are all plain comparable types (string/int/bool/time.Duration), so
// a direct == comparison is enough — no custom hashing needed.
func (l *SyslogAuditLogger) senderFor(cfg SyslogConfig) *syslogSender {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sender == nil || l.senderCfg != cfg {
		if l.sender != nil {
			_ = l.sender.Close()
		}
		l.sender = newSyslogSender(cfg)
		l.senderCfg = cfg
	}
	return l.sender
}

// closeSender drops any open connection — called when export transitions to
// disabled, so a live-reconfigured logger doesn't hold a stale, unused
// TCP/TLS connection open indefinitely.
func (l *SyslogAuditLogger) closeSender() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sender != nil {
		_ = l.sender.Close()
		l.sender = nil
		l.senderCfg = SyslogConfig{}
	}
}

// Log writes to the inner logger synchronously (its result is what's
// returned — unchanged contract from calling inner.Log directly), then
// fires the syslog export in a background goroutine. The export is
// deliberately asynchronous and non-blocking: unlike the local Postgres
// write, an external ARR is a genuinely optional, secondary destination
// that may be slow or briefly unreachable over a real network, and no
// caller on this codebase's hot paths (HTTP handlers, message processing)
// should ever wait on it.
func (l *SyslogAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	err := l.inner.Log(ctx, event)

	cfg, enabled := l.resolveConfig()
	if !enabled {
		l.closeSender()
		return err
	}

	resolved := resolveEvent(event)
	go func() {
		sender := l.senderFor(cfg)
		body, buildErr := buildAuditMessageXML(event, resolved, cfg.AuditSourceID, cfg.EnterpriseSiteID)
		if buildErr != nil {
			log.Printf("⚠️  [atna-syslog] failed to build AuditMessage XML for action %q: %v", event.Action, buildErr)
			return
		}
		tax := lookupTaxonomy(event.Action)
		message := buildSyslogMessage(cfg, resolved.RiskLevel, tax.ATNAEventID, body)
		if sendErr := sender.Send(message); sendErr != nil {
			log.Printf("⚠️  [atna-syslog] failed to export action %q to %s:%d (%s): %v",
				event.Action, cfg.Host, cfg.Port, cfg.Protocol, sendErr)
		}
	}()

	return err
}

// Close releases the underlying syslog connection (tcp/tls only — a no-op
// for udp). Not part of the AuditLogger interface; callers that construct a
// SyslogAuditLogger directly (rather than via NewPostgresAuditLogger) may
// call this on their own shutdown path if they want a clean disconnect —
// every existing construction site in this codebase is long-lived for the
// process's whole lifetime, so none needs to.
func (l *SyslogAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sender == nil {
		return nil
	}
	return l.sender.Close()
}
