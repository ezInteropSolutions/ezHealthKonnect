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

// CatchAction represents a built-in action to execute when an error is caught.
// These run automatically before any catch steps, providing common error handling
// patterns without requiring separate pipeline steps.
type CatchAction struct {
	Type    string                 `json:"type"`              // Action type identifier
	Enabled bool                   `json:"enabled"`           // Whether this action is active
	Config  map[string]interface{} `json:"config,omitempty"`  // Action-specific configuration
}

type tryCatchConfig struct {
	TrySteps     []string      `json:"trySteps"`          // Step IDs to execute in try block
	CatchSteps   []string      `json:"catchSteps"`        // Step IDs to execute on error
	FinallySteps []string      `json:"finallySteps"`      // Step IDs to always execute
	OnError      string        `json:"onError"`           // "catch" (default), "suppress", "rethrow"
	CatchActions []CatchAction `json:"catchActions,omitempty"` // Built-in catch actions
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
	if !trySuccess && config.OnError != "rethrow" {
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

		// Execute built-in catch actions (before catch steps)
		e.ExecuteCatchActions(config.CatchActions, tryErrorMessage, outputData, step)

		if e.childExecutor != nil && len(config.CatchSteps) > 0 {
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

// ExecuteCatchActions runs built-in catch actions when an error is caught.
// These execute before any catch steps, providing common error handling patterns.
// Exported so pipeline service can reuse for per-step error handling.
func (e *TryCatchExecutor) ExecuteCatchActions(
	actions []CatchAction,
	errorMessage string,
	outputData map[string]interface{},
	step *models.TransformationStep,
) {
	if len(actions) == 0 {
		return
	}

	for _, action := range actions {
		if !action.Enabled {
			continue
		}

		switch action.Type {
		case "log_error":
			// Log the error with configurable severity level
			level := "error"
			if l, ok := action.Config["level"].(string); ok {
				level = l
			}
			includeStepName := true
			if inc, ok := action.Config["includeStepName"].(bool); ok {
				includeStepName = inc
			}
			stepLabel := ""
			if includeStepName && step != nil {
				stepLabel = fmt.Sprintf(" [step: %s]", step.StepName)
			}
			switch level {
			case "warn":
				log.Printf("  ⚠️ [CATCH-LOG/WARN]%s %s", stepLabel, errorMessage)
			case "info":
				log.Printf("  ℹ️ [CATCH-LOG/INFO]%s %s", stepLabel, errorMessage)
			default:
				log.Printf("  🚨 [CATCH-LOG/ERROR]%s %s", stepLabel, errorMessage)
			}

		case "set_error_flag":
			// Set a boolean flag in output so downstream steps know an error was caught
			flagName := "_errorHandled"
			if fn, ok := action.Config["flagName"].(string); ok && fn != "" {
				flagName = fn
			}
			outputData[flagName] = true
			log.Printf("  🏴 [CATCH-ACTION] Set %s = true", flagName)

		case "store_error_details":
			// Store structured error details in output for downstream consumption
			varName := "_lastError"
			if vn, ok := action.Config["variableName"].(string); ok && vn != "" {
				varName = vn
			}
			outputData[varName] = map[string]interface{}{
				"message":   errorMessage,
				"timestamp": time.Now().Format(time.RFC3339),
				"stepId":    step.ID,
				"stepName":  step.StepName,
				"handled":   true,
			}
			log.Printf("  📦 [CATCH-ACTION] Stored error details in %s", varName)

		case "increment_error_counter":
			// Increment an error counter in output (useful for monitoring/alerting thresholds)
			counterName := "_errorCount"
			if cn, ok := action.Config["counterName"].(string); ok && cn != "" {
				counterName = cn
			}
			current := 0
			if v, ok := outputData[counterName].(int); ok {
				current = v
			} else if v, ok := outputData[counterName].(float64); ok {
				current = int(v)
			}
			outputData[counterName] = current + 1
			log.Printf("  🔢 [CATCH-ACTION] %s = %d", counterName, current+1)

		case "set_default_value":
			// Set a fallback/default value for a field when the try block fails
			fieldName := ""
			if fn, ok := action.Config["fieldName"].(string); ok {
				fieldName = fn
			}
			defaultValue := action.Config["defaultValue"]
			if fieldName != "" && defaultValue != nil {
				outputData[fieldName] = defaultValue
				log.Printf("  📝 [CATCH-ACTION] Set default %s = %v", fieldName, defaultValue)
			}

		case "send_alert":
			// Trigger an alert notification on failure
			channel := "system"
			if ch, ok := action.Config["channel"].(string); ok {
				channel = ch
			}
			severity := "high"
			if sv, ok := action.Config["severity"].(string); ok {
				severity = sv
			}
			stepName := ""
			if step != nil {
				stepName = step.StepName
			}
			log.Printf("  🔔 [CATCH-ACTION] Alert [%s/%s] Step '%s' failed: %s", channel, severity, stepName, errorMessage)
			// Store alert info in output for downstream alert service to pick up
			outputData["_alert"] = map[string]interface{}{
				"channel":   channel,
				"severity":  severity,
				"stepName":  stepName,
				"message":   errorMessage,
				"timestamp": time.Now().Format(time.RFC3339),
			}

		case "send_email":
			// Email notification on failure (stores intent for async email service)
			to := ""
			if t, ok := action.Config["to"].(string); ok {
				to = t
			}
			subject := "Pipeline step failed"
			if s, ok := action.Config["subject"].(string); ok && s != "" {
				subject = s
			}
			stepName := ""
			if step != nil {
				stepName = step.StepName
			}
			log.Printf("  📧 [CATCH-ACTION] Email to=%s subject='%s' step='%s'", to, subject, stepName)
			outputData["_emailNotification"] = map[string]interface{}{
				"to":        to,
				"subject":   subject,
				"body":      fmt.Sprintf("Step '%s' failed: %s", stepName, errorMessage),
				"stepName":  stepName,
				"timestamp": time.Now().Format(time.RFC3339),
			}

		case "record_metric":
			// Record failure metric for monitoring
			metricName := "step_failures_total"
			if mn, ok := action.Config["metricName"].(string); ok && mn != "" {
				metricName = mn
			}
			stepName := ""
			if step != nil {
				stepName = step.StepName
			}
			log.Printf("  📊 [CATCH-ACTION] Metric %s{step=\"%s\"} incremented", metricName, stepName)
			// Store metric for monitoring service to consume
			if outputData["_metrics"] == nil {
				outputData["_metrics"] = []interface{}{}
			}
			if metrics, ok := outputData["_metrics"].([]interface{}); ok {
				outputData["_metrics"] = append(metrics, map[string]interface{}{
					"name":      metricName,
					"value":     1,
					"labels":    map[string]string{"step": stepName},
					"timestamp": time.Now().Format(time.RFC3339),
				})
			}

		default:
			log.Printf("  ❓ [CATCH-ACTION] Unknown action type: %s", action.Type)
		}
	}
}

func (e *TryCatchExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("try-catch requires config with trySteps")
	}
	return nil
}
