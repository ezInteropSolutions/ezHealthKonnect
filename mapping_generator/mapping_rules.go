// mapping_generator/mapping_rules.go
// Core mapping rules engine for HL7-FHIR transformations
// Provides semantic mapping patterns and transformation logic

package mapping_generator

import (
	"fmt"
	"regexp"
	"strings"
)

// =====================================
// Core Mapping Pattern Structures
// =====================================

type MappingPattern struct {
	HL7Pattern     string            `json:"hl7Pattern"`     // "PID.5.{1,2}"
	FHIRPath       string            `json:"fhirPath"`       // "Patient.name[0].family"
	Transform      string            `json:"transform"`      // "direct", "gender_code", etc.
	Condition      string            `json:"condition"`      // Optional: "PV1.2 == 'I'"
	Required       bool              `json:"required"`
	Priority       int               `json:"priority"`
	Confidence     float64           `json:"confidence"`     // 0.0 - 1.0
	ValueMap       map[string]string `json:"valueMap"`
	MessageTypes   []string          `json:"messageTypes"`   // Applicable message families
	Description    string            `json:"description"`
}

type MappingRuleEngine struct {
	coreMappings       []MappingPattern
	transformFunctions map[string]TransformFunction
	valueSets          map[string]map[string]string
}

// =====================================
// Core Mapping Patterns Database
// =====================================

func NewMappingRuleEngine() *MappingRuleEngine {
	engine := &MappingRuleEngine{
		coreMappings:       make([]MappingPattern, 0),
		transformFunctions: make(map[string]TransformFunction),
		valueSets:          make(map[string]map[string]string),
	}

	engine.initializeCoreMappings()
	engine.initializeTransformFunctions()
	engine.initializeValueSets()

	return engine
}

