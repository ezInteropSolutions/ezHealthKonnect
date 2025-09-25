// services/transformation_fhir_extended.go
// Extended FHIR Transformation Service Methods
//
// 🎯 PURPOSE: Extended resource creation, validation, and utility methods for FHIR transformation
package services

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// SOURCE DATA ANALYSIS
// =====================================

// ResourceMapping represents mapping from source to FHIR resource
type ResourceMapping struct {
	ResourceType     string                 `json:"resourceType"`
	Profile          string                 `json:"profile,omitempty"`
	SourcePaths      []string               `json:"sourcePaths"`
	MappingRules     []FieldMappingRule     `json:"mappingRules"`
	RequiredFields   []string               `json:"requiredFields"`
	OptionalFields   []string               `json:"optionalFields"`
	Extensions       []ExtensionMapping     `json:"extensions,omitempty"`
	References       []ReferenceMapping     `json:"references,omitempty"`
	Priority         int                    `json:"priority"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type FieldMappingRule struct {
	SourcePath      string                 `json:"sourcePath"`
	TargetPath      string                 `json:"targetPath"`
	DataType        string                 `json:"dataType"`
	Transform       string                 `json:"transform,omitempty"`
	DefaultValue    interface{}            `json:"defaultValue,omitempty"`
	Condition       string                 `json:"condition,omitempty"`
	Validation      []ValidationRule       `json:"validation,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type ExtensionMapping struct {
	URL         string      `json:"url"`
	SourcePath  string      `json:"sourcePath"`
	ValueType   string      `json:"valueType"`
	Transform   string      `json:"transform,omitempty"`
	Required    bool        `json:"required"`
}

type ReferenceMapping struct {
	SourcePath     string `json:"sourcePath"`
	TargetPath     string `json:"targetPath"`
	ReferenceType  string `json:"referenceType"`
	IdentifierPath string `json:"identifierPath,omitempty"`
	DisplayPath    string `json:"displayPath,omitempty"`
}

// analyzeSourceData analyzes source data to determine FHIR resource mappings
func (s *FHIRTransformationService) analyzeSourceData(sourceData map[string]interface{}, sourceFormat MessageType) ([]ResourceMapping, error) {
	var mappings []ResourceMapping

	switch sourceFormat {
	case MessageTypeHL7:
		return s.analyzeHL7Data(sourceData)
	case MessageTypeJSON:
		return s.analyzeJSONData(sourceData)
	case MessageTypeXML:
		return s.analyzeXMLData(sourceData)
	default:
		return nil, fmt.Errorf("unsupported source format: %s", sourceFormat)
	}
}

