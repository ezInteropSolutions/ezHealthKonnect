// services/mapping/schema_map_generator.go
// Schema-driven HL7 v2 → FHIR position-map generator.
//
// Called at DESIGN TIME only (wizard, AI suggest, OOB template build).
// Output is a []GeneratedMapping slice that callers store in the
// hl7_fhir_templates table. The runtime executor reads the stored map
// directly — this package is never imported in the hot path.
//
// Usage:
//
//	gen := NewGenerator()
//	maps, err := gen.GenerateSegment("2.5", "ADT_A01", "PID", "Patient", "R4")
//	maps, err := gen.GenerateMessage("2.5", "ADT_A01", "R4")
package mapping

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
)

// GeneratedMapping is the output unit of the generator.
// Its JSON tags match the field names used in hl7_fhir_templates.template_config
// so callers can marshal directly into the database JSON.
type GeneratedMapping struct {
	HL7Path      string  `json:"hl7Path"`
	FHIRPath     string  `json:"fhirPath"`
	HL7DataType  string  `json:"hl7DataType"`
	FHIRDataType string  `json:"fhirDataType"`
	Transform    string  `json:"transform"`
	Required     bool    `json:"required"`
	Confidence   float64 `json:"confidence"`
	Notes        string  `json:"notes,omitempty"`
}

// SegmentMap is the generator output for one segment → resource pairing,
// shaped to drop directly into a template_config resources block.
type SegmentMap struct {
	Segment  string             `json:"segment"`
	Resource string             `json:"resource"`
	Optional bool               `json:"optional"`
	Mappings []GeneratedMapping `json:"mappings"`
}

// MessageMap is the full generator output for a message type — one SegmentMap
// per segment present in the message structure.
type MessageMap struct {
	MessageType string       `json:"messageType"`
	HL7Version  string       `json:"hl7Version"`
	FHIRVersion FHIRVersion  `json:"fhirVersion"`
	Segments    []SegmentMap `json:"segments"`
}

// segmentToResource is the authoritative segment → primary FHIR resource table.
// When a message type has a segment not listed here, the generator skips it
// with a warning rather than producing a wrong mapping.
var segmentToResource = map[string]string{
	"MSH": "MessageHeader",
	"PID": "Patient",
	"PV1": "Encounter",
	"EVN": "Encounter",
	"OBR": "DiagnosticReport",
	"OBX": "Observation",
	"ORC": "ServiceRequest",
	"AL1": "AllergyIntolerance",
	"DG1": "Condition",
	"NK1": "RelatedPerson",
	"IN1": "Coverage",
	"IN2": "Coverage",
	"ROL": "PractitionerRole",
	"GT1": "RelatedPerson",
	"PR1": "Procedure",
	"FT1": "ChargeItem",
	"TXA": "DocumentReference",
	"SCH": "Appointment",
	"AIS": "Appointment",
	"AIG": "Appointment", // General Resource → participant actor
	"AIL": "Appointment", // Location Resource → participant actor
	"AIP": "Appointment", // Personnel Resource → participant actor
	"RXA": "Immunization",
	"RXR": "Immunization",
	"STF": "Practitioner",
	"PRA": "PractitionerRole",
	"LOC": "Location",
	"CDM": "ChargeItemDefinition",
	"OM1": "ObservationDefinition",
	"MFI": "MessageHeader",
	"MFE": "MessageHeader",
	"PD1": "Patient",
	"PV2": "Encounter",
	"ACC": "Condition",
	"AUT": "ClaimResponse",
	"CTI": "ResearchStudy",
	"IAM": "AllergyIntolerance",
	"ZPD": "Patient",
}

// ── Field Classification ────────────────────────────────────────────────────
//
// FieldCategory classifies the semantic role of an HL7 v2 field for FHIR
// mapping purposes. Non-Mappable categories are skipped by GenerateSegment so
// they never appear in generated templates.
//
// Classification is name-driven (field.Name from the HL7 schema), not
// position-driven, so it works universally across message types and versions.
// Position-based exceptions (OBX.2) are kept only where the name alone is
// insufficient — and documented inline.
type FieldCategory int

