// mapping_generator/schema_analyzer.go
// Programmatic HL7-FHIR mapping generation engine
// Designed for million+ message processing with OOB templates

package mapping_generator

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// =====================================
// Core Schema Structures
// =====================================

type SchemaAnalyzer struct {
	HL7Schemas    map[string]map[string]*HL7MessageSchema // version -> messageType -> schema
	FHIRSchemas   map[string]*FHIRResourceSchema          // resourceType -> schema
	MappingRules  []MappingRule
	Transformers  map[string]TransformFunction
	schemasPath   string
}

type HL7MessageSchema struct {
	MessageType string                  `json:"messageType"`
	Version     string                  `json:"version"`
	Description string                  `json:"description"`
	Structure   map[string]*HL7Segment  `json:"structure"`
	SegmentOrder []string               `json:"segmentOrder,omitempty"`
}

type HL7Segment struct {
	Name        string                `json:"name,omitempty"`
	Description string                `json:"description"`
	Usage       string                `json:"usage"`       // R, O, C
	Repeat      string                `json:"repeat"`      // 1, *, [1..n]
	Sequence    string                `json:"sequence"`
	Type        string                `json:"type"`
	Fields      map[string]*HL7Field  `json:"fields"`
}

type HL7Field struct {
	Name         string                     `json:"name"`
	DataType     string                     `json:"dataType"`
	DataTypeName string                     `json:"dataTypeName"`
	Length       int                        `json:"length"`
	Usage        string                     `json:"usage"`
	Repeat       string                     `json:"repeat"`
	Description  string                     `json:"description"`
	TableID      string                     `json:"tableId"`
	TableName    string                     `json:"tableName"`
	Components   map[string]*HL7Component   `json:"components,omitempty"`
	Values       map[string]interface{}     `json:"values,omitempty"`
}

type HL7Component struct {
	Name         string                 `json:"name"`
	DataType     string                 `json:"dataType"`
	DataTypeName string                 `json:"dataTypeName"`
	Length       int                    `json:"length"`
	Usage        string                 `json:"usage"`
	Repeat       string                 `json:"repeat"`
	Description  string                 `json:"description"`
	TableID      string                 `json:"tableId"`
	TableName    string                 `json:"tableName"`
	Values       map[string]interface{} `json:"values,omitempty"`
}

type FHIRResourceSchema struct {
	ResourceType string                      `json:"resourceType"`
	Version      string                      `json:"version"`
	Name         string                      `json:"name"`
	Description  string                      `json:"description"`
	BaseResource string                      `json:"baseResource"`
	Elements     map[string]*FHIRElement     `json:"elements"`
}

type FHIRElement struct {
	Path         string   `json:"path"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	DataType     string   `json:"dataType"`
	Cardinality  string   `json:"cardinality"`
	Required     bool     `json:"required"`
	MustSupport  bool     `json:"mustSupport"`
	IsModifier   bool     `json:"isModifier"`
	IsSummary    bool     `json:"isSummary"`
	Constraints  []string `json:"constraints"`
}

// =====================================
// Mapping Rule Structures
// =====================================

type MappingRule struct {
	HL7Path       string            `json:"hl7Path"`       // "PID.5.1"
	FHIRPath      string            `json:"fhirPath"`      // "Patient.name[0].family"
	Transform     string            `json:"transform"`     // "direct", "gender_code", etc.
	Condition     string            `json:"condition"`     // Optional: "PID.5.7 == 'L'"
	Required      bool              `json:"required"`
	Priority      int               `json:"priority"`
	Confidence    float64           `json:"confidence"`    // 0.0 - 1.0
	ValueMap      map[string]string `json:"valueMap"`
	MessageTypes  []string          `json:"messageTypes"`  // Applicable message types
}

type TransformFunction struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`        // "direct", "valueset", "date_conversion"
	Description string            `json:"description"`
	Validation  string            `json:"validation"`
	Parameters  map[string]string `json:"parameters"`
}

