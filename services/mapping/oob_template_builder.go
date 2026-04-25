// services/mapping/oob_template_builder.go
// OOB (Out-of-Box) template builder — generates all standard HL7→FHIR
// mapping templates from FHIR StructureDefinitions and HL7 schemas.
//
// This replaces hand-written SQL migrations (V62, V106, V107, V108 etc.)
// with spec-driven generation. Running Build() produces a complete, correct
// set of templates and upserts them into hl7_fhir_templates.
//
// Adding a new FHIR version: add an entry to fhirVersions and call
// Build() — no other code changes required.
//
// API endpoint: POST /api/fhir/templates/rebuild-oob  (admin only)
package mapping

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// TemplateVersion is the profile_version written to every generated template.
const TemplateVersion = "2.0"

// OOBMessageDef describes one standard HL7 message type and which FHIR
// resources its segments should produce.
type OOBMessageDef struct {
	MessageType   string            // e.g. "ADT^A01"
	HL7Version    string            // e.g. "2.5"
	FHIRVersion   FHIRVersion       // e.g. FHIRVersionR4
	Description   string
	// SegmentOverrides lets callers force a specific resource for a segment
	// instead of using the default segmentToResource table.
	SegmentOverrides map[string]string // segmentName → fhirResourceType
}

// TemplateResourceBlock is one entry in template_config.resources.
type TemplateResourceBlock struct {
	Segment  string             `json:"segment"`
	Optional bool               `json:"optional"`
	Mappings []GeneratedMapping `json:"mappings"`
}

// TemplateConfig is the full template_config JSON written to the DB.
type TemplateConfig struct {
	Version     string                           `json:"version"`
	MessageType string                           `json:"messageType"`
	Context     map[string]string                `json:"context"`
	Resources   map[string]TemplateResourceBlock `json:"resources"`
}

// BuildResult reports the outcome for one message type.
type BuildResult struct {
	MessageType  string
	ResourcesBuilt int
	MappingsBuilt  int
	Warnings     []string
	Err          error
}

// OOBTemplateBuilder generates and persists OOB templates.
type OOBTemplateBuilder struct {
	gen *Generator
	db  *sql.DB
}

// NewOOBTemplateBuilder returns a builder wired to the global schema loaders
// and the given database connection.
func NewOOBTemplateBuilder(db *sql.DB) *OOBTemplateBuilder {
	return &OOBTemplateBuilder{
		gen: NewGenerator(),
		db:  db,
	}
}

