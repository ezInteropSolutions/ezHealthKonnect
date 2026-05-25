// controllers/quality_controller.go
//
// QualityController exposes the internal transformation quality review queue.
// All endpoints require super-admin auth — this is an integration team tool,
// not a user-facing feature.
//
// Routes (registered under /api/fhir/quality):
//   GET  /flagged              — list unreviewed flagged records, newest first
//   GET  /flagged/:id          — single record detail
//   POST /flagged/:id/review   — mark reviewed + add resolution note
//   GET  /stats                — score distribution and flag counts by message type
package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type QualityController struct {
	db *sql.DB
}

func NewQualityController(db *sql.DB) *QualityController {
	return &QualityController{db: db}
}

func (qc *QualityController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/flagged", qc.ListFlagged)
	rg.GET("/flagged/:id", qc.GetFlagged)
	rg.POST("/flagged/:id/review", qc.MarkReviewed)
	rg.GET("/stats", qc.Stats)
}

// qualityRecord mirrors the transformation_quality_scores table for JSON output.
type qualityRecord struct {
	ID                   string          `json:"id"`
	MessageID            string          `json:"message_id"`
	InterfaceID          string          `json:"interface_id"`
	MessageType          string          `json:"message_type"`
	Score                float64         `json:"score"`
	ResourceCounts       json.RawMessage `json:"resource_counts"`
	ExpectedResources    []string        `json:"expected_resources"`
	MissingResources     []string        `json:"missing_resources"`
	EmptyRequiredFields  json.RawMessage `json:"empty_required_fields"`
	UnhandledSegments    []string        `json:"unhandled_segments"`
	Warnings             []string        `json:"warnings"`
	Errors               []string        `json:"errors"`
	Flagged              bool            `json:"flagged"`
	FlagReason           string          `json:"flag_reason"`
	Reviewed             bool            `json:"reviewed"`
	ReviewedBy           *string         `json:"reviewed_by"`
	ReviewedAt           *time.Time      `json:"reviewed_at"`
	Resolution           *string         `json:"resolution"`
	CreatedAt            time.Time       `json:"created_at"`
}

