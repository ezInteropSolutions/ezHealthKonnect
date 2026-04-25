// controllers/zsegment_controller.go
//
// REST API for Z-segment mapping configuration.
//
// Routes (all prefixed /api/zsegments):
//
//	GET    /catalog/transforms                         → transform catalog for UI dropdowns
//
//	GET    /templates                                  → list all templates (optionally ?segmentId=ZEM)
//	POST   /templates                                  → create user template
//	GET    /templates/:templateId                      → get template detail
//	DELETE /templates/:templateId                      → delete user template (system templates protected)
//
//	GET    /interfaces/:interfaceId/mfn-config         → list MFI.1 overrides
//	PUT    /interfaces/:interfaceId/mfn-config         → upsert a single MFI.1 override
//	DELETE /interfaces/:interfaceId/mfn-config/:id     → delete a MFI.1 override
//
//	GET    /interfaces/:interfaceId                    → list Z-segment configs (with fields)
//	POST   /interfaces/:interfaceId                    → create Z-segment config (with optional fields)
//	GET    /interfaces/:interfaceId/:configId          → get single config
//	PUT    /interfaces/:interfaceId/:configId          → update config metadata
//	DELETE /interfaces/:interfaceId/:configId          → delete config (cascades fields)
//
//	POST   /interfaces/:interfaceId/apply-template     → apply template → new config
//	POST   /interfaces/:interfaceId/:configId/save-as-template → save config as template
//
//	PUT    /interfaces/:interfaceId/:configId/fields   → upsert a field mapping
//	DELETE /interfaces/:interfaceId/:configId/fields/:fieldId → delete a field mapping
//
//	POST   /detect                                     → detect Z-segments in a raw message
//
//	GET    /interfaces/:interfaceId/runtime-config     → runtime config for assembly
package controllers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// extractUserIDFromToken parses the Bearer JWT from the Authorization header
// and returns the userId claim.  Returns "" if the token is absent or malformed.
// This is a read-only decode (no signature verification) — the Node.js layer
// already verified the token before forwarding to Go.
func extractUserIDFromToken(ctx *gin.Context) string {
	auth := ctx.GetHeader("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	parts := strings.SplitN(auth[7:], ".", 3)
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if uid, ok := claims["userId"].(string); ok {
		return uid
	}
	return ""
}

// callerUserID returns the authenticated user's ID.
// Priority: gin context "userId" key → JWT Bearer claim → X-User-ID proxy header.
func callerUserID(ctx *gin.Context) string {
	if v, exists := ctx.Get("userId"); exists {
		if uid, ok := v.(string); ok && uid != "" {
			return uid
		}
	}
	if uid := extractUserIDFromToken(ctx); uid != "" {
		return uid
	}
	return ctx.GetHeader("X-User-ID")
}

// ZSegmentController handles all Z-segment mapping endpoints.
type ZSegmentController struct {
	svc *services.ZSegmentService
}

// NewZSegmentController constructs the controller.
func NewZSegmentController(svc *services.ZSegmentService) *ZSegmentController {
	return &ZSegmentController{svc: svc}
}

// RegisterRoutes wires all routes under the given router group.
// Call with the /api/zsegments group from main.go.
func (c *ZSegmentController) RegisterRoutes(rg *gin.RouterGroup) {
	// Catalog
	rg.GET("/catalog/transforms", c.GetTransformCatalog)

	// Templates
	rg.GET("/templates", c.ListTemplates)
	rg.POST("/templates", c.CreateTemplate)
	rg.GET("/templates/:templateId", c.GetTemplate)
	rg.DELETE("/templates/:templateId", c.DeleteTemplate)

	// Per-interface MFI.1 overrides
	iface := rg.Group("/interfaces/:interfaceId")
	iface.GET("/mfn-config", c.GetMFNConfig)
	iface.PUT("/mfn-config", c.UpsertMFNConfig)
	iface.DELETE("/mfn-config/:configId", c.DeleteMFNConfig)

	// Per-interface Z-segment configs
	iface.GET("", c.ListZSegmentConfigs)
	iface.POST("", c.CreateZSegmentConfig)
	iface.GET("/:configId", c.GetZSegmentConfig)
	iface.PUT("/:configId", c.UpdateZSegmentConfig)
	iface.DELETE("/:configId", c.DeleteZSegmentConfig)

	// Template apply / save-as
	iface.POST("/apply-template", c.ApplyTemplate)
	iface.POST("/:configId/save-as-template", c.SaveAsTemplate)

	// Field mappings
	iface.PUT("/:configId/fields", c.UpsertFieldMapping)
	iface.DELETE("/:configId/fields/:fieldId", c.DeleteFieldMapping)

	// Detection utility
	rg.POST("/detect", c.DetectZSegments)

	// Runtime config (used by assembly pipeline)
	iface.GET("/runtime-config", c.GetRuntimeConfig)
}

// ─────────────────────────────────────────────────────────────────────────────
// Catalog
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) GetTransformCatalog(ctx *gin.Context) {
	transforms, err := c.svc.GetTransformCatalog()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": transforms})
}

