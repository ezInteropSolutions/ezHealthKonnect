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

// CodeTemplatePreambleLoader is the minimal interface the executor needs from
// CodeTemplateService. Using an interface keeps this package free of a direct
// import of the services package (which imports this package), avoiding a cycle.
type CodeTemplatePreambleLoader interface {
	LoadForExecution(interfaceID string) ([]*models.CodeTemplateForExecution, error)
	BuildPreamble(templates []*models.CodeTemplateForExecution) string
}

type ScriptEnrichmentExecutor struct {
	*executors.BaseExecutor
	codeTemplates CodeTemplatePreambleLoader // nil = no injection (backward compat)
}

// NewScriptEnrichmentExecutor creates a script enrichment executor without code template support.
func NewScriptEnrichmentExecutor() *ScriptEnrichmentExecutor {
	return newScriptExecutor(nil)
}

// NewScriptEnrichmentExecutorWithTemplates creates a script enrichment executor that
// automatically injects code template function libraries into the goja VM before
// executing the user's script.
func NewScriptEnrichmentExecutorWithTemplates(loader CodeTemplatePreambleLoader) *ScriptEnrichmentExecutor {
	return newScriptExecutor(loader)
}

func newScriptExecutor(loader CodeTemplatePreambleLoader) *ScriptEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Script Enrichment",
		Description: "Enriches messages using custom JavaScript code (age calculation, business rules, etc.)",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}
	base := executors.NewBaseExecutor("enrichment.script", metadata)
	return &ScriptEnrichmentExecutor{
		BaseExecutor:  base,
		codeTemplates: loader,
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
		log.Printf("⚠️  Script execution failed: %v", err)

		// Store error in output for debugging
		executors.SetNestedValue(inputData, "_script_error", err.Error())

		// STANDARDIZED: No variables on error + execution details
		e.SetStepOutputWithDetails(inputData, map[string]interface{}{}, map[string]interface{}{
			"script_error": err.Error(),
		})

		// Always return error — pipeline service decides retry/catch behavior
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// STANDARDIZED: Set step output for tracking.
	// P7: stop writing to enriched.script; _stepOutput is the canonical output.
	// Downstream steps access via steps.{ns}.step_output.{field}.
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
	e.SetStepOutputWithDetails(inputData, variables, map[string]interface{}{})

	log.Printf("✅ [Script Enrichment] Script executed successfully")
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

	// Recursion guard: limits call stack depth to prevent stack-overflow from deeply
	// recursive scripts. 500 frames ≈ 500 nested function calls before a JS exception fires.
	vm.SetMaxCallStackSize(500)

	// Fast CPU interrupt: wire the context's Done channel to goja's interrupt mechanism.
	// vm.Interrupt() fires on the next opcode boundary (typically microseconds), which is
	// far faster than waiting for Go's goroutine scheduler to preempt the VM goroutine.
	// This means a `while(true){}` loop stops almost immediately when the timeout fires
	// instead of burning CPU for the full timeout duration.
	// NOTE: heap memory limiting (SetMemoryLimit) is not available in the installed goja
	// version. Container-level memory limits (docker --memory) serve as the outer guard.
	go func() {
		<-scriptCtx.Done()
		vm.Interrupt("script execution limit reached")
	}()

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

		// Set input data in VM (read-only view of the pipeline message)
		if err := vm.Set("input", inputData); err != nil {
			errorChan <- fmt.Errorf("failed to set input data: %w", err)
			return
		}

		// Pre-inject an empty `output` object so scripts can write to it without
		// declaring it first.  If the script neither returns a value nor modifies
		// output, the step produces an empty step_output (no-op, not an error).
		if err := vm.Set("output", map[string]interface{}{}); err != nil {
			errorChan <- fmt.Errorf("failed to set output object: %w", err)
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

		// Build code template preamble (injected at module scope so its functions
		// are callable from inside the IIFE that wraps the user's script).
		preamble := ""
		if e.codeTemplates != nil {
			interfaceID, _ := inputData["_interfaceId"].(string)
			if tpls, err := e.codeTemplates.LoadForExecution(interfaceID); err != nil {
				log.Printf("   ⚠️  [Script] Code template load failed: %v (continuing without)", err)
			} else if len(tpls) > 0 {
				preamble = e.codeTemplates.BuildPreamble(tpls)
				log.Printf("   📦 [Script] Injected %d code template(s) for interface '%s'", len(tpls), interfaceID)
			}
		}

		// Two script styles must both keep working:
		//   1. A bare trailing expression with no `return` (the original,
		//      still-documented style — e.g. `({ result: 42 });`). Running
		//      this *unwrapped* gives the natural JS completion-value
		//      semantics: the value of the last expression statement.
		//   2. An explicit `return ...;` statement (added later for control
		//      flow / early exit). `return` outside a function body is a
		//      compile-time SyntaxError, so these scripts can only run
		//      wrapped in an IIFE.
		// Try unwrapped first — it compiles for both plain-statement scripts
		// AND scripts that only mutate `output` (e.g. `output.x = 1;`), so a
		// successful unwrapped compile doesn't by itself mean "no return was
		// used". The two styles are disambiguated by the *type* of the
		// completion value below, not by which path compiled.
		unwrappedScript := preamble + config.Script
		wrappedScript := preamble + fmt.Sprintf("(function() {\n%s\n})()", config.Script)

		var value goja.Value
		var err error
		usedWrapped := false
		if script, compileErr := goja.Compile("enrichment_script", unwrappedScript, false); compileErr == nil {
			value, err = vm.RunProgram(script)
			if err != nil {
				errorChan <- fmt.Errorf("script execution error: %w", err)
				return
			}
		} else {
			// Unwrapped compile failed — most commonly a top-level `return`.
			// Fall back to the IIFE-wrapped form, which supports it.
			usedWrapped = true
			wrappedProgram, wrapCompileErr := goja.Compile("enrichment_script", wrappedScript, false)
			if wrapCompileErr != nil {
				errorChan <- fmt.Errorf("failed to compile script: %w", wrapCompileErr)
				return
			}
			value, err = vm.RunProgram(wrappedProgram)
			if err != nil {
				errorChan <- fmt.Errorf("script execution error: %w", err)
				return
			}
		}

		// Determine the result:
		//  1. Wrapped (explicit return) path: any non-nil returned value is
		//     intentional — use it directly, even a bare scalar.
		//  2. Unwrapped path: only a map/array completion value is treated as
		//     an intentional result (covers the bare-object-literal style).
		//     A bare scalar completion value (e.g. from a trailing assignment
		//     statement like `output.status = "active";`) is incidental, not
		//     an intended return — fall through to the `output` mutation
		//     check instead, same as when there's no completion value at all.
		var result interface{}
		exported := value.Export()
		if usedWrapped {
			result = exported
		} else {
			switch exported.(type) {
			case map[string]interface{}, []interface{}:
				result = exported
			default:
				result = nil
			}
		}
		if result == nil {
			outputVal := vm.Get("output")
			if outputVal != nil && !goja.IsUndefined(outputVal) && !goja.IsNull(outputVal) {
				result = outputVal.Export()
			}
		}
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

	// Add getNestedValue helper - extracts nested values from input data.
	// Documented/called as getNestedValue(input, "path") everywhere (see
	// GetConfigExample below and every UsageExample across the codebase), but
	// the lookup always runs against the closure's own inputData regardless
	// of what's passed as the first argument — so the path is taken as the
	// *last* argument, supporting that 2-arg convention.
	vm.Set("getNestedValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}

		path := call.Arguments[len(call.Arguments)-1].String()
		value := executors.GetNestedValue(inputData, path)
		return vm.ToValue(value)
	})

	// Add getHL7Field helper - extracts HL7 field using short notation (e.g., PID.5.1).
	// Both getHL7Field("PID.5") and getHL7Field(input, "PID.5") are documented
	// call conventions in the UI (ScriptEnrichmentEditor.js / PropertiesPanel.js
	// respectively) — path is taken as the last argument to support both.
	vm.Set("getHL7Field", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}

		fieldPath := call.Arguments[len(call.Arguments)-1].String()
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
				"description": "Deprecated (P7): script output is now accessed via steps.{ns}.step_output.*",
				"deprecated":  true,
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Script execution timeout in milliseconds",
				"default":     5000,
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
		"targetPath": "enriched.script.demographics",
		"timeoutMs":  5000,
	}
}

// ===============================================================
// VARIABLE PROVIDER INTERFACE IMPLEMENTATION
// ===============================================================

// GetOutputVariables returns the list of variables this executor will produce.
// For script enrichment, we can't parse JavaScript to determine exact output fields,
// so we return the step_output container path — run the test pipeline to see actual fields.
func (e *ScriptEnrichmentExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	// Compute the runtime namespace — same logic as the pipeline service uses
	namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	stepOutputBase := fmt.Sprintf("steps.%s.step_output", namespace)

	// Can't know exact output fields without running the script.
	// Return the container path; the test-run IntelliSense walk will enumerate actual fields.
	variables = append(variables, models.VariableDefinition{
		Name:         "script_result",
		Path:         stepOutputBase,
		DataType:     "object",
		Description:  "JavaScript enrichment results — run the test pipeline to see individual field paths in IntelliSense",
		UsageExample: fmt.Sprintf("%s.fieldName", stepOutputBase),
		Category:     "Script",
		Examples: []string{
			fmt.Sprintf("%s.age", stepOutputBase),
			fmt.Sprintf("%s.riskScore", stepOutputBase),
			fmt.Sprintf("%s.calculatedAt", stepOutputBase),
		},
	})

	return variables
}
