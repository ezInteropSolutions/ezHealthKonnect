// services/hl7_fhir_transform_service_v3.go
// TRUE SCHEMA-DRIVEN HL7 to FHIR transformation service
//
// 🎯 PRINCIPLE: ATOMIC MAPPING-DRIVEN APPROACH
// - Each database mapping = one atomic transformation
// - Handle composite HL7 fields automatically
// - Use schema for validation, mappings for content
// - Global logic works for any resource type
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7" // For composite field parsing
)

// =====================================
// SCHEMA-DRIVEN TRANSFORM SERVICE V3
// =====================================
// Note: Uses FieldMapping struct from hl7_fhir_transform_service.go

type HL7FHIRTransformServiceV3 struct {
	db          *sql.DB
	fhirLoader  *fhir.FHIRSchemaLoader
	schemaReady bool
}

func NewHL7FHIRTransformServiceV3(database *sql.DB) *HL7FHIRTransformServiceV3 {
	service := &HL7FHIRTransformServiceV3{
		db:         database,
		fhirLoader: fhir.GetFHIRSchemaLoader(),
	}

	// Verify FHIR schema loader is available
	if service.fhirLoader == nil {
		log.Printf("❌ ERROR: FHIR schema loader not available")
		service.schemaReady = false
	} else {
		// Test schema loading
		if available, err := service.fhirLoader.ListAvailableSchemas(); err == nil {
			if len(available) > 0 {
				service.schemaReady = true
				log.Printf("✅ Schema-driven service initialized with %d schemas", len(available))
			}
		}
	}

	return service
}

// =====================================
// MAIN TRANSFORMATION METHOD
// =====================================

func (s *HL7FHIRTransformServiceV3) Transform(
	ctx context.Context,
	request *TransformRequest,
) (*TransformResponse, error) {
	if !s.schemaReady {
		return nil, fmt.Errorf("FHIR schemas not loaded - cannot perform schema-driven transformation")
	}

	startTime := time.Now()
	response := s.initializeResponse(request)

	log.Printf("🔄 HL7→FHIR transformation v3: %s [ATOMIC MAPPING-DRIVEN]", response.RequestID)

	// Extract message type
	messageType, err := s.extractMessageType(request.ParsedHL7Data)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to extract message type: %v", err))
		return response, nil
	}
	response.MessageType = messageType

	// Get field mappings
	fieldMappings, err := s.getFieldMappings(ctx, messageType, request.TargetProfile)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get field mappings: %v", err))
		return response, nil
	}

	log.Printf("📋 Loaded %d atomic field mappings for message type %s", len(fieldMappings), messageType)

	// DEBUG: Log sample mappings to see what we have
	for i, mapping := range fieldMappings {
		if i >= 3 {
			break
		} // Show first 3
		log.Printf("🔍 Mapping %d: %s.%s → %s.%s (transform: %s)",
			i+1, mapping.SegmentName, mapping.HL7Field, mapping.FHIRResourceType, mapping.FHIRElementPath, mapping.DataTypeTransform)
	}

	// Determine which FHIR resources to create based on mappings
	resourceTypes := s.extractResourceTypes(fieldMappings)
	log.Printf("🎯 Will create resources: %v", resourceTypes)

	// Extract HL7 segments once for all resources
	enhancedSegments := s.extractEnhancedSegments(request.ParsedHL7Data)
	if enhancedSegments == nil {
		response.Errors = append(response.Errors, "No enhanced segments found in parsed HL7 data")
		return response, nil
	}

	// DEBUG: Show available HL7 segments
	segmentNames := make([]string, 0, len(enhancedSegments))
	for segName := range enhancedSegments {
		segmentNames = append(segmentNames, segName)
	}
	log.Printf("🔍 Available HL7 segments: %v", segmentNames)

	// Transform each resource type using atomic mappings
	transformStartTime := time.Now()
	var allResources []map[string]interface{}
	var allWarnings []string
	var allErrors []string
	stats := MappingStatistics{}

	for _, resourceType := range resourceTypes {
		log.Printf("🔧 Creating %s using atomic mappings", resourceType)

		// Load real FHIR schema for validation
		schema, err := s.loadFHIRSchema(resourceType, request.TargetProfile, request.FHIRVersion)
		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("Failed to load schema for %s: %v", resourceType, err))
			continue
		}

		// Get mappings for this resource type
		resourceMappings := s.filterMappingsForResource(fieldMappings, resourceType)
		log.Printf("📊 Processing %d mappings for %s", len(resourceMappings), resourceType)

		// Create resource using atomic mappings
		resource, warnings, errors, mappedCount := s.createResourceFromAtomicMappings(
			resourceType,
			schema,
			enhancedSegments,
			resourceMappings,
		)

		if resource != nil {
			allResources = append(allResources, resource)
		}

		allWarnings = append(allWarnings, warnings...)
		allErrors = append(allErrors, errors...)

		// Track successfully mapped fields
		stats.TotalFieldsMapped += mappedCount
	}

	// Populate response
	s.populateTransformResponse(
		response,
		allResources,
		stats,
		allWarnings,
		allErrors,
		transformStartTime,
		startTime,
		request.CreateBundle,
	)

	log.Printf("✅ Atomic mapping transformation completed: %s (%d resources, %s)",
		response.RequestID, len(allResources), response.Performance.TotalTime)

	return response, nil
}

// =====================================
// ATOMIC MAPPING-DRIVEN RESOURCE CREATION
// =====================================

