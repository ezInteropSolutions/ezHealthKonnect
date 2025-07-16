// fhir/schema_loader.go
// Enterprise-grade FHIR schema loader following ezHealthKonnect HL7 patterns
package fhir

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// =====================================
// FHIR SCHEMA TYPES - Enterprise Grade
// =====================================

type FHIRSchema struct {
	ResourceType string                  `json:"resourceType"`
	Version      string                  `json:"version"`
	Name         string                  `json:"name"`
	Title        string                  `json:"title,omitempty"`
	Description  string                  `json:"description"`
	BaseResource string                  `json:"baseResource,omitempty"`
	Profile      string                  `json:"profile,omitempty"`
	Elements     map[string]*FHIRElement `json:"elements"`
	Required     []string                `json:"required"`
	MustSupport  []string                `json:"mustSupport,omitempty"`
	LoadedAt     time.Time               `json:"loadedAt"`
	SourceFile   string                  `json:"sourceFile"`
	FilePath     string                  `json:"-"`
	LoadTime     time.Duration           `json:"-"`
}

type FHIRElement struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	DataType        string   `json:"dataType"`
	Cardinality     string   `json:"cardinality"`
	Required        bool     `json:"required"`
	MustSupport     bool     `json:"mustSupport"`
	IsModifier      bool     `json:"isModifier"`
	IsSummary       bool     `json:"isSummary"`
	Constraints     []string `json:"constraints,omitempty"`
	ValueSet        string   `json:"valueSet,omitempty"`
	BindingStrength string   `json:"bindingStrength,omitempty"`
}

type FHIRSchemaStats struct {
	TotalLoads   int       `json:"totalLoads"`
	CacheHits    int       `json:"cacheHits"`
	CacheMisses  int       `json:"cacheMisses"`
	LoadErrors   int       `json:"loadErrors"`
	LastLoaded   string    `json:"lastLoaded"`
	CacheSize    int       `json:"cacheSize"`
	LastLoadTime time.Time `json:"lastLoadTime"`
}

// =====================================
// ENTERPRISE FHIR SCHEMA LOADER
// =====================================

type FHIRSchemaLoader struct {
	schemaDir string
	cache     map[string]*FHIRSchema
	cacheMux  sync.RWMutex
	stats     FHIRSchemaStats
	statsMux  sync.Mutex
}

// Global FHIR schema loader instance
var fhirSchemaLoader *FHIRSchemaLoader

// =====================================
// INITIALIZATION - Enterprise Pattern
// =====================================

// InitFHIRSchemaLoader initializes the FHIR schema loader
func InitFHIRSchemaLoader(schemaDirectory string) {
	fmt.Printf("🔍 DEBUG: InitFHIRSchemaLoader called with directory: %s\n", schemaDirectory)

	// Ensure FHIR schema directory exists
	fhirSchemaDir := filepath.Join(schemaDirectory, "fhir")
	if _, err := os.Stat(fhirSchemaDir); os.IsNotExist(err) {
		fmt.Printf("⚠️ WARNING: FHIR schema directory does not exist: %s\n", fhirSchemaDir)
		if err := os.MkdirAll(fhirSchemaDir, 0755); err != nil {
			fmt.Printf("❌ ERROR: Failed to create FHIR schema directory: %v\n", err)
			return
		}

		// Create standard FHIR directory structure
		versions := []string{"R4", "R5"}
		for _, version := range versions {
			versionPath := filepath.Join(fhirSchemaDir, version)
			os.MkdirAll(filepath.Join(versionPath, "resources"), 0755)
			os.MkdirAll(filepath.Join(versionPath, "profiles", "us-core"), 0755)
			fmt.Printf("📁 Created FHIR structure: %s\n", versionPath)
		}
	}

	fhirSchemaLoader = &FHIRSchemaLoader{
		schemaDir: fhirSchemaDir,
		cache:     make(map[string]*FHIRSchema),
	}

	fmt.Printf("🚀 FHIR Schema Loader initialized successfully: %s\n", fhirSchemaDir)

	// Scan for existing FHIR schemas
	schemaFiles, err := scanForFHIRSchemas(fhirSchemaDir)
	if err != nil {
		fmt.Printf("⚠️ Warning: Error scanning for FHIR schemas: %v\n", err)
	} else {
		fmt.Printf("📊 Found %d FHIR schema files\n", len(schemaFiles))
		if len(schemaFiles) == 0 {
			createSampleFHIRSchema(fhirSchemaDir)
		} else {
			fmt.Printf("✅ FHIR schema files found:\n")
			for _, file := range schemaFiles {
				rel, _ := filepath.Rel(fhirSchemaDir, file)
				fmt.Printf("  📄 %s\n", rel)
			}
		}
	}
}

