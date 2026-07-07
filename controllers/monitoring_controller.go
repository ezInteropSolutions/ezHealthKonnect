// controllers/monitoring_controller.go
//
// MonitoringController serves the real-time ops monitoring dashboard.
//
// Routes (registered under /api/monitoring):
//
//	GET /summary          — KPI roll-up: messages today, error rate, avg latency,
//	                        interface counts, DLQ depth, alert severity counts
//	GET /alerts           — Active (un-ack'd) monitoring_alerts, newest first
//	GET /live-messages    — Latest N transformed messages across all interfaces
//	GET /connector-health — Per-interface connector last-run status
//	GET /interface-health — Per-interface quality + DLQ cards
//
// All endpoints return { success: true, data: <payload> }.
// They require proxy auth only (no role check) — the proxy layer enforces auth
// before the request reaches the Go backend.
package controllers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// MonitoringController handles the live monitoring dashboard API.
type MonitoringController struct {
	db *sql.DB
}

// NewMonitoringController creates a new controller backed by the shared DB pool.
func NewMonitoringController(db *sql.DB) *MonitoringController {
	return &MonitoringController{db: db}
}

// RegisterRoutes mounts the monitoring endpoints under the supplied router group.
func (mc *MonitoringController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/summary", mc.Summary)
	rg.GET("/alerts", mc.Alerts)
	rg.POST("/alerts/:id/ack", mc.AckAlert)
	rg.POST("/alerts/:id/acknowledge", mc.AckAlert) // alias used by monitoring.html
	rg.GET("/live-messages", mc.LiveMessages)
	rg.GET("/connector-health", mc.ConnectorHealth)
	rg.GET("/interface-health", mc.InterfaceHealth)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /summary
// ─────────────────────────────────────────────────────────────────────────────

type monitoringKPIs struct {
	MessagesToday       int     `json:"messages_today"`
	ErrorRate24h        float64 `json:"error_rate_24h"`
	AvgProcessingTimeMs float64 `json:"avg_processing_time_ms"`
	ActiveInterfaces    int     `json:"active_interfaces"`
	TotalInterfaces     int     `json:"total_interfaces"`
	DLQDepth            int     `json:"dlq_depth"`
}

type alertSevCounts struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

func (mc *MonitoringController) Summary(c *gin.Context) {
	ctx := c.Request.Context()

	kpis := monitoringKPIs{}
	sev := alertSevCounts{}

	// Messages today + error rate from transformation_executions
	midnight := time.Now().UTC().Truncate(24 * time.Hour)
	row := mc.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)                                                    AS total,
			COUNT(*) FILTER (WHERE status = 'failed')                   AS failed,
			COALESCE(AVG(total_time_ms) FILTER (WHERE status = 'completed'), 0) AS avg_ms
		FROM transformation_executions
		WHERE started_at >= $1`, midnight)
	var total, failed int
	var avgMs float64
	if err := row.Scan(&total, &failed, &avgMs); err == nil {
		kpis.MessagesToday = total
		if total > 0 {
			kpis.ErrorRate24h = float64(failed) / float64(total) * 100.0
		}
		kpis.AvgProcessingTimeMs = avgMs
	}

	// Interface counts
	mc.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_active = true AND status = 'active'),
			COUNT(*)
		FROM interfaces`).Scan(&kpis.ActiveInterfaces, &kpis.TotalInterfaces)

	// DLQ depth — pending + retrying
	mc.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM delivery_dlq WHERE status IN ('pending','retrying')`).Scan(&kpis.DLQDepth)

	// Alert severity counts from monitoring_alerts (un-ack'd)
	rows, err := mc.db.QueryContext(ctx, `
		SELECT severity, COUNT(*)
		FROM monitoring_alerts
		WHERE acknowledged = false
		GROUP BY severity`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sv string
			var cnt int
			if rows.Scan(&sv, &cnt) == nil {
				switch sv {
				case "critical":
					sev.Critical += cnt
				case "warning":
					sev.Warning += cnt
				default:
					sev.Info += cnt
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"kpis":                   kpis,
			"alert_count_by_severity": sev,
			"kpi_trends": gin.H{
				"messages_today_sparkline": []int{},
				"error_rate_sparkline":     []float64{},
			},
		},
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /alerts
// ─────────────────────────────────────────────────────────────────────────────

func (mc *MonitoringController) Alerts(c *gin.Context) {
	ctx := c.Request.Context()

	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			a.id,
			a.interface_id,
			COALESCE(i.name, a.interface_id::text, 'System') AS interface_name,
			a.alert_type,
			a.severity,
			a.message,
			a.acknowledged,
			COALESCE(EXTRACT(EPOCH FROM (NOW() - a.created_at))::int, 0) AS elapsed_seconds,
			a.created_at
		FROM monitoring_alerts a
		LEFT JOIN interfaces i ON i.id = a.interface_id
		WHERE a.acknowledged = false
		ORDER BY
			CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
			a.created_at DESC
		LIMIT 100`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	type alertRow struct {
		ID             string    `json:"id"`
		InterfaceID    *string   `json:"interface_id"`
		InterfaceName  string    `json:"interface_name"`
		AlertType      string    `json:"alert_type"`
		Severity       string    `json:"severity"`
		Message        string    `json:"message"`
		Acknowledged   bool      `json:"acknowledged"`
		ElapsedSeconds int       `json:"elapsed_seconds"`
		CreatedAt      time.Time `json:"created_at"`
	}
	var alerts []alertRow
	for rows.Next() {
		var a alertRow
		if err := rows.Scan(
			&a.ID, &a.InterfaceID, &a.InterfaceName,
			&a.AlertType, &a.Severity, &a.Message,
			&a.Acknowledged, &a.ElapsedSeconds, &a.CreatedAt,
		); err == nil {
			alerts = append(alerts, a)
		}
	}
	if alerts == nil {
		alerts = []alertRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": alerts})
}

