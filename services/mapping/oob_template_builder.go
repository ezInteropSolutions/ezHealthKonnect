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
	// ExcludeSegments lists segments that must be dropped before building
	// the resource blocks. Use this when a segment's default resource mapping
	// is semantically wrong for a particular message type (e.g. PV1→Encounter
	// is wrong for SIU where PV1 carries visit context, not an Encounter).
	ExcludeSegments []string
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
	gen        *Generator
	db         *sql.DB
	igAnchors  *IGAnchorService
}

// NewOOBTemplateBuilder returns a builder wired to the global schema loaders
// and the given database connection.
func NewOOBTemplateBuilder(db *sql.DB) *OOBTemplateBuilder {
	return &OOBTemplateBuilder{
		gen:       NewGenerator(),
		db:        db,
		igAnchors: NewIGAnchorService(db),
	}
}

// hl7SupportedVersions lists every HL7 v2 version for which OOB templates are
// generated. BuildOne gracefully skips a version when the schema package is
// not installed, so adding a version here is always safe.
var hl7SupportedVersions = []string{"2.3", "2.3.1", "2.4", "2.5", "2.5.1", "2.6", "2.7", "2.8"}

// fhirSupportedVersions lists the FHIR specification versions to generate templates for.
// R5 schema files exist alongside R4; both are built and stored as separate rows.
var fhirSupportedVersions = []FHIRVersion{FHIRVersionR4, FHIRVersionR5}

