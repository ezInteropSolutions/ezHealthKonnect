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
	"fmt"
	"log"
	"time"
)

// RetentionEnforcementService enforces per-table data retention policies.
type RetentionEnforcementService struct {
	db       *sql.DB
	interval time.Duration
}

// NewRetentionEnforcementService creates a service that runs every interval.
// Pass 0 to use the default (1 hour).
func NewRetentionEnforcementService(db *sql.DB, interval time.Duration) *RetentionEnforcementService {
	if interval <= 0 {
		interval = time.Hour
	}
	return &RetentionEnforcementService{db: db, interval: interval}
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

	r.enforceMessageTables(ctx, msgSettings.MessageRetentionDays)
	r.enforceDLQ(ctx, msgSettings.DLQRetentionDays)
	r.enforceQualityScores(ctx, alertSettings.MetricsRetentionDays)
	r.enforceMetrics(ctx, alertSettings.MetricsRetentionDays)
	r.enforceTransformHistory(ctx, alertSettings.HistoryRetentionDays)
}

// enforceMessageTables purges rows older than retentionDays from every
// messages_intf_* table registered in interface_table_metadata.
func (r *RetentionEnforcementService) enforceMessageTables(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	rows, err := r.db.QueryContext(ctx, `SELECT table_name FROM interface_table_metadata`)
	if err != nil {
		log.Printf("⚠️  [Retention] Failed to list interface tables: %v", err)
		return
	}
	defer rows.Close()

	cutoff := fmt.Sprintf("%d days", retentionDays)
	var tables []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			tables = append(tables, t)
		}
	}

	total := int64(0)
	for _, table := range tables {
		// Table name comes from our own metadata table — safe to interpolate.
		// The table name format is always messages_intf_<uuid> (enforced by InterfaceTableManager).
		q := fmt.Sprintf(
			`DELETE FROM %s WHERE received_at < NOW() - $1::INTERVAL`,
			sanitizeTableName(table),
		)
		res, err := r.db.ExecContext(ctx, q, cutoff)
		if err != nil {
			log.Printf("⚠️  [Retention] Error purging %s: %v", table, err)
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
