// services/metrics_collector.go
//
// MetricsCollector aggregates transformation and delivery metrics into
// interface_transformation_metrics every 5 minutes. It reads from
// transformation_quality_scores and delivery_dlq and writes a single summary
// row per interface per period.
//
// After each collection it checks interface_alert_thresholds and dispatches
// email / webhook notifications via AlertNotificationService when thresholds
// are breached. Cooldown columns (last_flag_alert_at, last_dlq_alert_at,
// cooldown_minutes) prevent alert flooding.
//
// Start it once at application startup:
//
//	mc := services.NewMetricsCollector(db, notificationSvc)
//	mc.Start(ctx)
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/services/metrics"
)

// MetricsCollector collects per-interface transformation metrics on a schedule.
type MetricsCollector struct {
	db              *sql.DB
	interval        time.Duration
	notificationSvc *AlertNotificationService // nil = log-only mode
}

// NewMetricsCollector creates a collector with the default 5-minute interval.
// Pass an AlertNotificationService to enable email/webhook dispatch; pass nil
// to run in log-only mode (useful in development / when SMTP is not configured).
func NewMetricsCollector(db *sql.DB, notificationSvc *AlertNotificationService) *MetricsCollector {
	return &MetricsCollector{
		db:              db,
		interval:        5 * time.Minute,
		notificationSvc: notificationSvc,
	}
}

// Start launches the collection goroutine. Stops when ctx is cancelled.
func (mc *MetricsCollector) Start(ctx context.Context) {
	go func() {
		// Collect once immediately on start, then on interval
		mc.collect(ctx)
		ticker := time.NewTicker(mc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mc.collect(ctx)
			}
		}
	}()
	log.Printf("[metrics] collector started (interval=%s)", mc.interval)
}

// collect runs a single aggregation pass covering the last 5 minutes.
func (mc *MetricsCollector) collect(ctx context.Context) {
	periodEnd := time.Now().UTC()
	periodStart := periodEnd.Add(-mc.interval)

	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			interface_id::text,
			NULL::text                                        AS message_type,
			COUNT(*)                                          AS messages_scored,
			COUNT(*) FILTER (WHERE flagged = true)            AS messages_flagged,
			ROUND(AVG(score)::NUMERIC, 2)                     AS avg_quality_score,
			ROUND(MIN(score)::NUMERIC, 2)                     AS min_quality_score
		FROM transformation_quality_scores
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY interface_id

		UNION ALL

		SELECT
			interface_id::text,
			message_type                                      AS message_type,
			COUNT(*)                                          AS messages_scored,
			COUNT(*) FILTER (WHERE flagged = true)            AS messages_flagged,
			ROUND(AVG(score)::NUMERIC, 2)                     AS avg_quality_score,
			ROUND(MIN(score)::NUMERIC, 2)                     AS min_quality_score
		FROM transformation_quality_scores
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY interface_id, message_type`,
		periodStart, periodEnd,
	)
	if err != nil {
		log.Printf("[metrics] collect query error: %v", err)
		return
	}
	defer rows.Close()

	type aggRow struct {
		interfaceID    string
		messageType    sql.NullString
		messagesScored int
		messagesFlagged int
		avgScore       sql.NullFloat64
		minScore       sql.NullFloat64
	}

	var aggs []aggRow
	for rows.Next() {
		var a aggRow
		if err := rows.Scan(
			&a.interfaceID, &a.messageType,
			&a.messagesScored, &a.messagesFlagged,
			&a.avgScore, &a.minScore,
		); err != nil {
			log.Printf("[metrics] scan error: %v", err)
			continue
		}
		aggs = append(aggs, a)
	}

	// DLQ depth snapshots per interface
	dlqDepths := mc.dlqDepths(ctx)

	// Prometheus: total unresolved DLQ depth across all interfaces.
	if metrics.DLQDepth != nil {
		total := 0
		for _, d := range dlqDepths {
			total += d
		}
		metrics.DLQDepth.Set(float64(total))
	}

	inserted := 0
	for _, a := range aggs {
		dlqDepth := dlqDepths[a.interfaceID]
		_, err := mc.db.ExecContext(ctx, `
			INSERT INTO interface_transformation_metrics
				(interface_id, message_type, period_start, period_end,
				 messages_scored, messages_flagged,
				 avg_quality_score, min_quality_score, dlq_depth)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			a.interfaceID, a.messageType,
			periodStart, periodEnd,
			a.messagesScored, a.messagesFlagged,
			a.avgScore, a.minScore,
			dlqDepth,
		)
		if err != nil {
			log.Printf("[metrics] insert error for interface %s: %v", a.interfaceID, err)
			continue
		}
		inserted++
	}

	if inserted > 0 {
		log.Printf("[metrics] wrote %d metric row(s) for period %s–%s",
			inserted,
			periodStart.Format("15:04:05"),
			periodEnd.Format("15:04:05"),
		)
	}

	// Check alert thresholds after each collection
	mc.checkAlerts(ctx, periodStart, periodEnd)
}

// dlqDepths returns current pending DLQ count per interface.
func (mc *MetricsCollector) dlqDepths(ctx context.Context) map[string]int {
	depths := map[string]int{}
	rows, err := mc.db.QueryContext(ctx, `
		SELECT interface_id::text, COUNT(*)
		FROM delivery_dlq
		WHERE status IN ('pending','retrying')
		GROUP BY interface_id`)
	if err != nil {
		return depths
	}
	defer rows.Close()
	for rows.Next() {
		var ifaceID string
		var count int
		if err := rows.Scan(&ifaceID, &count); err == nil {
			depths[ifaceID] = count
		}
	}
	return depths
}

