package control

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"time"
)

// TryCatchExecutor implements try-catch-finally error handling for child steps
type TryCatchExecutor struct {
	*executors.BaseExecutor
	childExecutor ChildStepExecutor
}

func NewTryCatchExecutor() *TryCatchExecutor {
	return &TryCatchExecutor{
		BaseExecutor: executors.NewBaseExecutor("control.try_catch", models.ExecutorMetadata{
			Name:        "Try-Catch Block",
			Description: "Wraps steps in error handling with try/catch/finally blocks",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Control Flow",
		}),
	}
}

func (e *TryCatchExecutor) SetChildExecutor(executor ChildStepExecutor) {
	e.childExecutor = executor
}

type tryCatchConfig struct {
	TrySteps     []string `json:"trySteps"`     // Step IDs to execute in try block
	CatchSteps   []string `json:"catchSteps"`   // Step IDs to execute on error
	FinallySteps []string `json:"finallySteps"` // Step IDs to always execute
	OnError      string   `json:"onError"`      // "catch" (default), "suppress", "rethrow"
}

func (e *TryCatchExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := tryCatchConfig{OnError: "catch"}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	// Also support childSteps for try block (backward compat with toolbox UI)
	if len(config.TrySteps) == 0 {
		if childSteps, ok := step.Config["childSteps"].([]interface{}); ok {
			for _, cs := range childSteps {
				if id, ok := cs.(string); ok {
					config.TrySteps = append(config.TrySteps, id)
				}
			}
		}
	}

	log.Printf("  🔒 Try-Catch starting: %d try steps, %d catch steps, %d finally steps",
		len(config.TrySteps), len(config.CatchSteps), len(config.FinallySteps))

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	var tryError error
	var tryErrorMessage string
	trySuccess := true
	currentInput := inputData

	// === TRY BLOCK ===
	if e.childExecutor != nil && len(config.TrySteps) > 0 {
		for i, childStepID := range config.TrySteps {
			childOutput, stepName, err := e.childExecutor(ctx, childStepID, currentInput)
			if err != nil {
				tryError = err
				tryErrorMessage = err.Error()
				trySuccess = false
				log.Printf("  ❌ Try block failed at step %d/%d (%s): %v",
					i+1, len(config.TrySteps), stepName, err)
				break
			}
			if childOutput != nil {
				currentInput = childOutput
				for k, v := range childOutput {
					outputData[k] = v
				}
			}
			log.Printf("  ✅ Try step %d/%d (%s) completed", i+1, len(config.TrySteps), stepName)
		}
	}

	// === CATCH BLOCK ===
	if !trySuccess && e.childExecutor != nil && len(config.CatchSteps) > 0 && config.OnError != "rethrow" {
		log.Printf("  🔄 Entering catch block...")

		// Inject error info into catch input
		catchInput := make(map[string]interface{})
		for k, v := range currentInput {
			catchInput[k] = v
		}
		catchInput["_error"] = map[string]interface{}{
			"message":   tryErrorMessage,
			"caught":    true,
			"timestamp": time.Now().Format(time.RFC3339),
		}

		for i, childStepID := range config.CatchSteps {
			childOutput, stepName, err := e.childExecutor(ctx, childStepID, catchInput)
			if err != nil {
				log.Printf("  ⚠️ Catch step %d/%d (%s) failed: %v", i+1, len(config.CatchSteps), stepName, err)
				break
			}
			if childOutput != nil {
				catchInput = childOutput
				for k, v := range childOutput {
					outputData[k] = v
				}
			}
			log.Printf("  ✅ Catch step %d/%d (%s) completed", i+1, len(config.CatchSteps), stepName)
		}
	}

	// === FINALLY BLOCK ===
	if e.childExecutor != nil && len(config.FinallySteps) > 0 {
		log.Printf("  🔄 Entering finally block...")
		finallyInput := make(map[string]interface{})
		for k, v := range currentInput {
			finallyInput[k] = v
		}
		finallyInput["_trySuccess"] = trySuccess
		if tryErrorMessage != "" {
			finallyInput["_tryError"] = tryErrorMessage
		}

		for i, childStepID := range config.FinallySteps {
			childOutput, stepName, err := e.childExecutor(ctx, childStepID, finallyInput)
			if err != nil {
				log.Printf("  ⚠️ Finally step %d/%d (%s) failed: %v", i+1, len(config.FinallySteps), stepName, err)
				continue // Finally steps keep executing even on error
			}
			if childOutput != nil {
				finallyInput = childOutput
				for k, v := range childOutput {
					outputData[k] = v
				}
			}
			log.Printf("  ✅ Finally step %d/%d (%s) completed", i+1, len(config.FinallySteps), stepName)
		}
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"success":       trySuccess,
		"error_caught":  !trySuccess,
		"error_message": tryErrorMessage,
	}
	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"try_success":    trySuccess,
		"try_steps":      len(config.TrySteps),
		"catch_steps":    len(config.CatchSteps),
		"finally_steps":  len(config.FinallySteps),
		"error_handling": config.OnError,
		"success":        true,
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	// If rethrow mode and there was an error, propagate it
	if !trySuccess && config.OnError == "rethrow" {
		return outputData, tryError
	}

	// If suppress mode or catch handled it, continue
	if trySuccess {
		log.Printf("  ✅ Try-Catch completed successfully (no errors)")
	} else {
		log.Printf("  ✅ Try-Catch completed (error caught and handled)")
	}

	return outputData, nil
}

func (e *TryCatchExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("try-catch requires config with trySteps")
	}
	return nil
}
