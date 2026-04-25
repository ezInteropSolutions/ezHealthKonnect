package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"ezhealthkonnect/services"
)

// MappingDeltaController handles the delta/override model for interface-level
// mapping customisations.  Interfaces that use an OOB template can store a
// *sparse* set of overrides (replace / add / remove) rather than a full copy.
type MappingDeltaController struct {
	db          *sql.DB
	transformSvc *services.HL7FHIRTransformServiceV3
}

// NewMappingDeltaController wires up the controller with a DB handle and the v3 service.
func NewMappingDeltaController(db *sql.DB, svc *services.HL7FHIRTransformServiceV3) *MappingDeltaController {
	return &MappingDeltaController{db: db, transformSvc: svc}
}

// RegisterRoutes mounts the delta endpoints onto the provided router group.
// Expected prefix: /api/fhir
//
//	PUT    /api/fhir/interfaces/:interfaceId/mapping-delta/:messageType
//	GET    /api/fhir/interfaces/:interfaceId/effective-mapping/:messageType
//	DELETE /api/fhir/interfaces/:interfaceId/mapping-update-notice/:messageType
//	GET    /api/fhir/mapping-updates                                          (all pending)
func (mc *MappingDeltaController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.PUT("/interfaces/:interfaceId/mapping-delta/:messageType", mc.SaveMappingDelta)
	rg.GET("/interfaces/:interfaceId/effective-mapping/:messageType", mc.GetEffectiveMapping)
	rg.DELETE("/interfaces/:interfaceId/mapping-update-notice/:messageType", mc.DismissUpdateNotice)
	rg.GET("/mapping-updates", mc.ListPendingMappingUpdates)
}

// saveDeltaRequest is the body expected by SaveMappingDelta.
type saveDeltaRequest struct {
	AtomicMappings []services.AtomicMapping `json:"atomicMappings"`
}

// SaveMappingDelta diffs the wizard's atomicMappings against the OOB template for
// the given messageType and persists only the sparse delta to
// interface_message_mappings.mapping_overrides.
//
// If the wizard mappings are identical to OOB the delta is cleared (mapping_overrides
// set to NULL) so the runtime falls through to the plain OOB path at no extra cost.
func (mc *MappingDeltaController) SaveMappingDelta(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	messageType := c.Param("messageType")

	if interfaceID == "" || messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and messageType are required"})
		return
	}

	var req saveDeltaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body: " + err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Compute the delta against the OOB template.
	delta, templateID, err := mc.transformSvc.ComputeDelta(ctx, messageType, req.AtomicMappings)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error":   fmt.Sprintf("delta computation failed: %s", err.Error()),
		})
		return
	}

	// Persist: always ensure the row exists with uses_standard_template = true.
	// Saving (whether pure OOB or custom) always clears any pending update notice —
	// the user has explicitly reviewed and saved the current state.
	if delta == nil {
		// No overrides — revert to pure OOB, clear delta and any update notice.
		_, err = mc.db.ExecContext(ctx, `
			INSERT INTO interface_message_mappings
				(interface_id, message_type, uses_standard_template, standard_template_id,
				 mapping_overrides, template_update_available, available_template_id, available_template_version)
			VALUES ($1, $2, true, $3, NULL, false, NULL, NULL)
			ON CONFLICT (interface_id, message_type) DO UPDATE SET
				uses_standard_template    = true,
				standard_template_id      = EXCLUDED.standard_template_id,
				mapping_overrides         = NULL,
				template_update_available = false,
				available_template_id     = NULL,
				available_template_version = NULL,
				updated_at                = CURRENT_TIMESTAMP
		`, interfaceID, messageType, templateID)
	} else {
		deltaJSON, jsonErr := json.Marshal(delta)
		if jsonErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to serialise delta"})
			return
		}
		_, err = mc.db.ExecContext(ctx, `
			INSERT INTO interface_message_mappings
				(interface_id, message_type, uses_standard_template, standard_template_id,
				 mapping_overrides, template_update_available, available_template_id, available_template_version)
			VALUES ($1, $2, true, $3, $4, false, NULL, NULL)
			ON CONFLICT (interface_id, message_type) DO UPDATE SET
				uses_standard_template    = true,
				standard_template_id      = EXCLUDED.standard_template_id,
				mapping_overrides         = EXCLUDED.mapping_overrides,
				template_update_available = false,
				available_template_id     = NULL,
				available_template_version = NULL,
				updated_at                = CURRENT_TIMESTAMP
		`, interfaceID, messageType, templateID, deltaJSON)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to save delta: " + err.Error()})
		return
	}

	overrideCount := 0
	if delta != nil {
		overrideCount = len(delta.Overrides)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"interfaceId":     interfaceID,
		"messageType":     messageType,
		"templateId":      templateID,
		"overrideCount":   overrideCount,
		"isPureOOB":       delta == nil,
	})
}

