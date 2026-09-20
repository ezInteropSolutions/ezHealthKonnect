// services/audit/audit_logger.go
//
// The single, shared Go audit-logging path for this codebase. Before this
// package existed, 6 call sites each hand-rolled their own raw
// `INSERT INTO audit_logs` with drifted vocabulary (result was
// success/failure/error in some, "blocked"/unset in others; risk_level was
// low/medium/high/critical in some, the literal non-enum string "info" in
// another) — and message-level PHI transmission (inbound receipt, outbound
// delivery, DLQ lifecycle, interface activate/deactivate) had zero coverage
// at all. AuditLogger fixes both: one generic AuditEvent, one taxonomy
// (audit_events.json) supplying the ATNA/RFC 3881 shape and HIPAA-compliance
// defaults per action, one Postgres-backed writer every caller shares.
//
// This is a leaf package (no ezhealthkonnect/* imports beyond nothing at
// all) so it can be constructor-injected anywhere in the codebase —
// including deep inside services/executors/* — without the import-cycle
// concerns that force other cross-cutting concerns (see
// models.DeliveryStatusFn's doc comment) through a context-value callback
// instead.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// AuditLogger is the interface every Go call site in this codebase uses to
// record a compliance-audit row. Accept this interface (never the concrete
// *PostgresAuditLogger) in constructors, per this repo's own DI mandate.
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent) error
}

// AuditEvent is the generic event every audit_logs row is built from — one
// struct for every event type (message lifecycle, DLQ lifecycle, interface
// lifecycle, application lifecycle, and the CDA/retention/transformation
// events migrated onto this logger), never one Go type per category.
//
// Result / RiskLevel / ComplianceFlags are optional overrides: leave them
// zero to use the audit_events.json taxonomy entry for Action. Set them
// explicitly only when the caller knows the real outcome differs from that
// action's default (e.g. a delivery failure using the MESSAGE_DELIVERED
// action would be a caller bug — use MESSAGE_DELIVERY_FAILED instead — but
// PHI_SAFETY_HALT's per-call phi_risk detail is exactly the kind of thing
// ComplianceFlags should override with).
//
// Never put raw clinical content, a patient name, or free text into
// Metadata / OldValues / NewValues — identifiers and coded references only.
// The message content itself already lives elsewhere (messages_intf_*.raw_
// message, object storage for outbound payloads) — carry InterfaceID/
// MessageID (via EntityType/EntityID or Metadata) so a row can be resolved
// back to that content, never duplicate it here. Where "which patient"
// matters for a future accounting-of-disclosures report, put the CDA/HL7
// identifier root+extension (never a name) into Metadata under
// "patient_key" — the same convention cda_dedupe_registry already
// established — once a caller actually has one available; nothing in this
// phase's chokepoints do yet.
type AuditEvent struct {
	Action     string // required; a key in audit_events.json (see lookupTaxonomy for the graceful-degradation fallback otherwise)
	UserID     string // uuid string; empty = SQL NULL (system/automated event, no interactive actor)
	SessionID  string
	EntityType string
	EntityID   string
	OldValues  map[string]interface{}
	NewValues  map[string]interface{}
	Metadata   map[string]interface{}
	IPAddress  string
	UserAgent  string
	RequestID  string
	DLQRowID   string // uuid string; empty = SQL NULL

	Result          string // "success" | "failure" | "error" — override; "" uses the taxonomy default
	ErrorMessage    string
	RiskLevel       string // "low" | "medium" | "high" | "critical" — override; "" uses the taxonomy default
	ComplianceFlags map[string]interface{}
}

// dbWriteTimeout bounds the detached background write below — an audit
// write must never let a genuinely stuck DB connection hang a caller
// forever, especially the synchronous controller call sites.
const dbWriteTimeout = 5 * time.Second

// PostgresAuditLogger writes AuditEvents to the audit_logs table.
type PostgresAuditLogger struct {
	db *sql.DB
}

// NewPostgresAuditLogger constructs the audit logger backed by db, and is
// the ONE construction point every one of this codebase's ~9 call sites
// uses (main.go, processing/engine.go, services/executor_registry.go, the
// DLQ/CDA/retention/pipeline services, etc.) — all of them already store
// the result in an audit.AuditLogger-typed field, never the concrete
// *PostgresAuditLogger (per this file's own DI-mandate comment on the
// AuditLogger interface), which is what makes it safe to return the
// interface here rather than the concrete type: transparently wrapping with
// ATNA syslog export (see syslog_audit_logger.go) needs zero changes at any
// of those 9 sites.
//
// Wraps UNCONDITIONALLY now (not gated on an env var check at construction
// time) — the wrapper itself re-resolves LIVE config (admin-UI settings via
// SettingsProvider, falling back to ATNA_SYSLOG_* env vars) on every single
// Log call and no-ops instantly when disabled, which is what lets an admin
// turn ATNA export on/off/reconfigured at runtime without a restart. A
// process that starts with export disabled (the default) pays only one
// cheap resolveSyslogConfig() call per audit event until it's ever enabled.
//
// db may be nil (dev/test mode with no database configured, or a unit test
// constructing its owning struct with db=nil) — Log then logs a warning and
// returns nil instead of writing, the same "degrade, never crash the
// caller" pattern every other optional dependency in this codebase follows.
func NewPostgresAuditLogger(db *sql.DB) AuditLogger {
	var logger AuditLogger = &PostgresAuditLogger{db: db}
	return newLiveSyslogAuditLogger(logger)
}

