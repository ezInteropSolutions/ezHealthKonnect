package controllers

import (
	"database/sql"
	"log"
	"net/http"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/response"

	"github.com/gin-gonic/gin"
)

// ResponseMappingController handles API endpoints for response mapping templates
type ResponseMappingController struct {
	service *response.ResponseMappingService
}

// NewResponseMappingController creates a new response mapping controller
func NewResponseMappingController(db *sql.DB) *ResponseMappingController {
	return &ResponseMappingController{
		service: response.NewResponseMappingService(db),
	}
}

// RegisterRoutes registers all response mapping routes
func (c *ResponseMappingController) RegisterRoutes(router *gin.RouterGroup) {
	templates := router.Group("/response-mapping-templates")
	{
		templates.POST("", c.CreateTemplate)              // Create new template
		templates.GET("", c.ListTemplates)                // List all templates
		templates.GET("/:templateId", c.GetTemplate)      // Get specific template
		templates.PUT("/:templateId", c.UpdateTemplate)   // Update template
		templates.DELETE("/:templateId", c.DeleteTemplate) // Delete template
		templates.GET("/:templateId/usage", c.GetTemplateUsage) // Get usage info
	}
}

// ================================================================
// CREATE TEMPLATE
// ================================================================

// CreateTemplate creates a new response mapping template
// POST /api/response-mapping-templates
func (c *ResponseMappingController) CreateTemplate(ctx *gin.Context) {
	var req models.CreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ Invalid create template request: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := getUserIDFromContext(ctx)

	// Create template
	template, err := c.service.CreateTemplate(req, userID)
	if err != nil {
		log.Printf("❌ Failed to create template: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create template: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Created response mapping template: %s (ID: %s)", template.TemplateName, template.ID)
	ctx.JSON(http.StatusCreated, gin.H{
		"success":  true,
		"template": template,
	})
}

// ================================================================
// LIST TEMPLATES
// ================================================================

// ListTemplates lists all available response mapping templates
// GET /api/response-mapping-templates?apiType=empi&vendor=epic
func (c *ResponseMappingController) ListTemplates(ctx *gin.Context) {
	// Get filter parameters
	apiType := ctx.Query("apiType")
	vendor := ctx.Query("vendor")
	includeSystemStr := ctx.DefaultQuery("includeSystem", "true")

	// Get user context
	userID := getUserIDFromContext(ctx)
	orgID := getOrgIDFromContext(ctx)

	includeSystem := includeSystemStr == "true"

	// List templates
	templates, err := c.service.ListTemplates(apiType, vendor, userID, orgID, includeSystem)
	if err != nil {
		log.Printf("❌ Failed to list templates: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list templates: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Listed %d response mapping templates", len(templates))
	ctx.JSON(http.StatusOK, gin.H{
		"success":     true,
		"templates":   templates,
		"total_count": len(templates),
	})
}

// ================================================================
// GET TEMPLATE
// ================================================================

// GetTemplate retrieves a specific template by ID
// GET /api/response-mapping-templates/:templateId
func (c *ResponseMappingController) GetTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	template, err := c.service.GetTemplateByID(templateID)
	if err != nil {
		log.Printf("❌ Failed to get template %s: %v", templateID, err)
		ctx.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Template not found: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Retrieved template: %s", template.TemplateName)
	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"template": template,
	})
}

// ================================================================
// UPDATE TEMPLATE
// ================================================================

// UpdateTemplate updates an existing template
// PUT /api/response-mapping-templates/:templateId
func (c *ResponseMappingController) UpdateTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	var req models.UpdateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ Invalid update template request: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Get user ID from context
	userID := getUserIDFromContext(ctx)

	// Update template
	template, err := c.service.UpdateTemplate(templateID, req, userID)
	if err != nil {
		log.Printf("❌ Failed to update template %s: %v", templateID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update template: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Updated template: %s", template.TemplateName)
	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"template": template,
	})
}

// ================================================================
// DELETE TEMPLATE
// ================================================================

// DeleteTemplate soft-deletes a template
// DELETE /api/response-mapping-templates/:templateId
func (c *ResponseMappingController) DeleteTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	// Get user ID from context
	userID := getUserIDFromContext(ctx)

	// Delete template
	err := c.service.DeleteTemplate(templateID, userID)
	if err != nil {
		log.Printf("❌ Failed to delete template %s: %v", templateID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete template: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Deleted template: %s", templateID)
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Template deleted successfully",
	})
}

// ================================================================
// GET TEMPLATE USAGE
// ================================================================

// GetTemplateUsage shows where a template is being used
// GET /api/response-mapping-templates/:templateId/usage
func (c *ResponseMappingController) GetTemplateUsage(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	usage, err := c.service.GetTemplateUsage(templateID)
	if err != nil {
		log.Printf("❌ Failed to get template usage for %s: %v", templateID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get template usage: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Retrieved usage for template %s: %d locations", templateID, len(usage))
	ctx.JSON(http.StatusOK, gin.H{
		"success":     true,
		"template_id": templateID,
		"usage":       usage,
		"usage_count": len(usage),
	})
}

// ================================================================
// HELPER FUNCTIONS
// ================================================================

// getUserIDFromContext extracts user ID from Gin context
func getUserIDFromContext(ctx *gin.Context) string {
	// Try to get from session or JWT token
	if userID, exists := ctx.Get("user_id"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}

	// Fallback for development/testing
	return "system"
}

// getOrgIDFromContext extracts organization ID from Gin context
func getOrgIDFromContext(ctx *gin.Context) string {
	if orgID, exists := ctx.Get("organization_id"); exists {
		if id, ok := orgID.(string); ok {
			return id
		}
	}
	return ""
}
