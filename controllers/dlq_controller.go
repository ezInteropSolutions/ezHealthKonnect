// controllers/dlq_controller.go
// Dead Letter Queue management API.
// Lists failed messages and provides replay (re-enqueue) operations.
//
// Routes (registered in main.go under /api/system/dlq):
//   GET    /api/system/dlq                      — list DLQ messages (paginated)
//   GET    /api/system/dlq/:id                  — get single DLQ entry
//   POST   /api/system/dlq/:id/replay           — re-enqueue one message
//   POST   /api/system/dlq/replay-all           — re-enqueue all unresolved (optional ?interface_id=X)
//   POST   /api/system/dlq/:id/resolve          — mark resolved without replay
//   DELETE /api/system/dlq/:id                  — hard delete (admin only)

package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// DLQController handles dead letter queue API operations.
type DLQController struct {
	db *sql.DB
}

// NewDLQController creates a new DLQ controller.
func NewDLQController(db *sql.DB) *DLQController {
	return &DLQController{db: db}
}

// RegisterRoutes mounts DLQ routes onto the provided group.
func (dc *DLQController) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("", dc.List)
	group.GET("/:id", dc.Get)
	group.POST("/:id/replay", dc.Replay)
	group.POST("/replay-all", dc.ReplayAll)
	group.POST("/:id/resolve", dc.Resolve)
	group.DELETE("/:id", dc.Delete)
}

// dlqRow mirrors the dead_letter_queue table columns returned by queries.
type dlqRow struct {
	ID             string     `json:"id"`
	InterfaceID    string     `json:"interface_id"`
	PipelineID     *string    `json:"pipeline_id"`
	MessageID      *string    `json:"message_id"`
	RawMessage     string     `json:"raw_message"`
	ErrorMessage   string     `json:"error_message"`
	ErrorStep      *string    `json:"error_step"`
	ErrorType      *string    `json:"error_type"`
	FailedAt       time.Time  `json:"failed_at"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	Resolved       bool       `json:"resolved"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolvedBy     *string    `json:"resolved_by"`
	ResolutionNote *string    `json:"resolution_note"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// List returns paginated DLQ entries.
// Query params: interface_id, resolved (true/false, default false), page, page_size
func (dc *DLQController) List(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}

	interfaceID := c.Query("interface_id")
	resolvedStr := c.DefaultQuery("resolved", "false")
	resolved, _ := strconv.ParseBool(resolvedStr)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Build query with optional interface filter
	args := []interface{}{resolved, pageSize, offset}
	whereClause := "WHERE resolved = $1"
	if interfaceID != "" {
		args = append([]interface{}{resolved, interfaceID, pageSize, offset}, args[2:]...)
		args[0] = resolved
		args[1] = interfaceID
		args[2] = pageSize
		args[3] = offset
		whereClause = "WHERE resolved = $1 AND interface_id = $2"
	}

	// Count query
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM dead_letter_queue %s", whereClause)
	countArgs := args[:len(args)-2] // remove LIMIT and OFFSET
	var total int
	if err := dc.db.QueryRowContext(c.Request.Context(), countSQL, countArgs...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Data query — parameterised LIMIT/OFFSET positions depend on whether interface_id filter used
	var dataSQL string
	if interfaceID != "" {
		dataSQL = `
			SELECT id, interface_id, pipeline_id, message_id, raw_message,
			       error_message, error_step, error_type, failed_at,
			       retry_count, max_retries, next_retry_at,
			       resolved, resolved_at, resolved_by, resolution_note,
			       created_at, updated_at
			FROM dead_letter_queue
			WHERE resolved = $1 AND interface_id = $2
			ORDER BY failed_at DESC
			LIMIT $3 OFFSET $4`
	} else {
		dataSQL = `
			SELECT id, interface_id, pipeline_id, message_id, raw_message,
			       error_message, error_step, error_type, failed_at,
			       retry_count, max_retries, next_retry_at,
			       resolved, resolved_at, resolved_by, resolution_note,
			       created_at, updated_at
			FROM dead_letter_queue
			WHERE resolved = $1
			ORDER BY failed_at DESC
			LIMIT $2 OFFSET $3`
	}

	rows, err := dc.db.QueryContext(c.Request.Context(), dataSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	var items []dlqRow
	for rows.Next() {
		var r dlqRow
		if scanErr := rows.Scan(
			&r.ID, &r.InterfaceID, &r.PipelineID, &r.MessageID, &r.RawMessage,
			&r.ErrorMessage, &r.ErrorStep, &r.ErrorType, &r.FailedAt,
			&r.RetryCount, &r.MaxRetries, &r.NextRetryAt,
			&r.Resolved, &r.ResolvedAt, &r.ResolvedBy, &r.ResolutionNote,
			&r.CreatedAt, &r.UpdatedAt,
		); scanErr != nil {
			continue
		}
		items = append(items, r)
	}
	if items == nil {
		items = []dlqRow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"pagination": gin.H{
			"page":       page,
			"page_size":  pageSize,
			"total":      total,
			"total_pages": (total + pageSize - 1) / pageSize,
		},
	})
}

// Get returns a single DLQ entry by id.
func (dc *DLQController) Get(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}
	id := c.Param("id")
	var r dlqRow
	err := dc.db.QueryRowContext(c.Request.Context(), `
		SELECT id, interface_id, pipeline_id, message_id, raw_message,
		       error_message, error_step, error_type, failed_at,
		       retry_count, max_retries, next_retry_at,
		       resolved, resolved_at, resolved_by, resolution_note,
		       created_at, updated_at
		FROM dead_letter_queue WHERE id = $1
	`, id).Scan(
		&r.ID, &r.InterfaceID, &r.PipelineID, &r.MessageID, &r.RawMessage,
		&r.ErrorMessage, &r.ErrorStep, &r.ErrorType, &r.FailedAt,
		&r.RetryCount, &r.MaxRetries, &r.NextRetryAt,
		&r.Resolved, &r.ResolvedAt, &r.ResolvedBy, &r.ResolutionNote,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "DLQ entry not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// Replay re-enqueues a single DLQ message into message_processing_queue.
// Increments retry_count and clears resolved flag. Does not delete the DLQ entry
// so the audit trail is preserved.
func (dc *DLQController) Replay(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}
	id := c.Param("id")

	var r dlqRow
	err := dc.db.QueryRowContext(c.Request.Context(), `
		SELECT id, interface_id, message_id, raw_message, error_type
		FROM dead_letter_queue WHERE id = $1 AND resolved = false
	`, id).Scan(&r.ID, &r.InterfaceID, &r.MessageID, &r.RawMessage, &r.ErrorType)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "DLQ entry not found or already resolved"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	messageID := ""
	if r.MessageID != nil {
		messageID = *r.MessageID
	}

	// Insert into message_processing_queue for retry
	queueID := generateDLQQueueID()
	_, insertErr := dc.db.ExecContext(c.Request.Context(), `
		INSERT INTO message_processing_queue (
			id, interface_id, message_id, message_data,
			message_type, status, priority, scheduled_for,
			attempts, max_attempts, created_at
		) VALUES ($1, $2, $3, $4, '', 'pending', 8, NOW(), 0, 3, NOW())
	`, queueID, r.InterfaceID, messageID, r.RawMessage)
	if insertErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": insertErr.Error()})
		return
	}

	// Mark DLQ entry as retried (increment retry_count, update updated_at)
	dc.db.ExecContext(c.Request.Context(), `
		UPDATE dead_letter_queue
		SET retry_count = retry_count + 1, next_retry_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "message re-enqueued for processing",
		"queue_id":   queueID,
		"message_id": messageID,
	})
}