// AckAlert acknowledges a monitoring alert (supports both /ack and /acknowledge paths).
func (mc *MonitoringController) AckAlert(c *gin.Context) {
	id := c.Param("id")
	_, err := mc.db.ExecContext(c.Request.Context(), `
		UPDATE monitoring_alerts
		SET acknowledged = true, acknowledged_at = NOW()
		WHERE id = $1::uuid`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /live-messages
// ─────────────────────────────────────────────────────────────────────────────

func (mc *MonitoringController) LiveMessages(c *gin.Context) {
	ctx := c.Request.Context()
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Use transformation_executions as the live feed — every processed message
	// gets an execution record with status, timing, and interface context.
	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			e.message_id,
			COALESCE(i.name, e.interface_id::text) AS interface_name,
			COALESCE(p.message_type, '')            AS message_type,
			e.status,
			e.total_time_ms,
			e.started_at
		FROM transformation_executions e
		LEFT JOIN interfaces i             ON i.id = e.interface_id
		LEFT JOIN transformation_pipelines p ON p.id = e.pipeline_id
		ORDER BY e.started_at DESC
		LIMIT $1`, limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	type msgRow struct {
		MessageID        string    `json:"message_id"`
		InterfaceName    string    `json:"interface_name"`
		MessageType      string    `json:"message_type"`
		Status           string    `json:"status"`
		ProcessingTimeMs *int64    `json:"processing_time_ms"`
		ReceivedAt       time.Time `json:"received_at"`
	}
	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(
			&m.MessageID, &m.InterfaceName, &m.MessageType,
			&m.Status, &m.ProcessingTimeMs, &m.ReceivedAt,
		); err == nil {
			msgs = append(msgs, m)
		}
	}
	if msgs == nil {
		msgs = []msgRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": msgs})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /connector-health
// ─────────────────────────────────────────────────────────────────────────────

func (mc *MonitoringController) ConnectorHealth(c *gin.Context) {
	ctx := c.Request.Context()

	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			i.name                                     AS interface_name,
			COALESCE(conn.connector_type, '')          AS connector_type,
			i.status,
			i.last_processed_at,
			COALESCE(i.successful_processed, 0)        AS messages_last_run
		FROM interfaces i
		LEFT JOIN LATERAL (
			SELECT ts.config->>'connectorType' AS connector_type
			FROM transformation_pipelines tp
			JOIN transformation_steps ts ON ts.pipeline_id = tp.id
			WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
			LIMIT 1
		) conn ON true
		WHERE i.is_active = true
		ORDER BY i.last_processed_at DESC NULLS LAST
		LIMIT 50`)
	if err != nil {
		log.Printf("MonitoringController.ConnectorHealth query failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	type connRow struct {
		InterfaceName   string     `json:"interface_name"`
		ConnectorType   string     `json:"connector_type"`
		LastRunStatus   string     `json:"last_run_status"`
		LastRunAt       *time.Time `json:"last_run_at"`
		MessagesLastRun int        `json:"messages_last_run"`
	}
	var result []connRow
	for rows.Next() {
		var r connRow
		if err := rows.Scan(
			&r.InterfaceName, &r.ConnectorType,
			&r.LastRunStatus, &r.LastRunAt, &r.MessagesLastRun,
		); err == nil {
			result = append(result, r)
		}
	}
	if result == nil {
		result = []connRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /interface-health
// ─────────────────────────────────────────────────────────────────────────────

func (mc *MonitoringController) InterfaceHealth(c *gin.Context) {
	ctx := c.Request.Context()
	midnight := time.Now().UTC().Truncate(24 * time.Hour)

	// Per-interface card data:
	//   - Basic info: id, name, status, connector type (first inbound connector)
	//   - Today's execution counts from transformation_executions
	//   - DLQ depth from delivery_dlq
	rows, err := mc.db.QueryContext(ctx, `
		SELECT
			i.id::text                                         AS interface_id,
			i.name                                             AS interface_name,
			i.status,
			COALESCE(conn.connector_type, '')                  AS connector_type,
			COALESCE(today.total, 0)                           AS messages_today,
			COALESCE(today.failed, 0)                          AS messages_failed,
			COALESCE(dlq.depth, 0)                             AS dlq_depth
		FROM interfaces i
		LEFT JOIN LATERAL (
			SELECT ts.config->>'connectorType' AS connector_type
			FROM transformation_pipelines tp
			JOIN transformation_steps ts ON ts.pipeline_id = tp.id
			WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
			LIMIT 1
		) conn ON true
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*)                                   AS total,
				COUNT(*) FILTER (WHERE status = 'failed') AS failed
			FROM transformation_executions
			WHERE interface_id = i.id AND started_at >= $1
		) today ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS depth
			FROM delivery_dlq
			WHERE interface_id = i.id AND status IN ('pending','retrying')
		) dlq ON true
		WHERE i.is_active = true
		ORDER BY i.name`, midnight)
	if err != nil {
		log.Printf("MonitoringController.InterfaceHealth query failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	type ifaceCard struct {
		InterfaceID     string  `json:"interface_id"`
		InterfaceName   string  `json:"interface_name"`
		Status          string  `json:"status"`
		ConnectorType   string  `json:"connector_type"`
		MessagesToday   int     `json:"messages_today"`
		ErrorRateToday  float64 `json:"error_rate_today"`
		DLQDepth        int     `json:"dlq_depth"`
		SparklineData   []int   `json:"sparkline_data"`
	}
	var cards []ifaceCard
	for rows.Next() {
		var card ifaceCard
		var failed int
		if err := rows.Scan(
			&card.InterfaceID, &card.InterfaceName, &card.Status, &card.ConnectorType,
			&card.MessagesToday, &failed, &card.DLQDepth,
		); err != nil {
			continue
		}
		if card.MessagesToday > 0 {
			card.ErrorRateToday = float64(failed) / float64(card.MessagesToday) * 100.0
		}
		card.SparklineData = []int{} // populated by metrics query in future iteration
		cards = append(cards, card)
	}
	if cards == nil {
		cards = []ifaceCard{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cards})
}
