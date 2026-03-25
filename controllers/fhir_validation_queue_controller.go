// controllers/fhir_validation_queue_controller.go
// FHIR Validation Review Queue API.
//
// Routes (registered in main.go under /api/fhir/validation-queue):
//   GET  /api/fhir/validation-queue              — list items (paginated, filterable)
//   GET  /api/fhir/validation-queue/:id          — get single item
//   PUT  /api/fhir/validation-queue/:id/review   — mark in_review (optimistic claim)
//   PUT  /api/fhir/validation-queue/:id/resolve  — resolve with provided values
//   PUT  /api/fhir/validation-queue/:id/override — deliver as-is (override)
//   PUT  /api/fhir/validation-queue/:id/reject   — reject → DLQ
//   GET  /api/fhir/validation-queue/stats        — summary counts

package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// FHIRValidationQueueController handles review queue operations.
type FHIRValidationQueueController struct {
	db *sql.DB
}

// NewFHIRValidationQueueController creates a new controller instance.
func NewFHIRValidationQueueController(db *sql.DB) *FHIRValidationQueueController {
	return &FHIRValidationQueueController{db: db}
}

// RegisterRoutes mounts routes on the provided group.
func (vc *FHIRValidationQueueController) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("", vc.List)
	group.GET("/stats", vc.Stats)
	group.GET("/:id", vc.Get)
	group.PUT("/:id/review", vc.MarkInReview)
	group.PUT("/:id/resolve", vc.Resolve)
	group.PUT("/:id/override", vc.Override)
	group.PUT("/:id/reject", vc.Reject)
}

// ─── Row model ────────────────────────────────────────────────────────────────

