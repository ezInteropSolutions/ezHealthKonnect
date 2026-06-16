// controllers/cda_schema_controller.go
// CDASchemaController — REST API for the CDA step-builder wizard.
//
// Endpoints:
//   GET  /api/cda/schema/sections                          → all sections from ccda_2_1.json
//   GET  /api/cda/schema/sections/:sectionKey/fields       → fields for one section
//   POST /api/cda/type-pair/infer                          → infer transform from (cdaDataType, fhirDataType)
//   GET  /api/cda/mappings/:interfaceId/:documentType      → merged OOB + interface delta
//   POST /api/cda/mappings/:interfaceId/:documentType/delta → save delta (wizard save path)
//   GET  /api/cda/templates/:documentType/version          → current OOB template version

package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	cdaSchema "ezhealthkonnect/cda"
	cdafhir "ezhealthkonnect/services/cda_fhir"

	"github.com/gin-gonic/gin"
)

// CDASchemaController exposes CDA schema browser and mapping delta management endpoints.
type CDASchemaController struct {
	db           *sql.DB
	schemaLoader *cdaSchema.CDASchemaLoader
	mapper       *cdafhir.GenericCDAFHIRMapper
	transformReg *cdafhir.CDATransformRegistry
}

// NewCDASchemaController constructs the controller.
// schemaLoader may be nil when the schema file is unavailable — schema endpoints
// will return 503 in that case. mapper is used for merged mapping resolution.
func NewCDASchemaController(
	db *sql.DB,
	loader *cdaSchema.CDASchemaLoader,
	mapper *cdafhir.GenericCDAFHIRMapper,
) *CDASchemaController {
	return &CDASchemaController{
		db:           db,
		schemaLoader: loader,
		mapper:       mapper,
		transformReg: cdafhir.NewCDATransformRegistry(),
	}
}

// RegisterRoutes mounts all CDA endpoints under the provided RouterGroup.
// Caller passes api.Group("/cda") from main.go.
func (cc *CDASchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	schema := rg.Group("/schema")
	{
		schema.GET("/sections", cc.GetSections)
		schema.GET("/sections/:sectionKey/fields", cc.GetSectionFields)
	}
	rg.POST("/type-pair/infer", cc.InferTypePair)
	rg.GET("/mappings/:interfaceId/:documentType", cc.GetMappings)
	rg.POST("/mappings/:interfaceId/:documentType/delta", cc.SaveMappingDelta)
	rg.POST("/mappings/:interfaceId/:documentType/compute-delta", cc.ComputeDelta)
	rg.GET("/templates/:documentType/version", cc.GetTemplateVersion)
}

// =========================================================
// GET /api/cda/schema/sections
// =========================================================

type sectionSummary struct {
	Key          string `json:"key"`
	DisplayName  string `json:"displayName"`
	LOINCCode    string `json:"loincCode"`
	FHIRResource string `json:"fhirResource,omitempty"`
	Conformance  string `json:"conformance"`
	IsHeader     bool   `json:"isHeader,omitempty"`
	FieldCount   int    `json:"fieldCount"`
}

// GetSections returns a summary list of all sections defined in ccda_2_1.json.
func (cc *CDASchemaController) GetSections(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	sections := cc.schemaLoader.AllSections()
	result := make([]sectionSummary, 0, len(sections))
	for _, s := range sections {
		result = append(result, sectionSummary{
			Key:         s.Key,
			DisplayName: s.DisplayName,
			LOINCCode:   s.LOINCCode,
			Conformance: s.Conformance,
			IsHeader:    s.IsHeader,
			FieldCount:  len(s.Fields),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sections": result,
		"count":    len(result),
	})
}

// =========================================================
// GET /api/cda/schema/sections/:sectionKey/fields
// =========================================================

type fieldSummary struct {
	Key          string `json:"key"`
	USCDIElement string `json:"uscdiElement"`
	DataType     string `json:"dataType"`
	Conformance  string `json:"conformance"`
	ValueSet     string `json:"valueSet,omitempty"`
	XPath        string `json:"xpath"`
}

// GetSectionFields returns all field definitions for a given section key.
func (cc *CDASchemaController) GetSectionFields(c *gin.Context) {
	sectionKey := c.Param("sectionKey")

	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded",
		})
		return
	}

	section := cc.schemaLoader.GetSection(sectionKey)
	if section == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("section %q not found in CDA schema", sectionKey),
		})
		return
	}

	fields := make([]fieldSummary, 0, len(section.Fields))
	for _, f := range section.Fields {
		fields = append(fields, fieldSummary{
			Key:          f.Key,
			USCDIElement: f.USCDIElement,
			DataType:     f.DataType,
			Conformance:  f.Conformance,
			ValueSet:     f.ValueSet,
			XPath:        f.XPath,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"sectionKey": sectionKey,
		"displayName": section.DisplayName,
		"loincCode":  section.LOINCCode,
		"fields":     fields,
		"count":      len(fields),
	})
}