func (mre *MappingRuleEngine) initializeCoreMappings() {
	// =====================================
	// Patient Demographics (Universal)
	// =====================================
	mre.coreMappings = append(mre.coreMappings, []MappingPattern{
		{
			HL7Pattern:   "PID.3.1",
			FHIRPath:     "Patient.identifier[0].value",
			Transform:    "string_direct",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient identifier (MRN)",
		},
		{
			HL7Pattern:   "PID.3.4",
			FHIRPath:     "Patient.identifier[0].system",
			Transform:    "identifier_system",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient identifier system",
		},
		{
			HL7Pattern:   "PID.5.1",
			FHIRPath:     "Patient.name[0].family",
			Transform:    "name_component",
			Required:     true,
			Priority:     1,
			Confidence:   0.98,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient family name",
		},
		{
			HL7Pattern:   "PID.5.2",
			FHIRPath:     "Patient.name[0].given[0]",
			Transform:    "name_component",
			Required:     false,
			Priority:     2,
			Confidence:   0.98,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient given name (first)",
		},
		{
			HL7Pattern:   "PID.5.3",
			FHIRPath:     "Patient.name[0].given[1]",
			Transform:    "name_component",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient middle name",
		},
		{
			HL7Pattern:   "PID.7",
			FHIRPath:     "Patient.birthDate",
			Transform:    "hl7_date_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.99,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient birth date",
		},
		{
			HL7Pattern:   "PID.8",
			FHIRPath:     "Patient.gender",
			Transform:    "hl7_table_0001_gender",
			Required:     false,
			Priority:     2,
			Confidence:   0.99,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient administrative gender",
			ValueMap: map[string]string{
				"M": "male",
				"F": "female",
				"U": "unknown",
				"O": "other",
				"A": "other",
				"N": "unknown",
			},
		},
		{
			HL7Pattern:   "PID.11.1",
			FHIRPath:     "Patient.address[0].line[0]",
			Transform:    "address_component",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient address street",
		},
		{
			HL7Pattern:   "PID.11.3",
			FHIRPath:     "Patient.address[0].city",
			Transform:    "address_component",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient address city",
		},
		{
			HL7Pattern:   "PID.11.4",
			FHIRPath:     "Patient.address[0].state",
			Transform:    "address_component",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient address state",
		},
		{
			HL7Pattern:   "PID.11.5",
			FHIRPath:     "Patient.address[0].postalCode",
			Transform:    "address_component",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient address postal code",
		},
		{
			HL7Pattern:   "PID.13.1",
			FHIRPath:     "Patient.telecom[0].value",
			Transform:    "telecom_value",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Patient phone number",
		},

		// =====================================
		// Encounter (ADT, ORU, ORM, DFT)
		// =====================================
		{
			HL7Pattern:   "PV1.2",
			FHIRPath:     "Encounter.class.code",
			Transform:    "hl7_table_0004_patient_class",
			Required:     false,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ADT", "ORU", "ORM", "DFT"},
			Description:  "Patient class (encounter type)",
			ValueMap: map[string]string{
				"I": "IMP",     // Inpatient
				"O": "AMB",     // Outpatient
				"E": "EMER",    // Emergency
				"P": "PRENC",   // Pre-admission
				"R": "OBSENC",  // Recurring patient
				"B": "OBSENC",  // Obstetrics
				"N": "AMB",     // Not applicable
			},
		},
		{
			HL7Pattern:   "PV1.19",
			FHIRPath:     "Encounter.identifier[0].value",
			Transform:    "string_direct",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "DFT"},
			Description:  "Visit number",
		},
		{
			HL7Pattern:   "PV1.3.1",
			FHIRPath:     "Encounter.location[0].location.display",
			Transform:    "location_mapping",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ADT", "ORU", "ORM", "DFT"},
			Description:  "Patient location - point of care",
		},
		{
			HL7Pattern:   "PV1.44",
			FHIRPath:     "Encounter.period.start",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "DFT"},
			Description:  "Admit date/time",
		},
		{
			HL7Pattern:   "PV1.45",
			FHIRPath:     "Encounter.period.end",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ADT"},
			Description:  "Discharge date/time",
		},

		// =====================================
		// Observation (ORU)
		// =====================================
		{
			HL7Pattern:   "OBX.3.1",
			FHIRPath:     "Observation.code.coding[0].code",
			Transform:    "coded_element_code",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ORU"},
			Description:  "Observation identifier code",
		},
		{
			HL7Pattern:   "OBX.3.2",
			FHIRPath:     "Observation.code.coding[0].display",
			Transform:    "coded_element_display",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ORU"},
			Description:  "Observation identifier text",
		},
		{
			HL7Pattern:   "OBX.3.3",
			FHIRPath:     "Observation.code.coding[0].system",
			Transform:    "coding_system_mapping",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ORU"},
			Description:  "Observation coding system",
		},
		{
			HL7Pattern:   "OBX.5",
			FHIRPath:     "Observation.valueQuantity.value",
			Transform:    "observation_value",
			Required:     false,
			Priority:     1,
			Confidence:   0.95,
			MessageTypes: []string{"ORU"},
			Description:  "Observation value",
		},
		{
			HL7Pattern:   "OBX.6.1",
			FHIRPath:     "Observation.valueQuantity.unit",
			Transform:    "unit_mapping",
			Required:     false,
			Priority:     2,
			Confidence:   0.90,
			MessageTypes: []string{"ORU"},
			Description:  "Observation units",
		},
		{
			HL7Pattern:   "OBX.7",
			FHIRPath:     "Observation.referenceRange[0].text",
			Transform:    "reference_range",
			Required:     false,
			Priority:     3,
			Confidence:   0.85,
			MessageTypes: []string{"ORU"},
			Description:  "Reference range",
		},
		{
			HL7Pattern:   "OBX.8",
			FHIRPath:     "Observation.interpretation[0].coding[0].code",
			Transform:    "hl7_table_0078_abnormal_flags",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ORU"},
			Description:  "Abnormal flags",
			ValueMap: map[string]string{
				"L":  "L",  // Low
				"H":  "H",  // High
				"LL": "LL", // Critical low
				"HH": "HH", // Critical high
				"N":  "N",  // Normal
				"A":  "A",  // Abnormal
			},
		},
		{
			HL7Pattern:   "OBX.11",
			FHIRPath:     "Observation.status",
			Transform:    "hl7_table_0085_observation_result_status",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ORU"},
			Description:  "Observation result status",
			ValueMap: map[string]string{
				"F": "final",
				"P": "preliminary",
				"C": "corrected",
				"D": "cancelled",
				"I": "registered",
				"N": "final",
				"O": "unknown",
				"R": "registered",
				"S": "preliminary",
				"X": "cancelled",
				"U": "unknown",
				"W": "preliminary",
			},
		},
		{
			HL7Pattern:   "OBX.14",
			FHIRPath:     "Observation.effectiveDateTime",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ORU"},
			Description:  "Date/time of observation",
		},

		// =====================================
		// ServiceRequest (ORM)
		// =====================================
		{
			HL7Pattern:   "ORC.1",
			FHIRPath:     "ServiceRequest.status",
			Transform:    "hl7_table_0119_order_control_codes",
			Required:     true,
			Priority:     1,
			Confidence:   0.95,
			MessageTypes: []string{"ORM"},
			Description:  "Order control",
			ValueMap: map[string]string{
				"NW": "active",
				"OK": "active",
				"UA": "on-hold",
				"CA": "cancelled",
				"DC": "cancelled",
				"CR": "draft",
				"SC": "active",
				"SN": "active",
				"XO": "cancelled",
				"XX": "entered-in-error",
			},
		},
		{
			HL7Pattern:   "ORC.2",
			FHIRPath:     "ServiceRequest.identifier[0].value",
			Transform:    "string_direct",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ORM"},
			Description:  "Placer order number",
		},
		{
			HL7Pattern:   "ORC.3",
			FHIRPath:     "ServiceRequest.identifier[1].value",
			Transform:    "string_direct",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"ORM"},
			Description:  "Filler order number",
		},
		{
			HL7Pattern:   "OBR.4.1",
			FHIRPath:     "ServiceRequest.code.coding[0].code",
			Transform:    "coded_element_code",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ORM"},
			Description:  "Universal service identifier code",
		},
		{
			HL7Pattern:   "OBR.4.2",
			FHIRPath:     "ServiceRequest.code.coding[0].display",
			Transform:    "coded_element_display",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ORM"},
			Description:  "Universal service identifier text",
		},
		{
			HL7Pattern:   "OBR.6",
			FHIRPath:     "ServiceRequest.occurrenceDateTime",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ORM"},
			Description:  "Requested date/time",
		},
		{
			HL7Pattern:   "OBR.7",
			FHIRPath:     "ServiceRequest.occurrenceDateTime",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ORM"},
			Description:  "Observation date/time (if no OBR.6)",
		},

		// =====================================
		// DocumentReference (MDM)
		// =====================================
		{
			HL7Pattern:   "TXA.12.1",
			FHIRPath:     "DocumentReference.type.coding[0].code",
			Transform:    "coded_element_code",
			Required:     true,
			Priority:     1,
			Confidence:   0.95,
			MessageTypes: []string{"MDM"},
			Description:  "Document type code",
		},
		{
			HL7Pattern:   "TXA.12.2",
			FHIRPath:     "DocumentReference.type.coding[0].display",
			Transform:    "coded_element_display",
			Required:     false,
			Priority:     2,
			Confidence:   0.90,
			MessageTypes: []string{"MDM"},
			Description:  "Document type display",
		},
		{
			HL7Pattern:   "TXA.4",
			FHIRPath:     "DocumentReference.date",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"MDM"},
			Description:  "Activity date/time",
		},
		{
			HL7Pattern:   "TXA.17",
			FHIRPath:     "DocumentReference.status",
			Transform:    "hl7_table_0271_document_completion_status",
			Required:     true,
			Priority:     1,
			Confidence:   0.95,
			MessageTypes: []string{"MDM"},
			Description:  "Document completion status",
			ValueMap: map[string]string{
				"DI": "preliminary",
				"DO": "final",
				"IP": "preliminary",
				"IN": "preliminary",
				"PA": "final",
				"AU": "final",
				"LA": "final",
			},
		},

		// =====================================
		// Appointment (SIU)
		// =====================================
		{
			HL7Pattern:   "SCH.1.1",
			FHIRPath:     "Appointment.identifier[0].value",
			Transform:    "string_direct",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"SIU"},
			Description:  "Placer appointment ID",
		},
		{
			HL7Pattern:   "SCH.2.1",
			FHIRPath:     "Appointment.identifier[1].value",
			Transform:    "string_direct",
			Required:     false,
			Priority:     3,
			Confidence:   0.95,
			MessageTypes: []string{"SIU"},
			Description:  "Filler appointment ID",
		},
		{
			HL7Pattern:   "SCH.11.4",
			FHIRPath:     "Appointment.start",
			Transform:    "hl7_datetime_to_fhir",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"SIU"},
			Description:  "Appointment start date/time",
		},
		{
			HL7Pattern:   "SCH.11.5",
			FHIRPath:     "Appointment.end",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"SIU"},
			Description:  "Appointment end date/time",
		},
		{
			HL7Pattern:   "SCH.25",
			FHIRPath:     "Appointment.status",
			Transform:    "hl7_table_0278_filler_status_codes",
			Required:     true,
			Priority:     1,
			Confidence:   0.95,
			MessageTypes: []string{"SIU"},
			Description:  "Filler status code",
			ValueMap: map[string]string{
				"Booked":    "booked",
				"Pending":   "pending",
				"Complete":  "fulfilled",
				"Cancelled": "cancelled",
				"Arrived":   "arrived",
				"Checked In": "checked-in",
				"Started":   "checked-in",
				"Noshow":    "noshow",
			},
		},

		// =====================================
		// MedicationRequest (RDE/RXE)
		// =====================================
		{
			HL7Pattern:   "RXE.2.1",
			FHIRPath:     "MedicationRequest.medicationCodeableConcept.coding[0].code",
			Transform:    "coded_element_code",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"RDE", "RXE"},
			Description:  "Give code",
		},
		{
			HL7Pattern:   "RXE.2.2",
			FHIRPath:     "MedicationRequest.medicationCodeableConcept.coding[0].display",
			Transform:    "coded_element_display",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"RDE", "RXE"},
			Description:  "Give code text",
		},
		{
			HL7Pattern:   "RXE.3",
			FHIRPath:     "MedicationRequest.dosageInstruction[0].doseAndRate[0].doseQuantity.value",
			Transform:    "numeric_value",
			Required:     false,
			Priority:     2,
			Confidence:   0.90,
			MessageTypes: []string{"RDE", "RXE"},
			Description:  "Give amount - minimum",
		},
		{
			HL7Pattern:   "RXE.5.1",
			FHIRPath:     "MedicationRequest.dosageInstruction[0].doseAndRate[0].doseQuantity.unit",
			Transform:    "unit_mapping",
			Required:     false,
			Priority:     3,
			Confidence:   0.85,
			MessageTypes: []string{"RDE", "RXE"},
			Description:  "Give units",
		},

		// =====================================
		// MessageHeader (Universal)
		// =====================================
		{
			HL7Pattern:   "MSH.3",
			FHIRPath:     "MessageHeader.source.name",
			Transform:    "string_direct",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Sending application",
		},
		{
			HL7Pattern:   "MSH.4",
			FHIRPath:     "MessageHeader.source.software",
			Transform:    "string_direct",
			Required:     false,
			Priority:     3,
			Confidence:   0.85,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Sending facility",
		},
		{
			HL7Pattern:   "MSH.9.1",
			FHIRPath:     "MessageHeader.eventCoding.code",
			Transform:    "string_direct",
			Required:     true,
			Priority:     1,
			Confidence:   0.99,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Message type",
		},
		{
			HL7Pattern:   "MSH.9.2",
			FHIRPath:     "MessageHeader.eventCoding.display",
			Transform:    "string_direct",
			Required:     false,
			Priority:     2,
			Confidence:   0.95,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Trigger event",
		},
		{
			HL7Pattern:   "MSH.7",
			FHIRPath:     "MessageHeader.meta.lastUpdated",
			Transform:    "hl7_datetime_to_fhir",
			Required:     false,
			Priority:     3,
			Confidence:   0.90,
			MessageTypes: []string{"ADT", "ORU", "ORM", "MDM", "SIU", "DFT", "RDE"},
			Description:  "Date/time of message",
		},
	}...)
}

