package controllers

import (
	"database/sql"
	"errors"
	"net/http"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// AlertRuleController handles all /api/alerts/* endpoints.
type AlertRuleController struct {
	svc *services.AlertRuleService
}

// NewAlertRuleController creates a new AlertRuleController.
func NewAlertRuleController(db *sql.DB) *AlertRuleController {
	return &AlertRuleController{svc: services.NewAlertRuleService(db)}
}

// Service exposes the underlying service so main.go can inject it into AnalyticsService.
func (ctrl *AlertRuleController) Service() *services.AlertRuleService {
	return ctrl.svc
}

// RegisterRoutes mounts all /api/alerts/* routes.
func (ctrl *AlertRuleController) RegisterRoutes(rg *gin.RouterGroup) {
	// Rules
	rg.GET("/rules", ctrl.ListRules)
	rg.POST("/rules", ctrl.CreateRule)
	rg.GET("/rules/:id", ctrl.GetRule)
	rg.PUT("/rules/:id", ctrl.UpdateRule)
	rg.DELETE("/rules/:id", ctrl.DeleteRule)
	rg.GET("/rules/:id/channels", ctrl.GetRuleChannels)
	rg.PUT("/rules/:id/channels", ctrl.AssignChannels)

	// Channels
	rg.GET("/channels", ctrl.ListChannels)
	rg.POST("/channels", ctrl.CreateChannel)
	rg.GET("/channels/:id", ctrl.GetChannel)
	rg.PUT("/channels/:id", ctrl.UpdateChannel)
	rg.DELETE("/channels/:id", ctrl.DeleteChannel)
	rg.POST("/channels/:id/test", ctrl.TestChannel)

	// Silences
	rg.GET("/silences", ctrl.ListSilences)
	rg.POST("/silences", ctrl.CreateSilence)
	rg.DELETE("/silences/:id", ctrl.DeleteSilence)

	// Fired alerts (monitoring_alerts table)
	rg.GET("/fired", ctrl.ListFiredAlerts)
	rg.POST("/fired/:id/acknowledge", ctrl.AcknowledgeFiredAlert)
	rg.POST("/fired/:id/resolve", ctrl.ResolveFiredAlert)
	rg.POST("/fired/acknowledge-all", ctrl.AcknowledgeAll)

	// History — all alerts including resolved
	rg.GET("/history", ctrl.ListAlertHistory)
}

// ── Rules ─────────────────────────────────────────────────────────────────────

// ListRules returns all alert rules.
// GET /api/alerts/rules?interfaceId=<uuid>
func (ctrl *AlertRuleController) ListRules(c *gin.Context) {
	rules, err := ctrl.svc.ListRules(c.Request.Context(), c.Query("interfaceId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if rules == nil {
		rules = []*services.AlertRule{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rules, "count": len(rules)})
}

// GetRule returns a single alert rule.
// GET /api/alerts/rules/:id
func (ctrl *AlertRuleController) GetRule(c *gin.Context) {
	id := c.Param("id")
	rule, err := ctrl.svc.GetRule(c.Request.Context(), id)
	if err != nil {
		// Invalid UUID format from postgres causes a pq error — treat as 404
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "rule not found"})
		return
	}
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "rule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

// CreateRule creates a new alert rule.
// POST /api/alerts/rules
func (ctrl *AlertRuleController) CreateRule(c *gin.Context) {
	var r services.AlertRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	if r.Name == "" || r.RuleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name and rule_type are required"})
		return
	}
	if r.Condition == "" {
		r.Condition = "gt"
	}
	if r.EvaluationWindowMins == 0 {
		r.EvaluationWindowMins = 60
	}
	if r.CooldownMinutes == 0 {
		r.CooldownMinutes = 60
	}
	if err := ctrl.svc.CreateRule(c.Request.Context(), &r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": r})
}

// UpdateRule updates a rule's mutable fields.
// PUT /api/alerts/rules/:id
func (ctrl *AlertRuleController) UpdateRule(c *gin.Context) {
	var r services.AlertRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	r.ID = c.Param("id")
	if err := ctrl.svc.UpdateRule(c.Request.Context(), &r); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "rule not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteRule removes an alert rule. Returns 403 for system rules.
// DELETE /api/alerts/rules/:id
func (ctrl *AlertRuleController) DeleteRule(c *gin.Context) {
	err := ctrl.svc.DeleteRule(c.Request.Context(), c.Param("id"))
	if errors.Is(err, services.ErrSystemRule) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "system rules cannot be deleted; you may disable them instead"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetRuleChannels returns channels assigned to a rule.
// GET /api/alerts/rules/:id/channels
func (ctrl *AlertRuleController) GetRuleChannels(c *gin.Context) {
	assignments, err := ctrl.svc.GetRuleChannels(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if assignments == nil {
		assignments = []*services.AlertRuleChannel{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assignments, "count": len(assignments)})
}

// AssignChannels replaces all channel assignments for a rule.
// PUT /api/alerts/rules/:id/channels
func (ctrl *AlertRuleController) AssignChannels(c *gin.Context) {
	var body struct {
		Channels []services.AlertRuleChannel `json:"channels"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	if err := ctrl.svc.AssignChannels(c.Request.Context(), c.Param("id"), body.Channels); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── Channels ──────────────────────────────────────────────────────────────────

// ListChannels returns all notification channels.
// GET /api/alerts/channels
func (ctrl *AlertRuleController) ListChannels(c *gin.Context) {
	channels, err := ctrl.svc.ListChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if channels == nil {
		channels = []*services.AlertChannel{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": channels, "count": len(channels)})
}

// GetChannel returns a single notification channel.
// GET /api/alerts/channels/:id
func (ctrl *AlertRuleController) GetChannel(c *gin.Context) {
	ch, err := ctrl.svc.GetChannel(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "channel not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ch})
}

// CreateChannel creates a new notification channel.
// POST /api/alerts/channels
func (ctrl *AlertRuleController) CreateChannel(c *gin.Context) {
	var ch services.AlertChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	if ch.Name == "" || ch.ChannelType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name and channel_type are required"})
		return
	}
	if ch.Config == nil {
		ch.Config = []byte("{}")
	}
	ch.IsEnabled = true
	if err := ctrl.svc.CreateChannel(c.Request.Context(), &ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": ch})
}

// UpdateChannel updates a channel's config.
// PUT /api/alerts/channels/:id
func (ctrl *AlertRuleController) UpdateChannel(c *gin.Context) {
	var ch services.AlertChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	ch.ID = c.Param("id")
	if err := ctrl.svc.UpdateChannel(c.Request.Context(), &ch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteChannel removes a channel.
// DELETE /api/alerts/channels/:id
func (ctrl *AlertRuleController) DeleteChannel(c *gin.Context) {
	if err := ctrl.svc.DeleteChannel(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestChannel sends a test payload and updates test_status.
// POST /api/alerts/channels/:id/test
func (ctrl *AlertRuleController) TestChannel(c *gin.Context) {
	if err := ctrl.svc.TestChannel(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test notification sent successfully"})
}

// ── Silences ──────────────────────────────────────────────────────────────────

// ListSilences returns all silences.
// GET /api/alerts/silences?activeOnly=true
func (ctrl *AlertRuleController) ListSilences(c *gin.Context) {
	activeOnly := c.Query("activeOnly") == "true"
	silences, err := ctrl.svc.ListSilences(c.Request.Context(), activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if silences == nil {
		silences = []*services.AlertSilence{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": silences, "count": len(silences)})
}

// CreateSilence creates a maintenance window silence.
// POST /api/alerts/silences
func (ctrl *AlertRuleController) CreateSilence(c *gin.Context) {
	var sil services.AlertSilence
	if err := c.ShouldBindJSON(&sil); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: " + err.Error()})
		return
	}
	if sil.Name == "" || sil.StartsAt == "" || sil.EndsAt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name, starts_at and ends_at are required"})
		return
	}
	if err := ctrl.svc.CreateSilence(c.Request.Context(), &sil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sil})
}

// DeleteSilence removes a silence.
// DELETE /api/alerts/silences/:id
func (ctrl *AlertRuleController) DeleteSilence(c *gin.Context) {
	if err := ctrl.svc.DeleteSilence(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "silence not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── Fired Alerts ──────────────────────────────────────────────────────────────

// ListFiredAlerts returns active (unacknowledged, unresolved) alerts.
// GET /api/alerts/fired?interfaceId=<uuid>&countOnly=true
func (ctrl *AlertRuleController) ListFiredAlerts(c *gin.Context) {
	interfaceID := c.Query("interfaceId")
	countOnly := c.Query("countOnly") == "true"

	alerts, count, err := ctrl.svc.ListFiredAlerts(c.Request.Context(), interfaceID, countOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if countOnly {
		c.JSON(http.StatusOK, gin.H{"success": true, "count": count})
		return
	}
	if alerts == nil {
		alerts = []*services.FiredAlert{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": alerts, "count": count})
}

// AcknowledgeFiredAlert marks an alert as acknowledged.
// POST /api/alerts/fired/:id/acknowledge
func (ctrl *AlertRuleController) AcknowledgeFiredAlert(c *gin.Context) {
	var body struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.AcknowledgedBy == "" {
		body.AcknowledgedBy = "system"
	}
	if err := ctrl.svc.AcknowledgeFiredAlert(c.Request.Context(), c.Param("id"), body.AcknowledgedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResolveFiredAlert marks an alert as manually resolved.
// POST /api/alerts/fired/:id/resolve
func (ctrl *AlertRuleController) ResolveFiredAlert(c *gin.Context) {
	if err := ctrl.svc.ResolveFiredAlert(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AcknowledgeAll bulk-acknowledges all active alerts.
// POST /api/alerts/fired/acknowledge-all
func (ctrl *AlertRuleController) AcknowledgeAll(c *gin.Context) {
	var body struct {
		InterfaceID    string `json:"interface_id"`
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.AcknowledgedBy == "" {
		body.AcknowledgedBy = "system"
	}
	count, err := ctrl.svc.AcknowledgeAll(c.Request.Context(), body.InterfaceID, body.AcknowledgedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "acknowledged": count})
}

// ListAlertHistory returns all monitoring_alerts (including resolved), newest first.
// GET /api/alerts/history
func (ctrl *AlertRuleController) ListAlertHistory(c *gin.Context) {
	alerts, err := ctrl.svc.ListAlertHistory(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []*services.FiredAlert{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": alerts, "count": len(alerts)})
}
