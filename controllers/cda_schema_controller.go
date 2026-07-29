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
//   GET  /api/cda/csv/sections                              → OOB cda.section_to_csv column catalog (name/path/fallbacks per section)
//   GET  /api/cda/dedupe/sections                           → OOB cda.dedupe identity-field catalog (name/path/default per section)
//   GET  /api/cda/dedupe/registry                           → admin-only: browse cda_dedupe_registry rows for one interface(+patient)
//   DELETE /api/cda/dedupe/registry                         → admin-only: purge cda_dedupe_registry rows for one interface+patient (GDPR erasure)

package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	cdaBuilder "ezhealthkonnect/cda/builder"
	"ezhealthkonnect/fhir"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	cdaTransform "ezhealthkonnect/services/executors/transform"

	"github.com/gin-gonic/gin"
)

// CDASchemaController exposes CDA schema browser and mapping delta management endpoints.
type CDASchemaController struct {
	db           *sql.DB
	schemaLoader *cdaSchema.CDASchemaLoader
	mapper       *cdafhir.GenericCDAFHIRMapper
	transformReg *cdafhir.CDATransformRegistry
	// declarativeTransformReg is the registry the live declarative engine
	// actually dispatches Fields[].Transform names to (see
	// declarative_rules_flatten.go) — distinct from transformReg above.
	// Also now the source for /type-pair/infer (see InferTypePair): the
	// older transformReg's own typePairDefaults table named 10 of 16
	// transforms that don't exist under those names in THIS registry, so
	// wiring its output into a saved Transform would silently produce an
	// unresolvable config for most type pairs — see
	// DeclarativeTransformRegistry.InferTransform's own doc comment.
	declarativeTransformReg *cdafhir.DeclarativeTransformRegistry
	// fhirSchemaLoader gives GetResourceFields access to real FHIR R4
	// StructureDefinition-derived data (name/description/dataType per
	// property) — used by the Add Field modal's "what to capture" search,
	// which needs official clinical-language descriptions the much thinner
	// USCDI vocabulary (uscdi/vocabulary.go, ~36 elements, no Medications
	// coverage at all) can't provide.
	fhirSchemaLoader *fhir.FHIRSchemaLoader
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
		db:                      db,
		schemaLoader:            loader,
		mapper:                  mapper,
		transformReg:            cdafhir.NewCDATransformRegistry(),
		declarativeTransformReg: cdafhir.NewDeclarativeTransformRegistry(),
		fhirSchemaLoader:        fhir.GetFHIRSchemaLoader(),
	}
}

// RegisterRoutes mounts all CDA endpoints under the provided RouterGroup.
// Caller passes api.Group("/cda") from main.go.
func (cc *CDASchemaController) RegisterRoutes(rg *gin.RouterGroup) {
	schema := rg.Group("/schema")
	{
		schema.GET("/sections", cc.GetSections)
		schema.GET("/sections/:sectionKey/fields", cc.GetSectionFields)
		schema.GET("/resource-fields/:resourceType", cc.GetResourceFields)
		schema.GET("/entry-fields", cc.GetEntryFields)
	}
	rg.GET("/canonical-sections", cc.GetCanonicalSections)
	rg.GET("/canonical-fields/section/:sectionKey", cc.GetCanonicalSectionFields)
	rg.GET("/canonical-fields/section/:sectionKey/repeating-group", cc.GetCanonicalSectionRepeatingGroup)
	rg.GET("/canonical-fields/header/:group", cc.GetCanonicalHeaderFields)
	rg.GET("/canonical-transforms", cc.GetCanonicalTransforms)
	rg.GET("/document-types", cc.GetDocumentTypes)
	rg.GET("/document-types/:documentType/requirements", cc.GetDocumentTypeRequirements)
	rg.GET("/transforms", cc.GetTransforms)
	rg.POST("/type-pair/infer", cc.InferTypePair)
	rg.GET("/mappings/:interfaceId/:documentType", cc.GetMappings)
	rg.GET("/mappings/:interfaceId/:documentType/raw", cc.GetRawMappingDelta)
	rg.POST("/mappings/:interfaceId/:documentType/delta", cc.SaveMappingDelta)
	rg.POST("/mappings/:interfaceId/:documentType/compute-delta", cc.ComputeDelta)
	rg.GET("/templates/:documentType/version", cc.GetTemplateVersion)
	rg.GET("/csv/sections", cc.GetCSVSections)
	rg.GET("/dedupe/sections", cc.GetDedupeSections)
	rg.GET("/dedupe/registry", cc.requireAdminRole(), cc.GetDedupeRegistry)
	rg.DELETE("/dedupe/registry", cc.requireAdminRole(), cc.PurgeDedupeRegistry)
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
// GET /api/cda/canonical-sections
// GET /api/cda/canonical-fields/section/:sectionKey
// GET /api/cda/canonical-fields/header/:group
// =========================================================

// GetCanonicalSections returns every section cda.build/cda.map_to_canonical
// can populate — sourced live from cda/builder.SectionCatalog (a thin
// wrapper over CDASchemaLoader.AllSections()), the cda.map_to_canonical
// step's section-key picker. Deliberately NOT the same list GetSections
// (above) returns: that one is filtered to sections the CDA→FHIR
// declarative engine dispatches, a different consumer with a different
// (smaller) real capability set.
func (cc *CDASchemaController) GetCanonicalSections(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	sections := cdaBuilder.SectionCatalog(cc.schemaLoader)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"sections": sections,
		"count":    len(sections),
	})
}

