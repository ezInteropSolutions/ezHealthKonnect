// services/cda_fhir/generic_mapper.go
// GenericCDAFHIRMapper — schema-driven CDA→FHIR transformation engine.
//
// Mirrors the HL7→FHIR v3 engine (HL7FHIRTransformServiceV3) exactly:
//   Template lookup:  getCDAFieldMappings() → interface_cda_mappings priority, then OOB
//   Field resolution: []CDAFieldMapping flat list keyed by sectionKey + cdaField
//   Delta merging:    mergeCDAMappings() applies CDAMappingOverride list to OOB base
//
// Sprint B: type definitions + DB template lookup (getCDAFieldMappings,
//   loadFromCDAOOBTemplates, mergeCDAMappings).
// Sprint C: full Map() execution — createFHIRResourceFromSection(), transform dispatch,
//   US Core profile injection, FHIRNarrativeGenerator, section-failure isolation,
//   processingResult structured output.

package cdafhir

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
	cdaterminology "ezhealthkonnect/services/cda_terminology"
	fhirnarrative "ezhealthkonnect/services/fhir_narrative"
)

// =========================================================
// CDA→FHIR mapping types
// (parallel to FieldMapping / MappingOverride in HL7 engine)
// =========================================================

// CDAFieldMapping is one atomic CDA field → FHIR path mapping.
// Equivalent to FieldMapping in hl7_fhir_transform_service.go.
type CDAFieldMapping struct {
	SectionKey    string            // "allergiesAndIntolerances"
	FHIRResource  string            // "AllergyIntolerance"
	Repeating     bool              // one FHIR resource per CDA entry
	USCoreProfile string            // US Core profile URL for meta.profile
	ContextLinks  map[string]string // cross-resource refs: {"patient":"patient"}
	CDAField      string            // "medicationAllergyCode"  (key from ccda_2_1.json)
	FHIRPath      string            // "AllergyIntolerance.code.coding[0].code"
	CDADataType   string            // "CD", "CE", "PQ", "TS", "CS"
	FHIRDataType  string            // "code", "string", "dateTime", "decimal"
	Transform     string            // named transform function
	ValueMap      map[string]string // inline code translation
	Conformance   string            // "SHALL", "SHOULD", "MAY"
	Required      bool
	Confidence    float64
}

// CDAMappingOverride is one delta operation applied on top of an OOB template.
// Equivalent to MappingOverride in the HL7 engine; extended for CDA section operations.
type CDAMappingOverride struct {
	Action      string            `json:"action"`       // "replace"|"add"|"remove"|"add_section"|"remove_section"
	SectionKey  string            `json:"sectionKey"`   // "allergiesAndIntolerances"
	CDAField    string            `json:"cdaField"`     // "severity"
	FHIRPath    string            `json:"fhirPath,omitempty"`
	Transform   string            `json:"transform,omitempty"`
	ValueMap    map[string]string `json:"valueMap,omitempty"`
	IsRequired  bool              `json:"isRequired,omitempty"`
	Conformance string            `json:"conformance,omitempty"`
	Confidence  float64           `json:"confidence,omitempty"`
	// Used for action="add_section": full new section definition
	SectionConfig map[string]interface{} `json:"sectionConfig,omitempty"`
}

// CDAMappingDelta is the full delta stored in interface_cda_mappings.mapping_overrides.
// Versioned so future tools can detect when a delta was built against an older OOB.
type CDAMappingDelta struct {
	Version        int                  `json:"version"`
	BaseTemplateID string               `json:"base_template_id"` // UUID of OOB template
	BasedOnVersion string               `json:"based_on_version"` // OOB template_config.version at save time
	Overrides      []CDAMappingOverride `json:"overrides"`
}

// CDAToFHIRConfig parameterises a Map() call; mirrors the step builder config tabs.
type CDAToFHIRConfig struct {
	DocType               string   // "CCD", "Discharge Summary", etc.
	CCDAVersion           string   // "2.1" (default)
	FHIRVersion           string   // "R4" (default)
	InterfaceID           string   // for interface-level delta overrides
	BundleType            string   // "collection" (default) | "document" | "transaction"
	ProfileMode           string   // "us-core" (default) | "base"
	EnabledSections       []string // nil = all sections
	OnSectionFailure      string   // "continue" (default) | "fail-fast"
	LogLevel              string   // "error" | "warning" | "info" (default) | "debug"
	TerminologyValidation bool
	MergeMode             string // "append" | "replace"

	// Assembly controls the post-mapping assembly layer (deduplication, panel synthesis).
	// Zero value = assembly enabled with all default rules active.
	Assembly assembly.AssemblyConfig
}

// ProcessingResult is the structured outcome written to _stepOutput.processingResult.
type ProcessingResult struct {
	DocumentType       string         `json:"documentType"`
	CCDAVersion        string         `json:"ccdaVersion"`
	PartialSuccess     bool           `json:"partialSuccess"`
	ResourcesProduced  int            `json:"resourcesProduced"`
	SectionsProcessed  int            `json:"sectionsProcessed"`
	SuccessfulSections []string       `json:"successfulSections"`
	FailedSections     []string       `json:"failedSections"`
	SectionErrors      []SectionError `json:"sectionErrors"`
}

// SectionError carries per-field error details for granular retry targeting.
type SectionError struct {
	SectionKey string `json:"sectionKey"`
	EntryIndex int    `json:"entryIndex"`
	FieldKey   string `json:"fieldKey"`
	Transform  string `json:"transform"`
	Error      string `json:"error"`
	Severity   string `json:"severity"` // "error" | "warning"
}

// MapOutput is the combined return value of Map() and MapDocument().
type MapOutput struct {
	FHIRBundle       map[string]interface{}
	ProcessingResult ProcessingResult
	// MappingLog carries the per-document transformation trace.
	// Summary is returned in HTTP responses; the full log is persisted async.
	MappingLog mappinglog.MappingLog
}

// =========================================================
// GenericCDAFHIRMapper
// =========================================================

