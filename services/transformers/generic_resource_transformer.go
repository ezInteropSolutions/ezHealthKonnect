// services/transformers/generic_resource_transformer.go
// UPDATED VERSION - Works with simplified schema adapter

package transformers

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/services/datatypes"
	"ezhealthkonnect/services/mappers"
	"ezhealthkonnect/services/schema"
)

// =====================================
// FIELD MAPPING STRUCTURE
// =====================================

type FieldMapping struct {
	SegmentName         string
	HL7Field            string
	HL7Component        string
	FHIRResourceType    string
	FHIRElementPath     string
	DataTypeTransform   string
	IsRequired          bool
	TransformationRules map[string]interface{}
}

// =====================================
// GENERIC RESOURCE TRANSFORMER
// =====================================

type GenericResourceTransformer struct {
	schemaLoader  *schema.FHIRSchemaAdapter
	dataTypeUtils *datatypes.FHIRDataTypeUtils
	valueMapper   *mappers.ValueMapper
}

func NewGenericResourceTransformer(schemaLoader *schema.FHIRSchemaAdapter, db *sql.DB) *GenericResourceTransformer {
	return &GenericResourceTransformer{
		schemaLoader:  schemaLoader,
		dataTypeUtils: datatypes.NewFHIRDataTypeUtils(),
		valueMapper:   mappers.NewValueMapper(db),
	}
}

// =====================================
// MAIN TRANSFORMATION METHOD
// =====================================

func (t *GenericResourceTransformer) CreateResource(
	resourceType string,
	hl7Data map[string]interface{},
	mappings []FieldMapping,
) (map[string]interface{}, []string, []string) {

	var warnings []string
	var errors []string

	log.Printf("🔧 Creating %s from HL7 data", resourceType)

	// Create base resource
	resource := t.createBaseResource(resourceType)

	// Apply field mappings
	applied := t.applyFieldMappings(resource, hl7Data, mappings, &warnings, &errors)
	if !applied {
		errors = append(errors, fmt.Sprintf("No HL7 data successfully mapped to %s", resourceType))
		return nil, warnings, errors
	}

	// Add narrative text generically
	t.addGenericNarrativeText(resource, resourceType)

	// Basic validation using simplified schema adapter
	if t.schemaLoader != nil {
		validationIssues := t.schemaLoader.ValidateResource(resource)
		for _, issue := range validationIssues {
			if issue.Severity == "error" {
				errors = append(errors, issue.Message)
			} else {
				warnings = append(warnings, issue.Message)
			}
		}
	}

	log.Printf("✅ %s created with %d warnings, %d errors", resourceType, len(warnings), len(errors))
	return resource, warnings, errors
}

// =====================================
// RESOURCE CREATION
// =====================================

func (t *GenericResourceTransformer) createBaseResource(resourceType string) map[string]interface{} {
	resource := map[string]interface{}{
		"resourceType": resourceType,
		"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().UnixNano()),
	}

	// Initialize required fields based on resource type
	switch resourceType {
	case "MessageHeader":
		resource["source"] = map[string]interface{}{
			"name":     "HL7 Interface System",
			"endpoint": "urn:oid:1.3.6.1.4.1.21367.2005.3.7",
		}
		resource["destination"] = []map[string]interface{}{
			{
				"name":     "FHIR Server",
				"endpoint": "http://localhost:8080/fhir",
			},
		}
	case "Patient":
		resource["active"] = true
	case "Encounter":
		resource["status"] = "in-progress"
		resource["class"] = map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "AMB",
			"display": "ambulatory",
		}
	}

	return resource
}

// =====================================
// FIELD MAPPING APPLICATION
// =====================================

func (t *GenericResourceTransformer) applyFieldMappings(
	resource map[string]interface{},
	hl7Data map[string]interface{},
	mappings []FieldMapping,
	warnings, errors *[]string,
) bool {
	mappingsApplied := 0

	for _, mapping := range mappings {
		success := t.applyFieldMapping(resource, hl7Data, mapping, warnings, errors)
		if success {
			mappingsApplied++
		}
	}

	return mappingsApplied > 0
}

func (t *GenericResourceTransformer) applyFieldMapping(
	resource map[string]interface{},
	hl7Data map[string]interface{},
	mapping FieldMapping,
	warnings, errors *[]string,
) bool {
	// Extract HL7 value
	hl7Value, found := t.extractHL7Value(hl7Data, mapping)
	if !found {
		if mapping.IsRequired {
			*warnings = append(*warnings,
				fmt.Sprintf("Required field %s.%s not found in HL7 data",
					mapping.SegmentName, mapping.HL7Field))
		}
		return false
	}

	if strings.TrimSpace(hl7Value) == "" {
		return false
	}

	// Transform using data type utilities
	transformedValue, err := t.transformValue(hl7Value, mapping, hl7Data)
	if err != nil {
		*warnings = append(*warnings,
			fmt.Sprintf("Failed to transform %s.%s: %v",
				mapping.SegmentName, mapping.HL7Field, err))
		return false
	}

	// Set value with proper FHIR structure handling
	if err := t.setFHIRValue(resource, mapping.FHIRElementPath, transformedValue); err != nil {
		*warnings = append(*warnings,
			fmt.Sprintf("Failed to set FHIR value for %s: %v",
				mapping.FHIRElementPath, err))
		return false
	}

	log.Printf("✅ Mapped %s.%s → %s.%s",
		mapping.SegmentName, mapping.HL7Field,
		resource["resourceType"], mapping.FHIRElementPath)
	return true
}

