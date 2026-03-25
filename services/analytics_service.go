package services

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Cache ────────────────────────────────────────────────────────────────────

type analyticsCacheEntry struct {
	data      interface{}
	expiresAt time.Time
}

// AnalyticsCache is a simple in-memory TTL cache for analytics query results.
type AnalyticsCache struct {
	mu      sync.RWMutex
	entries map[string]analyticsCacheEntry
}

func newAnalyticsCache() *AnalyticsCache {
	c := &AnalyticsCache{entries: make(map[string]analyticsCacheEntry)}
	go c.evictLoop()
	return c
}

func (c *AnalyticsCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func (c *AnalyticsCache) Set(key string, data interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = analyticsCacheEntry{data: data, expiresAt: time.Now().Add(ttl)}
}

func (c *AnalyticsCache) evictLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// ─── TTL constants ────────────────────────────────────────────────────────────

const (
	cacheTTLMonitoringSummary = 30 * time.Second
	cacheTTLLiveFeed          = 5 * time.Second
	cacheTTLInterfaceCards    = 30 * time.Second
	cacheTTLConnectorHealth   = 60 * time.Second
	cacheTTLAlerts            = 15 * time.Second
	cacheTTLTableList         = 5 * time.Minute
	cacheTTLReportDefinitions = 2 * time.Minute
)

// ─── Models ───────────────────────────────────────────────────────────────────

type KPIData struct {
	MessagesToday       int64   `json:"messages_today"`
	ErrorRate24h        float64 `json:"error_rate_24h"`
	AvgProcessingTimeMs float64 `json:"avg_processing_time_ms"`
	ActiveInterfaces    int     `json:"active_interfaces"`
	TotalInterfaces     int     `json:"total_interfaces"`
	DLQDepth            int     `json:"dlq_depth"`
}

type KPITrends struct {
	MessagesTodaySparkline []int64   `json:"messages_today_sparkline"`
	ErrorRateSparkline     []float64 `json:"error_rate_sparkline"`
}

type AlertCountBySeverity struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

type MonitoringSummary struct {
	KPIs               KPIData              `json:"kpis"`
	KPITrends          KPITrends            `json:"kpi_trends"`
	AlertCountBySeverity AlertCountBySeverity `json:"alert_count_by_severity"`
}

type LiveMessage struct {
	MessageID        string  `json:"message_id"`
	InterfaceID      string  `json:"interface_id"`
	InterfaceName    string  `json:"interface_name"`
	MessageType      string  `json:"message_type"`
	Status           string  `json:"status"`
	ReceivedAt       string  `json:"received_at"`
	ProcessingTimeMs *int64  `json:"processing_time_ms"`
}

type InterfaceHealthCard struct {
	InterfaceID      string    `json:"interface_id"`
	InterfaceName    string    `json:"interface_name"`
	ConnectorType    string    `json:"connector_type"`
	Status           string    `json:"status"`
	MessagesToday    int64     `json:"messages_today"`
	ErrorRateToday   float64   `json:"error_rate_today"`
	SparklineData    []int64   `json:"sparkline_data"`
	LastActivityAt   *string   `json:"last_activity_at"`
}

type ConnectorHealthRow struct {
	InterfaceID      string  `json:"interface_id"`
	InterfaceName    string  `json:"interface_name"`
	ConnectorType    string  `json:"connector_type"`
	Status           string  `json:"status"`
	LastRunAt        *string `json:"last_run_at"`
	LastRunStatus    string  `json:"last_run_status"`
	MessagesLastRun  int     `json:"messages_last_run"`
}

type MonitoringAlert struct {
	ID              string  `json:"id"`
	InterfaceID     *string `json:"interface_id"`
	InterfaceName   string  `json:"interface_name"`
	AlertType       string  `json:"alert_type"`
	Severity        string  `json:"severity"`
	Message         string  `json:"message"`
	Acknowledged    bool    `json:"acknowledged"`
	CreatedAt       string  `json:"created_at"`
	ElapsedSeconds  int64   `json:"elapsed_seconds"`
}

// ReportExecutionRequest mirrors the self-service builder config.
type ReportExecutionRequest struct {
	DataSource    string                 `json:"data_source"`
	Metrics       []string               `json:"metrics"`
	Dimensions    string                 `json:"dimensions"`
	Filters       map[string]interface{} `json:"filters"`
	Visualization string                 `json:"visualization"`
	VizOptions    map[string]interface{} `json:"viz_options"`
	Limit         int                    `json:"limit"`
}

type ReportResultRow map[string]interface{}

type ReportResult struct {
	Columns     []string          `json:"columns"`
	Rows        []ReportResultRow `json:"rows"`
	TotalRows   int               `json:"total_rows"`
	QueryTimeMs int64             `json:"query_time_ms"`
	CacheHit    bool              `json:"cache_hit"`
}

type ReportDefinition struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Slug              string                 `json:"slug"`
	Description       string                 `json:"description"`
	Category          string                 `json:"category"`
	DataSource        string                 `json:"data_source"`
	Metrics           []string               `json:"metrics"`
	Dimensions        string                 `json:"dimensions"`
	Filters           map[string]interface{} `json:"filters"`
	Visualization     string                 `json:"visualization"`
	VizOptions        map[string]interface{} `json:"viz_options"`
	IsSystem          bool                   `json:"is_system"`
	IsPublic          bool                   `json:"is_public"`
	CreatedByUserID   *string                `json:"created_by_user_id"`
	LastRunAt         *string                `json:"last_run_at"`
	RunCount          int                    `json:"run_count"`
	AvgRunTimeMs      *int                   `json:"avg_run_time_ms"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// AnalyticsService provides monitoring and reporting query operations.
type AnalyticsService struct {
	db           *sql.DB
	cache        *AnalyticsCache
	alertRuleSvc *AlertRuleService
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(db *sql.DB) *AnalyticsService {
	return &AnalyticsService{db: db, cache: newAnalyticsCache()}
}

// SetAlertRuleService injects the AlertRuleService so checkAndWriteAlerts can use
// dynamic DB-driven rules instead of hardcoded thresholds.
func (s *AnalyticsService) SetAlertRuleService(svc *AlertRuleService) {
	s.alertRuleSvc = svc
}

// ─── Monitoring ───────────────────────────────────────────────────────────────

// GetMonitoringSummary returns headline KPIs, sparkline trends, and alert counts.
func (s *AnalyticsService) GetMonitoringSummary(ctx context.Context) (*MonitoringSummary, error) {
	const cacheKey = "monitoring:summary"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if ms, ok := cached.(*MonitoringSummary); ok {
			return ms, nil
		}
	}

	summary := &MonitoringSummary{}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	last24h := now.Add(-24 * time.Hour)

	// Messages today — UNION across all interface tables
	tables, err := s.listMessageInterfaceTables(ctx)
	if err == nil && len(tables) > 0 {
		q := s.buildUnionCountQuery(tables, "received_at >= $1")
		row := s.db.QueryRowContext(ctx, q, todayStart)
		_ = row.Scan(&summary.KPIs.MessagesToday)
	}

	// Error rate over last 24h
	var totalExec, failedExec int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'failed')
		 FROM transformation_executions WHERE started_at >= $1`, last24h,
	).Scan(&totalExec, &failedExec)
	if totalExec > 0 {
		summary.KPIs.ErrorRate24h = float64(failedExec) / float64(totalExec) * 100
	}

	// Avg processing time last hour
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(AVG(total_time_ms), 0) FROM transformation_executions
		 WHERE started_at >= $1 AND status = 'completed'`, now.Add(-time.Hour),
	).Scan(&summary.KPIs.AvgProcessingTimeMs)

	// Active / total interfaces
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE status = 'active'), COUNT(*)
		 FROM interfaces`,
	).Scan(&summary.KPIs.ActiveInterfaces, &summary.KPIs.TotalInterfaces)

	// DLQ depth
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dead_letter_queue WHERE resolved = FALSE`,
	).Scan(&summary.KPIs.DLQDepth)

	// 7-hour sparkline for messages
	sparkline := make([]int64, 7)
	if len(tables) > 0 {
		for i := 6; i >= 0; i-- {
			hourStart := now.Add(-time.Duration(i+1) * time.Hour)
			hourEnd := now.Add(-time.Duration(i) * time.Hour)
			q := s.buildUnionCountQuery(tables, "received_at >= $1 AND received_at < $2")
			var cnt int64
			_ = s.db.QueryRowContext(ctx, q, hourStart, hourEnd).Scan(&cnt)
			sparkline[6-i] = cnt
		}
	}
	summary.KPITrends.MessagesTodaySparkline = sparkline

	// 7-hour error rate sparkline
	errSparkline := make([]float64, 7)
	for i := 6; i >= 0; i-- {
		hourStart := now.Add(-time.Duration(i+1) * time.Hour)
		hourEnd := now.Add(-time.Duration(i) * time.Hour)
		var tot, fail int64
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*), COUNT(*) FILTER (WHERE status='failed')
			 FROM transformation_executions WHERE started_at >= $1 AND started_at < $2`,
			hourStart, hourEnd,
		).Scan(&tot, &fail)
		if tot > 0 {
			errSparkline[6-i] = float64(fail) / float64(tot) * 100
		}
	}
	summary.KPITrends.ErrorRateSparkline = errSparkline

	// Alert counts
	rows, err := s.db.QueryContext(ctx,
		`SELECT severity, COUNT(*) FROM monitoring_alerts
		 WHERE acknowledged = FALSE GROUP BY severity`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sev string
			var cnt int
			if rows.Scan(&sev, &cnt) == nil {
				switch sev {
				case "critical":
					summary.AlertCountBySeverity.Critical = cnt
				case "warning":
					summary.AlertCountBySeverity.Warning = cnt
				case "info":
					summary.AlertCountBySeverity.Info = cnt
				}
			}
		}
	}

	// Threshold-based alert generation
	_ = s.checkAndWriteAlerts(ctx, summary)

	s.cache.Set(cacheKey, summary, cacheTTLMonitoringSummary)
	return summary, nil
}

