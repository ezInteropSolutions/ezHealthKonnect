// services/hl7_fhir_transform_service_v2.go
// Updated schema-driven HL7 to FHIR transformation service
//
// 🎯 PRINCIPLE: MODULAR ARCHITECTURE
// - Use dedicated transformers for each resource type
// - Schema-first validation and creation
// - Fixed validation issues (telecom array, MSH9 coding, narratives)
// - Follows dos and don'ts guidelines

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"ezhealthkonnect/services/schema"
	"ezhealthkonnect/services/transformers"
)

// =====================================
// UPDATED TRANSFORM SERVICE
// =====================================

type HL7FHIRTransformServiceV2 struct {
	db                 *sql.DB
	schemaLoader       *schema.FHIRSchemaAdapter
	genericTransformer *transformers.GenericResourceTransformer
	schemaReady        bool
}

func NewHL7FHIRTransformServiceV2(database *sql.DB) *HL7FHIRTransformServiceV2 {
	// Read from environment variable, fallback to default
	schemaDirectory := os.Getenv("FHIR_SCHEMA_DIR")
	if schemaDirectory == "" {
		schemaDirectory = "/app/schemas/fhir" // Docker default
	}

	log.Printf("🔧 Using FHIR schema directory: %s", schemaDirectory)

	schemaAdapter := schema.NewFHIRSchemaAdapter(database, schemaDirectory)

	service := &HL7FHIRTransformServiceV2{
		db:           database,
		schemaLoader: schemaAdapter,
		schemaReady:  true,
	}

	// Initialize single generic transformer
	service.genericTransformer = transformers.NewGenericResourceTransformer(schemaAdapter, database)

	log.Printf("✅ HL7FHIRTransformServiceV2 initialized with generic transformer")
	return service
}

// =====================================
// MAIN TRANSFORMATION METHOD
// =====================================

func (s *HL7FHIRTransformServiceV2) Transform(
	ctx context.Context,
	request *TransformRequest,
) (*TransformResponse, error) {
	startTime := time.Now()
	response := s.initializeResponse(request)

	log.Printf("🔄 HL7→FHIR transformation v2: %s [MODULAR + SCHEMA-DRIVEN]", response.RequestID)

	// Extract message type
	messageType, err := s.extractMessageType(request.ParsedHL7Data)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to extract message type: %v", err))
		return response, nil
	}
	response.MessageType = messageType

	// Get field mappings
	fieldMappings, err := s.getFieldMappings(ctx, messageType, request.TargetProfile)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get field mappings: %v", err))
		return response, nil
	}

	// Transform using modular approach
	transformStartTime := time.Now()
	resources, stats, warnings, errors := s.performModularTransformation(
		request.ParsedHL7Data,
		messageType,
		fieldMappings,
	)

	// Populate response
	s.populateTransformResponse(
		response,
		resources,
		stats,
		warnings,
		errors,
		transformStartTime,
		startTime,
		request.CreateBundle,
	)

	log.Printf("✅ HL7→FHIR transformation v2 completed: %s (%d resources, %s)",
		response.RequestID, len(resources), response.Performance.TotalTime)

	return response, nil
}

// =====================================
// MODULAR TRANSFORMATION LOGIC
// =====================================

func (s *HL7FHIRTransformServiceV2) performModularTransformation(
	parsedHL7 map[string]interface{},
	messageType string,
	fieldMappings []transformers.FieldMapping,
) ([]map[string]interface{}, MappingStatistics, []string, []string) {

	var allResources []map[string]interface{}
	var allWarnings []string
	var allErrors []string
	stats := MappingStatistics{}

	log.Printf("🔄 Starting generic transformation with %d mappings", len(fieldMappings))

	// Group mappings by resource type
	mappingsByResource := s.groupMappingsByResourceType(fieldMappings)

	// Transform each resource type using the generic transformer
	for resourceType, mappings := range mappingsByResource {
		log.Printf("🔧 Creating %s with %d mappings", resourceType, len(mappings))

		resource, warnings, errors := s.genericTransformer.CreateResource(
			resourceType,
			parsedHL7,
			mappings,
		)

		if resource != nil {
			allResources = append(allResources, resource)
			stats.TotalFieldsMapped += len(mappings)
		}

		allWarnings = append(allWarnings, warnings...)
		allErrors = append(allErrors, errors...)
	}

	log.Printf("✅ Generic transformation completed: %d resources created", len(allResources))
	return allResources, stats, allWarnings, allErrors
}