func (s *HL7FHIRTransformServiceV3) createResourceFromAtomicMappings(
	resourceType string,
	schema *fhir.FHIRSchema,
	enhancedSegments map[string]interface{},
	mappings []FieldMapping,
) (map[string]interface{}, []string, []string, int) {

	var warnings []string
	var errors []string

	log.Printf("📋 Creating %s using %d atomic mappings", resourceType, len(mappings))

	// Initialize base resource structure - MINIMAL, SCHEMA-COMPLIANT ONLY
	resource := map[string]interface{}{
		"resourceType": resourceType,
		"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().UnixNano()),
	}

	// Only add text if it exists in schema
	textPath := fmt.Sprintf("%s.text", resourceType)
	if _, exists := schema.Elements[textPath]; exists {
		resource["text"] = map[string]interface{}{
			"status": "generated",
			"div":    fmt.Sprintf("<div xmlns=\"http://www.w3.org/1999/xhtml\">%s resource created from HL7 message</div>", resourceType),
		}
	}

	mappedFieldCount := 0

	// Process each atomic mapping
	for i, mapping := range mappings {
		log.Printf("🔄 Processing mapping %d/%d: %s.%s.%s → %s.%s",
			i+1, len(mappings), mapping.SegmentName, mapping.HL7Field, mapping.HL7Component, resourceType, mapping.FHIRElementPath)

		// Extract HL7 value using atomic extraction
		hl7Value, found := s.extractHL7ValueAtomic(enhancedSegments, mapping)
		if !found {
			if mapping.IsRequired {
				warnings = append(warnings, fmt.Sprintf("Required mapping %s.%s → %s.%s has no HL7 data",
					mapping.SegmentName, mapping.HL7Field, resourceType, mapping.FHIRElementPath))
			}
			log.Printf("⚠️  No HL7 data found for %s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)
			continue
		}

		if hl7Value == "" {
			log.Printf("⚠️  Empty HL7 value for %s.%s", mapping.SegmentName, mapping.HL7Field)
			continue
		}

		log.Printf("✅ Found HL7 value: %s.%s = '%s'", mapping.SegmentName, mapping.HL7Field, hl7Value)

		// Transform value using atomic transformation
		transformedValue, err := s.transformValueAtomic(hl7Value, mapping)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to transform %s.%s: %v",
				mapping.SegmentName, mapping.HL7Field, err))
			continue
		}

		// Set in resource using atomic field setting
		err = s.setAtomicFieldInResource(resource, mapping.FHIRElementPath, transformedValue, schema)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to set %s: %v", mapping.FHIRElementPath, err))
			continue
		}

		mappedFieldCount++
		log.Printf("✅ Successfully mapped %s.%s → %s.%s",
			mapping.SegmentName, mapping.HL7Field, resourceType, mapping.FHIRElementPath)
	}

	// Only return resource if we successfully mapped some fields
	if mappedFieldCount == 0 {
		log.Printf("⚠️ Skipping %s - no HL7 data was successfully mapped", resourceType)
		return nil, warnings, errors, 0
	}

	// ✅ SCHEMA-DRIVEN: Ensure all required fields exist before validation
	s.ensureRequiredFieldsFromSchema(resource, schema, &warnings, &errors)

	// Validate against schema - STRICT validation before returning
	validationErrors := s.validateResourceAgainstSchema(resource, schema, &warnings)
	if len(validationErrors) > 0 {
		// Add validation errors to errors list
		errors = append(errors, validationErrors...)
		log.Printf("❌ %s failed schema validation with %d errors", resourceType, len(validationErrors))
	}

	log.Printf("✅ Created %s with %d mapped fields", resourceType, mappedFieldCount)
	return resource, warnings, errors, mappedFieldCount
}

// =====================================
// ATOMIC HL7 VALUE EXTRACTION
// =====================================

func (s *HL7FHIRTransformServiceV3) extractHL7ValueAtomic(
	enhancedSegments map[string]interface{},
	mapping FieldMapping,
) (string, bool) {

	// Get the segment
	segment, segmentExists := enhancedSegments[mapping.SegmentName].(map[string]interface{})
	if !segmentExists {
		log.Printf("🔍 Segment %s not found in HL7 data", mapping.SegmentName)
		return "", false
	}

	// Get the fields array
	fields, fieldsExists := segment["fields"].([]interface{})
	if !fieldsExists {
		log.Printf("🔍 No fields found in segment %s", mapping.SegmentName)
		return "", false
	}

	// Build the expected field key (with component if specified)
	var expectedFieldKey string
	if mapping.HL7Component != "" {
		expectedFieldKey = fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)
	} else {
		expectedFieldKey = fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field)
	}

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

// =====================================
// ATOMIC VALUE TRANSFORMATION
// =====================================