// GetFHIRSchemaLoader returns the FHIR schema loader instance
func GetFHIRSchemaLoader() *FHIRSchemaLoader {
	return fhirSchemaLoader
}

// =====================================
// CORE LOADING FUNCTIONS - Enterprise Grade
// =====================================

// LoadFHIRSchema loads a FHIR schema by resource type, profile, and version
func (fsl *FHIRSchemaLoader) LoadFHIRSchema(resourceType, profile, version string) (*FHIRSchema, error) {
	startTime := time.Now()

	fsl.statsMux.Lock()
	fsl.stats.TotalLoads++
	fsl.statsMux.Unlock()

	// Normalize inputs
	if version == "" {
		version = "R4" // Default to R4
	}
	if profile == "" {
		profile = "base" // Default to base profile
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%s_%s_%s", resourceType, profile, version)

	// Check cache first
	fsl.cacheMux.RLock()
	if schema, exists := fsl.cache[cacheKey]; exists {
		fsl.cacheMux.RUnlock()

		fsl.statsMux.Lock()
		fsl.stats.CacheHits++
		fsl.statsMux.Unlock()

		fmt.Printf("✅ CACHE HIT: Using cached FHIR schema for %s\n", cacheKey)
		return schema, nil
	}
	fsl.cacheMux.RUnlock()

	fsl.statsMux.Lock()
	fsl.stats.CacheMisses++
	fsl.statsMux.Unlock()

	// Resolve schema file path
	schemaPath := fsl.resolveSchemaPath(resourceType, profile, version)
	fmt.Printf("🔍 DEBUG: Looking for FHIR schema file: %s\n", schemaPath)

	// Check if file exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		// Try alternative paths
		alternativePath := fsl.tryAlternativePaths(resourceType, profile, version)
		if alternativePath != "" {
			schemaPath = alternativePath
			fmt.Printf("✅ Found alternative FHIR schema: %s\n", schemaPath)
		} else {
			fsl.statsMux.Lock()
			fsl.stats.LoadErrors++
			fsl.statsMux.Unlock()

			fmt.Printf("❌ No FHIR schema file found for %s\n", cacheKey)
			return nil, fmt.Errorf("FHIR schema file not found: %s", schemaPath)
		}
	}

	// Load and parse schema
	fmt.Printf("✅ Loading FHIR schema from: %s\n", schemaPath)
	schema, err := fsl.loadAndParseFHIRSchema(schemaPath)
	if err != nil {
		fsl.statsMux.Lock()
		fsl.stats.LoadErrors++
		fsl.statsMux.Unlock()

		fmt.Printf("❌ Failed to load FHIR schema %s: %v\n", cacheKey, err)
		return nil, fmt.Errorf("failed to load FHIR schema %s: %v", cacheKey, err)
	}

	// Cache the loaded schema
	fsl.cacheMux.Lock()
	fsl.cache[cacheKey] = schema
	fsl.cacheMux.Unlock()

	// Update stats
	schema.LoadTime = time.Since(startTime)

	fsl.statsMux.Lock()
	fsl.stats.LastLoaded = cacheKey
	fsl.stats.CacheSize = len(fsl.cache)
	fsl.stats.LastLoadTime = time.Now()
	fsl.statsMux.Unlock()

	fmt.Printf("✅ Successfully loaded and cached FHIR schema: %s (elements: %d)\n",
		cacheKey, len(schema.Elements))
	return schema, nil
}

// =====================================
// PATH RESOLUTION - Enterprise Logic
// =====================================

