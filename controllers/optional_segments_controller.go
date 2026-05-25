// controllers/optional_segments_controller.go
//
// REST endpoints for the Optional Segment Blocks feature.
//
// GET  /api/fhir/optional-segments
//      Query params: messageType (required), interfaceId (optional)
//      Returns the registry blocks for the message type with each block's
//      current enabled state for the interface.
//
// PATCH /api/fhir/optional-segments/:interfaceId/:messageType
//      Body: {"PV1_Encounter": true, "SPM_Specimen": false}
//      Merges the toggle state into custom_mapping_config["optional_segments"].
//      Also accepts "resource_policy" key in the same request body so the
//      mapping editor can save both in one round-trip.
package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"ezhealthkonnect/services/manifest"
)

// OptionalSegmentsController handles listing and persisting optional segment
// block toggle states.
type OptionalSegmentsController struct {
	db *sql.DB
}

func NewOptionalSegmentsController(db *sql.DB) *OptionalSegmentsController {
	return &OptionalSegmentsController{db: db}
}

func (c *OptionalSegmentsController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/optional-segments", c.List)
	rg.PATCH("/optional-segments/:interfaceId/:messageType", c.Save)
}

// BlockDTO is the response shape for one optional segment block.
type BlockDTO struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// List returns the available optional segment blocks for a message type,
// with the enabled state loaded from the interface's custom_mapping_config.
//
// GET /api/fhir/optional-segments?messageType=ORU%5ER01&interfaceId=<uuid>
func (ctrl *OptionalSegmentsController) List(c *gin.Context) {
	messageType := c.Query("messageType")
	if messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "messageType is required"})
		return
	}
	interfaceID := c.Query("interfaceId")

	blocks := manifest.GetAvailableBlocks(messageType)

	// Load the current enabled state for this interface (if provided).
	var optCfg map[string]bool
	if interfaceID != "" && ctrl.db != nil {
		optCfg = ctrl.loadOptionalSegmentConfig(c.Request.Context(), interfaceID, messageType)
	}

	dtos := make([]BlockDTO, len(blocks))
	for i, b := range blocks {
		dtos[i] = BlockDTO{
			Key:         b.Key,
			DisplayName: b.DisplayName,
			Description: b.Description,
			Enabled:     manifest.IsBlockEnabled(optCfg, b.Key),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"messageType": messageType,
		"blocks":      dtos,
	})
}

// Save merges optional segment toggle states (and optionally resource_policy)
// into custom_mapping_config for the given interface + message type row.
//
// PATCH /api/fhir/optional-segments/:interfaceId/:messageType
// Body: {"PV1_Encounter": true, "SPM_Specimen": false}
// Optional combined body: {"optional_segments": {...}, "resource_policy": {...}}
func (ctrl *OptionalSegmentsController) Save(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	messageType := c.Param("messageType")

	if interfaceID == "" || messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId and messageType are required"})
		return
	}

	// Accept two body shapes:
	// Shape A (simple): {"PV1_Encounter": true, "SPM_Specimen": false}
	// Shape B (combined): {"optional_segments": {...}, "resource_policy": {...}}
	var rawBody map[string]interface{}
	if err := c.ShouldBindJSON(&rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid JSON body"})
		return
	}

	// Detect shape B by checking for "optional_segments" or "resource_policy" keys.
	var newOptSegments map[string]bool
	var newResourcePolicy map[string]string

	if combined, ok := rawBody["optional_segments"]; ok {
		newOptSegments = toStringBoolMap(combined)
	}
	if rp, ok := rawBody["resource_policy"]; ok {
		newResourcePolicy = toStringStringMap(rp)
	}
	// Shape A: flat bool map (no "optional_segments" wrapper)
	if newOptSegments == nil && newResourcePolicy == nil {
		newOptSegments = make(map[string]bool)
		for k, v := range rawBody {
			if b, ok := v.(bool); ok {
				newOptSegments[k] = b
			}
		}
	}

	if err := ctrl.mergeConfig(c.Request.Context(), interfaceID, messageType, newOptSegments, newResourcePolicy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "optional segment config saved"})
}

// ──────────────────────────────────────────────────────────────────────────────
// Private helpers
// ──────────────────────────────────────────────────────────────────────────────

func (ctrl *OptionalSegmentsController) loadOptionalSegmentConfig(
	ctx context.Context, interfaceID, messageType string,
) map[string]bool {
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
		  AND custom_mapping_config IS NOT NULL
		LIMIT 1
	`
	var configBytes []byte
	if err := ctrl.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&configBytes); err != nil {
		return nil
	}
	return manifest.LoadOptionalSegmentConfig(configBytes)
}

// mergeConfig reads the existing custom_mapping_config, applies the new
// optional_segments and resource_policy values, and upserts the row.
func (ctrl *OptionalSegmentsController) mergeConfig(
	ctx context.Context,
	interfaceID, messageType string,
	newOptSegments map[string]bool,
	newResourcePolicy map[string]string,
) error {
	// Read existing config so we preserve atomicMappings and other keys.
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
		LIMIT 1
	`
	var existing []byte
	_ = ctrl.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&existing)

	// Decode or start fresh.
	cfg := map[string]interface{}{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &cfg)
	}

	// Merge optional_segments.
	if newOptSegments != nil {
		current := map[string]interface{}{}
		if prev, ok := cfg["optional_segments"].(map[string]interface{}); ok {
			current = prev
		}
		for k, v := range newOptSegments {
			current[k] = v
		}
		cfg["optional_segments"] = current
	}

	// Merge resource_policy.
	if newResourcePolicy != nil {
		current := map[string]interface{}{}
		if prev, ok := cfg["resource_policy"].(map[string]interface{}); ok {
			current = prev
		}
		for k, v := range newResourcePolicy {
			current[k] = v
		}
		cfg["resource_policy"] = current
	}

	updated, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	upsert := `
		INSERT INTO interface_message_mappings (interface_id, message_type, custom_mapping_config, uses_standard_template, created_at, updated_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		ON CONFLICT (interface_id, message_type)
		DO UPDATE SET
			custom_mapping_config = $3,
			updated_at = NOW()
	`
	_, err = ctrl.db.ExecContext(ctx, upsert, interfaceID, messageType, updated)
	return err
}

// toStringBoolMap coerces a decoded JSON value to map[string]bool.
func toStringBoolMap(v interface{}) map[string]bool {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]bool, len(m))
	for k, val := range m {
		if b, ok := val.(bool); ok {
			out[k] = b
		}
	}
	return out
}

// toStringStringMap coerces a decoded JSON value to map[string]string.
func toStringStringMap(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}
