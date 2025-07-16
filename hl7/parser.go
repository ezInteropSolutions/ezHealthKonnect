// FILE: hl7/parser.go
// Fixed version with correct package declaration and structure
package hl7

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseHL7Message performs complete HL7 parsing with components and repetitions
func ParseHL7Message(rawMessage string) *BasicParsedMessage {
	// Normalize line endings
	normalizedMessage := strings.ReplaceAll(rawMessage, "\r\n", "\n")
	normalizedMessage = strings.ReplaceAll(normalizedMessage, "\r", "\n")

	// Split the HL7 message into lines (segments)
	lines := strings.Split(normalizedMessage, "\n")

	segments := make(map[string]BasicSegment)
	messageType := "UNKNOWN"

	// Go through each line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) >= 3 {
			segmentName := line[:3]

			// Parse segment fields with complete HL7 structure
			fields := strings.Split(line, "|")
			segmentFields := make(map[string]string)

			// ✅ Handle MSH segment specially + ensure all positions are filled
			if segmentName == "MSH" {
				// MSH.1 is the field separator "|" itself (implicit)
				segmentFields["MSH.1"] = "|"

				// MSH fields start at MSH.2, but we need to ensure proper positioning
				for i := 1; i < len(fields); i++ {
					fieldKey := fmt.Sprintf("MSH.%d", i+1)
					segmentFields[fieldKey] = fields[i]
				}
			} else {
				// ✅ All other segments - ensure 1-based indexing with gap handling
				for i := 1; i < len(fields); i++ {
					fieldKey := fmt.Sprintf("%s.%d", segmentName, i)
					segmentFields[fieldKey] = fields[i]
				}
			}

			segments[segmentName] = BasicSegment{
				Name:   segmentName,
				Fields: segmentFields,
				Raw:    line,
			}

			// Extract message type from MSH segment
			if segmentName == "MSH" {
				messageType = extractMessageType(line)
			}
		}
	}

	return &BasicParsedMessage{
		Raw:         rawMessage,
		MessageType: messageType,
		Segments:    segments,
		ParsedAt:    time.Now().Format(time.RFC3339),
	}
}

// extractMessageType extracts message type from MSH segment
func extractMessageType(mshSegment string) string {
	fields := strings.Split(mshSegment, "|")
	// MSH.9 is at index 8 (since MSH.1 is implicit)
	if len(fields) > 8 {
		return fields[8]
	}
	return "UNKNOWN"
}

