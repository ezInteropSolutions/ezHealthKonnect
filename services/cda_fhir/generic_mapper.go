// services/cda_fhir/generic_mapper.go
// GenericCDAFHIRMapper — CDA→FHIR mapper struct, template/delta management.
//
// The actual document-to-Bundle conversion lives in
// declarative_document_mapper.go (DeclarativeMapDocument, the OOB
// declarative-rules engine — see Phase 4 of
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md). This file now holds:
//   - The GenericCDAFHIRMapper struct + constructor (dependency injection
//     for both DeclarativeMapDocument and the config-management API below).
//   - CDA mapping *configuration* management (getCDAFieldMappings,
//     loadFromCDAOOBTemplates, mergeCDAMappings, ComputeCDADelta,
//     GetCDAFieldMappingsPublic) — used by controllers/cda_schema_controller.go's
//     schema-browser/wizard endpoints to manage interface_cda_mappings/
//     cda_fhir_templates rows. This is config *management*, never execution —
//     confirmed via grep that controller never calls a Map/MapDocument-style
//     conversion method.
//   - Shared utilities used by DeclarativeMapDocument: writeTransformAuditLog,
//     isSectionEnabled, resourceIDPrefix.
//
// Phase 4 Slice D decommissioned the hardcoded Go path this file used to also
// contain: Map() (the V9-style DB-driven field-mapping executor),
// createFHIRResourceFromSection(), the legacy map-based header builders
// (buildLegacyPatient/Author/Custodian), and their now-orphaned helpers
// (mapGenderCode, isCoded, systemURIForField, oidToSystemURI,
// buildPatientIdentifier, buildAddressFromMap, oidToFHIRSystem,
// oidToTypeCode) — confirmed via grep to have had exactly 2 live callers
// (the executor's now-removed legacy fallback, and a test file), neither of
// which depended on the config-management code kept here.

package cdafhir

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
	cdaterminology "ezhealthkonnect/services/cda_terminology"
	"ezhealthkonnect/services/executors"
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
	CDAField      string            // e.g. "code", "reaction.manifestation[0]" — the FlattenSectionRules
	                                  // TargetPath-based key (declarative_rules_flatten.go), NOT a ccda_2_1.json schema key.
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

	// The following four fields are only meaningful for Action=="add" — a
	// genuinely new field with no existing MappingRow to inherit Scope/
	// SourcePath from (unlike "replace"/"remove", which only ever touch an
	// EXISTING row's own properties). See ApplyFieldOverrides/applyAddOverride
	// in declarative_rules_flatten.go for how these are consumed.

	// Scope is a Phase 1 path (MappingRow.Scope's own grammar), relative to
	// the matched entry (top-level add) or to the NestedUnder parent's own
	// matched node (nested add). Empty means "the entry/parent node itself".
	Scope string `json:"scope,omitempty"`
	// SourcePath is a Phase 1 path relative to Scope (MappingRow.SourcePath).
	SourcePath string `json:"sourcePath,omitempty"`
	// NestedUnder is the TargetPath of an EXISTING CollectAll+Fields parent
	// row (e.g. "reaction") this new field nests under. Empty = top-level.
	NestedUnder string `json:"nestedUnder,omitempty"`
	// TargetFHIRResources narrows which MappingRule variant(s) sharing this
	// SectionKey (matched by their own FHIRResource) the new row is inserted
	// into. Empty means every rule variant in the section — the same
	// indiscriminate fan-out "replace"/"remove" already have; only "add"
	// needs the ability to narrow this, since a resource-specific new field
	// (e.g. one that only makes sense on MedicationRequest, not
	// MedicationStatement) has no existing row to imply that scoping.
	TargetFHIRResources []string `json:"targetFhirResources,omitempty"`
	// CollectAll mirrors MappingRow.CollectAll — when true, the new row
	// captures EVERY node Scope matches as one FHIR array element each,
	// instead of only the first. Only meaningful (and only ever produced by
	// the UI) for a top-level add: applyAddOverride appends straight to
	// rule.Fields with no Fields of its own, the same "plain CollectAll"
	// shape as e.g. Medication's "entryRelationships[typeCode=RSON].entry" ->
	// reasonCode row.
	CollectAll bool `json:"collectAll,omitempty"`
}

// CDAMappingDelta is the full delta stored in interface_cda_mappings.mapping_overrides.
// Versioned so future tools can detect when a delta was built against an older OOB.
type CDAMappingDelta struct {
	Version        int                  `json:"version"`
	BaseTemplateID string               `json:"base_template_id"` // UUID of OOB template
	BasedOnVersion string               `json:"based_on_version"` // OOB template_config.version at save time
	Overrides      []CDAMappingOverride `json:"overrides"`
}