// analyzeHL7Data analyzes HL7 parsed data
func (s *FHIRTransformationService) analyzeHL7Data(sourceData map[string]interface{}) ([]ResourceMapping, error) {
	var mappings []ResourceMapping

	// Extract message type from source data
	messageType := s.extractHL7MessageType(sourceData)

	// Always create MessageHeader for HL7 messages
	messageHeaderMapping := ResourceMapping{
		ResourceType: "MessageHeader",
		Profile:      "base",
		SourcePaths:  []string{"messageHeader", "segments.MSH"},
		Priority:     1,
		MappingRules: []FieldMappingRule{
			{
				SourcePath: "messageHeader.messageType.messageCode",
				TargetPath: "eventCoding.code",
				DataType:   "code",
				Transform:  "hl7_message_type_to_coding",
			},
			{
				SourcePath: "messageHeader.sendingApplication",
				TargetPath: "source.name",
				DataType:   "string",
			},
			{
				SourcePath: "messageHeader.sendingFacility",
				TargetPath: "source.endpoint",
				DataType:   "uri",
				Transform:  "facility_to_endpoint",
			},
			{
				SourcePath: "messageHeader.messageControlID",
				TargetPath: "id",
				DataType:   "id",
			},
		},
		RequiredFields: []string{"eventCoding", "source"},
		Metadata: map[string]interface{}{
			"sourceSegment": "MSH",
			"priority":      "high",
		},
	}
	mappings = append(mappings, messageHeaderMapping)

	// Check for Patient information (PID segment)
	if s.hasSegment(sourceData, "PID") {
		patientMapping := ResourceMapping{
			ResourceType: "Patient",
			Profile:      "base",
			SourcePaths:  []string{"segmentGroups.PID"},
			Priority:     2,
			MappingRules: []FieldMappingRule{
				{
					SourcePath: "segments.PID.fields[2].value", // PID.3 - Patient Identifier
					TargetPath: "identifier",
					DataType:   "Identifier",
					Transform:  "cx_to_identifier",
				},
				{
					SourcePath: "segments.PID.fields[4].value", // PID.5 - Patient Name
					TargetPath: "name",
					DataType:   "HumanName",
					Transform:  "xpn_to_humanname",
				},
				{
					SourcePath: "segments.PID.fields[6].value", // PID.7 - Date of Birth
					TargetPath: "birthDate",
					DataType:   "date",
					Transform:  "ts_to_date",
				},
				{
					SourcePath: "segments.PID.fields[7].value", // PID.8 - Administrative Sex
					TargetPath: "gender",
					DataType:   "code",
					Transform:  "administrative_sex_to_gender",
				},
				{
					SourcePath: "segments.PID.fields[10].value", // PID.11 - Patient Address
					TargetPath: "address",
					DataType:   "Address",
					Transform:  "xad_to_address",
				},
			},
			RequiredFields: []string{"identifier"},
			Metadata: map[string]interface{}{
				"sourceSegment": "PID",
				"priority":      "high",
			},
		}
		mappings = append(mappings, patientMapping)
	}

	// Check for Organization information (based on sending/receiving facilities)
	if s.hasOrganizationData(sourceData) {
		orgMapping := ResourceMapping{
			ResourceType: "Organization",
			Profile:      "base",
			SourcePaths:  []string{"messageHeader.sendingFacility", "messageHeader.receivingFacility"},
			Priority:     3,
			MappingRules: []FieldMappingRule{
				{
					SourcePath: "messageHeader.sendingFacility",
					TargetPath: "name",
					DataType:   "string",
				},
				{
					SourcePath: "messageHeader.sendingFacility",
					TargetPath: "identifier.value",
					DataType:   "string",
					Transform:  "facility_to_identifier",
				},
			},
			RequiredFields: []string{"name"},
			Metadata: map[string]interface{}{
				"sourceField": "MSH.4",
				"priority":    "medium",
			},
		}
		mappings = append(mappings, orgMapping)
	}

	// Check for Encounter information (PV1 segment)
	if s.hasSegment(sourceData, "PV1") {
		encounterMapping := ResourceMapping{
			ResourceType: "Encounter",
			Profile:      "base",
			SourcePaths:  []string{"segmentGroups.PV1"},
			Priority:     4,
			MappingRules: []FieldMappingRule{
				{
					SourcePath: "segments.PV1.fields[1].value", // PV1.2 - Patient Class
					TargetPath: "class.code",
					DataType:   "code",
					Transform:  "patient_class_to_encounter_class",
				},
				{
					SourcePath: "segments.PV1.fields[2].value", // PV1.3 - Assigned Patient Location
					TargetPath: "location.location.display",
					DataType:   "string",
					Transform:  "pl_to_location",
				},
				{
					SourcePath: "segments.PV1.fields[18].value", // PV1.19 - Visit Number
					TargetPath: "identifier.value",
					DataType:   "string",
				},
			},
			RequiredFields: []string{"status", "class"},
			References: []ReferenceMapping{
				{
					SourcePath:    "segments.PID.fields[2].value",
					TargetPath:    "subject",
					ReferenceType: "Patient",
				},
			},
			Metadata: map[string]interface{}{
				"sourceSegment": "PV1",
				"priority":      "medium",
			},
		}
		mappings = append(mappings, encounterMapping)
	}

	// Check for Observation information (OBX segments)
	if s.hasSegment(sourceData, "OBX") {
		observationMapping := ResourceMapping{
			ResourceType: "Observation",
			Profile:      "base",
			SourcePaths:  []string{"segmentGroups.OBX"},
			Priority:     5,
			MappingRules: []FieldMappingRule{
				{
					SourcePath: "segments.OBX.fields[2].value", // OBX.3 - Observation Identifier
					TargetPath: "code",
					DataType:   "CodeableConcept",
					Transform:  "ce_to_codeableconcept",
				},
				{
					SourcePath: "segments.OBX.fields[4].value", // OBX.5 - Observation Value
					TargetPath: "valueString",
					DataType:   "string",
					Transform:  "obx_value_transform",
				},
				{
					SourcePath: "segments.OBX.fields[5].value", // OBX.6 - Units
					TargetPath: "valueQuantity.unit",
					DataType:   "string",
				},
				{
					SourcePath: "segments.OBX.fields[13].value", // OBX.14 - Date/Time of Observation
					TargetPath: "effectiveDateTime",
					DataType:   "dateTime",
					Transform:  "ts_to_datetime",
				},
			},
			RequiredFields: []string{"status", "code"},
			References: []ReferenceMapping{
				{
					SourcePath:    "segments.PID.fields[2].value",
					TargetPath:    "subject",
					ReferenceType: "Patient",
				},
			},
			Metadata: map[string]interface{}{
				"sourceSegment": "OBX",
				"priority":      "medium",
			},
		}
		mappings = append(mappings, observationMapping)
	}

	// Add more message type specific mappings
	switch messageType {
	case "ADT_A01", "ADT_A04": // Admit/Register
		// Additional mappings for admission
	case "ORU_R01": // Observation Result
		// Additional mappings for lab results
	case "ORM_O01": // Order Message
		// Additional mappings for orders
	}

	log.Printf("✅ Analyzed HL7 data: identified %d resource mappings", len(mappings))
	return mappings, nil
}