type MappingTemplate struct {
	MessageType  string                        `json:"messageType"`
	Version      string                        `json:"version"`
	GeneratedAt  time.Time                     `json:"generatedAt"`
	Coverage     CoverageStats                 `json:"coverage"`
	Resources    map[string]*ResourceMapping   `json:"resources"`
	Transforms   map[string]*TransformFunction `json:"transforms"`
}

type ResourceMapping struct {
	ResourceType string          `json:"resourceType"`
	Priority     int             `json:"priority"`
	Required     bool            `json:"required"`
	References   []ResourceRef   `json:"references,omitempty"`
	Mappings     []FieldMapping  `json:"mappings"`
}

type ResourceRef struct {
	Path   string `json:"path"`   // "subject"
	Target string `json:"target"` // "Patient"
	Field  string `json:"field"`  // "id"
}

type FieldMapping struct {
	HL7Path    string            `json:"hl7Path"`
	FHIRPath   string            `json:"fhirPath"`
	Transform  string            `json:"transform"`
	Required   bool              `json:"required"`
	Confidence float64           `json:"confidence"`
	ValueMap   map[string]string `json:"valueMap,omitempty"`
}

type CoverageStats struct {
	TotalHL7Fields   int     `json:"totalHL7Fields"`
	MappedFields     int     `json:"mappedFields"`
	CoveragePercent  float64 `json:"coveragePercent"`
	RequiredCoverage float64 `json:"requiredCoverage"`
}

// =====================================
// Core Schema Analysis Functions
// =====================================

func NewSchemaAnalyzer(schemasPath string) *SchemaAnalyzer {
	return &SchemaAnalyzer{
		HL7Schemas:   make(map[string]map[string]*HL7MessageSchema),
		FHIRSchemas:  make(map[string]*FHIRResourceSchema),
		MappingRules: make([]MappingRule, 0),
		Transformers: make(map[string]TransformFunction),
		schemasPath:  schemasPath,
	}
}

func (sa *SchemaAnalyzer) LoadHL7Schemas(versions []string) error {
	for _, version := range versions {
		versionPath := filepath.Join(sa.schemasPath, "hl7", version)
		if _, err := os.Stat(versionPath); os.IsNotExist(err) {
			continue
		}

		sa.HL7Schemas[version] = make(map[string]*HL7MessageSchema)

		err := filepath.Walk(versionPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !strings.HasSuffix(info.Name(), ".gz") {
				return nil
			}

			messageType := strings.TrimSuffix(info.Name(), ".gz")
			schema, err := sa.loadHL7Schema(path)
			if err != nil {
				fmt.Printf("Warning: Failed to load %s: %v\n", path, err)
				return nil
			}

			schema.MessageType = messageType
			schema.Version = version
			sa.HL7Schemas[version][messageType] = schema
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to load HL7 schemas for version %s: %w", version, err)
		}
	}

	return nil
}

func (sa *SchemaAnalyzer) loadHL7Schema(filePath string) (*HL7MessageSchema, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	var schema HL7MessageSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}

	return &schema, nil
}

func (sa *SchemaAnalyzer) LoadFHIRSchemas() error {
	fhirPath := filepath.Join(sa.schemasPath, "fhir", "R4", "resources")

	return filepath.Walk(fhirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(info.Name(), ".gz") {
			return nil
		}

		resourceType := strings.TrimSuffix(info.Name(), ".gz")
		schema, err := sa.loadFHIRSchema(path)
		if err != nil {
			fmt.Printf("Warning: Failed to load FHIR resource %s: %v\n", resourceType, err)
			return nil
		}

		schema.ResourceType = resourceType
		sa.FHIRSchemas[resourceType] = schema
		return nil
	})
}

func (sa *SchemaAnalyzer) loadFHIRSchema(filePath string) (*FHIRResourceSchema, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	var schema FHIRResourceSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}

	return &schema, nil
}