// checkAlerts checks interface_alert_thresholds after each collection period.
// When a threshold is breached and the cooldown has elapsed, it dispatches
// a notification via email and/or webhook and updates the last-alerted timestamp.
func (mc *MetricsCollector) checkAlerts(ctx context.Context, periodStart, _ time.Time) {
	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			m.interface_id::text,
			COALESCE(i.name, m.interface_id::text)  AS interface_name,
			m.messages_scored,
			m.messages_flagged,
			m.dlq_depth,
			t.max_flag_rate_pct,
			t.max_dlq_depth,
			COALESCE(t.cooldown_minutes, 30)         AS cooldown_minutes,
			t.alert_email,
			t.alert_webhook_url,
			t.last_flag_alert_at,
			t.last_dlq_alert_at
		FROM interface_transformation_metrics m
		JOIN interface_alert_thresholds t ON t.interface_id = m.interface_id
		LEFT JOIN interfaces i             ON i.id = m.interface_id::uuid
		WHERE m.period_start = $1
		  AND m.message_type IS NULL
		  AND t.enabled = true
		  AND m.messages_scored > 0`,
		periodStart,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ifaceID, ifaceName string
		var scored, flagged, dlqDepth int
		var maxFlagPct float64
		var maxDLQ, cooldownMins int
		var alertEmail, alertWebhook sql.NullString
		var lastFlagAlert, lastDLQAlert sql.NullTime

		if err := rows.Scan(
			&ifaceID, &ifaceName,
			&scored, &flagged, &dlqDepth,
			&maxFlagPct, &maxDLQ, &cooldownMins,
			&alertEmail, &alertWebhook,
			&lastFlagAlert, &lastDLQAlert,
		); err != nil {
			continue
		}

		now := time.Now().UTC()
		cooldown := time.Duration(cooldownMins) * time.Minute

		flagRate := float64(flagged) / float64(scored) * 100.0
		if flagRate > maxFlagPct && mc.cooldownElapsed(lastFlagAlert, cooldown, now) {
			msg := fmt.Sprintf("Interface %q: flag rate %.1f%% exceeds threshold %.1f%% (period %s)",
				ifaceName, flagRate, maxFlagPct, periodStart.Format("2006-01-02 15:04 UTC"))
			log.Printf("⚠️  [metrics] ALERT flag_rate interface=%s %s", ifaceID, msg)
			mc.dispatch(ctx, ifaceID, ifaceName, "flag_rate_exceeded", "warning", msg, alertEmail, alertWebhook)
			mc.db.ExecContext(ctx, `UPDATE interface_alert_thresholds SET last_flag_alert_at = NOW() WHERE interface_id = $1`, ifaceID)
		}

		if dlqDepth > maxDLQ && mc.cooldownElapsed(lastDLQAlert, cooldown, now) {
			msg := fmt.Sprintf("Interface %q: DLQ depth %d exceeds threshold %d",
				ifaceName, dlqDepth, maxDLQ)
			log.Printf("⚠️  [metrics] ALERT dlq_depth interface=%s %s", ifaceID, msg)
			mc.dispatch(ctx, ifaceID, ifaceName, "dlq_depth_exceeded", "critical", msg, alertEmail, alertWebhook)
			mc.db.ExecContext(ctx, `UPDATE interface_alert_thresholds SET last_dlq_alert_at = NOW() WHERE interface_id = $1`, ifaceID)
		}
	}
}

// cooldownElapsed returns true when enough time has passed since the last alert
// (or when no alert has ever been sent for this threshold).
func (mc *MetricsCollector) cooldownElapsed(last sql.NullTime, cooldown time.Duration, now time.Time) bool {
	if !last.Valid {
		return true
	}
	return now.Sub(last.Time) >= cooldown
}

// dispatch sends a notification to every configured channel for the interface.
// Silently skips channels that are not configured. Logs dispatch errors but does
// not treat them as fatal — a failed notification must not interrupt metrics collection.
func (mc *MetricsCollector) dispatch(
	ctx context.Context,
	ifaceID, ifaceName, alertType, severity, message string,
	alertEmail, alertWebhook sql.NullString,
) {
	if mc.notificationSvc == nil {
		return
	}

	alert := &FiredAlert{
		InterfaceID:   &ifaceID,
		InterfaceName: ifaceName,
		AlertType:     alertType,
		Severity:      severity,
		Message:       message,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if alertEmail.Valid && alertEmail.String != "" {
		cfg, _ := json.Marshal(map[string]string{"to": alertEmail.String})
		ch := &AlertChannel{ChannelType: "email", Config: cfg, IsEnabled: true}
		if err := mc.notificationSvc.DispatchToChannel(ctx, ch, alert); err != nil {
			log.Printf("⚠️  [metrics] email dispatch failed for interface %s: %v", ifaceID, err)
		}
	}

	if alertWebhook.Valid && alertWebhook.String != "" {
		cfg, _ := json.Marshal(map[string]string{"url": alertWebhook.String})
		ch := &AlertChannel{ChannelType: "webhook", Config: cfg, IsEnabled: true}
		if err := mc.notificationSvc.DispatchToChannel(ctx, ch, alert); err != nil {
			log.Printf("⚠️  [metrics] webhook dispatch failed for interface %s: %v", ifaceID, err)
		}
	}
}