// GetLiveMessages returns the most recent messages across all interface tables.
func (s *AnalyticsService) GetLiveMessages(ctx context.Context, limit int) ([]*LiveMessage, error) {
	const cacheKey = "monitoring:live_messages"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if msgs, ok := cached.([]*LiveMessage); ok {
			return msgs, nil
		}
	}

	// Fall back to information_schema if interface_table_metadata is empty —
	// this handles cases where tables exist but metadata wasn't populated.
	tables, err := s.listMessageInterfaceTables(ctx)
	if err != nil || len(tables) == 0 {
		rows, qErr := s.db.QueryContext(ctx,
			`SELECT table_name FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name LIKE 'messages_intf_%'
			 ORDER BY table_name`)
		if qErr != nil {
			return []*LiveMessage{}, nil
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil && safeTableNameRegex.MatchString(name) {
				tables = append(tables, name)
			}
		}
		if len(tables) == 0 {
			return []*LiveMessage{}, nil
		}
	}

	// Build UNION: each sub-query gets top 5 per table, then take the global top {limit}.
	// Table names use underscores (messages_intf_uuid_with_underscores); restore dashes
	// so the LEFT JOIN on interfaces.id (UUID with dashes) works correctly.
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		raw := strings.TrimPrefix(t, "messages_intf_")
		// UUIDs are 32 hex chars + 4 dashes = 36 chars; table uses underscores in place of dashes.
		// Replace underscores back to dashes at the known UUID positions (8-4-4-4-12).
		ifaceID := rawTableSuffixToUUID(raw)
		parts = append(parts, fmt.Sprintf(
			`(SELECT message_id, '%s' AS interface_id, message_type,
			         status, received_at, processing_time_ms
			  FROM %s ORDER BY received_at DESC LIMIT 5)`,
			ifaceID, t,
		))
	}
	union := strings.Join(parts, " UNION ALL ")
	q := fmt.Sprintf(
		`SELECT u.message_id, u.interface_id, COALESCE(i.name,'Unknown') AS interface_name,
		        u.message_type, u.status, u.received_at, u.processing_time_ms
		 FROM (%s) u
		 LEFT JOIN interfaces i ON i.id::text = u.interface_id
		 ORDER BY received_at DESC LIMIT %d`, union, limit,
	)

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return []*LiveMessage{}, nil
	}
	defer rows.Close()

	var msgs []*LiveMessage
	for rows.Next() {
		m := &LiveMessage{}
		var recvAt time.Time
		if err := rows.Scan(&m.MessageID, &m.InterfaceID, &m.InterfaceName,
			&m.MessageType, &m.Status, &recvAt, &m.ProcessingTimeMs); err == nil {
			m.ReceivedAt = recvAt.Format(time.RFC3339)
			msgs = append(msgs, m)
		}
	}
	if msgs == nil {
		msgs = []*LiveMessage{}
	}

	s.cache.Set(cacheKey, msgs, cacheTTLLiveFeed)
	return msgs, nil
}

