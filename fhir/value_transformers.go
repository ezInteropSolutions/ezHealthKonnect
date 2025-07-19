// fhir/value_transformers.go
// Schema-aware value transformation functions for FHIR
package fhir

import (
	"fmt"
	"strings"
)

// =====================================
// HL7 VALUE EXTRACTION
// =====================================

func (te *TransformationEngine) extractHL7Value(hl7Message map[string]interface{}, segment, field, component string) (interface{}, bool) {
	// Navigate to enhanced segments
	enhancedSegments, ok := hl7Message["enhancedSegments"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	// Find the segment
	segmentData, ok := enhancedSegments[segment].(map[string]interface{})
	if !ok {
		return nil, false
	}

	// Get fields
	fields, ok := segmentData["fields"].([]interface{})
	if !ok {
		return nil, false
	}

	// Find the field by number
	for _, fieldData := range fields {
		fieldMap, ok := fieldData.(map[string]interface{})
		if !ok {
			continue
		}

		key, ok := fieldMap["key"].(string)
		if !ok || !strings.HasSuffix(key, "."+field) {
			continue
		}

		// If no component specified, return the whole value
		if component == "" {
			if value, exists := fieldMap["value"]; exists {
				return value, true
			}
		} else {
			// Extract specific component
			if subfields, ok := fieldMap["subfields"].([]interface{}); ok {
				for _, subfieldData := range subfields {
					subfieldMap, ok := subfieldData.(map[string]interface{})
					if !ok {
						continue
					}

					subfieldKey, ok := subfieldMap["key"].(string)
					if !ok {
						continue
					}

					if strings.HasSuffix(subfieldKey, "."+component) {
						if value, exists := subfieldMap["value"]; exists {
							return value, true
						}
					}
				}
			}
		}
	}

	return nil, false
}

// =====================================
// VALUE TRANSFORMATION WITH SCHEMA AWARENESS
// =====================================

func (te *TransformationEngine) transformValue(hl7Value interface{}, transformLogic map[string]interface{}, schema *FHIRSchema) (interface{}, error) {
	transformType, ok := transformLogic["type"].(string)
	if !ok {
		return hl7Value, nil // Pass through if no transform type
	}

	hl7String := fmt.Sprintf("%v", hl7Value)
	if hl7String == "" || hl7String == "<nil>" {
		return nil, nil
	}

	switch transformType {
	case "identifier":
		return te.transformToFHIRIdentifier(hl7String, transformLogic), nil

	case "humanName":
		return te.transformToFHIRHumanName(hl7String, transformLogic), nil

	case "address":
		return te.transformToFHIRAddress(hl7String, transformLogic), nil

	case "contactPoint":
		return te.transformToFHIRContactPoint(hl7String, transformLogic), nil

	case "date":
		return te.transformToFHIRDate(hl7String), nil

	case "dateTime":
		return te.transformToFHIRDateTime(hl7String), nil

	case "code":
		return te.transformToFHIRCode(hl7String, transformLogic, schema), nil

	case "coding":
		return te.transformToFHIRCoding(hl7String, transformLogic, schema), nil

	case "codeableConcept":
		return te.transformToFHIRCodeableConcept(hl7String, transformLogic, schema), nil

	case "string":
		return hl7String, nil

	case "boolean":
		return te.transformToBoolean(hl7String), nil

	case "integer":
		return te.transformToInteger(hl7String), nil

	case "decimal":
		return te.transformToDecimal(hl7String), nil

	default:
		return hl7Value, nil
	}
}

// =====================================
// SPECIFIC FHIR DATA TYPE TRANSFORMERS
// =====================================

func (te *TransformationEngine) transformToFHIRIdentifier(hl7Value string, logic map[string]interface{}) map[string]interface{} {
	// HL7 CX format: ID^ID_TYPE^ASSIGNING_AUTHORITY^ID_TYPE_CODE
	components := strings.Split(hl7Value, "^")

	identifier := map[string]interface{}{
		"use": "usual",
	}

	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}

	if len(components) > 3 && components[3] != "" {
		// Map assigning authority to system
		system := te.mapAssigningAuthorityToSystem(components[3])
		identifier["system"] = system
	}

	// Add type coding if available
	if len(components) > 1 && components[1] != "" {
		identifier["type"] = map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
					"code":   components[1],
				},
			},
		}
	}

	return identifier
}