func (s *HL7FHIRTransformServiceV3) transformValueAtomic(
	hl7Value string,
	mapping FieldMapping,
) (interface{}, error) {

	if hl7Value == "" {
		return nil, nil
	}

	log.Printf("🔧 Transforming '%s' using '%s'", hl7Value, mapping.DataTypeTransform)

	switch mapping.DataTypeTransform {
	case "msh9_trigger_event_to_coding":
		// ✅ EXPLICIT CASE for trigger event transformation
		result := s.transformTriggerEventToCoding(hl7Value)
		log.Printf("✅ Trigger event transform: '%s' → %+v", hl7Value, result)
		return result, nil
	case "xtn_to_contactpoint", "phone_to_contactpoint":
		return s.transformPhoneToContactPointEnhanced(hl7Value, mapping.TransformationRules), nil
	case "gender_mapping", "administrative_sex", "gender":
		return s.transformGender(hl7Value), nil
	case "cx_to_identifier":
		return s.transformCXToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "xpn_to_humanname":
		return s.transformXPNToHumanName(hl7Value, mapping.TransformationRules), nil
	case "xad_to_address":
		return s.transformXADToAddress(hl7Value, mapping.TransformationRules), nil
	case "email_to_contactpoint":
		return s.transformEmailToContactPoint(hl7Value, mapping.TransformationRules), nil
	case "ts_to_datetime", "ts_to_date":
		return s.transformTSToDate(hl7Value), nil
	case "ce_to_codeableconcept":
		return s.transformCEToCodeableConcept(hl7Value, mapping.TransformationRules), nil
	case "ssn_to_identifier":
		return s.transformSSNToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "account_to_identifier":
		return s.transformAccountToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "msh9_to_coding":
		return s.transformMSH9ToCoding(hl7Value), nil
	case "control_id_to_reference":
		return s.transformControlIdToReference(hl7Value), nil
	case "":
		log.Printf("🔍 No transformation specified, using raw value")
		return hl7Value, nil
	default:
		log.Printf("⚠️ Unknown transformation '%s', using raw value", mapping.DataTypeTransform)
		return hl7Value, nil
	}
}

// =====================================
// SCHEMA-DRIVEN ATOMIC FIELD SETTING
// =====================================

func (s *HL7FHIRTransformServiceV3) setAtomicFieldInResource(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	// Handle nested paths using schema structure
	if strings.Contains(fieldPath, ".") {
		return s.setNestedFieldFromSchema(resource, fieldPath, value, schema)
	}

	// ✅ FIX: Allow known FHIR choice type fields without strict validation
	// FHIR choice types like event[x] are defined as eventCoding, eventUri, etc.
	knownChoiceFields := map[string]bool{
		"eventCoding":            true,
		"eventUri":               true,
		"eventCanonical":         true,
		"valueString":            true,
		"valueCode":              true,
		"valueCoding":            true,
		"valueBoolean":           true,
		"valueInteger":           true,
		"valueDecimal":           true,
		"valueDateTime":          true,
		"valueDate":              true,
		"valueTime":              true,
		"effectiveDateTime":      true,
		"effectivePeriod":        true,
		"subjectReference":       true,
		"subjectCodeableConcept": true,
	}

	if knownChoiceFields[fieldPath] {
		log.Printf("✅ Allowing known choice type field: %s", fieldPath)
		resource[fieldPath] = value
		return nil
	}

	// Handle simple fields - STRICT SCHEMA VALIDATION for non-choice types
	normalizedPath := s.normalizeFieldPath(fieldPath, schema.ResourceType)
	element, exists := schema.Elements[normalizedPath]

	if !exists {
		element, exists = schema.Elements[fieldPath]
	}

	// ✅ ENHANCED: Try to find choice type elements if direct lookup fails
	if !exists {
		choiceElement, choiceExists := s.findChoiceTypeElement(fieldPath, schema)
		if choiceExists {
			element = choiceElement
			exists = true
			log.Printf("✅ Found choice type element for %s", fieldPath)
		}
	}

	if !exists {
		// ✅ STRICT: Reject fields not in schema
		log.Printf("❌ REJECTED: Field %s not found in %s schema - invalid property", fieldPath, schema.ResourceType)
		return fmt.Errorf("field %s not found in FHIR schema for %s", fieldPath, schema.ResourceType)
	}

	// Handle cardinality based on schema
	if strings.Contains(element.Cardinality, "*") {
		return s.setArrayField(resource, fieldPath, value)
	} else {
		resource[fieldPath] = value
		return nil
	}
}

// Helper method to find choice type elements (e.g., event[x] for eventCoding)
func (s *HL7FHIRTransformServiceV3) findChoiceTypeElement(
	fieldPath string,
	schema *fhir.FHIRSchema,
) (*fhir.FHIRElement, bool) {

	// Common FHIR choice type patterns
	choicePatterns := map[string]string{
		"eventCoding":            "event[x]",
		"eventUri":               "event[x]",
		"eventCanonical":         "event[x]",
		"valueString":            "value[x]",
		"valueCode":              "value[x]",
		"valueCoding":            "value[x]",
		"valueBoolean":           "value[x]",
		"valueInteger":           "value[x]",
		"valueDecimal":           "value[x]",
		"valueDateTime":          "value[x]",
		"valueDate":              "value[x]",
		"valueTime":              "value[x]",
		"effectiveDateTime":      "effective[x]",
		"effectivePeriod":        "effective[x]",
		"subjectReference":       "subject[x]",
		"subjectCodeableConcept": "subject[x]",
	}

	// Check if this field is a known choice type
	if choicePattern, isChoice := choicePatterns[fieldPath]; isChoice {
		// Try to find the choice pattern in schema
		choicePath := fmt.Sprintf("%s.%s", schema.ResourceType, choicePattern)
		if element, exists := schema.Elements[choicePath]; exists {
			log.Printf("✅ Found choice type: %s → %s", fieldPath, choicePath)
			return element, true
		}

		// Try without resource prefix
		if element, exists := schema.Elements[choicePattern]; exists {
			log.Printf("✅ Found choice type: %s → %s", fieldPath, choicePattern)
			return element, true
		}
	}

	// Fallback: try to infer choice pattern from field name
	if strings.HasSuffix(fieldPath, "Coding") || strings.HasSuffix(fieldPath, "Uri") ||
		strings.HasSuffix(fieldPath, "Canonical") || strings.HasSuffix(fieldPath, "String") ||
		strings.HasSuffix(fieldPath, "Boolean") || strings.HasSuffix(fieldPath, "Integer") ||
		strings.HasSuffix(fieldPath, "DateTime") || strings.HasSuffix(fieldPath, "Date") ||
		strings.HasSuffix(fieldPath, "Reference") || strings.HasSuffix(fieldPath, "CodeableConcept") {

		// Extract base name (e.g., "eventCoding" -> "event")
		baseName := s.extractChoiceBaseName(fieldPath)
		if baseName != "" {
			choicePattern := baseName + "[x]"
			choicePath := fmt.Sprintf("%s.%s", schema.ResourceType, choicePattern)

			if element, exists := schema.Elements[choicePath]; exists {
				log.Printf("✅ Inferred choice type: %s → %s", fieldPath, choicePath)
				return element, true
			}
		}
	}

	return nil, false
}