const (
	// FieldCategoryMappable is the default: the field should be included in
	// the generated FHIR mapping.
	FieldCategoryMappable FieldCategory = iota

	// FieldCategorySetID covers "Set ID-*" fields (SI data type, sequential
	// integer position). They identify which repetition of a segment group
	// this is and have no FHIR counterpart.
	FieldCategorySetID

	// FieldCategoryActionCode covers "Segment Action Code" fields
	// (A=add, D=delete, U=update). They are CRUD verbs, not content.
	FieldCategoryActionCode

	// FieldCategoryDispatchControl covers fields whose value is read at
	// runtime to select a dispatch path rather than stored as content.
	// OBX.2 "Value Type" is the canonical example: its value (NM, ST, CWE…)
	// drives which FHIR value[x] variant OBX.5 maps to.
	FieldCategoryDispatchControl

	// FieldCategoryUnitModifier covers "*Units" qualifier fields. The unit
	// string modifies a sibling numeric field (e.g. SCH.10 qualifies SCH.9
	// duration). Emitting it as a standalone mapping creates false collisions
	// with CodeableConcept targets like Appointment.appointmentType.
	FieldCategoryUnitModifier

	// FieldCategoryContactInfo covers scheduling contact / entered-by person
	// fields in SCH, AIS, AIG, AIL, and AIP. These are XCN/XTN/XAD fields
	// representing the person who requested or entered the appointment — not
	// clinical content. Mapping them produces raw "id^family^given^" strings
	// in FHIR Reference.display, which violates the no-caret rule.
	FieldCategoryContactInfo

	// FieldCategoryResourceStatus covers "Filler Status Code" / "Placer
	// Status Code" in scheduling resource segments AIS/AIG/AIL/AIP. These
	// carry per-resource accept/decline status (not the appointment status),
	// and when mapped heuristically they overwrite Appointment.reasonCode
	// with values like "Scheduled".
	FieldCategoryResourceStatus

	// FieldCategoryProtocolDelimiter covers MSH.1 "Field Separator" (always
	// the literal "|" character) and MSH.2 "Encoding Characters" (always
	// "^~\&"). These define the HL7 encoding syntax; they are not content
	// and have no FHIR counterpart. Without this skip the heuristic maps
	// them to artefact paths like MessageHeader.implicitRules.
	FieldCategoryProtocolDelimiter
)

// classifyField returns the semantic category of an HL7 field given its
// parent segment name, field number, and human-readable field name from the
// schema. The caller should skip any field whose category is not
// FieldCategoryMappable.
func classifyField(segmentName, fieldNum, fieldName string) FieldCategory {
	lower := strings.ToLower(fieldName)

	if isSetIDField(lower) {
		return FieldCategorySetID
	}
	if isActionCodeField(lower) {
		return FieldCategoryActionCode
	}
	if isDispatchControlField(segmentName, lower) {
		return FieldCategoryDispatchControl
	}
	if isUnitModifierField(lower) {
		return FieldCategoryUnitModifier
	}
	if isSchedulingContactField(segmentName, lower) {
		return FieldCategoryContactInfo
	}
	if isResourceStatusField(segmentName, lower) {
		return FieldCategoryResourceStatus
	}
	if isProtocolDelimiterField(segmentName, lower) {
		return FieldCategoryProtocolDelimiter
	}
	return FieldCategoryMappable
}

// isSetIDField returns true for "Set ID-*" fields (e.g. "Set ID - PID",
// "Set ID – AIS"). These are sequential position integers with no FHIR meaning.
func isSetIDField(lowerName string) bool {
	return strings.HasPrefix(lowerName, "set id")
}

// isActionCodeField returns true for "Segment Action Code" fields.
func isActionCodeField(lowerName string) bool {
	return lowerName == "segment action code"
}

