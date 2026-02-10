package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"time"

	"github.com/dop251/goja"
)

// ===============================================================
// SCRIPT ENRICHMENT EXECUTOR
// ===============================================================
// Executes custom JavaScript code to enrich message data
// Implements Strategy Pattern - concrete strategy for script-based enrichment

type ScriptEnrichmentExecutor struct {
	*executors.BaseExecutor
}

// NewScriptEnrichmentExecutor creates a new script enrichment executor
func NewScriptEnrichmentExecutor() *ScriptEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Script Enrichment",
		Description: "Enriches messages using custom JavaScript code (age calculation, business rules, etc.)",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}

	base := executors.NewBaseExecutor("enrichment.script", metadata)

	return &ScriptEnrichmentExecutor{
		BaseExecutor: base,
	}
}

// Execute performs script-based enrichment
func (e *ScriptEnrichmentExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	log.Printf("📜📜📜 [Script Enrichment] EXECUTING CUSTOM SCRIPT FOR STEP: %s 📜📜📜", step.StepName)

	// Execute script with timeout
	result, err := e.executeScript(ctx, config, inputData)
	log.Printf("📜📜📜 [Script Enrichment] Script execution completed. Error: %v, Result is nil: %v", err, result == nil)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

		log.Printf("⚠️  Script execution failed, continuing without enrichment: %v", err)

		// Store error in output for debugging (even when not failing)
		executors.SetNestedValue(inputData, "_script_error", err.Error())

		// STANDARDIZED: No variables on error + execution details
		e.SetStepOutputWithDetails(inputData, map[string]interface{}{}, map[string]interface{}{
			"script_error": err.Error(),
			"target_path":  config.TargetPath,
		})

		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Store result in target path
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.script"
	}

	executors.SetNestedValue(inputData, targetPath, result)

	// STANDARDIZED: Set step output for tracking
	// Store the actual script result directly (flatten if it's a map)
	var variables map[string]interface{}

	// Debug: Log the actual result type and value
	log.Printf("🔍 [Script Enrichment] Result type: %T, value: %v, isNil: %v", result, result, result == nil)

	if resultMap, ok := result.(map[string]interface{}); ok {
		// Script returned an object - store fields directly as variables
		log.Printf("   📦 Script returned object with %d fields", len(resultMap))
		variables = resultMap
	} else {
		// Script returned a primitive or array - wrap it
		log.Printf("   📦 Script returned primitive/array, wrapping in script_result")
		variables = map[string]interface{}{
			"script_result": result,
		}
	}

	// STANDARDIZED: Variables (script output) + execution details
	e.SetStepOutputWithDetails(inputData, variables, map[string]interface{}{
		"target_path": targetPath,
	})

	log.Printf("✅ [Script Enrichment] Result stored at: %s", targetPath)
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// Validate checks if the step configuration is valid
func (e *ScriptEnrichmentExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses and validates the step configuration
func (e *ScriptEnrichmentExecutor) parseConfig(step *models.TransformationStep) (*models.ScriptEnrichmentConfig, error) {
	if step.Config == nil {
		return nil, fmt.Errorf("script enrichment requires configuration")
	}

	// Marshal to JSON then unmarshal to struct for type safety
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.ScriptEnrichmentConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if config.Script == "" {
		return nil, fmt.Errorf("script is required for script enrichment")
	}

	// Set defaults
	if config.TimeoutMs == 0 {
		config.TimeoutMs = 5000 // Default 5 seconds
	}

	return &config, nil
}

// executeScript executes the JavaScript code with timeout
func (e *ScriptEnrichmentExecutor) executeScript(
	ctx context.Context,
	config *models.ScriptEnrichmentConfig,
	inputData map[string]interface{},
) (interface{}, error) {

	// Create timeout context
	scriptCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMs)*time.Millisecond)
	defer cancel()

	// Create JavaScript runtime
	vm := goja.New()

	// Channel for script result
	resultChan := make(chan interface{}, 1)
	errorChan := make(chan error, 1)

	// Execute script in goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("script panic: %v", r)
			}
		}()

		// Debug: Log what the script receives
		inputKeys := make([]string, 0, len(inputData))
		for k := range inputData {
			inputKeys = append(inputKeys, k)
		}
		log.Printf("   🔍 Script input keys: %v", inputKeys)

		// Check if enriched exists
		if enriched, ok := inputData["enriched"].(map[string]interface{}); ok {
			enrichedKeys := make([]string, 0, len(enriched))
			for k := range enriched {
				enrichedKeys = append(enrichedKeys, k)
			}
			log.Printf("   🔍 enriched subkeys: %v", enrichedKeys)
		} else {
			log.Printf("   ⚠️  'enriched' key not found or not a map in inputData!")
		}

		// Set input data in VM (DEPRECATED - use $vars instead)
		if err := vm.Set("input", inputData); err != nil {
			errorChan <- fmt.Errorf("failed to set input data: %w", err)
			return
		}

		// NO-CODE: Inject flat variable namespace for easy access
		// Example: $vars.field_mapping.risk_weights instead of input.enriched.field_mapping.riskWeights
		if varContext, ok := inputData["_variableContext"].(*models.PipelineVariableContext); ok {
			vars := varContext.GetAll()
			if err := vm.Set("$vars", vars); err != nil {
				log.Printf("   ⚠️  Failed to set $vars: %v", err)
			} else {
				log.Printf("   📋 Injected $vars with %d variables", len(vars))
			}
		}

		// Set context variables if configured (DEPRECATED)
		if config.Context != nil && len(config.Context) > 0 {
			log.Printf("   ⚠️  [DEPRECATED] Context variables used in script. Use Metadata/Database Enrichment steps instead.")
			for key, value := range config.Context {
				if err := vm.Set(key, value); err != nil {
					errorChan <- fmt.Errorf("failed to set context variable %s: %w", key, err)
					return
				}
			}
		}

		// Add utility functions with access to input data
		e.addUtilityFunctions(vm, inputData)

		// Wrap script in a function to allow return statements
		wrappedScript := fmt.Sprintf("(function() { %s })()", config.Script)

		// Compile and run script
		script, err := goja.Compile("enrichment_script", wrappedScript, false)
		if err != nil {
			errorChan <- fmt.Errorf("failed to compile script: %w", err)
			return
		}

		value, err := vm.RunProgram(script)
		if err != nil {
			errorChan <- fmt.Errorf("script execution error: %w", err)
			return
		}

		// Export the result
		result := value.Export()
		resultChan <- result
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return nil, err
	case <-scriptCtx.Done():
		return nil, fmt.Errorf("script execution timeout after %dms", config.TimeoutMs)
	}
}

