// controllers/hl7_schema_controller.go
// HL7SchemaController — REST API backing the hl7.build step's no-code
// segment/field-mapping UI. The HL7-side mirror of CDASchemaController's and
// FHIRSchemaController's canonical-* endpoints.
//
// Endpoints:
//
//	GET /api/hl7/message-types                                    → every {messageType, triggerEvent} pair available for ?version (default 2.5.1)
//	GET /api/hl7/segments/:messageType/:triggerEvent              → every segment name defined for that message type, for ?version
//	GET /api/hl7/canonical-fields/:messageType/:triggerEvent/:segment → field/component keys hl7.build can write for one segment, for ?version
package controllers

import (
	"fmt"
	"net/http"

	"ezhealthkonnect/hl7"
	hl7builder "ezhealthkonnect/hl7/builder"

	"github.com/gin-gonic/gin"
)

// HL7SchemaController exposes read-only HL7 schema catalog endpoints.
// Deliberately holds no cached *hl7.RealHL7Schema: hl7.GetRealSchemaLoader()
// is fetched fresh on every request, since main.go's schema-init call sites
// don't guarantee ordering relative to controller construction — see
// hl7_build_executor.go's identical lazy-fetch discipline for the same
// reason. schemaDir is only needed for MessageTypeCatalog's own directory
// enumeration (RealSchemaLoader's schema directory field is unexported).
type HL7SchemaController struct {
	schemaDir string
}

// NewHL7SchemaController constructs the controller. schemaDir is the HL7
// schema directory (e.g. "./schemas/hl7"), the same path passed to
// hl7.InitSchemaLoader/InitRealSchemaLoader at startup.
func NewHL7SchemaController(schemaDir string) *HL7SchemaController {
	return &HL7SchemaController{schemaDir: schemaDir}
}

// RegisterRoutes wires the controller's endpoints under rg (expected to be
// api.Group("/hl7")).
func (hc *HL7SchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/message-types", hc.GetMessageTypes)
	rg.GET("/segments/:messageType/:triggerEvent", hc.GetSegmentNames)
	rg.GET("/canonical-fields/:messageType/:triggerEvent/:segment", hc.GetCanonicalFields)
}

// GetMessageTypes returns every {messageType, triggerEvent} pair with a
// compiled schema file for ?version (default "2.5.1"), for the hl7.build
// step's message-type picker.
func (hc *HL7SchemaController) GetMessageTypes(c *gin.Context) {
	version := c.DefaultQuery("version", "2.5.1")
	types := hl7builder.MessageTypeCatalog(hc.schemaDir, version)
	c.JSON(http.StatusOK, gin.H{"success": true, "messageTypes": types, "count": len(types)})
}

// GetSegmentNames returns every segment name defined for one
// messageType/triggerEvent/?version, for the hl7.build step's "add segment" picker.
func (hc *HL7SchemaController) GetSegmentNames(c *gin.Context) {
	loader := hl7.GetRealSchemaLoader()
	if loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "HL7 schema loader not yet initialized",
		})
		return
	}
	messageType := c.Param("messageType")
	triggerEvent := c.Param("triggerEvent")
	version := c.DefaultQuery("version", "2.5.1")

	schema, err := loader.LoadRealSchema(version, messageType, triggerEvent)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown messageType/triggerEvent/version %q/%q/%q: %v", messageType, triggerEvent, version, err),
		})
		return
	}
	names := hl7builder.SegmentNames(schema)
	c.JSON(http.StatusOK, gin.H{"success": true, "segments": names, "count": len(names)})
}

// GetCanonicalFields returns every field/component key hl7.build can write
// for one segment within one messageType/triggerEvent/?version.
func (hc *HL7SchemaController) GetCanonicalFields(c *gin.Context) {
	loader := hl7.GetRealSchemaLoader()
	if loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "HL7 schema loader not yet initialized",
		})
		return
	}
	messageType := c.Param("messageType")
	triggerEvent := c.Param("triggerEvent")
	segment := c.Param("segment")
	version := c.DefaultQuery("version", "2.5.1")

	schema, err := loader.LoadRealSchema(version, messageType, triggerEvent)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown messageType/triggerEvent/version %q/%q/%q: %v", messageType, triggerEvent, version, err),
		})
		return
	}
	fields := hl7builder.SegmentFieldCatalog(schema, segment)
	if fields == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown segment %q for %s^%s", segment, messageType, triggerEvent),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "fields": fields, "count": len(fields)})
}