// GetInterfaceHealthCards returns per-interface monitoring mini-card data.
func (s *AnalyticsService) GetInterfaceHealthCards(ctx context.Context) ([]*InterfaceHealthCard, error) {
	const cacheKey = "monitoring:interface_cards_v2"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if cards, ok := cached.([]*InterfaceHealthCard); ok {
			return cards, nil
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, interface_status FROM interfaces
		 WHERE interface_status = 'active'
		   AND is_active = TRUE
		   AND deleted_at IS NULL
		 ORDER BY name`)
	if err != nil {
		return []*InterfaceHealthCard{}, nil
	}
	defer rows.Close()

	var cards []*InterfaceHealthCard
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for rows.Next() {
		card := &InterfaceHealthCard{SparklineData: make([]int64, 7)}
		if err := rows.Scan(&card.InterfaceID, &card.InterfaceName, &card.Status); err != nil {
			continue
		}

		tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(card.InterfaceID, "-", "_"))
		if s.tableExists(ctx, tableName) {
			// Messages today
			_ = s.db.QueryRowContext(ctx,
				fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE received_at >= $1`, tableName),
				todayStart,
			).Scan(&card.MessagesToday)

			// Error rate today
			var tot, fail int64
			_ = s.db.QueryRowContext(ctx,
				fmt.Sprintf(`SELECT COUNT(*), COUNT(*) FILTER (WHERE status='failed')
				             FROM %s WHERE received_at >= $1`, tableName),
				todayStart,
			).Scan(&tot, &fail)
			if tot > 0 {
				card.ErrorRateToday = float64(fail) / float64(tot) * 100
			}

			// 7-point hourly sparkline
			for i := 6; i >= 0; i-- {
				hStart := now.Add(-time.Duration(i+1) * time.Hour)
				hEnd := now.Add(-time.Duration(i) * time.Hour)
				var cnt int64
				_ = s.db.QueryRowContext(ctx,
					fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE received_at >= $1 AND received_at < $2`, tableName),
					hStart, hEnd,
				).Scan(&cnt)
				card.SparklineData[6-i] = cnt
			}

			// Last activity
			var lastAt time.Time
			if err := s.db.QueryRowContext(ctx,
				fmt.Sprintf(`SELECT received_at FROM %s ORDER BY received_at DESC LIMIT 1`, tableName),
			).Scan(&lastAt); err == nil {
				ts := lastAt.Format(time.RFC3339)
				card.LastActivityAt = &ts
			}
		}

		// Connector type
		var connType string
		_ = s.db.QueryRowContext(ctx,
			`SELECT COALESCE(ct.type_name, 'unknown')
			 FROM interface_connectivity ic
			 JOIN connectivity_types ct ON ct.id = ic.inbound_connector_type_id
			 WHERE ic.interface_id = $1 LIMIT 1`,
			card.InterfaceID,
		).Scan(&connType)
		card.ConnectorType = connType

		cards = append(cards, card)
	}
	if cards == nil {
		cards = []*InterfaceHealthCard{}
	}

	// Sort: highest error rate first (unhealthy → warning → ok), then by name
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ErrorRateToday != cards[j].ErrorRateToday {
			return cards[i].ErrorRateToday > cards[j].ErrorRateToday
		}
		return cards[i].InterfaceName < cards[j].InterfaceName
	})

	s.cache.Set(cacheKey, cards, cacheTTLInterfaceCards)
	return cards, nil
}

// GetConnectorHealth returns interfaces whose latest connector run ended in an
// error state, or that have a connector configured but have never run.
// This is an attention-focused view — healthy connectors are omitted.
func (s *AnalyticsService) GetConnectorHealth(ctx context.Context) ([]*ConnectorHealthRow, error) {
	const cacheKey = "monitoring:connector_health"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if rows, ok := cached.([]*ConnectorHealthRow); ok {
			return rows, nil
		}
	}

	q := `
		SELECT
			i.id,
			i.name,
			COALESCE(ct.type_name, 'unknown') AS connector_type,
			i.status,
			cel.started_at,
			COALESCE(cel.status, 'no_runs') AS last_run_status,
			COALESCE(cel.messages_retrieved, 0) AS messages_last_run
		FROM interfaces i
		INNER JOIN interface_connectivity ic ON ic.interface_id = i.id
		LEFT JOIN connectivity_types ct ON ct.id = ic.inbound_connector_type_id
		LEFT JOIN LATERAL (
			SELECT started_at, status, messages_retrieved
			FROM connectivity_execution_log
			WHERE interface_id = i.id
			ORDER BY started_at DESC LIMIT 1
		) cel ON TRUE
		WHERE
			i.deleted_at IS NULL
			AND i.is_active = TRUE
			AND (
				cel.status IN ('failed', 'error', 'timeout', 'connection_refused')
				OR (cel.status IS NULL AND i.interface_status = 'active')
			)
		ORDER BY cel.started_at DESC NULLS LAST, i.name
	`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return []*ConnectorHealthRow{}, nil
	}
	defer rows.Close()

	var result []*ConnectorHealthRow
	for rows.Next() {
		r := &ConnectorHealthRow{}
		var lastRunAt sql.NullTime
		if err := rows.Scan(&r.InterfaceID, &r.InterfaceName, &r.ConnectorType,
			&r.Status, &lastRunAt, &r.LastRunStatus, &r.MessagesLastRun); err == nil {
			if lastRunAt.Valid {
				ts := lastRunAt.Time.Format(time.RFC3339)
				r.LastRunAt = &ts
			}
			result = append(result, r)
		}
	}
	if result == nil {
		result = []*ConnectorHealthRow{}
	}

	s.cache.Set(cacheKey, result, cacheTTLConnectorHealth)
	return result, nil
}

// GetActiveAlerts returns unacknowledged monitoring alerts.
func (s *AnalyticsService) GetActiveAlerts(ctx context.Context) ([]*MonitoringAlert, error) {
	const cacheKey = "monitoring:alerts"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if alerts, ok := cached.([]*MonitoringAlert); ok {
			return alerts, nil
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ma.id,
			ma.interface_id,
			COALESCE(i.name, 'System') AS interface_name,
			ma.alert_type,
			ma.severity,
			ma.message,
			ma.acknowledged,
			ma.created_at
		FROM monitoring_alerts ma
		LEFT JOIN interfaces i ON i.id = ma.interface_id
		WHERE ma.acknowledged = FALSE AND ma.auto_resolved = FALSE
		ORDER BY
			CASE ma.severity WHEN 'critical' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,
			ma.created_at DESC
		LIMIT 50
	`)
	if err != nil {
		return []*MonitoringAlert{}, nil
	}
	defer rows.Close()

	var alerts []*MonitoringAlert
	now := time.Now()
	for rows.Next() {
		a := &MonitoringAlert{}
		var createdAt time.Time
		var interfaceID sql.NullString
		if err := rows.Scan(&a.ID, &interfaceID, &a.InterfaceName,
			&a.AlertType, &a.Severity, &a.Message, &a.Acknowledged, &createdAt); err == nil {
			if interfaceID.Valid {
				a.InterfaceID = &interfaceID.String
			}
			a.CreatedAt = createdAt.Format(time.RFC3339)
			a.ElapsedSeconds = int64(now.Sub(createdAt).Seconds())
			alerts = append(alerts, a)
		}
	}
	if alerts == nil {
		alerts = []*MonitoringAlert{}
	}

	s.cache.Set(cacheKey, alerts, cacheTTLAlerts)
	return alerts, nil
}