// analyzeJSONData analyzes JSON data for FHIR mapping
func (s *FHIRTransformationService) analyzeJSONData(sourceData map[string]interface{}) ([]ResourceMapping, error) {
	var mappings []ResourceMapping

	// Generic JSON to FHIR mapping
	// This would be more sophisticated in production
	for key, value := range sourceData {
		if resourceType := s.inferResourceTypeFromKey(key); resourceType != "" {
			mapping := ResourceMapping{
				ResourceType: resourceType,
				Profile:      "base",
				SourcePaths:  []string{key},
				Priority:     1,
				MappingRules: s.generateJSONMappingRules(key, value),
				Metadata: map[string]interface{}{
					"sourceKey": key,
					"inferred":  true,
				},
			}
			mappings = append(mappings, mapping)
		}
	}

	log.Printf("✅ Analyzed JSON data: identified %d resource mappings", len(mappings))
	return mappings, nil
}

// analyzeXMLData analyzes XML data for FHIR mapping
func (s *FHIRTransformationService) analyzeXMLData(sourceData map[string]interface{}) ([]ResourceMapping, error) {
	// TODO: Implement XML analysis
	return []ResourceMapping{}, fmt.Errorf("XML analysis not yet implemented")
}

// =====================================
// FHIR RESOURCE CREATION
// =====================================

// createFHIRResources creates FHIR resources from mappings
func (s *FHIRTransformationService) createFHIRResources(mappings []ResourceMapping, request *FHIRTransformRequest) ([]FHIRResource, error) {
	var resources []FHIRResource
	resourceReferences := make(map[string]string) // Track created resources for references

	// Sort mappings by priority
	sortedMappings := s.sortMappingsByPriority(mappings)

	for _, mapping := range sortedMappings {
		resource, err := s.createFHIRResource(mapping, request.SourceData, resourceReferences)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to create %s resource: %v", mapping.ResourceType, err)
			continue
		}

		if resource != nil {
			resources = append(resources, *resource)

			// Track resource for references
			resourceKey := fmt.Sprintf("%s/%s", resource.ResourceType, resource.ID)
			resourceReferences[mapping.ResourceType] = resourceKey
		}
	}

	// Post-process references
	s.resolveResourceReferences(resources, resourceReferences)

	log.Printf("✅ Created %d FHIR resources", len(resources))
	return resources, nil
}

