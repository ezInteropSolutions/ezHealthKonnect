package services

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"

	"ezhealthkonnect/hl7"
)

func (s *HL7FHIRTransformServiceV3) extractHL7ValueAtomic(
	enhancedSegments map[string]interface{},
	mapping FieldMapping,
) (string, bool) {
	log.Printf("🔍 extractHL7ValueAtomic called: segment=%s, field=%s, component=%s",
		mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)

	// Get the segment
	segment, segmentExists := enhancedSegments[mapping.SegmentName].(map[string]interface{})
	if !segmentExists {
		log.Printf("🔍 Segment %s not found in HL7 data", mapping.SegmentName)
		return "", false
	}

	// DEBUG: Log segment keys to see what's available
	segmentKeys := make([]string, 0, len(segment))
	for k := range segment {
		segmentKeys = append(segmentKeys, k)
	}
	log.Printf("🔍 Segment %s has keys: %v", mapping.SegmentName, segmentKeys)

	// Get the fields array (handle both []interface{} and primitive.A from MongoDB)
	var fields []interface{}
	fieldsRaw, fieldsExists := segment["fields"]
	if !fieldsExists {
		log.Printf("🔍 No fields found in segment %s (key missing)", mapping.SegmentName)
		return "", false
	}

	// Try direct []interface{} first
	if fieldsSlice, ok := fieldsRaw.([]interface{}); ok {
		fields = fieldsSlice
	} else {
		// Try primitive.A from MongoDB (import "go.mongodb.org/mongo-driver/bson/primitive")
		// Convert any slice type to []interface{}
		fieldsValue := reflect.ValueOf(fieldsRaw)
		if fieldsValue.Kind() == reflect.Slice {
			fields = make([]interface{}, fieldsValue.Len())
			for i := 0; i < fieldsValue.Len(); i++ {
				fields[i] = fieldsValue.Index(i).Interface()
			}
			log.Printf("✅ Converted fields from %T to []interface{} (%d items)", fieldsRaw, len(fields))
		} else {
			log.Printf("🔍 fields key exists but wrong type: %T (not a slice)", fieldsRaw)
			return "", false
		}
	}

	// Build the expected field key (with component if specified)
	var expectedFieldKey string
	if mapping.HL7Component != "" {
		expectedFieldKey = fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)
	} else {
		expectedFieldKey = fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field)
	}

	log.Printf("🔍 Looking for field key: '%s' (component='%s')", expectedFieldKey, mapping.HL7Component)

	// Find the target field
	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, keyOk := fieldMap["key"].(string)
		if !keyOk {
			continue
		}

		// Handle component mapping (e.g., MSH.9.2)
		if mapping.HL7Component != "" {
			// Look for the component directly in subfields
			if key == fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field) {
				return s.extractComponentFromHL7Field(fieldMap, mapping)
			}
		} else {
			// Simple field match
			if key == expectedFieldKey {
				if value, valueOk := fieldMap["value"].(string); valueOk {
					return value, true
				}
			}
		}
	}

	log.Printf("🔍 Field %s not found in segment %s", expectedFieldKey, mapping.SegmentName)
	return "", false
}

// extractHL7FieldDirect looks up a single field value from enhancedSegments without
// a FieldMapping struct.  fieldKey must be the full qualified key, e.g. "OBX.2".
// Used to inject sibling control fields (OBX.2, OBX.6) into transformation rules.
func (s *HL7FHIRTransformServiceV3) extractHL7FieldDirect(
	enhancedSegments map[string]interface{},
	segName, fieldKey string,
) string {
	seg, ok := enhancedSegments[segName].(map[string]interface{})
	if !ok {
		return ""
	}
	fieldsRaw, ok := seg["fields"]
	if !ok {
		return ""
	}
	var fields []interface{}
	if fs, ok := fieldsRaw.([]interface{}); ok {
		fields = fs
	} else {
		v := reflect.ValueOf(fieldsRaw)
		if v.Kind() == reflect.Slice {
			fields = make([]interface{}, v.Len())
			for i := 0; i < v.Len(); i++ {
				fields[i] = v.Index(i).Interface()
			}
		}
	}
	for _, f := range fields {
		fm, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if k, ok := fm["key"].(string); ok && k == fieldKey {
			if val, ok := fm["value"].(string); ok {
				return val
			}
		}
	}
	return ""
}