// ListFlagged returns unreviewed flagged records, newest first.
// Query params: limit (default 50), offset (default 0), message_type (filter).
func (qc *QualityController) ListFlagged(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	msgType := c.Query("message_type")

	query := `
		SELECT id, message_id, interface_id, message_type, score,
		       resource_counts, expected_resources, missing_resources,
		       empty_required_fields, unhandled_segments,
		       warnings, errors, flagged, flag_reason,
		       reviewed, reviewed_by, reviewed_at, resolution, created_at
		FROM transformation_quality_scores
		WHERE flagged = true AND reviewed = false`
	args := []interface{}{}
	argIdx := 1

	if msgType != "" {
		query += fmt.Sprintf(" AND message_type = $%d", argIdx)
		args = append(args, msgType)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := qc.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	records, err := scanQualityRows(rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Total unreviewed count for badge display
	var total int
	countQuery := `SELECT COUNT(*) FROM transformation_quality_scores WHERE flagged = true AND reviewed = false`
	if msgType != "" {
		countQuery += " AND message_type = $1"
		qc.db.QueryRowContext(c.Request.Context(), countQuery, msgType).Scan(&total)
	} else {
		qc.db.QueryRowContext(c.Request.Context(), countQuery).Scan(&total)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"total":   total,
		"count":   len(records),
		"records": records,
	})
}

// GetFlagged returns a single quality score record by ID.
func (qc *QualityController) GetFlagged(c *gin.Context) {
	id := c.Param("id")
	rows, err := qc.db.QueryContext(c.Request.Context(), `
		SELECT id, message_id, interface_id, message_type, score,
		       resource_counts, expected_resources, missing_resources,
		       empty_required_fields, unhandled_segments,
		       warnings, errors, flagged, flag_reason,
		       reviewed, reviewed_by, reviewed_at, resolution, created_at
		FROM transformation_quality_scores WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	records, err := scanQualityRows(rows)
	if err != nil || len(records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "record not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "record": records[0]})
}

// MarkReviewed marks a flagged record as reviewed and stores the resolution note.
// Body: { "resolution": "Updated OBX→Observation mapping in OOB template" }
func (qc *QualityController) MarkReviewed(c *gin.Context) {
	id := c.Param("id")

	var body struct {
		Resolution string `json:"resolution"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	// Extract user from session (set by Node.js proxy auth header or session)
	reviewedBy := c.GetString("userId") // populated by auth middleware if present

	var reviewedByPtr *string
	if reviewedBy != "" {
		reviewedByPtr = &reviewedBy
	}

	_, err := qc.db.ExecContext(c.Request.Context(), `
		UPDATE transformation_quality_scores
		SET reviewed = true, reviewed_by = $2, reviewed_at = NOW(), resolution = $3
		WHERE id = $1`,
		id, reviewedByPtr, body.Resolution)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Stats returns score distribution and flag counts grouped by message type.
func (qc *QualityController) Stats(c *gin.Context) {
	rows, err := qc.db.QueryContext(c.Request.Context(), `
		SELECT
			message_type,
			COUNT(*)                                             AS total,
			COUNT(*) FILTER (WHERE flagged = true)              AS flagged,
			COUNT(*) FILTER (WHERE flagged = true AND reviewed = false) AS pending_review,
			ROUND(AVG(score)::NUMERIC, 1)                       AS avg_score,
			ROUND(MIN(score)::NUMERIC, 1)                       AS min_score,
			COUNT(*) FILTER (WHERE score >= 90)                 AS score_90_plus,
			COUNT(*) FILTER (WHERE score >= 80 AND score < 90)  AS score_80_89,
			COUNT(*) FILTER (WHERE score < 80)                  AS score_below_80
		FROM transformation_quality_scores
		WHERE created_at >= NOW() - INTERVAL '7 days'
		GROUP BY message_type
		ORDER BY flagged DESC, total DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type statRow struct {
		MessageType    string  `json:"message_type"`
		Total          int     `json:"total"`
		Flagged        int     `json:"flagged"`
		PendingReview  int     `json:"pending_review"`
		AvgScore       float64 `json:"avg_score"`
		MinScore       float64 `json:"min_score"`
		Score90Plus    int     `json:"score_90_plus"`
		Score8089      int     `json:"score_80_89"`
		ScoreBelow80   int     `json:"score_below_80"`
	}
	var stats []statRow
	for rows.Next() {
		var s statRow
		if err := rows.Scan(&s.MessageType, &s.Total, &s.Flagged, &s.PendingReview,
			&s.AvgScore, &s.MinScore, &s.Score90Plus, &s.Score8089, &s.ScoreBelow80); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "stats": stats, "period": "last_7_days"})
}

// scanQualityRows reads all rows from a quality scores query.
func scanQualityRows(rows *sql.Rows) ([]qualityRecord, error) {
	var records []qualityRecord
	for rows.Next() {
		var r qualityRecord
		var resourceCountsRaw, emptyFieldsRaw []byte
		if err := rows.Scan(
			&r.ID, &r.MessageID, &r.InterfaceID, &r.MessageType, &r.Score,
			&resourceCountsRaw,
			pq.Array(&r.ExpectedResources),
			pq.Array(&r.MissingResources),
			&emptyFieldsRaw,
			pq.Array(&r.UnhandledSegments),
			pq.Array(&r.Warnings),
			pq.Array(&r.Errors),
			&r.Flagged, &r.FlagReason,
			&r.Reviewed, &r.ReviewedBy, &r.ReviewedAt, &r.Resolution,
			&r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.ResourceCounts = json.RawMessage(resourceCountsRaw)
		r.EmptyRequiredFields = json.RawMessage(emptyFieldsRaw)
		records = append(records, r)
	}
	return records, rows.Err()
}
