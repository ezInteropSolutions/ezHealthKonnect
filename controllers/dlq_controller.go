// controllers/dlq_controller.go
//
// DLQController manages the dead-letter queue for failed outbound deliveries.
// Accessible to engineer, admin, and super_admin roles.
//
// Routes (registered under /api/fhir/dlq):
//   GET  /                       — list rows (filter: status, interface_id, limit, offset)
//   GET  /stats                  — global counts by status
//   GET  /interface/:id/stats    — per-interface counts by status (for depth badge)
//   GET  /:id                    — single row detail with payload + snapshots
//   POST /:id/redrive            — immediate redrive; body: {"mode":"from_failed_step"|"from_start"}
//   POST /:id/schedule           — schedule redrive; body: {"mode":"...","at":"RFC3339"}
//   POST /:id/abandon            — mark abandoned (no further retries)
//   POST /bulk-redrive           — body: {"ids":[...],"mode":"..."}
//   POST /bulk-schedule          — body: {"ids":[...],"mode":"...","at":"RFC3339"}
//   POST /bulk-abandon           — body: {"ids":[...]}
package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"ezhealthkonnect/services/connectors"

	"github.com/gin-gonic/gin"
)

// DLQController manages delivery_dlq via DLQServicer.
type DLQController struct {
	dlqSvc connectors.DLQServicer
	db     *sql.DB // for audit trail writes
}

// NewDLQController creates a DLQController backed by the given DLQServicer.
func NewDLQController(db *sql.DB, dlqSvc connectors.DLQServicer) *DLQController {
	return &DLQController{dlqSvc: dlqSvc, db: db}
}

// RegisterRoutes wires all DLQ endpoints onto the given router group.
func (dc *DLQController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", dc.List)
	rg.GET("/stats", dc.Stats)
	rg.GET("/interface/:ifaceId/stats", dc.InterfaceStats)
	rg.GET("/:id", dc.Get)
	rg.POST("/:id/redrive", dc.Redrive)
	rg.POST("/:id/schedule", dc.Schedule)
	rg.POST("/:id/abandon", dc.AbandonOne)
	rg.POST("/bulk-redrive", dc.BulkRedrive)
	rg.POST("/bulk-schedule", dc.BulkSchedule)
	rg.POST("/bulk-abandon", dc.BulkAbandon)
}

// List returns DLQ rows. Query params: status (default "pending"), interface_id, message_id, limit, offset.
func (dc *DLQController) List(c *gin.Context) {
	status := c.DefaultQuery("status", "pending")
	interfaceID := c.Query("interface_id")
	messageID := c.Query("message_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	rows, err := dc.dlqSvc.ListPending(c.Request.Context(), interfaceID, messageID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if rows == nil {
		rows = []*connectors.DLQRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "count": len(rows)})
}

// Stats returns aggregate counts grouped by status across all interfaces.
func (dc *DLQController) Stats(c *gin.Context) {
	counts, err := dc.dlqSvc.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": counts})
}

// InterfaceStats returns DLQ counts per status for a specific interface (depth badge).
func (dc *DLQController) InterfaceStats(c *gin.Context) {
	ifaceID := c.Param("ifaceId")
	counts, err := dc.dlqSvc.InterfaceStats(c.Request.Context(), ifaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": counts})
}

// Get returns a single DLQ row by ID.
func (dc *DLQController) Get(c *gin.Context) {
	row, err := dc.dlqSvc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// Redrive triggers immediate redrive of a single DLQ row.
// Works on pending, retrying, and abandoned rows.
// Body: {"mode": "from_failed_step" | "from_start"} (optional).
func (dc *DLQController) Redrive(c *gin.Context) {
	var body struct {
		Mode string `json:"mode"`
	}
	_ = c.ShouldBindJSON(&body)
	id := c.Param("id")

	if err := dc.dlqSvc.ExecuteRedrive(c.Request.Context(), id, body.Mode, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	dc.writeAudit(c, id, "dlq_redrive", fmt.Sprintf("immediate redrive, mode=%s", body.Mode))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Redrive triggered"})
}

// Schedule sets a future redrive time on any row (including abandoned).
// Resets attempt_count to 0. Body: {"mode":"...","at":"2006-01-02T15:04:05Z"}.
func (dc *DLQController) Schedule(c *gin.Context) {
	var body struct {
		Mode string `json:"mode"`
		At   string `json:"at"` // RFC3339
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.At == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "at (RFC3339) is required"})
		return
	}
	at, err := time.Parse(time.RFC3339, body.At)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "at must be RFC3339 (e.g. 2026-05-25T10:00:00Z)"})
		return
	}
	id := c.Param("id")
	if err := dc.dlqSvc.ScheduleRedrive(c.Request.Context(), id, body.Mode, at); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	dc.writeAudit(c, id, "dlq_schedule", fmt.Sprintf("scheduled redrive at=%s mode=%s", at.Format(time.RFC3339), body.Mode))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Redrive scheduled", "scheduled_at": at})
}