// ReplayAll re-enqueues all unresolved DLQ entries (optional: filter by interface_id).
func (dc *DLQController) ReplayAll(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}
	interfaceID := c.Query("interface_id")

	// Fetch all unresolved entries
	var querySQL string
	var queryArgs []interface{}
	if interfaceID != "" {
		querySQL = `SELECT id, interface_id, message_id, raw_message FROM dead_letter_queue WHERE resolved = false AND interface_id = $1`
		queryArgs = []interface{}{interfaceID}
	} else {
		querySQL = `SELECT id, interface_id, message_id, raw_message FROM dead_letter_queue WHERE resolved = false`
	}

	rows, err := dc.db.QueryContext(c.Request.Context(), querySQL, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	queued := 0
	failed := 0
	for rows.Next() {
		var dlqID, ifaceID string
		var msgID *string
		var rawMsg string
		if scanErr := rows.Scan(&dlqID, &ifaceID, &msgID, &rawMsg); scanErr != nil {
			failed++
			continue
		}
		messageID := ""
		if msgID != nil {
			messageID = *msgID
		}
		queueID := generateDLQQueueID()
		_, insertErr := dc.db.ExecContext(c.Request.Context(), `
			INSERT INTO message_processing_queue (
				id, interface_id, message_id, message_data,
				message_type, status, priority, scheduled_for,
				attempts, max_attempts, created_at
			) VALUES ($1, $2, $3, $4, '', 'pending', 8, NOW(), 0, 3, NOW())
		`, queueID, ifaceID, messageID, rawMsg)
		if insertErr != nil {
			failed++
		} else {
			queued++
			dc.db.ExecContext(c.Request.Context(), `
				UPDATE dead_letter_queue
				SET retry_count = retry_count + 1, next_retry_at = NOW(), updated_at = NOW()
				WHERE id = $1
			`, dlqID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"queued":  queued,
		"failed":  failed,
		"message": fmt.Sprintf("%d message(s) re-enqueued, %d failed", queued, failed),
	})
}

// Resolve marks a DLQ entry as resolved without re-processing.
func (dc *DLQController) Resolve(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}
	id := c.Param("id")

	var body struct {
		ResolvedBy string `json:"resolved_by"`
		Note       string `json:"resolution_note"`
	}
	_ = c.ShouldBindJSON(&body)

	res, err := dc.db.ExecContext(c.Request.Context(), `
		UPDATE dead_letter_queue
		SET resolved = true, resolved_at = NOW(),
		    resolved_by = $2, resolution_note = $3, updated_at = NOW()
		WHERE id = $1 AND resolved = false
	`, id, body.ResolvedBy, body.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "DLQ entry not found or already resolved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "DLQ entry resolved"})
}

// Delete hard-deletes a DLQ entry (admin operation).
func (dc *DLQController) Delete(c *gin.Context) {
	if dc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "database not available"})
		return
	}
	id := c.Param("id")
	res, err := dc.db.ExecContext(c.Request.Context(), `DELETE FROM dead_letter_queue WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "DLQ entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "DLQ entry deleted"})
}

// generateDLQQueueID creates a simple unique ID for queue entries
// without importing the uuid package (keeps controller dependency-light).
func generateDLQQueueID() string {
	return fmt.Sprintf("dlq-%d", time.Now().UnixNano())
}
