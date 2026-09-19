// controllers/ncpdp_telecom_schema_controller.go
// NCPDPTelecomSchemaController — REST API for the NCPDP Telecommunication
// D.0 pipeline step-builder UI (NCPDPTelecomStepBuilder.js). Mirrors
// controllers/ncpdp_schema_controller.go's/edi_schema_controller.go's own
// pattern (constructor-injected schema loader, gin.H{success,...} response
// shape, 503 when the schema didn't load) — just enough to keep the UI's
// segment/field/transaction names live against the real schema data
// instead of hardcoded in JS.
//
// Endpoints:
//
//	GET /api/ncpdp-telecom/schema/segments      → every segment's own fields
//	GET /api/ncpdp-telecom/schema/transactions  → registered transaction code/direction pairs + their segment lists
package controllers

import (
	"net/http"

	"ezhealthkonnect/ncpdptelecom"

	"github.com/gin-gonic/gin"
)

// NCPDPTelecomSchemaController exposes NCPDP Telecommunication D.0 schema
// browser endpoints.
type NCPDPTelecomSchemaController struct {
	loader *ncpdptelecom.TelecomSchemaLoader
}

// NewNCPDPTelecomSchemaController constructs the controller. loader may be
// nil when the schema directory is unavailable — endpoints return 503 in
// that case, matching every other schema controller's graceful-degradation
// convention.
func NewNCPDPTelecomSchemaController(loader *ncpdptelecom.TelecomSchemaLoader) *NCPDPTelecomSchemaController {
	return &NCPDPTelecomSchemaController{loader: loader}
}

// RegisterRoutes mounts NCPDP Telecom D.0 schema endpoints under the
// provided RouterGroup. Caller passes api.Group("/ncpdp-telecom") from main.go.
func (nc *NCPDPTelecomSchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	schema := rg.Group("/schema")
	{
		schema.GET("/segments", nc.GetSegments)
		schema.GET("/transactions", nc.ListTransactions)
	}
}

// =========================================================
// GET /api/ncpdp-telecom/schema/segments
// =========================================================

type telecomFieldSummary struct {
	Key      string `json:"key"`
	FieldID  string `json:"fieldId,omitempty"`
	Name     string `json:"name"`
	DataType string `json:"dataType"`
	Required bool   `json:"required"`
	Places   int    `json:"places,omitempty"`
	Width    int    `json:"width,omitempty"`
}

type telecomSegmentSummary struct {
	Key        string                 `json:"key"`
	Identifier string                 `json:"identifier"`
	Name       string                 `json:"name"`
	Fields     []telecomFieldSummary  `json:"fields"`
}

// GetSegments returns every segment in the D.0 schema — its own wire
// identifier and field list — the data source for
// NCPDPTelecomMapToCanonicalStepBuilder's segment/field picker.
func (nc *NCPDPTelecomSchemaController) GetSegments(c *gin.Context) {
	if nc.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "NCPDP Telecom D.0 schema not loaded — check schema directory configuration",
		})
		return
	}

	spec := nc.loader.Spec()
	result := make([]telecomSegmentSummary, 0, len(spec.Segments))
	for _, seg := range spec.Segments {
		result = append(result, toTelecomSegmentSummary(seg))
	}

	headerFields := make([]telecomFieldSummary, 0, len(spec.HeaderFields))
	for _, f := range spec.HeaderFields {
		headerFields = append(headerFields, toTelecomFieldSummary(f))
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"segments":     result,
		"count":        len(result),
		"headerFields": headerFields,
	})
}

func toTelecomSegmentSummary(s *ncpdptelecom.TelecomSegmentDef) telecomSegmentSummary {
	fields := make([]telecomFieldSummary, 0, len(s.Fields))
	for _, f := range s.Fields {
		fields = append(fields, toTelecomFieldSummary(f))
	}
	return telecomSegmentSummary{Key: s.Key, Identifier: s.Identifier, Name: s.Name, Fields: fields}
}

func toTelecomFieldSummary(f ncpdptelecom.TelecomFieldDef) telecomFieldSummary {
	return telecomFieldSummary{
		Key: f.Key, FieldID: f.FieldID, Name: f.Name,
		DataType: string(f.DataType), Required: f.Required, Places: f.Places, Width: f.Width,
	}
}

// =========================================================
// GET /api/ncpdp-telecom/schema/transactions
// =========================================================

type telecomTransactionSummary struct {
	Code                      string   `json:"code"`
	Name                      string   `json:"name"`
	Direction                 string   `json:"direction"`
	TransmissionGroupSegments []string `json:"transmissionGroupSegments"`
	TransactionGroupSegments  []string `json:"transactionGroupSegments"`
}

// ListTransactions returns every registered transaction code/direction
// pair, and the transmission-group/transaction-group segment lists for
// each — the data source for the transaction-type picker in
// ncpdptelecom.parse/validate/build/map_to_canonical's step config UI.
func (nc *NCPDPTelecomSchemaController) ListTransactions(c *gin.Context) {
	if nc.loader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "NCPDP Telecom D.0 schema not loaded — check schema directory configuration",
		})
		return
	}

	spec := nc.loader.Spec()
	result := make([]telecomTransactionSummary, 0, len(spec.Transactions))
	for _, tx := range spec.Transactions {
		result = append(result, telecomTransactionSummary{
			Code:                      tx.Code,
			Name:                      tx.Name,
			Direction:                 tx.Direction,
			TransmissionGroupSegments: tx.TransmissionGroupSegments,
			TransactionGroupSegments:  tx.TransactionGroupSegments,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"transactions": result,
		"count":        len(result),
	})
}