// GenericCDAFHIRMapper is the schema-driven CDA→FHIR engine.
// Equivalent to HL7FHIRTransformServiceV3 in the HL7 pipeline.
type GenericCDAFHIRMapper struct {
	db                *sql.DB
	schemaLoader      *cdaSchema.CDASchemaLoader
	transformReg      *CDATransformRegistry
	profileBuilder    *USCoreProfileBuilder
	narrativeGen      *fhirnarrative.FHIRNarrativeGenerator
	terminologySvc    *cdaterminology.TerminologyService // optional; nil = skip validation
	compositionMapper *CompositionMapper                 // for bundleType=document

	// templateCache keys: "<docType>|<ccdaVersion>|<fhirVersion>"
	templateCache map[string][]CDAFieldMapping
	templateMu    sync.RWMutex
}

// NewGenericCDAFHIRMapper constructs a mapper with all dependencies injected.
func NewGenericCDAFHIRMapper(db *sql.DB, loader *cdaSchema.CDASchemaLoader) *GenericCDAFHIRMapper {
	return &GenericCDAFHIRMapper{
		db:                db,
		schemaLoader:      loader,
		transformReg:      NewCDATransformRegistry(),
		profileBuilder:    NewUSCoreProfileBuilder(),
		narrativeGen:      fhirnarrative.NewFHIRNarrativeGenerator(),
		compositionMapper: NewCompositionMapper(),
		templateCache:     make(map[string][]CDAFieldMapping),
	}
}

// WithTerminologyService wires an optional TerminologyService for code validation
// and translation. When set and CDAToFHIRConfig.TerminologyValidation=true,
// coded fields are validated during createFHIRResourceFromSection().
func (m *GenericCDAFHIRMapper) WithTerminologyService(svc *cdaterminology.TerminologyService) *GenericCDAFHIRMapper {
	m.terminologySvc = svc
	return m
}

// =========================================================
// Template lookup (mirror of HL7 getFieldMappings)
// =========================================================

// getCDAFieldMappings returns the effective []CDAFieldMapping for a message,
// applying the three-tier priority:
//  1. Full custom (uses_standard_template=false): uses custom_mapping_config entirely
//  2. Delta      (uses_standard_template=true, overrides≠nil): OOB + merged delta
//  3. Pure OOB   (no row, or row with no overrides): OOB template unchanged
//
// interfaceID="" returns pure OOB (used for template validation at startup).
func (m *GenericCDAFHIRMapper) getCDAFieldMappings(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion, interfaceID string,
) ([]CDAFieldMapping, error) {

	if interfaceID == "" {
		return m.loadFromCDAOOBTemplates(ctx, docType, ccdaVersion, fhirVersion)
	}

	if m.db == nil {
		return nil, fmt.Errorf("cda_fhir: no DB connection — interface template lookup unavailable")
	}

	row := m.db.QueryRowContext(ctx, `
		SELECT uses_standard_template,
		       standard_template_id,
		       custom_mapping_config,
		       mapping_overrides
		FROM interface_cda_mappings
		WHERE interface_id = $1 AND document_type = $2
	`, interfaceID, docType)

	var (
		usesStandard       bool
		standardTemplateID sql.NullString
		customConfigJSON   sql.NullString
		overridesJSON      sql.NullString
	)

	err := row.Scan(&usesStandard, &standardTemplateID, &customConfigJSON, &overridesJSON)
	if err == sql.ErrNoRows {
		return m.loadFromCDAOOBTemplates(ctx, docType, ccdaVersion, fhirVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("cda_fhir: getCDAFieldMappings interface query: %w", err)
	}

	if !usesStandard && customConfigJSON.Valid {
		return m.parseMappingConfig([]byte(customConfigJSON.String))
	}

	oob, err := m.loadFromCDAOOBTemplates(ctx, docType, ccdaVersion, fhirVersion)
	if err != nil {
		return nil, err
	}

	if !overridesJSON.Valid || overridesJSON.String == "" || overridesJSON.String == "null" {
		return oob, nil
	}

	var delta CDAMappingDelta
	if err := json.Unmarshal([]byte(overridesJSON.String), &delta); err != nil {
		return nil, fmt.Errorf("cda_fhir: getCDAFieldMappings delta parse: %w", err)
	}
	return mergeCDAMappings(oob, delta.Overrides), nil
}

// =========================================================
// OOB template loading (mirror of HL7 loadFromV9OOBTemplates)
// =========================================================

func (m *GenericCDAFHIRMapper) loadFromCDAOOBTemplates(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion string,
) ([]CDAFieldMapping, error) {

	if m.db == nil {
		return nil, fmt.Errorf("cda_fhir: no DB connection — OOB template lookup unavailable")
	}

	cacheKey := docType + "|" + ccdaVersion + "|" + fhirVersion

	m.templateMu.RLock()
	if cached, ok := m.templateCache[cacheKey]; ok {
		m.templateMu.RUnlock()
		return cached, nil
	}
	m.templateMu.RUnlock()

	var configJSON string
	err := m.db.QueryRowContext(ctx, `
		SELECT template_config::text
		FROM cda_fhir_templates
		WHERE document_type = $1
		  AND ccda_version   = $2
		  AND fhir_version   = $3
		  AND is_default     = true
		ORDER BY us_core_version DESC
		LIMIT 1
	`, docType, ccdaVersion, fhirVersion).Scan(&configJSON)

	if err == sql.ErrNoRows {
		fallbackErr := m.db.QueryRowContext(ctx, `
			SELECT template_config::text
			FROM cda_fhir_templates
			WHERE document_type = $1 AND is_default = true
			ORDER BY ccda_version DESC, us_core_version DESC
			LIMIT 1
		`, docType).Scan(&configJSON)
		if fallbackErr == sql.ErrNoRows {
			return nil, fmt.Errorf("cda_fhir: no OOB template for document_type=%q", docType)
		}
		if fallbackErr != nil {
			return nil, fmt.Errorf("cda_fhir: OOB template fallback query: %w", fallbackErr)
		}
	} else if err != nil {
		return nil, fmt.Errorf("cda_fhir: OOB template query: %w", err)
	}

	mappings, err := m.parseMappingConfig([]byte(configJSON))
	if err != nil {
		return nil, err
	}

	m.templateMu.Lock()
	m.templateCache[cacheKey] = mappings
	m.templateMu.Unlock()

	return mappings, nil
}

// =========================================================
// JSONB template config → []CDAFieldMapping
// =========================================================

func (m *GenericCDAFHIRMapper) parseMappingConfig(configJSON []byte) ([]CDAFieldMapping, error) {
	var config struct {
		Sections map[string]struct {
			FHIRResource  string            `json:"fhirResource"`
			Repeating     bool              `json:"repeating"`
			USCoreProfile string            `json:"useCoreProfile"`
			ContextLinks  map[string]string `json:"contextLinks"`
			Mappings      []struct {
				CDAField     string            `json:"cdaField"`
				FHIRPath     string            `json:"fhirPath"`
				CDADataType  string            `json:"cdaDataType"`
				FHIRDataType string            `json:"fhirDataType"`
				Transform    string            `json:"transform"`
				ValueMap     map[string]string `json:"valueMap"`
				Conformance  string            `json:"conformance"`
				Required     bool              `json:"required"`
				Confidence   float64           `json:"confidence"`
			} `json:"mappings"`
		} `json:"sections"`
	}

	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("cda_fhir: parseMappingConfig: %w", err)
	}

	var result []CDAFieldMapping
	for sectionKey, secDef := range config.Sections {
		for _, mf := range secDef.Mappings {
			result = append(result, CDAFieldMapping{
				SectionKey:    sectionKey,
				FHIRResource:  secDef.FHIRResource,
				Repeating:     secDef.Repeating,
				USCoreProfile: secDef.USCoreProfile,
				ContextLinks:  secDef.ContextLinks,
				CDAField:      mf.CDAField,
				FHIRPath:      mf.FHIRPath,
				CDADataType:   mf.CDADataType,
				FHIRDataType:  mf.FHIRDataType,
				Transform:     mf.Transform,
				ValueMap:      mf.ValueMap,
				Conformance:   mf.Conformance,
				Required:      mf.Required,
				Confidence:    mf.Confidence,
			})
		}
	}
	return result, nil
}