// createFHIRResource creates a single FHIR resource
func (s *FHIRTransformationService) createFHIRResource(mapping ResourceMapping, sourceData map[string]interface{}, references map[string]string) (*FHIRResource, error) {
	startTime := time.Now()

	// Create base resource structure
	resourceContent := map[string]interface{}{
		"resourceType": mapping.ResourceType,
		"id":          s.generateResourceID(mapping.ResourceType),
	}

	// Apply mapping rules
	dataCompleteness := 0.0
	successfulMappings := 0

	for _, rule := range mapping.MappingRules {
		sourceValue, found := s.extractValueFromPath(sourceData, rule.SourcePath)
		if !found {
			if s.isRequiredField(rule.TargetPath, mapping.RequiredFields) {
				log.Printf("⚠️ Warning: Required field %s not found in source data", rule.SourcePath)
			}
			continue
		}

		// Apply transformation
		transformedValue, err := s.applyFieldTransformation(sourceValue, rule.Transform, rule.DataType)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to transform field %s: %v", rule.SourcePath, err)
			continue
		}

		// Set value in resource
		if err := s.setValueAtPath(resourceContent, rule.TargetPath, transformedValue); err != nil {
			log.Printf("⚠️ Warning: Failed to set value at %s: %v", rule.TargetPath, err)
			continue
		}

		successfulMappings++
	}

	// Calculate data completeness
	if len(mapping.MappingRules) > 0 {
		dataCompleteness = float64(successfulMappings) / float64(len(mapping.MappingRules)) * 100.0
	}

	// Add required fields with defaults
	s.addRequiredDefaults(resourceContent, mapping.ResourceType, mapping.RequiredFields)

	// Add extensions
	if len(mapping.Extensions) > 0 {
		extensions, err := s.createExtensions(mapping.Extensions, sourceData)
		if err == nil && len(extensions) > 0 {
			resourceContent["extension"] = extensions
		}
	}

	// Add references
	if len(mapping.References) > 0 {
		s.addResourceReferences(resourceContent, mapping.References, sourceData, references)
	}

	// Create FHIR resource
	resource := &FHIRResource{
		ID:               resourceContent["id"].(string),
		ResourceType:     mapping.ResourceType,
		Profile:          mapping.Profile,
		Version:          "R4",
		Content:          resourceContent,
		References:       []FHIRReference{},
		ValidationStatus: "NOT_VALIDATED",
		Metadata: FHIRResourceMetadata{
			SourceMapping:     fmt.Sprintf("%s_mapping", mapping.ResourceType),
			TransformationID:  uuid.New().String(),
			DataCompleteness:  dataCompleteness,
			GenerationMethod:  "automated_mapping",
			ProcessingTime:    time.Since(startTime),
			CustomMetadata:    mapping.Metadata,
		},
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	return resource, nil
}

// =====================================
// FIELD TRANSFORMATION METHODS
// =====================================

// applyFieldTransformation applies transformation to field values
func (s *FHIRTransformationService) applyFieldTransformation(value interface{}, transform, dataType string) (interface{}, error) {
	if transform == "" {
		return s.convertToDataType(value, dataType)
	}

	switch transform {
	case "hl7_message_type_to_coding":
		return s.transformMessageTypeToCoding(value)
	case "cx_to_identifier":
		return s.transformCXToIdentifier(value)
	case "xpn_to_humanname":
		return s.transformXPNToHumanName(value)
	case "ts_to_date":
		return s.transformTSToDate(value)
	case "ts_to_datetime":
		return s.transformTSToDateTime(value)
	case "administrative_sex_to_gender":
		return s.transformAdministrativeSexToGender(value)
	case "xad_to_address":
		return s.transformXADToAddress(value)
	case "facility_to_endpoint":
		return s.transformFacilityToEndpoint(value)
	case "facility_to_identifier":
		return s.transformFacilityToIdentifier(value)
	case "patient_class_to_encounter_class":
		return s.transformPatientClassToEncounterClass(value)
	case "pl_to_location":
		return s.transformPLToLocation(value)
	case "ce_to_codeableconcept":
		return s.transformCEToCodeableConcept(value)
	case "obx_value_transform":
		return s.transformOBXValue(value)
	default:
		log.Printf("⚠️ Warning: Unknown transformation: %s", transform)
		return s.convertToDataType(value, dataType)
	}
}