// isDispatchControlField returns true for fields whose value selects a
// runtime dispatch path rather than carrying mappable content.
// OBX.2 "Value Type" is the only current case; it is also position-guarded
// because some older schema files use different names for this field.
func isDispatchControlField(segmentName, lowerName string) bool {
	if segmentName == "OBX" && (lowerName == "value type" || fieldNameIsOBX2Alias(lowerName)) {
		return true
	}
	return false
}

// fieldNameIsOBX2Alias catches alternate names for OBX.2 seen in older schema
// files ("observation value type", "obx-2 value type", etc.).
func fieldNameIsOBX2Alias(lowerName string) bool {
	return strings.Contains(lowerName, "value type")
}

// isUnitModifierField returns true for fields whose name ends with "units"
// or "unit", indicating they qualify a sibling numeric field rather than
// carrying standalone mappable content (e.g. "Appointment Duration Units",
// "Duration Units", "Observation Units").
func isUnitModifierField(lowerName string) bool {
	return strings.HasSuffix(lowerName, " units") ||
		strings.HasSuffix(lowerName, " unit") ||
		strings.HasSuffix(lowerName, "units") // no leading space variant
}

// isSchedulingContactField returns true for contact-person, entered-by, and
// requesting-person fields in scheduling segments. These XCN/XTN/XAD fields
// represent administrative people, not clinical content. Mapping them
// produces raw "id^family^given^" composite strings in FHIR output.
func isSchedulingContactField(segmentName, lowerName string) bool {
	switch segmentName {
	case "SCH", "AIS", "AIG", "AIL", "AIP":
	default:
		return false
	}
	return strings.Contains(lowerName, "contact") ||
		strings.Contains(lowerName, "entered by") ||
		strings.Contains(lowerName, "requesting person") ||
		strings.Contains(lowerName, "filler contact") ||
		strings.Contains(lowerName, "placer contact")
}

// isResourceStatusField returns true for per-resource status fields in
// scheduling resource segments (AIS/AIG/AIL/AIP). "Filler Status Code" and
// "Placer Status Code" carry accept/decline/booked status at the resource
// level; they are not the appointment status and should not overwrite it.
// SCH.25 "Filler Status Code" is the appointment-level status and IS mappable
// — so we restrict this classification to resource segments only.
func isResourceStatusField(segmentName, lowerName string) bool {
	switch segmentName {
	case "AIS", "AIG", "AIL", "AIP":
	default:
		return false
	}
	return strings.Contains(lowerName, "filler status") ||
		strings.Contains(lowerName, "placer status")
}

// isProtocolDelimiterField returns true for MSH.1 "Field Separator" and
// MSH.2 "Encoding Characters". These fields define the HL7 wire encoding
// and are never content — their literal values ("|" and "^~\&") must not
// be mapped to FHIR elements.
func isProtocolDelimiterField(segmentName, lowerName string) bool {
	if segmentName != "MSH" {
		return false
	}
	return lowerName == "field separator" || lowerName == "encoding characters"
}

// Generator orchestrates schema lookups to produce position maps.
type Generator struct {
	hl7Loader  *hl7.RealSchemaLoader
	fhirLoader *fhir.FHIRSchemaLoader
	// dbAnchors optionally overrides / extends the hardcoded anchor table.
	// Loaded from ig_field_anchors by IGAnchorService; nil = use hardcoded only.
	// Keyed "SEGMENT.FIELD" → []Candidate, same format as knownAnchorsR4.
	dbAnchors map[string][]Candidate
}

// NewGenerator returns a Generator wired to the process-global schema loaders.
// Both loaders must have been initialised (InitRealSchemaLoader /
// InitFHIRSchemaLoader) before calling this.
func NewGenerator() *Generator {
	return &Generator{
		hl7Loader:  hl7.GetRealSchemaLoader(),
		fhirLoader: fhir.GetFHIRSchemaLoader(),
	}
}

// WithDBAnchors injects IG-sourced anchors from the database.
// DB anchors take priority over the hardcoded knownAnchorsR4/R5 tables —
// they carry confidence=1.0 and source="ig_db".
func (g *Generator) WithDBAnchors(anchors map[string][]Candidate) *Generator {
	g.dbAnchors = anchors
	return g
}