// =========================================================
// Delta merge (mirror of HL7 mergeMappings)
// =========================================================

func mergeCDAMappings(base []CDAFieldMapping, overrides []CDAMappingOverride) []CDAFieldMapping {
	if len(overrides) == 0 {
		return base
	}

	type key struct{ section, field string }
	index := make(map[key]int, len(base))
	for i, m := range base {
		index[key{m.SectionKey, m.CDAField}] = i
	}

	removedSections := make(map[string]bool)
	result := make([]CDAFieldMapping, len(base))
	copy(result, base)

	for _, ov := range overrides {
		k := key{ov.SectionKey, ov.CDAField}
		switch ov.Action {
		case "replace":
			if idx, ok := index[k]; ok {
				fm := &result[idx]
				if ov.FHIRPath != "" {
					fm.FHIRPath = ov.FHIRPath
				}
				if ov.Transform != "" {
					fm.Transform = ov.Transform
				}
				if ov.ValueMap != nil {
					fm.ValueMap = ov.ValueMap
				}
				if ov.Conformance != "" {
					fm.Conformance = ov.Conformance
				}
				if ov.Confidence > 0 {
					fm.Confidence = ov.Confidence
				}
				fm.Required = ov.IsRequired
			}

		case "add":
			if _, exists := index[k]; !exists {
				newMapping := CDAFieldMapping{
					SectionKey:  ov.SectionKey,
					CDAField:    ov.CDAField,
					FHIRPath:    ov.FHIRPath,
					Transform:   ov.Transform,
					ValueMap:    ov.ValueMap,
					Conformance: ov.Conformance,
					Required:    ov.IsRequired,
					Confidence:  ov.Confidence,
				}
				for _, m := range base {
					if m.SectionKey == ov.SectionKey {
						newMapping.FHIRResource = m.FHIRResource
						newMapping.Repeating = m.Repeating
						newMapping.USCoreProfile = m.USCoreProfile
						newMapping.ContextLinks = m.ContextLinks
						break
					}
				}
				result = append(result, newMapping)
				index[k] = len(result) - 1
			}

		case "remove":
			if idx, ok := index[k]; ok {
				result[idx].CDAField = "" // sentinel for removal
			}

		case "remove_section":
			removedSections[ov.SectionKey] = true

		case "add_section":
			if ov.SectionConfig != nil {
				result = append(result, CDAFieldMapping{
					SectionKey:   ov.SectionKey,
					FHIRResource: fmt.Sprintf("%v", ov.SectionConfig["fhirResource"]),
				})
			}
		}
	}

	filtered := result[:0]
	for _, m := range result {
		if m.CDAField == "" && m.FHIRPath == "" {
			continue
		}
		if removedSections[m.SectionKey] {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// =========================================================
// Map — full Sprint C implementation
// =========================================================

// Map converts a parsed CDA document (USCDI-keyed JSON from CDAParserService)
// to a FHIR R4 Bundle using the DB-driven field mappings.
//
// parsedCDA is the ParsedJSON field from models.ParserResult — a map with:
//   - "_format": "ccda"
//   - "header": { "patient": {...}, "author": {...}, "custodian": {...} }
//   - "sections": { sectionKey: { "entries": [...], "narrativeHTML": "..." }, ... }
func (m *GenericCDAFHIRMapper) Map(
	ctx context.Context,
	parsedCDA map[string]interface{},
	config CDAToFHIRConfig,
) (*MapOutput, error) {
	start := time.Now()

	// Apply config defaults
	if config.CCDAVersion == "" {
		config.CCDAVersion = "2.1"
	}
	if config.FHIRVersion == "" {
		config.FHIRVersion = "R4"
	}
	if config.BundleType == "" {
		config.BundleType = "collection"
	}
	if config.OnSectionFailure == "" {
		config.OnSectionFailure = "continue"
	}
	if config.DocType == "" {
		if dt, ok := parsedCDA["documentType"].(string); ok && dt != "" {
			config.DocType = dt
		} else {
			config.DocType = "CCD"
		}
	}

	// Prefer typed path (Sprint D) — if the CDA parse executor stored a typed
	// *CDADocument in parsedCDA, delegate to MapDocument() which uses zero XPath.
	if typed, ok := parsedCDA["_cdaDocument"]; ok {
		if doc, ok := typed.(*cdadocument.CDADocument); ok {
			return m.MapDocument(ctx, doc, config)
		}
	}

	// Load field mappings
	mappings, err := m.getCDAFieldMappings(ctx, config.DocType, config.CCDAVersion, config.FHIRVersion, config.InterfaceID)
	if err != nil {
		return nil, fmt.Errorf("cda_fhir: Map: %w", err)
	}

	// Warn at startup if any mapping has an unresolvable transform
	if warnings := m.transformReg.ValidateOOBMappings(mappings); len(warnings) > 0 {
		for _, w := range warnings {
			log.Printf("⚠️  [cda.to_fhir] mapping warning: %s", w)
		}
	}

	// Group mappings by sectionKey
	groups := buildSectionGroups(mappings)

	// Prepare result tracking
	pr := ProcessingResult{
		DocumentType: config.DocType,
		CCDAVersion:  config.CCDAVersion,
	}

	var (
		allResources []map[string]interface{}
		mu           sync.Mutex
		wg           sync.WaitGroup

		sectionErrors []SectionError
		failedSects   []string
		successSects  []string
	)

	// Extract data maps from parsedCDA
	headerMap, _ := parsedCDA["header"].(map[string]interface{})
	sectionsMap, _ := parsedCDA["sections"].(map[string]interface{})

	// Legacy map-based header resources (typed path delegates to MapDocument above).
	// patientHeader is kept so injectPatientExtensions receives race/ethnicity data.
	patientHeader := extractHeaderPatient(headerMap)
	patientRef := ""
	if pr := buildLegacyPatient(patientHeader); pr != nil {
		patientRef = "Patient/patient-1"
		allResources = append(allResources, pr)
	}
	if ar := buildLegacyAuthor(headerMap); ar != nil {
		allResources = append(allResources, ar)
	}
	if cr := buildLegacyCustodian(headerMap); cr != nil {
		allResources = append(allResources, cr)
	}

	// Process clinical sections in parallel (section failure isolation)
	for sectionKey, group := range groups {
		if isHeaderSection(sectionKey) {
			continue // header sections handled above
		}
		if !m.isSectionEnabled(sectionKey, config.EnabledSections) {
			continue
		}

		wg.Add(1)
		go func(sk string, grp sectionGroup) {
			defer wg.Done()

			entries := extractSectionEntries(sk, sectionsMap)
			if len(entries) == 0 {
				return
			}

			resources, errs := m.createFHIRResourceFromSection(sk, grp, entries, config)

			mu.Lock()
			defer mu.Unlock()

			if len(errs) > 0 && config.OnSectionFailure == "fail-fast" {
				failedSects = append(failedSects, sk)
				sectionErrors = append(sectionErrors, errs...)
			} else {
				if len(errs) > 0 {
					failedSects = append(failedSects, sk)
					sectionErrors = append(sectionErrors, errs...)
				} else {
					successSects = append(successSects, sk)
				}
				allResources = append(allResources, resources...)
			}
		}(sectionKey, group)
	}
	wg.Wait()

	// Inject US Core profiles and narratives
	for i, r := range allResources {
		rt := strField(r, "resourceType")

		// Profile injection
		templateProfile := ""
		if sg, ok := groups[sectionKeyForResource(rt, groups)]; ok {
			templateProfile = sg.USCoreProfile
		}
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(r, templateProfile)
			if rt == "Patient" {
				m.profileBuilder.injectPatientExtensions(r, patientHeader)
			}
		}

		// Narrative generation
		narrative := m.narrativeGen.Generate(r)
		if narrative != "" {
			r["text"] = map[string]interface{}{
				"status": "generated",
				"div":    narrative,
			}
		}

		allResources[i] = r
	}

	// Apply context links: inject patient reference on all clinical resources
	if patientRef != "" {
		for _, r := range allResources {
			m.injectPatientReference(r, patientRef, groups)
		}
	}

	// Build FHIR bundle entries
	entries := make([]interface{}, 0, len(allResources)+1)

	// For document bundles, Composition MUST be the first entry (FHIR R4 §3.1.3)
	if config.BundleType == "document" && m.compositionMapper != nil {
		comp := m.compositionMapper.BuildComposition(headerMap, allResources, config)
		compID := strField(comp, "id")
		entries = append(entries, map[string]interface{}{
			"fullUrl":  "urn:uuid:Composition/" + compID,
			"resource": comp,
		})
	}

	for _, r := range allResources {
		rt := strField(r, "resourceType")
		id := strField(r, "id")
		fullURL := "urn:uuid:" + rt + "/" + id
		entries = append(entries, map[string]interface{}{
			"fullUrl":  fullURL,
			"resource": r,
		})
	}

	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         config.BundleType,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"entry":        entries,
	}
	if config.ProfileMode != "base" {
		bundle["meta"] = map[string]interface{}{
			"profile": []interface{}{profileDocumentReference},
			"source":  "CDA/" + config.DocType,
		}
	}

	// Build processingResult
	pr.SuccessfulSections = successSects
	pr.FailedSections = failedSects
	pr.SectionErrors = sectionErrors
	pr.ResourcesProduced = len(allResources)
	pr.SectionsProcessed = len(successSects) + len(failedSects)
	pr.PartialSuccess = len(failedSects) > 0 && len(successSects) > 0

	durationMs := time.Since(start).Milliseconds()

	log.Printf("  ✅ [cda.to_fhir] %s → %d resources (%d sections ok, %d failed) in %dms",
		config.DocType, len(allResources), len(successSects), len(failedSects), durationMs)

	// Audit log — non-fatal, written in background goroutine.
	if m.db != nil {
		go m.writeTransformAuditLog(ctx, config, pr, durationMs)
	}

	return &MapOutput{FHIRBundle: bundle, ProcessingResult: pr}, nil
}

// writeTransformAuditLog inserts one audit_logs row per cda.to_fhir Map() call.
// Non-fatal — errors are logged but do not affect the transform result.
func (m *GenericCDAFHIRMapper) writeTransformAuditLog(
	ctx context.Context,
	config CDAToFHIRConfig,
	pr ProcessingResult,
	durationMs int64,
) {
	meta := map[string]interface{}{
		"interfaceId":       config.InterfaceID,
		"documentType":      config.DocType,
		"resourcesProduced": pr.ResourcesProduced,
		"failedSections":    pr.FailedSections,
		"durationMs":        durationMs,
		"partialSuccess":    pr.PartialSuccess,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		log.Printf("[cda.to_fhir] audit marshal error: %v", err)
		return
	}

	entityID := config.InterfaceID
	if entityID == "" {
		entityID = "system"
	}

	_, err = m.db.ExecContext(ctx, `
		INSERT INTO audit_logs
		    (user_id, user_email, action, entity_type, entity_id, metadata, created_at)
		VALUES
		    (NULL, 'system', 'cda_fhir_transform', 'interface', $1, $2::jsonb, NOW())
	`, entityID, string(metaJSON))
	if err != nil {
		log.Printf("[cda.to_fhir] audit write error: %v", err)
	}
}

// GetCDAFieldMappingsPublic exposes getCDAFieldMappings for the schema browser controller.
// Returns the effective flat []CDAFieldMapping for the given parameters.
func (m *GenericCDAFHIRMapper) GetCDAFieldMappingsPublic(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion, interfaceID string,
) ([]CDAFieldMapping, error) {
	return m.getCDAFieldMappings(ctx, docType, ccdaVersion, fhirVersion, interfaceID)
}

// InvalidateCache clears the in-process template cache.
func (m *GenericCDAFHIRMapper) InvalidateCache() {
	m.templateMu.Lock()
	m.templateCache = make(map[string][]CDAFieldMapping)
	m.templateMu.Unlock()
}

// =========================================================
// Section group construction
// =========================================================

type sectionGroup struct {
	FHIRResource  string
	Repeating     bool
	USCoreProfile string
	ContextLinks  map[string]string
	Mappings      []CDAFieldMapping
}

func buildSectionGroups(mappings []CDAFieldMapping) map[string]sectionGroup {
	groups := make(map[string]sectionGroup)
	for _, mf := range mappings {
		sg := groups[mf.SectionKey]
		if sg.FHIRResource == "" {
			sg.FHIRResource = mf.FHIRResource
			sg.Repeating = mf.Repeating
			sg.USCoreProfile = mf.USCoreProfile
			sg.ContextLinks = mf.ContextLinks
		}
		sg.Mappings = append(sg.Mappings, mf)
		groups[mf.SectionKey] = sg
	}
	return groups
}

// =========================================================
// Resource creation from section entries
// =========================================================

func (m *GenericCDAFHIRMapper) createFHIRResourceFromSection(
	sectionKey string,
	group sectionGroup,
	entries []map[string]interface{},
	config CDAToFHIRConfig,
) ([]map[string]interface{}, []SectionError) {
	var (
		resources []map[string]interface{}
		errs      []SectionError
	)

	for entryIdx, entry := range entries {
		resource := map[string]interface{}{
			"resourceType": group.FHIRResource,
			"id":           fmt.Sprintf("%s-%d", resourceIDPrefix(group.FHIRResource), entryIdx+1),
		}

		for _, mf := range group.Mappings {
			transformName := mf.Transform
			if transformName == "" {
				var inferErr error
				transformName, inferErr = m.transformReg.InferTransform(mf.CDADataType, mf.FHIRDataType)
				if inferErr != nil {
					if mf.Required || mf.Conformance == "SHALL" {
						errs = append(errs, SectionError{
							SectionKey: sectionKey,
							EntryIndex: entryIdx,
							FieldKey:   mf.CDAField,
							Transform:  "(infer)",
							Error:      inferErr.Error(),
							Severity:   "warning",
						})
					}
					continue
				}
			}

			value, transformErr := m.transformReg.ApplyTransform(transformName, entry, mf.CDAField, mf.ValueMap)
			if transformErr != nil {
				severity := "warning"
				if mf.Required || mf.Conformance == "SHALL" {
					severity = "error"
				}
				errs = append(errs, SectionError{
					SectionKey: sectionKey,
					EntryIndex: entryIdx,
					FieldKey:   mf.CDAField,
					Transform:  transformName,
					Error:      transformErr.Error(),
					Severity:   severity,
				})
				continue
			}

			if value == nil {
				if mf.Required || mf.Conformance == "SHALL" {
					errs = append(errs, SectionError{
						SectionKey: sectionKey,
						EntryIndex: entryIdx,
						FieldKey:   mf.CDAField,
						Transform:  transformName,
						Error:      "empty value for SHALL-conformance field",
						Severity:   "warning",
					})
				}
				continue
			}

			// Optional terminology validation for coded fields (CD/CE/CS/UID).
			if config.TerminologyValidation && m.terminologySvc != nil {
				if isCoded(mf.CDADataType) {
					if codeStr, ok := value.(string); ok && codeStr != "" {
						systemURI := systemURIForField(entry, mf.CDAField, mf.ValueMap)
						if systemURI != "" {
							vr := m.terminologySvc.Validate(systemURI, codeStr)
							if !vr.Valid {
								severity := "warning"
								if mf.Conformance == "SHALL" {
									severity = "error"
								}
								errs = append(errs, SectionError{
									SectionKey: sectionKey,
									EntryIndex: entryIdx,
									FieldKey:   mf.CDAField,
									Transform:  transformName,
									Error:      fmt.Sprintf("terminology validation: %s", vr.Message),
									Severity:   severity,
								})
							}
						}
					}
				}
			}

			// Strip resource type prefix from FHIR path: "AllergyIntolerance.code" → "code"
			fhirPath := stripResourcePrefix(mf.FHIRPath, group.FHIRResource)
			setFHIRPath(resource, fhirPath, value)
		}

		// Only include non-empty resources (at least one field was set beyond id/resourceType)
		if len(resource) > 2 {
			resources = append(resources, resource)
		}
	}

	return resources, errs
}

// =========================================================
// FHIR path setter
// =========================================================

// setFHIRPath navigates/creates the nested map structure described by a dot-separated
// FHIR path (with optional array indices) and sets the leaf value.
// Example: "code.coding[0].code" sets resource["code"]["coding"][0]["code"] = value.
func setFHIRPath(obj map[string]interface{}, path string, value interface{}) {
	if path == "" || obj == nil || value == nil {
		return
	}
	dot := strings.Index(path, ".")
	if dot == -1 {
		// Leaf node — handle array index notation like "coding[0]"
		if bracket := strings.Index(path, "["); bracket != -1 {
			key := path[:bracket]
			idx := parseBracketIndex(path[bracket:])
			arr := ensureArray(obj, key, idx+1)
			if m, ok := arr[idx].(map[string]interface{}); ok {
				_ = m // leaf — can't set on bare array element without a key
			}
			// Direct assignment to the array element if it's a leaf
			arr[idx] = value
			obj[key] = arr
		} else {
			obj[path] = value
		}
		return
	}

	seg := path[:dot]
	rest := path[dot+1:]

	if bracket := strings.Index(seg, "["); bracket != -1 {
		key := seg[:bracket]
		idx := parseBracketIndex(seg[bracket:])
		arr := ensureArray(obj, key, idx+1)
		child, ok := arr[idx].(map[string]interface{})
		if !ok {
			child = map[string]interface{}{}
		}
		setFHIRPath(child, rest, value)
		arr[idx] = child
		obj[key] = arr
	} else {
		child, ok := obj[seg].(map[string]interface{})
		if !ok {
			child = map[string]interface{}{}
		}
		setFHIRPath(child, rest, value)
		obj[seg] = child
	}
}

func parseBracketIndex(s string) int {
	// s looks like "[0]" or "[0].something" — extract the integer
	start := strings.Index(s, "[")
	end := strings.Index(s, "]")
	if start == -1 || end == -1 || end <= start {
		return 0
	}
	n, err := strconv.Atoi(s[start+1 : end])
	if err != nil {
		return 0
	}
	return n
}

func ensureArray(obj map[string]interface{}, key string, minLen int) []interface{} {
	existing, ok := obj[key].([]interface{})
	if !ok {
		existing = []interface{}{}
	}
	for len(existing) < minLen {
		existing = append(existing, map[string]interface{}{})
	}
	return existing
}

func stripResourcePrefix(fhirPath, resourceType string) string {
	prefix := resourceType + "."
	if strings.HasPrefix(fhirPath, prefix) {
		return fhirPath[len(prefix):]
	}
	return fhirPath
}

// =========================================================
// Legacy header resource builders (map-based fallback path)
// Used by Map() when parsedCDA["_cdaDocument"] is absent.
// The typed path (MapDocument / Sprint D) supersedes these.
// =========================================================

// buildLegacyPatient builds a FHIR Patient from the headerParser-produced map.
func buildLegacyPatient(patientData map[string]interface{}) map[string]interface{} {
	p := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-1",
	}
	if len(patientData) == 0 {
		return p
	}

	family := strField(patientData, "lastName")
	given := strField(patientData, "firstName")
	middle := strField(patientData, "middleName")
	if family != "" || given != "" {
		nameEntry := map[string]interface{}{"use": "official"}
		if family != "" {
			nameEntry["family"] = family
		}
		var givenList []interface{}
		if given != "" {
			givenList = append(givenList, given)
		}
		if middle != "" {
			givenList = append(givenList, middle)
		}
		if len(givenList) > 0 {
			nameEntry["given"] = givenList
		}
		p["name"] = []interface{}{nameEntry}
	}
	if dob := strField(patientData, "dateOfBirth"); dob != "" {
		p["birthDate"] = FormatDate(dob)
	}
	if sex := strField(patientData, "sex"); sex != "" {
		p["gender"] = mapGenderCode(sex)
	}
	if phone := strField(patientData, "phone"); phone != "" {
		p["telecom"] = []interface{}{
			map[string]interface{}{"system": "phone", "value": phone, "use": "home"},
		}
	}
	if addrRaw, ok := patientData["address"].(map[string]interface{}); ok {
		if addr := buildAddressFromMap(addrRaw); len(addr) > 0 {
			p["address"] = []interface{}{addr}
		}
	}
	if idsRaw, ok := patientData["ids"].([]interface{}); ok {
		var identifiers []interface{}
		for _, raw := range idsRaw {
			idMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			root := strField(idMap, "root")
			ext := strField(idMap, "extension")
			if ident := buildPatientIdentifier(root, ext); ident != nil {
				identifiers = append(identifiers, ident)
			}
		}
		if len(identifiers) > 0 {
			p["identifier"] = identifiers
		}
	}
	return p
}

