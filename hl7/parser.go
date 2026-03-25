// FILE: hl7/enhanced_parser.go
// Enhanced HL7 parser with proper handling of ^, &, and ~ delimiters
package hl7

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HL7 Delimiters - these are standard in HL7 messages
const (
	FieldSeparator        = "|"
	ComponentSeparator    = "^"
	RepetitionSeparator   = "~"
	EscapeCharacter       = "\\"
	SubcomponentSeparator = "&"
)

// EnhancedFieldData represents a fully parsed HL7 field with all its components
type EnhancedFieldData struct {
	RawValue       string            `json:"rawValue"`
	Repetitions    []FieldRepetition `json:"repetitions"`
	ComponentCount int               `json:"componentCount"`
	HasRepetitions bool              `json:"hasRepetitions"`
}

// FieldRepetition represents one repetition of a field (separated by ~)
type FieldRepetition struct {
	Index      int              `json:"index"`
	RawValue   string           `json:"rawValue"`
	Components []FieldComponent `json:"components"`
}

// FieldComponent represents one component of a field (separated by ^)
type FieldComponent struct {
	Index            int      `json:"index"`
	RawValue         string   `json:"rawValue"`
	Subcomponents    []string `json:"subcomponents"`
	HasSubcomponents bool     `json:"hasSubcomponents"`
}

// ParseHL7MessageEnhanced performs complete HL7 parsing with full delimiter support
// ✅ UPDATED: Now properly handles repeating segments (OBX, IN1, NK1, etc.)
func ParseHL7MessageEnhanced(rawMessage string) *BasicParsedMessage {
	// Normalize line endings
	normalizedMessage := strings.ReplaceAll(rawMessage, "\r\n", "\n")
	normalizedMessage = strings.ReplaceAll(normalizedMessage, "\r", "\n")

	// Split the HL7 message into lines (segments)
	lines := strings.Split(normalizedMessage, "\n")

	segments := make(map[string]BasicSegment)
	segmentGroups := make(map[string][]BasicSegment)  // ✅ NEW: Collect all instances
	segmentOrder := make([]string, 0)                  // ✅ NEW: Track order including repeats
	segmentCounts := make(map[string]int)              // ✅ NEW: Track index within each segment type
	messageType := "UNKNOWN"

	// Go through each line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) >= 3 {
			segmentName := line[:3]

			// Parse segment fields with enhanced HL7 structure
			fields := strings.Split(line, FieldSeparator)
			segmentFields := make(map[string]string)

			// Handle MSH segment specially
			if segmentName == "MSH" {
				// MSH.1 is the field separator "|" itself (implicit)
				segmentFields["MSH.1"] = FieldSeparator

				// MSH fields start at MSH.2
				for i := 1; i < len(fields); i++ {
					fieldKey := fmt.Sprintf("MSH.%d", i+1)
					segmentFields[fieldKey] = fields[i]
				}
			} else {
				// All other segments - 1-based indexing
				for i := 1; i < len(fields); i++ {
					fieldKey := fmt.Sprintf("%s.%d", segmentName, i)
					segmentFields[fieldKey] = fields[i]
				}
			}

			// ✅ NEW: Get segment index within its type
			segmentIndex := segmentCounts[segmentName]
			segmentCounts[segmentName]++

			basicSeg := BasicSegment{
				Name:         segmentName,
				Fields:       segmentFields,
				Raw:          line,
				SegmentIndex: segmentIndex, // ✅ NEW: Track which instance this is
			}

			// Store in both single-segment map (last wins for backwards compatibility)
			// and in segment groups array (all instances preserved)
			segments[segmentName] = basicSeg
			segmentGroups[segmentName] = append(segmentGroups[segmentName], basicSeg)
			segmentOrder = append(segmentOrder, segmentName)

			// Extract message type from MSH segment
			if segmentName == "MSH" {
				messageType = extractMessageType(line)
			}
		}
	}

	return &BasicParsedMessage{
		Raw:           rawMessage,
		MessageType:   messageType,
		Segments:      segments,
		SegmentGroups: segmentGroups, // ✅ NEW: All segment instances
		SegmentOrder:  segmentOrder,  // ✅ NEW: Order including repeats
		ParsedAt:      time.Now().Format(time.RFC3339),
	}
}

