package services

import (
	"fmt"
	"sort"
	"strings"
)

// narrativeRowsBuilder appends resource-specific rows to a narrative table.
// Method expressions are used so each builder is a plain function value stored
// in the registry — adding a new resource type is a single registry entry + method.
type narrativeRowsBuilder func(s *HL7FHIRTransformServiceV3, html *strings.Builder, resource map[string]interface{})

// narrativeBuilderRegistry maps FHIR resourceType → its row-builder.
// Register new resource types here; the engine picks them up automatically.
var narrativeBuilderRegistry = map[string]narrativeRowsBuilder{
	"Patient":          (*HL7FHIRTransformServiceV3).addPatientRows,
	"Encounter":        (*HL7FHIRTransformServiceV3).addEncounterRows,
	"MessageHeader":    (*HL7FHIRTransformServiceV3).addMessageHeaderRows,
	"DiagnosticReport": (*HL7FHIRTransformServiceV3).addDiagnosticReportRows,
	"Observation":      (*HL7FHIRTransformServiceV3).addObservationRows,
	"RelatedPerson":    (*HL7FHIRTransformServiceV3).addRelatedPersonRows,
	"Coverage":         (*HL7FHIRTransformServiceV3).addCoverageRows,
}

// generateNarrative creates a FHIR-spec compliant XHTML narrative (text.div).
// The div wrapper and xmlns are required by the FHIR narrative spec.
func (s *HL7FHIRTransformServiceV3) generateNarrative(resourceType string, resource map[string]interface{}) string {
	html := s.generateResourceTable(resourceType, resource)
	return fmt.Sprintf("<div xmlns=\"http://www.w3.org/1999/xhtml\">%s</div>", html)
}

// generateResourceTable renders an XHTML table of key resource fields.
// Looks up a registered builder for the resourceType; falls back to generic scalar rendering.
func (s *HL7FHIRTransformServiceV3) generateResourceTable(resourceType string, resource map[string]interface{}) string {
	var html strings.Builder

	html.WriteString("<table class=\"grid\" style=\"border-collapse:collapse;width:100%;\">")
	html.WriteString(fmt.Sprintf("<thead><tr style=\"background:#f0f0f0;\"><th colspan=\"2\" style=\"padding:8px;text-align:left;\">%s</th></tr></thead>", resourceType))
	html.WriteString("<tbody>")

	s.addTableRow(&html, "ID", fmt.Sprintf("%v", resource["id"]))

	if builder, ok := narrativeBuilderRegistry[resourceType]; ok {
		builder(s, &html, resource)
	} else {
		s.addGenericRows(&html, resource)
	}

	html.WriteString("</tbody></table>")
	return html.String()
}

// extractFirstGiven returns the first given name from a name map's "given" field.
// Handles both []interface{} (after JSON round-trip) and []string (direct engine output).
func extractFirstGiven(name map[string]interface{}) string {
	switch gv := name["given"].(type) {
	case []interface{}:
		if len(gv) > 0 {
			s, _ := gv[0].(string)
			return s
		}
	case []string:
		if len(gv) > 0 {
			return gv[0]
		}
	}
	return ""
}