// standardMessages is the authoritative catalogue of OOB message types.
// Each entry drives one template in hl7_fhir_templates.
var standardMessages = []OOBMessageDef{
	// ── ADT — Patient Administration ─────────────────────────────────────
	{MessageType: "ADT^A01", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Patient Admit/Visit Notification"},
	{MessageType: "ADT^A02", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Patient Transfer"},
	{MessageType: "ADT^A03", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Patient Discharge"},
	{MessageType: "ADT^A04", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Patient Registration"},
	{MessageType: "ADT^A05", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Patient Pre-Admit"},
	{MessageType: "ADT^A06", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Change Outpatient to Inpatient"},
	{MessageType: "ADT^A07", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Change Inpatient to Outpatient"},
	{MessageType: "ADT^A08", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Update Patient Information"},
	{MessageType: "ADT^A11", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Cancel Admit"},
	{MessageType: "ADT^A12", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Cancel Transfer"},
	{MessageType: "ADT^A13", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Cancel Discharge"},
	{MessageType: "ADT^A17", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Swap Patients"},
	{MessageType: "ADT^A28", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Add Person Information"},
	{MessageType: "ADT^A31", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Update Person Information"},
	{MessageType: "ADT^A40", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Merge Patient — Patient Identifier List"},
	{MessageType: "ADT^A45", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Move Visit Information — Visit Number"},

	// ── ORU — Observation Reporting ───────────────────────────────────────
	{MessageType: "ORU^R01", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Unsolicited Observation Message"},
	{MessageType: "ORU^R30", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Unsolicited Point-Of-Care Observation"},

	// ── ORM / OML — Orders ───────────────────────────────────────────────
	{MessageType: "ORM^O01", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "General Order Message"},
	{MessageType: "OML^O21", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Laboratory Order"},
	{MessageType: "OMG^O19", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "General Clinical Order"},

	// ── MDM — Medical Document Management ────────────────────────────────
	{MessageType: "MDM^T01", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Original Document Notification"},
	{MessageType: "MDM^T02", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Original Document Notification and Content"},
	{MessageType: "MDM^T11", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Document Cancel Notification"},

	// ── SIU — Scheduling ──────────────────────────────────────────────────
	{MessageType: "SIU^S12", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Notification of New Appointment Booking"},
	{MessageType: "SIU^S13", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Notification of Appointment Rescheduling"},
	{MessageType: "SIU^S14", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Notification of Appointment Modification"},
	{MessageType: "SIU^S15", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Notification of Appointment Cancellation"},
	{MessageType: "SIU^S17", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Notification of Appointment Deletion"},

	// ── VXU — Vaccination ─────────────────────────────────────────────────
	{MessageType: "VXU^V04", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Unsolicited Vaccination Record Update"},

	// ── DFT — Detailed Financial Transaction ─────────────────────────────
	{MessageType: "DFT^P03", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Post Detail Financial Transaction"},

	// ── BAR — Billing Account Record ─────────────────────────────────────
	{MessageType: "BAR^P01", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Add Patient Account"},
	{MessageType: "BAR^P02", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Purge Patient Account"},

	// ── MFN — Master File Notification ───────────────────────────────────
	{MessageType: "MFN^M02", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Master File — Staff Practitioner",
		SegmentOverrides: map[string]string{"STF": "Practitioner", "PRA": "PractitionerRole", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M04", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Master File — Charge Description",
		SegmentOverrides: map[string]string{"CDM": "ChargeItemDefinition", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M05", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Master File — Location",
		SegmentOverrides: map[string]string{"LOC": "Location", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M12", HL7Version: "2.5", FHIRVersion: FHIRVersionR4,
		Description: "Master File — Lab Observation",
		SegmentOverrides: map[string]string{"OM1": "ObservationDefinition", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
}

// contextForMessage returns the context block for a template based on
// which segments are expected in the message (used for contextLinks wiring).
var contextForMessage = map[string]map[string]string{
	"ADT": {"patient": "PID", "encounter": "PV1"},
	"ORU": {"patient": "PID", "encounter": "PV1", "order": "ORC"},
	"ORM": {"patient": "PID", "encounter": "PV1", "order": "ORC"},
	"OML": {"patient": "PID", "encounter": "PV1", "order": "ORC"},
	"OMG": {"patient": "PID", "encounter": "PV1", "order": "ORC"},
	"MDM": {"patient": "PID", "encounter": "PV1"},
	"SIU": {"patient": "PID"},
	"VXU": {"patient": "PID"},
	"DFT": {"patient": "PID", "encounter": "PV1"},
	"BAR": {"patient": "PID", "encounter": "PV1"},
	"MFN": {},
}

// BuildAll generates and persists OOB templates for all standard message types.
// Existing templates are replaced. After all templates are rebuilt, flags any
// custom interface mappings whose base template version has changed.
func (b *OOBTemplateBuilder) BuildAll(ctx context.Context) []BuildResult {
	results := make([]BuildResult, 0, len(standardMessages))
	for _, def := range standardMessages {
		select {
		case <-ctx.Done():
			return results
		default:
		}
		result := b.BuildOne(ctx, def)
		results = append(results, result)
		if result.Err != nil {
			log.Printf("[OOBBuilder] %s: ERROR %v", def.MessageType, result.Err)
		} else {
			log.Printf("[OOBBuilder] %s: %d resources, %d mappings, %d warnings",
				def.MessageType, result.ResourcesBuilt, result.MappingsBuilt, len(result.Warnings))
		}
	}
	flagged, err := b.flagOutdatedCustomMappings(ctx)
	if err != nil {
		log.Printf("[OOBBuilder] update-notification scan error: %v", err)
	} else if flagged > 0 {
		log.Printf("[OOBBuilder] flagged %d custom interface mapping(s) as having a template update available", flagged)
	}
	return results
}

// flagOutdatedCustomMappings finds every custom interface_message_mappings row
// whose mapping_overrides.based_on_version differs from the current OOB template's
// profile_version, and sets template_update_available = true on those rows.
// Returns the number of rows updated.
func (b *OOBTemplateBuilder) flagOutdatedCustomMappings(ctx context.Context) (int, error) {
	// Pull all custom mappings that have overrides with a based_on_version recorded.
	rows, err := b.db.QueryContext(ctx, `
		SELECT imm.id,
		       imm.message_type,
		       imm.mapping_overrides->>'based_on_version' AS based_on_version,
		       t.id                                       AS current_template_id,
		       t.profile_version                          AS current_version
		FROM   interface_message_mappings imm
		JOIN   hl7_fhir_templates t
		       ON  t.message_type = imm.message_type
		       AND t.is_default   = true
		WHERE  imm.mapping_overrides IS NOT NULL
		  AND  imm.mapping_overrides->>'based_on_version' IS NOT NULL
		  AND  imm.mapping_overrides->>'based_on_version' != COALESCE(t.profile_version, '1.0')
	`)
	if err != nil {
		return 0, fmt.Errorf("flagOutdatedCustomMappings query: %w", err)
	}
	defer rows.Close()

	type row struct {
		id               string
		currentTemplID   string
		currentVersion   string
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		var messageType, basedOn string
		if err := rows.Scan(&r.id, &messageType, &basedOn, &r.currentTemplID, &r.currentVersion); err != nil {
			continue
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(toUpdate) == 0 {
		return 0, nil
	}

	updated := 0
	for _, r := range toUpdate {
		_, err := b.db.ExecContext(ctx, `
			UPDATE interface_message_mappings
			SET    template_update_available  = true,
			       available_template_id      = $1::uuid,
			       available_template_version = $2
			WHERE  id = $3::uuid
		`, r.currentTemplID, r.currentVersion, r.id)
		if err != nil {
			log.Printf("[OOBBuilder] could not flag mapping %s: %v", r.id, err)
			continue
		}
		updated++
	}
	return updated, nil
}

// BuildOne generates and persists a single OOB template.
func (b *OOBTemplateBuilder) BuildOne(ctx context.Context, def OOBMessageDef) BuildResult {
	result := BuildResult{MessageType: def.MessageType}

	msgSchema, hl7Warnings, err := b.gen.GenerateMessage(def.HL7Version, def.MessageType, def.FHIRVersion)
	if err != nil {
		// Schema not found for this exact event — skip gracefully
		result.Err = fmt.Errorf("generate: %w", err)
		return result
	}
	result.Warnings = append(result.Warnings, hl7Warnings...)

	// Apply segment overrides to the generated maps
	if len(def.SegmentOverrides) > 0 {
		for i := range msgSchema.Segments {
			if override, ok := def.SegmentOverrides[msgSchema.Segments[i].Segment]; ok {
				msgSchema.Segments[i].Resource = override
				// Re-run matcher with correct resource for better FHIR paths
				corrected := b.regenerateWithResource(def, msgSchema.Segments[i].Segment, override)
				if corrected != nil {
					msgSchema.Segments[i] = *corrected
				}
			}
		}
	}

	// Build template_config
	eventFamily := strings.SplitN(def.MessageType, "^", 2)[0]
	ctx2, _ := contextForMessage[eventFamily]
	if ctx2 == nil {
		ctx2 = map[string]string{}
	}

	resources := make(map[string]TemplateResourceBlock, len(msgSchema.Segments))
	for _, seg := range msgSchema.Segments {
		if len(seg.Mappings) == 0 {
			continue
		}
		resourceKey := seg.Resource
		// De-duplicate resource keys (e.g. two segments → same resource → index suffix)
		if _, exists := resources[resourceKey]; exists {
			resourceKey = resourceKey + "_" + seg.Segment
		}
		resources[resourceKey] = TemplateResourceBlock{
			Segment:  seg.Segment,
			Optional: seg.Optional,
			Mappings: seg.Mappings,
		}
		result.ResourcesBuilt++
		result.MappingsBuilt += len(seg.Mappings)
	}

	cfg := TemplateConfig{
		Version:     TemplateVersion,
		MessageType: def.MessageType,
		Context:     ctx2,
		Resources:   resources,
	}

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		result.Err = fmt.Errorf("marshal template_config: %w", err)
		return result
	}

	// Collect FHIR resource type list
	fhirResources := uniqueResources(resources)
	frJSON, _ := json.Marshal(fhirResources)

	templateName := "OOB_" + strings.ReplaceAll(def.MessageType, "^", "_")

	err = b.upsertTemplate(ctx, def.MessageType, templateName, def.Description, string(cfgJSON), string(frJSON))
	if err != nil {
		result.Err = fmt.Errorf("upsert template: %w", err)
	}
	return result
}

// regenerateWithResource re-runs GenerateSegment with a specific fhirResource
// to get correctly-pathed mappings after an override.
func (b *OOBTemplateBuilder) regenerateWithResource(def OOBMessageDef, segment, fhirResource string) *SegmentMap {
	seg, err := b.gen.GenerateSegment(def.HL7Version, def.MessageType, segment, fhirResource, def.FHIRVersion)
	if err != nil {
		return nil
	}
	return seg
}

func (b *OOBTemplateBuilder) upsertTemplate(ctx context.Context,
	messageType, name, description, cfgJSON, frJSON string,
) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO hl7_fhir_templates
		    (message_type, template_name, template_description, profile_version,
		     template_config, fhir_resources, is_default, is_system, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, true, true, $7)
		ON CONFLICT (message_type, hl7_version, is_default)
		DO UPDATE SET
		    template_name        = EXCLUDED.template_name,
		    template_description = EXCLUDED.template_description,
		    profile_version      = EXCLUDED.profile_version,
		    template_config      = EXCLUDED.template_config,
		    fhir_resources       = EXCLUDED.fhir_resources,
		    is_system            = true,
		    updated_at           = EXCLUDED.updated_at`,
		messageType, name, description, TemplateVersion,
		cfgJSON, frJSON, time.Now(),
	)
	return err
}

func uniqueResources(resources map[string]TemplateResourceBlock) []string {
	seen := make(map[string]bool)
	var out []string
	// Resource keys are the map keys themselves (e.g. "Practitioner", "Patient")
	for resourceKey := range resources {
		// Strip any "_SEGMENT" suffix added for deduplication
		base := resourceKey
		if idx := strings.LastIndex(resourceKey, "_"); idx > 0 {
			candidate := resourceKey[:idx]
			if _, ok := segmentToResource[resourceKey[idx+1:]]; ok {
				base = candidate
			}
		}
		if !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}
	return out
}