// buildLegacyAuthor builds a FHIR Practitioner from the header author map.
func buildLegacyAuthor(header map[string]interface{}) map[string]interface{} {
	if header == nil {
		return nil
	}
	author, _ := header["author"].(map[string]interface{})
	given := strField(author, "given")
	family := strField(author, "family")
	if given == "" && family == "" {
		return nil
	}
	p := map[string]interface{}{
		"resourceType": "Practitioner",
		"id":           "author-1",
	}
	nameEntry := map[string]interface{}{"use": "official"}
	if family != "" {
		nameEntry["family"] = family
	}
	if given != "" {
		nameEntry["given"] = []interface{}{given}
	}
	p["name"] = []interface{}{nameEntry}
	if npi := strField(author, "npi"); npi != "" {
		p["identifier"] = []interface{}{
			map[string]interface{}{"system": "http://hl7.org/fhir/sid/us-npi", "value": npi},
		}
	}
	return p
}

// buildLegacyCustodian builds a FHIR Organization from the header custodian map.
func buildLegacyCustodian(header map[string]interface{}) map[string]interface{} {
	if header == nil {
		return nil
	}
	custodian, _ := header["custodian"].(map[string]interface{})
	name := strField(custodian, "name")
	if name == "" {
		return nil
	}
	org := map[string]interface{}{
		"resourceType": "Organization",
		"id":           "custodian-1",
		"name":         name,
	}
	if addrRaw, ok := custodian["address"].(map[string]interface{}); ok {
		if addr := buildAddressFromMap(addrRaw); len(addr) > 0 {
			org["address"] = []interface{}{addr}
		}
	}
	return org
}

