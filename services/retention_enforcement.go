// services/retention_enforcement.go
//
// RetentionEnforcementService enforces data retention across all message and
// operational tables. Runs on a configurable schedule (default: hourly).
//
// Retention is MANDATORY, not advisory — healthcare data must not be retained
// beyond the configured period. Every run logs exactly how many rows were purged
// so the HIPAA audit trail is complete.
//
// Tables managed:
//   messages_intf_*                  — per-interface message tables (MessageRetentionDays)
//   delivery_dlq                     — resolved/abandoned DLQ rows (DLQRetentionDays)
//   transformation_quality_scores    — quality scoring history (MetricsRetentionDays)
//   interface_transformation_metrics — aggregated metrics (MetricsRetentionDays)
//   transformation_executions        — pipeline execution log (HistoryRetentionDays)
//   transformation_step_executions   — step-level execution log (HistoryRetentionDays)
//
// Settings are re-read from AppSettingsCache on every run so changes made in
// the admin UI take effect on the next scheduled run without a restart.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/services/storage"
)

// RetentionEnforcementService enforces per-table data retention policies.
type RetentionEnforcementService struct {
	db         *sql.DB
	interval   time.Duration
	objStorage *storage.ObjectStorageService // optional; nil = DB-only retention (no object storage cleanup)
}

// NewRetentionEnforcementService creates a service that runs every interval.
// Pass 0 to use the default (1 hour). objStorage may be nil — when set, object
// storage artifacts (raw/parsed/transformed/logs/mapping_log) for purged messages
// are deleted alongside the DB row so they don't accumulate indefinitely.
func NewRetentionEnforcementService(db *sql.DB, interval time.Duration, objStorage *storage.ObjectStorageService) *RetentionEnforcementService {
	if interval <= 0 {
		interval = time.Hour
	}
	return &RetentionEnforcementService{db: db, interval: interval, objStorage: objStorage}
}

// Start launches the enforcement loop in a background goroutine. The loop
// runs once immediately on startup to clear any backlog, then on the ticker.
func (r *RetentionEnforcementService) Start(ctx context.Context) {
	go r.run(ctx)
}

func (r *RetentionEnforcementService) run(ctx context.Context) {
	r.enforce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.enforce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// enforce runs all retention policies once. Settings are read fresh each call.
func (r *RetentionEnforcementService) enforce(ctx context.Context) {
	settings := GetAppSettings()
	msgSettings := settings.GetMessageQueueSettings()
	alertSettings := settings.GetAlertDefaultSettings()
	cdaDedupeSettings := settings.GetCDADedupeSettings()

	r.enforceMessageTables(ctx, msgSettings.MessageRetentionDays)
	r.enforceDLQ(ctx, msgSettings.DLQRetentionDays)
	r.enforceQualityScores(ctx, alertSettings.MetricsRetentionDays)
	r.enforceMetrics(ctx, alertSettings.MetricsRetentionDays)
	r.enforceTransformHistory(ctx, alertSettings.HistoryRetentionDays)
	r.enforceCDADedupeRegistry(ctx, cdaDedupeSettings.RegistryRetentionDays)
}

// enforceMessageTables purges rows older than retentionDays from every
// messages_intf_* table registered in interface_table_metadata, deleting any
// associated object storage artifacts first (best-effort).
func (r *RetentionEnforcementService) enforceMessageTables(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	rows, err := r.db.QueryContext(ctx, `SELECT interface_id, table_name FROM interface_table_metadata`)
	if err != nil {
		log.Printf("⚠️  [Retention] Failed to list interface tables: %v", err)
		return
	}
	defer rows.Close()

	cutoff := fmt.Sprintf("%d days", retentionDays)
	type tableInfo struct {
		interfaceID string
		tableName   string
	}
	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if rows.Scan(&t.interfaceID, &t.tableName) == nil {
			tables = append(tables, t)
		}
	}

	total := int64(0)
	for _, t := range tables {
		// Table name comes from our own metadata table — safe to interpolate.
		// The table name format is always messages_intf_<uuid> (enforced by InterfaceTableManager).
		safe := sanitizeTableName(t.tableName)
		if safe == "" {
			continue
		}

		if r.objStorage != nil {
			r.deleteExpiredObjectStorage(ctx, t.interfaceID, safe, cutoff)
		}

		q := fmt.Sprintf(`DELETE FROM %s WHERE received_at < NOW() - $1::INTERVAL`, safe)
		res, err := r.db.ExecContext(ctx, q, cutoff)
		if err != nil {
			log.Printf("⚠️  [Retention] Error purging %s: %v", t.tableName, err)
			continue
		}
		n, _ := res.RowsAffected()
		total += n
	}

	if total > 0 {
		log.Printf("🗑️  [Retention] Purged %d messages older than %d days from %d interface tables",
			total, retentionDays, len(tables))
	}
}

