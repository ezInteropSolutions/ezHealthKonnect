// fhir_converter.go - Converts official FHIR definitions to our schema format
package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// =====================================
// FHIR BUNDLE STRUCTURE (from your download)
// =====================================

type FHIRBundle struct {
	ResourceType string      `json:"resourceType"`
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Entry        []FHIREntry `json:"entry"`
}

type FHIREntry struct {
	FullURL  string          `json:"fullUrl"`
	Resource json.RawMessage `json:"resource"`
}

type FHIRStructureDefinition struct {
	ResourceType   string            `json:"resourceType"`
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	Version        string            `json:"version"`
	Name           string            `json:"name"`
	Title          string            `json:"title"`
	Status         string            `json:"status"`
	Kind           string            `json:"kind"`
	Abstract       bool              `json:"abstract"`
	Type           interface{}       `json:"type"` // Can be string or other types
	BaseDefinition string            `json:"baseDefinition"`
	Description    string            `json:"description"`
	Snapshot       *FHIRSnapshot     `json:"snapshot"`
	Differential   *FHIRDifferential `json:"differential"`
}

type FHIRSnapshot struct {
	Element []FHIRElementDefinition `json:"element"`
}

type FHIRDifferential struct {
	Element []FHIRElementDefinition `json:"element"`
}

type FHIRElementDefinition struct {
	ID          string            `json:"id"`
	Path        string            `json:"path"`
	Short       string            `json:"short"`
	Definition  string            `json:"definition"`
	Min         int               `json:"min"`
	Max         string            `json:"max"`
	Type        []FHIRElementType `json:"type"`
	Binding     *FHIRBinding      `json:"binding"`
	Constraint  []FHIRConstraint  `json:"constraint"`
	MustSupport bool              `json:"mustSupport"`
	IsModifier  bool              `json:"isModifier"`
	IsSummary   bool              `json:"isSummary"`
}

type FHIRElementType struct {
	Code    string      `json:"code"`
	Profile interface{} `json:"profile"` // Can be string or array
}

type FHIRBinding struct {
	Strength    string `json:"strength"`
	ValueSet    string `json:"valueSet"`
	Description string `json:"description"`
}

type FHIRConstraint struct {
	Key        string `json:"key"`
	Severity   string `json:"severity"`
	Human      string `json:"human"`
	Expression string `json:"expression"`
}

// =====================================
// OUR OPTIMIZED SCHEMA FORMAT
// =====================================

type OptimizedFHIRSchema struct {
	ResourceType string                      `json:"resourceType"`
	Version      string                      `json:"version"`
	Name         string                      `json:"name"`
	Title        string                      `json:"title"`
	Description  string                      `json:"description"`
	BaseResource string                      `json:"baseResource,omitempty"`
	Profile      string                      `json:"profile,omitempty"`
	Elements     map[string]OptimizedElement `json:"elements"`
	Required     []string                    `json:"required"`
	MustSupport  []string                    `json:"mustSupport,omitempty"`
	LoadedAt     time.Time                   `json:"loadedAt"`
	SourceFile   string                      `json:"sourceFile"`
}

type OptimizedElement struct {
	Path            string                      `json:"path"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description"`
	DataType        string                      `json:"dataType"`
	Cardinality     string                      `json:"cardinality"`
	Required        bool                        `json:"required"`
	MustSupport     bool                        `json:"mustSupport"`
	IsModifier      bool                        `json:"isModifier"`
	IsSummary       bool                        `json:"isSummary"`
	ValueSet        string                      `json:"valueSet,omitempty"`
	BindingStrength string                      `json:"bindingStrength,omitempty"`
	Constraints     []string                    `json:"constraints,omitempty"`
	Children        map[string]OptimizedElement `json:"children,omitempty"`
}

// =====================================
// CONVERTER IMPLEMENTATION
// =====================================

type FHIRConverter struct {
	inputDir  string
	outputDir string
	version   string
	processed map[string]bool
	stats     ConversionStats
}