// Extract base name from choice type field (e.g., "eventCoding" -> "event")
func (s *HL7FHIRTransformServiceV3) extractChoiceBaseName(fieldPath string) string {
	commonSuffixes := []string{
		"Coding", "Uri", "Canonical", "String", "Code", "Boolean",
		"Integer", "Decimal", "DateTime", "Date", "Time", "Period",
		"Reference", "Quantity", "Range", "Ratio", "Attachment",
		"CodeableConcept", "Identifier",
	}

	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(fieldPath, suffix) {
			return strings.TrimSuffix(fieldPath, suffix)
		}
	}

	return ""
}

// Schema-driven nested field setting (e.g., "source.name", "destination.endpoint")
func (s *HL7FHIRTransformServiceV3) setNestedFieldFromSchema(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	parts := strings.Split(fieldPath, ".")
	if len(parts) != 2 {
		// For deeper nesting, could be extended
		resource[fieldPath] = value
		return nil
	}

	parentField := parts[0]
	childField := parts[1]

	// Check if parent field exists in schema - STRICT VALIDATION
	parentPath := s.normalizeFieldPath(parentField, schema.ResourceType)
	parentElement, parentExists := schema.Elements[parentPath]

	if !parentExists {
		// ✅ STRICT: Reject nested fields where parent doesn't exist in schema
		log.Printf("❌ REJECTED: Parent field %s not found in %s schema", parentPath, schema.ResourceType)
		return fmt.Errorf("parent field %s not found in FHIR schema for %s", parentField, schema.ResourceType)
	}

	// Check if child field exists in schema too
	childPath := fmt.Sprintf("%s.%s", parentPath, childField)
	if _, childExists := schema.Elements[childPath]; !childExists {
		log.Printf("❌ REJECTED: Child field %s not found in %s schema", childPath, schema.ResourceType)
		return fmt.Errorf("child field %s not found in FHIR schema for %s", childPath, schema.ResourceType)
	}

	// Create parent object if it doesn't exist
	if _, exists := resource[parentField]; !exists {
		// Check if parent is array type
		if strings.Contains(parentElement.Cardinality, "*") {
			resource[parentField] = []map[string]interface{}{}
		} else {
			resource[parentField] = map[string]interface{}{}
		}
	}

	// Set child value based on parent type
	if strings.Contains(parentElement.Cardinality, "*") {
		// Parent is array - handle array of objects
		return s.setNestedArrayField(resource, parentField, childField, value, schema)
	} else {
		// Parent is single object
		parentObj, ok := resource[parentField].(map[string]interface{})
		if !ok {
			return fmt.Errorf("parent field %s is not an object", parentField)
		}
		parentObj[childField] = value
		log.Printf("✅ Set nested field: %s.%s = %v", parentField, childField, value)
		return nil
	}
}

// Handle nested array fields (e.g., destination.name where destination is array)
func (s *HL7FHIRTransformServiceV3) setNestedArrayField(
	resource map[string]interface{},
	parentField string,
	childField string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	parentArray, ok := resource[parentField].([]map[string]interface{})
	if !ok {
		return fmt.Errorf("parent field %s is not an array of objects", parentField)
	}

	// For now, add to first object in array or create new object
	if len(parentArray) == 0 {
		// Create first object in array
		newObj := map[string]interface{}{
			childField: value,
		}
		resource[parentField] = []map[string]interface{}{newObj}
	} else {
		// Add to existing first object
		parentArray[0][childField] = value
	}

	log.Printf("✅ Set nested array field: %s[0].%s = %v", parentField, childField, value)
	return nil
}

