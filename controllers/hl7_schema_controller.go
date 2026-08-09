// controllers/hl7_schema_controller.go
// HL7SchemaController — REST API backing the hl7.build step's no-code
// segment/field-mapping UI. The HL7-side mirror of CDASchemaController's and
// FHIRSchemaController's canonical-* endpoints.
//
// Endpoints:
//
//	GET  /api/hl7/versions                                         → every HL7 version with a compiled schema directory
//	GET  /api/hl7/message-types                                    → every {messageType, triggerEvent} pair available for ?version (default 2.5.1)
//	GET  /api/hl7/segments/:messageType/:triggerEvent              → every segment name defined for that message type, for ?version
//	GET  /api/hl7/canonical-fields/:messageType/:triggerEvent/:segment → field/component keys hl7.build can write for one segment, for ?version
//	GET  /api/hl7/schema-tree/:messageType/:triggerEvent           → group/segment tree + required-segment spine, for ?version
//	POST /api/hl7/next-segments/:messageType/:triggerEvent         → grammar-guarded "what can I add next" for the segment picker, for ?version
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
	rg.GET("/versions", hc.GetVersions)
	rg.GET("/message-types", hc.GetMessageTypes)
	rg.GET("/segments/:messageType/:triggerEvent", hc.GetSegmentNames)
	rg.GET("/canonical-fields/:messageType/:triggerEvent/:segment", hc.GetCanonicalFields)
	rg.GET("/schema-tree/:messageType/:triggerEvent", hc.GetSchemaTree)
	rg.POST("/next-segments/:messageType/:triggerEvent", hc.GetNextAllowedSegments)
}

// GetVersions returns every HL7 version with a compiled schema directory, for
// the hl7.build step's version picker (replacing a free-text version input).
func (hc *HL7SchemaController) GetVersions(c *gin.Context) {
	versions := hl7builder.AvailableVersions(hc.schemaDir)
	c.JSON(http.StatusOK, gin.H{"success": true, "versions": versions, "count": len(versions)})
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

// GetSchemaTree returns the group/segment tree (with cumulative
// required/repeat annotations) and the required-segment spine for one
// messageType/triggerEvent/?version — backs the hl7.build step's
// auto-seeding of required segments and its per-segment "can this repeat"
// gating (hl7builder.SchemaTree/RequiredSpine).
func (hc *HL7SchemaController) GetSchemaTree(c *gin.Context) {
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
	tree := hl7builder.SchemaTree(schema)
	requiredSpine := hl7builder.RequiredSpine(schema)
	c.JSON(http.StatusOK, gin.H{"success": true, "tree": tree, "requiredSpine": requiredSpine})
}

// nextSegmentsRequest is the POST body for GetNextAllowedSegments: the
// segment names already configured, in order (MSH excluded — the caller
// never includes it, since it's auto-populated separately).
type nextSegmentsRequest struct {
	AddedSegments []string `json:"addedSegments"`
}

// GetNextAllowedSegments returns which segment names are grammatically valid
// to add next (the picker's default list) plus every segment in the schema
// (for the picker's explicit "add anyway (non-standard)" override) — backs
// the hl7.build step's ordering guardrail (hl7builder.NextAllowedSegments).
func (hc *HL7SchemaController) GetNextAllowedSegments(c *gin.Context) {
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

	var req nextSegmentsRequest
	_ = c.ShouldBindJSON(&req) // empty/missing body is valid — treated as "nothing added yet"

	allowed, all := hl7builder.NextAllowedSegments(schema, req.AddedSegments)
	c.JSON(http.StatusOK, gin.H{"success": true, "allowed": allowed, "all": all})
}