type ConversionStats struct {
	TotalProcessed   int
	ResourcesCreated int
	ProfilesCreated  int
	Errors           int
	StartTime        time.Time
}

func NewFHIRConverter(inputDir, outputDir, version string) *FHIRConverter {
	return &FHIRConverter{
		inputDir:  inputDir,
		outputDir: outputDir,
		version:   version,
		processed: make(map[string]bool),
		stats:     ConversionStats{StartTime: time.Now()},
	}
}

func (c *FHIRConverter) ConvertAll() error {
	fmt.Printf("🚀 FHIR Definition Converter v1.0\n")
	fmt.Printf("Input: %s\n", c.inputDir)
	fmt.Printf("Output: %s\n", c.outputDir)
	fmt.Printf("Version: %s\n\n", c.version)

	// Create output directory structure
	if err := c.createOutputStructure(); err != nil {
		return fmt.Errorf("failed to create output structure: %v", err)
	}

	// Step 1: Process base FHIR resources
	fmt.Printf("📦 Step 1: Processing base FHIR resources...\n")
	if err := c.processBaseResources(); err != nil {
		return fmt.Errorf("failed to process base resources: %v", err)
	}

	// Step 2: Process US Core profiles
	fmt.Printf("\n🇺🇸 Step 2: Processing US Core profiles...\n")
	if err := c.processUSCoreProfiles(); err != nil {
		return fmt.Errorf("failed to process US Core profiles: %v", err)
	}

	// Print statistics
	c.printStats()
	return nil
}

func (c *FHIRConverter) createOutputStructure() error {
	dirs := []string{
		filepath.Join(c.outputDir, "fhir", c.version, "resources"),
		filepath.Join(c.outputDir, "fhir", c.version, "datatypes"),
		filepath.Join(c.outputDir, "fhir", c.version, "profiles", "us-core"),
		filepath.Join(c.outputDir, "fhir", c.version, "valuesets"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		fmt.Printf("📁 Created: %s\n", dir)
	}
	return nil
}

func (c *FHIRConverter) processBaseResources() error {
	bundlePath := filepath.Join(c.inputDir, "profiles-resources.json")

	fmt.Printf("📖 Reading bundle: %s\n", bundlePath)
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("cannot read bundle file: %v", err)
	}

	var bundle FHIRBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("cannot parse bundle: %v", err)
	}

	fmt.Printf("📊 Bundle contains %d entries\n", len(bundle.Entry))

	resourceCount := 0
	for i, entry := range bundle.Entry {
		// First, check what type of resource this is
		var resourceCheck struct {
			ResourceType string `json:"resourceType"`
		}

		if err := json.Unmarshal(entry.Resource, &resourceCheck); err != nil {
			fmt.Printf("⚠️ [%d] Skipping entry - cannot determine resource type: %v\n", i+1, err)
			continue
		}

		// Only process StructureDefinition resources
		if resourceCheck.ResourceType != "StructureDefinition" {
			continue
		}

		// Now unmarshal as StructureDefinition
		var resource FHIRStructureDefinition
		if err := json.Unmarshal(entry.Resource, &resource); err != nil {
			fmt.Printf("⚠️ [%d] Failed to parse StructureDefinition: %v\n", i+1, err)
			c.stats.Errors++
			continue
		}

		// Only process resource definitions (not data types or extensions)
		if resource.Kind != "resource" || resource.Abstract {
			continue
		}

		// Get the type as string
		resourceType := ""
		if typeVal, ok := resource.Type.(string); ok {
			resourceType = typeVal
		} else {
			fmt.Printf("⚠️ [%d] Skipping - type is not string: %T\n", i+1, resource.Type)
			continue
		}

		// Skip base Resource and DomainResource
		if resourceType == "Resource" || resourceType == "DomainResource" {
			continue
		}

		fmt.Printf("🔄 [%d/%d] Processing %s\n", i+1, len(bundle.Entry), resourceType)

		schema, err := c.convertStructureDefinition(resource, "base")
		if err != nil {
			fmt.Printf("⚠️ Failed to convert %s: %v\n", resourceType, err)
			c.stats.Errors++
			continue
		}

		if err := c.saveSchema(schema, "resources"); err != nil {
			fmt.Printf("⚠️ Failed to save %s: %v\n", resourceType, err)
			c.stats.Errors++
			continue
		}

		resourceCount++
		c.stats.ResourcesCreated++
		fmt.Printf("✅ Saved: %s.gz\n", resourceType)
	}

	fmt.Printf("📊 Processed %d base resources\n", resourceCount)
	return nil
}