// AcknowledgeAlert marks an alert as acknowledged.
func (s *AnalyticsService) AcknowledgeAlert(ctx context.Context, alertID string, acknowledgedBy string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE monitoring_alerts
		 SET acknowledged = TRUE, acknowledged_by = $1, acknowledged_at = NOW(), updated_at = NOW()
		 WHERE id = $2`,
		acknowledgedBy, alertID,
	)
	// Bust cache so next poll sees the change immediately
	s.cache.Set("monitoring:alerts", nil, time.Nanosecond)
	return err
}

// ─── Reports ──────────────────────────────────────────────────────────────────

// ListReportDefinitions returns all report definitions (OOB + user-saved).
func (s *AnalyticsService) ListReportDefinitions(ctx context.Context, userID string, systemOnly bool) ([]*ReportDefinition, error) {
	const cacheKey = "reports:definitions"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if defs, ok := cached.([]*ReportDefinition); ok {
			return defs, nil
		}
	}

	q := `
		SELECT id, name, COALESCE(slug,''), COALESCE(description,''),
		       category, data_source, metrics, COALESCE(dimensions,''),
		       filters, visualization, viz_options,
		       is_system, is_public,
		       created_by_user_id, last_run_at, run_count, avg_run_time_ms,
		       created_at, updated_at
		FROM analytics_report_definitions
		ORDER BY is_system DESC, category, name
	`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return []*ReportDefinition{}, nil
	}
	defer rows.Close()

	var defs []*ReportDefinition
	for rows.Next() {
		d := &ReportDefinition{}
		var metrics []string
		var filtersJSON, vizOptsJSON []byte
		var metricsArr interface{}
		var createdBy sql.NullString
		var lastRunAt sql.NullTime
		var avgRunTime sql.NullInt64
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&d.ID, &d.Name, &d.Slug, &d.Description,
			&d.Category, &d.DataSource, &metricsArr, &d.Dimensions,
			&filtersJSON, &d.Visualization, &vizOptsJSON,
			&d.IsSystem, &d.IsPublic,
			&createdBy, &lastRunAt, &d.RunCount, &avgRunTime,
			&createdAt, &updatedAt,
		); err != nil {
			continue
		}
		_ = metrics
		_ = filtersJSON
		_ = vizOptsJSON

		d.CreatedAt = createdAt.Format(time.RFC3339)
		d.UpdatedAt = updatedAt.Format(time.RFC3339)
		if createdBy.Valid {
			d.CreatedByUserID = &createdBy.String
		}
		if lastRunAt.Valid {
			ts := lastRunAt.Time.Format(time.RFC3339)
			d.LastRunAt = &ts
		}
		if avgRunTime.Valid {
			v := int(avgRunTime.Int64)
			d.AvgRunTimeMs = &v
		}
		defs = append(defs, d)
	}
	if defs == nil {
		defs = []*ReportDefinition{}
	}

	s.cache.Set(cacheKey, defs, cacheTTLReportDefinitions)
	return defs, nil
}

// ExecuteReport runs an ad-hoc report query and returns results.
func (s *AnalyticsService) ExecuteReport(ctx context.Context, req *ReportExecutionRequest) (*ReportResult, error) {
	start := time.Now()

	if req.Limit <= 0 || req.Limit > 10000 {
		req.Limit = 1000
	}

	var (
		q    string
		args []interface{}
		err  error
	)

	switch req.DataSource {
	case "message_flow":
		q, args, err = s.buildMessageFlowQuery(ctx, req)
	case "pipeline_performance":
		q, args, err = s.buildPipelinePerformanceQuery(req)
	case "hipaa_audit":
		q, args, err = s.buildHIPAAAuditQuery(req)
	case "connector_activity":
		q, args, err = s.buildConnectorActivityQuery(req)
	case "dlq_analysis":
		q, args, err = s.buildDLQAnalysisQuery(req)
	case "user_activity":
		q, args, err = s.buildUserActivityQuery(req)
	default:
		return nil, fmt.Errorf("unknown data source: %s", req.DataSource)
	}

	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var resultRows []ReportResultRow

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(ReportResultRow, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		resultRows = append(resultRows, row)
	}
	if resultRows == nil {
		resultRows = []ReportResultRow{}
	}

	return &ReportResult{
		Columns:     cols,
		Rows:        resultRows,
		TotalRows:   len(resultRows),
		QueryTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// SaveReportDefinition persists a user-created report.
func (s *AnalyticsService) SaveReportDefinition(ctx context.Context, def *ReportDefinition) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO analytics_report_definitions
		    (name, description, category, data_source, metrics, dimensions,
		     filters, visualization, viz_options, is_system, is_public, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5::text[],$6,$7,$8,$9,FALSE,FALSE,$10)`,
		def.Name, def.Description, def.Category, def.DataSource,
		fmt.Sprintf("{%s}", strings.Join(def.Metrics, ",")),
		def.Dimensions,
		"{}",
		def.Visualization,
		"{}",
		def.CreatedByUserID,
	)
	s.cache.Set("reports:definitions", nil, time.Nanosecond) // bust
	return err
}

