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

// Generator orchestrates schema lookups to produce position maps.
type Generator struct {
	hl7Loader  *hl7.RealSchemaLoader
	fhirLoader *fhir.FHIRSchemaLoader
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

		candidates := Match(
			segmentName,
			fieldKey,
			fieldDef.Name,
			fieldDef.DataType,
			fhirResource,
			fhirSchema,
		)
		if len(candidates) == 0 {
			continue
		}

		best := candidates[0]
		typeEntry := LookupForVersion(fieldDef.DataType, fhirVer)

		gm := GeneratedMapping{
			HL7Path:      fieldKey,
			FHIRPath:     best.FHIRPath,
			HL7DataType:  fieldDef.DataType,
			FHIRDataType: typeEntry.FHIRType,
			Transform:    fhirAwareTransformKey(typeEntry.TransformKey, fieldDef.DataType, best.FHIRPath),
			Required:     fieldDef.Usage == "R",
			Confidence:   best.Confidence,
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
	for compKey, compDef := range fieldDef.Components {
		subPath := fieldKey + "." + componentPosition(compKey)
		anchor := knownAnchors[strings.ToUpper(subPath)]
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

	return baseKey
}
