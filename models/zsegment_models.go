// models/zsegment_models.go
//
// Data models for the enterprise Z-segment mapping configuration system.
//
// Three-tier design:
//
//	Tier 1 — hardcoded standard HL7 Table 0175 (STF→Practitioner, LOC→Location, CDM→ChargeItemDefinition)
//	Tier 2 — MFNInterfaceConfig: per-interface MFI.1 → FHIR resource overrides
//	Tier 3 — ZSegmentConfig + ZSegmentFieldMapping: per-interface Z-seg field mappings
//
// ZSegmentTemplate / ZSegmentTemplateField: reusable definitions that can be
// applied to any interface.  Applying a template *copies* fields into the
// interface's zsegment_field_mappings — the interface config is self-contained
// and survives template edits or deletion.
package models

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// Transform catalog
// ─────────────────────────────────────────────────────────────────────────────

// ZSegmentTransform describes a named field-level transform available in the
// system.  The full list lives in the zsegment_transforms DB table (seeded by
// V96 migration) and is served to the UI via the catalog endpoint.
type ZSegmentTransform struct {
	ID          int      `json:"id"          db:"id"`
	Code        string   `json:"code"        db:"code"`
	Label       string   `json:"label"       db:"label"`
	Description string   `json:"description" db:"description"`
	InputTypes  []string `json:"inputTypes"  db:"input_types"`
	SortOrder   int      `json:"sortOrder"   db:"sort_order"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Templates
// ─────────────────────────────────────────────────────────────────────────────

// ZSegmentTemplate is a reusable, sharable Z-segment mapping definition.
// OOB (is_system=true) templates are shipped with the product and cannot be
// edited or deleted.  User-created templates are per-tenant or public.
type ZSegmentTemplate struct {
	ID                 string                 `json:"id"                 db:"id"`
	TemplateName       string                 `json:"templateName"       db:"template_name"`
	SegmentID          string                 `json:"segmentId"          db:"segment_id"`
	TargetFHIRResource string                 `json:"targetFhirResource" db:"target_fhir_resource"`
	ResourceRole       string                 `json:"resourceRole"       db:"resource_role"`
	Description        string                 `json:"description"        db:"description"`
	Tags               []string               `json:"tags"               db:"tags"`
	IsSystem           bool                   `json:"isSystem"           db:"is_system"`
	IsPublic           bool                   `json:"isPublic"           db:"is_public"`
	CreatedByUserID    *string                `json:"createdByUserId"    db:"created_by_user_id"`
	UsageCount         int                    `json:"usageCount"         db:"usage_count"`
	CreatedAt          time.Time              `json:"createdAt"          db:"created_at"`
	UpdatedAt          time.Time              `json:"updatedAt"          db:"updated_at"`
	Fields             []ZSegmentTemplateField `json:"fields"`
}

// ZSegmentTemplateField is one field mapping entry inside a template.
type ZSegmentTemplateField struct {
	ID           string `json:"id"           db:"id"`
	TemplateID   string `json:"templateId"   db:"template_id"`
	Position     string `json:"position"     db:"position"`     // ZEM.1, ZEM.3.2, etc.
	FieldLabel   string `json:"fieldLabel"   db:"field_label"`
	HL7DataType  string `json:"hl7DataType"  db:"hl7_data_type"` // ST, XAD, XPN, XTN, CWE, …
	FHIRPath     string `json:"fhirPath"     db:"fhir_path"`
	Transform    string `json:"transform"    db:"transform"`
	IsRequired   bool   `json:"isRequired"   db:"is_required"`
	DefaultValue string `json:"defaultValue" db:"default_value"`
	SortOrder    int    `json:"sortOrder"    db:"sort_order"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier 2 — Per-interface MFI.1 overrides
// ─────────────────────────────────────────────────────────────────────────────

// MFNInterfaceConfig overrides the default MFI.1 → FHIR resource mapping for
// a specific interface.  Only non-standard MFI.1 values need an entry here;
// the hardcoded Tier 1 mappings (STF, LOC, CDM) always take precedence.
type MFNInterfaceConfig struct {
	ID            string    `json:"id"            db:"id"`
	InterfaceID   string    `json:"interfaceId"   db:"interface_id"`
	MFIIdentifier string    `json:"mfiIdentifier" db:"mfi_identifier"` // EMP, INS, PAY, M13, …
	FHIRResource  string    `json:"fhirResource"  db:"fhir_resource"`  // Organization, Person, …
	Description   string    `json:"description"   db:"description"`
	IsActive      bool      `json:"isActive"      db:"is_active"`
	CreatedAt     time.Time `json:"createdAt"     db:"created_at"`
	UpdatedAt     time.Time `json:"updatedAt"     db:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier 3 — Per-interface Z-segment configs
// ─────────────────────────────────────────────────────────────────────────────

// ZSegmentConfig declares that a given Z-segment ID appears in this interface's
// messages and specifies how it maps to a FHIR resource.
//
// resource_role values:
//
//	"primary"   — this Z-seg becomes the main assembled resource (replaces default)
//	"related"   — added as a second resource in the bundle; linked to primary via reference
//	"extension" — merges additional fields into an existing resource of the same type
type ZSegmentConfig struct {
	ID                 string               `json:"id"                 db:"id"`
	InterfaceID        string               `json:"interfaceId"        db:"interface_id"`
	SegmentID          string               `json:"segmentId"          db:"segment_id"`
	TargetFHIRResource string               `json:"targetFhirResource" db:"target_fhir_resource"`
	ResourceRole       string               `json:"resourceRole"       db:"resource_role"`
	Description        string               `json:"description"        db:"description"`
	OccurrenceIndex    int                  `json:"occurrenceIndex"    db:"occurrence_index"`
	SourceTemplateID   *string              `json:"sourceTemplateId"   db:"source_template_id"` // informational only
	IsActive           bool                 `json:"isActive"           db:"is_active"`
	SortOrder          int                  `json:"sortOrder"          db:"sort_order"`
	CreatedAt          time.Time            `json:"createdAt"          db:"created_at"`
	UpdatedAt          time.Time            `json:"updatedAt"          db:"updated_at"`
	FieldMappings      []ZSegmentFieldMapping `json:"fieldMappings"`
}

// ZSegmentFieldMapping maps one Z-segment position to a FHIR path on the
// target resource.  Fields are self-contained (copied from template on apply).
type ZSegmentFieldMapping struct {
	ID               string `json:"id"               db:"id"`
	ZSegmentConfigID string `json:"zsegmentConfigId" db:"zsegment_config_id"`
	Position         string `json:"position"         db:"position"`      // ZEM.1, ZEM.3, ZEM.3.2
	FieldLabel       string `json:"fieldLabel"       db:"field_label"`
	HL7DataType      string `json:"hl7DataType"      db:"hl7_data_type"`
	FHIRPath         string `json:"fhirPath"         db:"fhir_path"`
	Transform        string `json:"transform"        db:"transform"`
	IsRequired       bool   `json:"isRequired"       db:"is_required"`
	DefaultValue     string `json:"defaultValue"     db:"default_value"`
	SortOrder        int    `json:"sortOrder"        db:"sort_order"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Runtime config — passed to assembly at transform time
// ─────────────────────────────────────────────────────────────────────────────

// MFNRuntimeConfig is the resolved configuration handed to AssembleMFNResources.
// Loaded once per message via the ZSegmentService; nil means use fallback behaviour.
type MFNRuntimeConfig struct {
	// Tier 2: MFI.1 → FHIR resource overrides.
	// Keyed by upper-cased MFI.1 value (e.g. "EMP" → "Organization").
	MFIOverrides map[string]string `json:"mfiOverrides"`

	// Tier 3: ordered list of Z-segment configs for this interface.
	// Only active configs (is_active=true) are included.
	ZSegments []ZSegmentConfig `json:"zSegments"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response DTOs
// ─────────────────────────────────────────────────────────────────────────────

// CreateZSegmentConfigRequest is the payload for POST /api/interfaces/:id/zsegments
type CreateZSegmentConfigRequest struct {
	SegmentID          string                 `json:"segmentId"          binding:"required"`
	TargetFHIRResource string                 `json:"targetFhirResource" binding:"required"`
	ResourceRole       string                 `json:"resourceRole"`
	Description        string                 `json:"description"`
	OccurrenceIndex    int                    `json:"occurrenceIndex"`
	SortOrder          int                    `json:"sortOrder"`
	SourceTemplateID   *string                `json:"sourceTemplateId"`
	FieldMappings      []ZSegmentFieldMapping `json:"fieldMappings"`
}

// UpdateZSegmentConfigRequest is the payload for PUT /api/interfaces/:id/zsegments/:zid
type UpdateZSegmentConfigRequest struct {
	TargetFHIRResource string `json:"targetFhirResource"`
	ResourceRole       string `json:"resourceRole"`
	Description        string `json:"description"`
	OccurrenceIndex    int    `json:"occurrenceIndex"`
	IsActive           *bool  `json:"isActive"`
	SortOrder          int    `json:"sortOrder"`
}

// UpsertFieldMappingRequest is shared by create and update field mapping endpoints.
type UpsertFieldMappingRequest struct {
	Position     string `json:"position"     binding:"required"`
	FieldLabel   string `json:"fieldLabel"`
	HL7DataType  string `json:"hl7DataType"`
	FHIRPath     string `json:"fhirPath"     binding:"required"`
	Transform    string `json:"transform"`
	IsRequired   bool   `json:"isRequired"`
	DefaultValue string `json:"defaultValue"`
	SortOrder    int    `json:"sortOrder"`
}

// CreateZSegmentTemplateRequest is the payload for POST /api/zsegments/templates
type CreateZSegmentTemplateRequest struct {
	TemplateName       string                  `json:"templateName"       binding:"required"`
	SegmentID          string                  `json:"segmentId"          binding:"required"`
	TargetFHIRResource string                  `json:"targetFhirResource" binding:"required"`
	ResourceRole       string                  `json:"resourceRole"`
	Description        string                  `json:"description"`
	Tags               []string                `json:"tags"`
	IsPublic           bool                    `json:"isPublic"`
	Fields             []ZSegmentTemplateField `json:"fields"`
}

// ApplyTemplateRequest is the payload for
// POST /api/interfaces/:id/zsegments/apply-template
type ApplyTemplateRequest struct {
	TemplateID      string `json:"templateId"      binding:"required"`
	OccurrenceIndex int    `json:"occurrenceIndex"`
	SortOrder       int    `json:"sortOrder"`
}

// SaveAsTemplateRequest is the payload for
// POST /api/interfaces/:id/zsegments/:zid/save-as-template
type SaveAsTemplateRequest struct {
	TemplateName string   `json:"templateName" binding:"required"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	IsPublic     bool     `json:"isPublic"`
}