// =========================================================
// POST /api/cda/type-pair/infer
// =========================================================

// InferTypePair returns the default transform name for a (cdaDataType, fhirDataType) pair.
func (cc *CDASchemaController) InferTypePair(c *gin.Context) {
	var body struct {
		CDADataType  string `json:"cdaDataType"`
		FHIRDataType string `json:"fhirDataType"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "request body must contain cdaDataType and fhirDataType",
		})
		return
	}
	if body.CDADataType == "" || body.FHIRDataType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "cdaDataType and fhirDataType are required",
		})
		return
	}

	transform, err := cc.transformReg.InferTransform(body.CDADataType, body.FHIRDataType)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"inferred":  false,
			"transform": "",
			"message":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"inferred":  true,
		"transform": transform,
	})
}

// =========================================================
// GET /api/cda/mappings/:interfaceId/:documentType
// =========================================================

// GetMappings returns the merged effective CDA field mappings for the given interface
// and document type. Priority: interface delta → OOB template (mirrors HL7 getFieldMappings).
func (cc *CDASchemaController) GetMappings(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	documentType := c.Param("documentType")

	if cc.mapper == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA mapper not initialised",
		})
		return
	}

	mappings, err := cc.mapper.GetCDAFieldMappingsPublic(
		c.Request.Context(),
		documentType, "2.1", "R4", interfaceID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"interfaceId":  interfaceID,
		"documentType": documentType,
		"mappings":     mappings,
		"count":        len(mappings),
	})
}

// =========================================================
// POST /api/cda/mappings/:interfaceId/:documentType/delta
// =========================================================

// SaveMappingDelta upserts an interface_cda_mappings row with the supplied delta.
// Body is a CDAMappingDelta JSON (standard format from the wizard save path).
func (cc *CDASchemaController) SaveMappingDelta(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	documentType := c.Param("documentType")

	if cc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "database not available",
		})
		return
	}

	var delta cdafhir.CDAMappingDelta
	if err := c.ShouldBindJSON(&delta); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid delta payload: " + err.Error(),
		})
		return
	}

	deltaJSON, err := json.Marshal(delta)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to serialise delta: " + err.Error(),
		})
		return
	}

	var standardTemplateID *string
	if delta.BaseTemplateID != "" {
		standardTemplateID = &delta.BaseTemplateID
	}

	err = upsertCDAMappingDelta(
		c.Request.Context(), cc.db,
		interfaceID, documentType,
		standardTemplateID, delta.BasedOnVersion,
		string(deltaJSON),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"interfaceId":     interfaceID,
		"documentType":    documentType,
		"overridesStored": len(delta.Overrides),
	})
}

// upsertCDAMappingDelta performs an INSERT … ON CONFLICT DO UPDATE on interface_cda_mappings.
func upsertCDAMappingDelta(
	ctx context.Context, db *sql.DB,
	interfaceID, documentType string,
	standardTemplateID *string, basedOnVersion string,
	deltaJSON string,
) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO interface_cda_mappings
		    (interface_id, document_type, uses_standard_template, standard_template_id,
		     mapping_overrides, updated_at)
		VALUES ($1, $2, true, $3::uuid, $4::jsonb, NOW())
		ON CONFLICT (interface_id, document_type)
		DO UPDATE SET
		    uses_standard_template = true,
		    standard_template_id   = EXCLUDED.standard_template_id,
		    mapping_overrides       = EXCLUDED.mapping_overrides,
		    updated_at              = NOW()
	`, interfaceID, documentType, standardTemplateID, deltaJSON)
	if err != nil {
		return fmt.Errorf("cda_schema: upsert delta: %w", err)
	}
	return nil
}

