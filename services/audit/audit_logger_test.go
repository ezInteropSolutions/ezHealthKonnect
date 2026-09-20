package audit

import (
	"context"
	"testing"
)

func TestLookupTaxonomy_KnownAction_ReturnsRealEntry(t *testing.T) {
	entry := lookupTaxonomy("MESSAGE_RECEIVED")
	if entry.ATNAEventID != "110107" {
		t.Errorf("expected ATNA event id 110107, got %q", entry.ATNAEventID)
	}
	if entry.EventActionCode != "C" {
		t.Errorf("expected eventActionCode C, got %q", entry.EventActionCode)
	}
	if entry.DefaultResult != "success" {
		t.Errorf("expected defaultResult success, got %q", entry.DefaultResult)
	}
	if entry.DefaultRiskLevel != "low" {
		t.Errorf("expected defaultRiskLevel low, got %q", entry.DefaultRiskLevel)
	}
}

func TestLookupTaxonomy_UnknownAction_ReturnsGenericFallback(t *testing.T) {
	entry := lookupTaxonomy("SOME_ACTION_THAT_WILL_NEVER_BE_REGISTERED")
	if entry.ATNAEventID != "" {
		t.Errorf("expected no ATNA id for unregistered action, got %q", entry.ATNAEventID)
	}
	if entry.DefaultResult != "success" || entry.DefaultRiskLevel != "low" {
		t.Errorf("expected generic success/low fallback, got result=%q risk=%q", entry.DefaultResult, entry.DefaultRiskLevel)
	}
}

// Every action string referenced anywhere in the Go codebase's call sites
// must resolve to a real taxonomy entry — this is a regression guard against
// silently falling back to the generic default (which would still work, but
// would mean the taxonomy drifted out of sync with the code that emits it).
func TestLookupTaxonomy_AllWiredActionsAreRegistered(t *testing.T) {
	wiredActions := []string{
		"MESSAGE_RECEIVED",
		"MESSAGE_DELIVERED",
		"MESSAGE_DELIVERY_FAILED",
		"DLQ_MESSAGE_ENQUEUED",
		"DLQ_MESSAGE_REDRIVEN",
		"INTERFACE_ACTIVATED",
		"INTERFACE_DEACTIVATED",
		"APPLICATION_STARTED",
		"APPLICATION_STOPPED",
		"CDA_DEDUPE_REGISTRY_VIEWED",
		"CDA_DEDUPE_REGISTRY_PURGED",
		"CDA_DEDUPE_REGISTRY_RETENTION_PURGED",
		"PHI_SAFETY_HALT",
		"cda_fhir_transform",
		"transformation.pipeline.executed",
		"dlq_redrive",
		"dlq_schedule",
		"dlq_abandon",
	}
	tax := loadTaxonomy()
	for _, action := range wiredActions {
		if _, ok := tax[action]; !ok {
			t.Errorf("action %q is used by a Go call site but has no audit_events.json taxonomy entry", action)
		}
	}
}

func TestResolveEvent_NoOverrides_UsesTaxonomyDefaults(t *testing.T) {
	r := resolveEvent(AuditEvent{Action: "MESSAGE_DELIVERED"})
	if r.Result != "success" {
		t.Errorf("expected result success, got %q", r.Result)
	}
	if r.RiskLevel != "low" {
		t.Errorf("expected risk low, got %q", r.RiskLevel)
	}
	if r.ComplianceFlags["hipaa"] != true {
		t.Errorf("expected compliance_flags.hipaa=true from taxonomy default, got %v", r.ComplianceFlags)
	}
	atna, ok := r.Metadata["atna"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata.atna to be populated, got %v", r.Metadata)
	}
	if atna["event_id"] != "110106" {
		t.Errorf("expected atna.event_id 110106, got %v", atna["event_id"])
	}
	if atna["event_outcome_indicator"] != "0" {
		t.Errorf("expected outcome indicator 0 for success, got %v", atna["event_outcome_indicator"])
	}
}