// DeleteReportDefinition removes a user-created report (system reports are protected).
func (s *AnalyticsService) DeleteReportDefinition(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM analytics_report_definitions WHERE id = $1 AND is_system = FALSE`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("report not found or is a system report")
	}
	s.cache.Set("reports:definitions", nil, time.Nanosecond)
	return nil
}

// ─── Query Builders ───────────────────────────────────────────────────────────

func (s *AnalyticsService) buildMessageFlowQuery(ctx context.Context, req *ReportExecutionRequest) (string, []interface{}, error) {
	tables, _ := s.listMessageInterfaceTables(ctx)
	if len(tables) == 0 {
		return `SELECT 'no_data' AS message_type, 0 AS total_messages`, nil, nil
	}

	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}

	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		ifaceID := strings.TrimPrefix(t, "messages_intf_")
		ifaceIDWithDashes := strings.ReplaceAll(ifaceID, "_", "-")
		parts = append(parts, fmt.Sprintf(
			`SELECT message_id, '%s'::text AS interface_id, message_type,
			        status, received_at, COALESCE(processing_time_ms, 0) AS processing_time_ms
			 FROM %s WHERE received_at >= $1 AND received_at < $2`,
			ifaceIDWithDashes, t,
		))
	}
	union := strings.Join(parts, " UNION ALL ")

	dimension := req.Dimensions
	if dimension == "" {
		dimension = "day"
	}

	var q string
	switch dimension {
	case "interface_id":
		q = fmt.Sprintf(`
			SELECT u.interface_id,
			       COALESCE(i.name, u.interface_id) AS interface_name,
			       COUNT(*) AS total_messages,
			       COUNT(*) FILTER (WHERE u.status IN ('completed','processed')) AS processed_count,
			       COUNT(*) FILTER (WHERE u.status = 'failed') AS failed_count,
			       ROUND(COUNT(*) FILTER (WHERE u.status = 'failed') * 100.0 / NULLIF(COUNT(*),0), 2) AS error_rate_pct,
			       ROUND(AVG(u.processing_time_ms)::numeric, 2) AS avg_processing_time_ms
			FROM (%s) u
			LEFT JOIN interfaces i ON i.id::text = u.interface_id
			GROUP BY u.interface_id, i.name
			ORDER BY total_messages DESC LIMIT %d`, union, req.Limit)
	case "message_type":
		q = fmt.Sprintf(`
			SELECT message_type,
			       COUNT(*) AS total_messages,
			       COUNT(*) FILTER (WHERE status IN ('completed','processed')) AS processed_count,
			       COUNT(*) FILTER (WHERE status = 'failed') AS failed_count
			FROM (%s) u GROUP BY message_type ORDER BY total_messages DESC LIMIT %d`, union, req.Limit)
	case "status":
		q = fmt.Sprintf(`
			SELECT status, COUNT(*) AS total_messages
			FROM (%s) u GROUP BY status ORDER BY total_messages DESC`, union)
	default:
		// Time bucket (day/hour/week)
		bucket := "day"
		if dimension == "hour" || dimension == "week" {
			bucket = dimension
		}
		q = fmt.Sprintf(`
			SELECT DATE_TRUNC('%s', received_at) AS bucket,
			       COUNT(*) AS total_messages,
			       COUNT(*) FILTER (WHERE status IN ('completed','processed')) AS processed_count,
			       COUNT(*) FILTER (WHERE status = 'failed') AS failed_count
			FROM (%s) u
			GROUP BY 1 ORDER BY 1 DESC LIMIT %d`, bucket, union, req.Limit)
	}

	return q, args, nil
}

func (s *AnalyticsService) buildPipelinePerformanceQuery(req *ReportExecutionRequest) (string, []interface{}, error) {
	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}
	limit := req.Limit

	dimension := req.Dimensions
	if dimension == "" {
		dimension = "interface_id"
	}

	var q string
	switch dimension {
	case "step_type":
		q = fmt.Sprintf(`
			SELECT se.step_type,
			       COUNT(*) AS total_executions,
			       COUNT(*) FILTER (WHERE se.status = 'failed') AS failure_count,
			       ROUND(COUNT(*) FILTER (WHERE se.status = 'failed') * 100.0 / NULLIF(COUNT(*),0), 2) AS failure_rate_pct,
			       ROUND(AVG(se.duration_ms)::numeric, 2) AS avg_duration_ms
			FROM step_executions se
			JOIN transformation_executions te ON te.id = se.execution_id
			WHERE te.started_at >= $1 AND te.started_at < $2
			GROUP BY se.step_type
			ORDER BY failure_count DESC LIMIT %d`, limit)
	default:
		q = fmt.Sprintf(`
			SELECT te.interface_id,
			       COALESCE(i.name, te.interface_id::text) AS interface_name,
			       COUNT(*) AS total_executions,
			       COUNT(*) FILTER (WHERE te.status = 'completed') AS success_count,
			       ROUND(COUNT(*) FILTER (WHERE te.status = 'completed') * 100.0 / NULLIF(COUNT(*),0), 2) AS success_rate_pct,
			       ROUND(AVG(te.total_time_ms)::numeric, 2) AS avg_duration_ms,
			       ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY te.total_time_ms)::numeric, 2) AS p95_duration_ms,
			       ROUND(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY te.total_time_ms)::numeric, 2) AS p99_duration_ms
			FROM transformation_executions te
			LEFT JOIN interfaces i ON i.id = te.interface_id
			WHERE te.started_at >= $1 AND te.started_at < $2
			GROUP BY te.interface_id, i.name
			ORDER BY total_executions DESC LIMIT %d`, limit)
	}

	return q, args, nil
}

func (s *AnalyticsService) buildHIPAAAuditQuery(req *ReportExecutionRequest) (string, []interface{}, error) {
	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}
	q := fmt.Sprintf(`
		SELECT
			psa.step_name,
			psa.step_type,
			psa.interface_id,
			COALESCE(i.name, psa.interface_id::text) AS interface_name,
			psa.phi_fields_touched,
			psa.masked_fields,
			psa.duration_ms,
			psa.success,
			psa.processed_at
		FROM pipeline_step_audit psa
		LEFT JOIN interfaces i ON i.id = psa.interface_id
		WHERE psa.processed_at >= $1 AND psa.processed_at < $2
		ORDER BY psa.processed_at DESC LIMIT %d`, req.Limit)
	return q, args, nil
}

func (s *AnalyticsService) buildConnectorActivityQuery(req *ReportExecutionRequest) (string, []interface{}, error) {
	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}
	q := fmt.Sprintf(`
		SELECT
			cel.interface_id,
			COALESCE(i.name, cel.interface_id::text) AS interface_name,
			cel.execution_type,
			cel.status,
			cel.started_at,
			cel.completed_at,
			COALESCE(cel.duration_ms, 0) AS duration_ms,
			COALESCE(cel.messages_retrieved, 0) AS messages_retrieved,
			COALESCE(cel.messages_processed, 0) AS messages_processed,
			COALESCE(cel.messages_failed, 0) AS messages_failed
		FROM connectivity_execution_log cel
		LEFT JOIN interfaces i ON i.id = cel.interface_id
		WHERE cel.started_at >= $1 AND cel.started_at < $2
		ORDER BY cel.started_at DESC LIMIT %d`, req.Limit)
	return q, args, nil
}

func (s *AnalyticsService) buildDLQAnalysisQuery(req *ReportExecutionRequest) (string, []interface{}, error) {
	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}

	includeResolved := false
	if v, ok := req.Filters["include_resolved"]; ok {
		if b, ok := v.(bool); ok {
			includeResolved = b
		}
	}
	resolvedFilter := "AND resolved = FALSE"
	if includeResolved {
		resolvedFilter = ""
	}

	q := fmt.Sprintf(`
		SELECT
			COALESCE(dlq.error_type, 'unknown') AS error_type,
			COALESCE(i.name, dlq.interface_id::text) AS interface_name,
			COUNT(*) AS dlq_count,
			ROUND(AVG(dlq.retry_count)::numeric, 1) AS avg_retry_count,
			ROUND(COUNT(*) FILTER (WHERE dlq.resolved) * 100.0 / NULLIF(COUNT(*),0), 2) AS resolution_rate_pct,
			MAX(dlq.failed_at) AS latest_failure
		FROM dead_letter_queue dlq
		LEFT JOIN interfaces i ON i.id = dlq.interface_id
		WHERE dlq.failed_at >= $1 AND dlq.failed_at < $2 %s
		GROUP BY dlq.error_type, i.name, dlq.interface_id
		ORDER BY dlq_count DESC LIMIT %d`, resolvedFilter, req.Limit)
	return q, args, nil
}

func (s *AnalyticsService) buildUserActivityQuery(req *ReportExecutionRequest) (string, []interface{}, error) {
	start, end := s.parseTimeRange(req.Filters)
	args := []interface{}{start, end}
	q := fmt.Sprintf(`
		SELECT
			al.action,
			COALESCE(u.email, al.user_id::text) AS user_email,
			al.entity_type,
			al.result,
			al.risk_level,
			al.ip_address,
			al.created_at
		FROM audit_logs al
		LEFT JOIN users u ON u.id = al.user_id
		WHERE al.created_at >= $1 AND al.created_at < $2
		ORDER BY al.created_at DESC LIMIT %d`, req.Limit)
	return q, args, nil
}

// ─── Alert Generation ─────────────────────────────────────────────────────────

// checkAndWriteAlerts dispatches to dynamic (DB-driven) or legacy (hardcoded) evaluation.
func (s *AnalyticsService) checkAndWriteAlerts(ctx context.Context, summary *MonitoringSummary) error {
	if s.alertRuleSvc != nil {
		return s.checkAndWriteAlertsDynamic(ctx, summary)
	}
	return s.checkAndWriteAlertsLegacy(ctx, summary)
}

// checkAndWriteAlertsLegacy is the original hardcoded threshold evaluation.
// Used as a fallback when AlertRuleService is not injected (e.g. in tests).
func (s *AnalyticsService) checkAndWriteAlertsLegacy(ctx context.Context, summary *MonitoringSummary) error {
	type alertDef struct {
		alertType   string
		severity    string
		condition   bool
		message     string
		interfaceID *string
	}

	checks := []alertDef{
		{
			alertType: "HIGH_ERROR_RATE",
			severity:  "critical",
			condition: summary.KPIs.ErrorRate24h > 10,
			message:   fmt.Sprintf("System error rate %.1f%% exceeds 10%% critical threshold", summary.KPIs.ErrorRate24h),
		},
		{
			alertType: "HIGH_ERROR_RATE",
			severity:  "warning",
			condition: summary.KPIs.ErrorRate24h > 5 && summary.KPIs.ErrorRate24h <= 10,
			message:   fmt.Sprintf("System error rate %.1f%% exceeds 5%% warning threshold", summary.KPIs.ErrorRate24h),
		},
		{
			alertType: "SLOW_PROCESSING",
			severity:  "warning",
			condition: summary.KPIs.AvgProcessingTimeMs > 2000,
			message:   fmt.Sprintf("Average processing time %.0fms exceeds 2s threshold", summary.KPIs.AvgProcessingTimeMs),
		},
		{
			alertType: "DLQ_DEPTH_HIGH",
			severity:  "critical",
			condition: summary.KPIs.DLQDepth > 50,
			message:   fmt.Sprintf("Dead letter queue depth %d exceeds 50 critical threshold", summary.KPIs.DLQDepth),
		},
		{
			alertType: "DLQ_DEPTH_HIGH",
			severity:  "warning",
			condition: summary.KPIs.DLQDepth > 10 && summary.KPIs.DLQDepth <= 50,
			message:   fmt.Sprintf("Dead letter queue depth %d exceeds 10 warning threshold", summary.KPIs.DLQDepth),
		},
	}

	for _, check := range checks {
		if !check.condition {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE monitoring_alerts
				 SET auto_resolved = TRUE, resolved_at = NOW(), updated_at = NOW()
				 WHERE alert_type = $1 AND acknowledged = FALSE AND auto_resolved = FALSE
				   AND ($2::uuid IS NULL OR interface_id = $2)`,
				check.alertType, check.interfaceID,
			)
			continue
		}
		var existing int
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM monitoring_alerts
			 WHERE alert_type = $1 AND acknowledged = FALSE AND auto_resolved = FALSE
			   AND created_at > NOW() - INTERVAL '1 hour'`,
			check.alertType,
		).Scan(&existing)
		if existing == 0 {
			_, _ = s.db.ExecContext(ctx,
				`INSERT INTO monitoring_alerts (alert_type, severity, message, interface_id)
				 VALUES ($1, $2, $3, $4)`,
				check.alertType, check.severity, check.message, check.interfaceID,
			)
		}
	}
	return nil
}

// checkAndWriteAlertsDynamic evaluates all enabled alert_rules from the database,
// honouring per-interface overrides, active silences, and per-rule cooldowns.
func (s *AnalyticsService) checkAndWriteAlertsDynamic(ctx context.Context, summary *MonitoringSummary) error {
	rules, err := s.alertRuleSvc.ListRules(ctx, "")
	if err != nil {
		return fmt.Errorf("load alert rules: %w", err)
	}

	// Build a map of (interfaceID|""|ruleType) → best rule so per-interface rules win
	type ruleKey struct{ interfaceID, ruleType string }
	ruleMap := make(map[ruleKey]*AlertRule)
	for _, r := range rules {
		if !r.IsEnabled {
			continue
		}
		ifaceID := ""
		if r.InterfaceID != nil {
			ifaceID = *r.InterfaceID
		}
		k := ruleKey{ifaceID, r.RuleType}
		if existing, ok := ruleMap[k]; !ok || (ifaceID != "" && existing.InterfaceID == nil) {
			ruleMap[k] = r
		}
	}

	for _, rule := range ruleMap {
		currentValue := s.computeMetric(ctx, rule, summary)
		severity, triggered := s.determineSeverity(rule, currentValue)

		if !triggered {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE monitoring_alerts
				 SET auto_resolved = TRUE, resolved_at = NOW(), updated_at = NOW()
				 WHERE alert_type = $1 AND acknowledged = FALSE AND auto_resolved = FALSE
				   AND ($2::uuid IS NULL OR interface_id = $2::uuid)`,
				rule.RuleType, rule.InterfaceID,
			)
			continue
		}

		// Check silence
		silenced, _ := s.alertRuleSvc.IsSilenced(ctx, rule.InterfaceID, rule.ID)
		if silenced {
			continue
		}

		// Cooldown dedup
		var existing int
		_ = s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM monitoring_alerts
			 WHERE alert_type = $1 AND acknowledged = FALSE AND auto_resolved = FALSE
			   AND ($2::uuid IS NULL OR interface_id = $2::uuid)
			   AND created_at > NOW() - INTERVAL '%d minutes'`, rule.CooldownMinutes),
			rule.RuleType, rule.InterfaceID,
		).Scan(&existing)
		if existing > 0 {
			continue
		}

		// Determine interface name for message
		ifaceName := "System"
		if rule.InterfaceName != "" {
			ifaceName = rule.InterfaceName
		}

		message := s.buildAlertMessage(rule, currentValue, ifaceName)
		var newID string
		err := s.db.QueryRowContext(ctx,
			`INSERT INTO monitoring_alerts (alert_type, severity, message, interface_id)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			rule.RuleType, severity, message, rule.InterfaceID,
		).Scan(&newID)
		if err != nil {
			continue
		}

		// Dispatch notifications asynchronously
		fired := &FiredAlert{
			ID:            newID,
			InterfaceID:   rule.InterfaceID,
			InterfaceName: ifaceName,
			AlertType:     rule.RuleType,
			Severity:      severity,
			Message:       message,
		}
		go s.alertRuleSvc.DispatchAlert(context.Background(), fired, rule.ID)
	}
	return nil
}