func (mre *MappingRuleEngine) initializeTransformFunctions() {
	mre.transformFunctions = map[string]TransformFunction{
		"string_direct": {
			Name:        "string_direct",
			Type:        "direct",
			Description: "Direct string copy with null/empty validation",
			Validation:  "non_empty",
			Parameters:  map[string]string{"trim": "true"},
		},
		"name_component": {
			Name:        "name_component",
			Type:        "string_transform",
			Description: "Name component with proper case handling",
			Validation:  "non_empty",
			Parameters:  map[string]string{"case": "proper", "trim": "true"},
		},
		"hl7_date_to_fhir": {
			Name:        "hl7_date_to_fhir",
			Type:        "date_conversion",
			Description: "Convert HL7 date (YYYYMMDD) to FHIR date",
			Validation:  "valid_date",
			Parameters:  map[string]string{"input_format": "YYYYMMDD", "output_format": "YYYY-MM-DD"},
		},
		"hl7_datetime_to_fhir": {
			Name:        "hl7_datetime_to_fhir",
			Type:        "datetime_conversion",
			Description: "Convert HL7 timestamp to FHIR instant",
			Validation:  "valid_datetime",
			Parameters:  map[string]string{"input_format": "YYYYMMDDHHMMSS", "output_format": "YYYY-MM-DDTHH:MM:SSZ"},
		},
		"hl7_table_0001_gender": {
			Name:        "hl7_table_0001_gender",
			Type:        "valueset",
			Description: "HL7 Table 0001 - Administrative Sex",
			Validation:  "known_value",
			Parameters:  map[string]string{"table": "0001", "default": "unknown"},
		},
		"hl7_table_0004_patient_class": {
			Name:        "hl7_table_0004_patient_class",
			Type:        "valueset",
			Description: "HL7 Table 0004 - Patient Class",
			Validation:  "known_value",
			Parameters:  map[string]string{"table": "0004", "default": "AMB"},
		},
		"coded_element_code": {
			Name:        "coded_element_code",
			Type:        "coded_element",
			Description: "Extract code from coded element",
			Validation:  "non_empty",
			Parameters:  map[string]string{"component": "code"},
		},
		"coded_element_display": {
			Name:        "coded_element_display",
			Type:        "coded_element",
			Description: "Extract display text from coded element",
			Validation:  "non_empty",
			Parameters:  map[string]string{"component": "display"},
		},
		"observation_value": {
			Name:        "observation_value",
			Type:        "multi_type",
			Description: "Handle various observation value types",
			Validation:  "type_appropriate",
			Parameters:  map[string]string{"types": "numeric,string,datetime"},
		},
		"numeric_value": {
			Name:        "numeric_value",
			Type:        "numeric_conversion",
			Description: "Convert to numeric value",
			Validation:  "valid_number",
			Parameters:  map[string]string{"precision": "auto"},
		},
		"unit_mapping": {
			Name:        "unit_mapping",
			Type:        "unit_conversion",
			Description: "Map HL7 units to UCUM",
			Validation:  "known_unit",
			Parameters:  map[string]string{"target_system": "UCUM"},
		},
		"address_component": {
			Name:        "address_component",
			Type:        "string_transform",
			Description: "Address component with case handling",
			Validation:  "non_empty",
			Parameters:  map[string]string{"case": "proper", "trim": "true"},
		},
		"telecom_value": {
			Name:        "telecom_value",
			Type:        "string_transform",
			Description: "Telecom value with format validation",
			Validation:  "valid_phone_email",
			Parameters:  map[string]string{"format": "normalize"},
		},
		"identifier_system": {
			Name:        "identifier_system",
			Type:        "system_mapping",
			Description: "Map identifier authority to system URI",
			Validation:  "known_authority",
			Parameters:  map[string]string{"type": "identifier"},
		},
	}
}