// addGenericRows renders all scalar fields alphabetically for resource types
// that have no dedicated builder registered in narrativeBuilderRegistry.
func (s *HL7FHIRTransformServiceV3) addGenericRows(html *strings.Builder, resource map[string]interface{}) {
	keys := make([]string, 0, len(resource))
	for key := range resource {
		if key != "resourceType" && key != "id" && key != "text" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		switch v := resource[key].(type) {
		case string:
			s.addTableRow(html, key, v)
		case bool:
			s.addTableRow(html, key, fmt.Sprintf("%v", v))
		case float64:
			s.addTableRow(html, key, fmt.Sprintf("%g", v))
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addTableRow(buf *strings.Builder, label, value string) {
	if value == "" || value == "<nil>" {
		return
	}
	// Escape HTML entities in value
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")

	buf.WriteString(fmt.Sprintf("<tr><td style=\"padding:4px 8px;border:1px solid #ddd;font-weight:bold;\">%s</td><td style=\"padding:4px 8px;border:1px solid #ddd;\">%s</td></tr>",
		label, value))
}

func (s *HL7FHIRTransformServiceV3) addPatientRows(html *strings.Builder, resource map[string]interface{}) {
	// Name
	if names, ok := resource["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			family, _ := name["family"].(string)
			given := extractFirstGiven(name)
			if family != "" || given != "" {
				s.addTableRow(html, "Name", fmt.Sprintf("%s, %s", family, given))
			}
		}
	}

	// Gender
	if gender, ok := resource["gender"].(string); ok {
		s.addTableRow(html, "Gender", gender)
	}

	// Birth Date
	if birthDate, ok := resource["birthDate"].(string); ok {
		s.addTableRow(html, "Birth Date", birthDate)
	}

	// Address
	if addresses, ok := resource["address"].([]interface{}); ok && len(addresses) > 0 {
		if addr, ok := addresses[0].(map[string]interface{}); ok {
			var parts []string
			if city, ok := addr["city"].(string); ok && city != "" {
				parts = append(parts, city)
			}
			if state, ok := addr["state"].(string); ok && state != "" {
				parts = append(parts, state)
			}
			if len(parts) > 0 {
				s.addTableRow(html, "Address", strings.Join(parts, ", "))
			}
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addEncounterRows(html *strings.Builder, resource map[string]interface{}) {
	// Class
	if class, ok := resource["class"].(map[string]interface{}); ok {
		if code, ok := class["code"].(string); ok {
			classDesc := code
			// Map common codes to descriptions
			switch code {
			case "I":
				classDesc = "Inpatient"
			case "O":
				classDesc = "Outpatient"
			case "E":
				classDesc = "Emergency"
			}
			s.addTableRow(html, "Class", classDesc)
		}
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Period
	if period, ok := resource["period"].(map[string]interface{}); ok {
		if start, ok := period["start"].(string); ok {
			s.addTableRow(html, "Start", start)
		}
		if end, ok := period["end"].(string); ok {
			s.addTableRow(html, "End", end)
		}
	}

	// Location
	if locations, ok := resource["location"].([]interface{}); ok && len(locations) > 0 {
		if loc, ok := locations[0].(map[string]interface{}); ok {
			if location, ok := loc["location"].(map[string]interface{}); ok {
				if display, ok := location["display"].(string); ok {
					s.addTableRow(html, "Location", display)
				}
			}
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addMessageHeaderRows(html *strings.Builder, resource map[string]interface{}) {
	// Event
	if eventCoding, ok := resource["eventCoding"].(map[string]interface{}); ok {
		code, _ := eventCoding["code"].(string)
		display, _ := eventCoding["display"].(string)
		if code != "" {
			eventStr := code
			if display != "" {
				eventStr = fmt.Sprintf("%s (%s)", code, display)
			}
			s.addTableRow(html, "Event", eventStr)
		}
	}

	// Source
	if source, ok := resource["source"].(map[string]interface{}); ok {
		if name, ok := source["name"].(string); ok {
			s.addTableRow(html, "Source", name)
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addDiagnosticReportRows(html *strings.Builder, resource map[string]interface{}) {
	// Report name from code.text or code.coding[0].display
	reportName := ""
	if code, ok := resource["code"].(map[string]interface{}); ok {
		if t, ok2 := code["text"].(string); ok2 && t != "" {
			reportName = t
		} else if codings, ok2 := code["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				reportName, _ = c["display"].(string)
			}
		}
	}
	if reportName != "" {
		s.addTableRow(html, "Report", reportName)
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Effective date
	if eff, ok := resource["effectiveDateTime"].(string); ok {
		s.addTableRow(html, "Date", eff)
	}

	// Results count
	if results, ok := resource["result"].([]interface{}); ok {
		s.addTableRow(html, "Results", fmt.Sprintf("%d observation(s)", len(results)))
	}
}

func (s *HL7FHIRTransformServiceV3) addObservationRows(html *strings.Builder, resource map[string]interface{}) {
	// Code
	codeName := ""
	if code, ok := resource["code"].(map[string]interface{}); ok {
		if t, ok2 := code["text"].(string); ok2 && t != "" {
			codeName = t
		} else if codings, ok2 := code["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				codeName, _ = c["display"].(string)
			}
		}
	}
	if codeName != "" {
		s.addTableRow(html, "Code", codeName)
	}

	// Value — handle all value[x] types
	for _, vk := range []string{"valueString", "valueQuantity", "valueCodeableConcept",
		"valueDateTime", "valueBoolean", "valueInteger", "valueTime", "valueSampledData"} {
		if val, ok := resource[vk]; ok {
			switch v := val.(type) {
			case string:
				s.addTableRow(html, "Value", v)
			case bool:
				s.addTableRow(html, "Value", fmt.Sprintf("%v", v))
			case float64:
				s.addTableRow(html, "Value", fmt.Sprintf("%g", v))
			case map[string]interface{}:
				// Quantity: "value unit" or CodeableConcept: text or coding[0].display
				if num, ok2 := v["value"]; ok2 {
					unit, _ := v["unit"].(string)
					comp, _ := v["comparator"].(string)
					numStr := fmt.Sprintf("%g", num)
					if comp != "" {
						s.addTableRow(html, "Value", fmt.Sprintf("%s %s %s", comp, numStr, unit))
					} else {
						s.addTableRow(html, "Value", fmt.Sprintf("%s %s", numStr, unit))
					}
				} else if text, ok2 := v["text"].(string); ok2 && text != "" {
					s.addTableRow(html, "Value", text)
				} else if codings, ok2 := v["coding"].([]interface{}); ok2 && len(codings) > 0 {
					if c, ok3 := codings[0].(map[string]interface{}); ok3 {
						if disp, _ := c["display"].(string); disp != "" {
							s.addTableRow(html, "Value", disp)
						}
					}
				}
			}
			break // only one value[x] can be present
		}
	}
	if _, hasAtt := resource["valueAttachment"]; hasAtt {
		s.addTableRow(html, "Value", "(attachment)")
	}
	if derived, ok := resource["derivedFrom"].([]interface{}); ok && len(derived) > 0 {
		s.addTableRow(html, "Value", "(referenced document)")
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Date
	if eff, ok := resource["effectiveDateTime"].(string); ok {
		s.addTableRow(html, "Date", eff)
	}
}

func (s *HL7FHIRTransformServiceV3) addRelatedPersonRows(html *strings.Builder, resource map[string]interface{}) {
	// Name
	if names, ok := resource["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			family, _ := name["family"].(string)
			given := extractFirstGiven(name)
			if family != "" || given != "" {
				s.addTableRow(html, "Name", fmt.Sprintf("%s, %s", family, given))
			}
		}
	}

	// Gender
	if gender, ok := resource["gender"].(string); ok {
		s.addTableRow(html, "Gender", gender)
	}

	// Birth Date
	if birthDate, ok := resource["birthDate"].(string); ok {
		s.addTableRow(html, "Birth Date", birthDate)
	}
}

func (s *HL7FHIRTransformServiceV3) addCoverageRows(html *strings.Builder, resource map[string]interface{}) {
	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Subscriber ID
	if sid, ok := resource["subscriberId"].(string); ok {
		s.addTableRow(html, "Subscriber ID", sid)
	}

	// Class — show type code + value
	if classes, ok := resource["class"].([]interface{}); ok && len(classes) > 0 {
		if cls, ok := classes[0].(map[string]interface{}); ok {
			value, _ := cls["value"].(string)
			typeCode := ""
			if t, ok2 := cls["type"].(map[string]interface{}); ok2 {
				if codings, ok3 := t["coding"].([]interface{}); ok3 && len(codings) > 0 {
					if c, ok4 := codings[0].(map[string]interface{}); ok4 {
						typeCode, _ = c["code"].(string)
					}
				}
			}
			if value != "" {
				label := "Class"
				if typeCode != "" {
					label = fmt.Sprintf("Class (%s)", typeCode)
				}
				s.addTableRow(html, label, value)
			}
		}
	}

	// Relationship
	if rel, ok := resource["relationship"].(map[string]interface{}); ok {
		if codings, ok2 := rel["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				code, _ := c["code"].(string)
				if code != "" {
					s.addTableRow(html, "Relationship", strings.ToUpper(code))
				}
			}
		}
	}
}