// computeMetric returns the current measured value for a rule's metric type.
func (s *AnalyticsService) computeMetric(ctx context.Context, rule *AlertRule, summary *MonitoringSummary) float64 {
	switch rule.RuleType {
	case "ERROR_RATE":
		return summary.KPIs.ErrorRate24h
	case "PROCESSING_TIME":
		return summary.KPIs.AvgProcessingTimeMs
	case "DLQ_DEPTH":
		return float64(summary.KPIs.DLQDepth)
	case "INTERFACE_INACTIVE":
		if rule.InterfaceID == nil {
			return 1 // global INTERFACE_INACTIVE: not meaningful without a specific interface
		}
		var count int64
		tableName := "messages_intf_" + sanitizeUUID(*rule.InterfaceID)
		_ = s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE received_at > NOW() - INTERVAL '%d minutes'`,
				tableName, rule.EvaluationWindowMins),
		).Scan(&count)
		return float64(count)
	case "CONNECTOR_FAILURE":
		var count int64
		_ = s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM connectivity_execution_log
			 WHERE status = 'failed' AND created_at > NOW() - INTERVAL '%d minutes'`,
				rule.EvaluationWindowMins),
		).Scan(&count)
		return float64(count)
	case "THROUGHPUT_LOW":
		var count int64
		_ = s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM transformation_executions WHERE started_at > NOW() - INTERVAL '%d minutes'`,
				rule.EvaluationWindowMins),
		).Scan(&count)
		return float64(count)
	case "VALIDATION_FAILURE":
		var count int64
		_ = s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM transformation_executions
			 WHERE status = 'failed' AND started_at > NOW() - INTERVAL '%d minutes'`,
				rule.EvaluationWindowMins),
		).Scan(&count)
		return float64(count)
	}
	return 0
}