// resolveSchemaPath resolves the file path for a FHIR schema
func (fsl *FHIRSchemaLoader) resolveSchemaPath(resourceType, profile, version string) string {
	baseDir := filepath.Join(fsl.schemaDir, version)
	filename := resourceType + ".gz"

	if profile == "base" || profile == "" {
		// Base resource: R4/resources/Patient.gz
		return filepath.Join(baseDir, "resources", filename)
	} else {
		// Profile: R4/profiles/us-core/Patient.gz
		return filepath.Join(baseDir, "profiles", profile, filename)
	}
}

// tryAlternativePaths tries alternative file paths if primary path fails
func (fsl *FHIRSchemaLoader) tryAlternativePaths(resourceType, profile, version string) string {
	baseDir := filepath.Join(fsl.schemaDir, version)

	alternatives := []string{
		// Try lowercase
		filepath.Join(baseDir, "resources", strings.ToLower(resourceType)+".gz"),
		filepath.Join(baseDir, "profiles", profile, strings.ToLower(resourceType)+".gz"),

		// Try profile-prefixed names (fallback)
		filepath.Join(baseDir, "profiles", profile, fmt.Sprintf("%s-%s.gz", profile, strings.ToLower(resourceType))),

		// Try without profile directory
		filepath.Join(baseDir, resourceType+".gz"),
	}

	for _, altPath := range alternatives {
		if _, err := os.Stat(altPath); err == nil {
			return altPath
		}
	}

	return ""
}

// =====================================
// FILE LOADING - Enterprise Grade
// =====================================

// loadAndParseFHIRSchema loads and parses a FHIR schema from .gz file
func (fsl *FHIRSchemaLoader) loadAndParseFHIRSchema(filePath string) (*FHIRSchema, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open FHIR schema file %s: %v", filePath, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("cannot create gzip reader for %s: %v", filePath, err)
	}
	defer gzipReader.Close()

	var schema FHIRSchema
	decoder := json.NewDecoder(gzipReader)
	if err := decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("cannot decode JSON from %s: %v", filePath, err)
	}

	// Set metadata
	schema.LoadedAt = time.Now()
	schema.FilePath = filePath

	fmt.Printf("✅ FHIR schema loaded: %s v%s (elements: %d, required: %d)\n",
		schema.ResourceType, schema.Version, len(schema.Elements), len(schema.Required))

	// Validate schema structure
	if err := fsl.validateFHIRSchema(&schema); err != nil {
		fmt.Printf("⚠️ WARNING: FHIR schema validation issues: %v\n", err)
		// Don't fail on validation warnings, just log them
	}

	return &schema, nil
}

// =====================================
// VALIDATION - Enterprise Quality
// =====================================

// validateFHIRSchema performs basic validation on loaded schema
func (fsl *FHIRSchemaLoader) validateFHIRSchema(schema *FHIRSchema) error {
	var issues []string

	if schema.ResourceType == "" {
		issues = append(issues, "missing resourceType")
	}

	if schema.Version == "" {
		issues = append(issues, "missing version")
	}

	if len(schema.Elements) == 0 {
		issues = append(issues, "no elements defined")
	}

	// Validate required elements exist
	for _, reqPath := range schema.Required {
		if _, exists := schema.Elements[reqPath]; !exists {
			issues = append(issues, fmt.Sprintf("required element '%s' not found in elements", reqPath))
		}
	}

	// Validate mustSupport elements exist
	for _, msPath := range schema.MustSupport {
		if _, exists := schema.Elements[msPath]; !exists {
			issues = append(issues, fmt.Sprintf("mustSupport element '%s' not found in elements", msPath))
		}
	}

	if len(issues) > 0 {
		return fmt.Errorf("validation issues: %v", issues)
	}

	return nil
}

// =====================================
// UTILITY FUNCTIONS - Enterprise Support
// =====================================

// GetStats returns current loader statistics
func (fsl *FHIRSchemaLoader) GetStats() FHIRSchemaStats {
	fsl.cacheMux.RLock()
	fsl.statsMux.Lock()
	stats := fsl.stats
	stats.CacheSize = len(fsl.cache)
	fsl.statsMux.Unlock()
	fsl.cacheMux.RUnlock()
	return stats
}