// deleteExpiredObjectStorage removes object storage artifacts (raw, parsed,
// transformed, outbound, logs, mapping_log) for every row about to be purged
// from tableName. Runs before the DB DELETE so message_id/received_at are still
// available to reconstruct the date-partitioned storage keys. Best-effort —
// failures are logged, not fatal, since the DB row purge proceeds regardless.
func (r *RetentionEnforcementService) deleteExpiredObjectStorage(ctx context.Context, interfaceID, tableName, cutoff string) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT message_id, received_at FROM %s WHERE received_at < NOW() - $1::INTERVAL`, tableName),
		cutoff)
	if err != nil {
		log.Printf("⚠️  [Retention] Failed to list expired rows in %s for object storage cleanup: %v", tableName, err)
		return
	}
	defer rows.Close()

	type expiredRow struct {
		messageID  string
		receivedAt time.Time
	}
	var expired []expiredRow
	for rows.Next() {
		var row expiredRow
		if rows.Scan(&row.messageID, &row.receivedAt) == nil {
			expired = append(expired, row)
		}
	}

	count := 0
	for _, row := range expired {
		if err := r.objStorage.DeleteMessageObjects(ctx, interfaceID, row.messageID, row.receivedAt); err != nil {
			log.Printf("⚠️  [Retention] Failed to delete object storage for message %s: %v", row.messageID, err)
			continue
		}
		count++
	}
	if count > 0 {
		log.Printf("🗑️  [Retention] Deleted object storage artifacts for %d expired messages in %s", count, tableName)
	}
}

// enforceDLQ purges resolved and abandoned DLQ rows older than retentionDays.
// Pending/retrying rows are never purged — they may still be needed for redrive.
func (r *RetentionEnforcementService) enforceDLQ(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	res, err := r.db.ExecContext(ctx, `
		DELETE FROM delivery_dlq
		WHERE status IN ('resolved', 'abandoned')
		  AND created_at < NOW() - ($1 || ' days')::INTERVAL`,
		retentionDays)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging DLQ: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("🗑️  [Retention] Purged %d resolved/abandoned DLQ rows (>%d days old)", n, retentionDays)
	}
}

// enforceQualityScores purges reviewed quality score records older than retentionDays.
// Unreviewed flagged records are never purged — they require team action first.
func (r *RetentionEnforcementService) enforceQualityScores(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	res, err := r.db.ExecContext(ctx, `
		DELETE FROM transformation_quality_scores
		WHERE (reviewed = true OR flagged = false)
		  AND created_at < NOW() - ($1 || ' days')::INTERVAL`,
		retentionDays)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging quality scores: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("🗑️  [Retention] Purged %d quality score records (>%d days old)", n, retentionDays)
	}
}

// enforceMetrics purges aggregated metrics rows older than retentionDays.
func (r *RetentionEnforcementService) enforceMetrics(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	res, err := r.db.ExecContext(ctx, `
		DELETE FROM interface_transformation_metrics
		WHERE created_at < NOW() - ($1 || ' days')::INTERVAL`,
		retentionDays)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging metrics: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("🗑️  [Retention] Purged %d metrics rows (>%d days old)", n, retentionDays)
	}
}

// enforceTransformHistory purges pipeline execution history older than retentionDays.
// Step executions are deleted first to satisfy the foreign key constraint.
func (r *RetentionEnforcementService) enforceTransformHistory(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	cutoff := fmt.Sprintf("%d days", retentionDays)

	// Step executions reference transformation_executions — delete children first.
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM transformation_step_executions
		WHERE execution_id IN (
			SELECT id FROM transformation_executions
			WHERE created_at < NOW() - $1::INTERVAL
		)`, cutoff)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging step executions: %v", err)
	}

	res, err := r.db.ExecContext(ctx, `
		DELETE FROM transformation_executions
		WHERE created_at < NOW() - $1::INTERVAL`, cutoff)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging transform history: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("🗑️  [Retention] Purged %d pipeline execution records (>%d days old)", n, retentionDays)
	}
}

// enforceCDADedupeRegistry purges cda_dedupe_registry rows whose last_seen_at
// (NOT first_seen_at — an actively-recurring chronic fact must never be
// purged just because it was first registered long ago) has aged past
// retentionDays. Unlike the other enforceX methods above, this table holds
// PHI (a patient identifier plus clinical codes/dates), so — beyond the usual
// log.Printf — every run also writes ONE summary row to audit_logs
// (CDA_DEDUPE_REGISTRY_RETENTION_PURGED), satisfying HIPAA §164.310(d)(2)(i)'s
// disposal-tracking requirement for ePHI. One row per RUN, not per deleted
// row — logging per-row would reintroduce the same unbounded-growth problem
// this purge exists to solve, just in a second table.
func (r *RetentionEnforcementService) enforceCDADedupeRegistry(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 2555 // ~7 years — mirrors CDADedupeSettings' own default
	}

	res, err := r.db.ExecContext(ctx, `
		DELETE FROM cda_dedupe_registry
		WHERE last_seen_at < NOW() - ($1 || ' days')::INTERVAL`, retentionDays)
	if err != nil {
		log.Printf("⚠️  [Retention] Error purging cda_dedupe_registry: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return
	}
	log.Printf("🗑️  [Retention] Purged %d cda_dedupe_registry rows (>%d days since last seen)", n, retentionDays)

	metadata, _ := json.Marshal(map[string]interface{}{
		"retention_days": retentionDays,
		"rows_purged":    n,
	})
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, action, entity_type, metadata, result, risk_level, created_at)
		VALUES (gen_random_uuid(), 'CDA_DEDUPE_REGISTRY_RETENTION_PURGED', 'cda_dedupe_registry', $1::jsonb, 'success', 'low', NOW())
	`, string(metadata)); err != nil {
		log.Printf("⚠️  [Retention] Failed to write audit log for cda_dedupe_registry purge: %v", err)
	}
}

// sanitizeTableName ensures the table name only contains alphanumerics, underscores,
// and hyphens — preventing SQL injection from the metadata table.
// Valid format: messages_intf_<uuid> — letters, digits, underscores, hyphens only.
func sanitizeTableName(name string) string {
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			log.Printf("⚠️  [Retention] Rejecting unsafe table name: %q", name)
			return ""
		}
	}
	return name
}
