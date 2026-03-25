package controllers

import (
	"database/sql"
	"net/http"
	"strconv"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// AnalyticsController handles monitoring and reporting API endpoints.
type AnalyticsController struct {
	svc *services.AnalyticsService
}

// NewAnalyticsController creates a new AnalyticsController.
func NewAnalyticsController(db *sql.DB) *AnalyticsController {
	return &AnalyticsController{svc: services.NewAnalyticsService(db)}
}

// InjectAlertRuleService wires the AlertRuleService into the underlying AnalyticsService
// so that checkAndWriteAlerts uses dynamic DB-driven rules.
func (ctrl *AnalyticsController) InjectAlertRuleService(svc *services.AlertRuleService) {
	ctrl.svc.SetAlertRuleService(svc)
}

// RegisterRoutes mounts all /api/analytics/* routes onto the provided group.
func (ctrl *AnalyticsController) RegisterRoutes(rg *gin.RouterGroup) {
	mon := rg.Group("/monitoring")
	{
		mon.GET("/summary", ctrl.GetMonitoringSummary)
		mon.GET("/live-messages", ctrl.GetLiveMessages)
		mon.GET("/interface-health", ctrl.GetInterfaceHealthCards)
		mon.GET("/connector-health", ctrl.GetConnectorHealth)
		mon.GET("/alerts", ctrl.GetActiveAlerts)
		mon.POST("/alerts/:id/acknowledge", ctrl.AcknowledgeAlert)
	}

	rep := rg.Group("/reports")
	{
		rep.GET("", ctrl.ListReportDefinitions)
		rep.POST("/execute", ctrl.ExecuteReport)
		rep.POST("", ctrl.SaveReportDefinition)
		rep.DELETE("/:id", ctrl.DeleteReportDefinition)
	}
}

// ── Monitoring ────────────────────────────────────────────────────────────────

// GetMonitoringSummary returns headline KPIs, sparklines, and alert counts.
// GET /api/analytics/monitoring/summary
func (ctrl *AnalyticsController) GetMonitoringSummary(c *gin.Context) {
	summary, err := ctrl.svc.GetMonitoringSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetLiveMessages returns the most recent messages across all interface tables.
// GET /api/analytics/monitoring/live-messages?limit=50
func (ctrl *AnalyticsController) GetLiveMessages(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	msgs, err := ctrl.svc.GetLiveMessages(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": msgs, "count": len(msgs)})
}

// GetInterfaceHealthCards returns per-interface monitoring mini-card data.
// GET /api/analytics/monitoring/interface-health
func (ctrl *AnalyticsController) GetInterfaceHealthCards(c *gin.Context) {
	cards, err := ctrl.svc.GetInterfaceHealthCards(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cards, "count": len(cards)})
}

// GetConnectorHealth returns the latest connector run status per interface.
// GET /api/analytics/monitoring/connector-health
func (ctrl *AnalyticsController) GetConnectorHealth(c *gin.Context) {
	rows, err := ctrl.svc.GetConnectorHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "count": len(rows)})
}

// GetActiveAlerts returns unacknowledged monitoring alerts.
// GET /api/analytics/monitoring/alerts
func (ctrl *AnalyticsController) GetActiveAlerts(c *gin.Context) {
	alerts, err := ctrl.svc.GetActiveAlerts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": alerts, "count": len(alerts)})
}

// AcknowledgeAlert marks an alert as acknowledged.
// POST /api/analytics/monitoring/alerts/:id/acknowledge
func (ctrl *AnalyticsController) AcknowledgeAlert(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "alert id required"})
		return
	}

	var body struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.AcknowledgedBy == "" {
		body.AcknowledgedBy = "system"
	}

	if err := ctrl.svc.AcknowledgeAlert(c.Request.Context(), alertID, body.AcknowledgedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── Reports ───────────────────────────────────────────────────────────────────

// ListReportDefinitions returns all OOB + user-saved report definitions.
// GET /api/analytics/reports
func (ctrl *AnalyticsController) ListReportDefinitions(c *gin.Context) {
	userID := c.Query("user_id")
	defs, err := ctrl.svc.ListReportDefinitions(c.Request.Context(), userID, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": defs, "count": len(defs)})
}

// ExecuteReport runs an ad-hoc or saved report query.
// POST /api/analytics/reports/execute
func (ctrl *AnalyticsController) ExecuteReport(c *gin.Context) {
	var req services.ReportExecutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}

	result, err := ctrl.svc.ExecuteReport(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// SaveReportDefinition persists a user-created report definition.
// POST /api/analytics/reports
func (ctrl *AnalyticsController) SaveReportDefinition(c *gin.Context) {
	var def services.ReportDefinition
	if err := c.ShouldBindJSON(&def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}
	if def.Name == "" || def.DataSource == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name and data_source are required"})
		return
	}

	if err := ctrl.svc.SaveReportDefinition(c.Request.Context(), &def); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

// DeleteReportDefinition removes a user-created report (system reports protected).
// DELETE /api/analytics/reports/:id
func (ctrl *AnalyticsController) DeleteReportDefinition(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "report id required"})
		return
	}

	if err := ctrl.svc.DeleteReportDefinition(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
