// controllers/cda_validation_controller.go
// CDAValidationController — C-CDA 2.1 conformance validation REST endpoint.
//
// Endpoints:
//   POST /api/cda/validate
//     Accepts raw CDA XML (Content-Type: application/xml | text/xml)
//     or JSON body {"xml": "..."}.
//     Parses the document via CDAParserService (Sprint B) and runs
//     CDAConformanceValidator (Sprint C) against ccda_2_1.json rules.
//     Returns a ComplianceReport JSON.

package controllers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/cda/validator"
	cdaparser "ezhealthkonnect/services/parsers/cda"

	"github.com/gin-gonic/gin"
)

// CDAValidationController handles CDA conformance validation requests.
type CDAValidationController struct {
	parserSvc *cdaparser.CDAParserService
	validator *validator.CDAConformanceValidator
}

// NewCDAValidationController constructs the controller, initialising the CDA parser
// service and conformance validator from the given schema loader.
//
// If loader is nil (schema files unavailable), all endpoints return 503.
func NewCDAValidationController(loader *cdaSchema.CDASchemaLoader) *CDAValidationController {
	ctrl := &CDAValidationController{}
	if loader == nil {
		log.Printf("⚠️  [cda.validate] Schema loader is nil — /api/cda/validate will return 503")
		return ctrl
	}

	svc, err := cdaparser.NewFromSchemaDir("./cda/schemas")
	if err != nil {
		log.Printf("⚠️  [cda.validate] Parser service init failed: %v — /api/cda/validate will return 503", err)
		return ctrl
	}

	ctrl.parserSvc = svc
	ctrl.validator = validator.NewCDAConformanceValidator(loader)
	log.Printf("✅ [cda.validate] CDAValidationController ready")
	return ctrl
}

// RegisterRoutes mounts the validation endpoint under the provided RouterGroup.
// Caller passes api.Group("/cda") from main.go.
func (cc *CDAValidationController) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/validate", cc.ValidateRaw)
}

// ValidateRaw handles POST /api/cda/validate.
//
// Request formats:
//   - Content-Type: application/xml or text/xml — raw CDA XML body (50 MB cap)
//   - Content-Type: application/json            — JSON body {"xml": "...raw CDA..."}
//
// Response: {"success": true, "report": ComplianceReport}
func (cc *CDAValidationController) ValidateRaw(c *gin.Context) {
	if cc.validator == nil || cc.parserSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA validator not initialised — check schema directory configuration",
		})
		return
	}

	rawXML, err := cc.readXMLBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if strings.TrimSpace(rawXML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "request body is empty",
		})
		return
	}

	// Parse via Sprint B CDAParserService — populates result.TypedDocument.
	parseResult := cc.parserSvc.Parse(rawXML)
	if !parseResult.Success {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error":   "CDA parse failed: " + parseResult.Error,
		})
		return
	}

	doc, ok := parseResult.TypedDocument.(*cdadocument.CDADocument)
	if !ok || doc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "typed CDA document unavailable — Sprint B parser not active",
		})
		return
	}

	// Run Sprint C conformance validation.
	report := cc.validator.Validate(doc)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"report":  report,
	})
}

// readXMLBody reads the raw CDA XML from the request, supporting both
// XML content types (raw body) and JSON body {"xml": "..."}.
//
// The body is limited to 50 MB to prevent oversized document DoS.
func (cc *CDAValidationController) readXMLBody(c *gin.Context) (string, error) {
	const maxBytes = 50 * 1024 * 1024 // 50 MB

	ct := c.GetHeader("Content-Type")
	if strings.Contains(ct, "application/xml") || strings.Contains(ct, "text/xml") {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes))
		if err != nil {
			return "", fmt.Errorf("cannot read request body: %w", err)
		}
		return string(body), nil
	}

	// Default: JSON body {"xml": "..."}
	var payload struct {
		XML string `json:"xml"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		return "", fmt.Errorf(
			`expected JSON body {"xml": "..."} or Content-Type: application/xml: %w`, err)
	}
	return payload.XML, nil
}
