// fhir/schema_loader.go
// Enterprise-grade FHIR schema loader with ERROR-ONLY logging
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
// INITIALIZATION - SILENT
// =====================================

// InitFHIRSchemaLoader initializes the FHIR schema loader
func InitFHIRSchemaLoader(schemaDirectory string) {
	fhirSchemaDir := schemaDirectory

	// REMOVED: Auto-directory creation logic
	// The schema directory MUST exist as part of FHIR package setup
	if _, err := os.Stat(fhirSchemaDir); os.IsNotExist(err) {
		fmt.Printf("❌ FATAL: FHIR schema directory does not exist: %s\n", fhirSchemaDir)
		fmt.Printf("💡 HINT: Download and extract FHIR packages to this directory first\n")
		fmt.Printf("💡 Expected structure: %s/R4/resources/*.gz\n", fhirSchemaDir)
		fmt.Printf("💡 Expected structure: %s/R4/profiles/us-core/*.gz\n", fhirSchemaDir)
		return // Fail initialization
	}

	// Create the loader instance
	fhirSchemaLoader = &FHIRSchemaLoader{
		schemaDir: fhirSchemaDir,
		cache:     make(map[string]*FHIRSchema),
	}

	// FIXED: Scan for existing FHIR schemas using VERSION-AWARE scanning
	schemaFiles, err := scanVersionAwareFHIRSchemas(fhirSchemaDir)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to scan FHIR schemas: %v\n", err)
		return
	}

	if len(schemaFiles) == 0 {
		fmt.Printf("❌ FATAL: No FHIR schema files found in expected locations\n")
		fmt.Printf("💡 Expected files in: %s/R4/resources/*.gz\n", fhirSchemaDir)
		fmt.Printf("💡 Expected files in: %s/R4/profiles/*/*.gz\n", fhirSchemaDir)

		// List what directories actually exist
		if entries, err := os.ReadDir(fhirSchemaDir); err == nil {
			fmt.Printf("📁 Found directories: ")
			for _, entry := range entries {
				if entry.IsDir() {
					fmt.Printf("%s ", entry.Name())
				}
			}
			fmt.Printf("\n")
		}

		return // Fail initialization instead of creating samples
	}

	fmt.Printf("✅ FHIR schema loader initialized with %d schema files\n", len(schemaFiles))
}

func scanVersionAwareFHIRSchemas(baseDir string) ([]string, error) {
	var schemaFiles []string

	// Define expected FHIR versions and their paths
	versionPaths := []struct {
		version string
		paths   []string
	}{
		{
			version: "R4",
			paths: []string{
				filepath.Join(baseDir, "R4", "resources"),           // /app/schemas/fhir/R4/resources/
				filepath.Join(baseDir, "R4", "profiles", "us-core"), // /app/schemas/fhir/R4/profiles/us-core/
				filepath.Join(baseDir, "R4", "profiles", "base"),    // /app/schemas/fhir/R4/profiles/base/
			},
		},
		{
			version: "R5",
			paths: []string{
				filepath.Join(baseDir, "R5", "resources"),           // /app/schemas/fhir/R5/resources/
				filepath.Join(baseDir, "R5", "profiles", "us-core"), // /app/schemas/fhir/R5/profiles/us-core/
			},
		},
	}

	// Scan each version-specific path
	for _, versionInfo := range versionPaths {
		for _, scanPath := range versionInfo.paths {
			// Check if path exists
			if _, err := os.Stat(scanPath); os.IsNotExist(err) {
				continue // Skip non-existent paths (not an error)
			}

			// Scan for .gz files in this specific path
			pattern := filepath.Join(scanPath, "*.gz")
			files, err := filepath.Glob(pattern)
			if err != nil {
				fmt.Printf("⚠️ WARNING: Error scanning %s: %v\n", pattern, err)
				continue
			}

			// Add found files
			schemaFiles = append(schemaFiles, files...)

			if len(files) > 0 {
				fmt.Printf("📁 Found %d schema files in %s\n", len(files), scanPath)
			}
		}
	}

	return schemaFiles, nil
}

// GetFHIRSchemaLoader returns the FHIR schema loader instance
func GetFHIRSchemaLoader() *FHIRSchemaLoader {
	return fhirSchemaLoader
}

// =====================================
// CORE LOADING FUNCTIONS - SILENT
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

		// Silent cache hit - no logging
		return schema, nil
	}
	fsl.cacheMux.RUnlock()

	fsl.statsMux.Lock()
	fsl.stats.CacheMisses++
	fsl.statsMux.Unlock()

	// Resolve schema file path
	schemaPath := fsl.resolveSchemaPath(resourceType, profile, version)

	// Check if file exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		// Try alternative paths - SILENT
		alternativePath := fsl.tryAlternativePaths(resourceType, profile, version)
		if alternativePath != "" {
			schemaPath = alternativePath
		} else {
			fsl.statsMux.Lock()
			fsl.stats.LoadErrors++
			fsl.statsMux.Unlock()

			return nil, fmt.Errorf("FHIR schema file not found: %s", schemaPath)
		}
	}

	// Load and parse schema - SILENT
	schema, err := fsl.loadAndParseFHIRSchema(schemaPath)
	if err != nil {
		fsl.statsMux.Lock()
		fsl.stats.LoadErrors++
		fsl.statsMux.Unlock()

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

	// Silent success - no logging
	return schema, nil
}

// =====================================
// PATH RESOLUTION - SILENT
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
// FILE LOADING - SILENT
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

	// Silent success - no logging

	// Validate schema structure - only log errors
	if err := fsl.validateFHIRSchema(&schema); err != nil {
		fmt.Printf("❌ ERROR: FHIR schema validation failed: %v\n", err)
		// Don't fail on validation warnings, just log them
	}

	return &schema, nil
}

// =====================================
// VALIDATION - ERROR-ONLY
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
// UTILITY FUNCTIONS - SILENT
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

	fsl.cache = make(map[string]*FHIRSchema)
	// Silent cache clear - no logging
}

// ListAvailableSchemas returns list of available FHIR schemas
func (fsl *FHIRSchemaLoader) ListAvailableSchemas() ([]string, error) {
	var schemas []string

	// Scan R4 resources - CORRECT
	r4ResourcesDir := filepath.Join(fsl.schemaDir, "R4", "resources")
	if files, err := filepath.Glob(filepath.Join(r4ResourcesDir, "*.gz")); err == nil {
		for _, file := range files {
			resourceType := strings.TrimSuffix(filepath.Base(file), ".gz")
			schemas = append(schemas, fmt.Sprintf("R4_base_%s", resourceType))
		}
	}

	// Scan R4 US Core profiles - CORRECT
	r4ProfilesDir := filepath.Join(fsl.schemaDir, "R4", "profiles", "us-core")
	if files, err := filepath.Glob(filepath.Join(r4ProfilesDir, "*.gz")); err == nil {
		for _, file := range files {
			resourceType := strings.TrimSuffix(filepath.Base(file), ".gz")
			schemas = append(schemas, fmt.Sprintf("R4_us-core_%s", resourceType))
		}
	}

	// Add R5 support
	r5ResourcesDir := filepath.Join(fsl.schemaDir, "R5", "resources")
	if files, err := filepath.Glob(filepath.Join(r5ResourcesDir, "*.gz")); err == nil {
		for _, file := range files {
			resourceType := strings.TrimSuffix(filepath.Base(file), ".gz")
			schemas = append(schemas, fmt.Sprintf("R5_base_%s", resourceType))
		}
	}

	return schemas, nil
}

// =====================================
// HELPER FUNCTIONS - SILENT
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