// Data type conversion methods
func (s *FHIRTransformationService) convertToDataType(value interface{}, dataType string) (interface{}, error) {
	switch dataType {
	case "string":
		return fmt.Sprintf("%v", value), nil
	case "code":
		return strings.ToLower(fmt.Sprintf("%v", value)), nil
	case "id":
		return s.sanitizeID(fmt.Sprintf("%v", value)), nil
	case "uri":
		return fmt.Sprintf("urn:id:%v", value), nil
	case "date":
		return s.formatAsDate(value)
	case "dateTime":
		return s.formatAsDateTime(value)
	case "boolean":
		return s.convertToBoolean(value), nil
	case "integer":
		return s.convertToInteger(value)
	case "decimal":
		return s.convertToDecimal(value)
	default:
		return value, nil
	}
}

// Specific transformation methods
func (s *FHIRTransformationService) transformMessageTypeToCoding(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)
	parts := strings.Split(valueStr, "_")

	if len(parts) >= 2 {
		return map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
			"code":   parts[0],
			"display": s.getMessageTypeDisplay(parts[0], parts[1]),
		}, nil
	}

	return map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
		"code":   valueStr,
		"display": valueStr,
	}, nil
}

func (s *FHIRTransformationService) transformCXToIdentifier(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Parse HL7 CX format: ID^CheckDigit^CheckDigitScheme^AssigningAuthority&NamespaceID&UniversalID&UniversalIDType^IdentifierTypeCode
	parts := strings.Split(valueStr, "^")

	identifier := map[string]interface{}{
		"value": parts[0],
	}

	if len(parts) > 3 && parts[3] != "" {
		identifier["system"] = fmt.Sprintf("urn:oid:%s", parts[3])
	}

	if len(parts) > 4 && parts[4] != "" {
		identifier["type"] = map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
					"code":   parts[4],
				},
			},
		}
	}

	return identifier, nil
}

func (s *FHIRTransformationService) transformXPNToHumanName(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Parse HL7 XPN format: FamilyName^GivenName^MiddleName^Suffix^Prefix^Degree
	parts := strings.Split(valueStr, "^")

	name := map[string]interface{}{
		"use": "official",
	}

	var givenNames []string

	if len(parts) > 0 && parts[0] != "" {
		name["family"] = parts[0]
	}

	if len(parts) > 1 && parts[1] != "" {
		givenNames = append(givenNames, parts[1])
	}

	if len(parts) > 2 && parts[2] != "" {
		givenNames = append(givenNames, parts[2])
	}

	if len(givenNames) > 0 {
		name["given"] = givenNames
	}

	if len(parts) > 3 && parts[3] != "" {
		name["suffix"] = []string{parts[3]}
	}

	if len(parts) > 4 && parts[4] != "" {
		name["prefix"] = []string{parts[4]}
	}

	return name, nil
}

func (s *FHIRTransformationService) transformTSToDate(value interface{}) (interface{}, error) {
	return s.formatAsDate(value)
}

func (s *FHIRTransformationService) transformTSToDateTime(value interface{}) (interface{}, error) {
	return s.formatAsDateTime(value)
}

func (s *FHIRTransformationService) transformAdministrativeSexToGender(value interface{}) (interface{}, error) {
	valueStr := strings.ToUpper(fmt.Sprintf("%v", value))

	genderMap := map[string]string{
		"M": "male",
		"F": "female",
		"O": "other",
		"U": "unknown",
	}

	if gender, exists := genderMap[valueStr]; exists {
		return gender, nil
	}

	return "unknown", nil
}