// ClearCache clears the schema cache
func (fsl *FHIRSchemaLoader) ClearCache() {
	fsl.cacheMux.Lock()
	defer fsl.cacheMux.Unlock()

	oldSize := len(fsl.cache)
	fsl.cache = make(map[string]*FHIRSchema)

	fmt.Printf("🔄 FHIR schema cache cleared (was: %d schemas)\n", oldSize)
}

// ListAvailableSchemas returns list of available FHIR schemas
func (fsl *FHIRSchemaLoader) ListAvailableSchemas() ([]string, error) {
	var schemas []string

	// Scan R4 resources
	r4ResourcesDir := filepath.Join(fsl.schemaDir, "R4", "resources")
	if files, err := filepath.Glob(filepath.Join(r4ResourcesDir, "*.gz")); err == nil {
		for _, file := range files {
			resourceType := strings.TrimSuffix(filepath.Base(file), ".gz")
			schemas = append(schemas, fmt.Sprintf("R4_base_%s", resourceType))
		}
	}

	// Scan R4 US Core profiles
	r4ProfilesDir := filepath.Join(fsl.schemaDir, "R4", "profiles", "us-core")
	if files, err := filepath.Glob(filepath.Join(r4ProfilesDir, "*.gz")); err == nil {
		for _, file := range files {
			resourceType := strings.TrimSuffix(filepath.Base(file), ".gz")
			schemas = append(schemas, fmt.Sprintf("R4_us-core_%s", resourceType))
		}
	}

	// Future: Add R5 when available

	return schemas, nil
}

// =====================================
// HELPER FUNCTIONS
// =====================================

// scanForFHIRSchemas scans directory for FHIR .gz schema files
func scanForFHIRSchemas(baseDir string) ([]string, error) {
	var schemaFiles []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".gz") {
			schemaFiles = append(schemaFiles, path)
		}
		return nil
	})
	return schemaFiles, err
}

// createSampleFHIRSchema creates sample FHIR schemas for testing
func createSampleFHIRSchema(baseDir string) {
	fmt.Printf("🛠️ Creating sample FHIR schemas for testing...\n")

	// Create sample Patient schema
	r4ResourcesDir := filepath.Join(baseDir, "R4", "resources")
	if err := os.MkdirAll(r4ResourcesDir, 0755); err != nil {
		fmt.Printf("❌ Failed to create R4 resources directory: %v\n", err)
		return
	}

	samplePatientSchema := FHIRSchema{
		ResourceType: "Patient",
		Version:      "R4",
		Name:         "Patient",
		Description:  "Demographics and other administrative information about an individual or animal receiving care.",
		BaseResource: "DomainResource",
		Elements: map[string]*FHIRElement{
			"Patient.id": {
				Path:        "Patient.id",
				Name:        "Logical id of this artifact",
				Description: "The logical id of the resource, as used in the URL for the resource.",
				DataType:    "id",
				Cardinality: "0..1",
				Required:    false,
				MustSupport: false,
				IsSummary:   true,
			},
			"Patient.identifier": {
				Path:        "Patient.identifier",
				Name:        "An identifier for this patient",
				Description: "An identifier for this patient.",
				DataType:    "Identifier",
				Cardinality: "0..*",
				Required:    false,
				MustSupport: false,
				IsSummary:   true,
			},
			"Patient.name": {
				Path:        "Patient.name",
				Name:        "A name associated with the patient",
				Description: "A name associated with the individual.",
				DataType:    "HumanName",
				Cardinality: "0..*",
				Required:    false,
				MustSupport: false,
				IsSummary:   true,
			},
		},
		Required:   []string{},
		LoadedAt:   time.Now(),
		SourceFile: "Patient",
	}

	// Write sample schema
	patientPath := filepath.Join(r4ResourcesDir, "Patient.gz")
	if err := writeFHIRSchemaToFile(patientPath, &samplePatientSchema); err != nil {
		fmt.Printf("❌ Failed to write sample Patient schema: %v\n", err)
	} else {
		fmt.Printf("✅ Created sample FHIR schema: %s\n", patientPath)
	}
}

// writeFHIRSchemaToFile writes a FHIR schema to a compressed file
func writeFHIRSchemaToFile(filePath string, schema *FHIRSchema) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	encoder := json.NewEncoder(gzipWriter)
	encoder.SetIndent("", "  ")

	return encoder.Encode(schema)
}
