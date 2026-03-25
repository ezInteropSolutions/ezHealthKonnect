// services/hl7_fhir_transform_service.go
// Schema-driven HL7 to FHIR transformation service
//
// 🎯 PRINCIPLE: SCHEMA-FIRST APPROACH
// - Load FHIR schema from .gz files (no fallbacks)
// - Generate resources from schema (no hardcoding)
// - Only populate fields that have HL7 mappings
// - Fail fast if schema is missing/broken
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// =====================================
// REQUEST/RESPONSE STRUCTURES
// =====================================

type TransformRequest struct {
	ParsedHL7Data     map[string]interface{} `json:"parsedHL7Data" binding:"required"`
	MessageType       string                 `json:"messageType,omitempty"` // OOB: Injected from pipeline config
	TargetProfile     string                 `json:"targetProfile,omitempty"`
	FHIRVersion       string                 `json:"fhirVersion,omitempty"`
	CreateBundle      bool                   `json:"createBundle,omitempty"`
	ValidationMode    string                 `json:"validationMode,omitempty"`
	InterfaceID       string                 `json:"interfaceId,omitempty"`
	RequestID         string                 `json:"requestId,omitempty"`
	SelectedResources []string               `json:"selectedResources,omitempty"` // Optional: filter to specific FHIR resource types
}

type TransformResponse struct {
	Success            bool                     `json:"success"`
	RequestID          string                   `json:"requestId"`
	MessageType        string                   `json:"messageType"`
	FHIRResources      []map[string]interface{} `json:"fhirResources"`
	AtomicMappings     []AtomicMapping          `json:"atomicMappings"`
	Bundle             map[string]interface{}   `json:"bundle,omitempty"`
	ResourceCounts     map[string]int           `json:"resourceCounts"`
	MappingStats       MappingStatistics        `json:"mappingStats"`
	Warnings           []string                 `json:"warnings"`
	Errors             []string                 `json:"errors"`
	Performance        PerformanceMetrics       `json:"performance"`
	ValidationIssues   []ValidationIssue        `json:"validationIssues,omitempty"`
	// ValidationFailures collects required-field misses annotated with the
	// onMissing policy from the template.  Populated by ValidationPolicyEnforcer
	// during createResourceFromAtomicMappings.  When any entry has
	// Severity == "queue_review" the caller should NOT deliver the message but
	// instead insert a row into fhir_validation_queue.
	ValidationFailures []ValidationFailure      `json:"validationFailures,omitempty"`
}

type MappingStatistics struct {
	TotalFieldsMapped    int `json:"totalFieldsMapped"`
	RequiredFieldsMapped int `json:"requiredFieldsMapped"`
	OptionalFieldsMapped int `json:"optionalFieldsMapped"`
	UnmappedFields       int `json:"unmappedFields"`
	ValueSetTransforms   int `json:"valueSetTransforms"`
	DataTypeTransforms   int `json:"dataTypeTransforms"`
}

type PerformanceMetrics struct {
	TotalTime        string `json:"totalTime"`
	DatabaseTime     string `json:"databaseTime"`
	TransformTime    string `json:"transformTime"`
	ValidationTime   string `json:"validationTime"`
	ResourcesCreated int    `json:"resourcesCreated"`
}

