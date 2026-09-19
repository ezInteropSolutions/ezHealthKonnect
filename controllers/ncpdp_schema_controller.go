// controllers/ncpdp_schema_controller.go
// NCPDPSchemaController — REST API for the NCPDP SCRIPT pipeline
// step-builder UI (NCPDPStepBuilder.js's ncpdp.map_to_canonical group/field
// mapper). Mirrors controllers/edi_schema_controller.go's pattern
// (constructor-injected schema loader, gin.H{success,...} response shape,
// 503 when the schema didn't load) — just enough to keep the UI's group/
// field names live against the real schema data instead of hardcoded in JS.
//
// Endpoints:
//
//	GET /api/ncpdp/schema/groups                    → every group's name/fields/nested group refs
//	GET /api/ncpdp/schema/transactions/:key/groups   → the group tree for one transaction type
package controllers

import (
	"net/http"

	"ezhealthkonnect/ncpdp"

	"github.com/gin-gonic/gin"
)

// NCPDPSchemaController exposes NCPDP SCRIPT schema browser endpoints.
type NCPDPSchemaController struct {
	loader *ncpdp.NCPDPSchemaLoader
}

// NewNCPDPSchemaController constructs the controller. loader may be nil
// when the schema directory is unavailable — endpoints return 503 in that
// case, matching EDISchemaController/CDASchemaController's own
// graceful-degradation convention.
func NewNCPDPSchemaController(loader *ncpdp.NCPDPSchemaLoader) *NCPDPSchemaController {
	return &NCPDPSchemaController{loader: loader}
}

// RegisterRoutes mounts NCPDP schema endpoints under the provided
// RouterGroup. Caller passes api.Group("/ncpdp") from main.go.
func (nc *NCPDPSchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	schema := rg.Group("/schema")
	{
		schema.GET("/groups", nc.GetGroups)
		schema.GET("/transactions", nc.ListTransactions)
		schema.GET("/transactions/:key/groups", nc.GetTransactionGroups)
	}
}

// =========================================================
// GET /api/ncpdp/schema/groups
// =========================================================

type ncpdpFieldSummary struct {
	Key      string `json:"key"`
	XPath    string `json:"xpath"`
	Name     string `json:"name,omitempty"`
	DataType string `json:"dataType,omitempty"`
	Required bool   `json:"required"`
}

type ncpdpGroupRefSummary struct {
	Key        string `json:"key"`
	GroupKey   string `json:"groupKey"`
	XMLElement string `json:"xmlElement,omitempty"`
	Required   bool   `json:"required"`
	Repeatable bool   `json:"repeatable"`
}

type ncpdpGroupSummary struct {
	Key        string                  `json:"key"`
	XMLElement string                  `json:"xmlElement"`
	Name       string                  `json:"name,omitempty"`
	Fields     []ncpdpFieldSummary     `json:"fields,omitempty"`
	Groups     []ncpdpGroupRefSummary  `json:"groups,omitempty"`
}

// GetGroups returns every group in the shared library — name, own fields,
// and nested group refs — the data source for NCPDPMapToCanonicalStepBuilder's
// group/field picker.
func (nc *NCPDPSchemaController) GetGroups(c *gin.Context) {
	if nc.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "NCPDP SCRIPT schema not loaded — check schema directory configuration",
		})
		return
	}

	spec := nc.loader.Spec()
	result := make([]ncpdpGroupSummary, 0, len(spec.Groups))
	for _, g := range spec.Groups {
		result = append(result, toNCPDPGroupSummary(g))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"groups":         result,
		"count":          len(result),
		"headerGroupKey": spec.HeaderGroupKey,
	})
}

func toNCPDPGroupSummary(g *ncpdp.NCPDPGroupDef) ncpdpGroupSummary {
	fields := make([]ncpdpFieldSummary, 0, len(g.Fields))
	for _, f := range g.Fields {
		fields = append(fields, toNCPDPFieldSummary(f))
	}
	groups := make([]ncpdpGroupRefSummary, 0, len(g.Groups))
	for _, ref := range g.Groups {
		groups = append(groups, toNCPDPGroupRefSummary(ref))
	}
	return ncpdpGroupSummary{
		Key: g.Key, XMLElement: g.XMLElement, Name: g.Name,
		Fields: fields, Groups: groups,
	}
}

func toNCPDPFieldSummary(f ncpdp.NCPDPFieldDef) ncpdpFieldSummary {
	return ncpdpFieldSummary{Key: f.Key, XPath: f.XPath, Name: f.Name, DataType: f.DataType, Required: f.Required}
}

func toNCPDPGroupRefSummary(ref ncpdp.NCPDPGroupRef) ncpdpGroupRefSummary {
	return ncpdpGroupRefSummary{
		Key: ref.Key, GroupKey: ref.GroupKey, XMLElement: ref.XMLElement,
		Required: ref.Required, Repeatable: ref.Repeatable,
	}
}

// =========================================================
// GET /api/ncpdp/schema/transactions
// =========================================================

// ListTransactions returns every registered transaction type's key/name —
// the data source for the transaction-type picker in ncpdp.parse/validate/
// build/map_to_canonical's step config UI.
func (nc *NCPDPSchemaController) ListTransactions(c *gin.Context) {
	if nc.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "NCPDP SCRIPT schema not loaded — check schema directory configuration",
		})
		return
	}

	spec := nc.loader.Spec()
	type txSummary struct {
		Key  string `json:"key"`
		Name string `json:"name,omitempty"`
	}
	result := make([]txSummary, 0, len(spec.Transactions))
	for key, tx := range spec.Transactions {
		result = append(result, txSummary{Key: key, Name: tx.Name})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"transactions": result,
		"count":        len(result),
	})
}

// =========================================================
// GET /api/ncpdp/schema/transactions/:key/groups
// =========================================================

// GetTransactionGroups returns the top-level fields and group tree for one
// transaction type — the data source for ncpdp.map_to_canonical's
// bodyFields/bodyGroups picker, which mirrors this exact recursive shape in
// its own mapping config.
func (nc *NCPDPSchemaController) GetTransactionGroups(c *gin.Context) {
	if nc.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "NCPDP SCRIPT schema not loaded — check schema directory configuration",
		})
		return
	}

	key := c.Param("key")
	tx, err := nc.loader.GetTransaction(key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "unknown transaction type: " + key,
		})
		return
	}

	fields := make([]ncpdpFieldSummary, 0, len(tx.Fields))
	for _, f := range tx.Fields {
		fields = append(fields, toNCPDPFieldSummary(f))
	}
	groups := make([]ncpdpGroupRefSummary, 0, len(tx.Groups))
	for _, ref := range tx.Groups {
		groups = append(groups, toNCPDPGroupRefSummary(ref))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"transaction": key,
		"fields":      fields,
		"groups":      groups,
	})
}
