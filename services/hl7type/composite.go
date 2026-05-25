// services/hl7type/composite.go
// HL7 composite data type parsers.
// All functions are lenient and never return errors.
package hl7type

import (
	"fmt"
	"strings"
)

// ParseHD parses an HL7 HD (Hierarchic Designator):
//   namespaceId^universalId^universalIdType
// Returns the namespace ID, or "namespace^uid" if both are present.
func ParseHD(raw string) string {
	if raw == "" {
		return ""
	}
	components := strings.Split(raw, "^")

	ns := trimComp(components, 0)
	uid := trimComp(components, 1)

	if ns != "" && uid != "" {
		return ns + "^" + uid
	}
	if ns != "" {
		return ns
	}
	return uid
}

// ParseEI parses an HL7 EI (Entity Identifier):
//   entityId^namespaceId^universalId^universalIdType
// Returns a FHIR Identifier map.
func ParseEI(raw string, rules map[string]interface{}) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}
	components := strings.Split(raw, "^")

	identifier := map[string]interface{}{
		"use": "usual",
	}

	entityID := trimComp(components, 0)
	if entityID != "" {
		identifier["value"] = entityID
	}

	// Build system from namespaceId / universalId
	ns := trimComp(components, 1)
	uid := trimComp(components, 2)
	uidType := trimComp(components, 3)

	var systemStr string
	if uid != "" && (uidType == "ISO" || uidType == "URI") {
		systemStr = uid
	} else if uid != "" {
		systemStr = uid
	} else if ns != "" {
		systemStr = ns
	}

	if systemStr != "" {
		identifier["system"] = systemStr
	}

	// Apply rule overrides
	if rules != nil {
		if use, ok := rules["use"].(string); ok && use != "" {
			identifier["use"] = use
		}
		if sys, ok := rules["system"].(string); ok && sys != "" {
			identifier["system"] = sys
		}
	}

	return identifier
}

// ParseXON parses an HL7 XON (Extended Composite Name and ID for Organizations):
//   organizationName^organizationNameTypeCode^idNumber^...^assigningAuthority^...
// Returns the organization name string.
func ParseXON(raw string) string {
	if raw == "" {
		return ""
	}
	components := strings.Split(raw, "^")
	name := trimComp(components, 0)
	return name
}

// ParseDR parses an HL7 DR (Date/Time Range): startDateTime^endDateTime
// Returns a FHIR Period map {start, end} and any warnings.
func ParseDR(raw string) (map[string]interface{}, []string) {
	if raw == "" {
		return map[string]interface{}{}, nil
	}

	var warnings []string
	parts := strings.SplitN(raw, "^", 2)

	period := map[string]interface{}{}

	start := strings.TrimSpace(parts[0])
	if start != "" {
		startFHIR, w := ParseTS(start, "dateTime")
		warnings = append(warnings, w...)
		if startFHIR != "" {
			period["start"] = startFHIR
		}
	}

	if len(parts) > 1 {
		end := strings.TrimSpace(parts[1])
		if end != "" {
			endFHIR, w := ParseTS(end, "dateTime")
			warnings = append(warnings, w...)
			if endFHIR != "" {
				period["end"] = endFHIR
			}
		}
	}

	return period, warnings
}