// GetCanonicalSectionFields returns the canonical field keys
// cda.build/cda.map_to_canonical actually read/write for one section —
// sourced live from cda/builder.SectionFieldCatalog (a thin wrapper over
// CDASchemaLoader.GetSection(key).Fields), NOT the declarative CDA→FHIR
// MappingRule set GetSectionFields (above) exposes — a deliberately
// different dataset for a deliberately different consumer (the
// cda.map_to_canonical step's no-code mapping UI), per the CDA/CCD builder
// Phase 3 design.
func (cc *CDASchemaController) GetCanonicalSectionFields(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	sectionKey := c.Param("sectionKey")
	fields := cdaBuilder.SectionFieldCatalog(cc.schemaLoader, sectionKey)
	if fields == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown section key %q", sectionKey),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"fields":  fields,
		"count":   len(fields),
	})
}

// GetCanonicalSectionRepeatingGroup returns sectionKey's declared
// RepeatingGroup — its canonical groupedItemsKey plus per-item field
// vocabulary (e.g. Vital Signs' "components": vitalCode/value/...) — for the
// cda.map_to_canonical no-code Group By UI. Returns
// {"success":true,"repeatingGroup":null} (200, not 404) for a section that
// simply doesn't declare one — this is the common case (most sections never
// will), not an error condition; 404 stays reserved for a genuinely unknown
// sectionKey, matching GetCanonicalSectionFields' own not-found handling.
func (cc *CDASchemaController) GetCanonicalSectionRepeatingGroup(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	sectionKey := c.Param("sectionKey")
	if cc.schemaLoader.GetSection(sectionKey) == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown section key %q", sectionKey),
		})
		return
	}

	group := cdaBuilder.SectionRepeatingGroupCatalog(cc.schemaLoader, sectionKey)
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"repeatingGroup": group,
	})
}

// GetCanonicalHeaderFields returns the canonical header field keys
// cda.build actually reads for one header group ("patient" | "author" |
// "informant" | "documentationOf" | "documentationOfPerformer" |
// "encompassingEncounter") — sourced live from
// cda/builder.HeaderFieldCatalog.
func (cc *CDASchemaController) GetCanonicalHeaderFields(c *gin.Context) {
	group := c.Param("group")
	fields := cdaBuilder.HeaderFieldCatalog(group)
	if fields == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown header group %q", group),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"fields":  fields,
		"count":   len(fields),
	})
}

// =========================================================
// GET /api/cda/document-types
// GET /api/cda/document-types/:documentType/requirements
// =========================================================

// GetDocumentTypes returns every document type the guided-configuration UIs
// (cda.map_to_canonical, cda.build) can target, sourced from
// CDASchemaLoader.AllDocumentTypes() (the same documentTypeSections map
// cda/builder.BuildDocument itself reads at construction time — never a
// separately-maintained list).
func (cc *CDASchemaController) GetDocumentTypes(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	docTypes := cc.schemaLoader.AllDocumentTypes()
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"documentTypes": docTypes,
		"count":         len(docTypes),
	})
}

