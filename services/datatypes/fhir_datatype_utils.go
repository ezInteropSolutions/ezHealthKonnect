// services/datatypes/fhir_datatype_utils.go
// FIXED VERSION - Remove hl7.Component dependency

package datatypes

import (
	"fmt"
	"strings"
)

// =====================================
// FHIR DATA TYPE UTILITIES
// =====================================

type FHIRDataTypeUtils struct{}

func NewFHIRDataTypeUtils() *FHIRDataTypeUtils {
	return &FHIRDataTypeUtils{}
}

// =====================================
// IDENTIFIER TRANSFORMATIONS
// =====================================

// TransformCXToIdentifier converts HL7 v2 CX data type to FHIR Identifier
func (utils *FHIRDataTypeUtils) TransformCXToIdentifier(
	cx string,
	rules map[string]interface{},
) map[string]interface{} {
	components := strings.Split(cx, "^")
	identifier := map[string]interface{}{
		"use": "usual",
	}

	// CX.1 - ID Number
	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}

	// CX.4 - Assigning Authority
	if len(components) > 3 && components[3] != "" {
		identifier["system"] = components[3]
	}

	// CX.5 - Identifier Type Code
	if len(components) > 4 && components[4] != "" {
		identifier["type"] = map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":   components[4],
					"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		}
	}

	// Apply transformation rules
	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
		if system, ok := rules["system"].(string); ok {
			identifier["system"] = system
		}
	}

	return identifier
}

// TransformSSNToIdentifier creates a Social Security Number identifier
func (utils *FHIRDataTypeUtils) TransformSSNToIdentifier(
	ssn string,
	rules map[string]interface{},
) map[string]interface{} {
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

	// Apply rules
	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

// =====================================
// HUMAN NAME TRANSFORMATIONS
// =====================================

// TransformXPNToHumanName converts HL7 v2 XPN data type to FHIR HumanName
func (utils *FHIRDataTypeUtils) TransformXPNToHumanName(
	xpn string,
	rules map[string]interface{},
) map[string]interface{} {
	components := strings.Split(xpn, "^")
	name := map[string]interface{}{
		"use": "official",
	}

	// XPN.1 - Family Name
	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}

	// XPN.2 - Given Name
	var given []string
	if len(components) > 1 && components[1] != "" {
		given = append(given, components[1])
	}

	// XPN.3 - Second and Further Given Names
	if len(components) > 2 && components[2] != "" {
		given = append(given, components[2])
	}

	if len(given) > 0 {
		name["given"] = given
	}

	// XPN.4 - Suffix
	if len(components) > 3 && components[3] != "" {
		if suffix := components[3]; suffix != "" {
			name["suffix"] = []string{suffix}
		}
	}

	// XPN.5 - Prefix
	if len(components) > 4 && components[4] != "" {
		if prefix := components[4]; prefix != "" {
			name["prefix"] = []string{prefix}
		}
	}

	// XPN.7 - Name Type Code
	if len(components) > 6 && components[6] != "" {
		nameTypeCode := components[6]
		switch strings.ToUpper(nameTypeCode) {
		case "L":
			name["use"] = "official"
		case "D":
			name["use"] = "usual"
		case "M":
			name["use"] = "maiden"
		case "N":
			name["use"] = "nickname"
		}
	}

	// Apply transformation rules
	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			name["use"] = use
		}
	}

	return name
}

// =====================================
// CONTACT POINT TRANSFORMATIONS
// =====================================

func (utils *FHIRDataTypeUtils) TransformXTNToContactPoint(
	xtn string,
	rules map[string]interface{},
) map[string]interface{} {
	if xtn == "" {
		return nil
	}

	// Handle complex XTN field with multiple components
	if strings.Contains(xtn, "^") {
		return utils.processComplexXTNSingle(xtn, rules)
	}

	// Simple phone number/email
	return utils.createSimpleContactPoint(xtn, rules)
}