// ParseSN parses an HL7 SN (Structured Numeric):
//   comparator&num1&separator&num2
// Returns a FHIR Quantity or Range map and any warnings.
func ParseSN(raw string) (map[string]interface{}, []string) {
	if raw == "" {
		return map[string]interface{}{}, nil
	}

	var warnings []string
	components := strings.Split(raw, "&")

	comparator := trimComp(components, 0)
	num1Str := trimComp(components, 1)
	separator := trimComp(components, 2)
	num2Str := trimComp(components, 3)

	// If we have a range separator "-" or ":", build a Range
	if separator == "-" || separator == ":" || separator == "^" {
		result := map[string]interface{}{}
		if num1Str != "" {
			low := map[string]interface{}{"value": parseFloat(num1Str)}
			result["low"] = low
		}
		if num2Str != "" {
			high := map[string]interface{}{"value": parseFloat(num2Str)}
			result["high"] = high
		}
		if len(result) == 0 {
			warnings = append(warnings, fmt.Sprintf("SN range has no valid numbers: %q", raw))
		}
		return result, warnings
	}

	// Otherwise, build a Quantity
	quantity := map[string]interface{}{}
	if num1Str != "" {
		quantity["value"] = parseFloat(num1Str)
	} else if num2Str != "" {
		quantity["value"] = parseFloat(num2Str)
	}
	if comparator != "" {
		quantity["comparator"] = comparator
	}

	if len(quantity) == 0 {
		warnings = append(warnings, fmt.Sprintf("SN has no recognizable numeric value: %q", raw))
	}

	return quantity, warnings
}

// ParsePL parses an HL7 PL (Person Location):
//   pointOfCare^room^bed^facility^locationStatus^personLocationType^building^floor
// Returns a human-readable location string.
func ParsePL(raw string) string {
	if raw == "" {
		return ""
	}
	components := strings.Split(raw, "^")

	var parts []string
	for i, label := range []string{"pointOfCare", "room", "bed", "facility"} {
		val := trimComp(components, i)
		if val != "" {
			_ = label
			parts = append(parts, val)
		}
	}

	return strings.Join(parts, " / ")
}

// ParseXPN parses an HL7 XPN (Extended Person Name):
//   family^given^middle^suffix^prefix^degree^nameTypeCode
// Returns a FHIR HumanName map.
func ParseXPN(raw string, rules map[string]interface{}) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}

	components := strings.Split(raw, "^")
	name := map[string]interface{}{
		"use": "official",
	}

	family := trimComp(components, 0)
	given1 := trimComp(components, 1)
	middle := trimComp(components, 2)
	suffix := trimComp(components, 3)
	prefix := trimComp(components, 4)

	if family != "" {
		name["family"] = family
	}

	var givenList []string
	if given1 != "" {
		givenList = append(givenList, given1)
	}
	if middle != "" {
		givenList = append(givenList, middle)
	}
	if len(givenList) > 0 {
		name["given"] = givenList
	}

	if suffix != "" {
		name["suffix"] = []string{suffix}
	}
	if prefix != "" {
		name["prefix"] = []string{prefix}
	}

	// Apply rule overrides
	if rules != nil {
		if use, ok := rules["use"].(string); ok && use != "" {
			name["use"] = use
		}
	}

	return name
}

// ParseXAD parses an HL7 XAD (Extended Address):
//   street^other^city^state^zip^country^addressType
// Returns a FHIR Address map.
func ParseXAD(raw string, rules map[string]interface{}) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}

	components := strings.Split(raw, "^")
	address := map[string]interface{}{
		"use": "home",
	}

	street := trimComp(components, 0)
	other := trimComp(components, 1)
	city := trimComp(components, 2)
	state := trimComp(components, 3)
	zip := trimComp(components, 4)
	country := trimComp(components, 5)
	addrType := trimComp(components, 6)

	var lines []string
	if street != "" {
		lines = append(lines, street)
	}
	if other != "" {
		lines = append(lines, other)
	}
	if len(lines) > 0 {
		address["line"] = lines
	}
	if city != "" {
		address["city"] = city
	}
	if state != "" {
		address["state"] = state
	}
	if zip != "" {
		address["postalCode"] = zip
	}
	if country != "" {
		address["country"] = country
	}

	// Map HL7 address type codes to FHIR address use
	if addrType != "" {
		use := mapAddressType(addrType)
		address["use"] = use
	}

	// Apply rule overrides
	if rules != nil {
		if use, ok := rules["use"].(string); ok && use != "" {
			address["use"] = use
		}
	}

	return address
}