// GetDocumentTypeRequirements returns the combined guided-configuration
// catalog for one document type — every SHALL/SHOULD/MAY section plus every
// SHALL/SHOULD header field (patient/author/custodian) — sourced live from
// cda/builder.GetDocumentTypeRequirements. This is what
// MapToCanonicalBuilder.js and CDABuildStepBuilder's Requirements tab both
// consume to pre-populate and score completeness; see
// cda/builder/requirements_catalog.go and header_requirements.go for why
// this is a thin read over already-vetted schema data, not a second,
// hand-guessed conformance judgment.
func (cc *CDASchemaController) GetDocumentTypeRequirements(c *gin.Context) {
	if cc.schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA schema not loaded — check schema directory configuration",
		})
		return
	}

	documentType := c.Param("documentType")
	requirements := cdaBuilder.GetDocumentTypeRequirements(cc.schemaLoader, documentType)
	if requirements == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("unknown document type %q", documentType),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"requirements": requirements,
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
	Key       string `json:"key"`
	CDASource string `json:"cdaSource"`
	// FHIRResource identifies which of the section's MappingRule variant(s)
	// this field belongs to (e.g. "MedicationRequest" vs "MedicationStatement"
	// for the "medications" section) — was silently dropped here before even
	// though cdafhir.FlattenedField already carried it correctly. Needed now
	// because the Add Field UI's "applies to" picker, and a saved add's own
	// display, both depend on it; for a saved multi-resource add this is a
	// "+"-joined display string, not a single resource name.
	FHIRResource string            `json:"fhirResource,omitempty"`
	FHIRPath     string            `json:"fhirPath"`
	Transform    string            `json:"transform"`
	ValueMap     map[string]string `json:"valueMap,omitempty"`
	Conformance  string            `json:"conformance,omitempty"`
	Required     bool              `json:"required,omitempty"`
	NestedUnder  string            `json:"nestedUnder,omitempty"`
	IsModified   bool              `json:"isModified"`
	// Disabled is true when this field has an Action=="remove" override —
	// computed from the RAW override list (LoadCDAMappingOverridesPublic),
	// not the merged mappings map[ff.Key] lookup below, because
	// mergeCDAMappings' "remove" handling drops the field from that merged
	// list entirely, making it indistinguishable from "never overridden."
	Disabled bool `json:"disabled,omitempty"`
	// IsNew marks a field that exists ONLY as a saved Action=="add" override
	// — it has no cdafhir.FlattenedField counterpart at all, since it was
	// never a MappingRow literal to begin with.
	IsNew bool `json:"isNew,omitempty"`
}

// ruleVariantResponse describes one MappingRule sharing this section's
// SectionKey — most sections have exactly one; a few (Medications confirmed)
// have several, distinguished by FHIRResource (+ EntryMatch). Powers the Add
// Field modal's "applies to" and "nest under" pickers.
type ruleVariantResponse struct {
	FHIRResource   string   `json:"fhirResource"`
	EntryMatch     string   `json:"entryMatch,omitempty"`
	NestableGroups []string `json:"nestableGroups,omitempty"`
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

	// ruleVariants surfaces every MappingRule sharing this SectionKey (most
	// sections have exactly one) — the Add Field modal's "applies to" and
	// "nest under" pickers consume this directly, so no separate endpoint
	// is needed for it.
	ruleVariants := make([]ruleVariantResponse, 0, len(rules))
	for _, r := range rules {
		// Deduplicated: some rules deliberately carry TWO rows sharing one
		// TargetPath (e.g. "recorder" — a primary PractitionerRole-based row
		// plus a SkipIfResourceHasAnyOf-guarded bare-practitioner fallback,
		// see declarative_oob_rules.go). Real, intentional data — but for a
		// "nest under" picker the user only cares about the group NAME once.
		seenGroups := make(map[string]bool)
		var groups []string
		for _, row := range r.Fields {
			if len(row.Fields) > 0 && !seenGroups[row.TargetPath] {
				seenGroups[row.TargetPath] = true
				groups = append(groups, row.TargetPath)
			}
		}
		ruleVariants = append(ruleVariants, ruleVariantResponse{
			FHIRResource:   r.FHIRResource,
			EntryMatch:     r.EntryMatch,
			NestableGroups: groups,
		})
	}

	overridesByKey := make(map[string]cdafhir.CDAFieldMapping)
	disabledKeys := make(map[string]bool)
	var addOverrides []cdafhir.CDAMappingOverride
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

		// mergeCDAMappings drops a removed field from `mappings` entirely,
		// so the raw override list is the only place "this field was
		// removed" — or "add"ed — is still visible; see
		// flattenedFieldResponse.Disabled/.IsNew's own doc comments.
		for _, ov := range cc.mapper.LoadCDAMappingOverridesPublic(c.Request.Context(), interfaceID, documentType) {
			if ov.SectionKey != sectionKey {
				continue
			}
			switch ov.Action {
			case "remove":
				disabledKeys[ov.CDAField] = true
			case "add":
				addOverrides = append(addOverrides, ov)
			}
		}
	}

	fields := make([]flattenedFieldResponse, 0, len(flattened)+len(addOverrides))
	for _, ff := range flattened {
		resp := flattenedFieldResponse{
			Key:          ff.Key,
			CDASource:    ff.CDASourceDisplay,
			FHIRResource: ff.FHIRResource,
			FHIRPath:     ff.TargetPath,
			Transform:    ff.Transform,
			ValueMap:     ff.ValueMap,
			Conformance:  ff.Conformance,
			Required:     ff.Required,
			NestedUnder:  ff.NestedUnder,
		}
		if fm, ok := overridesByKey[ff.Key]; ok {
			resp.FHIRPath = fm.FHIRPath
			resp.Transform = fm.Transform
			resp.ValueMap = fm.ValueMap
			resp.Conformance = fm.Conformance
			resp.Required = fm.Required
			resp.IsModified = fm.FHIRPath != ff.TargetPath || fm.Transform != ff.Transform
		}
		if disabledKeys[ff.Key] {
			resp.Disabled = true
			resp.IsModified = true
		}
		fields = append(fields, resp)
	}

	// A saved Action=="add" override has no cdafhir.FlattenedField
	// counterpart at all — without this, a saved custom field would vanish
	// from the table on reload, since the loop above only ever walks the
	// OOB-derived `flattened` list.
	for _, ov := range addOverrides {
		fields = append(fields, flattenedFieldResponse{
			Key:          ov.CDAField,
			CDASource:    cdafhir.DescribeAddedSource(ov.Scope, ov.SourcePath),
			FHIRResource: strings.Join(ov.TargetFHIRResources, "+"),
			FHIRPath:     ov.FHIRPath,
			Transform:    ov.Transform,
			NestedUnder:  ov.NestedUnder,
			IsNew:        true,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"ruleVariants": ruleVariants,
		"sectionKey":   sectionKey,
		"displayName":  displayName,
		"loincCode":    loincCode,
		"fields":       fields,
		"count":        len(fields),
	})
}