// =========================================================
// GET /api/cda/templates/:documentType/version
// =========================================================

// =========================================================
// POST /api/cda/mappings/:interfaceId/:documentType/compute-delta
// =========================================================

// ComputeDelta accepts a flat []CDAAtomicMapping from the wizard and computes a
// sparse CDAMappingDelta by diffing against the current OOB template.
// If the incoming matches OOB exactly, no delta row is persisted and the
// response carries overridesStored=0. Otherwise the delta is persisted via
// upsertCDAMappingDelta and the versioning anchors are returned to the caller
// so the step config can be updated (standardTemplateId, basedOnVersion).
func (cc *CDASchemaController) ComputeDelta(c *gin.Context) {
	interfaceID  := c.Param("interfaceId")
	documentType := c.Param("documentType")

	if cc.mapper == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA mapper not initialised",
		})
		return
	}

	var body struct {
		Incoming    []cdafhir.CDAAtomicMapping `json:"incoming"`
		CCDAVersion string                     `json:"ccdaVersion"` // default "2.1"
		FHIRVersion string                     `json:"fhirVersion"` // default "R4"
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body: " + err.Error(),
		})
		return
	}

	ccdaVersion := body.CCDAVersion
	if ccdaVersion == "" {
		ccdaVersion = "2.1"
	}
	fhirVersion := body.FHIRVersion
	if fhirVersion == "" {
		fhirVersion = "R4"
	}

	delta, templateID, version, err := cc.mapper.ComputeCDADelta(
		c.Request.Context(),
		documentType, ccdaVersion, fhirVersion,
		body.Incoming,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Pure OOB — nothing to persist.
	if delta == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":         true,
			"interfaceId":     interfaceID,
			"documentType":    documentType,
			"templateId":      templateID,
			"version":         version,
			"overridesStored": 0,
		})
		return
	}

	// Persist the delta.
	deltaJSON, err := json.Marshal(delta)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to serialise delta: " + err.Error(),
		})
		return
	}

	var tmplPtr *string
	if templateID != "" {
		tmplPtr = &templateID
	}
	if upsertErr := upsertCDAMappingDelta(
		c.Request.Context(), cc.db,
		interfaceID, documentType,
		tmplPtr, version,
		string(deltaJSON),
	); upsertErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   upsertErr.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"interfaceId":     interfaceID,
		"documentType":    documentType,
		"templateId":      templateID,
		"version":         version,
		"overridesStored": len(delta.Overrides),
	})
}

// =========================================================
// GET /api/cda/templates/:documentType/version
// =========================================================

// GetTemplateVersion returns the version string of the current default OOB template
// for the given document type. Used by the step builder to detect upgrade availability.
func (cc *CDASchemaController) GetTemplateVersion(c *gin.Context) {
	documentType := c.Param("documentType")

	if cc.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "database not available",
		})
		return
	}

	var (
		templateID  string
		configJSON  string
	)
	err := cc.db.QueryRowContext(c.Request.Context(), `
		SELECT id::text, template_config::text
		FROM cda_fhir_templates
		WHERE document_type = $1
		  AND ccda_version = '2.1'
		  AND is_default = true
		ORDER BY us_core_version DESC
		LIMIT 1
	`, documentType).Scan(&templateID, &configJSON)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no OOB template found for document type %q", documentType),
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Extract version from template_config JSON
	var cfg struct {
		Version string `json:"version"`
	}
	version := "1.0"
	if jsonErr := json.Unmarshal([]byte(configJSON), &cfg); jsonErr == nil && cfg.Version != "" {
		version = cfg.Version
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"documentType": documentType,
		"templateId":   templateID,
		"version":      version,
	})
}
