package validation

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"
)

// FHIRValidationExecutor validates FHIR bundles and individual resources against
// the FHIR R4 specification using the compile-once r4.SchemaRegistry.
//
// The executor is registered under step type "fhir_validation" and the legacy
// alias "post.validation".  Output variables are identical to the previous
// implementation so existing pipelines need no changes.
type FHIRValidationExecutor struct {
	*executors.BaseExecutor
}

// NewFHIRValidationExecutor creates a new FHIR validation executor.
func NewFHIRValidationExecutor() *FHIRValidationExecutor {
	return &FHIRValidationExecutor{
		BaseExecutor: executors.NewBaseExecutor("fhir_validation", models.ExecutorMetadata{
			Name:        "FHIR Validation",
			Description: "Validates FHIR bundles and resources against R4 schema, terminology bindings, and constraints",
			Version:     "2.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Validation",
		}),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step config (parsed from step.Config JSON)
// ─────────────────────────────────────────────────────────────────────────────

type fhirValidationConfig struct {
	// ValidationLevel controls depth: "basic" | "standard" (default) | "strict"
	ValidationLevel string `json:"validation_level"`
	// Profile selects the FHIR profile: "base" (default) | "us-core"
	Profile string `json:"profile"`
	// FHIRVersion selects the specification version: "R4" (default) | "R5"
	FHIRVersion string `json:"fhir_version"`
	// RequiredResources lists resource types that MUST be present in a Bundle.
	RequiredResources []string `json:"required_resources"`
	// ValidateReferences enables internal Bundle reference resolution checks.
	ValidateReferences bool `json:"validate_references"`
	// FailOnError stops the pipeline when validation errors are found.
	FailOnError bool `json:"fail_on_error"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	cfg := fhirValidationConfig{
		ValidationLevel:    "standard",
		Profile:            "base",
		FHIRVersion:        "R4",
		ValidateReferences: true,
	}
	if step.Config != nil {
		if raw, err := json.Marshal(step.Config); err == nil {
			json.Unmarshal(raw, &cfg) //nolint:errcheck — defaults remain on parse failure
		}
	}

	log.Printf("  🔍 FHIR Validation starting (level: %s, profile: %s, version: %s)",
		cfg.ValidationLevel, cfg.Profile, cfg.FHIRVersion)

	opts := r4.ValidationOptions{
		Version:       cfg.FHIRVersion,
		Profile:       cfg.Profile,
		Level:         r4.ParseLevel(cfg.ValidationLevel),
		RequiredTypes: cfg.RequiredResources,
		ValidateRefs:  cfg.ValidateReferences,
	}

	fhirData, isBundle := fve.findFHIRData(inputData)
	if fhirData == nil {
		return fve.buildErrorOutput(inputData, startTime,
			"No FHIR data found in pipeline data (looked for fhirBundle, fhirResource, and resourceType)")
	}

	validator := r4.GetValidator()

	if isBundle {
		return fve.executeBundleValidation(validator, fhirData, opts, cfg, inputData, startTime)
	}
	return fve.executeResourceValidation(validator, fhirData, opts, cfg, inputData, startTime)
}

// ─────────────────────────────────────────────────────────────────────────────
// Bundle path
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) executeBundleValidation(
	validator *r4.FHIRR4Validator,
	bundle map[string]interface{},
	opts r4.ValidationOptions,
	cfg fhirValidationConfig,
	inputData map[string]interface{},
	startTime time.Time,
) (map[string]interface{}, error) {
	// Log exactly which resource types are present in the received bundle.
	entries, _ := bundle["entry"].([]interface{})
	var receivedTypes []string
	for _, e := range entries {
		if em, ok := e.(map[string]interface{}); ok {
			if res, ok := em["resource"].(map[string]interface{}); ok {
				if rt, ok := res["resourceType"].(string); ok {
					receivedTypes = append(receivedTypes, rt)
				}
			}
		}
	}
	log.Printf("  🔍 [FHIR Validation] Received bundle: %d entries, resource types: %v", len(entries), receivedTypes)
	if len(opts.RequiredTypes) > 0 {
		log.Printf("  🔍 [FHIR Validation] Required types (from config): %v", opts.RequiredTypes)
	}

	result := validator.ValidateBundle(bundle, opts)

	// Flatten all errors/warnings into the legacy string-slice format so that
	// downstream steps and the validation queue see the same shape as before.
	var allErrors, allWarnings []string
	for _, iss := range result.BundleIssues {
		if iss.Severity == "error" {
			allErrors = append(allErrors, fmtIssue(iss))
		} else {
			allWarnings = append(allWarnings, fmtIssue(iss))
		}
	}

	type resourceSummary struct {
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
		Valid        bool   `json:"valid"`
		Errors       []string `json:"errors,omitempty"`
		Warnings     []string `json:"warnings,omitempty"`
	}
	var resourceResults []resourceSummary
	resourcesValidated := make(map[string]bool)

	for _, rr := range result.Resources {
		sum := resourceSummary{
			ResourceType: rr.ResourceType,
			ResourceID:   rr.ResourceID,
			Valid:         rr.Valid,
		}
		for _, e := range rr.Errors {
			sum.Errors = append(sum.Errors, fmtIssue(e))
			allErrors = append(allErrors, fmtIssue(e))
		}
		for _, w := range rr.Warnings {
			sum.Warnings = append(sum.Warnings, fmtIssue(w))
			allWarnings = append(allWarnings, fmtIssue(w))
		}
		log.Printf("  🔍 [FHIR Validation] Resource %s/%s: valid=%v errors=%d warnings=%d",
			rr.ResourceType, rr.ResourceID, rr.Valid, len(sum.Errors), len(sum.Warnings))
		resourceResults = append(resourceResults, sum)
		resourcesValidated[rr.ResourceType] = rr.Valid
	}

	return fve.buildOutput(inputData, startTime, cfg, allErrors, allWarnings,
		resourceResults, len(result.Resources), "bundle")
}

// ─────────────────────────────────────────────────────────────────────────────
// Single-resource path
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) executeResourceValidation(
	validator *r4.FHIRR4Validator,
	resource map[string]interface{},
	opts r4.ValidationOptions,
	cfg fhirValidationConfig,
	inputData map[string]interface{},
	startTime time.Time,
) (map[string]interface{}, error) {
	rt, _ := resource["resourceType"].(string)
	log.Printf("  🔍 Validating standalone FHIR resource: %s", rt)

	rr := validator.ValidateResource(resource, opts)

	var allErrors, allWarnings []string
	type resourceSummary struct {
		ResourceType string   `json:"resource_type"`
		ResourceID   string   `json:"resource_id"`
		Valid        bool     `json:"valid"`
		Errors       []string `json:"errors,omitempty"`
		Warnings     []string `json:"warnings,omitempty"`
	}
	sum := resourceSummary{
		ResourceType: rr.ResourceType,
		ResourceID:   rr.ResourceID,
		Valid:         rr.Valid,
	}
	for _, e := range rr.Errors {
		s := fmtIssue(e)
		sum.Errors = append(sum.Errors, s)
		allErrors = append(allErrors, s)
	}
	for _, w := range rr.Warnings {
		s := fmtIssue(w)
		sum.Warnings = append(sum.Warnings, s)
		allWarnings = append(allWarnings, s)
	}

	// RequiredResources check for standalone: the resource type must match
	for _, required := range cfg.RequiredResources {
		if required != rt {
			allErrors = append(allErrors,
				fmt.Sprintf("Required resource type '%s' not found (got '%s')", required, rt))
		}
	}

	return fve.buildOutput(inputData, startTime, cfg, allErrors, allWarnings,
		[]interface{}{sum}, 1, "resource")
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared output builder — keeps the same variable/detail keys as v1 so that
// existing pipelines, tests, and the validation queue see no breaking change.
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) buildOutput(
	inputData map[string]interface{},
	startTime time.Time,
	cfg fhirValidationConfig,
	allErrors, allWarnings []string,
	resourceResults interface{},
	resourceCount int,
	mode string,
) (map[string]interface{}, error) {
	valid := len(allErrors) == 0

	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"valid":            valid,
		"resource_count":   resourceCount,
		"error_count":      len(allErrors),
		"warning_count":    len(allWarnings),
		"errors":           allErrors,
		"warnings":         allWarnings,
		"validation_mode":  mode,
	}

	details := map[string]interface{}{
		"duration_ms":      durationMs,
		"validation_level": cfg.ValidationLevel,
		"profile":          cfg.Profile,
		"fhir_version":     cfg.FHIRVersion,
		"validation_mode":  mode,
		"success":          true,
		"resource_count":   resourceCount,
		"error_count":      len(allErrors),
		"resource_results": resourceResults,
	}

	fve.SetStepOutputWithDetails(outputData, variables, details)

	if valid {
		log.Printf("  ✅ FHIR validation passed (%s, %d resources, %d warnings, %dms)",
			mode, resourceCount, len(allWarnings), durationMs)
	} else {
		log.Printf("  ⚠️  FHIR validation: %d errors, %d warnings (%s, %dms)",
			len(allErrors), len(allWarnings), mode, durationMs)
		if cfg.FailOnError {
			return outputData, fmt.Errorf("FHIR validation failed: %d errors — %s",
				len(allErrors), strings.Join(allErrors, "; "))
		}
	}

	return outputData, nil
}

func (fve *FHIRValidationExecutor) buildErrorOutput(
	inputData map[string]interface{},
	startTime time.Time,
	msg string,
) (map[string]interface{}, error) {
	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}
	variables := map[string]interface{}{
		"valid":          false,
		"resource_count": 0,
		"error_count":    1,
		"errors":         []string{msg},
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

// ─────────────────────────────────────────────────────────────────────────────
// FHIR data location — unchanged from v1
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) findFHIRData(inputData map[string]interface{}) (map[string]interface{}, bool) {
	type searchLoc struct {
		key    string
		parent map[string]interface{}
	}

	locations := []searchLoc{
		{"fhirBundle", inputData},
		{"fhirResource", inputData},
	}
	if msg, ok := inputData["message"].(map[string]interface{}); ok {
		locations = append(locations,
			searchLoc{"fhirBundle", msg},
			searchLoc{"fhirResource", msg},
		)
		// FHIR parser places resource fields at the root of execCtx.Message, so
		// inputData["message"]["resourceType"] IS the FHIR resource/bundle.
		if rt, ok := msg["resourceType"].(string); ok && rt != "" {
			isBundle := rt == "Bundle"
			log.Printf("  🔍 Found FHIR data at 'message.resourceType' (resourceType: %s, mode: %s)",
				rt, map[bool]string{true: "bundle", false: "resource"}[isBundle])
			return msg, isBundle
		}
	}
	if enriched, ok := inputData["enriched"].(map[string]interface{}); ok {
		locations = append(locations,
			searchLoc{"fhirBundle", enriched},
			searchLoc{"fhirResource", enriched},
		)
	}

	for _, loc := range locations {
		if data, ok := loc.parent[loc.key].(map[string]interface{}); ok {
			rt, _ := data["resourceType"].(string)
			isBundle := rt == "Bundle"
			log.Printf("  🔍 Found FHIR data at '%s' (resourceType: %s, mode: %s)",
				loc.key, rt, map[bool]string{true: "bundle", false: "resource"}[isBundle])
			return data, isBundle
		}
	}
	return nil, false
}

// ─────────────────────────────────────────────────────────────────────────────
// Validate — step-config validation (called before Execute)
// ─────────────────────────────────────────────────────────────────────────────

func (fve *FHIRValidationExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return nil
	}
	if level, ok := step.Config["validation_level"].(string); ok && level != "" {
		switch level {
		case "basic", "standard", "strict":
		default:
			return fmt.Errorf("invalid validation_level '%s': must be basic, standard, or strict", level)
		}
	}
	if profile, ok := step.Config["profile"].(string); ok && profile != "" {
		// Validate against registered profiles at call time so startup warnings
		// surface before the pipeline runs.
		reg := r4.GetRegistry()
		if reg != nil {
			version := "R4"
			if v, ok := step.Config["fhir_version"].(string); ok && v != "" {
				version = v
			}
			// We accept any profile name; unknown ones produce a warning at runtime.
			_ = version
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// fmtIssue converts a ValidationIssue to a human-readable string for the
// legacy errors/warnings string arrays.
func fmtIssue(iss r4.ValidationIssue) string {
	if iss.Path != "" {
		return fmt.Sprintf("[%s] %s: %s", iss.Code, iss.Path, iss.Message)
	}
	return fmt.Sprintf("[%s] %s", iss.Code, iss.Message)
}