// ConvertBasicToEnhanced converts basic segments to enhanced format
func ConvertBasicToEnhanced(basicSegments map[string]BasicSegment) []EnhancedSegment {
	enhancedList := make([]EnhancedSegment, 0)

	segmentDescriptions := map[string]string{
		"MSH": "Message Header - Contains sender, receiver, and message control information",
		"EVN": "Event Type - Describes the trigger event for this message",
		"PID": "Patient Identification - Primary patient demographics and identifiers",
		"NK1": "Next of Kin/Associated Parties - Emergency contacts and relationships",
		"PV1": "Patient Visit - Encounter details, location, and attending physician",
		"PV2": "Patient Visit Additional Info - Extended visit information and services",
		"OBX": "Observation/Result - Laboratory results, vital signs, or clinical observations",
		"OBR": "Observation Request - Orders for lab tests, procedures, or studies",
		"ORC": "Common Order - Order control information and status",
		"AL1": "Patient Allergy Information - Known allergies and adverse reactions",
		"DG1": "Diagnosis - Primary and secondary diagnoses with codes",
		"NTE": "Notes and Comments - Free text annotations and clarifications",
		"GT1": "Guarantor - Financial responsibility and billing information",
		"IN1": "Insurance - Primary insurance coverage details",
		"IN2": "Insurance Additional Info - Extended insurance information",
		"PR1": "Procedures - Surgical and medical procedures performed",
	}

	segmentPurposes := map[string]string{
		"MSH": "Establishes message routing and control parameters",
		"EVN": "Documents when and why this healthcare event occurred",
		"PID": "Uniquely identifies the patient across healthcare systems",
		"NK1": "Provides emergency contacts and patient relationships",
		"PV1": "Tracks patient location and care team assignments",
		"PV2": "Extends visit information with additional clinical context",
		"OBX": "Communicates clinical findings and test results",
		"OBR": "Orders diagnostic studies and procedures",
		"ORC": "Controls order workflow and status tracking",
		"AL1": "Alerts providers to patient safety considerations",
		"DG1": "Documents medical conditions for care planning",
		"NTE": "Adds contextual information not captured elsewhere",
	}

	segmentNames := getBasicHL7SegmentOrder(basicSegments)

	// Process segments in sorted order
	for _, segName := range segmentNames {
		basicSeg := basicSegments[segName]

		// Convert fields with proper position-based ordering
		fieldsList := make([]FieldInfo, 0)

		// Create a complete position map to handle gaps properly
		fieldPositions := make(map[int]string)
		maxPosition := 0

		// First pass: collect all field positions and values
		for fieldKey, fieldValue := range basicSeg.Fields {
			position, err := extractFieldPosition(fieldKey)
			if err != nil {
				fmt.Printf("⚠️ WARNING: Skipping field %s: %v\n", fieldKey, err)
				continue
			}

			fieldPositions[position] = fieldValue
			if position > maxPosition {
				maxPosition = position
			}
		}

		// Generate fields for ALL positions from 1 to maxPosition
		for position := 1; position <= maxPosition; position++ {
			fieldKey := fmt.Sprintf("%s.%d", segName, position)
			fieldValue := fieldPositions[position] // Will be empty string if not present

			// Get field metadata
			fieldName := getFieldName(segName, position)
			fieldDesc := getFieldDescription(segName, position)
			dataType := getFieldDataType(segName, position)
			optionality := getFieldOptionality(segName, position)

			// Parse field components and repetitions
			subfields := parseFieldComponents(fieldValue, fieldKey, segName, position)

			field := FieldInfo{
				Key:         fieldKey,
				Value:       fieldValue,
				Name:        fieldName,
				Description: fieldDesc,
				DataType:    dataType,
				Optionality: optionality,
				Cardinality: "[0..1]",
				Position:    position,
				HasValue:    fieldValue != "",
				Subfields:   subfields,
				Sequence:    position - 1,
			}

			fieldsList = append(fieldsList, field)
		}

		description := segmentDescriptions[segName]
		if description == "" {
			description = fmt.Sprintf("%s Segment - Parsed by ezHealthKonnect", segName)
		}

		purpose := segmentPurposes[segName]
		if purpose == "" {
			purpose = fmt.Sprintf("Processed by ezHealthKonnect high-performance engine")
		}

		enhancedSeg := EnhancedSegment{
			Key:              segName,
			Raw:              basicSeg.Raw,
			Name:             description,
			Description:      description,
			Purpose:          purpose,
			Fields:           fieldsList,
			FieldCount:       len(fieldsList),
			DictionarySource: "ezHealthKonnect_enhanced_parser",
			Required:         (segName == "MSH" || segName == "PID"),
			Repeating:        false,
		}

		enhancedList = append(enhancedList, enhancedSeg)
	}

	return enhancedList
}

// Helper function to extract field position from field key
func extractFieldPosition(fieldKey string) (int, error) {
	parts := strings.Split(fieldKey, ".")
	if len(parts) >= 2 {
		position, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid field position in key %s: %v", fieldKey, err)
		}
		if position < 1 {
			return 0, fmt.Errorf("invalid field position %d in key %s: positions must be >= 1", position, fieldKey)
		}
		return position, nil
	}
	return 0, fmt.Errorf("invalid field key format: %s (expected format: SEGMENT.POSITION)", fieldKey)
}

// Parse field components, repetitions, and subcomponents
func parseFieldComponents(fieldValue, fieldKey, segmentName string, position int) []SubfieldInfo {
	if fieldValue == "" {
		return []SubfieldInfo{}
	}

	var subfields []SubfieldInfo

	// Handle field repetitions (separated by ~)
	repetitions := strings.Split(fieldValue, "~")

	for repIndex, repetition := range repetitions {
		if strings.TrimSpace(repetition) == "" {
			continue
		}

		// Parse components within this repetition (separated by ^)
		components := strings.Split(repetition, "^")

		for compIndex, component := range components {
			if component == "" && compIndex > 0 {
				continue // Skip empty trailing components
			}

			componentPosition := compIndex + 1
			subfieldKey := fmt.Sprintf("%s.%d", fieldKey, componentPosition)

			if len(repetitions) > 1 {
				subfieldKey = fmt.Sprintf("%s[%d].%d", fieldKey, repIndex+1, componentPosition)
			}

			// Get component name based on field type and position
			componentName := getComponentName(segmentName, position, componentPosition)

			// Parse subcomponents within this component (separated by &)
			subcomponents := strings.Split(component, "&")
			hasSubcomponents := len(subcomponents) > 1

			subfield := SubfieldInfo{
				Key:         subfieldKey,
				Name:        componentName,
				DataType:    getComponentDataType(segmentName, position, componentPosition),
				Usage:       "O",
				Length:      len(component),
				Position:    componentPosition,
				Description: getComponentDescription(segmentName, position, componentPosition),
				HasValue:    component != "",
				Value:       component,
				Sequence:    compIndex,
			}

			// Add subcomponent info if present
			if hasSubcomponents {
				subfield.Description += fmt.Sprintf(" (Has %d subcomponents)", len(subcomponents))
			}

			subfields = append(subfields, subfield)
		}
	}

	return subfields
}