func (s *HL7FHIRTransformServiceV2) groupMappingsByResourceType(
	mappings []transformers.FieldMapping,
) map[string][]transformers.FieldMapping {
	grouped := make(map[string][]transformers.FieldMapping)

	for _, mapping := range mappings {
		resourceType := mapping.FHIRResourceType
		grouped[resourceType] = append(grouped[resourceType], mapping)
	}

	return grouped
}

// =====================================
// FIELD MAPPING RETRIEVAL
// =====================================

func (s *HL7FHIRTransformServiceV2) getFieldMappings(
	ctx context.Context,
	messageType, profile string,
) ([]transformers.FieldMapping, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `
		SELECT segment_name, hl7_field, 
		       COALESCE(hl7_component, '') as hl7_component,
		       fhir_resource_type, fhir_element_path, 
		       COALESCE(data_type_transform, '') as data_type_transform,
		       is_required,
		       COALESCE(transformation_rules, '{}') as transformation_rules
		FROM field_element_mappings fem
		WHERE EXISTS (
			SELECT 1 FROM message_fhir_templates mft 
			WHERE mft.message_type = $1 
			AND mft.fhir_resources::jsonb ? fem.fhir_resource_type
		)
		ORDER BY fem.fhir_resource_type, fem.segment_name, fem.hl7_field
	`

	rows, err := s.db.QueryContext(ctx, query, messageType)
	if err != nil {
		return nil, fmt.Errorf("failed to query field mappings: %w", err)
	}
	defer rows.Close()

	var mappings []transformers.FieldMapping
	for rows.Next() {
		var mapping transformers.FieldMapping
		var component sql.NullString
		var dataTypeTransform sql.NullString
		var rulesJSON sql.NullString

		err := rows.Scan(
			&mapping.SegmentName,
			&mapping.HL7Field,
			&component,
			&mapping.FHIRResourceType,
			&mapping.FHIRElementPath,
			&dataTypeTransform,
			&mapping.IsRequired,
			&rulesJSON,
		)
		if err != nil {
			log.Printf("⚠️ Failed to scan field mapping: %v", err)
			continue
		}

		// Set optional fields
		if component.Valid {
			mapping.HL7Component = component.String
		}
		if dataTypeTransform.Valid {
			mapping.DataTypeTransform = dataTypeTransform.String
		}

		// Parse transformation rules
		if rulesJSON.Valid && rulesJSON.String != "" && rulesJSON.String != "{}" {
			var rules map[string]interface{}
			if err := json.Unmarshal([]byte(rulesJSON.String), &rules); err == nil {
				mapping.TransformationRules = rules
			}
		}

		mappings = append(mappings, mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating field mappings: %w", err)
	}

	log.Printf("📋 Retrieved %d field mappings for message type: %s", len(mappings), messageType)
	return mappings, nil
}

// =====================================
// UTILITY METHODS (Reused from original)
// =====================================

func (s *HL7FHIRTransformServiceV2) initializeResponse(request *TransformRequest) *TransformResponse {
	requestID := request.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("transform_v2_%d", time.Now().UnixNano())
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

func (s *HL7FHIRTransformServiceV2) extractMessageType(parsedHL7 map[string]interface{}) (string, error) {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if messageType, exists := data["messageType"].(map[string]interface{}); exists {
			if name, nameOk := messageType["name"].(string); nameOk {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("message type not found in parsed HL7 data")
}

func (s *HL7FHIRTransformServiceV2) populateTransformResponse(
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

func (s *HL7FHIRTransformServiceV2) createBundle(
	resources []map[string]interface{},
	requestID, messageType string,
) map[string]interface{} {
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

// =====================================
// NOTE: REMOVED DUPLICATE TYPE DEFINITIONS
// The types TransformRequest, TransformResponse, etc.
// are already defined in hl7_fhir_transform_service.go
// =====================================