// ─────────────────────────────────────────────────────────────────────────────
// Templates
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) ListTemplates(ctx *gin.Context) {
	segmentID := strings.ToUpper(ctx.Query("segmentId"))
	templates, err := c.svc.ListTemplates(segmentID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": templates, "count": len(templates)})
}

func (c *ZSegmentController) GetTemplate(ctx *gin.Context) {
	tmpl, err := c.svc.GetTemplate(ctx.Param("templateId"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if tmpl == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "template not found"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": tmpl})
}

func (c *ZSegmentController) CreateTemplate(ctx *gin.Context) {
	var req models.CreateZSegmentTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	userID := callerUserID(ctx)
	tmpl, err := c.svc.CreateTemplate(req, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"success": true, "data": tmpl})
}

func (c *ZSegmentController) DeleteTemplate(ctx *gin.Context) {
	if err := c.svc.DeleteTemplate(ctx.Param("templateId")); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// MFI.1 overrides
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) GetMFNConfig(ctx *gin.Context) {
	configs, err := c.svc.GetMFNConfig(ctx.Param("interfaceId"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": configs})
}

func (c *ZSegmentController) UpsertMFNConfig(ctx *gin.Context) {
	var cfg models.MFNInterfaceConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := c.svc.UpsertMFNConfig(ctx.Param("interfaceId"), cfg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (c *ZSegmentController) DeleteMFNConfig(ctx *gin.Context) {
	if err := c.svc.DeleteMFNConfig(ctx.Param("interfaceId"), ctx.Param("configId")); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// Z-segment configs
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) ListZSegmentConfigs(ctx *gin.Context) {
	configs, err := c.svc.ListZSegmentConfigs(ctx.Param("interfaceId"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": configs, "count": len(configs)})
}

func (c *ZSegmentController) GetZSegmentConfig(ctx *gin.Context) {
	cfg, err := c.svc.GetZSegmentConfig(ctx.Param("interfaceId"), ctx.Param("configId"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if cfg == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "not found"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (c *ZSegmentController) CreateZSegmentConfig(ctx *gin.Context) {
	var req models.CreateZSegmentConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	cfg, err := c.svc.CreateZSegmentConfig(ctx.Param("interfaceId"), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"success": true, "data": cfg})
}

func (c *ZSegmentController) UpdateZSegmentConfig(ctx *gin.Context) {
	var req models.UpdateZSegmentConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	cfg, err := c.svc.UpdateZSegmentConfig(ctx.Param("interfaceId"), ctx.Param("configId"), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (c *ZSegmentController) DeleteZSegmentConfig(ctx *gin.Context) {
	if err := c.svc.DeleteZSegmentConfig(ctx.Param("interfaceId"), ctx.Param("configId")); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// Template apply / save-as
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) ApplyTemplate(ctx *gin.Context) {
	var req models.ApplyTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	cfg, err := c.svc.ApplyTemplate(ctx.Param("interfaceId"), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"success": true, "data": cfg})
}

func (c *ZSegmentController) SaveAsTemplate(ctx *gin.Context) {
	var req models.SaveAsTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	userID := callerUserID(ctx)
	tmpl, err := c.svc.SaveAsTemplate(
		ctx.Param("interfaceId"), ctx.Param("configId"), req, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"success": true, "data": tmpl})
}

// ─────────────────────────────────────────────────────────────────────────────
// Field mappings
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) UpsertFieldMapping(ctx *gin.Context) {
	var req models.UpsertFieldMappingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	f, err := c.svc.UpsertFieldMapping(ctx.Param("interfaceId"), ctx.Param("configId"), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

func (c *ZSegmentController) DeleteFieldMapping(ctx *gin.Context) {
	if err := c.svc.DeleteFieldMapping(
		ctx.Param("interfaceId"), ctx.Param("configId"), ctx.Param("fieldId"),
	); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// Detection utility
// ─────────────────────────────────────────────────────────────────────────────

// DetectZSegments parses a raw HL7 message and returns the Z-segment IDs found.
// Used by the wizard to auto-populate the Z-segment step when a sample message
// is provided.
//
// POST /api/zsegments/detect
// Body: { "rawMessage": "MSH|...\rZEM|...\r" }
func (c *ZSegmentController) DetectZSegments(ctx *gin.Context) {
	var body struct {
		RawMessage string `json:"rawMessage" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	found := services.DetectZSegmentsInMessage(body.RawMessage)

	// For each detected segment: match OOB templates + parse raw field values
	// from the sample message so the wizard can display actual data in the
	// configuration panel without an extra round-trip.
	type segmentResult struct {
		SegmentID          string                          `json:"segmentId"`
		HasOOBTemplate     bool                            `json:"hasOobTemplate"`
		SuggestedTemplates []models.ZSegmentTemplate       `json:"suggestedTemplates"`
		// ParsedFields contains the raw field values extracted from the sample
		// message, one entry per occurrence of the segment.  The wizard uses
		// this to pre-populate the "value from your message" column.
		ParsedFields       [][]services.ZSegmentParsedField `json:"parsedFields"`
	}

	results := make([]segmentResult, 0, len(found))
	for _, segID := range found {
		templates, _ := c.svc.ListTemplates(segID)
		oob := false
		for _, t := range templates {
			if t.IsSystem {
				oob = true
				break
			}
		}
		parsedFields := services.ParseZSegmentFields(body.RawMessage, segID)
		results = append(results, segmentResult{
			SegmentID:          segID,
			HasOOBTemplate:     oob,
			SuggestedTemplates: templates,
			ParsedFields:       parsedFields,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":          true,
		"detectedSegments": found,
		"count":            len(found),
		"data":             results,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Runtime config
// ─────────────────────────────────────────────────────────────────────────────

func (c *ZSegmentController) GetRuntimeConfig(ctx *gin.Context) {
	cfg, err := c.svc.GetRuntimeConfig(ctx.Param("interfaceId"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if cfg == nil {
		// No config — assembly will use fallback (hardcoded ZEM logic)
		ctx.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
			"message": "No Z-segment config defined for this interface — assembly will use hardcoded fallback",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}