func (te *TransformationEngine) transformToFHIRHumanName(hl7Value string, logic map[string]interface{}) map[string]interface{} {
	// HL7 XPN format: FAMILY^GIVEN^MIDDLE^SUFFIX^PREFIX^DEGREE^NAME_TYPE
	components := strings.Split(hl7Value, "^")

	name := map[string]interface{}{
		"use": "official",
	}

	// Family name
	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}

	// Given names
	var givenNames []string
	if len(components) > 1 && components[1] != "" {
		givenNames = append(givenNames, components[1])
	}
	if len(components) > 2 && components[2] != "" {
		givenNames = append(givenNames, components[2])
	}
	if len(givenNames) > 0 {
		name["given"] = givenNames
	}

	// Prefix
	if len(components) > 4 && components[4] != "" {
		name["prefix"] = []string{components[4]}
	}

	// Suffix
	if len(components) > 3 && components[3] != "" {
		name["suffix"] = []string{components[3]}
	}

	// Generate text representation
	var textParts []string
	if prefix, ok := name["prefix"].([]string); ok && len(prefix) > 0 {
		textParts = append(textParts, strings.Join(prefix, " "))
	}
	if given, ok := name["given"].([]string); ok && len(given) > 0 {
		textParts = append(textParts, strings.Join(given, " "))
	}
	if family, ok := name["family"].(string); ok && family != "" {
		textParts = append(textParts, family)
	}
	if suffix, ok := name["suffix"].([]string); ok && len(suffix) > 0 {
		textParts = append(textParts, strings.Join(suffix, " "))
	}

	if len(textParts) > 0 {
		name["text"] = strings.Join(textParts, " ")
	}

	return name
}

func (te *TransformationEngine) transformToFHIRAddress(hl7Value string, logic map[string]interface{}) map[string]interface{} {
	// HL7 XAD format: STREET^OTHER_DESIGNATION^CITY^STATE^ZIP^COUNTRY^ADDRESS_TYPE
	components := strings.Split(hl7Value, "^")

	address := map[string]interface{}{
		"use": "home",
	}

	// Line components
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

	// City
	if len(components) > 2 && components[2] != "" {
		address["city"] = components[2]
	}

	// State
	if len(components) > 3 && components[3] != "" {
		address["state"] = components[3]
	}

	// Postal code
	if len(components) > 4 && components[4] != "" {
		address["postalCode"] = components[4]
	}

	// Country
	if len(components) > 5 && components[5] != "" {
		address["country"] = components[5]
	}

	// Address type
	if len(components) > 6 && components[6] != "" {
		switch strings.ToUpper(components[6]) {
		case "H", "HOME":
			address["use"] = "home"
		case "W", "WORK":
			address["use"] = "work"
		case "T", "TEMP":
			address["use"] = "temp"
		}
	}

	return address
}

func (te *TransformationEngine) transformToFHIRContactPoint(hl7Value string, logic map[string]interface{}) map[string]interface{} {
	// HL7 XTN format: PHONE_NUMBER^TELECOM_USE^TELECOM_EQUIPMENT_TYPE
	components := strings.Split(hl7Value, "^")

	contactPoint := map[string]interface{}{}

	// Value
	if len(components) > 0 && components[0] != "" {
		contactPoint["value"] = components[0]
	}

	// System from logic or default to phone
	if system, ok := logic["system"].(string); ok {
		contactPoint["system"] = system
	} else {
		contactPoint["system"] = "phone"
	}

	// Use from logic or HL7 telecom use
	if use, ok := logic["use"].(string); ok {
		contactPoint["use"] = use
	} else if len(components) > 1 && components[1] != "" {
		switch strings.ToUpper(components[1]) {
		case "H", "HOME":
			contactPoint["use"] = "home"
		case "W", "WORK":
			contactPoint["use"] = "work"
		case "M", "MOBILE":
			contactPoint["use"] = "mobile"
		default:
			contactPoint["use"] = "home"
		}
	}

	return contactPoint
}

