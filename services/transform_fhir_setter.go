package services

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/fhir"
)

func (s *HL7FHIRTransformServiceV3) setAtomicFieldInResource(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	// Strip the "ResourceType." prefix that template fhirPaths carry
	// (e.g. "Practitioner.name[0].family" → "name[0].family").
	// setNestedFieldFromSchema handles this for simple 2-part paths, but
	// setFieldWithArrayIndices does not — causing values to land under a
	// spurious "Practitioner" key instead of directly on the resource.
	if schema != nil && schema.ResourceType != "" {
		if pfx := schema.ResourceType + "."; strings.HasPrefix(fieldPath, pfx) {
			fieldPath = strings.TrimPrefix(fieldPath, pfx)
		}
	}

	// meta.* paths — base FHIR Resource property present on every resource type
	// but not indexed in resource-specific schema elements.  Write directly into
	// the meta sub-object without schema validation.
	// Array paths like meta.security[0].code must fall through to
	// setFieldWithArrayIndices so they are built as proper arrays.
	if strings.HasPrefix(fieldPath, "meta.") {
		subField := fieldPath[len("meta."):]
		if !strings.Contains(subField, "[") {
			meta, ok := resource["meta"].(map[string]interface{})
			if !ok {
				meta = map[string]interface{}{}
				resource["meta"] = meta
			}
			meta[subField] = value
			return nil
		}
	}

	// Handle paths with array indices (e.g., "name[0].given[1]")
	if strings.Contains(fieldPath, "[") {
		return s.setFieldWithArrayIndices(resource, fieldPath, value, schema)
	}

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

	// Handle deep nesting (3+ levels) by recursively creating objects
	if len(parts) > 2 {
		return s.setDeeplyNestedField(resource, parts, value, schema)
	}

	if len(parts) < 2 {
		// Single field, set directly
		resource[fieldPath] = value
		return nil
	}

	parentField := parts[0]
	childField := parts[1]

	// ✅ FIX: If parentField is the resource type itself, treat this as a direct field
	if parentField == schema.ResourceType {
		// This is a direct field like "Patient.birthDate" or "PractitionerRole.code"
		log.Printf("🔧 Direct resource field: %s.%s -> setting %s directly", parentField, childField, childField)

		// Validate the child field exists in schema
		childPath := fmt.Sprintf("%s.%s", schema.ResourceType, childField)
		elem, exists := schema.Elements[childPath]
		if !exists {
			log.Printf("❌ REJECTED: Field %s not found in %s schema", childPath, schema.ResourceType)
			return fmt.Errorf("field %s not found in FHIR schema for %s", childField, schema.ResourceType)
		}

		// Enforce cardinality: 0..* / 1..* fields must be serialised as JSON arrays.
		// Without this check the field mapper sets them as plain objects when the
		// FHIR path carries no "[0]" index notation (e.g. PractitionerRole.code).
		if strings.Contains(elem.Cardinality, "*") {
			return s.setArrayField(resource, childField, value)
		}
		resource[childField] = value
		return nil
	}

	// Check if parent field exists in schema - STRICT VALIDATION for nested objects
	parentPath := s.normalizeFieldPath(parentField, schema.ResourceType)
	parentElement, parentExists := schema.Elements[parentPath]

	if !parentExists {
		// ✅ Try to find choice type element (e.g., "eventCoding" -> "event[x]")
		choiceElement, choiceExists := s.findChoiceTypeElement(parentField, schema)
		if choiceExists {
			parentElement = choiceElement
			parentExists = true
			log.Printf("✅ Found choice type parent for %s", parentField)
		}
	}

	if !parentExists {
		// ✅ STRICT: Reject nested fields where parent doesn't exist in schema
		log.Printf("❌ REJECTED: Parent field %s not found in %s schema", parentPath, schema.ResourceType)
		return fmt.Errorf("parent field %s not found in FHIR schema for %s", parentField, schema.ResourceType)
	}

	// Check if child field exists in schema too
	// Note: For complex types (arrays of objects), child validation may be relaxed
	childPath := fmt.Sprintf("%s.%s", parentPath, childField)
	_, childExists := schema.Elements[childPath]

	// ✅ FIX: If parent is a complex data type (Coding, HumanName, etc.), allow standard properties
	// Complex types like Coding have properties (code, display, system) that aren't in the resource schema
	if !childExists && s.isComplexDataType(parentElement.DataType) {
		// Allow standard properties for complex types
		if s.isStandardComplexTypeProperty(parentElement.DataType, childField) {
			log.Printf("✅ Allowing standard property %s for complex type %s", childField, parentElement.DataType)
			childExists = true
		}
	}

	// If parent is an array of complex types, don't strict validate children
	// (e.g., name is HumanName[], so name.family may not be directly in schema)
	if !childExists && !strings.Contains(parentElement.Cardinality, "*") {
		// Only reject for non-array parents
		log.Printf("❌ REJECTED: Child field %s not found in %s schema", childPath, schema.ResourceType)
		return fmt.Errorf("child field %s not found in FHIR schema for %s", childPath, schema.ResourceType)
	}

	if !childExists {
		log.Printf("ℹ️  Child field %s not in schema, but parent is array - allowing (complex type)", childPath)
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

// Handle paths with array indices like "name[0].given[1]"
func (s *HL7FHIRTransformServiceV3) setFieldWithArrayIndices(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {
	// Parse path with array indices: "name[0].given[1]" -> segments with indices
	// Use regex to split by dots while preserving array indices
	parts := strings.Split(fieldPath, ".")

	current := resource
	for i := 0; i < len(parts); i++ {
		part := parts[i]

		// Check if this part has an array index like "name[0]" or "given[1]"
		if strings.Contains(part, "[") {
			// Extract field name and index
			openBracket := strings.Index(part, "[")
			closeBracket := strings.Index(part, "]")
			if openBracket == -1 || closeBracket == -1 {
				return fmt.Errorf("invalid array syntax in path: %s", part)
			}

			fieldName := part[:openBracket]
			indexStr := part[openBracket+1 : closeBracket]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return fmt.Errorf("invalid array index: %s", indexStr)
			}

			// Ensure array exists
			if _, exists := current[fieldName]; !exists {
				current[fieldName] = []interface{}{}
			}

			// Get array
			arr, ok := current[fieldName].([]interface{})
			if !ok {
				return fmt.Errorf("field %s is not an array", fieldName)
			}

			// Extend array if needed
			for len(arr) <= index {
				arr = append(arr, map[string]interface{}{})
			}
			current[fieldName] = arr

			// If this is the last part, set the value
			if i == len(parts)-1 {
				arr[index] = value
				current[fieldName] = arr
				log.Printf("✅ Set array field: %s = %v", fieldPath, value)
				return nil
			}

			// Navigate into the array element
			elem, ok := arr[index].(map[string]interface{})
			if !ok {
				// Create a new map at this index
				elem = map[string]interface{}{}
				arr[index] = elem
			}
			current = elem

		} else {
			// Simple field name without array index
			if i == len(parts)-1 {
				// Last part - set the value
				current[part] = value
				log.Printf("✅ Set field: %s = %v", fieldPath, value)
				return nil
			}

			// Navigate deeper
			if _, exists := current[part]; !exists {
				current[part] = map[string]interface{}{}
			}

			next, ok := current[part].(map[string]interface{})
			if !ok {
				return fmt.Errorf("field %s is not an object", part)
			}
			current = next
		}
	}

	return nil
}

// Handle deeply nested fields (3+ levels) like "identifier.type.coding.code"
func (s *HL7FHIRTransformServiceV3) setDeeplyNestedField(
	resource map[string]interface{},
	parts []string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty field path")
	}

	// Navigate/create the nested structure
	current := resource
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		// Check if current level exists
		if _, exists := current[part]; !exists {
			// Create new object at this level
			current[part] = map[string]interface{}{}
		}

		// Try to cast to map for next level
		nextMap, ok := current[part].(map[string]interface{})
		if !ok {
			// Current level is not an object, can't nest further
			log.Printf("⚠️  Cannot nest into %s - not an object", strings.Join(parts[:i+1], "."))
			// Fall back to setting as flat string
			fullPath := strings.Join(parts, ".")
			resource[fullPath] = value
			return nil
		}

		current = nextMap
	}

	// Set the final value
	finalField := parts[len(parts)-1]
	current[finalField] = value
	log.Printf("✅ Set deeply nested field: %s = %v", strings.Join(parts, "."), value)
	return nil
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

func (s *HL7FHIRTransformServiceV3) normalizeFieldPath(fieldPath, resourceType string) string {
	// Fix double resource name issue: "Patient.Patient.identifier" -> "Patient.identifier"
	doubleResourcePrefix := resourceType + "." + resourceType + "."
	if strings.HasPrefix(fieldPath, doubleResourcePrefix) {
		// Remove the double resource prefix and re-add single prefix
		withoutDouble := strings.TrimPrefix(fieldPath, doubleResourcePrefix)
		return fmt.Sprintf("%s.%s", resourceType, withoutDouble)
	}

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

func (s *HL7FHIRTransformServiceV3) validateResourceAgainstSchema(
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	warnings *[]string,
) []string {

	var validationErrors []string

	// Check required fields from schema
	for _, requiredField := range schema.Required {
		// Check if this is a nested field (e.g., "MessageHeader.destination.endpoint")
		parts := strings.Split(requiredField, ".")
		if len(parts) > 2 {
			// This is a nested required field - check if parent exists first
			// e.g., "MessageHeader.destination.endpoint" -> check if "destination" exists
			parentField := parts[1]

			// If parent doesn't exist, skip validation for this nested required field
			if _, parentExists := resource[parentField]; !parentExists {
				continue // Parent is optional and not present, so child requirement doesn't apply
			}

			// Parent exists, now check if the nested field exists
			// For now, we'll skip deep validation of nested structures
			// This prevents false positives for conditional requirements
			continue
		}

		// For top-level required fields, validate normally
		normalizedField := s.getResourceFieldName(requiredField) // Remove "MessageHeader." prefix

		// ✅ Handle choice types like event[x] - check if ANY variant exists
		if strings.HasSuffix(normalizedField, "[x]") {
			// This is a choice type - check for any variant
			baseField := strings.TrimSuffix(normalizedField, "[x]")
			hasAnyVariant := false

			// Check all possible variants of the choice type
			for fieldName := range resource {
				if strings.HasPrefix(fieldName, baseField) {
					hasAnyVariant = true
					log.Printf("✅ Found choice type variant: %s for required field %s", fieldName, requiredField)
					break
				}
			}

			if !hasAnyVariant {
				validationError := fmt.Sprintf("FHIR Validation: Required field %s missing from resource", requiredField)
				validationErrors = append(validationErrors, validationError)
				*warnings = append(*warnings, fmt.Sprintf("Required field %s missing from FHIR resource", requiredField))
			}
		} else {
			// Regular field - check directly
			if _, hasValue := resource[normalizedField]; !hasValue {
				validationError := fmt.Sprintf("FHIR Validation: Required field %s missing from resource", requiredField)
				validationErrors = append(validationErrors, validationError)
				*warnings = append(*warnings, fmt.Sprintf("Required field %s missing from FHIR resource", requiredField))
			}
		}
	}

	// Check for properties that don't exist in schema
	for fieldName := range resource {
		if fieldName == "resourceType" || fieldName == "id" || fieldName == "text" ||
			fieldName == "meta" || fieldName == "extension" || fieldName == "modifierExtension" ||
			fieldName == "contained" || fieldName == "implicitRules" || fieldName == "language" {
			continue // Always-valid FHIR base properties
		}
		// Skip resourceType-named sub-objects (internal mapping engine artefact)
		if fieldName == schema.ResourceType {
			continue
		}

		// Check if field exists in schema
		fieldPath := fmt.Sprintf("%s.%s", schema.ResourceType, fieldName)
		_, exists := schema.Elements[fieldPath]

		// ✅ If not found directly, check if it's a choice type variant (e.g., eventCoding for event[x])
		if !exists {
			choiceElement, choiceExists := s.findChoiceTypeElement(fieldName, schema)
			if choiceExists {
				exists = true
				log.Printf("✅ Validated choice type property: %s (maps to %s)", fieldName, choiceElement.Path)
			}
		}

		if !exists {
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
	for i, part := range parts {
		if i == len(parts)-1 {
			// Leaf: key exists with a non-nil value — regardless of value type.
			// The previous implementation tried to type-assert to map/array and
			// returned false for plain string/bool/number leaves, causing false-
			// positive "Required child field X missing" warnings for fields like
			// source.endpoint or status that ARE present as scalar values.
			val, exists := current[part]
			return exists && val != nil
		}
		// Intermediate: navigate into map or first array element
		if obj, ok := current[part].(map[string]interface{}); ok {
			current = obj
		} else if arr, ok := current[part].([]interface{}); ok && len(arr) > 0 {
			if obj, ok := arr[0].(map[string]interface{}); ok {
				current = obj
			} else {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

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

// isComplexDataType checks if the data type is a FHIR complex type (not a primitive)
func (s *HL7FHIRTransformServiceV3) isComplexDataType(dataType string) bool {
	complexTypes := map[string]bool{
		"Coding":              true,
		"CodeableConcept":     true,
		"HumanName":           true,
		"Address":             true,
		"ContactPoint":        true,
		"Identifier":          true,
		"Reference":           true,
		"Period":              true,
		"Quantity":            true,
		"Range":               true,
		"Ratio":               true,
		"SampledData":         true,
		"Attachment":          true,
		"Annotation":          true,
		"Signature":           true,
		"Timing":              true,
		"Age":                 true,
		"Distance":            true,
		"Duration":            true,
		"Count":               true,
		"Money":               true,
		"ContactDetail":       true,
		"Contributor":         true,
		"DataRequirement":     true,
		"Expression":          true,
		"ParameterDefinition": true,
		"RelatedArtifact":     true,
		"TriggerDefinition":   true,
		"UsageContext":        true,
		"Dosage":              true,
		"Meta":                true,
	}
	return complexTypes[dataType]
}

// isStandardComplexTypeProperty checks if a field is a standard property of a FHIR complex type
func (s *HL7FHIRTransformServiceV3) isStandardComplexTypeProperty(dataType, property string) bool {
	// Define standard properties for each complex type
	standardProperties := map[string]map[string]bool{
		"Coding": {
			"system":       true,
			"version":      true,
			"code":         true,
			"display":      true,
			"userSelected": true,
		},
		"CodeableConcept": {
			"coding": true,
			"text":   true,
		},
		"HumanName": {
			"use":    true,
			"text":   true,
			"family": true,
			"given":  true,
			"prefix": true,
			"suffix": true,
			"period": true,
		},
		"Address": {
			"use":        true,
			"type":       true,
			"text":       true,
			"line":       true,
			"city":       true,
			"district":   true,
			"state":      true,
			"postalCode": true,
			"country":    true,
			"period":     true,
		},
		"ContactPoint": {
			"system": true,
			"value":  true,
			"use":    true,
			"rank":   true,
			"period": true,
		},
		"Identifier": {
			"use":      true,
			"type":     true,
			"system":   true,
			"value":    true,
			"period":   true,
			"assigner": true,
		},
		"Reference": {
			"reference":  true,
			"type":       true,
			"identifier": true,
			"display":    true,
		},
		"Period": {
			"start": true,
			"end":   true,
		},
		"Quantity": {
			"value":      true,
			"comparator": true,
			"unit":       true,
			"system":     true,
			"code":       true,
		},
	}

	if props, exists := standardProperties[dataType]; exists {
		return props[property]
	}
	return false
}

// filterResolvedRequiredFieldErrors removes "FHIR Validation: Required field X.Y missing"
// errors from allErrors when field Y is now present in the corresponding resource.
// This eliminates false positives that arise because validateResourceAgainstSchema runs
// before profile composites and post-processing inject participant, status, etc.
func filterResolvedRequiredFieldErrors(allErrors []string, resources []map[string]interface{}) []string {
	// Build a quick lookup: resourceType → set of top-level keys present
	presentFields := make(map[string]map[string]bool, len(resources))
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if rt == "" {
			continue
		}
		if presentFields[rt] == nil {
			presentFields[rt] = make(map[string]bool)
		}
		for k := range r {
			presentFields[rt][k] = true
		}
	}

	var kept []string
	for _, e := range allErrors {
		const prefix = "FHIR Validation: Required field "
		const suffix = " missing from resource"
		if strings.HasPrefix(e, prefix) && strings.HasSuffix(e, suffix) {
			// Extract "ResourceType.fieldName"
			inner := e[len(prefix) : len(e)-len(suffix)]
			if dotIdx := strings.Index(inner, "."); dotIdx != -1 {
				rt := inner[:dotIdx]
				field := inner[dotIdx+1:]
				if fields, ok := presentFields[rt]; ok && fields[field] {
					continue // field is now present — drop the error
				}
			}
		}
		kept = append(kept, e)
	}
	return kept
}

// filterResolvedRequiredFieldWarnings is the warnings counterpart of filterResolvedRequiredFieldErrors.
func filterResolvedRequiredFieldWarnings(allWarnings []string, resources []map[string]interface{}) []string {
	presentFields := make(map[string]map[string]bool, len(resources))
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if rt == "" {
			continue
		}
		if presentFields[rt] == nil {
			presentFields[rt] = make(map[string]bool)
		}
		for k := range r {
			presentFields[rt][k] = true
		}
	}

	var kept []string
	for _, w := range allWarnings {
		const prefix = "Required field "
		const suffix = " missing from FHIR resource"
		if strings.HasPrefix(w, prefix) && strings.HasSuffix(w, suffix) {
			inner := w[len(prefix) : len(w)-len(suffix)]
			if dotIdx := strings.Index(inner, "."); dotIdx != -1 {
				rt := inner[:dotIdx]
				field := inner[dotIdx+1:]
				if fields, ok := presentFields[rt]; ok && fields[field] {
					continue
				}
			}
		}
		kept = append(kept, w)
	}
	return kept
}