// =========================================================
// Context link injection
// =========================================================

// injectPatientReference sets the patient/subject reference on any resource
// whose section declares a patient context link.
func (m *GenericCDAFHIRMapper) injectPatientReference(
	resource map[string]interface{},
	patientRef string,
	groups map[string]sectionGroup,
) {
	rt := strField(resource, "resourceType")
	if rt == "Patient" {
		return // Patient doesn't reference itself
	}

	// Find the section group for this resource type
	for _, sg := range groups {
		if sg.FHIRResource != rt {
			continue
		}
		for linkKey := range sg.ContextLinks {
			ref := map[string]interface{}{"reference": patientRef}
			switch linkKey {
			case "patient":
				if _, exists := resource["patient"]; !exists {
					resource["patient"] = ref
				}
			case "subject":
				if _, exists := resource["subject"]; !exists {
					resource["subject"] = ref
				}
			}
		}
		return
	}
}

// =========================================================
// Utility helpers
// =========================================================

func extractHeaderPatient(header map[string]interface{}) map[string]interface{} {
	if header == nil {
		return nil
	}
	p, _ := header["patient"].(map[string]interface{})
	return p
}

func extractSectionEntries(sectionKey string, sectionsMap map[string]interface{}) []map[string]interface{} {
	if sectionsMap == nil {
		return nil
	}
	raw, ok := sectionsMap[sectionKey]
	if !ok {
		return nil
	}
	section, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return sectionEntriesToSlice(section["entries"])
}