// matchField resolves candidates for one field, checking DB anchors first.
func (g *Generator) matchField(
	segmentName, fieldKey, fieldName, hl7DataType, fhirResource string,
	fhirSchema *fhir.FHIRSchema,
	fhirVer FHIRVersion,
) []Candidate {
	key := strings.ToUpper(segmentName) + "." + fieldNumber(fieldKey)
	if g.dbAnchors != nil {
		if anchors, ok := g.dbAnchors[key]; ok && len(anchors) > 0 {
			return anchors
		}
	}
	return Match(segmentName, fieldKey, fieldName, hl7DataType, fhirResource, fhirSchema, fhirVer)
}

// GenerateSegment produces a position map for one segment → resource pairing.
//
//	hl7Version   e.g. "2.5"
//	messageType  e.g. "ADT_A01"  (used to load the schema file)
//	segmentName  e.g. "PID"
//	fhirResource e.g. "Patient"  (empty → looked up from segmentToResource)
//	fhirVer      e.g. FHIRVersionR4
func (g *Generator) GenerateSegment(
	hl7Version, messageType, segmentName, fhirResource string,
	fhirVer FHIRVersion,
) (*SegmentMap, error) {
	segmentName = strings.ToUpper(strings.TrimSpace(segmentName))

	// Resolve target resource
	if fhirResource == "" {
		var ok bool
		fhirResource, ok = segmentToResource[segmentName]
		if !ok {
			return nil, fmt.Errorf("no default FHIR resource for segment %s; pass fhirResource explicitly", segmentName)
		}
	}

	// Load HL7 schema — derive message type + trigger from e.g. "ADT_A01"
	msgType, triggerEvent := splitMessageType(messageType)
	hl7Schema, err := g.loadHL7Schema(hl7Version, msgType, triggerEvent)
	if err != nil {
		return nil, fmt.Errorf("load HL7 schema %s v%s: %w", messageType, hl7Version, err)
	}

	segDef, ok := hl7Schema.Segments[segmentName]
	if !ok {
		return nil, fmt.Errorf("segment %s not found in %s schema", segmentName, messageType)
	}

	// Load FHIR schema (nil is safe — matcher falls back to anchors only)
	fhirSchema := g.loadFHIRSchema(fhirResource, string(fhirVer))

	result := &SegmentMap{
		Segment:  segmentName,
		Resource: fhirResource,
		Optional: segDef.Usage != "R",
	}

	// Process fields in order
	fields := sortedFields(segDef)
	for _, fieldKey := range fields {
		fieldDef := segDef.Fields[fieldKey]
		if fieldDef.Usage == "X" { // withdrawn / not supported
			continue
		}

		// Skip fields that carry no mappable FHIR content: Set IDs, action codes,
		// dispatch selectors, unit modifiers, scheduling contact persons, and
		// per-resource status codes. See classifyField for the full taxonomy.
		if classifyField(segmentName, fieldNumber(fieldKey), fieldDef.Name) != FieldCategoryMappable {
			continue
		}

		candidates := g.matchField(
			segmentName,
			fieldKey,
			fieldDef.Name,
			fieldDef.DataType,
			fhirResource,
			fhirSchema,
			fhirVer,
		)
		if len(candidates) == 0 {
			continue
		}

		best := candidates[0]
		typeEntry := LookupForVersion(fieldDef.DataType, fhirVer)

		// Anchors are verified spec-level; heuristic passes get a minimum floor.
		minConf := 0.70
		if best.Source == "anchor" {
			minConf = 0.0 // anchors carry their own calibrated value
		}

		gm := GeneratedMapping{
			HL7Path:      fieldKey,
			FHIRPath:     best.FHIRPath,
			HL7DataType:  fieldDef.DataType,
			FHIRDataType: typeEntry.FHIRType,
			Transform:    fhirAwareTransformKey(typeEntry.TransformKey, fieldDef.DataType, best.FHIRPath),
			Required:     fieldDef.Usage == "R",
			Confidence:   math.Max(best.Confidence, minConf),
			Notes:        buildNotes(fieldDef.Name, fieldDef.DataType, typeEntry.Notes),
		}

		// Component-level mappings for composite fields (confidence ≥ 0.80)
		if typeEntry.ComponentSeparator && best.Confidence >= 0.80 && len(fieldDef.Components) > 0 {
			componentMaps := g.generateComponents(fieldKey, fieldDef, best.FHIRPath, fhirVer)
			if len(componentMaps) > 0 {
				result.Mappings = append(result.Mappings, componentMaps...)
				continue // components replace the parent mapping
			}
		}

		result.Mappings = append(result.Mappings, gm)
	}

	return result, nil
}

