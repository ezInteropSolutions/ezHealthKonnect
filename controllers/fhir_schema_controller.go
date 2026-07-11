// controllers/fhir_schema_controller.go
// FHIRSchemaController — REST API backing the fhir.build step's no-code
// field-mapping UI. The FHIR-side mirror of CDASchemaController's
// canonical-* endpoints.
//
// Endpoints:
//
//	GET /api/fhir/resource-types              → every resource type compiled for ?version (default R4)
//	GET /api/fhir/profiles/:resourceType       → every profile compiled for that resourceType+?version
//	GET /api/fhir/canonical-fields/:resourceType → element paths fhir.build can write, for ?profile (default base) + ?version
//	GET /api/fhir/transforms                   → DeclarativeTransformRegistry's named transforms, for the per-field "transform" picker
package controllers

import (
	"fmt"
	"net/http"

	fhirBuilder "ezhealthkonnect/fhir/builder"
	"ezhealthkonnect/fhir/r4"
	cdafhir "ezhealthkonnect/services/cda_fhir"

	"github.com/gin-gonic/gin"
)

// FHIRSchemaController exposes read-only FHIR schema catalog endpoints.
// Deliberately holds no injected schema loader/registry: fhir/r4.GetRegistry()
// is fetched fresh on every request, since r4.InitRegistry() can still be
// populating in a background goroutine when this controller is constructed
// (see main.go) — caching a pointer here would risk serving an empty catalog
// forever if that goroutine hadn't finished yet at construction time.
type FHIRSchemaController struct {
	transformReg *cdafhir.DeclarativeTransformRegistry
}

// NewFHIRSchemaController constructs the controller. transformReg is
// stateless (pure name -> function dispatch), safe to build once here.
func NewFHIRSchemaController() *FHIRSchemaController {
	return &FHIRSchemaController{transformReg: cdafhir.NewDeclarativeTransformRegistry()}
}

// RegisterRoutes wires the controller's endpoints under rg (expected to be
// api.Group("/fhir")).
func (fc *FHIRSchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/resource-types", fc.GetResourceTypes)
	rg.GET("/profiles/:resourceType", fc.GetProfiles)
	rg.GET("/canonical-fields/:resourceType", fc.GetCanonicalFields)
	rg.GET("/transforms", fc.GetTransforms)
}

// GetResourceTypes returns every FHIR resource type compiled for ?version
// (default "R4"), for the fhir.build step's resource-type picker.
func (fc *FHIRSchemaController) GetResourceTypes(c *gin.Context) {
	if r4.GetRegistry() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema registry not yet initialized",
		})
		return
	}
	version := c.DefaultQuery("version", "R4")
	types := fhirBuilder.ResourceTypeCatalog(version)
	c.JSON(http.StatusOK, gin.H{"success": true, "resourceTypes": types, "count": len(types)})
}

// GetProfiles returns every profile name (e.g. "base", "us-core") compiled
// for one resourceType+?version.
func (fc *FHIRSchemaController) GetProfiles(c *gin.Context) {
	if r4.GetRegistry() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema registry not yet initialized",
		})
		return
	}
	resourceType := c.Param("resourceType")
	version := c.DefaultQuery("version", "R4")
	profiles := fhirBuilder.ProfileCatalog(version, resourceType)
	c.JSON(http.StatusOK, gin.H{"success": true, "profiles": profiles, "count": len(profiles)})
}

// GetCanonicalFields returns every element path fhir.build can write for one
// resourceType (?profile default "base", ?version default "R4").
func (fc *FHIRSchemaController) GetCanonicalFields(c *gin.Context) {
	if r4.GetRegistry() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema registry not yet initialized",
		})
		return
	}
	resourceType := c.Param("resourceType")
	profile := c.DefaultQuery("profile", "base")
	version := c.DefaultQuery("version", "R4")

	fields := fhirBuilder.FieldCatalog(version, resourceType, profile)
	if fields == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown resourceType/profile/version %q/%q/%q", resourceType, profile, version),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "fields": fields, "count": len(fields)})
}

// GetTransforms returns every named DeclarativeTransformRegistry transform
// available to the fhir.build step's per-field "transform" picker.
func (fc *FHIRSchemaController) GetTransforms(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "transforms": fc.transformReg.AllDescriptions()})
}