func sectionEntriesToSlice(raw interface{}) []map[string]interface{} {
	switch v := raw.(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

func isHeaderSection(key string) bool {
	return strings.HasPrefix(key, "header.")
}

func (m *GenericCDAFHIRMapper) isSectionEnabled(key string, enabled []string) bool {
	if len(enabled) == 0 {
		return true
	}
	for _, k := range enabled {
		if k == key {
			return true
		}
	}
	return false
}

func sectionKeyForResource(resourceType string, groups map[string]sectionGroup) string {
	for k, sg := range groups {
		if sg.FHIRResource == resourceType {
			return k
		}
	}
	return ""
}

func resourceIDPrefix(resourceType string) string {
	return strings.ToLower(resourceType[:1]) + resourceType[1:]
}

func mapGenderCode(code string) string {
	switch strings.ToUpper(code) {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "UN", "UNK":
		return "unknown"
	case "O", "OTHER":
		return "other"
	default:
		return "unknown"
	}
}

// isCoded returns true when the CDA data type carries a coded value that
// can be validated against a terminology system.
func isCoded(cdaDataType string) bool {
	switch cdaDataType {
	case "CD", "CE", "CS", "CV", "UID":
		return true
	}
	return false
}

// systemURIForField extracts the FHIR system URI for a coded field.
// It checks for a companion "fieldKeySystem" entry in the entry map first
// (populated by GenericSectionProcessor from xpathSystem), then falls back
// to the first key in the valueMap (OID → URI pattern from the mapping).
func systemURIForField(entry map[string]interface{}, fieldKey string, vm map[string]string) string {
	// GenericSectionProcessor stores the system OID as fieldKey+"System"
	if sysVal, ok := entry[fieldKey+"System"].(string); ok && sysVal != "" {
		// The value might be an OID — try the common OID→URI mappings
		if uri := oidToSystemURI(sysVal); uri != "" {
			return uri
		}
		// Already a URI
		if strings.HasPrefix(sysVal, "http") {
			return sysVal
		}
	}
	// Fall back: if valueMap keys look like URIs, infer the system
	for k := range vm {
		if strings.HasPrefix(k, "http") {
			return k
		}
	}
	return ""
}

// oidToSystemURI maps common healthcare OIDs to FHIR system URIs.
func oidToSystemURI(oid string) string {
	switch oid {
	case "2.16.840.1.113883.6.96":
		return "http://snomed.info/sct"
	case "2.16.840.1.113883.6.88":
		return "http://www.nlm.nih.gov/research/umls/rxnorm"
	case "2.16.840.1.113883.6.1":
		return "http://loinc.org"
	case "2.16.840.1.113883.12.292":
		return "http://hl7.org/fhir/sid/cvx"
	case "2.16.840.1.113883.6.69":
		return "http://hl7.org/fhir/sid/ndc"
	case "2.16.840.1.113883.6.90":
		return "http://hl7.org/fhir/sid/icd-10-cm"
	case "2.16.840.1.113883.6.12":
		return "http://www.ama-assn.org/go/cpt"
	case "2.16.840.1.113883.3.26.1.1":
		return "http://ncithesaurus.nci.nih.gov"
	}
	return ""
}

// oidToFHIRSystem maps well-known CDA OID roots to canonical FHIR system URIs.
var oidToFHIRSystem = map[string]string{
	"2.16.840.1.113883.4.1":   "http://hl7.org/fhir/sid/us-ssn",
	"2.16.840.1.113883.4.6":   "http://hl7.org/fhir/sid/us-npi",
	"2.16.840.1.113883.4.572": "http://hl7.org/fhir/sid/us-medicare",
	"2.16.840.1.113883.4.927": "http://hl7.org/fhir/sid/us-mbi",
	"2.16.840.1.113883.4.3":   "http://hl7.org/fhir/sid/us-dl",
}

// oidToTypeCode maps well-known OID roots to HL7 v2 table 0203 identifier type codes.
var oidToTypeCode = map[string]string{
	"2.16.840.1.113883.4.1":   "SS",
	"2.16.840.1.113883.4.6":   "NPI",
	"2.16.840.1.113883.4.572": "MC",
	"2.16.840.1.113883.4.927": "MA",
	"2.16.840.1.113883.4.3":   "DL",
}

// buildPatientIdentifier converts a CDA <id root="..." extension="..."> pair into a
// FHIR Identifier with system, value, and type.  Returns nil for empty inputs.
func buildPatientIdentifier(root, extension string) map[string]interface{} {
	value := extension
	if value == "" {
		value = root // some systems put the ID in root with no extension
	}
	if value == "" {
		return nil
	}

	ident := map[string]interface{}{"value": value}

	// System URI
	if sys, ok := oidToFHIRSystem[root]; ok {
		ident["system"] = sys
	} else if root != "" {
		ident["system"] = "urn:oid:" + root
	}

	// Identifier type code (table 0203)
	typeCode := "PI" // Patient Internal Identifier — default for unknown OIDs
	if tc, ok := oidToTypeCode[root]; ok {
		typeCode = tc
	} else if strings.HasPrefix(root, "2.16.840.1.113883.3.") {
		typeCode = "MR" // facility-specific OID subtree → treat as MRN
	}
	ident["type"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
				"code":   typeCode,
			},
		},
	}

	return ident
}