func (s *FHIRTransformationService) transformXADToAddress(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Parse HL7 XAD format: StreetAddress^OtherDesignation^City^State^ZipCode^Country^AddressType
	parts := strings.Split(valueStr, "^")

	address := map[string]interface{}{
		"use": "home",
	}

	var lines []string
	if len(parts) > 0 && parts[0] != "" {
		lines = append(lines, parts[0])
	}
	if len(parts) > 1 && parts[1] != "" {
		lines = append(lines, parts[1])
	}
	if len(lines) > 0 {
		address["line"] = lines
	}

	if len(parts) > 2 && parts[2] != "" {
		address["city"] = parts[2]
	}

	if len(parts) > 3 && parts[3] != "" {
		address["state"] = parts[3]
	}

	if len(parts) > 4 && parts[4] != "" {
		address["postalCode"] = parts[4]
	}

	if len(parts) > 5 && parts[5] != "" {
		address["country"] = parts[5]
	}

	return address, nil
}

func (s *FHIRTransformationService) transformFacilityToEndpoint(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)
	return fmt.Sprintf("hl7://%s", valueStr), nil
}

func (s *FHIRTransformationService) transformFacilityToIdentifier(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)
	return map[string]interface{}{
		"system": "urn:ietf:rfc:3986",
		"value":  fmt.Sprintf("urn:oid:%s", valueStr),
	}, nil
}

func (s *FHIRTransformationService) transformPatientClassToEncounterClass(value interface{}) (interface{}, error) {
	valueStr := strings.ToUpper(fmt.Sprintf("%v", value))

	classMap := map[string]map[string]interface{}{
		"E": {
			"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":   "EMER",
			"display": "emergency",
		},
		"I": {
			"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":   "IMP",
			"display": "inpatient encounter",
		},
		"O": {
			"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":   "AMB",
			"display": "ambulatory",
		},
	}

	if class, exists := classMap[valueStr]; exists {
		return class, nil
	}

	return map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
		"code":   "UNK",
		"display": "unknown",
	}, nil
}

func (s *FHIRTransformationService) transformPLToLocation(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Parse HL7 PL format: PointOfCare^Room^Bed^Facility^LocationStatus^PersonLocationType^Building^Floor
	parts := strings.Split(valueStr, "^")

	var locationParts []string
	for i, part := range parts {
		if part != "" && i < 4 { // Use first 4 components
			locationParts = append(locationParts, part)
		}
	}

	return strings.Join(locationParts, " "), nil
}

func (s *FHIRTransformationService) transformCEToCodeableConcept(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Parse HL7 CE format: Identifier^Text^NameOfCodingSystem^AlternateIdentifier^AlternateText^NameOfAlternateCodingSystem
	parts := strings.Split(valueStr, "^")

	concept := map[string]interface{}{
		"coding": []map[string]interface{}{},
	}

	if len(parts) > 0 && parts[0] != "" {
		coding := map[string]interface{}{
			"code": parts[0],
		}

		if len(parts) > 1 && parts[1] != "" {
			coding["display"] = parts[1]
		}

		if len(parts) > 2 && parts[2] != "" {
			coding["system"] = s.mapCodingSystem(parts[2])
		}

		concept["coding"] = []map[string]interface{}{coding}
	}

	if len(parts) > 1 && parts[1] != "" {
		concept["text"] = parts[1]
	}

	return concept, nil
}

func (s *FHIRTransformationService) transformOBXValue(value interface{}) (interface{}, error) {
	// For now, just return as string - would need OBX.2 to determine actual type
	return fmt.Sprintf("%v", value), nil
}

// =====================================
// UTILITY METHODS
// =====================================

// Helper methods for resource creation
func (s *FHIRTransformationService) extractHL7MessageType(sourceData map[string]interface{}) string {
	if messageHeader, ok := sourceData["messageHeader"].(map[string]interface{}); ok {
		if messageType, ok := messageHeader["messageType"].(map[string]interface{}); ok {
			if structure, ok := messageType["messageStructure"].(string); ok {
				return structure
			}
		}
	}
	return "UNKNOWN"
}

func (s *FHIRTransformationService) hasSegment(sourceData map[string]interface{}, segmentName string) bool {
	if segmentGroups, ok := sourceData["segmentGroups"].(map[string]interface{}); ok {
		_, exists := segmentGroups[segmentName]
		return exists
	}
	return false
}