// GenerateMessage produces a full position map for every segment in a message.
// Segments not in segmentToResource (and without an explicit override) are
// skipped; the caller receives warnings via the returned []string.
func (g *Generator) GenerateMessage(
	hl7Version, messageType string,
	fhirVer FHIRVersion,
) (*MessageMap, []string, error) {
	msgType, triggerEvent := splitMessageType(messageType)
	hl7Schema, err := g.loadHL7Schema(hl7Version, msgType, triggerEvent)
	if err != nil {
		return nil, nil, fmt.Errorf("load HL7 schema %s v%s: %w", messageType, hl7Version, err)
	}

	result := &MessageMap{
		MessageType: messageType,
		HL7Version:  hl7Version,
		FHIRVersion: fhirVer,
	}
	var warnings []string

	// Process segments in message order
	for _, segName := range hl7Schema.SegmentOrder {
		resource, ok := segmentToResource[segName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("segment %s has no default FHIR resource — skipped", segName))
			continue
		}

		segMap, err := g.GenerateSegment(hl7Version, messageType, segName, resource, fhirVer)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("segment %s: %v", segName, err))
			continue
		}
		if len(segMap.Mappings) > 0 {
			result.Segments = append(result.Segments, *segMap)
		}
	}

	return result, warnings, nil
}

// GenerateForPresentSegments is the AI-suggest / wizard entry point.
// It accepts the raw HL7 message text, extracts which segments are present,
// and returns only mappings relevant to those segments — no PID suggestions
// for an MFN, no STF suggestions for an ADT.
func (g *Generator) GenerateForPresentSegments(
	rawMessage, hl7Version string,
	fhirVer FHIRVersion,
) ([]GeneratedMapping, []string, error) {
	present := extractPresentSegmentsFromText(rawMessage)
	if len(present) == 0 {
		return nil, nil, fmt.Errorf("no HL7 segments detected in message")
	}

	// Derive message type from MSH.9 in the raw message
	messageType := extractMessageTypeFromRaw(rawMessage)
	if messageType == "" {
		messageType = "ADT_A01" // safe fallback for schema file lookup
	}

	var all []GeneratedMapping
	var warnings []string

	for seg := range present {
		resource, ok := segmentToResource[seg]
		if !ok {
			continue
		}

		segMap, err := g.GenerateSegment(hl7Version, messageType, seg, resource, fhirVer)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", seg, err))
			continue
		}
		all = append(all, segMap.Mappings...)
	}

	// Sort by HL7 path for deterministic output
	sort.Slice(all, func(i, j int) bool {
		return all[i].HL7Path < all[j].HL7Path
	})

	return all, warnings, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func (g *Generator) loadHL7Schema(version, msgType, triggerEvent string) (*hl7.RealHL7Schema, error) {
	if g.hl7Loader == nil {
		return nil, fmt.Errorf("HL7 schema loader not initialised")
	}
	return g.hl7Loader.LoadRealSchema(version, msgType, triggerEvent)
}

func (g *Generator) loadFHIRSchema(resourceType, version string) *fhir.FHIRSchema {
	if g.fhirLoader == nil {
		return nil
	}
	schema, err := g.fhirLoader.LoadFHIRSchema(resourceType, "", version)
	if err != nil {
		return nil
	}
	return schema
}