func (mre *MappingRuleEngine) initializeValueSets() {
	mre.valueSets = map[string]map[string]string{
		"hl7_table_0001": { // Administrative Sex
			"M": "male",
			"F": "female",
			"O": "other",
			"U": "unknown",
			"A": "other",
			"N": "unknown",
		},
		"hl7_table_0004": { // Patient Class
			"I": "IMP",     // Inpatient
			"O": "AMB",     // Outpatient
			"E": "EMER",    // Emergency
			"P": "PRENC",   // Pre-admission
			"R": "OBSENC",  // Recurring
			"B": "OBSENC",  // Obstetrics
			"N": "AMB",     // Not applicable
		},
		"hl7_table_0085": { // Observation Result Status
			"F": "final",
			"P": "preliminary",
			"C": "corrected",
			"D": "cancelled",
			"I": "registered",
			"N": "final",
			"O": "unknown",
			"R": "registered",
			"S": "preliminary",
			"X": "cancelled",
			"U": "unknown",
			"W": "preliminary",
		},
		"hl7_table_0119": { // Order Control Codes
			"NW": "active",    // New order
			"OK": "active",    // Order accepted & OK
			"UA": "on-hold",   // Unable to accept order/service
			"CA": "cancelled", // Cancel order/service request
			"DC": "cancelled", // Discontinue order/service request
			"CR": "draft",     // Canceled as requested
			"SC": "active",    // Status changed
			"SN": "active",    // Send order/service number
			"XO": "cancelled", // Change order/service request
			"XX": "entered-in-error", // Order/service changed, unsolicited
		},
	}
}

