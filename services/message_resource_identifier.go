// services/message_resource_identifier.go
// Official HL7 to FHIR Resource Identifier Service
// Based on official HL7 working group specifications

package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type MessageResourceIdentifierService struct {
	db *sql.DB
}

type FHIRResourceTemplate struct {
	MessageType      string                    `json:"messageType"`
	FHIRResources    []string                  `json:"fhirResources"`
	ResourceLogic    map[string]ResourceConfig `json:"resourceLogic"`
	ResourcePriority map[string]int            `json:"resourcePriority"`
	Source           string                    `json:"source"`
}

type ResourceConfig struct {
	From       string   `json:"from"`
	Required   bool     `json:"required"`
	Priority   int      `json:"priority"`
	Condition  string   `json:"condition,omitempty"`
	Multiple   bool     `json:"multiple,omitempty"`
	References []string `json:"references,omitempty"`
}

func NewMessageResourceIdentifierService(db *sql.DB) *MessageResourceIdentifierService {
	return &MessageResourceIdentifierService{
		db: db,
	}
}

// GetResourcesForMessage returns FHIR resources needed for a message type with parsed HL7 data
func (s *MessageResourceIdentifierService) GetResourcesForMessage(messageType string, parsedHL7 map[string]interface{}, interfaceID string) (*FHIRResourceTemplate, error) {
	log.Printf("🔍 Identifying FHIR resources for message type: %s", messageType)

	// 1. Check for interface-specific overrides first
	template, err := s.getInterfaceOverride(interfaceID, messageType)
	if err == nil && template != nil {
		log.Printf("✅ Using interface-specific override for %s", interfaceID)
		return s.filterResourcesByContent(template, parsedHL7), nil
	}

	// 2. Get official HL7 working group template from database
	template, err = s.getOfficialTemplate(messageType)
	if err != nil {
		log.Printf("⚠️ No official template found for %s, using built-in fallback", messageType)
		template = s.getBuiltInTemplate(messageType)
	}

	// 3. Filter resources based on actual message content
	filteredTemplate := s.filterResourcesByContent(template, parsedHL7)

	log.Printf("✅ Identified %d FHIR resources for %s", len(filteredTemplate.FHIRResources), messageType)
	return filteredTemplate, nil
}

// getOfficialTemplate retrieves official HL7 working group mapping from database
func (s *MessageResourceIdentifierService) getOfficialTemplate(messageType string) (*FHIRResourceTemplate, error) {
	query := `
		SELECT message_type, fhir_resources, resource_conditions, resource_priorities, source
		FROM message_fhir_templates 
		WHERE message_type = $1 AND is_default = true
		ORDER BY id LIMIT 1`

	var template FHIRResourceTemplate
	var fhirResourcesJSON, conditionsJSON, prioritiesJSON string

	err := s.db.QueryRow(query, messageType).Scan(
		&template.MessageType,
		&fhirResourcesJSON,
		&conditionsJSON,
		&prioritiesJSON,
		&template.Source,
	)

	if err != nil {
		return nil, fmt.Errorf("no official template found for message type %s: %w", messageType, err)
	}

	// Parse JSON fields
	if err := json.Unmarshal([]byte(fhirResourcesJSON), &template.FHIRResources); err != nil {
		return nil, fmt.Errorf("error parsing FHIR resources: %w", err)
	}

	if err := json.Unmarshal([]byte(conditionsJSON), &template.ResourceLogic); err != nil {
		return nil, fmt.Errorf("error parsing resource conditions: %w", err)
	}

	if err := json.Unmarshal([]byte(prioritiesJSON), &template.ResourcePriority); err != nil {
		return nil, fmt.Errorf("error parsing resource priorities: %w", err)
	}

	return &template, nil
}