// determineSeverity returns the triggered severity and whether the rule is in breach.
func (s *AnalyticsService) determineSeverity(rule *AlertRule, value float64) (string, bool) {
	compare := func(v, threshold float64) bool {
		switch rule.Condition {
		case "gt":
			return v > threshold
		case "gte":
			return v >= threshold
		case "lt":
			return v < threshold
		case "lte":
			return v <= threshold
		}
		return false
	}

	if rule.ThresholdCritical != nil && compare(value, *rule.ThresholdCritical) {
		return "critical", true
	}
	if rule.ThresholdWarning != nil && compare(value, *rule.ThresholdWarning) {
		return "warning", true
	}
	return "", false
}

// buildAlertMessage formats a human-readable alert message.
func (s *AnalyticsService) buildAlertMessage(rule *AlertRule, value float64, ifaceName string) string {
	switch rule.RuleType {
	case "ERROR_RATE":
		return fmt.Sprintf("%s error rate %.1f%% exceeds threshold", ifaceName, value)
	case "PROCESSING_TIME":
		return fmt.Sprintf("%s average processing time %.0fms exceeds threshold", ifaceName, value)
	case "DLQ_DEPTH":
		return fmt.Sprintf("Dead letter queue depth %.0f exceeds threshold", value)
	case "INTERFACE_INACTIVE":
		return fmt.Sprintf("Interface %s has received no messages in the last %d minutes", ifaceName, rule.EvaluationWindowMins)
	case "THROUGHPUT_LOW":
		return fmt.Sprintf("Message throughput %.0f msg/%dmin is below threshold", value, rule.EvaluationWindowMins)
	case "VALIDATION_FAILURE":
		return fmt.Sprintf("%.0f validation failures detected in the last %d minutes", value, rule.EvaluationWindowMins)
	}
	return fmt.Sprintf("Alert %s: current value %.2f", rule.RuleType, value)
}