// =====================================
// Mapping Generation Methods
// =====================================

func (mre *MappingRuleEngine) GetMappingsForMessageType(messageType string) []MappingPattern {
	var mappings []MappingPattern

	messageFamily := mre.extractMessageFamily(messageType)

	for _, pattern := range mre.coreMappings {
		if mre.isApplicableToMessageType(pattern, messageFamily) {
			mappings = append(mappings, pattern)
		}
	}

	return mappings
}

func (mre *MappingRuleEngine) extractMessageFamily(messageType string) string {
	// Extract family from messageType (e.g., "ADT^A01" -> "ADT")
	parts := strings.Split(messageType, "^")
	if len(parts) > 0 {
		return parts[0]
	}
	return messageType
}

func (mre *MappingRuleEngine) isApplicableToMessageType(pattern MappingPattern, messageFamily string) bool {
	for _, mt := range pattern.MessageTypes {
		if mt == messageFamily {
			return true
		}
	}
	return false
}

func (mre *MappingRuleEngine) GetFieldMappingsForResource(messageType, resourceType string) []FieldMapping {
	var fieldMappings []FieldMapping

	mappings := mre.GetMappingsForMessageType(messageType)

	for _, pattern := range mappings {
		if mre.isTargetResource(pattern.FHIRPath, resourceType) {
			fieldMappings = append(fieldMappings, FieldMapping{
				HL7Path:    pattern.HL7Pattern,
				FHIRPath:   pattern.FHIRPath,
				Transform:  pattern.Transform,
				Required:   pattern.Required,
				Confidence: pattern.Confidence,
				ValueMap:   pattern.ValueMap,
			})
		}
	}

	return fieldMappings
}