func TestResolveEvent_ExplicitOverrides_WinOverTaxonomyDefaults(t *testing.T) {
	r := resolveEvent(AuditEvent{
		Action:          "MESSAGE_DELIVERED", // taxonomy default: success/low
		Result:          "failure",
		RiskLevel:       "critical",
		ComplianceFlags: map[string]interface{}{"custom": true},
	})
	if r.Result != "failure" {
		t.Errorf("expected overridden result failure, got %q", r.Result)
	}
	if r.RiskLevel != "critical" {
		t.Errorf("expected overridden risk critical, got %q", r.RiskLevel)
	}
	if r.ComplianceFlags["custom"] != true || r.ComplianceFlags["hipaa"] != nil {
		t.Errorf("expected caller-supplied compliance_flags to fully replace taxonomy default, got %v", r.ComplianceFlags)
	}
	atna := r.Metadata["atna"].(map[string]interface{})
	if atna["event_outcome_indicator"] != "4" {
		t.Errorf("expected outcome indicator 4 for failure, got %v", atna["event_outcome_indicator"])
	}
}

func TestResolveEvent_UnregisteredAction_StillProducesUsableDefaults(t *testing.T) {
	r := resolveEvent(AuditEvent{Action: "NOT_IN_TAXONOMY"})
	if r.Result != "success" || r.RiskLevel != "low" {
		t.Errorf("expected generic success/low fallback, got result=%q risk=%q", r.Result, r.RiskLevel)
	}
	if _, ok := r.Metadata["atna"]; ok {
		t.Errorf("expected no atna block for an unregistered action, got %v", r.Metadata["atna"])
	}
	if len(r.ComplianceFlags) != 0 {
		t.Errorf("expected empty compliance_flags for an unregistered action, got %v", r.ComplianceFlags)
	}
}

func TestResolveEvent_CallerMetadataPreservedAlongsideATNA(t *testing.T) {
	r := resolveEvent(AuditEvent{
		Action:   "MESSAGE_RECEIVED",
		Metadata: map[string]interface{}{"interface_id": "abc-123", "message_id": "msg-1"},
	})
	if r.Metadata["interface_id"] != "abc-123" || r.Metadata["message_id"] != "msg-1" {
		t.Errorf("expected caller metadata preserved, got %v", r.Metadata)
	}
	if _, ok := r.Metadata["atna"]; !ok {
		t.Errorf("expected atna block also present, got %v", r.Metadata)
	}
}

func TestPostgresAuditLogger_NilDB_DropsEventWithoutError(t *testing.T) {
	// NewPostgresAuditLogger returns the AuditLogger interface (not the
	// concrete *PostgresAuditLogger) so it can transparently wrap itself
	// with ATNA syslog export when configured — see syslog_audit_logger.go.
	var l AuditLogger = NewPostgresAuditLogger(nil)
	if err := l.Log(context.Background(), AuditEvent{Action: "MESSAGE_RECEIVED"}); err != nil {
		t.Errorf("expected nil-db Log to degrade gracefully, got error: %v", err)
	}
}

func TestPostgresAuditLogger_NilLogger_DropsEventWithoutError(t *testing.T) {
	var l *PostgresAuditLogger
	if err := l.Log(context.Background(), AuditEvent{Action: "MESSAGE_RECEIVED"}); err != nil {
		t.Errorf("expected nil *PostgresAuditLogger.Log to degrade gracefully, got error: %v", err)
	}
}

func TestPostgresAuditLogger_EmptyAction_ReturnsError(t *testing.T) {
	// Action validation runs before the nil-db short-circuit — an empty
	// Action is always a caller bug, independent of DB availability, so a
	// nil db (as in every other test in this file) must not mask it.
	l := NewPostgresAuditLogger(nil)
	err := l.Log(context.Background(), AuditEvent{Action: ""})
	if err == nil {
		t.Fatal("expected an error for empty Action, got nil")
	}
}