func buildAddressFromMap(addr map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{"use": "home"}
	if street := strField(addr, "street"); street != "" {
		result["line"] = []interface{}{street}
	}
	if city := strField(addr, "city"); city != "" {
		result["city"] = city
	}
	if state := strField(addr, "state"); state != "" {
		result["state"] = state
	}
	if postal := strField(addr, "postalCode"); postal != "" {
		result["postalCode"] = postal
	}
	if country := strField(addr, "country"); country != "" {
		result["country"] = country
	}
	if len(result) == 1 { // only "use"
		return nil
	}
	return result
}

// =========================================================
// ComputeCDADelta (Sprint E3)
// Mirror of HL7FHIRTransformServiceV3.ComputeDelta()
// =========================================================

// CDAAtomicMapping is the flat wire format sent by the wizard when saving
// section-level field overrides. Each row represents one (sectionKey, cdaField)
// pair with the user-configured fhirPath and transform.
type CDAAtomicMapping struct {
	SectionKey string `json:"sectionKey"`
	CDAField   string `json:"cdaField"`
	FHIRPath   string `json:"fhirPath"`
	Transform  string `json:"transform"`
}

// ComputeCDADelta compares the incoming wizard mappings against the current OOB
// template and returns a sparse CDAMappingDelta containing only overrides.
// Returns nil delta when the incoming exactly matches the OOB (pure OOB path).
//
// Parameters:
//   ctx         — request context
//   docType     — "CCD", "Discharge Summary", etc.
//   ccdaVersion — "2.1"
//   fhirVersion — "R4"
//   incoming    — []CDAAtomicMapping from the wizard
//
// Returns:
//   delta      — nil if no overrides needed (pure OOB)
//   templateID — UUID of the OOB template the delta is anchored to
//   version    — version string extracted from template_config.version
//   err
func (m *GenericCDAFHIRMapper) ComputeCDADelta(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion string,
	incoming []CDAAtomicMapping,
) (*CDAMappingDelta, string, string, error) {

	templateID, version, oob, err := m.loadOOBTemplateWithMeta(ctx, docType, ccdaVersion, fhirVersion)
	if err != nil {
		return nil, "", "", err
	}

	// Build an index of the OOB by (sectionKey, cdaField).
	type fieldKey struct{ section, field string }
	oobIndex := make(map[fieldKey]CDAFieldMapping, len(oob))
	for _, fm := range oob {
		oobIndex[fieldKey{fm.SectionKey, fm.CDAField}] = fm
	}

	// Build an index of incoming by the same key.
	incomingIndex := make(map[fieldKey]CDAAtomicMapping, len(incoming))
	for _, am := range incoming {
		incomingIndex[fieldKey{am.SectionKey, am.CDAField}] = am
	}

	var overrides []CDAMappingOverride

	// Walk incoming: detect replacements and additions.
	for k, am := range incomingIndex {
		oobFM, exists := oobIndex[k]
		if !exists {
			// New field added by the user — not in OOB.
			overrides = append(overrides, CDAMappingOverride{
				Action:     "add",
				SectionKey: am.SectionKey,
				CDAField:   am.CDAField,
				FHIRPath:   am.FHIRPath,
				Transform:  am.Transform,
			})
			continue
		}
		// Field exists in OOB — check if user changed it.
		if am.FHIRPath != oobFM.FHIRPath || am.Transform != oobFM.Transform {
			overrides = append(overrides, CDAMappingOverride{
				Action:     "replace",
				SectionKey: am.SectionKey,
				CDAField:   am.CDAField,
				FHIRPath:   am.FHIRPath,
				Transform:  am.Transform,
			})
		}
	}

	// Walk OOB: fields present in OOB but absent from incoming are explicit removals.
	for k, oobFM := range oobIndex {
		if _, present := incomingIndex[k]; !present {
			overrides = append(overrides, CDAMappingOverride{
				Action:     "remove",
				SectionKey: oobFM.SectionKey,
				CDAField:   oobFM.CDAField,
			})
		}
	}

	// No overrides → pure OOB, no need to store a delta.
	if len(overrides) == 0 {
		return nil, templateID, version, nil
	}

	delta := &CDAMappingDelta{
		Version:        1,
		BaseTemplateID: templateID,
		BasedOnVersion: version,
		Overrides:      overrides,
	}
	return delta, templateID, version, nil
}