func (mre *MappingRuleEngine) isTargetResource(fhirPath, resourceType string) bool {
	return strings.HasPrefix(fhirPath, resourceType+".")
}

// =====================================
// Pattern Matching and Validation
// =====================================

func (mre *MappingRuleEngine) ValidateMapping(pattern MappingPattern) error {
	// Validate HL7 path format
	if !mre.isValidHL7Path(pattern.HL7Pattern) {
		return fmt.Errorf("invalid HL7 path format: %s", pattern.HL7Pattern)
	}

	// Validate FHIR path format
	if !mre.isValidFHIRPath(pattern.FHIRPath) {
		return fmt.Errorf("invalid FHIR path format: %s", pattern.FHIRPath)
	}

	// Validate transform function exists
	if _, exists := mre.transformFunctions[pattern.Transform]; !exists {
		return fmt.Errorf("unknown transform function: %s", pattern.Transform)
	}

	// Validate confidence range
	if pattern.Confidence < 0.0 || pattern.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got: %f", pattern.Confidence)
	}

	return nil
}

func (mre *MappingRuleEngine) isValidHL7Path(path string) bool {
	// Basic HL7 path validation (e.g., "PID.5.1", "OBX.3")
	pattern := regexp.MustCompile(`^[A-Z]{2,3}(\.[0-9]+)*$`)
	return pattern.MatchString(path)
}

func (mre *MappingRuleEngine) isValidFHIRPath(path string) bool {
	// Basic FHIR path validation (e.g., "Patient.name[0].family")
	pattern := regexp.MustCompile(`^[A-Z][a-zA-Z]+(\.[a-zA-Z]+(\[[0-9]+\])?)*$`)
	return pattern.MatchString(path)
}

func (mre *MappingRuleEngine) GetTransformFunction(name string) (TransformFunction, bool) {
	transform, exists := mre.transformFunctions[name]
	return transform, exists
}

func (mre *MappingRuleEngine) GetValueSet(name string) (map[string]string, bool) {
	valueSet, exists := mre.valueSets[name]
	return valueSet, exists
}

func (mre *MappingRuleEngine) GetSupportedMessageTypes() []string {
	typeMap := make(map[string]bool)
	for _, pattern := range mre.coreMappings {
		for _, msgType := range pattern.MessageTypes {
			typeMap[msgType] = true
		}
	}

	var types []string
	for msgType := range typeMap {
		types = append(types, msgType)
	}

	return types
}