// generateComponents maps individual components of a composite HL7 field to
// child FHIR element paths (e.g. XPN components → HumanName sub-elements).
func (g *Generator) generateComponents(
	fieldKey string,
	fieldDef hl7.RealFieldDef,
	parentFHIRPath string,
	fhirVer FHIRVersion,
) []GeneratedMapping {
	if len(fieldDef.Components) == 0 {
		return nil
	}

	var result []GeneratedMapping
	anchorTable := anchorsForVersion(fhirVer)
	for compKey, compDef := range fieldDef.Components {
		subPath := fieldKey + "." + componentPosition(compKey)
		anchor := anchorTable[strings.ToUpper(subPath)]
		if len(anchor) == 0 {
			continue // only emit component mappings we have anchors for
		}

		best := anchor[0]
		typeEntry := LookupForVersion(compDef.DataType, fhirVer)

		result = append(result, GeneratedMapping{
			HL7Path:      subPath,
			FHIRPath:     best.FHIRPath,
			HL7DataType:  compDef.DataType,
			FHIRDataType: typeEntry.FHIRType,
			Transform:    fhirAwareTransformKey(typeEntry.TransformKey, compDef.DataType, best.FHIRPath),
			Required:     compDef.Usage == "R",
			Confidence:   best.Confidence,
			Notes:        compDef.Name,
		})
	}
	return result
}

// splitMessageType converts "ADT_A01" or "ADT^A01" to ("ADT", "A01").
func splitMessageType(mt string) (msgType, triggerEvent string) {
	mt = strings.ReplaceAll(mt, "^", "_")
	parts := strings.SplitN(mt, "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return mt, ""
}

// componentPosition extracts position number from a component key like "PID.5.1" → "1".
func componentPosition(compKey string) string {
	parts := strings.Split(compKey, ".")
	if len(parts) >= 3 {
		return parts[2]
	}
	return compKey
}

// sortedFields returns field keys in ascending field-number order.
func sortedFields(seg hl7.RealSegmentDef) []string {
	keys := make([]string, 0, len(seg.Fields))
	for k := range seg.Fields {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fieldOrder(keys[i]) < fieldOrder(keys[j])
	})
	return keys
}