// ParseXTN parses an HL7 XTN (Extended Telecommunication Number):
//   number^useCode^equipType^email^countryCode^cityCode^local^ext
// Returns a FHIR ContactPoint map.
func ParseXTN(raw string, rules map[string]interface{}) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}

	components := strings.Split(raw, "^")

	number := trimComp(components, 0)
	useCode := trimComp(components, 1)
	equipType := trimComp(components, 2)
	email := trimComp(components, 3)

	contactPoint := map[string]interface{}{}

	// Determine system
	system := "phone"
	if email != "" {
		system = "email"
		contactPoint["value"] = email
	} else if equipType != "" {
		system = mapEquipTypeToFHIR(equipType)
		if number != "" {
			contactPoint["value"] = number
		}
	} else if number != "" {
		contactPoint["value"] = number
	}
	contactPoint["system"] = system

	// Map use code
	use := mapTelecomUseCode(useCode)
	if use != "" {
		contactPoint["use"] = use
	}

	// Apply rule overrides
	if rules != nil {
		if ruleUse, ok := rules["use"].(string); ok && ruleUse != "" {
			contactPoint["use"] = ruleUse
		}
		if ruleSys, ok := rules["system"].(string); ok && ruleSys != "" {
			contactPoint["system"] = ruleSys
		}
	}

	return contactPoint
}

// ParseCX parses an HL7 CX (Extended Composite ID with Check Digit):
//   id^check^checkScheme^assigningAuth^typeCode^assigningFacility
// Returns a FHIR Identifier map.
func ParseCX(raw string, rules map[string]interface{}) map[string]interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}

	components := strings.Split(raw, "^")
	identifier := map[string]interface{}{
		"use": "usual",
	}

	id := trimComp(components, 0)
	if id != "" {
		identifier["value"] = id
	}

	// Assigning authority (component 3, 0-based index 3) — use as system
	assigningAuth := trimComp(components, 3)
	if assigningAuth != "" {
		identifier["system"] = assigningAuth
	}

	// Type code (component 4, 0-based index 4)
	typeCode := trimComp(components, 4)
	if typeCode != "" {
		identifier["type"] = map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
					"code":   typeCode,
				},
			},
		}
	}

	// Apply rule overrides
	if rules != nil {
		if use, ok := rules["use"].(string); ok && use != "" {
			identifier["use"] = use
		}
		if sys, ok := rules["system"].(string); ok && sys != "" {
			identifier["system"] = sys
		}
	}

	return identifier
}

// ─────────────────────────── helpers ───────────────────────────

func trimComp(components []string, idx int) string {
	if idx < len(components) {
		return strings.TrimSpace(components[idx])
	}
	return ""
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func mapAddressType(hl7Type string) string {
	switch strings.ToUpper(hl7Type) {
	case "H", "HOME":
		return "home"
	case "W", "WORK":
		return "work"
	case "T", "TEMP", "TEMPORARY":
		return "temp"
	case "O", "OLD", "BAD":
		return "old"
	case "B", "BILLING":
		return "billing"
	default:
		return "home"
	}
}

func mapEquipTypeToFHIR(equipType string) string {
	switch strings.ToUpper(equipType) {
	case "PH", "PHONE":
		return "phone"
	case "FX", "FAX":
		return "fax"
	case "MD", "MODEM":
		return "other"
	case "CP", "CELL":
		return "phone"
	case "SAT":
		return "phone"
	case "TDD", "TTY":
		return "other"
	case "X400":
		return "email"
	case "NET", "EMAIL":
		return "email"
	default:
		return "phone"
	}
}

func mapTelecomUseCode(code string) string {
	switch strings.ToUpper(code) {
	case "PRN", "HOME":
		return "home"
	case "WPN", "WORK":
		return "work"
	case "NET":
		return "home"
	case "BPN":
		return "mobile"
	case "ORN":
		return "old"
	default:
		return ""
	}
}