func (s *HL7FHIRTransformServiceV3) extractComponentFromHL7Field(
	fieldMap map[string]interface{},
	mapping FieldMapping,
) (string, bool) {

	// Look for the exact component in subfields
	if subfields, subfieldOk := fieldMap["subfields"].([]interface{}); subfieldOk {
		expectedComponentKey := fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)

		log.Printf("🔍 Looking for component key: %s", expectedComponentKey)

		for _, subfield := range subfields {
			subfieldMap, ok := subfield.(map[string]interface{})
			if !ok {
				continue
			}

			subfieldKey, keyOk := subfieldMap["key"].(string)
			if !keyOk {
				continue
			}

			log.Printf("🔍 Found subfield key: %s", subfieldKey)

			if subfieldKey == expectedComponentKey {
				if value, valueOk := subfieldMap["value"].(string); valueOk {
					log.Printf("✅ Extracted component %s: %s", expectedComponentKey, value)
					return value, true
				}
			}
		}
	}

	// Fallback: Manual parsing using the HL7 parser
	if value, valueOk := fieldMap["value"].(string); valueOk && value != "" {
		log.Printf("🔄 Fallback: Manual parsing component %s from value: %s", mapping.HL7Component, value)
		return s.manualParseComponentValue(value, mapping)
	}

	log.Printf("⚠️ Component %s not found in field", mapping.HL7Component)
	return "", false
}

func (s *HL7FHIRTransformServiceV3) manualParseComponentValue(
	fieldValue string,
	mapping FieldMapping,
) (string, bool) {

	// Use the HL7 parser to handle composite fields properly
	enhancedFieldData := hl7.ParseHL7Field(fieldValue)
	if enhancedFieldData != nil && len(enhancedFieldData.Repetitions) > 0 {
		rep := enhancedFieldData.Repetitions[0]

		componentPos, err := strconv.Atoi(mapping.HL7Component)
		if err != nil {
			return "", false
		}

		if componentPos > 0 && componentPos <= len(rep.Components) {
			componentValue := rep.Components[componentPos-1].RawValue
			return strings.TrimSpace(componentValue), true
		}
	}

	// Simple fallback: split by ^ and extract component
	components := strings.Split(fieldValue, "^")
	componentPos, err := strconv.Atoi(mapping.HL7Component)
	if err != nil || componentPos < 1 || componentPos > len(components) {
		return "", false
	}

	return strings.TrimSpace(components[componentPos-1]), true
}

// parseHL7Path parses HL7 path like "PID.3.1" into segment, field, and component
func (s *HL7FHIRTransformServiceV3) parseHL7Path(hl7Path string) (segment, field, component string) {
	parts := strings.Split(hl7Path, ".")
	if len(parts) < 2 {
		return hl7Path, "", ""
	}

	segment = parts[0]
	field = parts[1]

	if len(parts) >= 3 {
		component = parts[2]
	}

	return segment, field, component
}

// extractSegmentGroup retrieves all instances of a segment type from segmentGroups,
// handling both the typed map[string][]hl7.EnhancedSegment path and the JSON-unmarshaled path.
func (s *HL7FHIRTransformServiceV3) extractSegmentGroup(parsedHL7Data map[string]interface{}, segmentName string) []hl7.EnhancedSegment {
	rawGroups, exists := parsedHL7Data["segmentGroups"]
	if !exists {
		return nil
	}

	// Typed path — comes directly from the Go parser (test controller, processing engine)
	if typed, ok := rawGroups.(map[string][]hl7.EnhancedSegment); ok {
		return typed[segmentName]
	}

	// JSON-unmarshaled path — marshal back and decode into typed struct
	jsonBytes, err := json.Marshal(rawGroups)
	if err != nil {
		log.Printf("⚠️ extractSegmentGroup: marshal failed: %v", err)
		return nil
	}
	var converted map[string][]hl7.EnhancedSegment
	if err = json.Unmarshal(jsonBytes, &converted); err != nil {
		log.Printf("⚠️ extractSegmentGroup: unmarshal failed: %v", err)
		return nil
	}
	return converted[segmentName]
}

