// services/hl7_fhir_transform_service.go
//
// Shared request/response/mapping types for the HL7→FHIR transformation
// services. Originally this file also held HL7FHIRTransformService (v1), a
// full transformation implementation — deleted 2026-08-16 as confirmed dead
// code (NewHL7FHIRTransformService had zero callers anywhere in the repo;
// its "schema" logic was a hand-built, hardcoded 2-resource-type stub
// explicitly marked TEMPORARY/TODO, unrelated to the real FHIR schema
// system). The types below are still real and shared with
// HL7FHIRTransformServiceV3 (services/hl7_fhir_transform_service_v3.go),
// which is the actual production implementation.
package services

import (
	"ezhealthkonnect/services/hl7assembly"
)

// =====================================
// REQUEST/RESPONSE STRUCTURES
// =====================================

type TransformRequest struct {
	ParsedHL7Data  map[string]interface{} `json:"parsedHL7Data" binding:"required"`
	MessageType    string                 `json:"messageType,omitempty"` // OOB: Injected from pipeline config
	TargetProfile  string                 `json:"targetProfile,omitempty"`
	FHIRVersion    string                 `json:"fhirVersion,omitempty"`
	CreateBundle   bool                   `json:"createBundle,omitempty"`
	ValidationMode string                 `json:"validationMode,omitempty"`
	InterfaceID    string                 `json:"interfaceId,omitempty"`
	RequestID      string                 `json:"requestId,omitempty"`
	// SkipAssembly suppresses the built-in ORU post-processing pass.
	// Set by HL7FHIRMappingExecutor when running inside a pipeline that has an
	// hl7.assemble_observations step — avoids doing the work twice.
	SkipAssembly bool `json:"skipAssembly,omitempty"`
	// AssemblyRules selectively disables individual OOB assembly transforms.
	AssemblyRules hl7assembly.AssemblyRules `json:"assemblyRules,omitempty"`
	// EmbeddedMappings are wizard-saved field mappings stored directly in the
	// pipeline step config. When present they take priority over DB lookups.
	EmbeddedMappings []map[string]interface{} `json:"embeddedMappings,omitempty"`
}

type TransformResponse struct {
	Success          bool                     `json:"success"`
	RequestID        string                   `json:"requestId"`
	MessageType      string                   `json:"messageType"`
	FHIRResources    []map[string]interface{} `json:"fhirResources"`
	AtomicMappings   []AtomicMapping          `json:"atomicMappings"`
	Bundle           map[string]interface{}   `json:"bundle,omitempty"`
	ResourceCounts   map[string]int           `json:"resourceCounts"`
	MappingStats     MappingStatistics        `json:"mappingStats"`
	Warnings         []string                 `json:"warnings"`
	Errors           []string                 `json:"errors"`
	Performance      PerformanceMetrics       `json:"performance"`
	ValidationIssues []ValidationIssue        `json:"validationIssues,omitempty"`
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
	// HL7DataType is the HL7 data type from the real schema (e.g. "TS", "CX", "XPN").
	// Populated by enrichMappingsWithDataTypes; empty when schema is unavailable.
	HL7DataType string `json:"hl7DataType,omitempty"`
	// FHIRDataType is the FHIR element data type from the FHIR schema (e.g. "dateTime", "CodeableConcept").
	// Populated by enrichMappingsWithDataTypes; empty when schema is unavailable.
	FHIRDataType string `json:"fhirDataType,omitempty"`
	// Confidence is the semantic matcher score from the OOB template generator (0.0–1.0).
	// Stored in the template JSON and forwarded to the frontend for display.
	Confidence float64 `json:"confidence,omitempty"`
	// StaticValue carries a literal constant for DataTypeTransform == "static_value" mappings.
	// When set, HL7 extraction is skipped and this value is written directly to the FHIR path.
	StaticValue string `json:"staticValue,omitempty"`
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
