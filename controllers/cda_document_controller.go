// controllers/cda_document_controller.go
// CDADocumentController — REST API for stored CDA documents.
//
// Endpoints (all under /api/cda/documents):
//
//	POST   /                     — persist a parsed CDA document (for direct API ingestion)
//	GET    /                     — list stored documents (paginated, filterable)
//	GET    /:id                  — get document metadata
//	GET    /:id/xml              — download original raw CDA XML
//	GET    /:id/json             — download parsed JSON
//	DELETE /:id                  — soft-delete a document

package controllers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cdastorage "ezhealthkonnect/services/cda_storage"
	cdaparser "ezhealthkonnect/services/parsers/cda"

	"github.com/gin-gonic/gin"
)

// CDADocumentController handles persistence and retrieval of stored CDA documents.
type CDADocumentController struct {
	store     cdastorage.CDADocumentStore
	parserSvc *cdaparser.CDAParserService
}

// NewCDADocumentController constructs the controller.
// store must not be nil; parserSvc may be nil (POST /api/cda/documents falls back to
// returning 503 if the parser is unavailable).
func NewCDADocumentController(store cdastorage.CDADocumentStore, parserSvc *cdaparser.CDAParserService) *CDADocumentController {
	return &CDADocumentController{store: store, parserSvc: parserSvc}
}

// RegisterRoutes mounts all document endpoints under the provided RouterGroup.
// Caller passes api.Group("/cda/documents") from main.go.
func (cc *CDADocumentController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", cc.Store)
	rg.GET("", cc.List)
	rg.GET("/:id", cc.Get)
	rg.GET("/:id/xml", cc.GetXML)
	rg.GET("/:id/json", cc.GetJSON)
	rg.DELETE("/:id", cc.Delete)
}

// ─── POST / ──────────────────────────────────────────────────────────────────

// Store parses and persists a CDA document submitted directly to the API.
// This endpoint is for direct document ingestion (e.g. from a CDAPH tool or test harness);
// documents processed via the pipeline are stored automatically by the cda.parse executor.
//
// Request: Content-Type application/xml or text/xml with raw CDA XML body (50 MB cap),
//
//	or JSON body {"xml": "..."}.
//
// Response: {"success": true, "document": StoredDocument}
func (cc *CDADocumentController) Store(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}
	if cc.parserSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "CDA parser not initialised — check schema directory"})
		return
	}

	rawXML, err := cdaReadXMLBody(c, 50*1024*1024)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(rawXML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "request body is empty"})
		return
	}

	result := cc.parserSvc.Parse(rawXML)
	if !result.Success {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error":   "CDA parse failed: " + result.Error,
		})
		return
	}

	interfaceID := c.Query("interfaceId")
	messageID := c.Query("messageId")

	input := &cdastorage.SaveInput{
		InterfaceID:   interfaceID,
		MessageID:     messageID,
		RawXML:        rawXML,
		ParsedJSON:    result.ParsedJSON,
		TypedDocument: result.TypedDocument,
		SectionCount:  countSections(result.ParsedJSON),
		FieldCount:    len(result.EnhancedFields),
	}

	stored, err := cc.store.Save(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fmt.Sprintf("store failed: %v", err)})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "document": stored})
}

// ─── GET / ───────────────────────────────────────────────────────────────────

// List returns a paginated list of stored CDA documents.
//
// Query params: interfaceId, dateFrom (RFC3339), dateTo (RFC3339), status, page (1-based), limit.
func (cc *CDADocumentController) List(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}

	filter := cdastorage.ListFilter{
		InterfaceID: c.Query("interfaceId"),
		Status:      c.Query("status"),
		Page:        parseIntQuery(c, "page", 1),
		Limit:       parseIntQuery(c, "limit", 20),
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	if df := c.Query("dateFrom"); df != "" {
		if t, err := time.Parse(time.RFC3339, df); err == nil {
			filter.DateFrom = &t
		}
	}
	if dt := c.Query("dateTo"); dt != "" {
		if t, err := time.Parse(time.RFC3339, dt); err == nil {
			filter.DateTo = &t
		}
	}

	docs, total, err := cc.store.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      docs,
		"total":     total,
		"page":      filter.Page,
		"limit":     filter.Limit,
		"pageCount": pageCount(total, filter.Limit),
	})
}

// ─── GET /:id ────────────────────────────────────────────────────────────────

// Get retrieves metadata for a single stored document.
func (cc *CDADocumentController) Get(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}
	doc, err := cc.store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "document": doc})
}

// ─── GET /:id/xml ────────────────────────────────────────────────────────────

// GetXML returns the original raw CDA XML as application/xml.
func (cc *CDADocumentController) GetXML(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}
	xml, err := cc.store.GetRawXML(c.Request.Context(), c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if xml == "" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "raw XML not available for this document"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="cda-%s.xml"`, c.Param("id")))
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(xml))
}

// ─── GET /:id/json ───────────────────────────────────────────────────────────

// GetJSON returns the parsed JSON map as application/json.
func (cc *CDADocumentController) GetJSON(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}
	parsed, err := cc.store.GetParsedJSON(c.Request.Context(), c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "document": parsed})
}

// ─── DELETE /:id ─────────────────────────────────────────────────────────────

// Delete soft-deletes a stored document (sets deleted_at, data is retained).
func (cc *CDADocumentController) Delete(c *gin.Context) {
	if cc.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "document store not initialised"})
		return
	}
	if err := cc.store.Delete(c.Request.Context(), c.Param("id")); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── Private helpers ─────────────────────────────────────────────────────────

func cdaReadXMLBody(c *gin.Context, maxBytes int64) (string, error) {
	ct := c.GetHeader("Content-Type")
	if strings.Contains(ct, "application/xml") || strings.Contains(ct, "text/xml") {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes))
		if err != nil {
			return "", fmt.Errorf("cannot read request body: %w", err)
		}
		return string(body), nil
	}
	var payload struct {
		XML string `json:"xml"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		return "", fmt.Errorf(`expected JSON body {"xml": "..."} or Content-Type: application/xml: %w`, err)
	}
	return payload.XML, nil
}

func countSections(parsedJSON map[string]interface{}) int {
	if parsedJSON == nil {
		return 0
	}
	if sections, ok := parsedJSON["sections"].(map[string]interface{}); ok {
		return len(sections)
	}
	return 0
}

func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	if s := c.Query(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return defaultVal
}

func pageCount(total, limit int) int {
	if limit <= 0 {
		return 1
	}
	pages := total / limit
	if total%limit > 0 {
		pages++
	}
	return pages
}