// enhancedSegmentToMap converts a single hl7.EnhancedSegment occurrence into the
// map[string]interface{} shape extractHL7ValueAtomic expects — a JSON round-trip,
// since EnhancedSegment's json tags already match that shape exactly (the same
// technique extractSegmentGroup/ExtractSegmentGroup use for the whole-map case).
func enhancedSegmentToMap(seg hl7.EnhancedSegment) map[string]interface{} {
	b, err := json.Marshal(seg)
	if err != nil {
		log.Printf("⚠️ enhancedSegmentToMap: marshal failed: %v", err)
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("⚠️ enhancedSegmentToMap: unmarshal failed: %v", err)
		return nil
	}
	return m
}

// segFieldValue returns the value of a named field (e.g. "OBX.3") from an EnhancedSegment.
func segFieldValue(seg hl7.EnhancedSegment, key string) string {
	for _, f := range seg.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// mfeActionFromSegments returns the MFE.1 action code (MAD/MUP/MDL/MDC) from
// enhancedSegments, upper-cased and trimmed. Returns "" when no MFE segment is present.
// Used to distinguish a genuine MDL/MDC deactivation from a false produced by
// transformToBoolean's default-false behaviour on an unrecognised STF.7 value.
func (s *HL7FHIRTransformServiceV3) mfeActionFromSegments(enhancedSegments map[string]interface{}) string {
	mfeSeg, ok := enhancedSegments["MFE"].(map[string]interface{})
	if !ok {
		return ""
	}
	fields, ok := mfeSeg["fields"].([]interface{})
	if !ok {
		return ""
	}
	for _, f := range fields {
		if fm, ok2 := f.(map[string]interface{}); ok2 {
			if key, _ := fm["key"].(string); key == "MFE.1" {
				if val, _ := fm["value"].(string); val != "" {
					return strings.ToUpper(strings.TrimSpace(val))
				}
			}
		}
	}
	return ""
}

// resolveHL7FieldFromParsed resolves an HL7 path like "PID.3" or "SCH.25"
// against the parsed HL7 data map (supporting both segmentGroups and enhancedSegments).
func (s *HL7FHIRTransformServiceV3) resolveHL7FieldFromParsed(
	parsedHL7 map[string]interface{},
	fieldPath string,
) string {
	if fieldPath == "" {
		return ""
	}
	fieldPath = strings.TrimSpace(fieldPath)

	// Split into at most 3 parts: segment, field, component
	parts := strings.SplitN(fieldPath, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	segName := parts[0]
	fieldNum := parts[1]
	componentNum := ""
	if len(parts) == 3 {
		componentNum = parts[2]
	}

	segs := s.extractSegmentGroup(parsedHL7, segName)
	if len(segs) == 0 {
		return ""
	}
	seg := segs[0]

	wantFieldKey := segName + "." + fieldNum
	for _, f := range seg.Fields {
		if f.Key == wantFieldKey {
			if componentNum == "" {
				return strings.TrimSpace(f.Value)
			}
			// 3-part path: find sub-component by key first
			wantSubKey := wantFieldKey + "." + componentNum
			for _, sf := range f.Subfields {
				if sf.Key == wantSubKey {
					return strings.TrimSpace(sf.Value)
				}
			}
			// Fallback: split raw value by "^" and extract by position
			compIdx := 0
			fmt.Sscanf(componentNum, "%d", &compIdx)
			if compIdx > 0 {
				comps := strings.Split(f.Value, "^")
				if compIdx <= len(comps) {
					return strings.TrimSpace(comps[compIdx-1])
				}
			}
			return ""
		}
	}
	// Field index fallback (1-based)
	fieldIdx := 0
	fmt.Sscanf(fieldNum, "%d", &fieldIdx)
	if fieldIdx > 0 && fieldIdx <= len(seg.Fields) {
		f := seg.Fields[fieldIdx-1]
		if componentNum == "" {
			return strings.TrimSpace(f.Value)
		}
		compIdx := 0
		fmt.Sscanf(componentNum, "%d", &compIdx)
		if compIdx > 0 {
			comps := strings.Split(f.Value, "^")
			if compIdx <= len(comps) {
				return strings.TrimSpace(comps[compIdx-1])
			}
		}
	}
	return ""
}

func (s *HL7FHIRTransformServiceV3) extractResourceTypes(mappings []FieldMapping) []string {
	resourceTypeSet := make(map[string]bool)
	var resourceTypes []string

	for _, mapping := range mappings {
		if !resourceTypeSet[mapping.FHIRResourceType] {
			resourceTypes = append(resourceTypes, mapping.FHIRResourceType)
			resourceTypeSet[mapping.FHIRResourceType] = true
		}
	}

	return resourceTypes
}

func (s *HL7FHIRTransformServiceV3) filterMappingsForResource(mappings []FieldMapping, resourceType string) []FieldMapping {
	var filtered []FieldMapping
	for _, mapping := range mappings {
		if mapping.FHIRResourceType == resourceType {
			filtered = append(filtered, mapping)
		}
	}
	return filtered
}

// extractFieldMappingsFromWizardArray converts wizard-saved array format to FieldMapping array
// Wizard format: [{ "sourcePath": "PID.3", "targetPath": "Patient.identifier[0].value", "transformType": "string_direct", "isRequired": true }, ...]
func (s *HL7FHIRTransformServiceV3) extractFieldMappingsFromWizardArray(mappingArray []map[string]interface{}) []FieldMapping {
	var mappings []FieldMapping

	for idx, item := range mappingArray {
		sourcePath, _ := item["sourcePath"].(string)
		targetPath, _ := item["targetPath"].(string)
		transformType, _ := item["transformType"].(string)
		isRequired, _ := item["isRequired"].(bool)

		if sourcePath == "" || targetPath == "" {
			log.Printf("⚠️ V3 Service: Skipping item %d - sourcePath='%s', targetPath='%s'", idx, sourcePath, targetPath)
			continue
		}

		// SI (Sequence ID) guard: HL7 segment field 1 (e.g. OBR.1, OBX.1) is a
		// positional counter and must never overwrite a FHIR resource id.
		{
			srcParts := strings.Split(sourcePath, ".")
			tgtParts := strings.Split(targetPath, ".")
			if len(srcParts) == 2 && srcParts[1] == "1" &&
				len(tgtParts) >= 2 && tgtParts[len(tgtParts)-1] == "id" {
				log.Printf("⏭️  Skipping SI field %s → %s (sequence ID must not be used as FHIR resource id)", sourcePath, targetPath)
				continue
			}
		}

		log.Printf("🔍 V3 Service: Processing item %d - source='%s', target='%s', transform='%s'", idx, sourcePath, targetPath, transformType)

		// Parse sourcePath (e.g., "PID.3.1" -> segment="PID", field="3", component="1")
		sourceParts := strings.Split(sourcePath, ".")
		if len(sourceParts) < 2 {
			continue
		}

		segmentName := sourceParts[0]
		fieldNum := sourceParts[1]
		component := ""
		if len(sourceParts) > 2 {
			component = sourceParts[2]
		}

		// Parse targetPath (e.g., "Patient.identifier[0].value" -> resource="Patient", path="identifier[0].value")
		// KEEP array indices in the path - we'll handle them during field setting
		targetParts := strings.Split(targetPath, ".")
		if len(targetParts) < 2 {
			continue
		}

		resourceType := targetParts[0]
		elementPath := strings.Join(targetParts[1:], ".")

		// Create mapping
		mapping := FieldMapping{
			SegmentName:       segmentName,
			HL7Field:          fieldNum,
			HL7Component:      component,
			FHIRResourceType:  resourceType,
			FHIRElementPath:   elementPath,
			DataTypeTransform: transformType,
			IsRequired:        isRequired,
		}

		mappings = append(mappings, mapping)
	}

	return mappings
}

// extractFieldMappingsFromWizardConfig converts wizard-saved mapping format to FieldMapping array
func (s *HL7FHIRTransformServiceV3) extractFieldMappingsFromWizardConfig(mappingData map[string]interface{}) []FieldMapping {
	var mappings []FieldMapping

	// The wizard saves mappings in format: { "PID.3": { "fhirPath": "Patient.identifier", ... }, ... }
	for hl7Path, mappingValue := range mappingData {
		mappingObj, ok := mappingValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract segment name and field from HL7 path (e.g., "PID.3" -> segment="PID", field="3")
		parts := strings.Split(hl7Path, ".")
		if len(parts) < 2 {
			continue
		}

		segmentName := parts[0]
		fieldNum := parts[1]

		// Extract FHIR path (e.g., "Patient.identifier")
		fhirPath, _ := mappingObj["fhirPath"].(string)
		if fhirPath == "" {
			continue
		}

		// Extract resource type from FHIR path
		fhirParts := strings.Split(fhirPath, ".")
		if len(fhirParts) < 2 {
			continue
		}

		resourceType := fhirParts[0]
		elementPath := strings.Join(fhirParts[1:], ".")

		// Create mapping
		mapping := FieldMapping{
			SegmentName:       segmentName,
			HL7Field:          fieldNum,
			HL7Component:      "", // Can be extended from mappingObj if needed
			FHIRResourceType:  resourceType,
			FHIRElementPath:   elementPath,
			DataTypeTransform: getDataTypeTransform(mappingObj),
			IsRequired:        getIsRequired(mappingObj),
		}

		mappings = append(mappings, mapping)
	}

	return mappings
}

func getDataTypeTransform(mappingObj map[string]interface{}) string {
	if transform, ok := mappingObj["transform"].(string); ok {
		return transform
	}
	return ""
}

func getIsRequired(mappingObj map[string]interface{}) bool {
	if required, ok := mappingObj["required"].(bool); ok {
		return required
	}
	return false
}

// extractContextLinks parses the "context" and per-resource "contextLinks" blocks
// from a parsed template JSON object.  Returns nil when the template is v1.0
// (no "context" block present) — callers fall back to existing hardcoded wiring.
func extractContextLinks(templateData map[string]interface{}) *ContextLinks {
	contextRaw, hasContext := templateData["context"]
	if !hasContext {
		return nil // v1.0 template — existing hardcoded wiring applies
	}
	contextMap, ok := contextRaw.(map[string]interface{})
	if !ok || len(contextMap) == 0 {
		return nil
	}

	cl := &ContextLinks{
		RoleToSegment: make(map[string]string),
		ResourceLinks: make(map[string]map[string]string),
	}

	for role, segRaw := range contextMap {
		if seg, ok := segRaw.(string); ok {
			cl.RoleToSegment[role] = seg
		}
	}

	if resourcesRaw, ok := templateData["resources"]; ok {
		if resources, ok := resourcesRaw.(map[string]interface{}); ok {
			for resourceType, resourceDataRaw := range resources {
				resourceData, ok := resourceDataRaw.(map[string]interface{})
				if !ok {
					continue
				}
				linksRaw, ok := resourceData["contextLinks"]
				if !ok {
					continue
				}
				linksMap, ok := linksRaw.(map[string]interface{})
				if !ok {
					continue
				}
				links := make(map[string]string)
				for field, roleRaw := range linksMap {
					if role, ok := roleRaw.(string); ok {
						links[field] = role
					}
				}
				if len(links) > 0 {
					cl.ResourceLinks[resourceType] = links
				}
			}
		}
	}

	if len(cl.RoleToSegment) == 0 {
		return nil
	}
	return cl
}