// =========================================================
// GET /api/cda/schema/resource-fields/:resourceType
// =========================================================

// resourceFieldResponse is one property of a FHIR resource type, sourced from
// a real R4 StructureDefinition (via fhir.FHIRSchemaLoader) — official name/
// description/dataType text, not the much thinner USCDI vocabulary used by
// /api/fhir/field-search elsewhere in the app (uscdi/vocabulary.go covers
// ~36 hand-picked elements total with zero Medications coverage; this
// endpoint covers every property HL7 actually defines on the resource).
type resourceFieldResponse struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DataType    string `json:"dataType"`
	Cardinality string `json:"cardinality"`
	Required    bool   `json:"required"`
	ValueSet    string `json:"valueSet,omitempty"`
}

// GetResourceFields returns every element of a FHIR R4 resource type (e.g.
// "AllergyIntolerance") from its real StructureDefinition, for the Add Field
// modal's "what to capture" search — matching against Name+Description text
// lets a user search in clinical language ("criticality", "reaction
// severity") instead of needing to already know CDA structure.
//
// Known limitation, inherited from cmd/build-fhir-schemas/build_fhir_schemas.py's
// get_element_type() (keeps only the first type.code): a choice-type element
// like "onset[x]" exposes ONE guessed DataType, not the full union
// (dateTime|Age|Period|Range|string) — not fixable without re-running that
// build script against the vendored FHIR package.
func (cc *CDASchemaController) GetResourceFields(c *gin.Context) {
	resourceType := c.Param("resourceType")

	if cc.fhirSchemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema loader not initialised",
		})
		return
	}

	schema, err := cc.fhirSchemaLoader.LoadFHIRSchema(resourceType, "us-core", "R4")
	if err != nil {
		schema, err = cc.fhirSchemaLoader.LoadFHIRSchema(resourceType, "base", "R4")
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("no FHIR R4 schema found for resource type %q", resourceType),
		})
		return
	}

	elements := make([]resourceFieldResponse, 0, len(schema.Elements))
	for _, el := range schema.Elements {
		elements = append(elements, resourceFieldResponse{
			Path:        el.Path,
			Name:        el.Name,
			Description: el.Description,
			DataType:    el.DataType,
			Cardinality: el.Cardinality,
			Required:    el.Required,
			ValueSet:    el.ValueSet,
		})
	}
	sort.Slice(elements, func(i, j int) bool { return elements[i].Path < elements[j].Path })

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"resourceType": resourceType,
		"elements":     elements,
		"count":        len(elements),
	})
}