// CDAToFHIRConfig parameterises a DeclarativeMapDocument() call; mirrors the
// step builder config tabs.
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

	// PlanOfCareEncounterTarget overrides which FHIR resource Plan-of-Care
	// section "entryType=encounter" entries (CDA classCode=ENC, typically
	// moodCode=APT -- a planned/future visit) map to: "Appointment" (the
	// Go-literal PlanOfCareMappingRules() default) or "Encounter" (reuses
	// encounterFields() -- richer participant/location/identifier support,
	// see declarative_oob_rules.go). Empty string means "not set by this
	// pipeline step" -- DeclarativeMapDocument then falls back to the
	// interface's own processing_rules.cda.planOfCareEncounterTarget
	// default, and finally to "Encounter" if neither is set. Set by
	// cda_to_fhir_executor.go from the step's own config JSON.
	PlanOfCareEncounterTarget string

	// Assembly controls the post-mapping assembly layer (deduplication, panel synthesis).
	// Zero value = assembly enabled with all default rules active.
	Assembly assembly.AssemblyConfig

	// CoverageTracker is set by cda_to_fhir_executor.go from the message
	// envelope's "_coverageTracker" key (see cda_coverage_tracker.go) only
	// when the owning interface has opted into CDA Coverage Audit. nil
	// (the common case) makes tracking a no-op — see DeclarativeEngine's
	// own CoverageTracker field doc comment for how this is consumed.
	CoverageTracker *executors.CDACoverageTracker
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

// MapOutput is the combined return value of DeclarativeMapDocument().
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
	db             *sql.DB
	schemaLoader   *cdaSchema.CDASchemaLoader
	transformReg   *CDATransformRegistry
	profileBuilder *USCoreProfileBuilder
	narrativeGen   *fhirnarrative.FHIRNarrativeGenerator
	terminologySvc *cdaterminology.TerminologyService // optional; nil = skip validation

	// templateCache keys: "<docType>|<ccdaVersion>|<fhirVersion>"
	templateCache map[string][]CDAFieldMapping
	templateMu    sync.RWMutex
}

// NewGenericCDAFHIRMapper constructs a mapper with all dependencies injected.
func NewGenericCDAFHIRMapper(db *sql.DB, loader *cdaSchema.CDASchemaLoader) *GenericCDAFHIRMapper {
	return &GenericCDAFHIRMapper{
		db:             db,
		schemaLoader:   loader,
		transformReg:   NewCDATransformRegistry(),
		profileBuilder: NewUSCoreProfileBuilder(),
		narrativeGen:   fhirnarrative.NewFHIRNarrativeGenerator(),
		templateCache:  make(map[string][]CDAFieldMapping),
	}
}

// WithTerminologyService wires an optional TerminologyService for code validation
// and translation. When set and CDAToFHIRConfig.TerminologyValidation=true,
// coded fields are validated during conversion.
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

// loadCDAMappingOverrides returns the raw []CDAMappingOverride stored for one
// interface+docType — the runtime counterpart to getCDAFieldMappings, used by
// DeclarativeMapDocument (declarative_document_mapper.go) to patch a cloned
// []MappingRule before execution. Returns nil (meaning "no patch — use OOB
// as-is") on any of: no DB, no interfaceID, no row, a full-custom row
// (uses_standard_template=false — that tier is handled entirely by
// getCDAFieldMappings/the config-browsing API, never by runtime rule
// patching), a NULL/empty overrides column, or a JSON parse error (logged,
// not fatal, mirroring writeTransformAuditLog's own non-fatal logging style).
func (m *GenericCDAFHIRMapper) loadCDAMappingOverrides(ctx context.Context, interfaceID, docType string) []CDAMappingOverride {
	delta := m.loadCDAMappingDelta(ctx, interfaceID, docType)
	if delta == nil {
		return nil
	}
	return delta.Overrides
}

// loadCDAMappingDelta is the same lookup as loadCDAMappingOverrides but
// returns the FULL CDAMappingDelta envelope (version/base_template_id/
// based_on_version, not just the bare Overrides slice) -- needed by
// LoadCDAMappingDeltaPublic so an exported delta can be POSTed straight back
// to SaveMappingDelta (which expects the whole envelope) without losing
// those fields. Same nil-on-any-of-these-conditions contract as
// loadCDAMappingOverrides (see that function's doc comment); the two share
// this one query so they can never observe a different row.
func (m *GenericCDAFHIRMapper) loadCDAMappingDelta(ctx context.Context, interfaceID, docType string) *CDAMappingDelta {
	if m.db == nil || interfaceID == "" {
		return nil
	}

	var (
		usesStandard  bool
		overridesJSON sql.NullString
	)
	err := m.db.QueryRowContext(ctx, `
		SELECT uses_standard_template, mapping_overrides
		FROM interface_cda_mappings
		WHERE interface_id = $1 AND document_type = $2
	`, interfaceID, docType).Scan(&usesStandard, &overridesJSON)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[cda.to_fhir] loadCDAMappingDelta query error (interface=%s, docType=%s): %v", interfaceID, docType, err)
		}
		return nil
	}
	if !usesStandard || !overridesJSON.Valid || overridesJSON.String == "" || overridesJSON.String == "null" {
		return nil
	}

	var delta CDAMappingDelta
	if err := json.Unmarshal([]byte(overridesJSON.String), &delta); err != nil {
		log.Printf("[cda.to_fhir] loadCDAMappingDelta parse error (interface=%s, docType=%s): %v", interfaceID, docType, err)
		return nil
	}
	return &delta
}

