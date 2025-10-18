package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"ezhealthkonnect/models"
)

// StepExecutor interface - all executors must implement this
type StepExecutor interface {
	// Execute runs the transformation step
	Execute(
		ctx context.Context,
		step *models.TransformationStep,
		inputData map[string]interface{},
	) (map[string]interface{}, error)

	// GetStepType returns the step type this executor handles
	GetStepType() string

	// Validate validates step configuration
	Validate(step *models.TransformationStep) error
}

// ExecutorRegistry manages all available step executors (Factory Pattern + OOB)
type ExecutorRegistry struct {
	executors map[string]StepExecutor
	db        *sql.DB
}

// NewExecutorRegistry creates a new executor registry with auto-registration (OOB)
func NewExecutorRegistry(db *sql.DB) *ExecutorRegistry {
	registry := &ExecutorRegistry{
		executors: make(map[string]StepExecutor),
		db:        db,
	}

	// OOB: Auto-register all built-in executors
	registry.autoRegisterExecutors()

	log.Printf("✅ Executor Registry initialized with %d executors", len(registry.executors))

	return registry
}

// autoRegisterExecutors registers all built-in executors (OOB pattern)
func (er *ExecutorRegistry) autoRegisterExecutors() {
	// Pre-processing executors
	er.Register(NewValidationExecutor(er.db))
	er.Register(NewEnrichmentExecutor(er.db))

	// Core executors
	er.Register(NewHL7FHIRMappingExecutor(er.db))

	// Post-processing executors
	er.Register(NewFHIRValidationExecutor())

	// Custom executors
	er.Register(NewJavaScriptExecutor())
	er.Register(NewGenericExecutor())

	log.Println("  ✓ Registered: Validation, Enrichment, HL7→FHIR Mapping, FHIR Validation, JavaScript, Generic")
}

// Register adds an executor to the registry
func (er *ExecutorRegistry) Register(executor StepExecutor) {
	stepType := executor.GetStepType()
	er.executors[stepType] = executor
	log.Printf("  📝 Registered executor: %s", stepType)
}

// GetExecutor returns the executor for a given step type
func (er *ExecutorRegistry) GetExecutor(stepType string) StepExecutor {
	if executor, exists := er.executors[stepType]; exists {
		return executor
	}

	// Fallback to generic executor
	log.Printf("⚠️  No specific executor for '%s', using generic executor", stepType)
	return er.executors["generic"]
}

// ListExecutors returns all registered executor types
func (er *ExecutorRegistry) ListExecutors() []string {
	types := make([]string, 0, len(er.executors))
	for stepType := range er.executors {
		types = append(types, stepType)
	}
	return types
}

// ===============================================================
// VALIDATION EXECUTOR
// ===============================================================

// ValidationExecutor handles validation steps
type ValidationExecutor struct {
	db *sql.DB
}

func NewValidationExecutor(db *sql.DB) *ValidationExecutor {
	return &ValidationExecutor{db: db}
}

func (ve *ValidationExecutor) GetStepType() string {
	return "pre.validation"
}

func (ve *ValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// Extract validation rules from config
	rules, ok := step.Config["rules"].([]interface{})
	if !ok {
		return inputData, nil // No rules, pass through
	}

	var errors []string

	for _, ruleInterface := range rules {
		rule, ok := ruleInterface.(map[string]interface{})
		if !ok {
			continue
		}

		field, _ := rule["field"].(string)
		required, _ := rule["required"].(bool)

		if required {
			// Check if field exists in input data
			value := getNestedValue(inputData, field)
			if value == nil || value == "" {
				errors = append(errors, fmt.Sprintf("Required field missing: %s", field))
			}
		}
	}

	if len(errors) > 0 {
		return inputData, fmt.Errorf("validation failed: %v", errors)
	}

	// Validation passed - pass through input
	return inputData, nil
}

func (ve *ValidationExecutor) Validate(step *models.TransformationStep) error {
	if step.Config["rules"] == nil {
		return fmt.Errorf("validation step requires 'rules' in config")
	}
	return nil
}

// ===============================================================
// ENRICHMENT EXECUTOR
// ===============================================================

// EnrichmentExecutor handles enrichment steps (API calls, database lookups)
type EnrichmentExecutor struct {
	db *sql.DB
}

func NewEnrichmentExecutor(db *sql.DB) *EnrichmentExecutor {
	return &EnrichmentExecutor{db: db}
}

func (ee *EnrichmentExecutor) GetStepType() string {
	return "pre.enrichment"
}

func (ee *EnrichmentExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// TODO: Implement actual enrichment logic
	// For MVP, this is a placeholder that passes through

	log.Printf("  📍 Enrichment step: %s (placeholder - implement API call)", step.StepName)

	// Example: Call external API
	// apiURL := step.Config["api_url"].(string)
	// response := callExternalAPI(apiURL, inputData)
	// merge response into inputData

	return inputData, nil
}

func (ee *EnrichmentExecutor) Validate(step *models.TransformationStep) error {
	return nil // Placeholder
}