// cdaDirectEntryFields is a hand-authored catalog of the direct, top-level
// fields available on any CDA <entry> act (cda/document/types.go's CDAEntry
// struct) — the fields the "Direct field on this entry" Add Field pattern
// lets a user reach without any relationship traversal. Descriptions are
// paraphrased from CDAEntry's own doc comments (the real clinical meaning
// already documented there), not invented — there is no vendored/generated
// CDA schema equivalent to FHIR's StructureDefinition .gz files (CDAFieldDef
// in cda/schema_loader.go only carries a short USCDI label, no description),
// so unlike GetResourceFields this list is maintained by hand and should be
// extended if CDAEntry gains new direct fields.
var cdaDirectEntryFields = []resourceFieldResponse{
	{Path: "code", Name: "Code", Description: "The coded clinical concept for this entry — e.g. the allergy substance, problem, or procedure code.", DataType: "CD"},
	{Path: "statusCode", Name: "Status Code", Description: "The act's current status, e.g. \"active\", \"completed\", \"aborted\".", DataType: "CS"},
	{Path: "effectiveTime", Name: "Effective Time", Description: "When this act was or is effective — a single date/time or a date range (low/high).", DataType: "TS / IVL_TS"},
	{Path: "value", Name: "Value", Description: "The observation's result value — a coded value, quantity, or text depending on the entry template.", DataType: "ANY (CD/PQ/ST/…)"},
	{Path: "text", Name: "Text", Description: "This entry's own free-text content — used by templates that carry content as narrative rather than a coded value (e.g. Medication Free Text Sig, Instruction).", DataType: "ST"},
	{Path: "negationInd", Name: "Negation Indicator", Description: "Marks this act as negated — e.g. \"not given\" on an Immunization, or \"no known allergy\" on an Allergy Observation.", DataType: "BL"},
	{Path: "interpretationCode", Name: "Interpretation Code", Description: "The Result Observation's interpretation, e.g. High/Low/Normal/Abnormal.", DataType: "CD"},
	{Path: "referenceRangeText", Name: "Reference Range Text", Description: "The free-text normal reference range for a lab result, e.g. \"4.0 - 10.0 Thou/uL\".", DataType: "ST"},
	{Path: "repeatNumber", Name: "Repeat Number", Description: "Refill/allowed-administration count, or occurrence number, for a medication administration.", DataType: "INT"},
	{Path: "routeCode", Name: "Route Code", Description: "How a medication is administered, e.g. oral, intravenous, topical.", DataType: "CD"},
	{Path: "doseQuantity", Name: "Dose Quantity", Description: "The amount of medication given per administration.", DataType: "PQ"},
	{Path: "rateQuantity", Name: "Rate Quantity", Description: "The rate of administration for an infusion or continuous medication.", DataType: "PQ"},
	{Path: "quantity", Name: "Quantity (Supply)", Description: "The quantity dispensed or supplied — e.g. a medication refill/dispense amount.", DataType: "PQ"},
	{Path: "targetSiteCode", Name: "Target Site Code", Description: "The anatomical site a procedure was performed on.", DataType: "CD"},
	{Path: "classCode", Name: "Class Code", Description: "The HL7 act class for this entry, e.g. observation, act, procedure, substance administration.", DataType: "CS"},
	{Path: "moodCode", Name: "Mood Code", Description: "Whether this act already happened, is ordered, or is intended.", DataType: "CS"},
	{Path: "nullFlavor", Name: "Null Flavor", Description: "Why a value is absent, e.g. \"no information\", \"unknown\", \"not applicable\".", DataType: "CS"},
	{Path: "id", Name: "Id", Description: "The entry's own unique instance identifier(s).", DataType: "II"},
	{Path: "approachSiteCode", Name: "Approach Site Code", Description: "The anatomical site of administration, e.g. \"Right Deltoid\" for an injection — used by medication and immunization entries.", DataType: "CD"},
	{Path: "administrationUnitCode", Name: "Administration Unit Code", Description: "The dosage form/unit, e.g. \"CAPSULE, DELAYED RELEASE\" — distinct from Dose Quantity's numeric amount+unit.", DataType: "CD"},
	{Path: "maxDoseQuantity", Name: "Max Dose Quantity", Description: "The maximum allowed dose per time period — usually only the numerator (max single/daily dose amount) is populated in real documents.", DataType: "RTO"},
	{Path: "precondition", Name: "Precondition", Description: "The clinical criterion that must be met before administering (PRN conditions).", DataType: "CD"},
	{Path: "relatedSubjectCode", Name: "Related Subject Code", Description: "The Family History Organizer's family member relationship to the patient, e.g. \"mother\", \"father\".", DataType: "CD"},
}

