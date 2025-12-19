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

	base := executors.NewBaseExecutor("pre.enrichment.script", metadata)

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

	log.Printf("📜 [Script Enrichment] Executing custom script")

	// Execute script with timeout
	result, err := e.executeScript(ctx, config, inputData)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

		log.Printf("⚠️  Script execution failed, continuing without enrichment: %v", err)
		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Store result in target path
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.script"
	}

	executors.SetNestedValue(inputData, targetPath, result)

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

		// Set input data in VM
		if err := vm.Set("input", inputData); err != nil {
			errorChan <- fmt.Errorf("failed to set input data: %w", err)
			return
		}

		// Set context variables if configured
		if config.Context != nil {
			for key, value := range config.Context {
				if err := vm.Set(key, value); err != nil {
					errorChan <- fmt.Errorf("failed to set context variable %s: %w", key, err)
					return
				}
			}
		}

		// Add utility functions
		e.addUtilityFunctions(vm)

		// Compile and run script
		script, err := goja.Compile("enrichment_script", config.Script, false)
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
func (e *ScriptEnrichmentExecutor) addUtilityFunctions(vm *goja.Runtime) {
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

	// Add getNestedValue helper
	vm.Set("getNestedValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Null()
		}

		obj := call.Arguments[0].Export()
		path := call.Arguments[1].String()

		if objMap, ok := obj.(map[string]interface{}); ok {
			value := executors.GetNestedValue(objMap, path)
			return vm.ToValue(value)
		}

		return goja.Null()
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
			"context": map[string]interface{}{
				"type":        "object",
				"description": "Additional variables to make available in script context",
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
		"context": map[string]interface{}{
			"hospitalId":  "HOSPITAL_001",
			"environment": "production",
		},
		"targetPath":  "enriched.demographics.age",
		"timeoutMs":   5000,
		"failOnError": false,
	}
}