func (s *FHIRTransformationService) hasOrganizationData(sourceData map[string]interface{}) bool {
	if messageHeader, ok := sourceData["messageHeader"].(map[string]interface{}); ok {
		if facility, ok := messageHeader["sendingFacility"].(string); ok {
			return facility != ""
		}
	}
	return false
}

func (s *FHIRTransformationService) inferResourceTypeFromKey(key string) string {
	keyLower := strings.ToLower(key)

	if strings.Contains(keyLower, "patient") {
		return "Patient"
	}
	if strings.Contains(keyLower, "organization") || strings.Contains(keyLower, "facility") {
		return "Organization"
	}
	if strings.Contains(keyLower, "encounter") || strings.Contains(keyLower, "visit") {
		return "Encounter"
	}
	if strings.Contains(keyLower, "observation") || strings.Contains(keyLower, "result") {
		return "Observation"
	}

	return ""
}

func (s *FHIRTransformationService) generateJSONMappingRules(key string, value interface{}) []FieldMappingRule {
	// Generate basic mapping rules for JSON data
	return []FieldMappingRule{
		{
			SourcePath: key,
			TargetPath: "text.div",
			DataType:   "string",
			Transform:  "json_to_narrative",
		},
	}
}

func (s *FHIRTransformationService) sortMappingsByPriority(mappings []ResourceMapping) []ResourceMapping {
	// Simple sort by priority
	for i := 0; i < len(mappings)-1; i++ {
		for j := i + 1; j < len(mappings); j++ {
			if mappings[i].Priority > mappings[j].Priority {
				mappings[i], mappings[j] = mappings[j], mappings[i]
			}
		}
	}
	return mappings
}

func (s *FHIRTransformationService) generateResourceID(resourceType string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(resourceType), uuid.New().String()[:8])
}

func (s *FHIRTransformationService) extractValueFromPath(data map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			value, exists := current[part]
			return value, exists
		}

		next, exists := current[part]
		if !exists {
			return nil, false
		}

		if nextMap, ok := next.(map[string]interface{}); ok {
			current = nextMap
		} else {
			return nil, false
		}
	}

	return nil, false
}

func (s *FHIRTransformationService) setValueAtPath(data map[string]interface{}, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}

		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}

		if nextMap, ok := current[part].(map[string]interface{}); ok {
			current = nextMap
		} else {
			return fmt.Errorf("cannot set value at path %s: intermediate value is not a map", path)
		}
	}

	return nil
}

func (s *FHIRTransformationService) isRequiredField(targetPath string, requiredFields []string) bool {
	for _, required := range requiredFields {
		if strings.HasPrefix(targetPath, required) {
			return true
		}
	}
	return false
}

func (s *FHIRTransformationService) addRequiredDefaults(resourceContent map[string]interface{}, resourceType string, requiredFields []string) {
	switch resourceType {
	case "Patient":
		// No required fields for Patient in base profile
	case "MessageHeader":
		if _, exists := resourceContent["eventCoding"]; !exists {
			resourceContent["eventCoding"] = map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
				"code":   "UNK",
				"display": "Unknown",
			}
		}
		if _, exists := resourceContent["source"]; !exists {
			resourceContent["source"] = map[string]interface{}{
				"name":     "Unknown Source",
				"endpoint": "urn:unknown",
			}
		}
	case "Observation":
		if _, exists := resourceContent["status"]; !exists {
			resourceContent["status"] = "final"
		}
		if _, exists := resourceContent["code"]; !exists {
			resourceContent["code"] = map[string]interface{}{
				"coding": []map[string]interface{}{
					{
						"system":  "http://loinc.org",
						"code":    "33747-0",
						"display": "General observation",
					},
				},
			}
		}
	case "Encounter":
		if _, exists := resourceContent["status"]; !exists {
			resourceContent["status"] = "finished"
		}
		if _, exists := resourceContent["class"]; !exists {
			resourceContent["class"] = map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
				"code":    "UNK",
				"display": "unknown",
			}
		}
	}
}