// DecodeHL7Escapes replaces standard HL7 escape sequences with their literal characters.
// encodingChars is the value of MSH.2, e.g. "^~\&".
// If empty, the HL7 standard defaults (^~\&) are assumed.
// The following sequences are decoded:
//   \F\  → | (field separator)
//   \S\  → ^ (component separator, from MSH.2[0])
//   \T\  → & (subcomponent separator, from MSH.2[3])
//   \R\  → ~ (repetition separator, from MSH.2[1])
//   \E\  → \ (escape character itself) — decoded LAST
//   \H\  → "" (start highlight — tag stripped)
//   \N\  → "" (end highlight — tag stripped)
//   \.br\ → \n (line break)
func DecodeHL7Escapes(value string, encodingChars string) string {
	if value == "" || !strings.Contains(value, EscapeCharacter) {
		return value
	}

	// Extract actual delimiter characters from MSH.2
	comp := "^"
	rep := "~"
	esc := "\\"
	sub := "&"
	if len(encodingChars) >= 1 {
		comp = string(encodingChars[0])
	}
	if len(encodingChars) >= 2 {
		rep = string(encodingChars[1])
	}
	if len(encodingChars) >= 3 {
		esc = string(encodingChars[2])
	}
	if len(encodingChars) >= 4 {
		sub = string(encodingChars[3])
	}

	result := value
	result = strings.ReplaceAll(result, `\F\`, FieldSeparator) // always "|"
	result = strings.ReplaceAll(result, `\S\`, comp)
	result = strings.ReplaceAll(result, `\T\`, sub)
	result = strings.ReplaceAll(result, `\R\`, rep)
	result = strings.ReplaceAll(result, `\.br\`, "\n")
	result = strings.ReplaceAll(result, `\H\`, "") // strip highlight start
	result = strings.ReplaceAll(result, `\N\`, "") // strip highlight end
	result = strings.ReplaceAll(result, `\E\`, esc) // MUST be last to avoid double-decode
	return result
}

// ExtractEncodingChars extracts MSH.2 (encoding characters) from a raw HL7 message.
// Returns standard defaults "^~\\&" if not found.
func ExtractEncodingChars(rawMessage string) string {
	for _, line := range strings.SplitN(strings.ReplaceAll(rawMessage, "\r", "\n"), "\n", 5) {
		if strings.HasPrefix(line, "MSH") && len(line) >= 8 {
			return line[4:8] // characters at positions 4-7 are MSH.2
		}
	}
	return `^~\&`
}

// ParseHL7Field parses a single HL7 field with all delimiters
func ParseHL7Field(fieldValue string) *EnhancedFieldData {
	if fieldValue == "" {
		return &EnhancedFieldData{
			RawValue:       "",
			Repetitions:    []FieldRepetition{},
			ComponentCount: 0,
			HasRepetitions: false,
		}
	}

	result := &EnhancedFieldData{
		RawValue:       fieldValue,
		Repetitions:    []FieldRepetition{},
		HasRepetitions: strings.Contains(fieldValue, RepetitionSeparator),
	}

	// Split by repetition separator (~)
	repetitions := strings.Split(fieldValue, RepetitionSeparator)
	maxComponents := 0

	for repIndex, repetition := range repetitions {
		if strings.TrimSpace(repetition) == "" && len(repetitions) > 1 {
			continue // Skip empty repetitions unless it's the only one
		}

		fieldRep := FieldRepetition{
			Index:      repIndex + 1,
			RawValue:   repetition,
			Components: []FieldComponent{},
		}

		// Split by component separator (^)
		components := strings.Split(repetition, ComponentSeparator)

		// Track maximum number of components across all repetitions
		if len(components) > maxComponents {
			maxComponents = len(components)
		}

		for compIndex, component := range components {
			fieldComp := FieldComponent{
				Index:            compIndex + 1,
				RawValue:         component,
				Subcomponents:    []string{},
				HasSubcomponents: strings.Contains(component, SubcomponentSeparator),
			}

			// Split by subcomponent separator (&)
			if fieldComp.HasSubcomponents {
				fieldComp.Subcomponents = strings.Split(component, SubcomponentSeparator)
			} else {
				fieldComp.Subcomponents = []string{component}
			}

			fieldRep.Components = append(fieldRep.Components, fieldComp)
		}

		result.Repetitions = append(result.Repetitions, fieldRep)
	}

	result.ComponentCount = maxComponents
	return result
}

// ConvertBasicToEnhancedWithDelimiters converts basic segments to enhanced format with proper delimiter parsing
func ConvertBasicToEnhancedWithDelimiters(basicSegments map[string]BasicSegment) []EnhancedSegment {
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

	segmentNames := getBasicHL7SegmentOrder(basicSegments)

	// Process segments in sorted order
	for _, segName := range segmentNames {
		basicSeg := basicSegments[segName]

		// Convert fields with enhanced delimiter parsing
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

			// Parse field with enhanced delimiter support
			enhancedFieldData := ParseHL7Field(fieldValue)
			subfields := convertEnhancedFieldToSubfields(enhancedFieldData, fieldKey, segName, position)

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
			description = fmt.Sprintf("%s Segment - Parsed by ezHealthKonnect Enhanced Engine", segName)
		}

		purpose := fmt.Sprintf("Processed by ezHealthKonnect high-performance engine with full delimiter support")

		enhancedSeg := EnhancedSegment{
			Key:              segName,
			Raw:              basicSeg.Raw,
			Name:             description,
			Description:      description,
			Purpose:          purpose,
			Fields:           fieldsList,
			FieldCount:       len(fieldsList),
			DictionarySource: "ezHealthKonnect_enhanced_delimiter_parser",
			Required:         (segName == "MSH" || segName == "PID"),
			Repeating:        false,
		}

		enhancedList = append(enhancedList, enhancedSeg)
	}

	return enhancedList
}

// convertEnhancedFieldToSubfields converts enhanced field data to subfield info
func convertEnhancedFieldToSubfields(enhancedField *EnhancedFieldData, fieldKey, segmentName string, position int) []SubfieldInfo {
	var subfields []SubfieldInfo

	if enhancedField == nil || len(enhancedField.Repetitions) == 0 {
		return subfields
	}

	for repIndex, repetition := range enhancedField.Repetitions {
		for compIndex, component := range repetition.Components {
			componentPosition := compIndex + 1

			// Build subfield key
			subfieldKey := fmt.Sprintf("%s.%d", fieldKey, componentPosition)
			if enhancedField.HasRepetitions {
				subfieldKey = fmt.Sprintf("%s[%d].%d", fieldKey, repIndex+1, componentPosition)
			}

			// Get component name based on field type and position
			componentName := getComponentName(segmentName, position, componentPosition)

			// Build description with subcomponent info
			description := getComponentDescription(segmentName, position, componentPosition)
			if component.HasSubcomponents {
				description += fmt.Sprintf(" (Has %d subcomponents: %s)",
					len(component.Subcomponents),
					strings.Join(component.Subcomponents, " & "))
			}

			subfield := SubfieldInfo{
				Key:         subfieldKey,
				Name:        componentName,
				DataType:    getComponentDataType(segmentName, position, componentPosition),
				Usage:       "O",
				Length:      len(component.RawValue),
				Position:    componentPosition,
				Description: description,
				HasValue:    component.RawValue != "",
				Value:       component.RawValue,
				Sequence:    compIndex,
			}

			subfields = append(subfields, subfield)
		}
	}

	return subfields
}

// demonstrateDelimiterParsing shows how the enhanced parser handles delimiters
func DemonstrateDelimiterParsing() {
	// Test cases for delimiter parsing
	testFields := []string{
		"(407)939-1289^^^theMainMouse@disney.com ^", // Your example
		"Smith^John^M^^Dr^MD",                       // Patient name with components
		"Home~Work~Mobile",                          // Repetitions
		"123&456&789^Component2",                    // Subcomponents
		"Simple field",                              // No delimiters
		"",                                          // Empty field
		"^^^",                                       // Empty components
		"Rep1~Rep2~Rep3",                            // Multiple repetitions
	}

	fmt.Println("=== Enhanced HL7 Delimiter Parsing Demonstration ===\n")

	for i, fieldValue := range testFields {
		fmt.Printf("Test %d: \"%s\"\n", i+1, fieldValue)

		enhancedField := ParseHL7Field(fieldValue)

		fmt.Printf("  Raw Value: \"%s\"\n", enhancedField.RawValue)
		fmt.Printf("  Has Repetitions: %t\n", enhancedField.HasRepetitions)
		fmt.Printf("  Component Count: %d\n", enhancedField.ComponentCount)
		fmt.Printf("  Repetitions: %d\n", len(enhancedField.Repetitions))

		for repIndex, repetition := range enhancedField.Repetitions {
			fmt.Printf("    Repetition %d: \"%s\"\n", repIndex+1, repetition.RawValue)

			for compIndex, component := range repetition.Components {
				fmt.Printf("      Component %d: \"%s\"", compIndex+1, component.RawValue)
				if component.HasSubcomponents {
					fmt.Printf(" [Subcomponents: %s]", strings.Join(component.Subcomponents, " & "))
				}
				fmt.Println()
			}
		}
		fmt.Println()
	}
}

// Helper functions (keeping existing ones and adding new ones as needed)

// extractFieldPosition - keeping the existing implementation
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

// TestSpecificExample tests your specific delimiter example
func TestSpecificExample() {
	example := "(407)939-1289^^^theMainMouse@disney.com ^"

	fmt.Printf("=== Testing Your Specific Example ===\n")
	fmt.Printf("Input: \"%s\"\n\n", example)

	parsed := ParseHL7Field(example)

	fmt.Printf("Parsing Results:\n")
	fmt.Printf("- Raw Value: \"%s\"\n", parsed.RawValue)
	fmt.Printf("- Has Repetitions: %t\n", parsed.HasRepetitions)
	fmt.Printf("- Component Count: %d\n", parsed.ComponentCount)
	fmt.Printf("- Number of Repetitions: %d\n\n", len(parsed.Repetitions))

	if len(parsed.Repetitions) > 0 {
		rep := parsed.Repetitions[0]
		fmt.Printf("Repetition 1 Components:\n")

		for i, comp := range rep.Components {
			fmt.Printf("  Component %d: \"%s\"", i+1, comp.RawValue)
			if comp.RawValue == "" {
				fmt.Printf(" (empty)")
			}
			if comp.HasSubcomponents && len(comp.Subcomponents) > 1 {
				fmt.Printf(" [Subcomponents: %s]", strings.Join(comp.Subcomponents, " & "))
			}
			fmt.Println()
		}
	}

	fmt.Printf("\nThis would be interpreted as:\n")
	fmt.Printf("- Component 1: Phone number \"(407)939-1289\"\n")
	fmt.Printf("- Component 2: Empty (reserved/unused)\n")
	fmt.Printf("- Component 3: Empty (reserved/unused)\n")
	fmt.Printf("- Component 4: Empty (reserved/unused)\n")
	fmt.Printf("- Component 5: Email \"theMainMouse@disney.com \"\n")
}

// Keep existing helper functions from original parser.go:
// getFieldName, getFieldDescription, getFieldDataType, getFieldOptionality,
// getComponentName, getComponentDataType, getComponentDescription,
// getBasicHL7SegmentOrder, extractMessageType

func extractMessageType(mshSegment string) string {
	fields := strings.Split(mshSegment, "|")
	// MSH.9 is at index 8 (since MSH.1 is implicit)
	if len(fields) > 8 {
		return fields[8]
	}
	return "UNKNOWN"
}

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
			4: "Alternate Patient ID", 5: "Patient Name", 6: "Mother's Maiden Name",
			7: "Date/Time of Birth", 8: "Administrative Sex", 9: "Patient Alias",
			10: "Race", 11: "Patient Address", 12: "County Code",
			13: "Phone Number - Home", 14: "Phone Number - Business",
			15: "Primary Language", 16: "Marital Status", 17: "Religion",
			18: "Patient Account Number", 19: "SSN Number - Patient",
			20: "Driver's License Number", 21: "Mother's Identifier",
			22: "Ethnic Group", 23: "Birth Place", 24: "Multiple Birth Indicator",
			25: "Birth Order", 26: "Citizenship", 27: "Veterans Military Status",
			29: "Patient Death Date and Time", 30: "Patient Death Indicator",
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
	}

	if segFields, exists := descriptions[segmentName]; exists {
		if desc, exists := segFields[position]; exists {
			return desc
		}
	}

	return fmt.Sprintf("%s field %d", segmentName, position)
}

func getFieldDataType(segmentName string, position int) string {
	dataTypes := map[string]map[int]string{
		"MSH": {3: "HD", 4: "HD", 7: "TS", 9: "MSG", 10: "ST", 11: "PT", 12: "VID"},
		"PID": {3: "CX", 5: "XPN", 7: "TS", 8: "IS", 11: "XAD", 13: "XTN", 14: "XTN"},
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
			13: { // Phone Number (XTN)
				1: "Telephone Number",
				2: "Telecommunication Use Code",
				3: "Telecommunication Equipment Type",
				4: "Email Address",
			},
			14: { // Business Phone (XTN)
				1: "Telephone Number",
				2: "Telecommunication Use Code",
				3: "Telecommunication Equipment Type",
				4: "Email Address",
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

func getComponentDataType(segmentName string, fieldPosition, componentPosition int) string {
	dataTypeMap := map[string]map[int]map[int]string{
		"PID": {
			5:  {1: "FN", 2: "ST", 3: "ST", 4: "ST", 5: "ST", 6: "ST"},
			7:  {1: "TS"},
			13: {1: "ST", 2: "ID", 3: "ID", 4: "ST"},
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
			13: {
				1: "Telephone number (with area code)",
				2: "Telecommunication use code (PRN=Primary, WPN=Work, etc.)",
				3: "Equipment type (PH=Phone, CP=Cell, etc.)",
				4: "Email address",
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