// ===============================================================
// HL7→FHIR MAPPING EXECUTOR
// ===============================================================

// HL7FHIRMappingExecutor handles HL7 to FHIR transformation
type HL7FHIRMappingExecutor struct {
	db                *sql.DB
	transformService  *HL7FHIRTransformServiceV3 // Reuse existing service
}

func NewHL7FHIRMappingExecutor(db *sql.DB) *HL7FHIRMappingExecutor {
	return &HL7FHIRMappingExecutor{
		db:               db,
		transformService: NewHL7FHIRTransformServiceV3(db),
	}
}

func (hme *HL7FHIRMappingExecutor) GetStepType() string {
	return "core.mapping"
}

func (hme *HL7FHIRMappingExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  🔄 HL7→FHIR transformation starting...")

	// Get interface ID from config or input
	interfaceID, _ := step.Config["interface_id"].(string)
	if interfaceID == "" {
		interfaceID, _ = inputData["interface_id"].(string)
	}

	messageType, _ := inputData["messageType"].(string)

	// Call existing transformation service using Transform method
	req := &TransformRequest{
		ParsedHL7Data: inputData,
		MessageType:   messageType,
		FHIRVersion:   "R4",
		CreateBundle:  true,
		InterfaceID:   interfaceID,
	}

	resp, err := hme.transformService.Transform(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("HL7→FHIR transformation failed: %w", err)
	}

	// Add FHIR bundle to output
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["fhirBundle"] = resp.Bundle

	log.Printf("  ✅ HL7→FHIR transformation complete")

	return outputData, nil
}

func (hme *HL7FHIRMappingExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// FHIR VALIDATION EXECUTOR
// ===============================================================

// FHIRValidationExecutor validates FHIR bundles
type FHIRValidationExecutor struct{}

func NewFHIRValidationExecutor() *FHIRValidationExecutor {
	return &FHIRValidationExecutor{}
}

func (fve *FHIRValidationExecutor) GetStepType() string {
	return "post.validation"
}

func (fve *FHIRValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// Extract FHIR bundle
	fhirBundle, ok := inputData["fhirBundle"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no fhirBundle found for validation")
	}

	// Basic validation
	resourceType, _ := fhirBundle["resourceType"].(string)
	if resourceType != "Bundle" {
		return nil, fmt.Errorf("invalid FHIR resource type: %s (expected Bundle)", resourceType)
	}

	// TODO: Add comprehensive FHIR validation
	// - Validate against R4 schema
	// - Check required fields
	// - Validate references

	log.Printf("  ✅ FHIR validation passed")

	return inputData, nil
}

func (fve *FHIRValidationExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// JAVASCRIPT EXECUTOR (Custom Scripts)
// ===============================================================

// JavaScriptExecutor executes custom JavaScript code
type JavaScriptExecutor struct{}

func NewJavaScriptExecutor() *JavaScriptExecutor {
	return &JavaScriptExecutor{}
}

func (jse *JavaScriptExecutor) GetStepType() string {
	return "custom.javascript"
}

func (jse *JavaScriptExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// TODO: Implement JavaScript runtime (goja)
	// For MVP, this is a placeholder

	if step.ScriptContent == nil || *step.ScriptContent == "" {
		return inputData, fmt.Errorf("no script content provided")
	}

	log.Printf("  📝 JavaScript execution: %s (placeholder - implement goja)", step.StepName)

	// Example goja implementation:
	// vm := goja.New()
	// vm.Set("input", inputData)
	// result, err := vm.RunString(*step.ScriptContent)
	// return result as map[string]interface{}

	return inputData, nil
}

func (jse *JavaScriptExecutor) Validate(step *models.TransformationStep) error {
	if step.ScriptContent == nil || *step.ScriptContent == "" {
		return fmt.Errorf("JavaScript step requires script_content")
	}
	return nil
}

// ===============================================================
// GENERIC EXECUTOR (Fallback)
// ===============================================================

// GenericExecutor is a fallback executor for unknown step types
type GenericExecutor struct{}

func NewGenericExecutor() *GenericExecutor {
	return &GenericExecutor{}
}

func (ge *GenericExecutor) GetStepType() string {
	return "generic"
}

func (ge *GenericExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	log.Printf("  ⚙️  Generic executor: %s (pass-through)", step.StepName)

	// Generic executor just passes through the input
	return inputData, nil
}

func (ge *GenericExecutor) Validate(step *models.TransformationStep) error {
	return nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// getNestedValue retrieves a value from nested map using dot notation
// Example: getNestedValue(data, "patient.name.family") => data["patient"]["name"]["family"]
func getNestedValue(data map[string]interface{}, path string) interface{} {
	keys := splitPath(path)
	current := interface{}(data)

	for _, key := range keys {
		if currentMap, ok := current.(map[string]interface{}); ok {
			current = currentMap[key]
		} else {
			return nil
		}
	}

	return current
}

// splitPath splits a dot-notation path into keys
func splitPath(path string) []string {
	// Simple implementation - can be enhanced for array indices
	result := []string{}
	current := ""

	for _, char := range path {
		if char == '.' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}