type ValidationQueueItem struct {
	ID                 string          `json:"id"`
	InterfaceID        string          `json:"interface_id"`
	InterfaceName      string          `json:"interface_name"`
	MessageID          *string         `json:"message_id"`
	MessageType        *string         `json:"message_type"`
	RawMessage         *string         `json:"raw_message"`
	PartialFHIR        json.RawMessage `json:"partial_fhir"`
	ValidationFailures json.RawMessage `json:"validation_failures"`
	Status             string          `json:"status"`
	Resolution         json.RawMessage `json:"resolution"`
	AssignedTo         *string         `json:"assigned_to"`
	Notes              *string         `json:"notes"`
	CreatedAt          time.Time       `json:"queued_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// ─── List ─────────────────────────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) List(c *gin.Context) {
	status := c.Query("status")   // optional filter
	limit  := 200
	offset := 0
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = l
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}

	query := `
		SELECT
			q.id, q.interface_id,
			COALESCE(i.name, q.interface_id::text) AS interface_name,
			q.message_id, q.message_type,
			q.raw_hl7, q.partial_fhir,
			q.validation_failures, q.status,
			q.resolution, q.assigned_to, q.notes,
			q.created_at, q.updated_at
		FROM fhir_validation_queue q
		LEFT JOIN interfaces i ON i.id = q.interface_id`

	args := []interface{}{}
	if status != "" && status != "all" {
		query += " WHERE q.status = $1"
		args = append(args, status)
		query += " ORDER BY q.created_at DESC LIMIT $2 OFFSET $3"
		args = append(args, limit, offset)
	} else {
		query += " ORDER BY q.created_at DESC LIMIT $1 OFFSET $2"
		args = append(args, limit, offset)
	}

	rows, err := vc.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	items := []ValidationQueueItem{}
	for rows.Next() {
		var it ValidationQueueItem
		var rawHL7, partialFHIR, validationFailures, resolution []byte
		err := rows.Scan(
			&it.ID, &it.InterfaceID, &it.InterfaceName,
			&it.MessageID, &it.MessageType,
			&rawHL7, &partialFHIR,
			&validationFailures, &it.Status,
			&resolution, &it.AssignedTo, &it.Notes,
			&it.CreatedAt, &it.UpdatedAt,
		)
		if err != nil {
			continue
		}
		if rawHL7 != nil {
			s := string(rawHL7)
			it.RawMessage = &s
		}
		if partialFHIR != nil {
			it.PartialFHIR = json.RawMessage(partialFHIR)
		}
		if validationFailures != nil {
			it.ValidationFailures = json.RawMessage(validationFailures)
		}
		if resolution != nil {
			it.Resolution = json.RawMessage(resolution)
		}
		items = append(items, it)
	}

	c.JSON(http.StatusOK, gin.H{"data": items, "count": len(items)})
}

// ─── Get single ───────────────────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) Get(c *gin.Context) {
	id := c.Param("id")
	query := `
		SELECT
			q.id, q.interface_id,
			COALESCE(i.name, q.interface_id::text) AS interface_name,
			q.message_id, q.message_type,
			q.raw_hl7, q.partial_fhir,
			q.validation_failures, q.status,
			q.resolution, q.assigned_to, q.notes,
			q.created_at, q.updated_at
		FROM fhir_validation_queue q
		LEFT JOIN interfaces i ON i.id = q.interface_id
		WHERE q.id = $1`

	var it ValidationQueueItem
	var rawHL7, partialFHIR, validationFailures, resolution []byte
	err := vc.db.QueryRowContext(c.Request.Context(), query, id).Scan(
		&it.ID, &it.InterfaceID, &it.InterfaceName,
		&it.MessageID, &it.MessageType,
		&rawHL7, &partialFHIR,
		&validationFailures, &it.Status,
		&resolution, &it.AssignedTo, &it.Notes,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rawHL7 != nil {
		s := string(rawHL7)
		it.RawMessage = &s
	}
	if partialFHIR != nil {
		it.PartialFHIR = json.RawMessage(partialFHIR)
	}
	if validationFailures != nil {
		it.ValidationFailures = json.RawMessage(validationFailures)
	}
	if resolution != nil {
		it.Resolution = json.RawMessage(resolution)
	}

	c.JSON(http.StatusOK, it)
}

// ─── Mark in_review ───────────────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) MarkInReview(c *gin.Context) {
	id := c.Param("id")
	_, err := vc.db.ExecContext(c.Request.Context(),
		`UPDATE fhir_validation_queue SET status = 'in_review', updated_at = NOW()
		 WHERE id = $1 AND status = 'pending_review'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Resolve (with provided values) ──────────────────────────────────────────

func (vc *FHIRValidationQueueController) Resolve(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Overrides     map[string]string `json:"overrides"`
		Notes         string            `json:"notes"`
		Resolution    string            `json:"resolution"`     // e.g. "provided"
		ProvidedValues map[string]string `json:"provided_values"` // alias
		ReviewerNotes string            `json:"reviewer_notes"`
	}
	_ = c.ShouldBindJSON(&body)

	// merge aliases
	if body.ProvidedValues != nil && body.Overrides == nil {
		body.Overrides = body.ProvidedValues
	}
	notes := body.Notes
	if notes == "" {
		notes = body.ReviewerNotes
	}

	resolution := map[string]interface{}{
		"action":      "provide_values",
		"overrides":   body.Overrides,
		"resolved_by": resolvedByFromCtx(c),
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
		"notes":       notes,
	}
	resJSON, _ := json.Marshal(resolution)

	_, err := vc.db.ExecContext(c.Request.Context(),
		`UPDATE fhir_validation_queue
		 SET status = 'resolved_provided', resolution = $2, notes = $3, updated_at = NOW()
		 WHERE id = $1`, id, string(resJSON), notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Override (deliver as-is) ─────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) Override(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Notes string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&body)

	resolution := map[string]interface{}{
		"action":      "override",
		"resolved_by": resolvedByFromCtx(c),
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
		"notes":       body.Notes,
	}
	resJSON, _ := json.Marshal(resolution)

	_, err := vc.db.ExecContext(c.Request.Context(),
		`UPDATE fhir_validation_queue
		 SET status = 'resolved_override', resolution = $2, notes = $3, updated_at = NOW()
		 WHERE id = $1`, id, string(resJSON), body.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Reject ───────────────────────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) Reject(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Notes string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&body)

	resolution := map[string]interface{}{
		"action":      "reject",
		"resolved_by": resolvedByFromCtx(c),
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
		"notes":       body.Notes,
	}
	resJSON, _ := json.Marshal(resolution)

	_, err := vc.db.ExecContext(c.Request.Context(),
		`UPDATE fhir_validation_queue
		 SET status = 'resolved_rejected', resolution = $2, notes = $3, updated_at = NOW()
		 WHERE id = $1`, id, string(resJSON), body.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func (vc *FHIRValidationQueueController) Stats(c *gin.Context) {
	row := vc.db.QueryRowContext(c.Request.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending_review')  AS pending,
			COUNT(*) FILTER (WHERE status = 'in_review')       AS in_review,
			COUNT(*) FILTER (WHERE status LIKE 'resolved%')    AS resolved,
			COUNT(*)                                           AS total
		FROM fhir_validation_queue`)

	var pending, inReview, resolved, total int
	if err := row.Scan(&pending, &inReview, &resolved, &total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"pending":   pending,
		"in_review": inReview,
		"resolved":  resolved,
		"total":     total,
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func resolvedByFromCtx(c *gin.Context) string {
	// Try to extract user identity from session/JWT claims set by middleware
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(map[string]interface{}); ok {
			if email, ok := u["email"].(string); ok && email != "" {
				return email
			}
			if username, ok := u["username"].(string); ok && username != "" {
				return username
			}
		}
	}
	return "unknown"
}
