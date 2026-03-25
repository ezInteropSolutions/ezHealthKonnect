package controllers

import (
	"net/http"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// CodeTemplateController handles CRUD and interface-linking for Code Templates.
// Code Templates are named JavaScript function libraries automatically injected
// into every script step's goja VM (inspired by Mirth Connect Code Templates).
type CodeTemplateController struct {
	svc *services.CodeTemplateService
}

// NewCodeTemplateController creates a controller backed by the given service.
func NewCodeTemplateController(svc *services.CodeTemplateService) *CodeTemplateController {
	return &CodeTemplateController{svc: svc}
}

// RegisterRoutes mounts all /api/code-templates/* routes onto the provided group.
// IMPORTANT: static-prefix routes must be registered BEFORE the /:id wildcard
// to avoid a Gin radix-tree conflict (static nodes have priority over wildcards).
func (ctrl *CodeTemplateController) RegisterRoutes(rg *gin.RouterGroup) {
	// Collection endpoints
	rg.GET("", ctrl.List)
	rg.POST("", ctrl.Create)

	// Static-prefix routes — registered BEFORE /:id wildcard
	rg.GET("/for-interface/:interfaceId", ctrl.ListForInterface)
	rg.POST("/link", ctrl.LinkToInterface)
	rg.DELETE("/link/:templateId/:interfaceId", ctrl.UnlinkFromInterface)
	rg.POST("/invalidate-cache", ctrl.InvalidateCache)

	// Instance CRUD — wildcard last so static routes take priority
	rg.GET("/:id", ctrl.Get)
	rg.PUT("/:id", ctrl.Update)
	rg.DELETE("/:id", ctrl.Delete)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// List godoc
// GET /api/code-templates?category=hl7&scope=global&is_active=true
func (ctrl *CodeTemplateController) List(c *gin.Context) {
	templates, err := ctrl.svc.List(
		c.Query("category"),
		c.Query("scope"),
		c.Query("is_active"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": templates, "count": len(templates)})
}

// Get godoc
// GET /api/code-templates/:id
func (ctrl *CodeTemplateController) Get(c *gin.Context) {
	t, err := ctrl.svc.Get(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "code template not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Create godoc
// POST /api/code-templates
func (ctrl *CodeTemplateController) Create(c *gin.Context) {
	var req struct {
		Name               string   `json:"name" binding:"required"`
		Description        string   `json:"description"`
		Category           string   `json:"category"`
		Code               string   `json:"code" binding:"required"`
		FunctionSignatures []string `json:"function_signatures"`
		Scope              string   `json:"scope"`
		IsActive           *bool    `json:"is_active"`
		SortOrder          *int     `json:"sort_order"`
		CreatedByUserID    *string  `json:"created_by_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Defaults
	if req.Category == "" {
		req.Category = "general"
	}
	if req.Scope == "" {
		req.Scope = "global"
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	sortOrder := 100
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	if req.FunctionSignatures == nil {
		req.FunctionSignatures = []string{}
	}

	t := &models.CodeTemplate{
		Name:               req.Name,
		Description:        req.Description,
		Category:           req.Category,
		Code:               req.Code,
		FunctionSignatures: req.FunctionSignatures,
		Scope:              req.Scope,
		IsActive:           isActive,
		SortOrder:          sortOrder,
		CreatedByUserID:    req.CreatedByUserID,
	}
	created, err := ctrl.svc.Create(t)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": created})
}

// Update godoc
// PUT /api/code-templates/:id
func (ctrl *CodeTemplateController) Update(c *gin.Context) {
	var req struct {
		Name               string   `json:"name" binding:"required"`
		Description        string   `json:"description"`
		Category           string   `json:"category"`
		Code               string   `json:"code" binding:"required"`
		FunctionSignatures []string `json:"function_signatures"`
		Scope              string   `json:"scope"`
		IsActive           bool     `json:"is_active"`
		SortOrder          int      `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.FunctionSignatures == nil {
		req.FunctionSignatures = []string{}
	}

	t := &models.CodeTemplate{
		Name:               req.Name,
		Description:        req.Description,
		Category:           req.Category,
		Code:               req.Code,
		FunctionSignatures: req.FunctionSignatures,
		Scope:              req.Scope,
		IsActive:           req.IsActive,
		SortOrder:          req.SortOrder,
	}
	updated, err := ctrl.svc.Update(c.Param("id"), t)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "code template not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

// Delete godoc
// DELETE /api/code-templates/:id
func (ctrl *CodeTemplateController) Delete(c *gin.Context) {
	if err := ctrl.svc.Delete(c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "code template not found" {
			status = http.StatusNotFound
		}
		if err.Error() == "system templates cannot be deleted" {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Code template deleted"})
}

// ── INTERFACE SCOPING ──────────────────────────────────────────────────────────

// ListForInterface godoc
// GET /api/code-templates/for-interface/:interfaceId
// Returns all templates that will be injected for a given interface
// (all active global templates + any interface-scoped templates linked to it).
func (ctrl *CodeTemplateController) ListForInterface(c *gin.Context) {
	templates, err := ctrl.svc.ListForInterface(c.Param("interfaceId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": templates, "count": len(templates)})
}

// LinkToInterface godoc
// POST /api/code-templates/link
// Body: { "template_id": "...", "interface_id": "..." }
func (ctrl *CodeTemplateController) LinkToInterface(c *gin.Context) {
	var req struct {
		TemplateID  string `json:"template_id" binding:"required"`
		InterfaceID string `json:"interface_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := ctrl.svc.LinkToInterface(req.TemplateID, req.InterfaceID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template linked to interface"})
}

// UnlinkFromInterface godoc
// DELETE /api/code-templates/link/:templateId/:interfaceId
func (ctrl *CodeTemplateController) UnlinkFromInterface(c *gin.Context) {
	if err := ctrl.svc.UnlinkFromInterface(c.Param("templateId"), c.Param("interfaceId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Template unlinked from interface"})
}

// ── ADMIN ─────────────────────────────────────────────────────────────────────

// InvalidateCache godoc
// POST /api/code-templates/invalidate-cache
// Flushes all in-process cache entries so the next script execution reloads from DB.
func (ctrl *CodeTemplateController) InvalidateCache(c *gin.Context) {
	ctrl.svc.InvalidateAll()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Code template cache invalidated"})
}