// sanitizeUUID strips hyphens from a UUID for use in table names.
func sanitizeUUID(id string) string {
	result := ""
	for _, c := range id {
		if (c >= 'a' && c <= 'f') || (c >= '0' && c <= '9') {
			result += string(c)
		} else if c == '-' {
			result += "_"
		}
	}
	return result
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var safeTableNameRegex = regexp.MustCompile(`^messages_intf_[a-f0-9_]+$`)

func (s *AnalyticsService) listMessageInterfaceTables(ctx context.Context) ([]string, error) {
	const cacheKey = "analytics:table_list"
	if cached, ok := s.cache.Get(cacheKey); ok {
		if tables, ok := cached.([]string); ok {
			return tables, nil
		}
	}

	// Cross-join with information_schema.tables to guarantee only real tables
	// are returned — stale or truncated rows in interface_table_metadata would
	// cause the UNION query to fail with "table does not exist".
	rows, err := s.db.QueryContext(ctx,
		`SELECT itm.table_name
		 FROM interface_table_metadata itm
		 JOIN information_schema.tables ist
		   ON ist.table_name = itm.table_name AND ist.table_schema = 'public'
		 WHERE itm.table_name LIKE 'messages_intf_%'
		 ORDER BY itm.table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && safeTableNameRegex.MatchString(name) {
			tables = append(tables, name)
		}
	}

	s.cache.Set(cacheKey, tables, cacheTTLTableList)
	return tables, nil
}

// rawTableSuffixToUUID converts the underscore-based table suffix back to a
// standard UUID string with dashes (e.g. "762aebb9_0408_4a42_82c5_202f13f28315"
// → "762aebb9-0408-4a42-82c5-202f13f28315").  If the suffix doesn't look like a
// UUID it is returned unchanged so the LEFT JOIN on interfaces.id still works.
func rawTableSuffixToUUID(s string) string {
	// A UUID has exactly 5 groups of hex chars separated by dashes: 8-4-4-4-12
	parts := strings.SplitN(s, "_", 5)
	if len(parts) == 5 &&
		len(parts[0]) == 8 && len(parts[1]) == 4 &&
		len(parts[2]) == 4 && len(parts[3]) == 4 && len(parts[4]) == 12 {
		return strings.Join(parts, "-")
	}
	return s
}

func (s *AnalyticsService) buildUnionCountQuery(tables []string, whereClause string) string {
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		parts = append(parts, fmt.Sprintf(`SELECT 1 FROM %s WHERE %s`, t, whereClause))
	}
	return fmt.Sprintf(`SELECT COUNT(*) FROM (%s) sub`, strings.Join(parts, " UNION ALL "))
}

func (s *AnalyticsService) tableExists(ctx context.Context, tableName string) bool {
	var exists bool
	_ = s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
		tableName,
	).Scan(&exists)
	return exists
}

func (s *AnalyticsService) parseTimeRange(filters map[string]interface{}) (time.Time, time.Time) {
	now := time.Now()
	end := now

	tr, _ := filters["time_range"].(string)
	switch tr {
	case "last_1h":
		return now.Add(-time.Hour), end
	case "last_7d":
		return now.Add(-7 * 24 * time.Hour), end
	case "last_30d":
		return now.Add(-30 * 24 * time.Hour), end
	case "last_90d":
		return now.Add(-90 * 24 * time.Hour), end
	default: // last_24h
		return now.Add(-24 * time.Hour), end
	}
}