func (utils *FHIRDataTypeUtils) processComplexXTNSingle(
	xtn string,
	rules map[string]interface{},
) map[string]interface{} {
	components := strings.Split(xtn, "^")

	// Take the first non-empty component as the primary contact
	for i, component := range components {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}

		contactPoint := utils.createContactPointFromComponent(component, i+1, rules)
		if contactPoint != nil {
			return contactPoint
		}
	}

	return nil
}

func (utils *FHIRDataTypeUtils) processComplexXTN(
	xtn string,
	rules map[string]interface{},
) interface{} {
	var contactPoints []map[string]interface{}

	// Parse components using simple string splitting (removed hl7.Component dependency)
	components := strings.Split(xtn, "^")
	for i, component := range components {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}

		contactPoint := utils.createContactPointFromComponent(component, i+1, rules)
		if contactPoint != nil {
			contactPoints = append(contactPoints, contactPoint)
		}
	}

	// Return single ContactPoint or array
	if len(contactPoints) == 1 {
		return contactPoints[0]
	} else if len(contactPoints) > 1 {
		return contactPoints
	}

	return nil
}

func (utils *FHIRDataTypeUtils) createContactPointFromComponent(
	value string,
	componentNum int,
	rules map[string]interface{},
) map[string]interface{} {
	contactPoint := utils.determineContactPointType(value)

	// Set use based on component position (XTN structure)
	switch componentNum {
	case 1, 2, 3:
		contactPoint["use"] = "home"
	case 4, 5:
		contactPoint["use"] = "work"
	default:
		contactPoint["use"] = "other"
	}

	contactPoint["value"] = value

	// Apply rules
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

func (utils *FHIRDataTypeUtils) createSimpleContactPoint(
	value string,
	rules map[string]interface{},
) map[string]interface{} {
	contactPoint := utils.determineContactPointType(value)
	contactPoint["value"] = value
	contactPoint["use"] = "home"

	// Apply rules
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

func (utils *FHIRDataTypeUtils) determineContactPointType(value string) map[string]interface{} {
	if strings.Contains(value, "@") {
		return map[string]interface{}{
			"system": "email",
		}
	}
	return map[string]interface{}{
		"system": "phone",
	}
}

// =====================================
// ADDRESS TRANSFORMATIONS
// =====================================

// TransformXADToAddress converts HL7 v2 XAD data type to FHIR Address
func (utils *FHIRDataTypeUtils) TransformXADToAddress(
	xad string,
	rules map[string]interface{},
) map[string]interface{} {
	components := strings.Split(xad, "^")
	address := map[string]interface{}{
		"use": "home",
	}

	// XAD.1 - Street Address
	var lines []string
	if len(components) > 0 && components[0] != "" {
		lines = append(lines, components[0])
	}

	// XAD.2 - Other Designation
	if len(components) > 1 && components[1] != "" {
		lines = append(lines, components[1])
	}

	if len(lines) > 0 {
		address["line"] = lines
	}

	// XAD.3 - City
	if len(components) > 2 && components[2] != "" {
		address["city"] = components[2]
	}

	// XAD.4 - State or Province
	if len(components) > 3 && components[3] != "" {
		address["state"] = components[3]
	}

	// XAD.5 - Zip or Postal Code
	if len(components) > 4 && components[4] != "" {
		address["postalCode"] = components[4]
	}

	// XAD.6 - Country
	if len(components) > 5 && components[5] != "" {
		address["country"] = components[5]
	}

	// XAD.7 - Address Type
	if len(components) > 6 && components[6] != "" {
		addressType := strings.ToUpper(components[6])
		switch addressType {
		case "H":
			address["use"] = "home"
		case "O", "B":
			address["use"] = "work"
		case "T":
			address["use"] = "temp"
		case "M":
			address["use"] = "billing"
		}
	}

	// Apply transformation rules
	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			address["use"] = use
		}
	}

	return address
}