func (c *FHIRConverter) processUSCoreProfiles() error {
	// Process individual US Core profile files
	usCoreFiles, err := filepath.Glob(filepath.Join(c.inputDir, "StructureDefinition-us-core-*.json"))
	if err != nil {
		return fmt.Errorf("failed to list US Core files: %v", err)
	}

	fmt.Printf("📊 Found %d US Core profiles\n", len(usCoreFiles))

	for i, filePath := range usCoreFiles {
		filename := filepath.Base(filePath)
		fmt.Printf("🔄 [%d/%d] Processing %s\n", i+1, len(usCoreFiles), filename)

		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("⚠️ Failed to read %s: %v\n", filename, err)
			c.stats.Errors++
			continue
		}

		var structDef FHIRStructureDefinition
		if err := json.Unmarshal(data, &structDef); err != nil {
			fmt.Printf("⚠️ Failed to parse %s: %v\n", filename, err)
			c.stats.Errors++
			continue
		}

		// Only process resource profiles
		if structDef.Kind != "resource" || structDef.Abstract {
			continue
		}

		// Get the type as string
		resourceType := ""
		if typeVal, ok := structDef.Type.(string); ok {
			resourceType = typeVal
		} else {
			fmt.Printf("⚠️ Skipping %s - type is not string: %T\n", filename, structDef.Type)
			continue
		}

		schema, err := c.convertStructureDefinition(structDef, "us-core")
		if err != nil {
			fmt.Printf("⚠️ Failed to convert %s: %v\n", resourceType, err)
			c.stats.Errors++
			continue
		}

		// Save to us-core subdirectory
		if err := c.saveSchema(schema, filepath.Join("profiles", "us-core")); err != nil {
			fmt.Printf("⚠️ Failed to save %s: %v\n", resourceType, err)
			c.stats.Errors++
			continue
		}

		c.stats.ProfilesCreated++
		fmt.Printf("✅ Saved: %s.gz (US Core)\n", resourceType)
	}

	return nil
}

func (c *FHIRConverter) convertStructureDefinition(structDef FHIRStructureDefinition, profile string) (*OptimizedFHIRSchema, error) {
	// Get the type as string
	resourceType := ""
	if typeVal, ok := structDef.Type.(string); ok {
		resourceType = typeVal
	} else {
		return nil, fmt.Errorf("type field is not a string: %T", structDef.Type)
	}

	schema := &OptimizedFHIRSchema{
		ResourceType: resourceType,
		Version:      c.version,
		Name:         structDef.Name,
		Title:        structDef.Title,
		Description:  structDef.Description,
		Elements:     make(map[string]OptimizedElement),
		Required:     make([]string, 0),
		MustSupport:  make([]string, 0),
		LoadedAt:     time.Now(),
		SourceFile:   structDef.ID,
	}

	if profile != "base" {
		schema.Profile = profile
	}

	// Extract base resource from URL
	if structDef.BaseDefinition != "" {
		parts := strings.Split(structDef.BaseDefinition, "/")
		if len(parts) > 0 {
			schema.BaseResource = parts[len(parts)-1]
		}
	}

	// Process elements from snapshot
	if structDef.Snapshot != nil {
		for _, element := range structDef.Snapshot.Element {
			// Skip the root element
			if element.Path == resourceType {
				continue
			}

			optimizedElement := c.convertElement(element)
			schema.Elements[element.Path] = optimizedElement

			// Track required and must-support elements
			if element.Min > 0 {
				schema.Required = append(schema.Required, element.Path)
			}
			if element.MustSupport {
				schema.MustSupport = append(schema.MustSupport, element.Path)
			}
		}
	}

	c.stats.TotalProcessed++
	return schema, nil
}