// loadOOBTemplateWithMeta is like loadFromCDAOOBTemplates but also returns
// the template UUID and version string from template_config.
func (m *GenericCDAFHIRMapper) loadOOBTemplateWithMeta(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion string,
) (templateID, version string, mappings []CDAFieldMapping, err error) {

	if m.db == nil {
		return "", "", nil, fmt.Errorf("cda_fhir: no DB connection for OOB template lookup")
	}

	var configJSON string
	err = m.db.QueryRowContext(ctx, `
		SELECT id::text, template_config::text
		FROM cda_fhir_templates
		WHERE document_type = $1
		  AND ccda_version   = $2
		  AND fhir_version   = $3
		  AND is_default     = true
		ORDER BY us_core_version DESC
		LIMIT 1
	`, docType, ccdaVersion, fhirVersion).Scan(&templateID, &configJSON)

	if err == sql.ErrNoRows {
		// Try fallback without version constraints
		err = m.db.QueryRowContext(ctx, `
			SELECT id::text, template_config::text
			FROM cda_fhir_templates
			WHERE document_type = $1 AND is_default = true
			ORDER BY ccda_version DESC, us_core_version DESC
			LIMIT 1
		`, docType).Scan(&templateID, &configJSON)
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("cda_fhir: OOB template lookup: %w", err)
	}

	// Extract version from template_config.version.
	var cfgMeta struct {
		Version string `json:"version"`
	}
	version = "1.0"
	if jsonErr := json.Unmarshal([]byte(configJSON), &cfgMeta); jsonErr == nil && cfgMeta.Version != "" {
		version = cfgMeta.Version
	}

	mappings, err = m.parseMappingConfig([]byte(configJSON))
	if err != nil {
		return "", "", nil, err
	}

	return templateID, version, mappings, nil
}