// atnaOutcomeIndicator maps a result value onto RFC 3881's
// EventOutcomeIndicator numeric code (0=Success, 4=Minor failure,
// 8=Serious failure, 12=Major failure — this codebase's Result vocabulary
// never reaches the "Major failure" tier, so 8 is the ceiling used here).
func atnaOutcomeIndicator(result string) string {
	switch result {
	case "success":
		return "0"
	case "failure":
		return "4"
	case "error":
		return "8"
	default:
		return "4"
	}
}

// Log writes one audit_logs row for event. The taxonomy entry for
// event.Action supplies ATNA identity fields (folded into metadata.atna,
// since audit_logs has no dedicated ATNA columns yet — see this file's own
// package doc) and the HIPAA-compliance defaults event doesn't override.
//
// The actual INSERT runs against a context detached from ctx (bounded by
// dbWriteTimeout instead) — mirroring the pre-existing precedent in
// transformation_pipeline_service.go's writeTransformationAudit, which
// explicitly uses context.Background() so a caller's HTTP request context
// expiring (or a shutdown context firing) can never silently drop a
// compliance-critical audit row.
func (l *PostgresAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	if event.Action == "" {
		return fmt.Errorf("audit: Action is required")
	}
	if l == nil || l.db == nil {
		log.Printf("⚠️  [audit] no database configured — dropping audit event %q", event.Action)
		return nil
	}

	resolved := resolveEvent(event)

	metadataJSON, err := json.Marshal(resolved.Metadata)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}
	complianceJSON, err := json.Marshal(resolved.ComplianceFlags)
	if err != nil {
		return fmt.Errorf("audit: marshal compliance_flags: %w", err)
	}

	var oldValuesJSON, newValuesJSON interface{}
	if event.OldValues != nil {
		b, err := json.Marshal(event.OldValues)
		if err != nil {
			return fmt.Errorf("audit: marshal old_values: %w", err)
		}
		oldValuesJSON = string(b)
	}
	if event.NewValues != nil {
		b, err := json.Marshal(event.NewValues)
		if err != nil {
			return fmt.Errorf("audit: marshal new_values: %w", err)
		}
		newValuesJSON = string(b)
	}

	writeCtx, cancel := context.WithTimeout(context.Background(), dbWriteTimeout)
	defer cancel()

	_, execErr := l.db.ExecContext(writeCtx, `
		INSERT INTO audit_logs (
			user_id, session_id, action, entity_type, entity_id,
			old_values, new_values, metadata, ip_address, user_agent,
			request_id, result, error_message, risk_level, compliance_flags,
			dlq_row_id, created_at
		) VALUES (
			NULLIF($1,'')::uuid, $2, $3, $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, NULLIF($9,'')::inet, $10,
			$11, $12, $13, $14, $15::jsonb,
			NULLIF($16,'')::uuid, NOW()
		)`,
		event.UserID, nullIfEmpty(event.SessionID), event.Action, nullIfEmpty(event.EntityType), nullIfEmpty(event.EntityID),
		oldValuesJSON, newValuesJSON, string(metadataJSON), event.IPAddress, nullIfEmpty(event.UserAgent),
		nullIfEmpty(event.RequestID), resolved.Result, nullIfEmpty(event.ErrorMessage), resolved.RiskLevel, string(complianceJSON),
		event.DLQRowID,
	)
	return execErr
}

// resolvedEvent is the pure, DB-free result of applying event's overrides
// (or lack thereof) onto its taxonomy entry — factored out of Log so the
// taxonomy-resolution and ATNA-enrichment logic is unit-testable without a
// database.
type resolvedEvent struct {
	Result          string
	RiskLevel       string
	ComplianceFlags map[string]interface{}
	Metadata        map[string]interface{}
}

func resolveEvent(event AuditEvent) resolvedEvent {
	tax := lookupTaxonomy(event.Action)

	result := event.Result
	if result == "" {
		result = tax.DefaultResult
	}
	if result == "" {
		result = "success"
	}

	riskLevel := event.RiskLevel
	if riskLevel == "" {
		riskLevel = tax.DefaultRiskLevel
	}
	if riskLevel == "" {
		riskLevel = "low"
	}

	complianceFlags := event.ComplianceFlags
	if complianceFlags == nil {
		complianceFlags = tax.ComplianceFlags
	}
	if complianceFlags == nil {
		complianceFlags = map[string]interface{}{}
	}

	metadata := make(map[string]interface{}, len(event.Metadata)+1)
	for k, v := range event.Metadata {
		metadata[k] = v
	}
	if tax.ATNAEventID != "" {
		metadata["atna"] = map[string]interface{}{
			"event_id":                tax.ATNAEventID,
			"event_id_display":        tax.ATNAEventIDDisplay,
			"event_action_code":       tax.EventActionCode,
			"event_outcome_indicator": atnaOutcomeIndicator(result),
		}
	}

	return resolvedEvent{
		Result:          result,
		RiskLevel:       riskLevel,
		ComplianceFlags: complianceFlags,
		Metadata:        metadata,
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