// addUtilityFunctions adds helpful utility functions to the JavaScript runtime
func (e *ScriptEnrichmentExecutor) addUtilityFunctions(vm *goja.Runtime, inputData map[string]interface{}) {
	// Add console.log for debugging
	console := vm.NewObject()
	console.Set("log", func(call goja.FunctionCall) goja.Value {
		var args []interface{}
		for _, arg := range call.Arguments {
			args = append(args, arg.Export())
		}
		log.Printf("   [Script] %v", args)
		return goja.Undefined()
	})
	vm.Set("console", console)

	// Add Date helper
	vm.Set("parseHL7Date", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		hl7Date := call.Arguments[0].String()
		if len(hl7Date) < 8 {
			return goja.Null()
		}

		// Parse HL7 date format: YYYYMMDD or YYYYMMDDHHmmss
		year := hl7Date[0:4]
		month := hl7Date[4:6]
		day := hl7Date[6:8]

		dateStr := fmt.Sprintf("%s-%s-%s", year, month, day)

		if len(hl7Date) >= 14 {
			hour := hl7Date[8:10]
			minute := hl7Date[10:12]
			second := hl7Date[12:14]
			dateStr += fmt.Sprintf("T%s:%s:%s", hour, minute, second)
		}

		return vm.ToValue(dateStr)
	})

	// Add getNestedValue helper - extracts nested values from input data
	vm.Set("getNestedValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}

		path := call.Arguments[0].String()
		value := executors.GetNestedValue(inputData, path)
		return vm.ToValue(value)
	})

	// Add getHL7Field helper - extracts HL7 field using short notation (e.g., PID.5.1)
	vm.Set("getHL7Field", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}

		fieldPath := call.Arguments[0].String()
		value := executors.GetNestedValue(inputData, fieldPath)
		return vm.ToValue(value)
	})

	// Add calculateAge helper
	vm.Set("calculateAge", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		hl7Date := call.Arguments[0].String()
		if len(hl7Date) < 8 {
			return goja.Null()
		}

		// Parse birth date
		yearStr := hl7Date[0:4]
		monthStr := hl7Date[4:6]
		dayStr := hl7Date[6:8]

		// Convert to integers
		year := 0
		month := 0
		day := 0
		fmt.Sscanf(yearStr, "%d", &year)
		fmt.Sscanf(monthStr, "%d", &month)
		fmt.Sscanf(dayStr, "%d", &day)

		birthDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		today := time.Now()

		age := today.Year() - birthDate.Year()
		if today.YearDay() < birthDate.YearDay() {
			age--
		}

		return vm.ToValue(age)
	})
}