// =====================================
// VALUE TRANSFORMATION
// =====================================

func (t *GenericResourceTransformer) transformValue(
	hl7Value string,
	mapping FieldMapping,
	hl7Data map[string]interface{},
) (interface{}, error) {
	switch mapping.DataTypeTransform {
	case "xpn_to_humanname":
		return t.dataTypeUtils.TransformXPNToHumanName(hl7Value, mapping.TransformationRules), nil
	case "xtn_to_contactpoint":
		return t.dataTypeUtils.TransformXTNToContactPoint(hl7Value, mapping.TransformationRules), nil
	case "cx_to_identifier":
		return t.dataTypeUtils.TransformCXToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "xad_to_address":
		return t.dataTypeUtils.TransformXADToAddress(hl7Value, mapping.TransformationRules), nil
	case "ts_to_date":
		return t.dataTypeUtils.TransformTSToDate(hl7Value), nil
	case "ts_to_datetime":
		return t.dataTypeUtils.TransformTSToDateTime(hl7Value), nil
	case "ce_to_codeableconcept":
		return t.dataTypeUtils.TransformCEToCodeableConcept(hl7Value, mapping.TransformationRules), nil
	case "msh9_to_coding":
		return t.transformMSH9ToCoding(hl7Data), nil
	case "gender_mapping":
		return t.valueMapper.MapGender(hl7Value), nil
	case "pv1_2_to_class":
		return t.valueMapper.MapPatientClass(hl7Value), nil
	default:
		return hl7Value, nil
	}
}

func (t *GenericResourceTransformer) transformMSH9ToCoding(hl7Data map[string]interface{}) map[string]interface{} {
	messageType := t.extractMSH9Component(hl7Data, "MSH.9.1")
	triggerEvent := t.extractMSH9Component(hl7Data, "MSH.9.2")

	coding := map[string]interface{}{
		"system":  "http://terminology.hl7.org/CodeSystem/v2-0003",
		"code":    messageType,
		"display": messageType,
	}

	if triggerEvent != "" {
		coding["extension"] = []map[string]interface{}{
			{
				"url":       "http://hl7.org/fhir/StructureDefinition/event-trigger",
				"valueCode": triggerEvent,
			},
		}
		coding["display"] = fmt.Sprintf("%s %s", messageType, triggerEvent)
	}

	return coding
}

// =====================================
// NARRATIVE GENERATION
// =====================================

func (t *GenericResourceTransformer) addGenericNarrativeText(
	resource map[string]interface{},
	resourceType string,
) {
	var content string

	switch resourceType {
	case "MessageHeader":
		content = t.generateMessageHeaderNarrative(resource)
	case "Patient":
		content = t.generatePatientNarrative(resource)
	case "Encounter":
		content = t.generateEncounterNarrative(resource)
	default:
		content = fmt.Sprintf("<p><b>%s Resource</b></p>", resourceType)
	}

	narrative := fmt.Sprintf(
		"<div xmlns=\"http://www.w3.org/1999/xhtml\">%s</div>",
		content,
	)

	resource["text"] = map[string]interface{}{
		"status": "generated",
		"div":    narrative,
	}
}

func (t *GenericResourceTransformer) generateMessageHeaderNarrative(resource map[string]interface{}) string {
	eventInfo := "HL7 v2 message"
	if eventCoding, ok := resource["eventCoding"].(map[string]interface{}); ok {
		if display, ok := eventCoding["display"].(string); ok {
			eventInfo = display
		}
	}
	return fmt.Sprintf("<p><b>Message Header</b></p><p>Event: %s</p>", eventInfo)
}

func (t *GenericResourceTransformer) generatePatientNarrative(resource map[string]interface{}) string {
	patientName := "Unknown Patient"
	if nameArray, ok := resource["name"].([]interface{}); ok && len(nameArray) > 0 {
		if name, ok := nameArray[0].(map[string]interface{}); ok {
			if family, ok := name["family"].(string); ok {
				if given, ok := name["given"].([]interface{}); ok && len(given) > 0 {
					if givenName, ok := given[0].(string); ok {
						patientName = fmt.Sprintf("%s %s", givenName, family)
					}
				}
			}
		}
	}
	return fmt.Sprintf("<p><b>Patient: %s</b></p>", patientName)
}