// Get component names based on HL7 standards
func getComponentName(segmentName string, fieldPosition, componentPosition int) string {
	componentMap := map[string]map[int]map[int]string{
		"PID": {
			5: { // Patient Name (XPN)
				1: "Family Name",
				2: "Given Name",
				3: "Second/Middle Name",
				4: "Suffix",
				5: "Prefix",
				6: "Degree",
			},
			11: { // Patient Address (XAD)
				1: "Street Address",
				2: "Other Designation",
				3: "City",
				4: "State/Province",
				5: "Zip/Postal Code",
				6: "Country",
			},
		},
		"MSH": {
			9: { // Message Type (MSG)
				1: "Message Code",
				2: "Trigger Event",
				3: "Message Structure",
			},
		},
		"PV1": {
			3: { // Assigned Patient Location (PL)
				1: "Point of Care",
				2: "Room",
				3: "Bed",
				4: "Facility",
			},
			7: { // Attending Doctor (XCN)
				1: "ID Number",
				2: "Family Name",
				3: "Given Name",
				4: "Middle Initial",
				5: "Suffix",
			},
		},
	}

	if segmentMap, exists := componentMap[segmentName]; exists {
		if fieldMap, exists := segmentMap[fieldPosition]; exists {
			if name, exists := fieldMap[componentPosition]; exists {
				return name
			}
		}
	}

	return fmt.Sprintf("Component %d", componentPosition)
}

// Get component data types
func getComponentDataType(segmentName string, fieldPosition, componentPosition int) string {
	dataTypeMap := map[string]map[int]map[int]string{
		"PID": {
			5: {1: "FN", 2: "ST", 3: "ST", 4: "ST", 5: "ST", 6: "ST"},
			7: {1: "TS"},
		},
		"MSH": {
			9: {1: "ID", 2: "ID", 3: "ID"},
		},
	}

	if segmentMap, exists := dataTypeMap[segmentName]; exists {
		if fieldMap, exists := segmentMap[fieldPosition]; exists {
			if dataType, exists := fieldMap[componentPosition]; exists {
				return dataType
			}
		}
	}

	return "ST"
}

// Get component descriptions
func getComponentDescription(segmentName string, fieldPosition, componentPosition int) string {
	descMap := map[string]map[int]map[int]string{
		"PID": {
			5: {
				1: "Patient's family name (surname)",
				2: "Patient's given name (first name)",
				3: "Patient's middle name or initial",
				4: "Name suffix (Jr., Sr., III, etc.)",
				5: "Name prefix (Dr., Mr., Ms., etc.)",
				6: "Professional degree (MD, RN, etc.)",
			},
		},
		"MSH": {
			9: {
				1: "Message type code (ADT, ORU, etc.)",
				2: "Trigger event code (A01, R01, etc.)",
				3: "Message structure identifier",
			},
		},
	}

	if segmentMap, exists := descMap[segmentName]; exists {
		if fieldMap, exists := segmentMap[fieldPosition]; exists {
			if desc, exists := fieldMap[componentPosition]; exists {
				return desc
			}
		}
	}

	return fmt.Sprintf("Component %d of %s.%d", componentPosition, segmentName, fieldPosition)
}

// getBasicHL7SegmentOrder returns segments in basic HL7 order
func getBasicHL7SegmentOrder(basicSegments map[string]BasicSegment) []string {
	standardOrder := []string{
		"MSH", "EVN", "PID", "NK1", "PV1", "PV2", "ROL",
		"OBX", "OBR", "ORC", "RXO", "RXR", "RXC", "RXA", "RXE",
		"DG1", "AL1", "PR1", "GT1", "IN1", "IN2", "IN3",
		"ACC", "UB1", "UB2", "NTE", "DSC",
	}

	var orderedSegments []string
	segmentExists := make(map[string]bool)

	for _, segmentName := range standardOrder {
		if _, exists := basicSegments[segmentName]; exists {
			orderedSegments = append(orderedSegments, segmentName)
			segmentExists[segmentName] = true
		}
	}

	var remainingSegments []string
	for segmentName := range basicSegments {
		if !segmentExists[segmentName] {
			remainingSegments = append(remainingSegments, segmentName)
		}
	}

	sort.Strings(remainingSegments)
	orderedSegments = append(orderedSegments, remainingSegments...)

	return orderedSegments
}