type ValidationIssue struct {
	Severity     string `json:"severity"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	ResourceType string `json:"resourceType,omitempty"`
	Path         string `json:"path,omitempty"`
}

// ValidationFailure records a single missing-required-field event together
// with the onMissing policy that was configured for it in the OOB template.
// Field     - FHIR element path, e.g. "Patient.identifier[0].value"
// HL7Path   - source HL7 path, e.g. "PID.3.1"
// MissingCode - machine-readable code, e.g. "MISSING_MRN"
// Severity  - mirrors the onMissing policy: "queue_review", "warn", "reject", "default"
type ValidationFailure struct {
	Field       string `json:"field"`
	HL7Path     string `json:"hl7Path"`
	MissingCode string `json:"missingCode"`
	Severity    string `json:"severity"`
}

// =====================================
// SCHEMA STRUCTURES
// =====================================

type FHIRResourceSchema struct {
	ResourceType string                   `json:"resourceType"`
	Fields       map[string]*FHIRFieldDef `json:"fields"`
	Required     []string                 `json:"required"`
}

type FHIRFieldDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Cardinality string `json:"cardinality"`
	IsArray     bool   `json:"isArray"`
	Required    bool   `json:"required"`
	Min         int    `json:"min"`
	Max         string `json:"max"`
}

// =====================================
// DATABASE STRUCTURES
// =====================================

type FieldMapping struct {
	ID                  int                    `json:"id"`
	SegmentName         string                 `json:"segmentName"`
	HL7Field            string                 `json:"hl7Field"`
	HL7Component        string                 `json:"hl7Component"`
	HL7SubComponent     string                 `json:"hl7SubComponent"`
	FHIRResourceType    string                 `json:"fhirResourceType"`
	FHIRElementPath     string                 `json:"fhirElementPath"`
	DataTypeTransform   string                 `json:"dataTypeTransform"`
	ValueSetMappingID   int                    `json:"valueSetMappingId"`
	MappingConditions   map[string]interface{} `json:"mappingConditions"`
	TransformationRules map[string]interface{} `json:"transformationRules"`
	IsRequired          bool                   `json:"isRequired"`
	Cardinality         string                 `json:"cardinality"`
	// HL7DataType is populated automatically from the HL7 schema (e.g. "TS", "CX", "XPN").
	// Used by AutoTranslate when DataTypeTransform is empty.
	HL7DataType  string `json:"hl7DataType,omitempty"`
	// FHIRDataType is populated automatically from the FHIR schema (e.g. "dateTime", "CodeableConcept").
	// Used by AutoTranslate when DataTypeTransform is empty.
	FHIRDataType string `json:"fhirDataType,omitempty"`
}

type ValueSetMapping struct {
	ID          int    `json:"id"`
	MappingName string `json:"mappingName"`
	HL7Table    string `json:"hl7Table"`
	HL7Value    string `json:"hl7Value"`
	FHIRSystem  string `json:"fhirSystem"`
	FHIRCode    string `json:"fhirCode"`
	FHIRDisplay string `json:"fhirDisplay"`
	MappingType string `json:"mappingType"`
}

// =====================================
// TRANSFORMATION SERVICE
// =====================================

type HL7FHIRTransformService struct {
	db          *sql.DB
	fhirLoader  *fhir.FHIRSchemaLoader
	schemaCache map[string]*FHIRResourceSchema
	schemaReady bool
}

func NewHL7FHIRTransformService(database *sql.DB) *HL7FHIRTransformService {
	service := &HL7FHIRTransformService{
		db:          database,
		fhirLoader:  fhir.GetFHIRSchemaLoader(),
		schemaCache: make(map[string]*FHIRResourceSchema),
		schemaReady: false,
	}

	// Load FHIR schemas from .gz files - REQUIRED
	if err := service.loadFHIRSchemasFromGZ(); err != nil {
		log.Fatalf("❌ FATAL: Failed to load FHIR schemas: %v", err)
	}

	return service
}

// =====================================
// SCHEMA LOADING - REQUIRED IMPLEMENTATION
// =====================================

func (s *HL7FHIRTransformService) loadFHIRSchemasFromGZ() error {
	log.Printf("🔧 Loading FHIR schemas from .gz files...")

	// TODO: Replace with actual implementation
	// This MUST read StructureDefinition resources from FHIR package .gz files
	// and build the schema cache. NO fallbacks allowed.

	if s.fhirLoader == nil {
		return fmt.Errorf("FHIR schema loader not available")
	}

	// Example implementation outline:
	//
	// 1. Read FHIR package (e.g., hl7.fhir.r4.core#4.0.1.tgz)
	// 2. Extract StructureDefinition-*.json files
	// 3. Parse each StructureDefinition
	// 4. Build FHIRResourceSchema from snapshot.element arrays
	// 5. Cache schemas by resource type

	// TEMPORARY: Load essential schemas for development
	// This MUST be replaced with actual .gz file reading
	s.loadEssentialSchemasFromLoader()

	s.schemaReady = true
	log.Printf("✅ FHIR schemas loaded successfully")
	return nil
}

func (s *HL7FHIRTransformService) loadEssentialSchemasFromLoader() {
	// TEMPORARY: Create minimal schemas for development
	// This MUST be replaced with actual StructureDefinition parsing

	// MessageHeader schema
	s.schemaCache["MessageHeader"] = &FHIRResourceSchema{
		ResourceType: "MessageHeader",
		Fields: map[string]*FHIRFieldDef{
			"id":          {Name: "id", Type: "id", Required: false, Min: 0, Max: "1"},
			"meta":        {Name: "meta", Type: "Meta", Required: false, Min: 0, Max: "1"},
			"text":        {Name: "text", Type: "Narrative", Required: false, Min: 0, Max: "1"},
			"eventCoding": {Name: "eventCoding", Type: "Coding", Required: false, Min: 0, Max: "1"},
			"eventUri":    {Name: "eventUri", Type: "uri", Required: false, Min: 0, Max: "1"},
			"destination": {Name: "destination", Type: "BackboneElement", Required: false, Min: 1, Max: "*", IsArray: true},
			"sender":      {Name: "sender", Type: "Reference", Required: false, Min: 0, Max: "1"},
			"enterer":     {Name: "enterer", Type: "Reference", Required: false, Min: 0, Max: "1"},
			"author":      {Name: "author", Type: "Reference", Required: false, Min: 0, Max: "1"},
			"source":      {Name: "source", Type: "BackboneElement", Required: true, Min: 1, Max: "1"},
			"responsible": {Name: "responsible", Type: "Reference", Required: false, Min: 0, Max: "1"},
			"reason":      {Name: "reason", Type: "CodeableConcept", Required: false, Min: 0, Max: "1"},
			"response":    {Name: "response", Type: "BackboneElement", Required: false, Min: 0, Max: "1"},
			"focus":       {Name: "focus", Type: "Reference", Required: false, Min: 0, Max: "*", IsArray: true},
			"definition":  {Name: "definition", Type: "canonical", Required: false, Min: 0, Max: "1"},
		},
		Required: []string{"source"},
	}

	// Patient schema
	s.schemaCache["Patient"] = &FHIRResourceSchema{
		ResourceType: "Patient",
		Fields: map[string]*FHIRFieldDef{
			"id":                   {Name: "id", Type: "id", Required: false, Min: 0, Max: "1"},
			"meta":                 {Name: "meta", Type: "Meta", Required: false, Min: 0, Max: "1"},
			"text":                 {Name: "text", Type: "Narrative", Required: false, Min: 0, Max: "1"},
			"identifier":           {Name: "identifier", Type: "Identifier", Required: false, Min: 0, Max: "*", IsArray: true},
			"active":               {Name: "active", Type: "boolean", Required: false, Min: 0, Max: "1"},
			"name":                 {Name: "name", Type: "HumanName", Required: false, Min: 0, Max: "*", IsArray: true},
			"telecom":              {Name: "telecom", Type: "ContactPoint", Required: false, Min: 0, Max: "*", IsArray: true},
			"gender":               {Name: "gender", Type: "code", Required: false, Min: 0, Max: "1"},
			"birthDate":            {Name: "birthDate", Type: "date", Required: false, Min: 0, Max: "1"},
			"deceased":             {Name: "deceased", Type: "boolean", Required: false, Min: 0, Max: "1"},
			"address":              {Name: "address", Type: "Address", Required: false, Min: 0, Max: "*", IsArray: true},
			"maritalStatus":        {Name: "maritalStatus", Type: "CodeableConcept", Required: false, Min: 0, Max: "1"},
			"multipleBirth":        {Name: "multipleBirth", Type: "boolean", Required: false, Min: 0, Max: "1"},
			"photo":                {Name: "photo", Type: "Attachment", Required: false, Min: 0, Max: "*", IsArray: true},
			"contact":              {Name: "contact", Type: "BackboneElement", Required: false, Min: 0, Max: "*", IsArray: true},
			"communication":        {Name: "communication", Type: "BackboneElement", Required: false, Min: 0, Max: "*", IsArray: true},
			"generalPractitioner":  {Name: "generalPractitioner", Type: "Reference", Required: false, Min: 0, Max: "*", IsArray: true},
			"managingOrganization": {Name: "managingOrganization", Type: "Reference", Required: false, Min: 0, Max: "1"},
			"link":                 {Name: "link", Type: "BackboneElement", Required: false, Min: 0, Max: "*", IsArray: true},
		},
		Required: []string{},
	}

	log.Printf("⚠️ WARNING: Using temporary schema definitions - must implement actual .gz file parsing")
}

// =====================================
// SCHEMA-DRIVEN RESOURCE CREATION
// =====================================

func (s *HL7FHIRTransformService) createResourceFromSchema(resourceType string) (map[string]interface{}, error) {
	if !s.schemaReady {
		return nil, fmt.Errorf("FHIR schemas not loaded")
	}

	schema, exists := s.schemaCache[resourceType]
	if !exists {
		return nil, fmt.Errorf("schema not found for resource type: %s", resourceType)
	}

	resource := map[string]interface{}{
		"resourceType": resourceType,
		"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().UnixNano()),
	}

	// Only initialize required fields from schema
	for _, fieldName := range schema.Required {
		if fieldDef, exists := schema.Fields[fieldName]; exists {
			// Initialize required field based on schema definition
			if err := s.initializeRequiredField(resource, fieldName, fieldDef); err != nil {
				log.Printf("⚠️ Failed to initialize required field %s: %v", fieldName, err)
			}
		}
	}

	log.Printf("✅ Created %s resource from schema with %d required fields", resourceType, len(schema.Required))
	return resource, nil
}

func (s *HL7FHIRTransformService) initializeRequiredField(resource map[string]interface{}, fieldName string, fieldDef *FHIRFieldDef) error {
	switch fieldName {
	case "source":
		// MessageHeader.source is required
		resource["source"] = map[string]interface{}{
			"name":     "HL7 Interface",
			"endpoint": "urn:oid:1.3.6.1.4.1.21367.2005.3.7",
		}
	case "destination":
		// MessageHeader.destination is required array
		resource["destination"] = []map[string]interface{}{
			{
				"name":     "FHIR Server",
				"endpoint": "http://localhost:8080/fhir",
			},
		}
	case "text":
		// Narrative text for robust management
		resource["text"] = map[string]interface{}{
			"status": "generated",
			"div":    fmt.Sprintf("<div xmlns=\"http://www.w3.org/1999/xhtml\">%s resource</div>", resource["resourceType"]),
		}
	default:
		log.Printf("⚠️ Unknown required field initialization: %s", fieldName)
	}

	return nil
}

// =====================================
// SCHEMA-DRIVEN FIELD VALIDATION
// =====================================

func (s *HL7FHIRTransformService) validateFieldAgainstSchema(resourceType, fieldName string) error {
	if !s.schemaReady {
		return fmt.Errorf("FHIR schemas not loaded")
	}

	schema, exists := s.schemaCache[resourceType]
	if !exists {
		return fmt.Errorf("schema not found for resource type: %s", resourceType)
	}

	fieldDef, exists := schema.Fields[fieldName]
	if !exists {
		return fmt.Errorf("field %s not found in %s schema", fieldName, resourceType)
	}

	log.Printf("✅ Field %s.%s validated against schema (type: %s, cardinality: %s)",
		resourceType, fieldName, fieldDef.Type, fieldDef.Cardinality)
	return nil
}

// =====================================
// MAIN TRANSFORMATION METHOD
// =====================================

func (s *HL7FHIRTransformService) Transform(ctx context.Context, request *TransformRequest) (*TransformResponse, error) {
	if !s.schemaReady {
		return nil, fmt.Errorf("FHIR schemas not loaded - cannot perform transformation")
	}

	startTime := time.Now()
	response := s.initializeResponse(request)

	// Extract message type
	messageType, err := s.extractMessageType(request.ParsedHL7Data)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to extract message type: %v", err))
		return response, nil
	}
	response.MessageType = messageType

	log.Printf("🔄 HL7→FHIR transformation: %s (MessageType: %s) [SCHEMA-DRIVEN]", response.RequestID, messageType)

	// Get field mappings
	fieldMappings, err := s.getFieldMappings(ctx, messageType, request.TargetProfile)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get field mappings: %v", err))
		return response, nil
	}

	// Apply transformations using schema
	transformStartTime := time.Now()
	resources, stats, warnings, errors := s.applySchemaBasedTransformations(
		request.ParsedHL7Data,
		messageType,
		fieldMappings,
	)

	s.populateTransformResponse(response, resources, stats, warnings, errors, transformStartTime, startTime, request.CreateBundle)

	log.Printf("✅ HL7→FHIR transformation completed: %s (%d resources, %s) [SCHEMA-DRIVEN]",
		response.RequestID, len(resources), response.Performance.TotalTime)

	return response, nil
}

func (s *HL7FHIRTransformService) initializeResponse(request *TransformRequest) *TransformResponse {
	requestID := request.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("transform_%d", time.Now().UnixNano())
	}

	return &TransformResponse{
		Success:        false,
		RequestID:      requestID,
		FHIRResources:  []map[string]interface{}{},
		ResourceCounts: make(map[string]int),
		Warnings:       []string{},
		Errors:         []string{},
		MappingStats:   MappingStatistics{},
		Performance:    PerformanceMetrics{},
	}
}

func (s *HL7FHIRTransformService) populateTransformResponse(
	response *TransformResponse,
	resources []map[string]interface{},
	stats MappingStatistics,
	warnings, errors []string,
	transformStartTime, startTime time.Time,
	createBundle bool,
) {
	response.FHIRResources = resources
	response.MappingStats = stats
	response.Warnings = append(response.Warnings, warnings...)
	response.Errors = append(response.Errors, errors...)
	response.Performance.TransformTime = time.Since(transformStartTime).String()

	// Count resources by type
	for _, resource := range resources {
		if resourceType, ok := resource["resourceType"].(string); ok {
			response.ResourceCounts[resourceType]++
		}
	}

	// Create bundle if requested
	if createBundle && len(resources) > 0 {
		bundle := s.createBundle(resources, response.RequestID, response.MessageType)
		response.Bundle = bundle
	}

	response.Success = len(response.Errors) == 0
	response.Performance.TotalTime = time.Since(startTime).String()
	response.Performance.ResourcesCreated = len(resources)
}

// =====================================
// SCHEMA-BASED TRANSFORMATION LOGIC
// =====================================

func (s *HL7FHIRTransformService) applySchemaBasedTransformations(
	parsedHL7 map[string]interface{},
	messageType string,
	fieldMappings []FieldMapping,
) ([]map[string]interface{}, MappingStatistics, []string, []string) {

	log.Printf("🔄 Starting schema-based transformations with %d mappings", len(fieldMappings))

	var resources []map[string]interface{}
	var warnings []string
	var errors []string
	stats := MappingStatistics{}

	enhancedSegments := s.extractEnhancedSegments(parsedHL7)
	if enhancedSegments == nil {
		errors = append(errors, "No enhanced segments found in parsed HL7 data")
		return resources, stats, warnings, errors
	}

	resourcesMap := make(map[string]map[string]interface{})

	// Process mappings - only create resources for fields that have HL7 data
	for i, mapping := range fieldMappings {
		log.Printf("Processing mapping %d/%d: %s.%s → %s.%s",
			i+1, len(fieldMappings), mapping.SegmentName, mapping.HL7Field,
			mapping.FHIRResourceType, mapping.FHIRElementPath)

		// Validate field against schema first
		if err := s.validateFieldAgainstSchema(mapping.FHIRResourceType, mapping.FHIRElementPath); err != nil {
			errors = append(errors, fmt.Sprintf("Schema validation failed for %s.%s: %v",
				mapping.FHIRResourceType, mapping.FHIRElementPath, err))
			continue
		}

		// Ensure resource exists (create from schema)
		if _, exists := resourcesMap[mapping.FHIRResourceType]; !exists {
			resource, err := s.createResourceFromSchema(mapping.FHIRResourceType)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to create resource from schema: %v", err))
				continue
			}
			resourcesMap[mapping.FHIRResourceType] = resource
		}

		// Extract HL7 value
		hl7Value, found := s.extractHL7Value(enhancedSegments, mapping)
		if !found {
			if mapping.IsRequired {
				warnings = append(warnings, fmt.Sprintf("Required field %s.%s not found in HL7 data",
					mapping.SegmentName, mapping.HL7Field))
			}
			continue
		}

		if hl7Value == "" {
			continue // Skip empty values
		}

		// Transform and set value using schema
		if s.processSchemaBasedMapping(hl7Value, mapping, resourcesMap, &stats, &warnings) {
			stats.TotalFieldsMapped++
			if mapping.IsRequired {
				stats.RequiredFieldsMapped++
			} else {
				stats.OptionalFieldsMapped++
			}
		}
	}

	// Convert to slice - only include resources with actual data
	for resourceType, resource := range resourcesMap {
		if s.hasMeaningfulContent(resource) {
			log.Printf("Including %s resource with mapped HL7 data", resourceType)
			resources = append(resources, resource)
		} else {
			log.Printf("Skipping %s resource - no HL7 data mapped", resourceType)
		}
	}

	return resources, stats, warnings, errors
}

func (s *HL7FHIRTransformService) processSchemaBasedMapping(
	hl7Value string,
	mapping FieldMapping,
	resourcesMap map[string]map[string]interface{},
	stats *MappingStatistics,
	warnings *[]string,
) bool {
	// Transform value
	transformedValue, err := s.transformValue(hl7Value, mapping)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("Transformation failed for %s.%s: %v",
			mapping.SegmentName, mapping.HL7Field, err))
		return false
	}

	// Set value using schema information
	resource := resourcesMap[mapping.FHIRResourceType]
	if err := s.setFHIRValueFromSchema(resource, mapping.FHIRElementPath, transformedValue, mapping.FHIRResourceType); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("Failed to set FHIR value: %v", err))
		return false
	}

	return true
}

func (s *HL7FHIRTransformService) setFHIRValueFromSchema(resource map[string]interface{}, fieldPath string, value interface{}, resourceType string) error {
	schema := s.schemaCache[resourceType]
	fieldDef, exists := schema.Fields[fieldPath]
	if !exists {
		return fmt.Errorf("field %s not found in schema for %s", fieldPath, resourceType)
	}

	// Set value based on schema cardinality
	if fieldDef.IsArray || fieldDef.Max == "*" {
		// Handle array field
		if existing, exists := resource[fieldPath]; exists {
			if existingArray, ok := existing.([]interface{}); ok {
				if valueArray, isArray := value.([]interface{}); isArray {
					resource[fieldPath] = append(existingArray, valueArray...)
				} else {
					resource[fieldPath] = append(existingArray, value)
				}
			}
		} else {
			if valueArray, isArray := value.([]interface{}); isArray {
				resource[fieldPath] = valueArray
			} else {
				resource[fieldPath] = []interface{}{value}
			}
		}
	} else {
		// Single value field
		resource[fieldPath] = value
	}

	return nil
}

func (s *HL7FHIRTransformService) hasMeaningfulContent(resource map[string]interface{}) bool {
	// Count fields that contain actual HL7-mapped data (not just required defaults)
	meaningfulFields := 0

	for key, value := range resource {
		if key == "resourceType" || key == "id" {
			continue
		}

		// Skip default required fields that weren't populated with HL7 data
		if key == "source" || key == "destination" || key == "text" {
			continue
		}

		if s.isValueMeaningful(value) {
			meaningfulFields++
		}
	}

	return meaningfulFields > 0
}

func (s *HL7FHIRTransformService) isValueMeaningful(value interface{}) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		for _, item := range v {
			if s.isValueMeaningful(item) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		for _, mapValue := range v {
			if s.isValueMeaningful(mapValue) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// =====================================
// FIELD MAPPING & HL7 EXTRACTION
// =====================================

func (s *HL7FHIRTransformService) getFieldMappings(ctx context.Context, messageType, profile string) ([]FieldMapping, error) {
	if s.db != nil {
		mappings, err := s.loadFieldMappingsFromDB(ctx, messageType, profile)
		if err == nil && len(mappings) > 0 {
			log.Printf("Using database field mappings (%d mappings)", len(mappings))
			return mappings, nil
		}
	}

	return nil, fmt.Errorf("no field mappings found for message type: %s (no fallback mappings)", messageType)
}

func (s *HL7FHIRTransformService) loadFieldMappingsFromDB(ctx context.Context, messageType, profile string) ([]FieldMapping, error) {
	query := `
		SELECT id, segment_name, hl7_field, hl7_component, 
		       fhir_resource_type, fhir_element_path, 
		       COALESCE(data_type_transform, '') as data_type_transform,
		       is_required, cardinality, 
		       COALESCE(transformation_rules, '{}') as transformation_rules
		FROM field_element_mappings fem
		WHERE EXISTS (
			SELECT 1 FROM message_fhir_templates mft 
			WHERE mft.message_type = $1 
			AND mft.fhir_resources::jsonb ? fem.fhir_resource_type
		)
		ORDER BY fem.segment_name, fem.hl7_field
	`

	rows, err := s.db.QueryContext(ctx, query, messageType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []FieldMapping
	for rows.Next() {
		mapping := s.scanFieldMapping(rows)
		if mapping != nil {
			mappings = append(mappings, *mapping)
		}
	}

	return mappings, rows.Err()
}

func (s *HL7FHIRTransformService) scanFieldMapping(rows *sql.Rows) *FieldMapping {
	var mapping FieldMapping
	var component sql.NullString
	var dataTypeTransform sql.NullString
	var rulesJSON sql.NullString

	err := rows.Scan(
		&mapping.ID,
		&mapping.SegmentName,
		&mapping.HL7Field,
		&component,
		&mapping.FHIRResourceType,
		&mapping.FHIRElementPath,
		&dataTypeTransform,
		&mapping.IsRequired,
		&mapping.Cardinality,
		&rulesJSON,
	)
	if err != nil {
		log.Printf("Failed to scan field mapping: %v", err)
		return nil
	}

	if component.Valid {
		mapping.HL7Component = component.String
	}

	if dataTypeTransform.Valid {
		mapping.DataTypeTransform = dataTypeTransform.String
	} else {
		// Auto-detect missing transforms based on FHIR element path
		mapping.DataTypeTransform = s.inferTransformFromFHIRPath(mapping.FHIRElementPath)
	}

	if rulesJSON.Valid && rulesJSON.String != "" && rulesJSON.String != "{}" {
		var rules map[string]interface{}
		if err := json.Unmarshal([]byte(rulesJSON.String), &rules); err == nil {
			mapping.TransformationRules = rules
		}
	}

	return &mapping
}

func (s *HL7FHIRTransformService) inferTransformFromFHIRPath(fhirPath string) string {
	switch fhirPath {
	case "gender":
		return "gender_mapping"
	case "eventCoding":
		return "msh9_to_coding"
	case "telecom":
		return "xtn_to_contactpoint"
	case "identifier":
		return "cx_to_identifier"
	case "name":
		return "xpn_to_humanname"
	case "address":
		return "xad_to_address"
	case "birthDate":
		return "ts_to_date"
	default:
		return ""
	}
}

// =====================================
// HL7 VALUE EXTRACTION
// =====================================

func (s *HL7FHIRTransformService) extractHL7Value(segments map[string]interface{}, mapping FieldMapping) (string, bool) {
	expectedFieldKey := s.buildFieldKey(mapping)

	segment, segmentExists := segments[mapping.SegmentName].(map[string]interface{})
	if !segmentExists {
		return "", false
	}

	fields, fieldsExists := segment["fields"].([]interface{})
	if !fieldsExists {
		return "", false
	}

	targetField, fieldFound := s.findTargetField(fields, expectedFieldKey)
	if !fieldFound {
		return "", false
	}

	return s.extractValueFromField(targetField, mapping, expectedFieldKey)
}

func (s *HL7FHIRTransformService) buildFieldKey(mapping FieldMapping) string {
	if strings.Contains(mapping.HL7Field, ".") {
		return mapping.HL7Field
	}
	return fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field)
}

func (s *HL7FHIRTransformService) findTargetField(fields []interface{}, expectedFieldKey string) (map[string]interface{}, bool) {
	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, keyOk := fieldMap["key"].(string)
		if !keyOk || key != expectedFieldKey {
			continue
		}

		return fieldMap, true
	}
	return nil, false
}

func (s *HL7FHIRTransformService) extractValueFromField(
	targetField map[string]interface{},
	mapping FieldMapping,
	fieldKey string,
) (string, bool) {
	if mapping.HL7Component == "" {
		if value, valueOk := targetField["value"].(string); valueOk {
			return value, true
		}
		return "", false
	}

	return s.extractComponentFromSubfields(targetField, mapping, fieldKey)
}

func (s *HL7FHIRTransformService) extractComponentFromSubfields(
	field map[string]interface{},
	mapping FieldMapping,
	fieldKey string) (string, bool) {

	if subfields, subfieldOk := field["subfields"].([]interface{}); subfieldOk {
		possibleKeys := []string{
			fmt.Sprintf("%s.%s", fieldKey, mapping.HL7Component),
			fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component),
		}

		for _, expectedKey := range possibleKeys {
			for _, subfield := range subfields {
				subfieldMap, ok := subfield.(map[string]interface{})
				if !ok {
					continue
				}

				subfieldKey, keyOk := subfieldMap["key"].(string)
				if !keyOk || subfieldKey != expectedKey {
					continue
				}

				if value, valueOk := subfieldMap["value"].(string); valueOk {
					return value, true
				}
			}
		}
	}

	return s.manualParseFieldValue(field, mapping)
}

func (s *HL7FHIRTransformService) manualParseFieldValue(field map[string]interface{}, mapping FieldMapping) (string, bool) {
	if value, valueOk := field["value"].(string); valueOk && value != "" {
		enhancedFieldData := hl7.ParseHL7Field(value)
		if enhancedFieldData != nil && len(enhancedFieldData.Repetitions) > 0 {
			rep := enhancedFieldData.Repetitions[0]

			componentPos, err := strconv.Atoi(mapping.HL7Component)
			if err != nil {
				return "", false
			}

			if componentPos > 0 && componentPos <= len(rep.Components) {
				componentValue := rep.Components[componentPos-1].RawValue
				return componentValue, true
			}
		}

		return value, true
	}

	return "", false
}

// =====================================
// VALUE TRANSFORMATION
// =====================================

func (s *HL7FHIRTransformService) transformValue(hl7Value string, mapping FieldMapping) (interface{}, error) {
	if hl7Value == "" {
		return nil, nil
	}

	switch mapping.DataTypeTransform {
	case "xtn_to_contactpoint", "phone_to_contactpoint":
		return s.transformPhoneToContactPointEnhanced(hl7Value, mapping.TransformationRules), nil
	case "gender_mapping", "administrative_sex", "gender":
		return s.transformGender(hl7Value), nil
	case "cx_to_identifier":
		return s.transformCXToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "xpn_to_humanname":
		return s.transformXPNToHumanName(hl7Value, mapping.TransformationRules), nil
	case "xad_to_address":
		return s.transformXADToAddress(hl7Value, mapping.TransformationRules), nil
	case "email_to_contactpoint":
		return s.transformEmailToContactPoint(hl7Value, mapping.TransformationRules), nil
	case "ts_to_datetime", "ts_to_date":
		return s.transformTSToDate(hl7Value), nil
	case "ce_to_codeableconcept":
		return s.transformCEToCodeableConcept(hl7Value, mapping.TransformationRules), nil
	case "ssn_to_identifier":
		return s.transformSSNToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "account_to_identifier":
		return s.transformAccountToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "msh9_to_coding":
		return s.transformMSH9ToCoding(hl7Value), nil
	case "":
		return hl7Value, nil
	default:
		return hl7Value, nil
	}
}

// =====================================
// DATA TYPE TRANSFORMERS
// =====================================

func (s *HL7FHIRTransformService) transformMSH9ToCoding(msh9 string) map[string]interface{} {
	components := strings.Split(msh9, "^")
	eventCoding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
	}

	if len(components) > 0 && components[0] != "" {
		messageType := components[0]
		triggerEvent := ""

		if len(components) > 1 && components[1] != "" {
			triggerEvent = components[1]
		}

		if triggerEvent != "" {
			eventCoding["code"] = fmt.Sprintf("%s^%s", messageType, triggerEvent)
			eventCoding["display"] = fmt.Sprintf("%s %s", messageType, triggerEvent)
		} else {
			eventCoding["code"] = messageType
			eventCoding["display"] = messageType
		}
	}

	return eventCoding
}

func (s *HL7FHIRTransformService) transformGender(gender string) string {
	upperGender := strings.ToUpper(strings.TrimSpace(gender))

	if s.db != nil {
		query := `SELECT fhir_code FROM value_set_mappings 
				 WHERE mapping_name = 'administrative_sex' AND hl7_value = $1 LIMIT 1`

		var fhirCode string
		if err := s.db.QueryRow(query, upperGender).Scan(&fhirCode); err == nil {
			return fhirCode
		}
	}

	// Direct mapping fallback
	switch upperGender {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "O", "OTHER":
		return "other"
	case "U", "UNKNOWN":
		return "unknown"
	default:
		return "unknown"
	}
}

func (s *HL7FHIRTransformService) transformPhoneToContactPointEnhanced(phone string, rules map[string]interface{}) interface{} {
	if phone == "" {
		return nil
	}

	if strings.Contains(phone, "^") {
		return s.processXTNField(phone, rules)
	}

	return s.createSimpleContactPoint(phone, rules)
}

func (s *HL7FHIRTransformService) processXTNField(phone string, rules map[string]interface{}) interface{} {
	var contactPoints []map[string]interface{}

	enhancedFieldData := hl7.ParseHL7Field(phone)
	if enhancedFieldData == nil || len(enhancedFieldData.Repetitions) == 0 {
		components := strings.Split(phone, "^")
		for _, component := range components {
			component = strings.TrimSpace(component)
			if component == "" {
				continue
			}

			var contactPoint map[string]interface{}
			if strings.Contains(component, "@") {
				contactPoint = map[string]interface{}{
					"system": "email",
					"use":    "home",
					"value":  component,
				}
			} else {
				contactPoint = map[string]interface{}{
					"system": "phone",
					"use":    "home",
					"value":  component,
				}
			}
			contactPoints = append(contactPoints, contactPoint)
		}
	} else {
		for _, repetition := range enhancedFieldData.Repetitions {
			for compIndex, component := range repetition.Components {
				value := strings.TrimSpace(component.RawValue)
				if value == "" {
					continue
				}

				contactPoint := s.createContactPointFromComponent(value, compIndex+1, rules)
				contactPoints = append(contactPoints, contactPoint)
			}
		}
	}

	if len(contactPoints) == 1 {
		return contactPoints[0]
	}
	return contactPoints
}

func (s *HL7FHIRTransformService) createContactPointFromComponent(value string, componentNum int, rules map[string]interface{}) map[string]interface{} {
	var contactPoint map[string]interface{}

	if componentNum == 1 {
		if strings.Contains(value, "@") {
			contactPoint = map[string]interface{}{
				"system": "email",
				"use":    "home",
				"value":  value,
			}
		} else {
			contactPoint = map[string]interface{}{
				"system": "phone",
				"use":    "home",
				"value":  value,
			}
		}
	} else if componentNum == 4 || componentNum == 5 {
		if strings.Contains(value, "@") {
			contactPoint = map[string]interface{}{
				"system": "email",
				"use":    "home",
				"value":  value,
			}
		} else {
			contactPoint = map[string]interface{}{
				"system": "phone",
				"use":    "work",
				"value":  value,
			}
		}
	} else {
		if strings.Contains(value, "@") {
			contactPoint = map[string]interface{}{
				"system": "email",
				"use":    "home",
				"value":  value,
			}
		} else {
			contactPoint = map[string]interface{}{
				"system": "phone",
				"use":    "other",
				"value":  value,
			}
		}
	}

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

func (s *HL7FHIRTransformService) createSimpleContactPoint(phone string, rules map[string]interface{}) map[string]interface{} {
	contactPoint := map[string]interface{}{
		"system": "phone",
		"use":    "home",
		"value":  phone,
	}

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

func (s *HL7FHIRTransformService) transformCXToIdentifier(cx string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(cx, "^")
	identifier := map[string]interface{}{
		"use": "usual",
	}

	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformService) transformXPNToHumanName(xpn string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xpn, "^")
	name := map[string]interface{}{
		"use": "official",
	}

	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}

	var given []string
	if len(components) > 1 && components[1] != "" {
		given = append(given, components[1])
	}
	if len(components) > 2 && components[2] != "" {
		given = append(given, components[2])
	}
	if len(given) > 0 {
		name["given"] = given
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			name["use"] = use
		}
	}

	return name
}

func (s *HL7FHIRTransformService) transformXADToAddress(xad string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(xad, "^")
	address := map[string]interface{}{
		"use": "home",
	}

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

	if len(components) > 2 && components[2] != "" {
		address["city"] = components[2]
	}
	if len(components) > 3 && components[3] != "" {
		address["state"] = components[3]
	}
	if len(components) > 4 && components[4] != "" {
		address["postalCode"] = components[4]
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			address["use"] = use
		}
	}

	return address
}

func (s *HL7FHIRTransformService) transformEmailToContactPoint(email string, rules map[string]interface{}) map[string]interface{} {
	contactPoint := map[string]interface{}{
		"system": "email",
		"use":    "home",
		"value":  email,
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

func (s *HL7FHIRTransformService) transformTSToDate(ts string) string {
	if len(ts) >= 8 {
		year := ts[0:4]
		month := ts[4:6]
		day := ts[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return ts
}

func (s *HL7FHIRTransformService) transformCEToCodeableConcept(ce string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(ce, "^")
	codeableConcept := map[string]interface{}{
		"coding": []map[string]interface{}{},
	}

	if len(components) > 0 && components[0] != "" {
		coding := map[string]interface{}{
			"code": components[0],
		}

		if len(components) > 1 && components[1] != "" {
			coding["display"] = components[1]
		}

		if len(components) > 2 && components[2] != "" {
			coding["system"] = components[2]
		}

		if rules != nil {
			if system, ok := rules["system"].(string); ok {
				coding["system"] = system
			}
		}

		codeableConcept["coding"] = []map[string]interface{}{coding}

		if display, ok := coding["display"].(string); ok {
			codeableConcept["text"] = display
		}
	}

	return codeableConcept
}

func (s *HL7FHIRTransformService) transformSSNToIdentifier(ssn string, rules map[string]interface{}) map[string]interface{} {
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

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformService) transformAccountToIdentifier(account string, rules map[string]interface{}) map[string]interface{} {
	identifier := map[string]interface{}{
		"use":   "secondary",
		"value": account,
		"type": map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":    "AN",
					"display": "Account Number",
					"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		},
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

// =====================================
// UTILITY METHODS
// =====================================

func (s *HL7FHIRTransformService) extractMessageType(parsedHL7 map[string]interface{}) (string, error) {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if messageType, exists := data["messageType"].(map[string]interface{}); exists {
			if name, nameOk := messageType["name"].(string); nameOk {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("message type not found in parsed HL7 data")
}

func (s *HL7FHIRTransformService) extractEnhancedSegments(parsedHL7 map[string]interface{}) map[string]interface{} {
	if parsedHL7 == nil {
		return nil
	}

	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{}); ok {
			return enhancedSegments
		}
	}

	return nil
}

func (s *HL7FHIRTransformService) createBundle(resources []map[string]interface{}, requestID, messageType string) map[string]interface{} {
	entries := make([]map[string]interface{}, len(resources))

	for i, resource := range resources {
		resourceID := ""
		if id, exists := resource["id"].(string); exists {
			resourceID = id
		}

		entries[i] = map[string]interface{}{
			"fullUrl":  fmt.Sprintf("urn:uuid:%s", resourceID),
			"resource": resource,
		}
	}

	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           fmt.Sprintf("bundle-%s", requestID),
		"type":         "message",
		"timestamp":    time.Now().Format(time.RFC3339),
		"entry":        entries,
	}
}