func (c *FHIRConverter) convertElement(element FHIRElementDefinition) OptimizedElement {
	// Build cardinality string
	cardinality := fmt.Sprintf("%d..%s", element.Min, element.Max)
	if element.Max == "*" {
		cardinality = fmt.Sprintf("%d..*", element.Min)
	}

	// Determine primary data type
	dataType := "string" // default
	if len(element.Type) > 0 {
		dataType = element.Type[0].Code
	}

	optimized := OptimizedElement{
		Path:        element.Path,
		Name:        element.Short,
		Description: element.Definition,
		DataType:    dataType,
		Cardinality: cardinality,
		Required:    element.Min > 0,
		MustSupport: element.MustSupport,
		IsModifier:  element.IsModifier,
		IsSummary:   element.IsSummary,
		Constraints: make([]string, 0),
		Children:    make(map[string]OptimizedElement),
	}

	// Add value set binding if present
	if element.Binding != nil {
		if element.Binding.ValueSet != "" {
			optimized.ValueSet = element.Binding.ValueSet
		}
		optimized.BindingStrength = element.Binding.Strength
	}

	// Add constraints
	for _, constraint := range element.Constraint {
		optimized.Constraints = append(optimized.Constraints, constraint.Human)
	}

	return optimized
}

func (c *FHIRConverter) saveSchema(schema *OptimizedFHIRSchema, category string) error {
	filename := fmt.Sprintf("%s.gz", schema.ResourceType)
	filePath := filepath.Join(c.outputDir, "fhir", c.version, category, filename)

	fmt.Printf("   💾 Saving to: %s\n", filePath)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filePath, err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Printf("   ⚠️ Warning: Failed to close gzip writer: %v\n", err)
		}
	}()

	encoder := json.NewEncoder(gzipWriter)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(schema); err != nil {
		return fmt.Errorf("failed to encode schema: %v", err)
	}

	// Explicitly flush the gzip writer
	if err := gzipWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush gzip writer: %v", err)
	}

	// Check file size after writing
	if stat, err := file.Stat(); err == nil {
		fmt.Printf("   📊 File size: %d bytes\n", stat.Size())
		if stat.Size() == 0 {
			return fmt.Errorf("file was created but is empty")
		}
	}

	return nil
}

func (c *FHIRConverter) printStats() {
	duration := time.Since(c.stats.StartTime)

	fmt.Printf("\n🎉 CONVERSION COMPLETED!\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("⏱️  Duration: %v\n", duration)
	fmt.Printf("📊 Total Processed: %d\n", c.stats.TotalProcessed)
	fmt.Printf("🏥 Base Resources: %d\n", c.stats.ResourcesCreated)
	fmt.Printf("🇺🇸 US Core Profiles: %d\n", c.stats.ProfilesCreated)
	fmt.Printf("❌ Errors: %d\n", c.stats.Errors)
	fmt.Printf("==========================================\n")
}

// =====================================
// MAIN FUNCTION
// =====================================

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("FHIR Definition Converter v1.0\n")
		fmt.Printf("Usage: %s <input-dir> <output-dir> <version>\n\n", os.Args[0])
		fmt.Printf("Example: %s ./fhir-definitions ./schemas R4\n\n", os.Args[0])
		fmt.Printf("Input directory should contain:\n")
		fmt.Printf("  - profiles-resources.json (base FHIR)\n")
		fmt.Printf("  - StructureDefinition-us-core-*.json (US Core profiles)\n")
		os.Exit(1)
	}

	inputDir := os.Args[1]
	outputDir := os.Args[2]
	version := os.Args[3]

	converter := NewFHIRConverter(inputDir, outputDir, version)
	if err := converter.ConvertAll(); err != nil {
		fmt.Printf("❌ Conversion failed: %v\n", err)
		os.Exit(1)
	}
}
