// controllers/schema_controller.go
// REST API for schema package management — browse catalog, install, remove, check progress.

package controllers

import (
	"ezhealthkonnect/services/schema"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SchemaController handles schema package management endpoints.
type SchemaController struct {
	manager *schema.SchemaPackageManager
}

// NewSchemaController creates a controller backed by the given manager.
func NewSchemaController(manager *schema.SchemaPackageManager) *SchemaController {
	return &SchemaController{manager: manager}
}

// GetCatalog returns all available schema packages with their install status.
// GET /api/system/schemas/catalog
func (sc *SchemaController) GetCatalog(c *gin.Context) {
	packages, err := sc.manager.GetCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": packages, "count": len(packages)})
}

// GetInstalled returns only the packages currently installed on disk.
// GET /api/system/schemas/installed
func (sc *SchemaController) GetInstalled(c *gin.Context) {
	packages, err := sc.manager.GetInstalled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": packages, "count": len(packages)})
}

// InstallPackage starts an async download and install of a schema package.
// POST /api/system/schemas/install
// Body: { "package_id": "fhir-r4-base" }
func (sc *SchemaController) InstallPackage(c *gin.Context) {
	var body struct {
		PackageID string `json:"package_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "package_id is required"})
		return
	}

	// Resolve package from catalog
	packages, err := sc.manager.GetCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var target *schema.SchemaPackage
	for _, p := range packages {
		if p.ID == body.PackageID {
			target = p
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "package not found: " + body.PackageID})
		return
	}
	if target.Status == schema.StatusInstalled {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "package already installed"})
		return
	}

	if err := sc.manager.Install(target); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "Download started — poll /api/system/schemas/progress/" + body.PackageID,
		"package_id": body.PackageID,
	})
}

// GetProgress returns the download/install progress for a package.
// GET /api/system/schemas/progress/:id
func (sc *SchemaController) GetProgress(c *gin.Context) {
	packageID := c.Param("id")
	progress := sc.manager.GetProgress(packageID)
	if progress == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "no active download for: " + packageID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": progress})
}

// RemovePackage deletes a schema package from disk.
// DELETE /api/system/schemas/:id
func (sc *SchemaController) RemovePackage(c *gin.Context) {
	packageID := c.Param("id")

	packages, err := sc.manager.GetCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	var target *schema.SchemaPackage
	for _, p := range packages {
		if p.ID == packageID {
			target = p
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "package not found: " + packageID})
		return
	}
	if target.Status != schema.StatusInstalled {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "package is not installed"})
		return
	}

	if err := sc.manager.Remove(target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Package removed: " + packageID})
}