// GetEntryFields returns the hand-authored cdaDirectEntryFields catalog, in
// the same response shape as GetResourceFields so the Add Field modal's
// search-result renderer can be reused for both the FHIR-side "what to
// capture" search and the CDA-side "Direct field on this entry" search.
func (cc *CDASchemaController) GetEntryFields(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"elements": cdaDirectEntryFields,
		"count":    len(cdaDirectEntryFields),
	})
}

// =========================================================
// GET /api/cda/transforms
// =========================================================

type transformSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetTransforms returns every registered CDA→FHIR transform name with its
// human-readable description. The Section Field Editor fetches this once per
// step-builder instance and caches it client-side, so hovering/focusing a
// Transform cell can show what the named function actually does instead of
// just repeating its (often cryptic) name.
func (cc *CDASchemaController) GetTransforms(c *gin.Context) {
	if cc.declarativeTransformReg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "transform registry not initialised",
		})
		return
	}

	descriptions := cc.declarativeTransformReg.AllDescriptions()
	transforms := make([]transformSummary, 0, len(descriptions))
	for name, desc := range descriptions {
		transforms = append(transforms, transformSummary{Name: name, Description: desc})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"transforms": transforms,
		"count":      len(transforms),
	})
}

// =========================================================
// GET /api/cda/canonical-transforms
// =========================================================

// GetCanonicalTransforms returns every registered cda.map_to_canonical
// value-transform name with its human-readable description — mirrors
// GetTransforms above exactly, but sources from
// transform.CanonicalTransformDescriptions() (services/executors/transform/
// canonical_value_transforms.go), a deliberately separate, CDA-target-only
// registry: see that file's doc comment for why declarativeTransformReg
// above (FHIR-resource-shaped output) isn't reusable for this step's
// Transform picker.
func (cc *CDASchemaController) GetCanonicalTransforms(c *gin.Context) {
	descriptions := cdaTransform.CanonicalTransformDescriptions()
	transforms := make([]transformSummary, 0, len(descriptions))
	for name, desc := range descriptions {
		transforms = append(transforms, transformSummary{Name: name, Description: desc})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"transforms": transforms,
		"count":      len(transforms),
	})
}

// =========================================================
// POST /api/cda/type-pair/infer
// =========================================================

// InferTypePair returns the default transform name for a (cdaDataType, fhirDataType) pair.
// Uses declarativeTransformReg (the runtime engine's own registry), not the
// older transformReg — see this struct's own field doc comment for why.
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

	transform, err := cc.declarativeTransformReg.InferTransform(body.CDADataType, body.FHIRDataType)
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
// GET /api/cda/mappings/:interfaceId/:documentType/raw
// =========================================================

// GetRawMappingDelta returns the RAW, unmerged CDAMappingDelta stored for one
// interface+docType (as opposed to GetMappings' merged OOB+delta view) --
// the exact shape SaveMappingDelta expects as its POST body, so the pipeline
// builder's JSON Export/Import can round-trip an interface's actual custom
// field mappings alongside the step config. "delta": null (with success:
// true) means the interface has no custom overrides for this doc type --
// not an error, just nothing to carry.
func (cc *CDASchemaController) GetRawMappingDelta(c *gin.Context) {
	interfaceID := c.Param("interfaceId")
	documentType := c.Param("documentType")

	if cc.mapper == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "CDA mapper not initialised",
		})
		return
	}

	delta := cc.mapper.LoadCDAMappingDeltaPublic(c.Request.Context(), interfaceID, documentType)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"interfaceId":  interfaceID,
		"documentType": documentType,
		"delta":        delta, // null when no custom overrides exist
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
	interfaceID := c.Param("interfaceId")
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

	// Reject a bad "add" override (unknown resource, unknown nested group,
	// missing fhirPath) before it's ever persisted — the runtime engine
	// (applyAddOverride) stays purely defensive/silent about these same
	// cases by design, so this is the only place they're actually caught.
	if delta != nil {
		for _, ov := range delta.Overrides {
			if err := cdafhir.ValidateAddOverride(ov); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   err.Error(),
				})
				return
			}
		}
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
		templateID string
		configJSON string
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
