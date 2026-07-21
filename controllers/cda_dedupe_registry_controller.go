// controllers/cda_dedupe_registry_controller.go
// Admin-only access + erasure endpoints for cda_dedupe_registry — the
// persistent identity table backing cda.dedupe's crossMessage mode. Separate
// file from cda_schema_controller.go (same CDASchemaController type), same
// convention already established by cda_csv_schema_controller.go and
// cda_dedupe_schema_controller.go for a different step's config UI.
//
// This table stores a patient identifier (patient_key) plus clinical identity
// data (identity_key: codes + dates) indefinitely unless purged — PHI, not
// generic operational metadata. Confirmed by repo-wide grep before this file
// was added: nothing previously exposed this table to any UI or API; the
// only way to inspect or delete a row was direct SQL access.
//
//   GET    /api/cda/dedupe/registry   → list rows for one interface (+ optional patientKey)
//   DELETE /api/cda/dedupe/registry   → purge every row for one interface+patientKey
//
// Both require admin/super_admin (requireAdminRole) and are audit-logged —
// HIPAA §164.312(b) requires recording activity involving ePHI, and GDPR
// Art. 17 (erasure) / Art. 15 (access) both need a traceable record of who
// acted on a data subject's behalf and why. Deliberately NOT logging every
// automatic in-band suppression decision here — that's routine processing,
// already traced more cheaply via the registry row's own first/last-seen
// columns and the per-message step-output history every pipeline step gets;
// logging per-suppression would reintroduce the unbounded-growth problem
// this design otherwise avoids.
package controllers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// requireAdminRole restricts an endpoint to admin/super_admin users. Mirrors
// AIController.requireAdminRole exactly. Defense-in-depth: the Node.js proxy
// (app.js) also gates this specific route with requireAuth+requireAdmin
// before forwarding — this check exists in case Go is ever reached directly
// or the proxy layer's gate is misconfigured.
func (cc *CDASchemaController) requireAdminRole() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		role := ctx.GetHeader("X-User-Role")
		if role != "admin" && role != "super_admin" {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin role required",
			})
			return
		}
		ctx.Next()
	}
}

// dedupeRegistryRowResponse is the wire shape of one cda_dedupe_registry row.
type dedupeRegistryRowResponse struct {
	ID                 string `json:"id"`
	InterfaceID        string `json:"interfaceId"`
	PatientKey         string `json:"patientKey"`
	SectionKey         string `json:"sectionKey"`
	IdentityKey        string `json:"identityKey"`
	FirstSeenMessageID string `json:"firstSeenMessageId,omitempty"`
	LastSeenMessageID  string `json:"lastSeenMessageId,omitempty"`
	FirstSeenAt        string `json:"firstSeenAt"`
	LastSeenAt         string `json:"lastSeenAt"`
	SeenCount          int    `json:"seenCount"`
}