// AbandonOne marks a single DLQ row as abandoned (no further retries).
func (dc *DLQController) AbandonOne(c *gin.Context) {
	id := c.Param("id")
	if err := dc.dlqSvc.Abandon(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	dc.writeAudit(c, id, "dlq_abandon", "single abandon")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Row abandoned"})
}

// BulkRedrive redrives multiple DLQ rows immediately.
// Body: {"ids":[...],"mode":"..."}
func (dc *DLQController) BulkRedrive(c *gin.Context) {
	var body struct {
		IDs  []string `json:"ids"`
		Mode string   `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ids array is required"})
		return
	}

	ctx := c.Request.Context()
	results := make([]map[string]interface{}, 0, len(body.IDs))
	for _, id := range body.IDs {
		err := dc.dlqSvc.ExecuteRedrive(ctx, id, body.Mode, nil)
		entry := map[string]interface{}{"id": id}
		if err != nil {
			entry["success"] = false
			entry["error"] = err.Error()
		} else {
			entry["success"] = true
		}
		results = append(results, entry)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}

// BulkSchedule reactivates multiple rows and schedules them for redrive.
// Body: {"ids":[...],"mode":"...","at":"RFC3339"}
func (dc *DLQController) BulkSchedule(c *gin.Context) {
	var body struct {
		IDs  []string `json:"ids"`
		Mode string   `json:"mode"`
		At   string   `json:"at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 || body.At == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ids and at (RFC3339) are required"})
		return
	}
	at, err := time.Parse(time.RFC3339, body.At)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "at must be RFC3339"})
		return
	}
	affected, err := dc.dlqSvc.BulkSchedule(c.Request.Context(), body.IDs, body.Mode, at)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "affected": affected, "scheduled_at": at})
}

// BulkAbandon marks multiple DLQ rows as abandoned. Body: {"ids":[...]}
func (dc *DLQController) BulkAbandon(c *gin.Context) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ids array is required"})
		return
	}

	affected, err := dc.dlqSvc.BulkAbandon(c.Request.Context(), body.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "affected": affected})
}

// writeAudit inserts a row into audit_logs for DLQ actions.
// Non-fatal — errors are logged but do not affect the response.
func (dc *DLQController) writeAudit(c *gin.Context, dlqRowID, action, detail string) {
	if dc.db == nil {
		return
	}
	userID, _ := c.Get("userId")
	meta := fmt.Sprintf(`{"dlq_row_id":"%s","detail":"%s"}`, dlqRowID, detail)
	_, err := dc.db.ExecContext(c.Request.Context(), `
		INSERT INTO audit_logs
		    (user_id, action, entity_type, entity_id, metadata, dlq_row_id, created_at)
		VALUES
		    ($1, $2, 'dlq_row', $3, $4::jsonb, $5::uuid, NOW())`,
		userID, action, dlqRowID, meta, dlqRowID,
	)
	if err != nil {
		fmt.Printf("[dlq audit] write failed: %v\n", err)
	}
}