func (t *GenericResourceTransformer) generateEncounterNarrative(resource map[string]interface{}) string {
	status := "unknown"
	if statusValue, ok := resource["status"].(string); ok {
		status = statusValue
	}
	return fmt.Sprintf("<p><b>Encounter</b></p><p>Status: %s</p>", strings.Title(status))
}

// =====================================
// HELPER METHODS (same as before)
// =====================================

func (t *GenericResourceTransformer) extractHL7Value(
	hl7Data map[string]interface{},
	mapping FieldMapping,
) (string, bool) {
	data, ok := hl7Data["data"].(map[string]interface{})
	if !ok {
		return "", false
	}

	enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{})
	if !ok {
		return "", false
	}

	segment, ok := enhancedSegments[mapping.SegmentName].(map[string]interface{})
	if !ok {
		return "", false
	}

	fields, ok := segment["fields"].([]interface{})
	if !ok {
		return "", false
	}

	expectedKey := fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field)
	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, ok := fieldMap["key"].(string)
		if !ok || key != expectedKey {
			continue
		}

		if mapping.HL7Component == "" {
			if value, ok := fieldMap["value"].(string); ok {
				return value, true
			}
		} else {
			return t.extractComponentValue(fieldMap, mapping)
		}
	}

	return "", false
}

func (t *GenericResourceTransformer) extractComponentValue(
	field map[string]interface{},
	mapping FieldMapping,
) (string, bool) {
	if subfields, ok := field["subfields"].([]interface{}); ok {
		expectedKey := fmt.Sprintf("%s.%s.%s",
			mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)

		for _, subfield := range subfields {
			subfieldMap, ok := subfield.(map[string]interface{})
			if !ok {
				continue
			}

			key, ok := subfieldMap["key"].(string)
			if !ok || key != expectedKey {
				continue
			}

			if value, ok := subfieldMap["value"].(string); ok {
				return value, true
			}
		}
	}

	return "", false
}

func (t *GenericResourceTransformer) setFHIRValue(
	resource map[string]interface{},
	elementPath string,
	value interface{},
) error {
	// Handle known FHIR array fields specially
	switch elementPath {
	case "identifier", "name", "telecom", "address":
		return t.addToArrayFieldSmart(resource, elementPath, value)
	default:
		// Single value fields
		resource[elementPath] = value
		return nil
	}
}

// Replace the addToArrayField method in services/transformers/generic_resource_transformer.go

func (t *GenericResourceTransformer) addToArrayFieldSmart(
	resource map[string]interface{},
	fieldName string,
	value interface{},
) error {
	// Initialize array if it doesn't exist
	if _, exists := resource[fieldName]; !exists {
		resource[fieldName] = []interface{}{}
	}

	// Get existing array
	existingArray, ok := resource[fieldName].([]interface{})
	if !ok {
		return fmt.Errorf("field %s is not an array", fieldName)
	}

	// Handle different value types appropriately
	switch v := value.(type) {
	case []interface{}:
		// If transformation returned an array, add each element individually
		for _, item := range v {
			if item != nil {
				existingArray = append(existingArray, item)
			}
		}
	case []map[string]interface{}:
		// Handle array of maps (common for contact points, identifiers)
		for _, item := range v {
			if item != nil {
				existingArray = append(existingArray, item)
			}
		}
	case map[string]interface{}:
		// Single object, add directly
		if v != nil {
			existingArray = append(existingArray, v)
		}
	case nil:
		// Skip nil values
		return nil
	default:
		// Other single values
		existingArray = append(existingArray, v)
	}

	resource[fieldName] = existingArray
	return nil
}

func (t *GenericResourceTransformer) extractMSH9Component(hl7Data map[string]interface{}, componentKey string) string {
	data, ok := hl7Data["data"].(map[string]interface{})
	if !ok {
		return ""
	}

	enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{})
	if !ok {
		return ""
	}

	mshSegment, ok := enhancedSegments["MSH"].(map[string]interface{})
	if !ok {
		return ""
	}

	fields, ok := mshSegment["fields"].([]interface{})
	if !ok {
		return ""
	}

	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, ok := fieldMap["key"].(string)
		if !ok || key != "MSH.9" {
			continue
		}

		if subfields, ok := fieldMap["subfields"].([]interface{}); ok {
			for _, subfield := range subfields {
				subfieldMap, ok := subfield.(map[string]interface{})
				if !ok {
					continue
				}

				subfieldKey, ok := subfieldMap["key"].(string)
				if !ok || subfieldKey != componentKey {
					continue
				}

				if value, ok := subfieldMap["value"].(string); ok {
					return strings.TrimSpace(value)
				}
			}
		}
	}

	return ""
}