// =====================================
// DATE/TIME TRANSFORMATIONS
// =====================================

// TransformTSToDate converts HL7 v2 TS (timestamp) to FHIR date
func (utils *FHIRDataTypeUtils) TransformTSToDate(ts string) string {
	// HL7 v2 timestamp format: YYYYMMDD[HHMMSS[.SSSS]][+/-ZZZZ]
	if len(ts) >= 8 {
		year := ts[0:4]
		month := ts[4:6]
		day := ts[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return ts
}

// TransformTSToDateTime converts HL7 v2 TS to FHIR dateTime
func (utils *FHIRDataTypeUtils) TransformTSToDateTime(ts string) string {
	if len(ts) >= 14 {
		year := ts[0:4]
		month := ts[4:6]
		day := ts[6:8]
		hour := ts[8:10]
		minute := ts[10:12]
		second := ts[12:14]
		return fmt.Sprintf("%s-%s-%sT%s:%s:%s", year, month, day, hour, minute, second)
	} else if len(ts) >= 8 {
		return utils.TransformTSToDate(ts)
	}
	return ts
}

// =====================================
// CODE/VALUE TRANSFORMATIONS
// =====================================

// TransformGender converts HL7 v2 administrative sex to FHIR gender
func (utils *FHIRDataTypeUtils) TransformGender(gender string) string {
	upperGender := strings.ToUpper(strings.TrimSpace(gender))

	switch upperGender {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "O", "OTHER":
		return "other"
	case "U", "UNKNOWN", "":
		return "unknown"
	default:
		return "unknown"
	}
}

// TransformCEToCodeableConcept converts HL7 v2 CE data type to FHIR CodeableConcept
func (utils *FHIRDataTypeUtils) TransformCEToCodeableConcept(
	ce string,
	rules map[string]interface{},
) map[string]interface{} {
	components := strings.Split(ce, "^")
	codeableConcept := map[string]interface{}{
		"coding": []map[string]interface{}{},
	}

	if len(components) > 0 && components[0] != "" {
		coding := map[string]interface{}{
			"code": components[0],
		}

		// CE.2 - Text
		if len(components) > 1 && components[1] != "" {
			coding["display"] = components[1]
			codeableConcept["text"] = components[1]
		}

		// CE.3 - Coding System
		if len(components) > 2 && components[2] != "" {
			coding["system"] = components[2]
		}

		// Apply transformation rules for system override
		if rules != nil {
			if system, ok := rules["system"].(string); ok {
				coding["system"] = system
			}
		}

		codeableConcept["coding"] = []map[string]interface{}{coding}
	}

	return codeableConcept
}

// =====================================
// UTILITY METHODS
// =====================================

// ParseHL7Component extracts a specific component from an HL7 field
func (utils *FHIRDataTypeUtils) ParseHL7Component(
	fieldValue string,
	componentIndex int,
) string {
	if fieldValue == "" {
		return ""
	}

	// Handle repetitions (~) and components (^)
	repetitions := strings.Split(fieldValue, "~")
	if len(repetitions) == 0 {
		return ""
	}

	components := strings.Split(repetitions[0], "^")
	if componentIndex < 1 || componentIndex > len(components) {
		return ""
	}

	return strings.TrimSpace(components[componentIndex-1])
}

// IsValidEmail checks if a string is a valid email format
func (utils *FHIRDataTypeUtils) IsValidEmail(email string) bool {
	return strings.Contains(email, "@") &&
		strings.Contains(email, ".") &&
		len(email) > 5
}

// NormalizePhoneNumber removes common phone number formatting
func (utils *FHIRDataTypeUtils) NormalizePhoneNumber(phone string) string {
	// Remove common separators and formatting
	normalized := strings.ReplaceAll(phone, "(", "")
	normalized = strings.ReplaceAll(normalized, ")", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, ".", "")

	return normalized
}