// getInterfaceOverride checks for interface-specific resource mapping overrides
func (s *MessageResourceIdentifierService) getInterfaceOverride(interfaceID, messageType string) (*FHIRResourceTemplate, error) {
	if interfaceID == "" {
		return nil, fmt.Errorf("no interface ID provided")
	}

	query := `
		SELECT fhir_resources, custom_conditions
		FROM interface_resource_overrides 
		WHERE interface_id = $1 AND message_type = $2
		ORDER BY created_at DESC LIMIT 1`

	var fhirResourcesJSON, conditionsJSON string
	err := s.db.QueryRow(query, interfaceID, messageType).Scan(&fhirResourcesJSON, &conditionsJSON)

	if err != nil {
		return nil, err // No override found
	}

	template := &FHIRResourceTemplate{
		MessageType: messageType,
		Source:      fmt.Sprintf("INTERFACE_OVERRIDE_%s", interfaceID),
	}

	// Parse JSON fields
	if err := json.Unmarshal([]byte(fhirResourcesJSON), &template.FHIRResources); err != nil {
		return nil, fmt.Errorf("error parsing override FHIR resources: %w", err)
	}

	if err := json.Unmarshal([]byte(conditionsJSON), &template.ResourceLogic); err != nil {
		return nil, fmt.Errorf("error parsing override conditions: %w", err)
	}

	return template, nil
}