// LoadCDAMappingDeltaPublic exposes loadCDAMappingDelta to callers outside
// this package (controllers/cda_schema_controller.go's new GET raw-delta
// endpoint, which lets the pipeline builder's JSON Export carry an
// interface's actual field-mapping overrides along with the step config --
// see PropertiesPanel.js's EXTERNAL_MAPPING_STORES). Returns nil under the
// exact same conditions as loadCDAMappingDelta (no DB, no interfaceID, no
// row, full-custom row, empty/NULL overrides, or a parse error).
func (m *GenericCDAFHIRMapper) LoadCDAMappingDeltaPublic(ctx context.Context, interfaceID, docType string) *CDAMappingDelta {
	return m.loadCDAMappingDelta(ctx, interfaceID, docType)
}

// loadPlanOfCareEncounterTargetDefault returns the interface-level default
// for Plan-of-Care "entryType=encounter" entries' target FHIR resource
// ("Appointment" | "Encounter"), read from interfaces.processing_rules ->
// 'cda' ->> 'planOfCareEncounterTarget'. Returns "" (no interface-level
// default set) on any of: no DB, no interfaceID, no row, NULL
// processing_rules, or the key simply absent -- DeclarativeMapDocument
// falls back to a hardcoded "Encounter" default in that case, mirroring
// loadCDAMappingOverrides' own "nil means use OOB as-is" convention. Reuses
// m.db the same way loadCDAMappingOverrides does -- no new DB infra.
func (m *GenericCDAFHIRMapper) loadPlanOfCareEncounterTargetDefault(ctx context.Context, interfaceID string) string {
	if m.db == nil || interfaceID == "" {
		return ""
	}
	var target sql.NullString
	err := m.db.QueryRowContext(ctx, `
		SELECT processing_rules -> 'cda' ->> 'planOfCareEncounterTarget'
		FROM interfaces
		WHERE id = $1
	`, interfaceID).Scan(&target)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[cda.to_fhir] loadPlanOfCareEncounterTargetDefault query error (interface=%s): %v", interfaceID, err)
		}
		return ""
	}
	return target.String
}

// CountDeclarativeFields returns how many addressable fields
// FlattenSectionRules produces for sectionKey — used by GetSections'
// fieldCount, replacing the static ccda_2_1.json schema's stale count.
func (m *GenericCDAFHIRMapper) CountDeclarativeFields(sectionKey string) int {
	return len(FlattenSectionRules(declarativeSectionRuleGroupsCache[sectionKey]))
}

// =========================================================
// OOB template loading (mirror of HL7 loadFromV9OOBTemplates)
// =========================================================

// loadFromDeclarativeRules derives the effective OOB []CDAFieldMapping
// directly from declarativeSectionRuleGroupsCache — the same rule set
// DeclarativeMapDocument actually executes — instead of the orphaned
// cda_fhir_templates.template_config content (see this file's header
// comment / Phase 4 Slice D note). docType is accepted but currently unused
// for filtering: every *MappingRules() section applies regardless of CDA
// document type, matching DeclarativeMapDocument's own behavior of
// dispatching purely by sectionKey presence in the parsed document. Kept as
// a parameter for signature parity with the function this replaces, so a
// future doc-type-scoped rule set is a non-breaking addition later.
func loadFromDeclarativeRules(docType string) []CDAFieldMapping {
	var result []CDAFieldMapping
	for sectionKey, rules := range declarativeSectionRuleGroupsCache {
		for _, ff := range FlattenSectionRules(rules) {
			result = append(result, CDAFieldMapping{
				SectionKey:   sectionKey,
				FHIRResource: ff.FHIRResource,
				CDAField:     ff.Key,
				FHIRPath:     ff.TargetPath,
				Transform:    ff.Transform,
				ValueMap:     ff.ValueMap,
				Conformance:  ff.Conformance,
				Required:     ff.Required,
			})
		}
	}
	return result
}

