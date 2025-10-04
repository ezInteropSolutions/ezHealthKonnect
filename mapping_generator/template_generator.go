// mapping_generator/template_generator.go
// Template generation engine for OOB HL7-FHIR mappings
// Generates high-confidence mapping templates for standard message types

package mapping_generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// =====================================
// Template Generation Engine
// =====================================

type TemplateGenerator struct {
	schemaAnalyzer   *SchemaAnalyzer
	mappingEngine    *MappingRuleEngine
	outputPath       string
	generatedAt      time.Time
	supportedVersions []string
}

type GenerationConfig struct {
	MessageTypes      []string  `json:"messageTypes"`      // ["ADT^A01", "ORU^R01", etc.]
	HL7Versions       []string  `json:"hl7Versions"`       // ["2.3", "2.5.1"]
	OutputPath        string    `json:"outputPath"`        // "./templates"
	MinConfidence     float64   `json:"minConfidence"`     // 0.85
	IncludeMetadata   bool      `json:"includeMetadata"`   // true
	CompressOutput    bool      `json:"compressOutput"`    // true
	ValidateTemplates bool      `json:"validateTemplates"` // true
}

type TemplateGenerationResults struct {
	TotalTemplates     int                         `json:"totalTemplates"`
	SuccessfulTemplates int                        `json:"successfulTemplates"`
	FailedTemplates    int                         `json:"failedTemplates"`
	GenerationTime     time.Duration               `json:"generationTime"`
	Templates          map[string]*MappingTemplate `json:"templates"`
	Errors             []string                    `json:"errors"`
	Stats              GenerationStats             `json:"stats"`
}

type GenerationStats struct {
	TotalMappings        int     `json:"totalMappings"`
	RequiredMappings     int     `json:"requiredMappings"`
	HighConfidenceMappings int   `json:"highConfidenceMappings"`
	AverageConfidence    float64 `json:"averageConfidence"`
	ResourceDistribution map[string]int `json:"resourceDistribution"`
}

// =====================================
// Core Template Generation
// =====================================

func NewTemplateGenerator(schemasPath, outputPath string) *TemplateGenerator {
	return &TemplateGenerator{
		schemaAnalyzer:    NewSchemaAnalyzer(schemasPath),
		mappingEngine:     NewMappingRuleEngine(),
		outputPath:        outputPath,
		generatedAt:       time.Now(),
		supportedVersions: []string{"2.3", "2.5.1"},
	}
}

func (tg *TemplateGenerator) GenerateOOBTemplates(config GenerationConfig) (*TemplateGenerationResults, error) {
	startTime := time.Now()

	results := &TemplateGenerationResults{
		Templates: make(map[string]*MappingTemplate),
		Errors:    make([]string, 0),
		Stats: GenerationStats{
			ResourceDistribution: make(map[string]int),
		},
	}

	// Load schemas
	fmt.Println("🔄 Loading HL7 schemas...")
	if err := tg.schemaAnalyzer.LoadHL7Schemas(config.HL7Versions); err != nil {
		return nil, fmt.Errorf("failed to load HL7 schemas: %w", err)
	}

	fmt.Println("🔄 Loading FHIR schemas...")
	if err := tg.schemaAnalyzer.LoadFHIRSchemas(); err != nil {
		return nil, fmt.Errorf("failed to load FHIR schemas: %w", err)
	}

	tg.schemaAnalyzer.PrintStats()

	// Generate templates for each message type and version
	for _, version := range config.HL7Versions {
		for _, messageType := range config.MessageTypes {
			templateKey := fmt.Sprintf("%s_%s", messageType, version)

			fmt.Printf("🔄 Generating template for %s (v%s)...\n", messageType, version)

			template, err := tg.GenerateTemplate(messageType, version, config)
			if err != nil {
				results.Errors = append(results.Errors, fmt.Sprintf("%s: %v", templateKey, err))
				results.FailedTemplates++
				continue
			}

			results.Templates[templateKey] = template
			results.SuccessfulTemplates++
			tg.updateStats(&results.Stats, template)
		}
	}

	results.TotalTemplates = len(config.MessageTypes) * len(config.HL7Versions)
	results.GenerationTime = time.Since(startTime)
	tg.calculateAverageConfidence(&results.Stats)

	// Save templates if output path is provided
	if config.OutputPath != "" {
		if err := tg.SaveTemplates(results.Templates, config); err != nil {
			results.Errors = append(results.Errors, fmt.Sprintf("Failed to save templates: %v", err))
		}
	}

	return results, nil
}