func (s *FHIRTransformationService) createExtensions(extensionMappings []ExtensionMapping, sourceData map[string]interface{}) ([]map[string]interface{}, error) {
	var extensions []map[string]interface{}

	for _, mapping := range extensionMappings {
		value, found := s.extractValueFromPath(sourceData, mapping.SourcePath)
		if !found && mapping.Required {
			continue
		}

		transformedValue, err := s.applyFieldTransformation(value, mapping.Transform, mapping.ValueType)
		if err != nil {
			continue
		}

		extension := map[string]interface{}{
			"url":   mapping.URL,
			"value": transformedValue,
		}

		extensions = append(extensions, extension)
	}

	return extensions, nil
}

func (s *FHIRTransformationService) addResourceReferences(resourceContent map[string]interface{}, referenceMappings []ReferenceMapping, sourceData map[string]interface{}, references map[string]string) {
	for _, refMapping := range referenceMappings {
		if refTarget, exists := references[refMapping.ReferenceType]; exists {
			reference := map[string]interface{}{
				"reference": refTarget,
			}

			if refMapping.DisplayPath != "" {
				if displayValue, found := s.extractValueFromPath(sourceData, refMapping.DisplayPath); found {
					reference["display"] = fmt.Sprintf("%v", displayValue)
				}
			}

			s.setValueAtPath(resourceContent, refMapping.TargetPath, reference)
		}
	}
}

func (s *FHIRTransformationService) resolveResourceReferences(resources []FHIRResource, references map[string]string) {
	// Update resources with proper references after all resources are created
	for i := range resources {
		// This would update references in the resource content
		// Implementation depends on specific reference resolution needs
	}
}

// Format and conversion helper methods
func (s *FHIRTransformationService) formatAsDate(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Try to parse HL7 timestamp formats
	formats := []string{
		"20060102",
		"200601021504",
		"20060102150405",
	}

	for _, format := range formats {
		if len(valueStr) >= len("20060102") {
			if t, err := time.Parse(format[:len(valueStr)], valueStr); err == nil {
				return t.Format("2006-01-02"), nil
			}
		}
	}

	return valueStr, nil
}

func (s *FHIRTransformationService) formatAsDateTime(value interface{}) (interface{}, error) {
	valueStr := fmt.Sprintf("%v", value)

	// Try to parse HL7 timestamp formats
	formats := []string{
		"20060102150405",
		"200601021504",
		"2006010215",
	}

	for _, format := range formats {
		if len(valueStr) >= len(format) {
			if t, err := time.Parse(format, valueStr[:len(format)]); err == nil {
				return t.Format("2006-01-02T15:04:05Z"), nil
			}
		}
	}

	return valueStr, nil
}

func (s *FHIRTransformationService) convertToBoolean(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes"
	case int:
		return v != 0
	default:
		return false
	}
}

func (s *FHIRTransformationService) convertToInteger(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("cannot convert %v to integer", value)
	}
}

func (s *FHIRTransformationService) convertToDecimal(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0.0, fmt.Errorf("cannot convert %v to decimal", value)
	}
}

func (s *FHIRTransformationService) sanitizeID(id string) string {
	// Remove invalid characters for FHIR ID
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-\.]`)
	return reg.ReplaceAllString(id, "")
}

func (s *FHIRTransformationService) getMessageTypeDisplay(messageCode, triggerEvent string) string {
	displays := map[string]string{
		"ADT": "Admission, Discharge, Transfer",
		"ORU": "Observation Result",
		"ORM": "Order Message",
		"SIU": "Schedule Information",
		"MDM": "Medical Document Management",
	}

	if display, exists := displays[messageCode]; exists {
		return display
	}

	return fmt.Sprintf("%s %s", messageCode, triggerEvent)
}

func (s *FHIRTransformationService) mapCodingSystem(hl7System string) string {
	systemMap := map[string]string{
		"LN":  "http://loinc.org",
		"SNM": "http://snomed.info/sct",
		"ICD9": "http://hl7.org/fhir/sid/icd-9-cm",
		"ICD10": "http://hl7.org/fhir/sid/icd-10-cm",
		"CPT": "http://www.ama-assn.org/go/cpt",
	}

	if system, exists := systemMap[hl7System]; exists {
		return system
	}

	return fmt.Sprintf("urn:oid:%s", hl7System)
}

// Continue with validation and bundle creation methods in next part...