// getBuiltInTemplate provides fallback templates based on official HL7 WG specifications
func (s *MessageResourceIdentifierService) getBuiltInTemplate(messageType string) *FHIRResourceTemplate {
	// Built-in templates based on official HL7 working group mappings
	templates := map[string]*FHIRResourceTemplate{
		"ADT^A01": {
			MessageType:   "ADT^A01",
			FHIRResources: []string{"Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition", "Procedure"},
			ResourceLogic: map[string]ResourceConfig{
				"Bundle":             {From: "MSH", Required: true, Priority: 1},
				"MessageHeader":      {From: "MSH", Required: true, Priority: 1},
				"Provenance":         {From: "MSH,EVN,PID", Multiple: true, Priority: 1},
				"Patient":            {From: "PID,PD1,PV1", Required: true, Priority: 2},
				"Encounter":          {From: "PV1,PV2", Required: true, Priority: 3, References: []string{"Patient"}},
				"Coverage":           {From: "PV1,IN1", Condition: "insurance_present", Priority: 4},
				"RelatedPerson":      {From: "NK1", Condition: "next_of_kin_present", Multiple: true, Priority: 5},
				"Observation":        {From: "PD1,OBX", Condition: "observations_present", Multiple: true, Priority: 6},
				"AllergyIntolerance": {From: "AL1", Condition: "allergies_present", Multiple: true, Priority: 7},
				"Condition":          {From: "DG1", Condition: "diagnosis_present", Multiple: true, Priority: 8},
				"Procedure":          {From: "PR1", Condition: "procedures_present", Multiple: true, Priority: 9},
			},
			Source: "BUILTIN_HL7_WG",
		},
		"ORU^R01": {
			MessageType:   "ORU^R01",
			FHIRResources: []string{"Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "DiagnosticReport", "ServiceRequest", "Observation", "Specimen", "PractitionerRole"},
			ResourceLogic: map[string]ResourceConfig{
				"Bundle":           {From: "MSH", Required: true, Priority: 1},
				"MessageHeader":    {From: "MSH", Required: true, Priority: 1},
				"Provenance":       {From: "MSH,PID", Multiple: true, Priority: 1},
				"Patient":          {From: "PID,PD1,PV1", Required: true, Priority: 2},
				"Encounter":        {From: "PV1,PV2", Condition: "visit_present", Priority: 3, References: []string{"Patient"}},
				"DiagnosticReport": {From: "ORC,OBR", Required: true, Multiple: true, Priority: 6, References: []string{"Patient", "Encounter"}},
				"ServiceRequest":   {From: "ORC,OBR", Condition: "service_request_needed", Multiple: true, Priority: 6},
				"Observation":      {From: "OBX", Multiple: true, Priority: 7, References: []string{"DiagnosticReport", "Patient"}},
				"Specimen":         {From: "OBR,SPM", Condition: "specimen_present", Multiple: true, Priority: 8},
				"PractitionerRole": {From: "PRT", Condition: "practitioner_present", Multiple: true, Priority: 9},
			},
			Source: "BUILTIN_HL7_WG",
		},
		"ORM^O01": {
			MessageType:   "ORM^O01",
			FHIRResources: []string{"Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "ServiceRequest", "Task", "MedicationRequest", "Condition", "Observation"},
			ResourceLogic: map[string]ResourceConfig{
				"Bundle":            {From: "MSH", Required: true, Priority: 1},
				"MessageHeader":     {From: "MSH", Required: true, Priority: 1},
				"Provenance":        {From: "MSH,PID,ORC", Multiple: true, Priority: 1},
				"Patient":           {From: "PID,PD1,PV1", Required: true, Priority: 2},
				"Encounter":         {From: "PV1,PV2", Condition: "visit_present", Priority: 3, References: []string{"Patient"}},
				"ServiceRequest":    {From: "ORC,OBR", Required: true, Multiple: true, Priority: 6, References: []string{"Patient"}},
				"Task":              {From: "ORC", Condition: "task_needed", Multiple: true, Priority: 6, References: []string{"ServiceRequest"}},
				"MedicationRequest": {From: "RXO", Condition: "medication_order", Multiple: true, Priority: 7},
				"Condition":         {From: "DG1", Condition: "diagnosis_present", Multiple: true, Priority: 8, References: []string{"Patient"}},
				"Observation":       {From: "PD1,OBX", Condition: "observations_present", Multiple: true, Priority: 9},
			},
			Source: "BUILTIN_HL7_WG",
		},
	}

	if template, exists := templates[messageType]; exists {
		return template
	}

	// Minimal fallback for unknown message types
	return &FHIRResourceTemplate{
		MessageType:   messageType,
		FHIRResources: []string{"Bundle", "MessageHeader", "Patient"},
		ResourceLogic: map[string]ResourceConfig{
			"Bundle":        {From: "MSH", Required: true, Priority: 1},
			"MessageHeader": {From: "MSH", Required: true, Priority: 1},
			"Patient":       {From: "PID", Required: true, Priority: 2},
		},
		Source: "MINIMAL_FALLBACK",
	}
}

// filterResourcesByContent filters resources based on actual message content and conditions
func (s *MessageResourceIdentifierService) filterResourcesByContent(template *FHIRResourceTemplate, parsedHL7 map[string]interface{}) *FHIRResourceTemplate {
	var filteredResources []string
	filteredLogic := make(map[string]ResourceConfig)

	for _, resource := range template.FHIRResources {
		config, exists := template.ResourceLogic[resource]
		if !exists {
			// Include resources without specific conditions
			filteredResources = append(filteredResources, resource)
			continue
		}

		// Check if required segments exist
		if s.segmentsExist(config.From, parsedHL7) {
			// Check additional conditions
			if config.Condition == "" || s.evaluateCondition(config.Condition, parsedHL7) {
				filteredResources = append(filteredResources, resource)
				filteredLogic[resource] = config
			}
		} else if config.Required {
			// Always include required resources even if segments missing (creates empty resource)
			filteredResources = append(filteredResources, resource)
			filteredLogic[resource] = config
		}
	}

	return &FHIRResourceTemplate{
		MessageType:      template.MessageType,
		FHIRResources:    filteredResources,
		ResourceLogic:    filteredLogic,
		ResourcePriority: template.ResourcePriority,
		Source:           template.Source,
	}
}

// segmentsExist checks if any of the specified segments exist in parsed HL7 data
func (s *MessageResourceIdentifierService) segmentsExist(fromSegments string, parsedHL7 map[string]interface{}) bool {
	segments := strings.Split(fromSegments, ",")
	for _, segment := range segments {
		segmentName := strings.TrimSpace(strings.Split(segment, ".")[0]) // Get segment name before field
		if _, exists := parsedHL7[segmentName]; exists {
			return true
		}
	}
	return false
}

// evaluateCondition evaluates business logic conditions for resource inclusion
func (s *MessageResourceIdentifierService) evaluateCondition(condition string, parsedHL7 map[string]interface{}) bool {
	switch condition {
	case "insurance_present":
		return s.checkField(parsedHL7, "PV1", "20") || s.segmentsExist("IN1", parsedHL7)
	case "next_of_kin_present":
		return s.segmentsExist("NK1", parsedHL7)
	case "observations_present":
		return s.segmentsExist("OBX", parsedHL7) || s.checkField(parsedHL7, "PD1", "7")
	case "allergies_present":
		return s.segmentsExist("AL1", parsedHL7)
	case "diagnosis_present":
		return s.segmentsExist("DG1", parsedHL7)
	case "procedures_present":
		return s.segmentsExist("PR1", parsedHL7)
	case "visit_present":
		return s.segmentsExist("PV1", parsedHL7)
	case "specimen_present":
		return s.segmentsExist("SPM", parsedHL7) || s.checkSpecimenInOBR(parsedHL7)
	case "service_request_needed":
		return s.segmentsExist("ORC", parsedHL7)
	case "task_needed":
		return s.checkTaskNeeded(parsedHL7)
	case "medication_order":
		return s.segmentsExist("RXO", parsedHL7)
	case "practitioner_present":
		return s.checkPractitionerInPRT(parsedHL7)
	case "PV1.43_valued":
		return s.checkField(parsedHL7, "PV1", "43")
	case "PV1.20_valued":
		return s.checkField(parsedHL7, "PV1", "20")
	case "PD1.7_valued":
		return s.checkField(parsedHL7, "PD1", "7")
	default:
		log.Printf("⚠️ Unknown condition: %s, defaulting to true", condition)
		return true
	}
}

// Helper methods for condition evaluation
func (s *MessageResourceIdentifierService) checkField(parsedHL7 map[string]interface{}, segment, field string) bool {
	if segmentData, exists := parsedHL7[segment]; exists {
		if segmentMap, ok := segmentData.(map[string]interface{}); ok {
			if fieldData, fieldExists := segmentMap[field]; fieldExists {
				// Check if field has a non-empty value
				if fieldStr, ok := fieldData.(string); ok {
					return strings.TrimSpace(fieldStr) != ""
				}
				// For complex field structures
				return fieldData != nil
			}
		}
	}
	return false
}

func (s *MessageResourceIdentifierService) checkSpecimenInOBR(parsedHL7 map[string]interface{}) bool {
	// Check if OBR contains specimen-related fields (15, 16, etc.)
	return s.checkField(parsedHL7, "OBR", "15") || s.checkField(parsedHL7, "OBR", "16")
}

func (s *MessageResourceIdentifierService) checkTaskNeeded(parsedHL7 map[string]interface{}) bool {
	// Task is needed when ORC-1 indicates a fillable order
	if orcData, exists := parsedHL7["ORC"]; exists {
		if orcMap, ok := orcData.(map[string]interface{}); ok {
			if orc1, exists := orcMap["1"]; exists {
				if orderControl, ok := orc1.(string); ok {
					fillableControls := []string{"NW", "RF", "AF", "RP", "RU", "XO"}
					for _, control := range fillableControls {
						if strings.EqualFold(orderControl, control) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (s *MessageResourceIdentifierService) checkPractitionerInPRT(parsedHL7 map[string]interface{}) bool {
	// Check if PRT contains practitioner role codes
	return s.checkField(parsedHL7, "PRT", "4") || s.checkField(parsedHL7, "PRT", "5")
}

// SaveInterfaceOverride allows interfaces to customize their resource mappings
func (s *MessageResourceIdentifierService) SaveInterfaceOverride(interfaceID, messageType string, customResources []string, customConditions map[string]ResourceConfig, createdBy string) error {
	resourcesJSON, err := json.Marshal(customResources)
	if err != nil {
		return fmt.Errorf("error marshaling custom resources: %w", err)
	}

	conditionsJSON, err := json.Marshal(customConditions)
	if err != nil {
		return fmt.Errorf("error marshaling custom conditions: %w", err)
	}

	query := `
		INSERT INTO interface_resource_overrides (interface_id, message_type, fhir_resources, custom_conditions, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (interface_id, message_type) 
		DO UPDATE SET fhir_resources = $3, custom_conditions = $4, created_by = $5, created_at = NOW()`

	_, err = s.db.Exec(query, interfaceID, messageType, string(resourcesJSON), string(conditionsJSON), createdBy)
	if err != nil {
		return fmt.Errorf("error saving interface override: %w", err)
	}

	log.Printf("✅ Saved interface override for %s message type %s", interfaceID, messageType)
	return nil
}