func (tg *TemplateGenerator) GenerateTemplate(messageType, version string, config GenerationConfig) (*MappingTemplate, error) {
	// Get HL7 schema for this message type and version
	hl7Schema, err := tg.schemaAnalyzer.AnalyzeMessageType(version, messageType)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze message type %s v%s: %w", messageType, version, err)
	}

	// Determine target FHIR resources
	targetResources := tg.schemaAnalyzer.GetTargetFHIRResources(messageType)

	// Create template structure
	template := &MappingTemplate{
		MessageType: messageType,
		Version:     version,
		GeneratedAt: tg.generatedAt,
		Resources:   make(map[string]*ResourceMapping),
		Transforms:  make(map[string]*TransformFunction),
		Coverage: CoverageStats{
			TotalHL7Fields: len(tg.schemaAnalyzer.GetFieldPaths(hl7Schema)),
		},
	}

	// Generate mappings for each target resource
	for priority, resourceType := range targetResources {
		resourceMapping := tg.generateResourceMapping(messageType, resourceType, config, priority+1)
		if len(resourceMapping.Mappings) > 0 {
			template.Resources[resourceType] = resourceMapping
		}
	}

	// Collect all transform functions used
	tg.collectTransformFunctions(template)

	// Calculate coverage statistics
	tg.calculateCoverage(template)

	// Add resource references
	tg.addResourceReferences(template, messageType)

	return template, nil
}

func (tg *TemplateGenerator) generateResourceMapping(messageType, resourceType string, config GenerationConfig, priority int) *ResourceMapping {
	// Get field mappings for this resource
	fieldMappings := tg.mappingEngine.GetFieldMappingsForResource(messageType, resourceType)

	// Filter by confidence threshold
	var filteredMappings []FieldMapping
	for _, mapping := range fieldMappings {
		if mapping.Confidence >= config.MinConfidence {
			filteredMappings = append(filteredMappings, mapping)
		}
	}

	// Sort by priority (required first, then by confidence)
	sort.Slice(filteredMappings, func(i, j int) bool {
		if filteredMappings[i].Required != filteredMappings[j].Required {
			return filteredMappings[i].Required
		}
		return filteredMappings[i].Confidence > filteredMappings[j].Confidence
	})

	return &ResourceMapping{
		ResourceType: resourceType,
		Priority:     priority,
		Required:     tg.isResourceRequired(resourceType, messageType),
		References:   []ResourceRef{}, // Will be populated later
		Mappings:     filteredMappings,
	}
}

func (tg *TemplateGenerator) isResourceRequired(resourceType, messageType string) bool {
	// Define required resources per message type
	requiredResources := map[string][]string{
		"ADT": {"Patient", "MessageHeader"},
		"ORU": {"Patient", "Observation", "MessageHeader"},
		"ORM": {"Patient", "ServiceRequest", "MessageHeader"},
		"MDM": {"Patient", "DocumentReference", "MessageHeader"},
		"SIU": {"Patient", "Appointment", "MessageHeader"},
		"DFT": {"Patient", "Account", "MessageHeader"},
		"RDE": {"Patient", "MedicationRequest", "MessageHeader"},
		"RXE": {"MedicationRequest"},
	}

	messageFamily := strings.Split(messageType, "^")[0]
	if required, exists := requiredResources[messageFamily]; exists {
		for _, req := range required {
			if req == resourceType {
				return true
			}
		}
	}

	return false
}

func (tg *TemplateGenerator) collectTransformFunctions(template *MappingTemplate) {
	functionMap := make(map[string]*TransformFunction)

	for _, resource := range template.Resources {
		for _, mapping := range resource.Mappings {
			if transform, exists := tg.mappingEngine.GetTransformFunction(mapping.Transform); exists {
				transformCopy := transform
				functionMap[mapping.Transform] = &transformCopy
			}
		}
	}

	template.Transforms = functionMap
}