// GetDedupeRegistry lists cda_dedupe_registry rows for one interface
// (required — an unscoped full-table query of PHI is never allowed) and,
// optionally, one patient. Every call writes a CDA_DEDUPE_REGISTRY_VIEWED
// audit_logs row: viewing this table is itself an access event HIPAA
// §164.312(b) requires recording.
func (cc *CDASchemaController) GetDedupeRegistry(c *gin.Context) {
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interfaceId is required",
		})
		return
	}
	patientKey := c.Query("patientKey")

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	query := `
		SELECT id, interface_id, patient_key, section_key, identity_key,
		       COALESCE(first_seen_message_id, ''), COALESCE(last_seen_message_id, ''),
		       first_seen_at, last_seen_at, seen_count
		FROM cda_dedupe_registry
		WHERE interface_id = $1`
	args := []interface{}{interfaceID}
	argN := 2
	if patientKey != "" {
		query += " AND patient_key = $" + strconv.Itoa(argN)
		args = append(args, patientKey)
		argN++
	}
	query += " ORDER BY last_seen_at DESC LIMIT $" + strconv.Itoa(argN) + " OFFSET $" + strconv.Itoa(argN+1)
	args = append(args, limit, offset)

	rows, err := cc.db.Query(query, args...)
	if err != nil {
		log.Printf("⚠️  GetDedupeRegistry query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to query dedupe registry"})
		return
	}
	defer rows.Close()

	results := make([]dedupeRegistryRowResponse, 0, limit)
	for rows.Next() {
		var r dedupeRegistryRowResponse
		var firstSeenAt, lastSeenAt time.Time
		if err := rows.Scan(
			&r.ID, &r.InterfaceID, &r.PatientKey, &r.SectionKey, &r.IdentityKey,
			&r.FirstSeenMessageID, &r.LastSeenMessageID,
			&firstSeenAt, &lastSeenAt, &r.SeenCount,
		); err != nil {
			continue
		}
		r.FirstSeenAt = firstSeenAt.Format(time.RFC3339)
		r.LastSeenAt = lastSeenAt.Format(time.RFC3339)
		results = append(results, r)
	}

	// Audit the view itself — actor from X-User-ID (forwarded by the Node
	// proxy from the authenticated session), never blocking the response on
	// a logging failure.
	userID := c.GetHeader("X-User-ID")
	metadata, _ := json.Marshal(map[string]interface{}{
		"interface_id":        interfaceID,
		"patient_key":         patientKey,
		"row_count_returned":  len(results),
	})
	if _, err := cc.db.Exec(`
		INSERT INTO audit_logs (id, user_id, action, entity_type, entity_id, metadata, result, risk_level, created_at)
		VALUES (gen_random_uuid(), NULLIF($1, '')::uuid, 'CDA_DEDUPE_REGISTRY_VIEWED', 'cda_dedupe_registry', $2, $3::jsonb, 'success', 'medium', NOW())
	`, userID, interfaceID, string(metadata)); err != nil {
		log.Printf("⚠️  Failed to write audit log for dedupe registry view: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"count":   len(results),
	})
}

// purgeDedupeRegistryRequest is the body for PurgeDedupeRegistry. Reason is
// required — a GDPR erasure action performed by an administrator on a data
// subject's behalf should always carry a stated reason for the compliance
// record, not just a bare delete.
type purgeDedupeRegistryRequest struct {
	InterfaceID string `json:"interfaceId"`
	PatientKey  string `json:"patientKey"`
	Reason      string `json:"reason"`
}

// PurgeDedupeRegistry deletes every cda_dedupe_registry row for one patient
// on one interface — the GDPR Art. 17 (right to erasure) path for this
// table, since nothing else in this codebase can remove these rows short of
// direct SQL. Writes exactly ONE audit_logs row summarizing the purge (not
// one per row deleted) — deleting PHI is a high-risk action that must be
// traceable to who did it, for whom, and why, without itself becoming a
// second unbounded PHI log.
func (cc *CDASchemaController) PurgeDedupeRegistry(c *gin.Context) {
	var req purgeDedupeRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.InterfaceID == "" || req.PatientKey == "" || req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "interfaceId, patientKey, and reason are all required",
		})
		return
	}

	res, err := cc.db.Exec(`
		DELETE FROM cda_dedupe_registry
		WHERE interface_id = $1 AND patient_key = $2
	`, req.InterfaceID, req.PatientKey)
	if err != nil {
		log.Printf("⚠️  PurgeDedupeRegistry delete failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to purge dedupe registry"})
		return
	}
	rowsDeleted, _ := res.RowsAffected()

	userID := c.GetHeader("X-User-ID")
	metadata, _ := json.Marshal(map[string]interface{}{
		"interface_id": req.InterfaceID,
		"reason":       req.Reason,
		"rows_deleted": rowsDeleted,
	})
	if _, err := cc.db.Exec(`
		INSERT INTO audit_logs (id, user_id, action, entity_type, entity_id, metadata, result, risk_level, created_at)
		VALUES (gen_random_uuid(), NULLIF($1, '')::uuid, 'CDA_DEDUPE_REGISTRY_PURGED', 'cda_dedupe_registry', $2, $3::jsonb, 'success', 'high', NOW())
	`, userID, req.PatientKey, string(metadata)); err != nil {
		log.Printf("⚠️  Failed to write audit log for dedupe registry purge: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"rowsDeleted": rowsDeleted,
	})
}