// fieldOrder returns the numeric position of a field key (e.g. "PID.5" → 5).
func fieldOrder(key string) int {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return 9999
	}
	n := 0
	for _, c := range parts[1] {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

// extractPresentSegmentsFromText returns the set of segment names in a raw
// HL7 message. Handles \r, \r\n, and \n line endings.
func extractPresentSegmentsFromText(msg string) map[string]bool {
	present := make(map[string]bool)
	norm := strings.ReplaceAll(strings.ReplaceAll(msg, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(norm, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		seg := line
		if idx := strings.Index(line, "|"); idx > 0 {
			seg = line[:idx]
		}
		seg = strings.TrimSpace(seg)
		if len(seg) >= 2 && len(seg) <= 4 {
			present[seg] = true
		}
	}
	return present
}

// extractMessageTypeFromRaw reads MSH.9 from a raw HL7 message and returns
// e.g. "ADT_A01". Returns "" on any failure.
func extractMessageTypeFromRaw(msg string) string {
	norm := strings.ReplaceAll(strings.ReplaceAll(msg, "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(norm, "\n") {
		if !strings.HasPrefix(line, "MSH|") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 10 {
			return ""
		}
		// MSH.9 is at index 9 (MSH field separator is fields[1])
		msgTypeField := fields[9]
		parts := strings.Split(msgTypeField, "^")
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "_" + parts[1]
		}
		if parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

// buildNotes combines field and type notes into a concise reasoning string
// for the AI suggest UI.
func buildNotes(fieldName, hl7DataType, typeNotes string) string {
	if typeNotes == "" {
		return fmt.Sprintf("%s (%s)", fieldName, hl7DataType)
	}
	return fmt.Sprintf("%s (%s). %s", fieldName, hl7DataType, typeNotes)
}

// fhirAwareTransformKey overrides the base registry transform key when the FHIR
// target type implies a different conversion than the HL7 source type alone.
//
// Rules (ordered by precedence):
//  1. FHIR path ends in "active" + HL7 type is coded (ID/IS) → hl7_active_flag
//  2. FHIR path ends in "birthDate" or "date" (date-only) + transform is datetime → hl7_timestamp_to_fhir_date
//  3. FHIR path ends in ".value" or ".id" + HL7 type is coded element (CE/CWE/CNE) → ce_code_only
//  4. FHIR path contains "meta.tag" and ends in ".code" + HL7 type is CE → ce_code_only
//  5. FHIR path is "response.code" → mfi_response_level
//  8. XCN/XPN type + FHIR path ends in ".display" → xcn_to_reference
//  9. CE/CWE/CNE/XCN/XPN type + FHIR path ends in ".actor.display" → xcn_to_reference
//
// Anything else falls through to the base key.
func fhirAwareTransformKey(baseKey, hl7DataType, fhirPath string) string {
	lowerPath := strings.ToLower(fhirPath)
	upperType := strings.ToUpper(hl7DataType)

	// Rule 1: boolean active flag
	if (strings.HasSuffix(lowerPath, ".active") || strings.HasSuffix(lowerPath, "/active")) &&
		(upperType == "ID" || upperType == "IS") {
		return "hl7_active_flag"
	}

	// Rule 2: date-only targets must not use datetime transform
	if baseKey == "hl7_timestamp_to_fhir_datetime" {
		if strings.HasSuffix(lowerPath, ".birthdate") ||
			strings.HasSuffix(lowerPath, ".deceaseddate") ||
			strings.HasSuffix(lowerPath, ".date") {
			return "hl7_timestamp_to_fhir_date"
		}
	}

	// Rule 3: identifier.value and similar string slots should not receive a CodeableConcept
	if (upperType == "CE" || upperType == "CWE" || upperType == "CNE") && baseKey == "ce_to_codeableconcept" {
		if strings.HasSuffix(lowerPath, ".value") ||
			strings.HasSuffix(lowerPath, "[0].value") ||
			strings.HasSuffix(lowerPath, "[1].value") {
			return "ce_code_only"
		}
	}

	// Rule 4: meta.tag[x].code must be a simple string code
	if (upperType == "CE" || upperType == "CWE" || upperType == "CNE") && baseKey == "ce_to_codeableconcept" {
		if strings.Contains(lowerPath, "meta.tag") && strings.HasSuffix(lowerPath, ".code") {
			return "ce_code_only"
		}
	}

	// Rule 5: MessageHeader.response.code uses MFI response level mapping
	if strings.HasSuffix(lowerPath, "response.code") {
		return "mfi_response_level"
	}

	// Rule 6: Observation.value[x] — runtime dispatch by OBX.2 value type.
	// The engine reads OBX.2 at message-processing time and injects it as
	// "value_type" in TransformationRules before calling transformOBXValueByType.
	if strings.HasSuffix(fhirPath, "value[x]") {
		return "obx_value_by_type"
	}

	// Rule 7: Appointment.status — SCH.25 codes ("Scheduled","Arrived","Cancelled",…)
	// are HL7 v2 scheduling vocabulary; FHIR requires "booked","arrived","cancelled",…
	if strings.HasSuffix(fhirPath, "Appointment.status") || strings.HasSuffix(fhirPath, "appointment.status") {
		return "siu_appointment_status"
	}

	// Rule 8: XCN/XPN composite mapped to any FHIR .display slot must always use
	// xcn_to_reference so the name components are extracted correctly before storage.
	// This guards against stale templates that were generated before this rule existed.
	if (upperType == "XCN" || upperType == "XPN") && strings.HasSuffix(lowerPath, ".display") {
		return "xcn_to_reference"
	}

	// Rule 9: CE/CWE/CNE/XCN/XPN fields mapped to .actor.display must produce a
	// clean display string, not a CodeableConcept. xcn_to_reference handles both
	// XCN-style (id^family^given) and CE-style (code^text) composites identically.
	if strings.HasSuffix(lowerPath, ".actor.display") {
		switch upperType {
		case "CE", "CWE", "CNE", "XCN", "XPN":
			return "xcn_to_reference"
		}
	}

	return baseKey
}