func (tg *TemplateGenerator) calculateCoverage(template *MappingTemplate) {
	totalMappings := 0
	requiredMappings := 0

	for _, resource := range template.Resources {
		totalMappings += len(resource.Mappings)
		for _, mapping := range resource.Mappings {
			if mapping.Required {
				requiredMappings++
			}
		}
	}

	template.Coverage.MappedFields = totalMappings
	template.Coverage.RequiredCoverage = float64(requiredMappings)

	if template.Coverage.TotalHL7Fields > 0 {
		template.Coverage.CoveragePercent = float64(totalMappings) / float64(template.Coverage.TotalHL7Fields) * 100
	}
}

func (tg *TemplateGenerator) addResourceReferences(template *MappingTemplate, messageType string) {
	// Define standard resource references
	references := map[string][]ResourceRef{
		"Encounter": {
			{Path: "subject", Target: "Patient", Field: "id"},
		},
		"Observation": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "encounter", Target: "Encounter", Field: "id"},
		},
		"DiagnosticReport": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "encounter", Target: "Encounter", Field: "id"},
		},
		"ServiceRequest": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "encounter", Target: "Encounter", Field: "id"},
		},
		"DocumentReference": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "context.encounter", Target: "Encounter", Field: "id"},
		},
		"Appointment": {
			{Path: "participant[0].actor", Target: "Patient", Field: "id"},
		},
		"MedicationRequest": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "encounter", Target: "Encounter", Field: "id"},
		},
		"Account": {
			{Path: "subject[0]", Target: "Patient", Field: "id"},
		},
		"ChargeItem": {
			{Path: "subject", Target: "Patient", Field: "id"},
			{Path: "context", Target: "Encounter", Field: "id"},
		},
	}

	for resourceType, resource := range template.Resources {
		if refs, exists := references[resourceType]; exists {
			// Only add references to resources that exist in this template
			var validRefs []ResourceRef
			for _, ref := range refs {
				if _, targetExists := template.Resources[ref.Target]; targetExists {
					validRefs = append(validRefs, ref)
				}
			}
			resource.References = validRefs
		}
	}
}

// =====================================
// Template Persistence
// =====================================

func (tg *TemplateGenerator) SaveTemplates(templates map[string]*MappingTemplate, config GenerationConfig) error {
	if err := os.MkdirAll(config.OutputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for templateKey, template := range templates {
		fileName := fmt.Sprintf("%s.json", templateKey)
		filePath := filepath.Join(config.OutputPath, fileName)

		data, err := json.MarshalIndent(template, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal template %s: %w", templateKey, err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write template file %s: %w", filePath, err)
		}

		fmt.Printf("✅ Saved template: %s\n", filePath)
	}

	// Save generation summary
	summaryPath := filepath.Join(config.OutputPath, "generation_summary.json")
	summary := map[string]interface{}{
		"generatedAt":      tg.generatedAt.Format(time.RFC3339),
		"totalTemplates":   len(templates),
		"messageTypes":     config.MessageTypes,
		"hl7Versions":      config.HL7Versions,
		"minConfidence":    config.MinConfidence,
		"templateKeys":     tg.getTemplateKeys(templates),
	}

	summaryData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal generation summary: %w", err)
	}

	if err := os.WriteFile(summaryPath, summaryData, 0644); err != nil {
		return fmt.Errorf("failed to write generation summary: %w", err)
	}

	fmt.Printf("✅ Saved generation summary: %s\n", summaryPath)
	return nil
}