// GetConfigSchema returns the JSON schema for configuration
func (e *ScriptEnrichmentExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":     "object",
		"required": []string{"script"},
		"properties": map[string]interface{}{
			"script": map[string]interface{}{
				"type":        "string",
				"description": "JavaScript code to execute (use 'input' variable for message data)",
			},
			"targetPath": map[string]interface{}{
				"type":        "string",
				"description": "Where to store the script result",
				"default":     "enriched.script",
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Script execution timeout in milliseconds",
				"default":     5000,
			},
			"failOnError": map[string]interface{}{
				"type":        "boolean",
				"description": "Stop pipeline if script fails",
				"default":     false,
			},
		},
	}
}

// GetConfigExample returns an example configuration
func (e *ScriptEnrichmentExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"script": `
// Extract patient date of birth from HL7 message
var dob = getNestedValue(input, "enhancedSegments.PID.fields.7.value"); // PID.7

// Calculate age using helper function
var age = calculateAge(dob);

// Get configuration from previous enrichment step (if needed)
var config = getNestedValue(input, "enriched.metadata.config");

// Determine age group
var ageGroup = "unknown";
if (age < 18) {
    ageGroup = "pediatric";
} else if (age >= 18 && age < 65) {
    ageGroup = "adult";
} else if (age >= 65) {
    ageGroup = "geriatric";
}

// Log for debugging
console.log("Calculated age:", age, "Group:", ageGroup);

// Return enrichment data
return {
    age: age,
    ageGroup: ageGroup,
    calculatedAt: new Date().toISOString()
};
`,
		"targetPath":  "enriched.script.demographics",
		"timeoutMs":   5000,
		"failOnError": false,
	}
}

// ===============================================================
// VARIABLE PROVIDER INTERFACE IMPLEMENTATION
// ===============================================================

// GetOutputVariables returns the list of variables this executor will produce
// For script enrichment, we can't parse JavaScript to determine exact output fields,
// so we return a generic variable pointing to the target path
func (e *ScriptEnrichmentExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	// Parse config to get target path
	config, err := e.parseConfig(step)
	if err != nil {
		log.Printf("⚠️  [ScriptEnrichment] Failed to parse config for variable discovery: %v", err)
		return variables
	}

	// Determine target path where results will be stored
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.script"
	}

	// For script enrichment, we can't determine exact output fields without executing the script
	// Return a generic object variable that users can reference with dot notation
	variables = append(variables, models.VariableDefinition{
		Name:         "script_result",
		Path:         targetPath,
		DataType:     "object",
		Description:  "JavaScript enrichment results (access fields with .fieldName)",
		UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.fieldName")`, targetPath),
		Category:     "Script",
		Examples: []string{
			fmt.Sprintf(`%s.age`, targetPath),
			fmt.Sprintf(`%s.riskScore`, targetPath),
			fmt.Sprintf(`%s.calculatedAt`, targetPath),
		},
	})

	return variables
}
