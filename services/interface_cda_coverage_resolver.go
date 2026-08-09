// services/interface_cda_coverage_resolver.go
// Shared resolution of the per-interface CDA Coverage Audit opt-in flag
// (interfaces.cda_coverage_audit_config), mirroring
// interface_debug_resolver.go's ResolveInterfaceDebugLevel — a single,
// lightweight, timeout-bounded lookup used from ExecutePipeline so the
// interface row doesn't need to already be loaded there.
package services

import (
	"context"
	"database/sql"
	"time"
)

// ResolveCDACoverageAuditEnabled reports whether interfaceID has opted into
// CDA Coverage Audit. Disabled means either NULL cda_coverage_audit_config
// (the default — see V207 migration, never sent by this row's editor) or
// the JSON literal null (a real, non-SQL-NULL JSONB value that
// interfacesController.js's updateInterface writes when a user explicitly
// unchecks the feature (no schema change involved — same JSONB column,
// just a real `null` value instead of an absent one). Any other non-null
// value means enabled. Errors (including a nil
// db, or the row not being found) resolve to disabled — this is optional,
// best-effort observability, never something a lookup failure should turn
// into a pipeline error.
func ResolveCDACoverageAuditEnabled(db *sql.DB, interfaceID string) (enabled bool, err error) {
	if db == nil || interfaceID == "" {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	query := `SELECT cda_coverage_audit_config IS NOT NULL AND cda_coverage_audit_config <> 'null'::jsonb FROM interfaces WHERE id = $1`
	scanErr := db.QueryRowContext(ctx, query, interfaceID).Scan(&enabled)
	if scanErr != nil {
		return false, scanErr
	}
	return enabled, nil
}