// getFieldName returns intelligent field names based on HL7 standards
func getFieldName(segmentName string, position int) string {
	fieldNames := map[string]map[int]string{
		"MSH": {
			1: "Field Separator", 2: "Encoding Characters", 3: "Sending Application",
			4: "Sending Facility", 5: "Receiving Application", 6: "Receiving Facility",
			7: "Date/Time Of Message", 8: "Security", 9: "Message Type",
			10: "Message Control ID", 11: "Processing ID", 12: "Version ID",
		},
		"PID": {
			1: "Set ID", 2: "Patient ID", 3: "Patient Identifier List",
			5: "Patient Name", 7: "Date/Time of Birth", 8: "Administrative Sex",
			11: "Patient Address", 13: "Phone Number - Home", 18: "Patient Account Number",
		},
		"PV1": {
			1: "Set ID", 2: "Patient Class", 3: "Assigned Patient Location",
			7: "Attending Doctor", 8: "Referring Doctor", 19: "Visit Number",
			44: "Admit Date/Time", 45: "Discharge Date/Time",
		},
		"OBX": {
			1: "Set ID", 2: "Value Type", 3: "Observation Identifier",
			5: "Observation Value", 6: "Units", 7: "References Range",
			8: "Abnormal Flags", 11: "Observation Result Status",
		},
	}

	if segFields, exists := fieldNames[segmentName]; exists {
		if name, exists := segFields[position]; exists {
			return name
		}
	}

	return fmt.Sprintf("Field %d", position)
}

// getFieldDescription returns detailed field descriptions
func getFieldDescription(segmentName string, position int) string {
	descriptions := map[string]map[int]string{
		"MSH": {
			1:  "Field separator character",
			2:  "Encoding characters (component, repetition, escape, subcomponent)",
			3:  "Name of the application that is sending the message",
			4:  "Name of the facility that is sending the message",
			5:  "Name of the application that is receiving the message",
			6:  "Name of the facility that is receiving the message",
			7:  "Date and time the message was created",
			8:  "Security information for the message",
			9:  "Message type and trigger event code",
			10: "Unique control ID for this message",
			11: "Processing ID (Production, Training, or Debug)",
			12: "HL7 version number",
		},
		"PID": {
			1:  "Sequence number for patient identification",
			2:  "External patient ID (deprecated)",
			3:  "Internal patient identifier (MRN, Account Number, etc.)",
			4:  "Alternate patient ID",
			5:  "Legal name of the patient",
			6:  "Mother's maiden name",
			7:  "Date and time of birth",
			8:  "Administrative sex (M=Male, F=Female, U=Unknown)",
			9:  "Patient alias names",
			10: "Race information",
			11: "Patient address information",
			12: "County code",
			13: "Phone number - home",
			14: "Phone number - business",
			15: "Primary language",
			16: "Marital status",
			17: "Religion",
			18: "Patient account number",
			19: "Social security number",
		},
		"EVN": {
			1: "Event type code",
			2: "Date/time when event occurred",
			3: "Date/time when event was planned",
			4: "Event reason code",
			5: "Operator ID",
			6: "Date/time event started",
		},
		"PV1": {
			1:  "Set ID for patient visit",
			2:  "Patient class (I=Inpatient, O=Outpatient, E=Emergency, etc.)",
			3:  "Assigned patient location",
			4:  "Admission type",
			5:  "Preadmit number",
			6:  "Prior patient location",
			7:  "Attending doctor",
			8:  "Referring doctor",
			9:  "Consulting doctor",
			10: "Hospital service",
			19: "Visit number",
			44: "Admit date/time",
			45: "Discharge date/time",
		},
	}

	if segFields, exists := descriptions[segmentName]; exists {
		if desc, exists := segFields[position]; exists {
			return desc
		}
	}

	return fmt.Sprintf("%s field %d", segmentName, position)
}

// getFieldDataType returns HL7 data types
func getFieldDataType(segmentName string, position int) string {
	dataTypes := map[string]map[int]string{
		"MSH": {3: "HD", 4: "HD", 7: "TS", 9: "MSG", 10: "ST", 11: "PT", 12: "VID"},
		"PID": {3: "CX", 5: "XPN", 7: "TS", 8: "IS", 11: "XAD", 13: "XTN"},
		"PV1": {2: "IS", 3: "PL", 7: "XCN", 8: "XCN", 19: "CX"},
		"OBX": {2: "ID", 3: "CE", 5: "varies", 6: "CE", 8: "IS", 11: "ID"},
	}

	if segFields, exists := dataTypes[segmentName]; exists {
		if dataType, exists := segFields[position]; exists {
			return dataType
		}
	}

	return "ST"
}

// getFieldOptionality returns field requirement status
func getFieldOptionality(segmentName string, position int) string {
	required := map[string]map[int]string{
		"MSH": {1: "R", 2: "R", 9: "R", 10: "R", 11: "R", 12: "R"},
		"PID": {3: "R", 5: "R"},
		"PV1": {2: "R"},
	}

	if segFields, exists := required[segmentName]; exists {
		if opt, exists := segFields[position]; exists {
			return opt
		}
	}

	return "O"
}