// ListPendingMappingUpdates returns all interface mappings that have a template
// update available, grouped by interface ID.
// Response: { success, updates: { [interfaceId]: [{messageType, availableVersion}] } }
func (mc *MappingDeltaController) ListPendingMappingUpdates(c *gin.Context) {
	rows, err := mc.db.QueryContext(c.Request.Context(), `
		SELECT interface_id::text, message_type, COALESCE(available_template_version, '')
		FROM   interface_message_mappings
		WHERE  template_update_available = true
		ORDER  BY interface_id, message_type
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type updateEntry struct {
		MessageType      string `json:"messageType"`
		AvailableVersion string `json:"availableVersion"`
	}
	updates := make(map[string][]updateEntry)
	for rows.Next() {
		var ifaceID, msgType, version string
		if err := rows.Scan(&ifaceID, &msgType, &version); err != nil {
			continue
		}
		updates[ifaceID] = append(updates[ifaceID], updateEntry{MessageType: msgType, AvailableVersion: version})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "updates": updates})
}

// DismissUpdateNotice clears the template_update_available flag on a mapping row.
// Called when the user chooses "Keep my custom" or after applying the update.
func (mc *MappingDeltaController) DismissUpdateNotice(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	messageType := c.Param("messageType")

	if interfaceID == "" || messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and messageType are required"})
		return
	}

	_, err := mc.db.ExecContext(c.Request.Context(), `
		UPDATE interface_message_mappings
		SET    template_update_available  = false,
		       available_template_id      = NULL,
		       available_template_version = NULL
		WHERE  interface_id = $1 AND message_type = $2
	`, interfaceID, messageType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetEffectiveMapping returns the merged (OOB + delta overrides) atomicMappings for
// a given interface + messageType.  The wizard uses this to pre-populate its mapping
// screen with the interface's current effective configuration.
func (mc *MappingDeltaController) GetEffectiveMapping(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	messageType := c.Param("messageType")

	if interfaceID == "" || messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and messageType are required"})
		return
	}

	ctx := c.Request.Context()

	// Build a minimal TransformRequest so getFieldMappings can use the delta path.
	req := &services.TransformRequest{
		InterfaceID: interfaceID,
		MessageType: messageType,
	}

	fieldMappings, _, err := mc.transformSvc.GetFieldMappingsPublic(ctx, messageType, "", req)
	if err != nil || len(fieldMappings) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no mappings found for %s/%s", interfaceID, messageType),
		})
		return
	}

	atomicMappings := mc.transformSvc.ConvertToAtomicMappingsPublic(fieldMappings)

	// Also fetch metadata: overrides, update availability.
	var overrideCount int
	var templateID sql.NullString
	var updateAvailable bool
	var availableVersion sql.NullString
	mc.db.QueryRowContext(ctx, `
		SELECT standard_template_id,
		       COALESCE(jsonb_array_length(mapping_overrides->'overrides'), 0),
		       COALESCE(template_update_available, false),
		       available_template_version
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
	`, interfaceID, messageType).Scan(&templateID, &overrideCount, &updateAvailable, &availableVersion)

	c.JSON(http.StatusOK, gin.H{
		"success":                  true,
		"interfaceId":              interfaceID,
		"messageType":              messageType,
		"templateId":               templateID.String,
		"overrideCount":            overrideCount,
		"isPureOOB":                overrideCount == 0,
		"templateUpdateAvailable":  updateAvailable,
		"availableTemplateVersion": availableVersion.String,
		"atomicMappings":           atomicMappings,
		"count":                    len(atomicMappings),
	})
}