// baseCatalog lists every standard message type once (version-agnostic).
// init() expands this into standardMessages by crossing with hl7SupportedVersions
// and fhirSupportedVersions.
var baseCatalog = []OOBMessageDef{
	// ── ADT — Patient Administration ─────────────────────────────────────
	{MessageType: "ADT^A01", FHIRVersion: FHIRVersionR4, Description: "Patient Admit/Visit Notification"},
	{MessageType: "ADT^A02", FHIRVersion: FHIRVersionR4, Description: "Patient Transfer"},
	{MessageType: "ADT^A03", FHIRVersion: FHIRVersionR4, Description: "Patient Discharge"},
	{MessageType: "ADT^A04", FHIRVersion: FHIRVersionR4, Description: "Patient Registration"},
	{MessageType: "ADT^A05", FHIRVersion: FHIRVersionR4, Description: "Patient Pre-Admit"},
	{MessageType: "ADT^A06", FHIRVersion: FHIRVersionR4, Description: "Change Outpatient to Inpatient"},
	{MessageType: "ADT^A07", FHIRVersion: FHIRVersionR4, Description: "Change Inpatient to Outpatient"},
	{MessageType: "ADT^A08", FHIRVersion: FHIRVersionR4, Description: "Update Patient Information"},
	{MessageType: "ADT^A11", FHIRVersion: FHIRVersionR4, Description: "Cancel Admit"},
	{MessageType: "ADT^A12", FHIRVersion: FHIRVersionR4, Description: "Cancel Transfer"},
	{MessageType: "ADT^A13", FHIRVersion: FHIRVersionR4, Description: "Cancel Discharge"},
	{MessageType: "ADT^A17", FHIRVersion: FHIRVersionR4, Description: "Swap Patients"},
	{MessageType: "ADT^A28", FHIRVersion: FHIRVersionR4, Description: "Add Person Information"},
	{MessageType: "ADT^A31", FHIRVersion: FHIRVersionR4, Description: "Update Person Information"},
	{MessageType: "ADT^A39", FHIRVersion: FHIRVersionR4, Description: "Merge Patient — Person Identifier List"},
	{MessageType: "ADT^A40", FHIRVersion: FHIRVersionR4, Description: "Merge Patient — Patient Identifier List"},
	{MessageType: "ADT^A41", FHIRVersion: FHIRVersionR4, Description: "Merge Patient — Patient Account Number"},
	{MessageType: "ADT^A42", FHIRVersion: FHIRVersionR4, Description: "Move Visit Information — Patient Identifier List"},
	{MessageType: "ADT^A43", FHIRVersion: FHIRVersionR4, Description: "Move Patient Information — Patient Identifier List"},
	{MessageType: "ADT^A45", FHIRVersion: FHIRVersionR4, Description: "Move Visit Information — Visit Number"},

	// ── ORU — Observation Reporting ───────────────────────────────────────
	{MessageType: "ORU^R01", FHIRVersion: FHIRVersionR4, Description: "Unsolicited Observation Message"},
	{MessageType: "ORU^R30", FHIRVersion: FHIRVersionR4, Description: "Unsolicited Point-Of-Care Observation"},

	// ── ORM / OML — Orders ───────────────────────────────────────────────
	{MessageType: "ORM^O01", FHIRVersion: FHIRVersionR4, Description: "General Order Message"},
	{MessageType: "OML^O21", FHIRVersion: FHIRVersionR4, Description: "Laboratory Order"},
	{MessageType: "OMG^O19", FHIRVersion: FHIRVersionR4, Description: "General Clinical Order"},

	// ── MDM — Medical Document Management ────────────────────────────────
	{MessageType: "MDM^T01", FHIRVersion: FHIRVersionR4, Description: "Original Document Notification"},
	{MessageType: "MDM^T02", FHIRVersion: FHIRVersionR4, Description: "Original Document Notification and Content"},
	{MessageType: "MDM^T11", FHIRVersion: FHIRVersionR4, Description: "Document Cancel Notification"},

	// ── SIU — Scheduling ──────────────────────────────────────────────────
	// PV1 is now included: it maps to Encounter which is correct — even a booking
	// message may carry a pre-admit Encounter (FHIR status "planned"). When
	// SCH.25 = COMPLETE/FULFILLED the Encounter represents the actual visit.
	// V116 assembly rules wire Encounter.appointment → Appointment and
	// Encounter.subject → Patient.
	// RGS is still excluded: structural grouping segment with no FHIR target.
	// TQ1 injected via SegmentOverrides (absent from global segmentToResource)
	// so it maps to Appointment timing fields only for SIU messages.
	// AIG/AIL/AIP map to Appointment and are merged into one block.
	{MessageType: "SIU^S12", FHIRVersion: FHIRVersionR4, Description: "Notification of New Appointment Booking",
		SegmentOverrides: map[string]string{"TQ1": "Appointment"},
		ExcludeSegments:  []string{"RGS"}},
	{MessageType: "SIU^S13", FHIRVersion: FHIRVersionR4, Description: "Notification of Appointment Rescheduling",
		SegmentOverrides: map[string]string{"TQ1": "Appointment"},
		ExcludeSegments:  []string{"RGS"}},
	{MessageType: "SIU^S14", FHIRVersion: FHIRVersionR4, Description: "Notification of Appointment Modification",
		SegmentOverrides: map[string]string{"TQ1": "Appointment"},
		ExcludeSegments:  []string{"RGS"}},
	{MessageType: "SIU^S15", FHIRVersion: FHIRVersionR4, Description: "Notification of Appointment Cancellation",
		SegmentOverrides: map[string]string{"TQ1": "Appointment"},
		ExcludeSegments:  []string{"RGS"}},
	{MessageType: "SIU^S17", FHIRVersion: FHIRVersionR4, Description: "Notification of Appointment Deletion",
		SegmentOverrides: map[string]string{"TQ1": "Appointment"},
		ExcludeSegments:  []string{"RGS"}},

	// ── VXU — Vaccination ─────────────────────────────────────────────────
	{MessageType: "VXU^V04", FHIRVersion: FHIRVersionR4, Description: "Unsolicited Vaccination Record Update"},

	// ── DFT — Detailed Financial Transaction ─────────────────────────────
	{MessageType: "DFT^P03", FHIRVersion: FHIRVersionR4, Description: "Post Detail Financial Transaction"},

	// ── BAR — Billing Account Record ─────────────────────────────────────
	{MessageType: "BAR^P01", FHIRVersion: FHIRVersionR4, Description: "Add Patient Account"},
	{MessageType: "BAR^P02", FHIRVersion: FHIRVersionR4, Description: "Purge Patient Account"},

	// ── MFN — Master File Notification ───────────────────────────────────
	{MessageType: "MFN^M02", FHIRVersion: FHIRVersionR4, Description: "Master File — Staff Practitioner",
		SegmentOverrides: map[string]string{"STF": "Practitioner", "PRA": "PractitionerRole", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M04", FHIRVersion: FHIRVersionR4, Description: "Master File — Charge Description",
		SegmentOverrides: map[string]string{"CDM": "ChargeItemDefinition", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M05", FHIRVersion: FHIRVersionR4, Description: "Master File — Location",
		SegmentOverrides: map[string]string{"LOC": "Location", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
	{MessageType: "MFN^M12", FHIRVersion: FHIRVersionR4, Description: "Master File — Lab Observation",
		SegmentOverrides: map[string]string{"OM1": "ObservationDefinition", "MFI": "MessageHeader", "MFE": "MessageHeader"}},
}

// standardMessages is the expanded catalogue:
//
//	baseCatalog × hl7SupportedVersions × fhirSupportedVersions
//
// Populated by init(); BuildOne skips any combination whose schema is not installed.
var standardMessages []OOBMessageDef

func init() {
	cap := len(baseCatalog) * len(hl7SupportedVersions) * len(fhirSupportedVersions)
	standardMessages = make([]OOBMessageDef, 0, cap)
	for _, base := range baseCatalog {
		for _, hl7Ver := range hl7SupportedVersions {
			for _, fhirVer := range fhirSupportedVersions {
				entry := base
				entry.HL7Version = hl7Ver
				entry.FHIRVersion = fhirVer
				standardMessages = append(standardMessages, entry)
			}
		}
	}
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

// msgTypeKeySegments maps a message type (or family prefix) to its characteristic
// non-universal segments. A message type has "IG coverage" if at least one key
// segment appears in ig_field_anchors. MSH/EVN are excluded — they're universal
// and not meaningful filters.
//
// Exact message-type keys take precedence over family-prefix keys (used for
// MFN subtypes that each cover different master segments).
var msgTypeKeySegments = map[string][]string{
	// Families matched by prefix (before "^")
	"ADT": {"PID", "PV1", "DG1", "AL1"},
	"ORU": {"OBX"},
	"ORM": {"ORC"},
	"OML": {"ORC"},
	"OMG": {"ORC"},
	"MDM": {"TXA"},
	"SIU": {"SCH"},
	"VXU": {"RXA"},
	"DFT": {"FT1"},
	"BAR": {"BPT"},
	// MFN subtypes — each subtype has its own master segment
	"MFN^M02": {"STF", "PRA"},
	"MFN^M04": {"CDM"},
	"MFN^M05": {"LOC"},
	"MFN^M12": {"OM1"},
}

// BuildForMessageTypes generates OOB templates only for the given message types
// (e.g. ["SIU^S12", "SIU^S13"]) across all HL7 and FHIR versions. This is much
// faster than BuildAll when only a subset of templates need refreshing after an
// anchor-table or Rule change. Pass a nil/empty slice to fall through to BuildAll.
func (b *OOBTemplateBuilder) BuildForMessageTypes(ctx context.Context, messageTypes []string) []BuildResult {
	if len(messageTypes) == 0 {
		return b.BuildAll(ctx)
	}
	want := make(map[string]bool, len(messageTypes))
	for _, mt := range messageTypes {
		want[mt] = true
	}
	var results []BuildResult
	for _, def := range standardMessages {
		if !want[def.MessageType] {
			continue
		}
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
	return results
}

// BuildWithIGCoverage generates OOB templates only for message types that have
// meaningful IG anchor coverage in ig_field_anchors. Message types whose
// characteristic segments are absent from the IG are skipped — we don't
// degrade them with heuristic-only generation when the IG has nothing to add.
//
// Called via POST /api/fhir/templates/rebuild-oob-ig
func (b *OOBTemplateBuilder) BuildWithIGCoverage(ctx context.Context) []BuildResult {
	covered, err := b.coveredSegments(ctx)
	if err != nil {
		log.Printf("[OOBBuilder/IG] ig-coverage check failed: %v — aborting", err)
		return nil
	}
	if len(covered) == 0 {
		log.Printf("[OOBBuilder/IG] ig_field_anchors table is empty — nothing to rebuild")
		return nil
	}

	log.Printf("[OOBBuilder/IG] IG-covered segments: %v", segmentSetKeys(covered))

	var results []BuildResult
	for _, def := range standardMessages {
		select {
		case <-ctx.Done():
			return results
		default:
		}
		if !b.hasIGCoverage(def.MessageType, covered) {
			continue
		}
		result := b.BuildOne(ctx, def)
		results = append(results, result)
		if result.Err != nil {
			log.Printf("[OOBBuilder/IG] %s hl7v%s fhir%s: ERROR %v",
				def.MessageType, def.HL7Version, def.FHIRVersion, result.Err)
		} else {
			log.Printf("[OOBBuilder/IG] %s hl7v%s fhir%s: %d resources, %d mappings",
				def.MessageType, def.HL7Version, def.FHIRVersion, result.ResourcesBuilt, result.MappingsBuilt)
		}
	}

	flagged, err := b.flagOutdatedCustomMappings(ctx)
	if err != nil {
		log.Printf("[OOBBuilder] update-notification scan error: %v", err)
	} else if flagged > 0 {
		log.Printf("[OOBBuilder] flagged %d custom mapping(s) as having a template update available", flagged)
	}
	return results
}

// coveredSegments returns the set of segment names that have active rows in
// ig_field_anchors — used to decide which message types to rebuild.
func (b *OOBTemplateBuilder) coveredSegments(ctx context.Context) (map[string]bool, error) {
	rows, err := b.db.QueryContext(ctx,
		`SELECT DISTINCT UPPER(segment) FROM ig_field_anchors WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("coveredSegments: %w", err)
	}
	defer rows.Close()
	covered := make(map[string]bool)
	for rows.Next() {
		var seg string
		if err := rows.Scan(&seg); err == nil {
			covered[seg] = true
		}
	}
	return covered, rows.Err()
}

// hasIGCoverage returns true when at least one of a message type's characteristic
// segments is present in the covered set queried from ig_field_anchors.
func (b *OOBTemplateBuilder) hasIGCoverage(messageType string, covered map[string]bool) bool {
	// Exact match first (handles MFN^M02 vs MFN^M04 distinction)
	if segs, ok := msgTypeKeySegments[messageType]; ok {
		for _, s := range segs {
			if covered[strings.ToUpper(s)] {
				return true
			}
		}
		return false
	}
	// Fall back to family prefix
	family := strings.SplitN(messageType, "^", 2)[0]
	if segs, ok := msgTypeKeySegments[family]; ok {
		for _, s := range segs {
			if covered[strings.ToUpper(s)] {
				return true
			}
		}
	}
	return false
}

func segmentSetKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

	// Load IG field anchors from DB and inject into the generator.
	// DB anchors override the hardcoded tables in semantic_matcher.go —
	// new IG mappings take effect without any Go code changes.
	if dbAnchors, err := b.igAnchors.LoadForVersion(ctx, def.FHIRVersion); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("ig_anchor_service: %v (using hardcoded anchors)", err))
	} else if dbAnchors != nil {
		b.gen.WithDBAnchors(dbAnchors)
	}

	msgSchema, hl7Warnings, err := b.gen.GenerateMessage(def.HL7Version, def.MessageType, def.FHIRVersion)
	if err != nil {
		// Schema not found for this exact event — skip gracefully
		result.Err = fmt.Errorf("generate: %w", err)
		return result
	}
	result.Warnings = append(result.Warnings, hl7Warnings...)

	// Drop excluded segments before any further processing.
	// ExcludeSegments is used when a segment's default resource is semantically
	// wrong for this message family (e.g. PV1→Encounter is wrong for SIU).
	if len(def.ExcludeSegments) > 0 {
		excludeSet := make(map[string]bool, len(def.ExcludeSegments))
		for _, s := range def.ExcludeSegments {
			excludeSet[s] = true
		}
		kept := msgSchema.Segments[:0]
		for _, seg := range msgSchema.Segments {
			if !excludeSet[seg.Segment] {
				kept = append(kept, seg)
			}
		}
		msgSchema.Segments = kept
	}

	// Apply segment overrides to the generated maps.
	// Two passes:
	//   Pass 1 — re-target segments that GenerateMessage already produced.
	//   Pass 2 — inject segments listed in SegmentOverrides that GenerateMessage
	//            skipped (absent from global segmentToResource). TQ1 in SIU is
	//            the canonical example: it has no global default resource so it
	//            was silently skipped; the override injects it as "Appointment".
	if len(def.SegmentOverrides) > 0 {
		// Pass 1: re-target existing segments
		for i := range msgSchema.Segments {
			if override, ok := def.SegmentOverrides[msgSchema.Segments[i].Segment]; ok {
				msgSchema.Segments[i].Resource = override
				corrected := b.regenerateWithResource(def, msgSchema.Segments[i].Segment, override)
				if corrected != nil {
					msgSchema.Segments[i] = *corrected
				}
			}
		}

		// Pass 2: inject override-only segments (not in segmentToResource globally)
		existing := make(map[string]bool, len(msgSchema.Segments))
		for _, s := range msgSchema.Segments {
			existing[s.Segment] = true
		}
		for segName, resource := range def.SegmentOverrides {
			if existing[segName] {
				continue // already handled in pass 1
			}
			injected, err := b.gen.GenerateSegment(def.HL7Version, def.MessageType, segName, resource, def.FHIRVersion)
			if err != nil || injected == nil || len(injected.Mappings) == 0 {
				continue // schema doesn't know this segment for this message version — skip silently
			}
			msgSchema.Segments = append(msgSchema.Segments, *injected)
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
		filtered := make([]GeneratedMapping, 0, len(seg.Mappings))
		for _, m := range seg.Mappings {
			// OOB templates contain only IG ConceptMap-backed mappings.
			// Heuristic candidates ("type_match", "name_similarity") are surfaced
			// in the wizard UI with confidence scores but must never appear in
			// OOB templates as if they were specified by the IG.
			if m.Source != "anchor" && m.Source != "ig_db" {
				continue
			}
			lp := strings.ToLower(m.FHIRPath)
			if !strings.Contains(lp, ".response.code") && !strings.Contains(lp, ".response.identifier") {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		resourceKey := seg.Resource
		if existing, exists := resources[resourceKey]; exists {
			// Multiple segments produce the same FHIR resource (e.g. SCH+AIG+AIL+AIP
			// all → Appointment, or PV1+PV2 → Encounter). Merge their mappings into
			// one block so filterMappingsForResource finds them all under the right key.
			existing.Mappings = append(existing.Mappings, filtered...)
			resources[resourceKey] = existing
		} else {
			resources[resourceKey] = TemplateResourceBlock{
				Segment:  seg.Segment,
				Optional: seg.Optional,
				Mappings: filtered,
			}
			result.ResourcesBuilt++
		}
		result.MappingsBuilt += len(filtered)
	}

	// Apply field-level corrections that schema-driven generation cannot infer.
	resources = applyServiceRequestOverrides(resources)
	resources = applyChargeItemOverrides(resources)

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

	templateName := "OOB_" + strings.ReplaceAll(def.MessageType, "^", "_") +
		"_v" + def.HL7Version + "_" + string(def.FHIRVersion)

	err = b.upsertTemplate(ctx, def.MessageType, def.HL7Version, string(def.FHIRVersion), templateName, def.Description, string(cfgJSON), string(frJSON))
	if err != nil {
		result.Err = fmt.Errorf("upsert template: %w", err)
	}
	return result
}

// regenerateWithResource re-runs GenerateSegment with a specific fhirResource
// to get correctly-pathed mappings after an override.
// DB anchors are already loaded into b.gen by BuildOne — no reload needed.
func (b *OOBTemplateBuilder) regenerateWithResource(def OOBMessageDef, segment, fhirResource string) *SegmentMap {
	seg, err := b.gen.GenerateSegment(def.HL7Version, def.MessageType, segment, fhirResource, def.FHIRVersion)
	if err != nil {
		return nil
	}
	return seg
}

func (b *OOBTemplateBuilder) upsertTemplate(ctx context.Context,
	messageType, hl7Version, fhirVersion, name, description, cfgJSON, frJSON string,
) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO hl7_fhir_templates
		    (message_type, hl7_version, fhir_version, template_name, template_description,
		     profile_version, template_config, fhir_resources, is_default, is_system, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, true, true, $9)
		ON CONFLICT (message_type, hl7_version, fhir_version, is_default)
		DO UPDATE SET
		    template_name        = EXCLUDED.template_name,
		    template_description = EXCLUDED.template_description,
		    profile_version      = EXCLUDED.profile_version,
		    template_config      = EXCLUDED.template_config,
		    fhir_resources       = EXCLUDED.fhir_resources,
		    is_system            = true,
		    updated_at           = EXCLUDED.updated_at`,
		messageType, hl7Version, fhirVersion, name, description, TemplateVersion,
		cfgJSON, frJSON, time.Now(),
	)
	return err
}

// ── Post-generation field overrides ──────────────────────────────────────────
//
// These tables encode corrections that schema-driven generation cannot infer:
//   - HL7 fields with no valid FHIR target (exclusions)
//   - Fields whose HL7 table → FHIR value binding is unambiguous per the IG
//     ConceptMaps and should be baked into the generated template (valueMaps)
//
// Rules applied in applyServiceRequestOverrides, called by BuildOne.

// serviceRequestExclusions lists ORC source fields that must never appear in a
// ServiceRequest resource block.  ORC.13 (Enterer's Location, PL composite) is
// the canonical example: the HL7-to-FHIR IG has no ConceptMap entry for it and
// its raw PL value (e.g. "^^^^OFFICE^^^^^Office") is incompatible with every
// ServiceRequest element.
var serviceRequestExclusions = map[string]bool{
	"ORC.13": true,
}

// serviceRequestValueMaps injects valueMap annotations into generated ServiceRequest
// mappings for fields whose HL7 table → FHIR value binding is standard.
// Applied after schema-driven generation so the upsert always writes the correct map.
var serviceRequestValueMaps = map[string]map[string]interface{}{
	// ORC.1 (Order Control Code, HL7 Table 0119) → ServiceRequest.intent
	"ORC.1": {
		"NW": "order", "CA": "proposal", "OC": "proposal", "HD": "proposal",
		"RP": "order", "SC": "order", "IP": "order", "CM": "order",
		"DC": "proposal", "DE": "proposal", "DF": "proposal", "DR": "proposal",
		"FU": "plan", "LI": "plan", "PA": "plan",
		"RE": "reflex-order", "RL": "order", "RO": "reflex-order",
		"RQ": "proposal", "RR": "order", "RU": "order",
		"SN": "order", "SS": "order", "UA": "order", "UN": "order",
		"UR": "order", "UX": "order", "XO": "order", "XR": "order",
	},
	// ORC.5 (Order Status, HL7 Table 0038) → ServiceRequest.status
	// Includes plain-English variants sent by systems that don't follow Table 0038.
	"ORC.5": {
		"A":           "active",
		"CA":          "revoked",
		"CM":          "completed",
		"DC":          "revoked",
		"ER":          "entered-in-error",
		"HD":          "on-hold",
		"IP":          "active",
		"RP":          "revoked",
		"SC":          "active",
		"Pending":     "active",
		"Scheduled":   "active",
		"Hold":        "on-hold",
		"Cancelled":   "revoked",
		"Completed":   "completed",
		"In Progress": "active",
		"New":         "draft",
	},
}

// applyServiceRequestOverrides post-processes the generated resources map:
//   - removes excluded ORC fields from ServiceRequest
//   - injects standard valueMaps for ORC.1 (intent) and ORC.5 (status)
//
// Called by BuildOne after generation, before JSON marshaling.
// chargeItemExclusions lists FT1 source fields that must never appear in a
// ChargeItem resource block because their raw HL7 values are incompatible
// with every ChargeItem element path they would otherwise be heuristically
// mapped to.
//
//   - FT1.6 (Transaction Type, IS "CG"/"CR") has no ChargeItem counterpart;
//     heuristically it lands on code, which is already correctly anchored to
//     FT1.7 (the actual procedure/charge code).
//   - FT1.11 (Transaction Amount Extended, NM "0.000000") lands on
//     definitionCanonical — price is not a canonical URL.
//   - FT1.2 (Transaction ID, NM "1133") lands on implicitRules — a numeric
//     transaction ID is not a FHIR implicitRules URI.
var chargeItemExclusions = map[string]bool{
	"FT1.6":  true, // Transaction Type (CG/CR) — no ChargeItem target
	"FT1.11": true, // Transaction Amount Extended — not a canonical URL
	"FT1.2":  true, // Transaction ID — not a FHIR implicitRules URI
}

// applyChargeItemOverrides post-processes the generated resources map:
//   - removes excluded FT1 fields from ChargeItem (see chargeItemExclusions)
//
// Called by BuildOne after generation, before JSON marshaling.
func applyChargeItemOverrides(resources map[string]TemplateResourceBlock) map[string]TemplateResourceBlock {
	block, ok := resources["ChargeItem"]
	if !ok {
		return resources
	}
	kept := make([]GeneratedMapping, 0, len(block.Mappings))
	for _, m := range block.Mappings {
		if chargeItemExclusions[m.HL7Path] {
			continue
		}
		kept = append(kept, m)
	}
	block.Mappings = kept
	resources["ChargeItem"] = block
	return resources
}

func applyServiceRequestOverrides(resources map[string]TemplateResourceBlock) map[string]TemplateResourceBlock {
	block, ok := resources["ServiceRequest"]
	if !ok {
		return resources
	}

	kept := make([]GeneratedMapping, 0, len(block.Mappings))
	for _, m := range block.Mappings {
		if serviceRequestExclusions[m.HL7Path] {
			continue
		}
		if vm, hasVM := serviceRequestValueMaps[m.HL7Path]; hasVM {
			m.ValueMap = vm
		}
		kept = append(kept, m)
	}
	block.Mappings = kept
	resources["ServiceRequest"] = block
	return resources
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