func (m *GenericCDAFHIRMapper) loadFromCDAOOBTemplates(
	ctx context.Context,
	docType, ccdaVersion, fhirVersion string,
) ([]CDAFieldMapping, error) {
	return loadFromDeclarativeRules(docType), nil
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

// writeTransformAuditLog inserts one audit_logs row per CDA→FHIR conversion call.
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

// LoadCDAMappingOverridesPublic exposes loadCDAMappingOverrides (the RAW,
// unmerged []CDAMappingOverride — the same list ApplyFieldOverrides patches
// rules with at document-processing time) to callers outside this package.
// GetSectionFields needs this specifically because mergeCDAMappings'
// "remove" handling drops a removed field from its merged []CDAFieldMapping
// output entirely — indistinguishable there from "never had an override" —
// so the Section Field Editor must check the raw Action itself to render a
// removed field as removed instead of silently showing its OOB default.
func (m *GenericCDAFHIRMapper) LoadCDAMappingOverridesPublic(ctx context.Context, interfaceID, docType string) []CDAMappingOverride {
	return m.loadCDAMappingOverrides(ctx, interfaceID, docType)
}

// InvalidateCache clears the in-process template cache.
func (m *GenericCDAFHIRMapper) InvalidateCache() {
	m.templateMu.Lock()
	m.templateCache = make(map[string][]CDAFieldMapping)
	m.templateMu.Unlock()
}

// =========================================================
// Utility helpers
// =========================================================

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

func resourceIDPrefix(resourceType string) string {
	return strings.ToLower(resourceType[:1]) + resourceType[1:]
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
	// Disabled marks this field as explicitly removed by the user (the
	// Section Field Editor's "Remove" action) — an EXPLICIT per-field
	// signal, deliberately never inferred from a field's absence from
	// incoming: incoming is a sparse list of only the fields the user
	// touched in the currently-open section (see _buildAtomicMappings in
	// CDAStepBuilder.js), and treating absence as removal previously
	// produced 276 spurious removals from a single-field edit (see the
	// comment below on the deliberately-not-walking-absence loop).
	Disabled bool `json:"disabled,omitempty"`

	// The following mirror CDAMappingOverride's own "add"-only fields
	// (same names/shapes) so ComputeCDADelta's "field not found in OOB"
	// branch can copy them straight through when building the resulting
	// Action=="add" override. Empty/unused for every other field.
	Scope               string   `json:"scope,omitempty"`
	SourcePath          string   `json:"sourcePath,omitempty"`
	NestedUnder         string   `json:"nestedUnder,omitempty"`
	TargetFHIRResources []string `json:"targetFhirResources,omitempty"`
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

	// Walk incoming: detect removals, replacements, and additions.
	for k, am := range incomingIndex {
		// Disabled is an explicit per-field signal from the UI's "Remove"
		// button — checked before the replace/add branches below, and
		// unconditionally, regardless of whether the field exists in OOB:
		// its FHIRPath/Transform are irrelevant once removed (applyRow skips
		// the row before ever reading them), so nothing else about this
		// field needs to be diffed.
		if am.Disabled {
			overrides = append(overrides, CDAMappingOverride{
				Action:     "remove",
				SectionKey: am.SectionKey,
				CDAField:   am.CDAField,
			})
			continue
		}

		oobFM, exists := oobIndex[k]
		if !exists {
			// New field added by the user — not in OOB. Scope/SourcePath/
			// NestedUnder/TargetFHIRResources carry the new field's actual
			// definition through to ApplyFieldOverrides/applyAddOverride at
			// runtime — without these, "add" would have a FHIR-side target
			// but no way to ever produce a value for it.
			overrides = append(overrides, CDAMappingOverride{
				Action:              "add",
				SectionKey:          am.SectionKey,
				CDAField:            am.CDAField,
				FHIRPath:            am.FHIRPath,
				Transform:           am.Transform,
				Scope:               am.Scope,
				SourcePath:          am.SourcePath,
				NestedUnder:         am.NestedUnder,
				TargetFHIRResources: am.TargetFHIRResources,
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

	// Deliberately NOT walking "OOB fields absent from incoming ⇒ remove":
	// incoming is, by the Section Field Editor's actual design
	// (CDAStepBuilder.js's _buildAtomicMappings), a SPARSE list containing
	// only fields the user explicitly touched in the currently-open section
	// — never a full inventory of every field across every section. Treating
	// absence as removal here previously meant editing ONE field produced a
	// delta with a "remove" entry for every OTHER field in every OTHER
	// section (confirmed live: 276 spurious removals from a single-field
	// edit) — the UI has no "delete this field's mapping" affordance at all,
	// so there is no user intent to interpret absence as removal against.

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

	mappings = loadFromDeclarativeRules(docType)

	return templateID, version, mappings, nil
}