func (tg *TemplateGenerator) getTemplateKeys(templates map[string]*MappingTemplate) []string {
	var keys []string
	for key := range templates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// =====================================
// Statistics and Reporting
// =====================================

func (tg *TemplateGenerator) updateStats(stats *GenerationStats, template *MappingTemplate) {
	for _, resource := range template.Resources {
		stats.ResourceDistribution[resource.ResourceType]++

		for _, mapping := range resource.Mappings {
			stats.TotalMappings++
			if mapping.Required {
				stats.RequiredMappings++
			}
			if mapping.Confidence >= 0.95 {
				stats.HighConfidenceMappings++
			}
		}
	}
}

func (tg *TemplateGenerator) calculateAverageConfidence(stats *GenerationStats) {
	if stats.TotalMappings == 0 {
		stats.AverageConfidence = 0.0
		return
	}

	// This is a simplified calculation - in practice, you'd track individual confidences
	stats.AverageConfidence = float64(stats.HighConfidenceMappings) / float64(stats.TotalMappings)
}

func (tg *TemplateGenerator) PrintGenerationReport(results *TemplateGenerationResults) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎯 OOB TEMPLATE GENERATION REPORT")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("📊 Generation Statistics:\n")
	fmt.Printf("   Total Templates: %d\n", results.TotalTemplates)
	fmt.Printf("   Successful: %d\n", results.SuccessfulTemplates)
	fmt.Printf("   Failed: %d\n", results.FailedTemplates)
	fmt.Printf("   Generation Time: %v\n", results.GenerationTime)

	fmt.Printf("\n📈 Mapping Statistics:\n")
	fmt.Printf("   Total Mappings: %d\n", results.Stats.TotalMappings)
	fmt.Printf("   Required Mappings: %d\n", results.Stats.RequiredMappings)
	fmt.Printf("   High Confidence (95%+): %d\n", results.Stats.HighConfidenceMappings)
	fmt.Printf("   Average Confidence: %.1f%%\n", results.Stats.AverageConfidence*100)

	fmt.Printf("\n🏥 Resource Distribution:\n")
	for resource, count := range results.Stats.ResourceDistribution {
		fmt.Printf("   %s: %d templates\n", resource, count)
	}

	if len(results.Errors) > 0 {
		fmt.Printf("\n❌ Errors (%d):\n", len(results.Errors))
		for _, err := range results.Errors {
			fmt.Printf("   • %s\n", err)
		}
	}

	fmt.Printf("\n✅ Generated Templates:\n")
	for key := range results.Templates {
		fmt.Printf("   • %s\n", key)
	}

	fmt.Println(strings.Repeat("=", 60))
}

// =====================================
// Utility Functions
// =====================================

func (tg *TemplateGenerator) GetDefaultConfig() GenerationConfig {
	return GenerationConfig{
		MessageTypes: []string{
			"ADT^A01", "ADT^A04", "ADT^A08", "ADT^A03", // ADT family
			"ORU^R01",                                   // Lab results
			"ORM^O01",                                   // Orders
			"MDM^T01", "MDM^T02",                       // Documents
			"SIU^S12",                                   // Scheduling
			"DFT^P03",                                   // Financial
			"RDE^O11",                                   // Pharmacy orders
		},
		HL7Versions:       []string{"2.3", "2.5.1"},
		OutputPath:        "./templates",
		MinConfidence:     0.85,
		IncludeMetadata:   true,
		CompressOutput:    false,
		ValidateTemplates: true,
	}
}

func (tg *TemplateGenerator) ValidateTemplate(template *MappingTemplate) []string {
	var errors []string

	// Validate basic structure
	if template.MessageType == "" {
		errors = append(errors, "missing message type")
	}

	if template.Version == "" {
		errors = append(errors, "missing version")
	}

	if len(template.Resources) == 0 {
		errors = append(errors, "no resources defined")
	}

	// Validate each resource
	for resourceType, resource := range template.Resources {
		if resource.ResourceType != resourceType {
			errors = append(errors, fmt.Sprintf("resource type mismatch: %s vs %s", resourceType, resource.ResourceType))
		}

		if len(resource.Mappings) == 0 {
			errors = append(errors, fmt.Sprintf("no mappings for resource %s", resourceType))
		}

		// Validate mappings
		for i, mapping := range resource.Mappings {
			if mapping.HL7Path == "" {
				errors = append(errors, fmt.Sprintf("missing HL7 path in %s mapping %d", resourceType, i))
			}

			if mapping.FHIRPath == "" {
				errors = append(errors, fmt.Sprintf("missing FHIR path in %s mapping %d", resourceType, i))
			}

			if mapping.Transform == "" {
				errors = append(errors, fmt.Sprintf("missing transform in %s mapping %d", resourceType, i))
			}
		}
	}

	return errors
}