func (te *TransformationEngine) transformToFHIRDate(hl7Value string) string {
	// HL7 format: YYYYMMDD
	if len(hl7Value) >= 8 {
		year := hl7Value[0:4]
		month := hl7Value[4:6]
		day := hl7Value[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return hl7Value
}

func (te *TransformationEngine) transformToFHIRDateTime(hl7Value string) string {
	// HL7 format: YYYYMMDDHHMMSS[.SSSS][+/-ZZZZ]
	if len(hl7Value) >= 14 {
		year := hl7Value[0:4]
		month := hl7Value[4:6]
		day := hl7Value[6:8]
		hour := hl7Value[8:10]
		minute := hl7Value[10:12]
		second := hl7Value[12:14]

		result := fmt.Sprintf("%s-%s-%sT%s:%s:%s", year, month, day, hour, minute, second)

		// Handle timezone if present
		if len(hl7Value) > 14 {
			remainder := hl7Value[14:]
			if strings.Contains(remainder, "+") || strings.Contains(remainder, "-") {
				result += remainder
			} else {
				result += "Z" // Default to UTC
			}
		} else {
			result += "Z"
		}

		return result
	} else if len(hl7Value) >= 8 {
		return te.transformToFHIRDate(hl7Value)
	}
	return hl7Value
}

func (te *TransformationEngine) transformToFHIRCode(hl7Value string, logic map[string]interface{}, schema *FHIRSchema) string {
	// Handle value set mapping if specified
	if valueSet, ok := logic["valueSet"].(string); ok {
		if mappedCode := te.mapValueSetCode(hl7Value, valueSet); mappedCode != "" {
			return mappedCode
		}
	}
	return hl7Value
}

func (te *TransformationEngine) transformToFHIRCoding(hl7Value string, logic map[string]interface{}, schema *FHIRSchema) map[string]interface{} {
	// HL7 CE/CWE format: ID^TEXT^CODING_SYSTEM^ALT_ID^ALT_TEXT^ALT_CODING_SYSTEM
	components := strings.Split(hl7Value, "^")

	coding := map[string]interface{}{}

	if len(components) > 0 && components[0] != "" {
		coding["code"] = components[0]
	}

	if len(components) > 1 && components[1] != "" {
		coding["display"] = components[1]
	}

	if len(components) > 2 && components[2] != "" {
		system := te.mapCodingSystemToFHIR(components[2])
		coding["system"] = system
	}

	return coding
}

func (te *TransformationEngine) transformToFHIRCodeableConcept(hl7Value string, logic map[string]interface{}, schema *FHIRSchema) map[string]interface{} {
	coding := te.transformToFHIRCoding(hl7Value, logic, schema)

	concept := map[string]interface{}{
		"coding": []map[string]interface{}{coding},
	}

	// Add text if display is available
	if display, ok := coding["display"].(string); ok {
		concept["text"] = display
	}

	return concept
}

// =====================================
// UTILITY TRANSFORMERS
// =====================================

func (te *TransformationEngine) transformToBoolean(hl7Value string) bool {
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "Y", "YES", "TRUE", "1", "T":
		return true
	default:
		return false
	}
}

func (te *TransformationEngine) transformToInteger(hl7Value string) interface{} {
	if hl7Value == "" {
		return nil
	}
	// Simple integer conversion - could be enhanced
	return hl7Value
}

func (te *TransformationEngine) transformToDecimal(hl7Value string) interface{} {
	if hl7Value == "" {
		return nil
	}
	// Simple decimal conversion - could be enhanced
	return hl7Value
}

// =====================================
// MAPPING FUNCTIONS
// =====================================

func (te *TransformationEngine) mapAssigningAuthorityToSystem(authority string) string {
	// Common authority mappings
	mapping := map[string]string{
		"SSN":      "http://hl7.org/fhir/sid/us-ssn",
		"NPI":      "http://hl7.org/fhir/sid/us-npi",
		"DL":       "urn:oid:2.16.840.1.113883.4.3.25", // Driver's license
		"PASSPORT": "http://hl7.org/fhir/sid/passport-USA",
	}

	if system, exists := mapping[strings.ToUpper(authority)]; exists {
		return system
	}

	// Default to OID format
	return fmt.Sprintf("urn:oid:%s", authority)
}

func (te *TransformationEngine) mapCodingSystemToFHIR(hl7System string) string {
	mapping := map[string]string{
		"L":      "http://loinc.org",
		"LN":     "http://loinc.org",
		"SNM":    "http://snomed.info/sct",
		"SNOMED": "http://snomed.info/sct",
		"ICD9":   "http://hl7.org/fhir/sid/icd-9-cm",
		"ICD10":  "http://hl7.org/fhir/sid/icd-10-cm",
		"CPT":    "http://www.ama-assn.org/go/cpt",
		"HCPCS":  "https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets",
	}

	upperSystem := strings.ToUpper(hl7System)
	if system, exists := mapping[upperSystem]; exists {
		return system
	}

	return hl7System
}

func (te *TransformationEngine) mapValueSetCode(hl7Code, valueSet string) string {
	// This would query the database for value set mappings
	// For now, return the original code
	return ""
}

// =====================================
// CONDITION EVALUATION
// =====================================

func (te *TransformationEngine) evaluateCondition(condition string, hl7Message map[string]interface{}) bool {
	// Simple condition evaluation - could be enhanced with a proper expression parser
	if strings.Contains(condition, "segment exists") {
		segmentType := strings.Fields(condition)[0]
		enhancedSegments, ok := hl7Message["enhancedSegments"].(map[string]interface{})
		if !ok {
			return false
		}
		_, exists := enhancedSegments[segmentType]
		return exists
	}

	// Default to true for unknown conditions
	return true
}

// =====================================
// FHIR PATH SETTING WITH SCHEMA VALIDATION
// =====================================

func (te *TransformationEngine) setFHIRValue(resource map[string]interface{}, fhirPath string, value interface{}, schema *FHIRSchema) error {
	if value == nil {
		return nil
	}

	// Parse FHIR path (e.g., "Patient.name", "Patient.identifier")
	parts := strings.Split(fhirPath, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid FHIR path: %s", fhirPath)
	}

	fieldName := parts[1]

	// Check if field exists in schema
	if schema != nil {
		elementPath := fmt.Sprintf("%s.%s", parts[0], fieldName)
		if element, exists := schema.Elements[elementPath]; exists {
			// Validate cardinality
			if strings.Contains(element.Cardinality, "*") || strings.Contains(element.Cardinality, "..") {
				// Array field
				if existing, ok := resource[fieldName]; ok {
					if existingArray, ok := existing.([]interface{}); ok {
						resource[fieldName] = append(existingArray, value)
					} else {
						resource[fieldName] = []interface{}{existing, value}
					}
				} else {
					resource[fieldName] = []interface{}{value}
				}
			} else {
				// Single value field
				resource[fieldName] = value
			}
		} else {
			// Field not in schema, but still set it
			resource[fieldName] = value
		}
	} else {
		// No schema available, use heuristics
		arrayFields := map[string]bool{
			"identifier": true, "name": true, "address": true, "telecom": true,
		}

		if arrayFields[fieldName] {
			if existing, ok := resource[fieldName]; ok {
				if existingArray, ok := existing.([]interface{}); ok {
					resource[fieldName] = append(existingArray, value)
				} else {
					resource[fieldName] = []interface{}{existing, value}
				}
			} else {
				resource[fieldName] = []interface{}{value}
			}
		} else {
			resource[fieldName] = value
		}
	}

	return nil
}

// =====================================
// RESOURCE VALIDATION
// =====================================

func (te *TransformationEngine) isResourcePopulated(resource map[string]interface{}) bool {
	count := 0
	for key, value := range resource {
		if key == "resourceType" {
			continue
		}
		if value != nil && value != "" {
			if str, ok := value.(string); ok && str != "" {
				count++
			} else if arr, ok := value.([]interface{}); ok && len(arr) > 0 {
				count++
			} else if obj, ok := value.(map[string]interface{}); ok && len(obj) > 0 {
				count++
			}
		}
	}
	return count > 0
}

func (te *TransformationEngine) ensureRequiredFields(resource map[string]interface{}, schema *FHIRSchema) map[string]interface{} {
	if schema == nil {
		return resource
	}

	for _, requiredPath := range schema.Required {
		parts := strings.Split(requiredPath, ".")
		if len(parts) >= 2 {
			fieldName := parts[1]
			if _, exists := resource[fieldName]; !exists {
				if element, elementExists := schema.Elements[requiredPath]; elementExists {
					resource[fieldName] = te.getDefaultValueForDataType(element.DataType)
				}
			}
		}
	}

	return resource
}