func (s *HL7FHIRTransformServiceV3) setArrayField(
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

// =====================================
// FIELD PATH HELPERS
// =====================================

func (s *HL7FHIRTransformServiceV3) normalizeFieldPath(fieldPath, resourceType string) string {
	if strings.HasPrefix(fieldPath, resourceType+".") {
		return fieldPath // Already normalized
	}
	return fmt.Sprintf("%s.%s", resourceType, fieldPath)
}

func (s *HL7FHIRTransformServiceV3) getResourceFieldName(fullPath string) string {
	// Remove resource type prefix: "Patient.telecom" -> "telecom"
	if parts := strings.SplitN(fullPath, ".", 2); len(parts) == 2 {
		return parts[1]
	}
	return fullPath
}

// =====================================
// SCHEMA VALIDATION
// =====================================

func (s *HL7FHIRTransformServiceV3) validateResourceAgainstSchema(
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	warnings *[]string,
) []string {

	var validationErrors []string

	// Check required fields from schema
	for _, requiredField := range schema.Required {
		normalizedField := s.getResourceFieldName(requiredField) // Remove "MessageHeader." prefix

		if _, hasValue := resource[normalizedField]; !hasValue {
			validationError := fmt.Sprintf("FHIR Validation: Required field %s missing from resource", requiredField)
			validationErrors = append(validationErrors, validationError)
			*warnings = append(*warnings, fmt.Sprintf("Required field %s missing from FHIR resource", requiredField))
		}
	}

	// Check for properties that don't exist in schema
	for fieldName := range resource {
		if fieldName == "resourceType" || fieldName == "id" {
			continue // These are always valid
		}

		// Check if field exists in schema
		fieldPath := fmt.Sprintf("%s.%s", schema.ResourceType, fieldName)
		if _, exists := schema.Elements[fieldPath]; !exists {
			validationError := fmt.Sprintf("FHIR Validation: Property %s does not exist in %s schema", fieldName, schema.ResourceType)
			validationErrors = append(validationErrors, validationError)
		}
	}

	return validationErrors
}

// ✅ SCHEMA-DRIVEN: Simple required field validation - no defaults, no database config
func (s *HL7FHIRTransformServiceV3) ensureRequiredFieldsFromSchema(
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	warnings *[]string,
	errors *[]string,
) {

	for _, requiredFieldPath := range schema.Required {
		if s.isRequiredFieldSatisfied(resource, requiredFieldPath) {
			continue // Field requirement is satisfied
		}

		// Determine if this is a parent or child field
		if strings.Contains(requiredFieldPath, ".") {
			// Child field - only required if parent exists
			s.handleRequiredChildField(resource, requiredFieldPath, warnings)
		} else if strings.Contains(requiredFieldPath, "[x]") {
			// Choice field - check if any choice is present
			s.handleRequiredChoiceField(resource, requiredFieldPath, errors)
		} else {
			// Parent field - absolutely required
			s.handleRequiredParentField(resource, requiredFieldPath, errors)
		}
	}
}

// Check if a required field is satisfied
func (s *HL7FHIRTransformServiceV3) isRequiredFieldSatisfied(resource map[string]interface{}, requiredFieldPath string) bool {
	// Handle choice types like event[x]
	if strings.Contains(requiredFieldPath, "[x]") {
		if strings.Contains(requiredFieldPath, "event[x]") {
			// Check if ANY event field exists
			eventFields := []string{"eventCoding", "eventUri", "eventCanonical"}
			for _, eventField := range eventFields {
				if _, exists := resource[eventField]; exists {
					return true
				}
			}
		}
		return false
	}

	// Handle nested fields like "source.endpoint"
	if strings.Contains(requiredFieldPath, ".") {
		return s.nestedFieldExists(resource, requiredFieldPath)
	}

	// Handle simple fields
	fieldName := s.getResourceFieldName(requiredFieldPath)
	_, exists := resource[fieldName]
	return exists
}

// Handle required parent fields - these MUST exist
func (s *HL7FHIRTransformServiceV3) handleRequiredParentField(
	resource map[string]interface{},
	fieldPath string,
	errors *[]string,
) {
	errorMsg := fmt.Sprintf("CRITICAL: Required parent field %s missing - no HL7 mapping found", fieldPath)
	*errors = append(*errors, errorMsg)
	log.Printf("❌ %s", errorMsg)
}

// Handle required choice fields like event[x] - one choice MUST exist
func (s *HL7FHIRTransformServiceV3) handleRequiredChoiceField(
	resource map[string]interface{},
	fieldPath string,
	errors *[]string,
) {
	if strings.Contains(fieldPath, "event[x]") {
		errorMsg := fmt.Sprintf("CRITICAL: Required field %s missing - need HL7 mapping like MSH.9.2 → eventCoding", fieldPath)
		*errors = append(*errors, errorMsg)
		log.Printf("❌ %s", errorMsg)
	}
}

// Handle required child fields - only required if parent exists
func (s *HL7FHIRTransformServiceV3) handleRequiredChildField(
	resource map[string]interface{},
	fieldPath string,
	warnings *[]string,
) {
	parts := strings.Split(s.getResourceFieldName(fieldPath), ".")
	parentField := parts[0]

	// Check if parent exists
	if _, parentExists := resource[parentField]; !parentExists {
		log.Printf("✅ Child field %s not required - parent %s doesn't exist", fieldPath, parentField)
		return
	}

	// Parent exists, so child is required
	warningMsg := fmt.Sprintf("Required child field %s missing - need HL7 mapping", fieldPath)
	*warnings = append(*warnings, warningMsg)
	log.Printf("⚠️ %s", warningMsg)
}

// Check if nested field exists (reuse existing method)
func (s *HL7FHIRTransformServiceV3) nestedFieldExists(resource map[string]interface{}, fieldPath string) bool {
	parts := strings.Split(s.getResourceFieldName(fieldPath), ".")

	current := resource
	for _, part := range parts {
		if obj, ok := current[part].(map[string]interface{}); ok {
			current = obj
		} else if i, ok := current[part]; ok && i != nil {
			// Handle arrays - check if first element has the nested structure
			if arr, ok := i.([]interface{}); ok && len(arr) > 0 {
				if obj, ok := arr[0].(map[string]interface{}); ok {
					current = obj
				} else {
					return false
				}
			} else {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// =====================================
// SCHEMA LOADING HELPERS
// =====================================

func (s *HL7FHIRTransformServiceV3) loadFHIRSchema(resourceType, profile, version string) (*fhir.FHIRSchema, error) {
	if profile == "" {
		profile = "base"
	}
	if version == "" {
		version = "R4"
	}

	schema, err := s.fhirLoader.LoadFHIRSchema(resourceType, profile, version)
	if err != nil {
		// Try fallback to base profile
		if profile != "base" {
			return s.fhirLoader.LoadFHIRSchema(resourceType, "base", version)
		}
		return nil, err
	}

	return schema, nil
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

// =====================================
// FIELD MAPPING & HL7 EXTRACTION (Reused from V1)
// =====================================

func (s *HL7FHIRTransformServiceV3) getFieldMappings(ctx context.Context, messageType, profile string) ([]FieldMapping, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available for field mappings")
	}

	query := `
		SELECT segment_name, hl7_field, 
		       COALESCE(hl7_component, '') as hl7_component,
		       fhir_resource_type, fhir_element_path, 
		       COALESCE(data_type_transform, '') as data_type_transform,
		       is_required,
		       COALESCE(transformation_rules, '{}') as transformation_rules
		FROM field_element_mappings fem
		WHERE EXISTS (
			SELECT 1 FROM message_fhir_templates mft 
			WHERE mft.message_type = $1 
			AND mft.fhir_resources::jsonb ? fem.fhir_resource_type
		)
		ORDER BY fem.fhir_resource_type, fem.segment_name, fem.hl7_field
	`

	rows, err := s.db.QueryContext(ctx, query, messageType)
	if err != nil {
		return nil, fmt.Errorf("failed to query field mappings: %w", err)
	}
	defer rows.Close()

	var mappings []FieldMapping
	for rows.Next() {
		var mapping FieldMapping
		var component sql.NullString
		var dataTypeTransform sql.NullString
		var rulesJSON sql.NullString

		err := rows.Scan(
			&mapping.SegmentName,
			&mapping.HL7Field,
			&component,
			&mapping.FHIRResourceType,
			&mapping.FHIRElementPath,
			&dataTypeTransform,
			&mapping.IsRequired,
			&rulesJSON,
		)
		if err != nil {
			continue
		}

		if component.Valid {
			mapping.HL7Component = component.String
		}
		if dataTypeTransform.Valid {
			mapping.DataTypeTransform = dataTypeTransform.String
		}

		if rulesJSON.Valid && rulesJSON.String != "" && rulesJSON.String != "{}" {
			var rules map[string]interface{}
			if err := json.Unmarshal([]byte(rulesJSON.String), &rules); err == nil {
				mapping.TransformationRules = rules
			}
		}

		mappings = append(mappings, mapping)
	}

	log.Printf("📊 Found %d field mappings for message type %s", len(mappings), messageType)
	return mappings, nil
}

// =====================================
// UTILITY METHODS (Reused)
// =====================================

func (s *HL7FHIRTransformServiceV3) initializeResponse(request *TransformRequest) *TransformResponse {
	requestID := request.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("transform_v3_%d", time.Now().UnixNano())
	}

	return &TransformResponse{
		Success:        false,
		RequestID:      requestID,
		FHIRResources:  []map[string]interface{}{},
		ResourceCounts: make(map[string]int),
		Warnings:       []string{},
		Errors:         []string{},
		MappingStats:   MappingStatistics{},
		Performance:    PerformanceMetrics{},
	}
}

func (s *HL7FHIRTransformServiceV3) extractMessageType(parsedHL7 map[string]interface{}) (string, error) {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if messageType, exists := data["messageType"].(map[string]interface{}); exists {
			if name, nameOk := messageType["name"].(string); nameOk {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("message type not found in parsed HL7 data")
}

func (s *HL7FHIRTransformServiceV3) extractEnhancedSegments(parsedHL7 map[string]interface{}) map[string]interface{} {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{}); ok {
			return enhancedSegments
		}
	}
	return nil
}

func (s *HL7FHIRTransformServiceV3) populateTransformResponse(
	response *TransformResponse,
	resources []map[string]interface{},
	stats MappingStatistics,
	warnings, errors []string,
	transformStartTime, startTime time.Time,
	createBundle bool,
) {
	response.FHIRResources = resources
	response.MappingStats = stats
	response.Warnings = append(response.Warnings, warnings...)
	response.Errors = append(response.Errors, errors...)
	response.Performance.TransformTime = time.Since(transformStartTime).String()

	for _, resource := range resources {
		if resourceType, ok := resource["resourceType"].(string); ok {
			response.ResourceCounts[resourceType]++
		}
	}

	if createBundle && len(resources) > 0 {
		bundle := s.createBundle(resources, response.RequestID, response.MessageType)
		response.Bundle = bundle
	}

	response.Success = len(response.Errors) == 0
	response.Performance.TotalTime = time.Since(startTime).String()
	response.Performance.ResourcesCreated = len(resources)
}

func (s *HL7FHIRTransformServiceV3) createBundle(resources []map[string]interface{}, requestID, messageType string) map[string]interface{} {
	entries := make([]map[string]interface{}, len(resources))

	for i, resource := range resources {
		resourceID := ""
		if id, exists := resource["id"].(string); exists {
			resourceID = id
		}

		entries[i] = map[string]interface{}{
			"fullUrl":  fmt.Sprintf("urn:uuid:%s", resourceID),
			"resource": resource,
		}
	}

	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           fmt.Sprintf("bundle-%s", requestID),
		"type":         "message",
		"timestamp":    time.Now().Format(time.RFC3339),
		"entry":        entries,
	}
}

// =====================================
// DATA TYPE TRANSFORMERS (Complete Implementation)
// =====================================

func (s *HL7FHIRTransformServiceV3) transformGender(gender string) string {
	upperGender := strings.ToUpper(strings.TrimSpace(gender))

	// ✅ DATABASE-DRIVEN: Look up gender mapping from PostgreSQL
	if s.db != nil {
		query := `SELECT fhir_code FROM value_set_mappings 
				 WHERE mapping_name = 'administrative_sex' AND hl7_value = $1 LIMIT 1`

		var fhirCode string
		if err := s.db.QueryRow(query, upperGender).Scan(&fhirCode); err == nil {
			log.Printf("✅ Gender mapping from DB: %s → %s", gender, fhirCode)
			return fhirCode
		}
	}

	// ✅ NO HARDCODED FALLBACK - return as-is if no mapping found
	log.Printf("⚠️ No gender mapping found for '%s' in database, using original value", gender)
	return strings.ToLower(upperGender)
}

func (s *HL7FHIRTransformServiceV3) transformPhoneToContactPointEnhanced(phone string, rules map[string]interface{}) interface{} {
	if phone == "" {
		return nil
	}

	// Handle composite XTN fields like "(407)939-1289^^^theMainMouse@disney.com"
	if strings.Contains(phone, "^") {
		return s.processXTNField(phone, rules)
	}

	return s.createSimpleContactPoint(phone, rules)
}

func (s *HL7FHIRTransformServiceV3) processXTNField(phone string, rules map[string]interface{}) interface{} {
	var contactPoints []map[string]interface{}

	// Use HL7 parser for proper composite field handling
	enhancedFieldData := hl7.ParseHL7Field(phone)
	if enhancedFieldData == nil || len(enhancedFieldData.Repetitions) == 0 {
		// Fallback: simple split parsing
		components := strings.Split(phone, "^")
		for i, component := range components {
			component = strings.TrimSpace(component)
			if component == "" {
				continue
			}

			contactPoint := s.createContactPointFromComponent(component, i+1, rules)
			if contactPoint != nil {
				contactPoints = append(contactPoints, contactPoint)
			}
		}
	} else {
		// Enhanced parsing
		for _, repetition := range enhancedFieldData.Repetitions {
			for compIndex, component := range repetition.Components {
				value := strings.TrimSpace(component.RawValue)
				if value == "" {
					continue
				}

				contactPoint := s.createContactPointFromComponent(value, compIndex+1, rules)
				if contactPoint != nil {
					contactPoints = append(contactPoints, contactPoint)
				}
			}
		}
	}

	if len(contactPoints) == 1 {
		return contactPoints[0]
	}
	return contactPoints
}

func (s *HL7FHIRTransformServiceV3) createContactPointFromComponent(value string, componentNum int, rules map[string]interface{}) map[string]interface{} {
	var contactPoint map[string]interface{}

	if strings.Contains(value, "@") {
		// Email
		contactPoint = map[string]interface{}{
			"system": "email",
			"use":    "home", // Default, can be overridden by rules
			"value":  value,
		}
	} else {
		// Phone
		contactPoint = map[string]interface{}{
			"system": "phone",
			"use":    "home", // Default, can be overridden by rules or database mapping
			"value":  value,
		}

		// ✅ DATABASE-DRIVEN: Look up phone use mapping based on component position
		if s.db != nil {
			phoneUse := s.lookupPhoneUseFromDatabase(componentNum)
			if phoneUse != "" {
				contactPoint["use"] = phoneUse
			}
		}
	}

	// Apply transformation rules if provided (these come from database)
	if rules != nil {
		if system, ok := rules["system"].(string); ok {
			contactPoint["system"] = system
		}
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

// ✅ DATABASE-DRIVEN: Look up phone use based on HL7 XTN component position
func (s *HL7FHIRTransformServiceV3) lookupPhoneUseFromDatabase(componentNum int) string {
	query := `
		SELECT fhir_code 
		FROM value_set_mappings 
		WHERE mapping_name = 'xtn_component_use' AND hl7_value = $1 
		LIMIT 1
	`

	var use string
	err := s.db.QueryRow(query, fmt.Sprintf("%d", componentNum)).Scan(&use)
	if err != nil {
		log.Printf("🔍 No XTN component use mapping found for component %d", componentNum)
		return ""
	}

	log.Printf("✅ XTN component mapping: component %d → use '%s'", componentNum, use)
	return use
}

func (s *HL7FHIRTransformServiceV3) createSimpleContactPoint(phone string, rules map[string]interface{}) map[string]interface{} {
	contactPoint := map[string]interface{}{
		"system": "phone",
		"use":    "home",
		"value":  phone,
	}

	if rules != nil {
		if system, ok := rules["system"].(string); ok {
			contactPoint["system"] = system
		}
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

func (s *HL7FHIRTransformServiceV3) transformCXToIdentifier(cx string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(cx, "^")
	identifier := map[string]interface{}{
		"use": "usual",
	}

	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformServiceV3) transformXPNToHumanName(xpn string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xpn, "^")
	name := map[string]interface{}{
		"use": "official",
	}

	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}

	var given []string
	if len(components) > 1 && components[1] != "" {
		given = append(given, components[1])
	}
	if len(components) > 2 && components[2] != "" {
		given = append(given, components[2])
	}
	if len(given) > 0 {
		name["given"] = given
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			name["use"] = use
		}
	}

	return name
}

func (s *HL7FHIRTransformServiceV3) transformXADToAddress(xad string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xad, "^")
	address := map[string]interface{}{
		"use": "home",
	}

	var lines []string
	if len(components) > 0 && components[0] != "" {
		lines = append(lines, components[0])
	}
	if len(components) > 1 && components[1] != "" {
		lines = append(lines, components[1])
	}
	if len(lines) > 0 {
		address["line"] = lines
	}

	if len(components) > 2 && components[2] != "" {
		address["city"] = components[2]
	}
	if len(components) > 3 && components[3] != "" {
		address["state"] = components[3]
	}
	if len(components) > 4 && components[4] != "" {
		address["postalCode"] = components[4]
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			address["use"] = use
		}
	}

	return address
}

func (s *HL7FHIRTransformServiceV3) transformEmailToContactPoint(email string, rules map[string]interface{}) map[string]interface{} {
	contactPoint := map[string]interface{}{
		"system": "email",
		"use":    "home",
		"value":  email,
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

func (s *HL7FHIRTransformServiceV3) transformTSToDate(ts string) string {
	if len(ts) >= 8 {
		year := ts[0:4]
		month := ts[4:6]
		day := ts[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return ts
}

func (s *HL7FHIRTransformServiceV3) transformCEToCodeableConcept(ce string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(ce, "^")
	codeableConcept := map[string]interface{}{
		"coding": []map[string]interface{}{},
	}

	if len(components) > 0 && components[0] != "" {
		coding := map[string]interface{}{
			"code": components[0],
		}

		if len(components) > 1 && components[1] != "" {
			coding["display"] = components[1]
		}

		if len(components) > 2 && components[2] != "" {
			coding["system"] = components[2]
		}

		if rules != nil {
			if system, ok := rules["system"].(string); ok {
				coding["system"] = system
			}
		}

		codeableConcept["coding"] = []map[string]interface{}{coding}

		if display, ok := coding["display"].(string); ok {
			codeableConcept["text"] = display
		}
	}

	return codeableConcept
}

func (s *HL7FHIRTransformServiceV3) transformSSNToIdentifier(ssn string, rules map[string]interface{}) map[string]interface{} {
	identifier := map[string]interface{}{
		"use":   "secondary",
		"value": ssn,
		"type": map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":    "SS",
					"display": "Social Security Number",
					"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		},
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformServiceV3) transformAccountToIdentifier(account string, rules map[string]interface{}) map[string]interface{} {
	identifier := map[string]interface{}{
		"use":   "secondary",
		"value": account,
		"type": map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":    "AN",
					"display": "Account Number",
					"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		},
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformServiceV3) transformMSH9ToCoding(msh9 string) map[string]interface{} {
	components := strings.Split(msh9, "^")
	eventCoding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
	}

	if len(components) > 0 && components[0] != "" {
		messageType := components[0]
		triggerEvent := ""

		if len(components) > 1 && components[1] != "" {
			triggerEvent = components[1]
		}

		if triggerEvent != "" {
			eventCoding["code"] = fmt.Sprintf("%s^%s", messageType, triggerEvent)
			eventCoding["display"] = fmt.Sprintf("%s %s", messageType, triggerEvent)
		} else {
			eventCoding["code"] = messageType
			eventCoding["display"] = messageType
		}
	}

	return eventCoding
}

// NEW: Transform just trigger event (MSH.9.2) to simple Coding for MessageHeader.eventCoding
func (s *HL7FHIRTransformServiceV3) transformTriggerEventToCoding(triggerEvent string) map[string]interface{} {
	// triggerEvent should be just "A04", "A01", etc. from MSH.9.2
	triggerEvent = strings.TrimSpace(triggerEvent)

	coding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
		"code":   triggerEvent,
	}

	// ✅ NO HARDCODING: Look up display from database value_set_mappings
	if s.db != nil {
		display := s.lookupDisplayFromDatabase("v2-0003", triggerEvent)
		if display != "" {
			coding["display"] = display
		}
	}

	log.Printf("✅ Created Coding from trigger event: %s → %+v", triggerEvent, coding)
	return coding
}

// ✅ DATABASE-DRIVEN: Look up display names from PostgreSQL value_set_mappings
func (s *HL7FHIRTransformServiceV3) lookupDisplayFromDatabase(codeSystem, code string) string {
	query := `
		SELECT fhir_display 
		FROM value_set_mappings 
		WHERE fhir_system LIKE $1 AND (hl7_value = $2 OR fhir_code = $2)
		ORDER BY mapping_type ASC 
		LIMIT 1
	`

	var display string
	err := s.db.QueryRow(query, "%"+codeSystem+"%", code).Scan(&display)
	if err != nil {
		log.Printf("🔍 No display mapping found for %s#%s in database", codeSystem, code)
		return ""
	}

	log.Printf("✅ Found display mapping: %s#%s → %s", codeSystem, code, display)
	return display
}

// Transform MSH.10 Control ID to proper Reference for MessageHeader.focus
func (s *HL7FHIRTransformServiceV3) transformControlIdToReference(controlId string) map[string]interface{} {
	// MessageHeader.focus expects Reference objects, not primitive identifiers
	// Since we don't have actual Patient IDs at this point, we'll create a logical reference
	controlId = strings.TrimSpace(controlId)

	// Create a proper Reference object (not just identifier string)
	reference := map[string]interface{}{
		"identifier": map[string]interface{}{ // ✅ Identifier object, not primitive
			"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
			"value":  controlId,
		},
		"display": fmt.Sprintf("HL7 Message Control ID: %s", controlId),
	}

	log.Printf("✅ Created Reference from control ID: %s → %+v", controlId, reference)
	return reference
}

// =====================================
// HELPER FUNCTIONS
// =====================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
