// controllers/cda_schema_controller.go
// CDASchemaController — REST API for the CDA step-builder wizard.
//
// Endpoints:
//   GET  /api/cda/schema/sections                          → sections the live declarative engine dispatches (ccda_2_1.json used only for cosmetic metadata)
//   GET  /api/cda/schema/sections/:sectionKey/fields       → real declarative MappingRow fields for one section, optionally merged with an interface's saved override
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

// GetSections returns a summary list of every section the live declarative
// engine actually dispatches (cdafhir.DeclarativeDispatchedSectionKeys()),
// cross-referenced against ccda_2_1.json for cosmetic metadata only
// (displayName/LOINC code/conformance) — never for field content. A section
// present in ccda_2_1.json but with no declarative rule group registered is
// deliberately excluded: it can't be edited here because the engine
// wouldn't apply the edit anyway.
func (cc *CDASchemaController) GetSections(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	dispatched := make(map[string]bool)
	for _, k := range cdafhir.DeclarativeDispatchedSectionKeys() {
		dispatched[k] = true
	}

	sections := cc.schemaLoader.AllSections()
	result := make([]sectionSummary, 0, len(sections))
	for _, s := range sections {
		if !dispatched[s.Key] {
			continue
		}
		fieldCount := len(s.Fields)
		if cc.mapper != nil {
			fieldCount = cc.mapper.CountDeclarativeFields(s.Key)
		}
		result = append(result, sectionSummary{
			Key:         s.Key,
			DisplayName: s.DisplayName,
			LOINCCode:   s.LOINCCode,
			Conformance: s.Conformance,
			IsHeader:    s.IsHeader,
			FieldCount:  fieldCount,
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

// fieldSummary was the old ccda_2_1.json-backed response shape for
// GetSectionFields. Superseded by flattenedFieldResponse (real declarative
// rule fields, not static USCDI schema fields) — left in place as it's
// otherwise-unused dead code, not deleted, since this static schema is
// still legitimately read elsewhere (GetSections' cosmetic metadata).
type fieldSummary struct {
	Key          string `json:"key"`
	USCDIElement string `json:"uscdiElement"`
	DataType     string `json:"dataType"`
	Conformance  string `json:"conformance"`
	ValueSet     string `json:"valueSet,omitempty"`
	XPath        string `json:"xpath"`
}

// flattenedFieldResponse is one row of the Section Field Editor table —
// sourced from cdafhir.FlattenSectionRules (the real declarative MappingRow
// set), merged with any saved interface-level override.
type flattenedFieldResponse struct {
	Key         string            `json:"key"`
	CDASource   string            `json:"cdaSource"`
	FHIRPath    string            `json:"fhirPath"`
	Transform   string            `json:"transform"`
	ValueMap    map[string]string `json:"valueMap,omitempty"`
	Conformance string            `json:"conformance,omitempty"`
	Required    bool              `json:"required,omitempty"`
	NestedUnder string            `json:"nestedUnder,omitempty"`
	IsModified  bool              `json:"isModified"`
}

// GetSectionFields returns every real, addressable field for a section,
// sourced live from the declarative engine's own MappingRule set (NOT
// ccda_2_1.json — see this file's header comment / Phase reconciliation
// note). Optional query params:
//
//	documentType — defaults to "CCD"
//	interfaceId  — when set, each field's fhirPath/transform/etc. reflect
//	               that interface's saved override (if any), and isModified
//	               is set accordingly. Omitted means pure OOB.
func (cc *CDASchemaController) GetSectionFields(c *gin.Context) {
	sectionKey := c.Param("sectionKey")
	documentType := c.DefaultQuery("documentType", "CCD")
	interfaceID := c.Query("interfaceId")

	if cc.mapper == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA mapper not initialised",
		})
		return
	}

	rules := cdafhir.DeclarativeSectionRules(sectionKey)
	if len(rules) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("section %q has no live declarative rule group", sectionKey),
		})
		return
	}
	flattened := cdafhir.FlattenSectionRules(rules)

	var displayName, loincCode string
	if cc.schemaLoader != nil {
		if section := cc.schemaLoader.GetSection(sectionKey); section != nil {
			displayName = section.DisplayName
			loincCode = section.LOINCCode
		}
	}

	overridesByKey := make(map[string]cdafhir.CDAFieldMapping)
	if interfaceID != "" {
		mappings, err := cc.mapper.GetCDAFieldMappingsPublic(c.Request.Context(), documentType, "2.1", "R4", interfaceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		for _, fm := range mappings {
			if fm.SectionKey == sectionKey {
				overridesByKey[fm.CDAField] = fm
			}
		}
	}

	fields := make([]flattenedFieldResponse, 0, len(flattened))
	for _, ff := range flattened {
		resp := flattenedFieldResponse{
			Key:         ff.Key,
			CDASource:   ff.CDASourceDisplay,
			FHIRPath:    ff.TargetPath,
			Transform:   ff.Transform,
			ValueMap:    ff.ValueMap,
			Conformance: ff.Conformance,
			Required:    ff.Required,
			NestedUnder: ff.NestedUnder,
		}
		if fm, ok := overridesByKey[ff.Key]; ok {
			resp.FHIRPath = fm.FHIRPath
			resp.Transform = fm.Transform
			resp.ValueMap = fm.ValueMap
			resp.Conformance = fm.Conformance
			resp.Required = fm.Required
			resp.IsModified = fm.FHIRPath != ff.TargetPath || fm.Transform != ff.Transform
		}
		fields = append(fields, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"sectionKey":  sectionKey,
		"displayName": displayName,
		"loincCode":   loincCode,
		"fields":      fields,
		"count":       len(fields),
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

	// Pure OOB — clear any previously-stored override (this is what makes
	// "reset every field back to OOB, then save" actually take effect: a
	// nil delta here means the incoming list now matches OOB exactly, so any
	// prior interface_cda_mappings row's mapping_overrides must be cleared,
	// not silently left in place — leaving it would mean the OLD override
	// keeps applying at runtime even after the user reset every field).
	if delta == nil {
		if cc.db != nil {
			if _, err := cc.db.ExecContext(c.Request.Context(), `
				UPDATE interface_cda_mappings
				SET mapping_overrides = NULL, updated_at = NOW()
				WHERE interface_id = $1 AND document_type = $2
			`, interfaceID, documentType); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   fmt.Sprintf("clearing stale overrides: %v", err),
				})
				return
			}
		}
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
