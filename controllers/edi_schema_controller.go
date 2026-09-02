// controllers/edi_schema_controller.go
// EDISchemaController — REST API for the EDI X12 pipeline step-builder UI
// (EDIStepBuilder.js's custom-rule picker and edi.map_to_canonical's
// segment/loop field mapper). Mirrors controllers/cda_schema_controller.go's
// pattern (constructor-injected schema loader, gin.H{success,...} response
// shape, 503 when the schema didn't load) at a much smaller scale — just
// enough to keep the UI's segment/element/loop names live against the real
// schema data instead of hardcoded in JS.
//
// Endpoints:
//
//	GET /api/edi/schema/segments                          → every segment ID's name/elements/repeats
//	GET /api/edi/schema/transaction-sets/:id/loops         → the loop tree for one transaction set
package controllers

import (
	"net/http"

	"ezhealthkonnect/edi"

	"github.com/gin-gonic/gin"
)

// EDISchemaController exposes EDI X12 schema browser endpoints.
type EDISchemaController struct {
	loader *edi.X12SchemaLoader
}

// NewEDISchemaController constructs the controller. loader may be nil when
// the schema directory is unavailable — endpoints return 503 in that case,
// matching CDASchemaController's own graceful-degradation convention.
func NewEDISchemaController(loader *edi.X12SchemaLoader) *EDISchemaController {
	return &EDISchemaController{loader: loader}
}

// RegisterRoutes mounts EDI schema endpoints under the provided RouterGroup.
// Caller passes api.Group("/edi") from main.go.
func (ec *EDISchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	schema := rg.Group("/schema")
	{
		schema.GET("/segments", ec.GetSegments)
		schema.GET("/transaction-sets/:id/loops", ec.GetTransactionSetLoops)
	}
}

// =========================================================
// GET /api/edi/schema/segments
// =========================================================

type elementSummary struct {
	Pos      string `json:"pos"`
	Key      string `json:"key"`
	Name     string `json:"name,omitempty"`
	DataType string `json:"dataType,omitempty"`
}

type repeatSummary struct {
	Key         string            `json:"key"`
	StartPos    int               `json:"startPos"`
	GroupSize   int               `json:"groupSize"`
	MaxGroups   int               `json:"maxGroups"`
	GroupFields []elementSummary  `json:"groupFields"`
}

type segmentSummary struct {
	ID       string           `json:"id"`
	Name     string           `json:"name,omitempty"`
	Usage    string           `json:"usage,omitempty"`
	MaxUse   string           `json:"maxUse,omitempty"`
	Elements []elementSummary `json:"elements"`
	Repeats  []repeatSummary  `json:"repeats,omitempty"`
}

// GetSegments returns every segment in the shared library — name, own
// elements (position/key/name/dataType), and any intra-segment repeat
// groups (e.g. CAS's reason/amount/quantity trios) — the data source for
// EdiValidateStepBuilder's custom-rule segment/position picker (rendered by
// element KEY, never raw position numbers) and edi.map_to_canonical's
// per-segment field mapper.
func (ec *EDISchemaController) GetSegments(c *gin.Context) {
	if ec.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "EDI schema not loaded — check schema directory configuration",
		})
		return
	}

	spec := ec.loader.Spec()
	result := make([]segmentSummary, 0, len(spec.Segments))
	for _, seg := range spec.Segments {
		result = append(result, toSegmentSummary(seg))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"segments": result,
		"count":    len(result),
	})
}

func toSegmentSummary(seg *edi.X12SegmentDef) segmentSummary {
	elements := make([]elementSummary, 0, len(seg.Elements))
	for _, el := range seg.Elements {
		elements = append(elements, toElementSummary(el))
	}
	repeats := make([]repeatSummary, 0, len(seg.Repeats))
	for _, r := range seg.Repeats {
		groupFields := make([]elementSummary, 0, len(r.GroupFields))
		for _, gf := range r.GroupFields {
			groupFields = append(groupFields, toElementSummary(gf))
		}
		repeats = append(repeats, repeatSummary{
			Key: r.Key, StartPos: r.StartPos, GroupSize: r.GroupSize,
			MaxGroups: r.MaxGroups, GroupFields: groupFields,
		})
	}
	return segmentSummary{
		ID: seg.ID, Name: seg.Name, Usage: seg.Usage, MaxUse: seg.MaxUse,
		Elements: elements, Repeats: repeats,
	}
}

func toElementSummary(el *edi.X12ElementDef) elementSummary {
	return elementSummary{
		Pos: el.Pos, Key: el.Key, Name: el.Name, DataType: string(el.DataType),
	}
}

// =========================================================
// GET /api/edi/schema/transaction-sets/:id/loops
// =========================================================

type loopSummary struct {
	ID         string        `json:"id"`
	Name       string        `json:"name,omitempty"`
	Repeat     string        `json:"repeat,omitempty"`
	SegmentIDs []string      `json:"segmentIds,omitempty"`
	Loops      []loopSummary `json:"loops,omitempty"`
}

// GetTransactionSetLoops returns the loop tree (id/name/repeat/nested
// loops, recursively) for one transaction set — the data source for
// edi.map_to_canonical's loop picker, which mirrors this exact recursive
// shape in its own mapping config.
func (ec *EDISchemaController) GetTransactionSetLoops(c *gin.Context) {
	if ec.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "EDI schema not loaded — check schema directory configuration",
		})
		return
	}

	txID := c.Param("id")
	txSet := ec.loader.GetTransactionSet(txID)
	if txSet == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "unknown transaction set: " + txID,
		})
		return
	}

	loops := make([]loopSummary, 0, len(txSet.Loops))
	for _, l := range txSet.Loops {
		loops = append(loops, toLoopSummary(l))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"transactionSet": txID,
		"loops":          loops,
	})
}

func toLoopSummary(l *edi.X12LoopDef) loopSummary {
	children := make([]loopSummary, 0, len(l.Loops))
	for _, c := range l.Loops {
		children = append(children, toLoopSummary(c))
	}
	return loopSummary{ID: l.ID, Name: l.Name, Repeat: l.Repeat, SegmentIDs: l.SegmentIDs, Loops: children}
}
