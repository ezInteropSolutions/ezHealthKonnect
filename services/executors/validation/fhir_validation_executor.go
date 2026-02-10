package validation

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"
)

// FHIRValidationExecutor validates FHIR bundles and individual resources
type FHIRValidationExecutor struct {
	*executors.BaseExecutor
}

// NewFHIRValidationExecutor creates a new FHIR validation executor
func NewFHIRValidationExecutor() *FHIRValidationExecutor {
	return &FHIRValidationExecutor{
		BaseExecutor: executors.NewBaseExecutor("fhir_validation", models.ExecutorMetadata{
			Name:        "FHIR Validation",
			Description: "Validates FHIR bundles and resources against R4 schema",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Validation",
		}),
	}
}

type fhirValidationConfig struct {
	ValidationLevel        string   `json:"validation_level"`         // basic, standard, strict
	RequiredResources      []string `json:"required_resources"`       // e.g., ["Patient", "Encounter"]
	ValidateReferences     bool     `json:"validate_references"`      // check internal bundle references
	ValidateRequiredFields bool     `json:"validate_required_fields"` // check FHIR required fields per resource
	FailOnError            bool     `json:"fail_on_error"`            // stop pipeline if validation errors found
}

type resourceValidationResult struct {
	ResourceType string   `json:"resource_type"`
	ResourceID   string   `json:"resource_id"`
	Valid        bool     `json:"valid"`
	Errors       []string `json:"errors,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

func (fve *FHIRValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	// Parse config
	config := fhirValidationConfig{
		ValidationLevel:        "standard",
		ValidateReferences:     true,
		ValidateRequiredFields: true,
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	log.Printf("  🔍 FHIR Validation starting (level: %s)", config.ValidationLevel)

	// Find FHIR data - either a Bundle or a standalone resource
	fhirData, isBundle := fve.findFHIRData(inputData)
	if fhirData == nil {
		return fve.buildErrorOutput(inputData, startTime, "No FHIR data found in pipeline data (looked for fhirBundle, fhirResource, and resourceType)")
	}

	if isBundle {
		return fve.executeBundleValidation(fhirData, config, inputData, startTime)
	}
	return fve.executeSingleResourceValidation(fhirData, config, inputData, startTime)
}

// executeBundleValidation validates a FHIR Bundle and all its entry resources
func (fve *FHIRValidationExecutor) executeBundleValidation(
	bundle map[string]interface{},
	config fhirValidationConfig,
	inputData map[string]interface{},
	startTime time.Time,
) (map[string]interface{}, error) {
	var allErrors []string
	var allWarnings []string
	var resourceResults []resourceValidationResult

	// 1. Validate bundle structure
	bundleErrors, bundleWarnings := fve.validateBundleStructure(bundle)
	allErrors = append(allErrors, bundleErrors...)
	allWarnings = append(allWarnings, bundleWarnings...)

	// 2. Extract entries - handle both []interface{} and []map[string]interface{}
	var entries []interface{}
	if e, ok := bundle["entry"].([]interface{}); ok {
		entries = e
	} else if typedEntries, ok := bundle["entry"].([]map[string]interface{}); ok {
		entries = make([]interface{}, len(typedEntries))
		for i, te := range typedEntries {
			entries[i] = te
		}
	}
	resourceCount := len(entries)

	// 3. Validate each resource
	resourceTypesFound := make(map[string]bool)
	referenceTargets := make(map[string]bool) // fullUrl -> exists

	// First pass: collect all fullUrls for reference validation
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if fullUrl, ok := entryMap["fullUrl"].(string); ok {
			referenceTargets[fullUrl] = true
		}
		if resource, ok := entryMap["resource"].(map[string]interface{}); ok {
			if rt, ok := resource["resourceType"].(string); ok {
				if id, ok := resource["id"].(string); ok {
					referenceTargets[rt+"/"+id] = true
				}
			}
		}
	}

	// Second pass: validate each resource
	for i, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			allErrors = append(allErrors, fmt.Sprintf("Entry %d is not a valid object", i))
			continue
		}

		resource, ok := entryMap["resource"].(map[string]interface{})
		if !ok {
			allErrors = append(allErrors, fmt.Sprintf("Entry %d has no resource", i))
			continue
		}

		result := fve.validateResource(resource, config, referenceTargets)
		resourceResults = append(resourceResults, result)
		resourceTypesFound[result.ResourceType] = true

		allErrors = append(allErrors, result.Errors...)
		allWarnings = append(allWarnings, result.Warnings...)
	}

	// 4. Check required resources
	if len(config.RequiredResources) > 0 {
		for _, required := range config.RequiredResources {
			if !resourceTypesFound[required] {
				allErrors = append(allErrors, fmt.Sprintf("Required resource type '%s' not found in bundle", required))
			}
		}
	}

	return fve.buildValidationOutput(inputData, startTime, config, allErrors, allWarnings, resourceResults, resourceCount, "bundle")
}

// executeSingleResourceValidation validates a standalone FHIR resource (not in a Bundle)
func (fve *FHIRValidationExecutor) executeSingleResourceValidation(
	resource map[string]interface{},
	config fhirValidationConfig,
	inputData map[string]interface{},
	startTime time.Time,
) (map[string]interface{}, error) {
	rt, _ := resource["resourceType"].(string)
	log.Printf("  🔍 Validating standalone FHIR resource: %s", rt)

	var allErrors []string
	var allWarnings []string

	// No bundle-level reference targets for standalone resources
	referenceTargets := make(map[string]bool)

	// Validate the single resource
	result := fve.validateResource(resource, config, referenceTargets)

	allErrors = append(allErrors, result.Errors...)
	allWarnings = append(allWarnings, result.Warnings...)

	// Check required resources (the single resource must match if specified)
	if len(config.RequiredResources) > 0 {
		for _, required := range config.RequiredResources {
			if required != rt {
				allErrors = append(allErrors, fmt.Sprintf("Required resource type '%s' not found (got '%s')", required, rt))
			}
		}
	}

	return fve.buildValidationOutput(inputData, startTime, config, allErrors, allWarnings, []resourceValidationResult{result}, 1, "resource")
}

// buildValidationOutput constructs the output data with validation results
func (fve *FHIRValidationExecutor) buildValidationOutput(
	inputData map[string]interface{},
	startTime time.Time,
	config fhirValidationConfig,
	allErrors []string,
	allWarnings []string,
	resourceResults []resourceValidationResult,
	resourceCount int,
	validationMode string,
) (map[string]interface{}, error) {
	valid := len(allErrors) == 0
	resourcesValidated := make(map[string]bool)
	for _, r := range resourceResults {
		resourcesValidated[r.ResourceType] = r.Valid
	}

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"valid":               valid,
		"resource_count":      resourceCount,
		"error_count":         len(allErrors),
		"warning_count":       len(allWarnings),
		"errors":              allErrors,
		"warnings":            allWarnings,
		"resources_validated": resourcesValidated,
		"validation_mode":     validationMode,
	}

	details := map[string]interface{}{
		"duration_ms":      durationMs,
		"validation_level": config.ValidationLevel,
		"validation_mode":  validationMode,
		"success":          true,
		"resource_count":   resourceCount,
		"error_count":      len(allErrors),
		"resource_results": resourceResults,
	}

	fve.SetStepOutputWithDetails(outputData, variables, details)

	if valid {
		log.Printf("  ✅ FHIR validation passed (%s mode, %d resources, %d warnings)", validationMode, resourceCount, len(allWarnings))
	} else {
		log.Printf("  ⚠️ FHIR validation found %d errors, %d warnings (%s mode)", len(allErrors), len(allWarnings), validationMode)
		if config.FailOnError {
			return outputData, fmt.Errorf("FHIR validation failed: %d errors found (%s)", len(allErrors), strings.Join(allErrors, "; "))
		}
	}

	return outputData, nil
}

// findFHIRData looks for FHIR data (bundle or standalone resource) in the pipeline data.
// Returns the FHIR data map and a boolean indicating if it's a Bundle (true) or standalone resource (false).
func (fve *FHIRValidationExecutor) findFHIRData(inputData map[string]interface{}) (map[string]interface{}, bool) {
	// Search locations in priority order
	searchLocations := []struct {
		key    string
		parent map[string]interface{}
	}{
		// 1. Direct fhirBundle (from HL7→FHIR Transform)
		{"fhirBundle", inputData},
		// 2. Direct fhirResource (from API enrichment or standalone resource)
		{"fhirResource", inputData},
	}

	// Add nested locations (message.*, enriched.*)
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		searchLocations = append(searchLocations,
			struct {
				key    string
				parent map[string]interface{}
			}{"fhirBundle", msg},
			struct {
				key    string
				parent map[string]interface{}
			}{"fhirResource", msg},
		)
	}
	if enriched, ok := inputData["enriched"].(map[string]interface{}); ok {
		searchLocations = append(searchLocations,
			struct {
				key    string
				parent map[string]interface{}
			}{"fhirBundle", enriched},
			struct {
				key    string
				parent map[string]interface{}
			}{"fhirResource", enriched},
		)
	}

	// Search each location
	for _, loc := range searchLocations {
		if data, ok := loc.parent[loc.key].(map[string]interface{}); ok {
			rt, _ := data["resourceType"].(string)
			isBundle := rt == "Bundle"
			log.Printf("  🔍 Found FHIR data at '%s' (resourceType: %s, mode: %s)", loc.key, rt, map[bool]string{true: "bundle", false: "resource"}[isBundle])
			return data, isBundle
		}
	}

	return nil, false
}

// validateBundleStructure checks the top-level bundle structure
func (fve *FHIRValidationExecutor) validateBundleStructure(bundle map[string]interface{}) ([]string, []string) {
	var errors, warnings []string

	// Check resourceType
	rt, _ := bundle["resourceType"].(string)
	if rt != "Bundle" {
		errors = append(errors, fmt.Sprintf("Invalid resourceType: '%s' (expected 'Bundle')", rt))
	}

	// Check bundle type
	bundleType, _ := bundle["type"].(string)
	validTypes := map[string]bool{
		"document": true, "message": true, "transaction": true,
		"transaction-response": true, "batch": true, "batch-response": true,
		"history": true, "searchset": true, "collection": true,
	}
	if bundleType == "" {
		errors = append(errors, "Bundle missing required 'type' field")
	} else if !validTypes[bundleType] {
		warnings = append(warnings, fmt.Sprintf("Unusual bundle type: '%s'", bundleType))
	}

	// Check entry array exists
	if _, ok := bundle["entry"]; !ok {
		warnings = append(warnings, "Bundle has no 'entry' array")
	} else if entries, ok := bundle["entry"].([]interface{}); ok && len(entries) == 0 {
		warnings = append(warnings, "Bundle has empty 'entry' array")
	}

	return errors, warnings
}

// validateResource validates a single FHIR resource
func (fve *FHIRValidationExecutor) validateResource(
	resource map[string]interface{},
	config fhirValidationConfig,
	referenceTargets map[string]bool,
) resourceValidationResult {
	result := resourceValidationResult{Valid: true}

	// Check resourceType
	rt, ok := resource["resourceType"].(string)
	if !ok || rt == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "Resource missing 'resourceType'")
		result.ResourceType = "Unknown"
		return result
	}
	result.ResourceType = rt

	// Check id
	id, _ := resource["id"].(string)
	result.ResourceID = id
	if id == "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: missing 'id' field", rt))
	}

	// Basic level: just check resourceType and id
	if config.ValidationLevel == "basic" {
		return result
	}

	// Standard level: check required fields per resource type
	if config.ValidateRequiredFields {
		fve.validateRequiredFields(resource, rt, &result)
	}

	// Standard level: validate references within bundle
	if config.ValidateReferences {
		fve.validateReferences(resource, rt, referenceTargets, &result)
	}

	// Strict level: validate against FHIR schema if available
	if config.ValidationLevel == "strict" {
		fve.validateAgainstSchema(resource, rt, &result)
	}

	return result
}

// validateRequiredFields checks that known required fields are present
func (fve *FHIRValidationExecutor) validateRequiredFields(
	resource map[string]interface{},
	resourceType string,
	result *resourceValidationResult,
) {
	// FHIR R4 required fields per resource type
	requiredFields := map[string][]string{
		"Patient":       {},                       // Patient has no strictly required fields in base spec
		"Encounter":     {"status", "class"},
		"MessageHeader": {"event", "source"},
		"Observation":   {"status", "code"},
		"Condition":     {"subject"},
		"Procedure":     {"status", "subject"},
		"AllergyIntolerance": {"patient"},
		"DiagnosticReport":   {"status", "code"},
		"MedicationRequest":  {"status", "intent", "medication", "subject"},
		"Immunization":       {"status", "vaccineCode", "patient", "occurrence"},
		"Coverage":           {"status", "beneficiary"},
		"Practitioner":       {},
		"Organization":       {},
		"Location":           {},
	}

	fields, known := requiredFields[resourceType]
	if !known {
		return
	}

	for _, field := range fields {
		val, exists := resource[field]
		if !exists || val == nil {
			// Check inside the nested resource structure (our transform nests under resourceType key)
			if nested, ok := resource[resourceType].(map[string]interface{}); ok {
				if nVal, nExists := nested[field]; nExists && nVal != nil {
					continue
				}
			}
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: missing required field '%s'", resourceType, field))
			result.Valid = false
		}
	}
}

// validateReferences checks that reference targets exist within the bundle
func (fve *FHIRValidationExecutor) validateReferences(
	resource map[string]interface{},
	resourceType string,
	referenceTargets map[string]bool,
	result *resourceValidationResult,
) {
	fve.findAndValidateRefs(resource, resourceType, referenceTargets, result, "")
}

// findAndValidateRefs recursively finds reference objects and validates them
func (fve *FHIRValidationExecutor) findAndValidateRefs(
	data interface{},
	resourceType string,
	referenceTargets map[string]bool,
	result *resourceValidationResult,
	path string,
) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this is a Reference object
		if ref, ok := v["reference"].(string); ok {
			// Skip absolute URLs and contained references
			if !strings.HasPrefix(ref, "http") && !strings.HasPrefix(ref, "#") {
				if !referenceTargets[ref] && !strings.HasPrefix(ref, "urn:uuid:") || (strings.HasPrefix(ref, "urn:uuid:") && !referenceTargets[ref]) {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s%s: unresolved reference '%s'", resourceType, path, ref))
				}
			}
		}
		// Recurse into nested objects
		for key, val := range v {
			fve.findAndValidateRefs(val, resourceType, referenceTargets, result, path+"."+key)
		}
	case []interface{}:
		for i, item := range v {
			fve.findAndValidateRefs(item, resourceType, referenceTargets, result, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// validateAgainstSchema validates resource against FHIR R4 schema if available
func (fve *FHIRValidationExecutor) validateAgainstSchema(
	resource map[string]interface{},
	resourceType string,
	result *resourceValidationResult,
) {
	loader := fhir.GetFHIRSchemaLoader()
	if loader == nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%s: schema validation skipped (loader not available)", resourceType))
		return
	}

	schema, err := loader.LoadFHIRSchema(resourceType, "", "R4")
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%s: schema not found for strict validation", resourceType))
		return
	}

	// Check schema-defined required fields
	for _, reqField := range schema.Required {
		val, exists := resource[reqField]
		if !exists || val == nil {
			if nested, ok := resource[resourceType].(map[string]interface{}); ok {
				if nVal, nExists := nested[reqField]; nExists && nVal != nil {
					continue
				}
			}
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: schema requires field '%s'", resourceType, reqField))
			result.Valid = false
		}
	}

	// Validate element data types
	for elemPath, element := range schema.Elements {
		if !element.Required {
			continue
		}
		// Extract the field name from path (e.g., "Patient.gender" -> "gender")
		parts := strings.SplitN(elemPath, ".", 2)
		if len(parts) < 2 {
			continue
		}
		fieldName := parts[1]
		if strings.Contains(fieldName, ".") {
			continue // Skip nested paths for now
		}

		val, exists := resource[fieldName]
		if !exists || val == nil {
			if nested, ok := resource[resourceType].(map[string]interface{}); ok {
				if nVal, nExists := nested[fieldName]; nExists && nVal != nil {
					continue
				}
			}
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: required element '%s' is missing", resourceType, elemPath))
			result.Valid = false
		}
	}
}

func (fve *FHIRValidationExecutor) buildErrorOutput(
	inputData map[string]interface{},
	startTime time.Time,
	errorMsg string,
) (map[string]interface{}, error) {
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	variables := map[string]interface{}{
		"valid":          false,
		"resource_count": 0,
		"error_count":    1,
		"errors":         []string{errorMsg},
		"warnings":       []string{},
	}
	details := map[string]interface{}{
		"duration_ms": time.Since(startTime).Milliseconds(),
		"success":     true,
		"error_count": 1,
	}
	fve.SetStepOutputWithDetails(outputData, variables, details)
	return outputData, nil
}

func (fve *FHIRValidationExecutor) Validate(step *models.TransformationStep) error {
	if step.Config != nil {
		level, _ := step.Config["validation_level"].(string)
		if level != "" && level != "basic" && level != "standard" && level != "strict" {
			return fmt.Errorf("invalid validation_level: '%s' (must be basic, standard, or strict)", level)
		}
	}
	return nil
}