// =====================================
// Analysis Functions
// =====================================

func (sa *SchemaAnalyzer) AnalyzeMessageType(version, messageType string) (*HL7MessageSchema, error) {
	versionSchemas, exists := sa.HL7Schemas[version]
	if !exists {
		return nil, fmt.Errorf("version %s not loaded", version)
	}

	schema, exists := versionSchemas[messageType]
	if !exists {
		return nil, fmt.Errorf("message type %s not found in version %s", messageType, version)
	}

	return schema, nil
}

func (sa *SchemaAnalyzer) GetTargetFHIRResources(messageType string) []string {
	// Define standard FHIR resource mappings for each message type family
	resourceMappings := map[string][]string{
		"ADT": {"Patient", "Encounter", "MessageHeader"},
		"ORU": {"Patient", "Observation", "DiagnosticReport", "Specimen", "MessageHeader"},
		"ORM": {"Patient", "ServiceRequest", "Practitioner", "MessageHeader"},
		"MDM": {"Patient", "DocumentReference", "Practitioner", "MessageHeader"},
		"SIU": {"Patient", "Appointment", "Practitioner", "Location", "MessageHeader"},
		"DFT": {"Patient", "Account", "ChargeItem", "Encounter", "MessageHeader"},
		"RDE": {"Patient", "MedicationRequest", "Medication", "MessageHeader"},
		"RXE": {"MedicationRequest", "Medication"},
	}

	// Extract message family from messageType (e.g., "ADT_A01" -> "ADT")
	parts := strings.Split(messageType, "_")
	if len(parts) == 0 {
		return []string{"MessageHeader"} // Default fallback
	}

	messageFamily := parts[0]
	if resources, exists := resourceMappings[messageFamily]; exists {
		return resources
	}

	return []string{"MessageHeader"} // Default fallback
}

func (sa *SchemaAnalyzer) GetFieldPaths(schema *HL7MessageSchema) []string {
	var paths []string

	for segmentName, segment := range schema.Structure {
		for fieldName, field := range segment.Fields {
			// Add main field path
			paths = append(paths, fieldName)

			// Add component paths if they exist
			if field.Components != nil {
				for componentName := range field.Components {
					paths = append(paths, componentName)
				}
			}
		}
	}

	sort.Strings(paths)
	return paths
}

// =====================================
// Utility Functions
// =====================================

func (sa *SchemaAnalyzer) GetLoadedVersions() []string {
	var versions []string
	for version := range sa.HL7Schemas {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions
}

func (sa *SchemaAnalyzer) GetLoadedMessageTypes(version string) []string {
	var messageTypes []string
	if versionSchemas, exists := sa.HL7Schemas[version]; exists {
		for messageType := range versionSchemas {
			messageTypes = append(messageTypes, messageType)
		}
	}
	sort.Strings(messageTypes)
	return messageTypes
}

func (sa *SchemaAnalyzer) GetLoadedFHIRResources() []string {
	var resources []string
	for resourceType := range sa.FHIRSchemas {
		resources = append(resources, resourceType)
	}
	sort.Strings(resources)
	return resources
}

func (sa *SchemaAnalyzer) PrintStats() {
	fmt.Println("=== Schema Analyzer Statistics ===")
	fmt.Printf("HL7 Versions Loaded: %d\n", len(sa.HL7Schemas))

	totalMessageTypes := 0
	for version, schemas := range sa.HL7Schemas {
		fmt.Printf("  %s: %d message types\n", version, len(schemas))
		totalMessageTypes += len(schemas)
	}

	fmt.Printf("Total HL7 Message Types: %d\n", totalMessageTypes)
	fmt.Printf("FHIR Resources Loaded: %d\n", len(sa.FHIRSchemas))
	fmt.Printf("Mapping Rules: %d\n", len(sa.MappingRules))
	fmt.Printf("Transform Functions: %d\n", len(sa.Transformers))
	fmt.Println("=====================================")
}