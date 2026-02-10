package control

import (
	"context"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// ===============================================================
// IF-THEN-ELSE EXECUTOR
// ===============================================================

// IfThenElseExecutor implements conditional logic with if/then/else branching
type IfThenElseExecutor struct {
	*executors.BaseExecutor
}

// NewIfThenElseExecutor creates a new if-then-else executor
func NewIfThenElseExecutor() *IfThenElseExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "If-Then-Else",
		Description: "Conditional execution with if/then/else logic. Supports cross-field comparisons, validation, routing, and metadata enrichment.",
		Version:     "2.0.0",
		Author:      "ezHealthKonnect",
		Category:    "control",
	}

	base := executors.NewBaseExecutor("if_then_else", metadata)

	return &IfThenElseExecutor{
		BaseExecutor: base,
	}
}

// Execute performs conditional logic execution
func (e *IfThenElseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	log.Printf("🔀 [IfThenElse] EXECUTING - Config type: %T, Config keys: %v", step.Config, getMapKeys(step.Config))

	// PHASE 3 FIX: Support multiple conditions processing
	// Parse configuration - support both old and new formats
	var conditionsArray []map[string]interface{}
	var legacyMode bool = false

	// Check for NEWEST format (conditions array at root level with onTrue/onFalse objects)
	log.Printf("🔀 [IfThenElse] Checking for conditions array...")
	if conditions, ok := step.Config["conditions"].([]interface{}); ok && len(conditions) > 0 {
		log.Printf("🔀 [IfThenElse] SUCCESS - Found conditions array with %d items", len(conditions))
		log.Printf("🔀 [IfThenElse] Found %d conditions to process", len(conditions))

		for i, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			// Store each condition with its actions
			conditionsArray = append(conditionsArray, condMap)

			condName := getStringValue(condMap, "name")
			if condName == "" {
				condName = fmt.Sprintf("Condition %d", i+1)
			}
			log.Printf("   [%d] %s", i+1, condName)
		}

		log.Printf("🔀 [IfThenElse] Using NEWEST format - will process ALL %d conditions", len(conditionsArray))
	}

	// Check for newer format (ifThenElse with conditions array and thenActions/elseActions)
	if len(conditionsArray) == 0 {
		if ifThenElse, ok := step.Config["ifThenElse"].(map[string]interface{}); ok {
			conditions, _ := ifThenElse["conditions"].([]interface{})
			if len(conditions) > 0 {
				for _, cond := range conditions {
					condMap, ok := cond.(map[string]interface{})
					if ok {
						conditionsArray = append(conditionsArray, condMap)
					}
				}
				log.Printf("🔀 [IfThenElse] Using new format (ifThenElse.conditions) - %d conditions", len(conditionsArray))
			}
		}
	}

	// Fallback to oldest format (flat condition at root) - single condition only
	if len(conditionsArray) == 0 {
		oldCondition, ok := step.Config["condition"].(map[string]interface{})
		if !ok {
			// Better error message with config structure debugging
			log.Printf("❌ [IfThenElse] Config validation failed. Config keys: %v", getMapKeys(step.Config))
			err := fmt.Errorf("condition is required - expected format: { conditions: [{ condition: {...}, onTrue: {...}, onFalse: {...} }] } (got config keys: %v)", getMapKeys(step.Config))
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

		// Convert to new format internally
		legacyMode = true
		thenActions, _ := step.Config["then_actions"].([]interface{})
		elseActions, _ := step.Config["else_actions"].([]interface{})

		conditionsArray = append(conditionsArray, map[string]interface{}{
			"name":      "Legacy Condition",
			"condition": oldCondition,
			"onTrue":    thenActions,
			"onFalse":   elseActions,
		})

		log.Printf("🔀 [IfThenElse] Using OLDEST format (flat condition/then_actions/else_actions) - converted to new format")
	}

	// PHASE 1 FIX: Don't copy ALL input data (prevents accumulation)
	// Only keep the original message and pipeline metadata
	outputData := make(map[string]interface{})

	// Preserve original message (needed for condition evaluation)
	if message, ok := inputData["message"]; ok {
		outputData["message"] = message
	}

	// Preserve routing directives (needed for pipeline control)
	if routing, ok := inputData["_routing"]; ok {
		outputData["_routing"] = routing
	}

	// Preserve metadata (needed for pipeline tracking)
	if metadata, ok := inputData["_metadata"]; ok {
		outputData["_metadata"] = metadata
	}

	log.Printf("🔀 [IfThenElse] Input keys: %v → Output will have isolated step data", getMapKeys(inputData))

	// Determine evaluation data (message vs full data)
	evaluationData := outputData
	if message, ok := outputData["message"].(map[string]interface{}); ok {
		evaluationData = message
		log.Printf("🔀 [IfThenElse] Evaluating conditions against message data")
		log.Printf("🔀 [IfThenElse] Message keys: %v", getMapKeys(message))
	} else {
		log.Printf("🔀 [IfThenElse] Evaluating conditions against full data (legacy mode)")
		log.Printf("🔀 [IfThenElse] outputData keys: %v", getMapKeys(outputData))
		// Also try inputData directly if outputData["message"] doesn't exist
		if _, hasES := inputData["enhancedSegments"]; hasES {
			evaluationData = inputData
			log.Printf("🔀 [IfThenElse] Using inputData directly (has enhancedSegments)")
		}
	}

	// PHASE 3 FIX: Process ALL conditions sequentially
	conditionResults := make([]map[string]interface{}, 0)
	totalActionsExecuted := 0
	var finalRoutingAction map[string]interface{}
	overallBranchTaken := "false" // Default to false, set to "true" if any condition evaluates to true

	for condIdx, condMap := range conditionsArray {
		condName := getStringValue(condMap, "name")
		if condName == "" {
			condName = fmt.Sprintf("Condition %d", condIdx+1)
		}

		log.Printf("\n🔀 [IfThenElse] Processing: %s", condName)

		// Extract condition object
		var condition map[string]interface{}
		if legacyMode {
			condition = condMap["condition"].(map[string]interface{})
		} else {
			condition, _ = condMap["condition"].(map[string]interface{})
		}

		// Evaluate condition
		conditionMet, err := e.evaluateCondition(condition, evaluationData)
		if err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return outputData, fmt.Errorf("condition evaluation failed for %s: %v", condName, err)
		}

		log.Printf("   ✓ Evaluated: %v", conditionMet)

		// Determine which actions to execute
		var actionsToExecute []interface{}
		var branchTaken string

		if conditionMet {
			branchTaken = "then"
			overallBranchTaken = "true" // At least one condition was true
			// Handle both single object and array formats
			if onTrue, ok := condMap["onTrue"].(map[string]interface{}); ok {
				actionsToExecute = []interface{}{onTrue}
			} else if onTrue, ok := condMap["onTrue"].([]interface{}); ok {
				actionsToExecute = onTrue
			}
			log.Printf("   → THEN branch: %d actions", len(actionsToExecute))
		} else {
			branchTaken = "else"
			// Handle both single object and array formats
			if onFalse, ok := condMap["onFalse"].(map[string]interface{}); ok {
				actionsToExecute = []interface{}{onFalse}
			} else if onFalse, ok := condMap["onFalse"].([]interface{}); ok {
				actionsToExecute = onFalse
			}
			log.Printf("   → ELSE branch: %d actions", len(actionsToExecute))
		}

		// Execute actions for this condition
		actionsExecuted := 0
		for i, action := range actionsToExecute {
			actionMap, ok := action.(map[string]interface{})
			if !ok {
				continue
			}

			actionType := getStringValue(actionMap, "action")
			fmt.Printf("🔀🔀🔀 [IfThenElse] Action %d: %s (config: %+v)\n", i+1, actionType, actionMap)

			// Check if this is a routing action
			if actionType == "route_to_step" || actionType == "route_to" {
				// Save routing action for LAST (after all conditions processed)
				finalRoutingAction = actionMap
				fmt.Printf("🔀🔀🔀 [IfThenElse] ROUTING ACTION - stepId=%v, skipSteps=%v\n",
					actionMap["stepId"], actionMap["skipSteps"])
			} else {
				// Execute non-routing actions immediately
				if err := e.executeAction(actionMap, outputData); err != nil {
					e.PostExecute(ctx, step, err, time.Since(start))
					return outputData, fmt.Errorf("action execution failed for %s: %v", condName, err)
				}
				actionsExecuted++
			}
		}

		totalActionsExecuted += actionsExecuted

		// Track condition result
		conditionResults = append(conditionResults, map[string]interface{}{
			"name":            condName,
			"evaluated":       conditionMet,
			"branch":          branchTaken,
			"actions_executed": actionsExecuted,
		})

		log.Printf("   ✅ Completed: %d actions executed", actionsExecuted)
	}

	// PHASE 3 FIX: Execute routing action LAST (after all conditions processed)
	if finalRoutingAction != nil {
		actionType := getStringValue(finalRoutingAction, "action")
		fmt.Printf("\n🔀🔀🔀 [IfThenElse] EXECUTING FINAL ROUTING: %s\n", actionType)
		fmt.Printf("🔀🔀🔀 [IfThenElse] finalRoutingAction = %+v\n", finalRoutingAction)

		if err := e.executeAction(finalRoutingAction, outputData); err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return outputData, fmt.Errorf("routing action execution failed: %v", err)
		}

		// DEBUG: Verify routing was set
		if routing, ok := outputData["_routing"].(map[string]interface{}); ok {
			fmt.Printf("🔀🔀🔀 [IfThenElse] ROUTING SET: nextStep=%v, skipSteps=%v\n",
				routing["nextStep"], routing["skipSteps"])
		} else {
			fmt.Printf("🔀🔀🔀 [IfThenElse] WARNING: _routing NOT FOUND after executeAction!\n")
			fmt.Printf("🔀🔀🔀 [IfThenElse] OutputData keys: %v\n", getMapKeys(outputData))
		}
	} else {
		fmt.Printf("🔀🔀🔀 [IfThenElse] NO ROUTING ACTION to execute\n")
	}

	// STANDARDIZED: Variables (empty for if-then-else, it's routing logic) + execution details
	outputData["_stepOutput"] = map[string]interface{}{}
	outputData["_executionDetails"] = map[string]interface{}{
		"conditions_processed":   len(conditionsArray),
		"conditions_results":     conditionResults,
		"total_actions_executed": totalActionsExecuted,
		"routing_applied":        finalRoutingAction != nil,
		"branch_taken":           overallBranchTaken,
	}

	// CRITICAL: Set branch routing for automatic branch exclusion
	// This tells the pipeline service which branch was taken so it can skip the other branch's steps
	if _, exists := outputData["_routing"]; !exists {
		outputData["_routing"] = make(map[string]interface{})
	}
	routingMap := outputData["_routing"].(map[string]interface{})
	routingMap["branchTaken"] = overallBranchTaken
	log.Printf("🔀 [IfThenElse] Branch taken: %s (will auto-skip %s branch steps)", overallBranchTaken, map[string]string{"true": "false", "false": "true"}[overallBranchTaken])

	log.Printf("\n✅ [IfThenElse] Completed: %d conditions, %d actions, routing=%v",
		len(conditionsArray), totalActionsExecuted, finalRoutingAction != nil)

	// Post-execution tracking
	e.PostExecute(ctx, step, nil, time.Since(start))
	return outputData, nil
}

// evaluateCondition evaluates a condition against data
func (e *IfThenElseExecutor) evaluateCondition(condition map[string]interface{}, data map[string]interface{}) (bool, error) {
	field := getStringValue(condition, "field")
	operator := getStringValue(condition, "operator")

	// Get first field value - use shared HL7-aware resolver from base_executor
	// This handles paths like "MSH.9", "PID.5.1" automatically
	fieldValue := executors.GetNestedValue(data, field)

	// Determine comparison value (constant or field)
	var compareValue interface{}
	compareToField := getStringValue(condition, "compareToField")
	if compareToField != "" {
		// Cross-field comparison - use shared HL7-aware resolver
		compareValue = executors.GetNestedValue(data, compareToField)
		log.Printf("   Comparing: %s (%v) %s %s (%v)", field, fieldValue, operator, compareToField, compareValue)
	} else {
		// Constant comparison
		compareValue = condition["value"]
		log.Printf("   Comparing: %s (%v) %s %v", field, fieldValue, operator, compareValue)
	}

	// Perform comparison
	switch operator {
	case "equals":
		return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", compareValue), nil

	case "not_equals":
		return fmt.Sprintf("%v", fieldValue) != fmt.Sprintf("%v", compareValue), nil

	case "contains":
		fieldStr := fmt.Sprintf("%v", fieldValue)
		valueStr := fmt.Sprintf("%v", compareValue)
		return strings.Contains(fieldStr, valueStr), nil

	case "starts_with":
		fieldStr := fmt.Sprintf("%v", fieldValue)
		valueStr := fmt.Sprintf("%v", compareValue)
		return strings.HasPrefix(fieldStr, valueStr), nil

	case "ends_with":
		fieldStr := fmt.Sprintf("%v", fieldValue)
		valueStr := fmt.Sprintf("%v", compareValue)
		return strings.HasSuffix(fieldStr, valueStr), nil

	case "greater_than":
		fieldNum := toFloat64(fieldValue)
		valueNum := toFloat64(compareValue)
		return fieldNum > valueNum, nil

	case "greater_than_or_equal":
		fieldNum := toFloat64(fieldValue)
		valueNum := toFloat64(compareValue)
		return fieldNum >= valueNum, nil

	case "less_than":
		fieldNum := toFloat64(fieldValue)
		valueNum := toFloat64(compareValue)
		return fieldNum < valueNum, nil

	case "less_than_or_equal":
		fieldNum := toFloat64(fieldValue)
		valueNum := toFloat64(compareValue)
		return fieldNum <= valueNum, nil

	case "exists":
		return fieldValue != nil, nil

	case "not_exists":
		return fieldValue == nil, nil

	case "regex_match":
		fieldStr := fmt.Sprintf("%v", fieldValue)
		pattern := fmt.Sprintf("%v", compareValue)
		matched, err := regexp.MatchString(pattern, fieldStr)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %v", err)
		}
		return matched, nil

	case "in_list":
		valueList, ok := compareValue.([]interface{})
		if !ok {
			return false, fmt.Errorf("value must be a list for 'in_list' operator")
		}
		fieldStr := fmt.Sprintf("%v", fieldValue)
		for _, item := range valueList {
			if fmt.Sprintf("%v", item) == fieldStr {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// executeAction executes a single action
func (e *IfThenElseExecutor) executeAction(actionMap map[string]interface{}, outputData map[string]interface{}) error {
	actionType := getStringValue(actionMap, "action")

	switch actionType {
	case "continue":
		// No-op, just continue to next step
		return nil

	case "reject":
		errorMessage := getStringValue(actionMap, "errorMessage")
		if errorMessage == "" {
			errorMessage = "Condition validation failed"
		}
		severity := getStringValue(actionMap, "severity")
		if severity == "" {
			severity = "error"
		}
		log.Printf("   ❌ REJECT: %s (severity: %s)", errorMessage, severity)
		return fmt.Errorf("REJECT: %s", errorMessage)

	case "log_warning":
		message := getStringValue(actionMap, "message")
		log.Printf("   ⚠️  WARNING: %s", message)
		continueProcessing := getBoolValue(actionMap, "continue", true)
		if !continueProcessing {
			return fmt.Errorf("Processing stopped after warning: %s", message)
		}

	case "log_error":
		message := getStringValue(actionMap, "message")
		log.Printf("   ❌ ERROR: %s", message)
		continueProcessing := getBoolValue(actionMap, "continue", false)
		if !continueProcessing {
			return fmt.Errorf("Processing stopped after error: %s", message)
		}

	case "set_metadata":
		metadata, ok := actionMap["metadata"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("metadata must be an object")
		}

		// Ensure _metadata exists
		if _, exists := outputData["_metadata"]; !exists {
			outputData["_metadata"] = make(map[string]interface{})
		}

		metadataMap, ok := outputData["_metadata"].(map[string]interface{})
		if !ok {
			outputData["_metadata"] = make(map[string]interface{})
			metadataMap = outputData["_metadata"].(map[string]interface{})
		}

		for key, value := range metadata {
			metadataMap[key] = value
			log.Printf("      Set _metadata.%s = %v", key, value)
		}

	case "route_to":
		destination := getStringValue(actionMap, "destination")
		queue := getStringValue(actionMap, "queue")
		nextStep := getStringValue(actionMap, "nextStep") // Support conditional routing

		// Also support "stepId" from UI (backward compatibility)
		if nextStep == "" {
			nextStep = getStringValue(actionMap, "stepId")
		}

		// Ensure _routing exists
		if _, exists := outputData["_routing"]; !exists {
			outputData["_routing"] = make(map[string]interface{})
		}

		routingMap, ok := outputData["_routing"].(map[string]interface{})
		if !ok {
			outputData["_routing"] = make(map[string]interface{})
			routingMap = outputData["_routing"].(map[string]interface{})
		}

		if destination != "" {
			routingMap["destination"] = destination
			log.Printf("      Set _routing.destination = %s", destination)
		}
		if queue != "" {
			routingMap["queue"] = queue
			log.Printf("      Set _routing.queue = %s", queue)
		}
		if nextStep != "" {
			routingMap["nextStep"] = nextStep
			log.Printf("      Set _routing.nextStep = %s (conditional jump)", nextStep)
		}

	case "set_field":
		field := getStringValue(actionMap, "field")
		value := actionMap["value"]
		setNestedValue(outputData, field, value)
		log.Printf("      Set %s = %v", field, value)

	case "set_value": // Alias for set_field
		field := getStringValue(actionMap, "field")
		value := actionMap["value"]
		setNestedValue(outputData, field, value)
		log.Printf("      Set %s = %v", field, value)

	case "copy_field":
		sourceField := getStringValue(actionMap, "source")
		targetField := getStringValue(actionMap, "target")
		value := getNestedValue(outputData, sourceField)
		if value != nil {
			setNestedValue(outputData, targetField, value)
			log.Printf("      Copied %s → %s (%v)", sourceField, targetField, value)
		}

	case "delete_field":
		field := getStringValue(actionMap, "field")
		e.deleteField(outputData, field)
		log.Printf("      Deleted %s", field)

	case "route_to_step":
		// Support both single step (stepId/targetStepId) and multiple steps (targetStepIds)
		stepId := getStringValue(actionMap, "stepId")
		if stepId == "" {
			stepId = getStringValue(actionMap, "targetStepId")
		}

		// Check for multiple target steps (array)
		var targetStepIds []string
		if ids, ok := actionMap["targetStepIds"].([]interface{}); ok && len(ids) > 0 {
			for _, id := range ids {
				if sid, ok := id.(string); ok && sid != "" {
					targetStepIds = append(targetStepIds, sid)
				}
			}
		}

		// Validate: need either single stepId or multiple targetStepIds
		if stepId == "" && len(targetStepIds) == 0 {
			return fmt.Errorf("stepId or targetStepIds is required for route_to_step action")
		}

		// Ensure _routing exists
		if _, exists := outputData["_routing"]; !exists {
			outputData["_routing"] = make(map[string]interface{})
		}

		routingMap, ok := outputData["_routing"].(map[string]interface{})
		if !ok {
			outputData["_routing"] = make(map[string]interface{})
			routingMap = outputData["_routing"].(map[string]interface{})
		}

		// Set routing: multiple steps take priority over single step
		if len(targetStepIds) > 0 {
			routingMap["nextSteps"] = targetStepIds
			log.Printf("      🔀 Set _routing.nextSteps = %v (multi-step routing)", targetStepIds)
		} else {
			routingMap["nextStep"] = stepId
			log.Printf("      🔀 Set _routing.nextStep = %s (conditional routing)", stepId)
		}

		// Support for exclusive branches - skipSteps lists step IDs to skip after routing
		// This allows true/false branches to be mutually exclusive
		if skipSteps, ok := actionMap["skipSteps"].([]interface{}); ok && len(skipSteps) > 0 {
			skipStepIDs := make([]string, 0, len(skipSteps))
			for _, s := range skipSteps {
				if sid, ok := s.(string); ok {
					skipStepIDs = append(skipStepIDs, sid)
				}
			}
			routingMap["skipSteps"] = skipStepIDs
			log.Printf("      🔀 Set _routing.skipSteps = %v (exclusive branch)", skipStepIDs)
		}

	case "log_info":
		message := getStringValue(actionMap, "message")
		log.Printf("   ℹ️  INFO: %s", message)
		// Always continue

	case "log_debug":
		message := getStringValue(actionMap, "message")
		log.Printf("   🔍 DEBUG: %s", message)
		// Always continue

	default:
		return fmt.Errorf("unsupported action: %s", actionType)
	}

	return nil
}

// deleteField deletes a nested field from data
func (e *IfThenElseExecutor) deleteField(data map[string]interface{}, path string) {
	keys := strings.Split(path, ".")
	if len(keys) == 1 {
		delete(data, keys[0])
		return
	}

	current := data
	for i := 0; i < len(keys)-1; i++ {
		next, ok := current[keys[i]].(map[string]interface{})
		if !ok {
			return
		}
		current = next
	}

	delete(current, keys[len(keys)-1])
}

// Validate validates the step configuration
func (e *IfThenElseExecutor) Validate(step *models.TransformationStep) error {
	// Support multiple formats: conditions array (new), ifThenElse.conditions (newer), or condition (legacy)

	// Check for NEWEST format: conditions array
	if conditions, ok := step.Config["conditions"].([]interface{}); ok && len(conditions) > 0 {
		// Validate each condition in the array
		for i, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				return fmt.Errorf("condition %d must be an object", i+1)
			}

			condition, ok := condMap["condition"].(map[string]interface{})
			if !ok {
				return fmt.Errorf("condition %d is missing 'condition' field", i+1)
			}

			field := getStringValue(condition, "field")
			operator := getStringValue(condition, "operator")
			if field == "" || operator == "" {
				return fmt.Errorf("condition %d must have field and operator", i+1)
			}
		}
		return nil
	}

	// Check for newer format: ifThenElse.conditions
	if ifThenElse, ok := step.Config["ifThenElse"].(map[string]interface{}); ok {
		if conditions, ok := ifThenElse["conditions"].([]interface{}); ok && len(conditions) > 0 {
			return nil // Basic validation passed
		}
	}

	// Fallback to legacy format: single condition object
	condition, ok := step.Config["condition"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("condition is required")
	}

	field := getStringValue(condition, "field")
	operator := getStringValue(condition, "operator")
	if field == "" || operator == "" {
		return fmt.Errorf("condition must have field and operator")
	}

	return nil
}

// ===============================================================
// SWITCH-CASE EXECUTOR
// ===============================================================

// SwitchCaseExecutor implements switch-case logic for multi-way branching
type SwitchCaseExecutor struct {
	*executors.BaseExecutor
}

// NewSwitchCaseExecutor creates a new switch-case executor
func NewSwitchCaseExecutor() *SwitchCaseExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Switch/Case",
		Description: "Multiple condition branching with switch/case logic",
		Version:     "2.0.0",
		Author:      "ezHealthKonnect",
		Category:    "control",
	}

	base := executors.NewBaseExecutor("switch_case", metadata)

	return &SwitchCaseExecutor{
		BaseExecutor: base,
	}
}

// Execute performs switch-case logic execution
func (e *SwitchCaseExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	fmt.Printf("🔀 [SwitchCase] Full step config: %+v\n", step.Config)

	field := getStringValue(step.Config, "field")
	cases, ok := step.Config["cases"].([]interface{})
	if !ok {
		fmt.Printf("🔀 [SwitchCase] ERROR: cases is not []interface{}, type: %T\n", step.Config["cases"])
		err := fmt.Errorf("invalid cases configuration")
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	fmt.Printf("🔀 [SwitchCase] Found %d cases, field='%s'\n", len(cases), field)

	// Handle default actions - support both formats:
	// 1. Array format: default: [{ action: 'continue' }]
	// 2. Object format: default: { actions: [{ action: 'continue' }] }
	var defaultActions []interface{}
	if defaultConfig, ok := step.Config["default"].(map[string]interface{}); ok {
		// Object format with 'actions' property
		if actions, ok := defaultConfig["actions"].([]interface{}); ok {
			defaultActions = actions
		}
	} else if actions, ok := step.Config["default"].([]interface{}); ok {
		// Direct array format
		defaultActions = actions
	}

	// Get comparison options
	caseInsensitive := false
	trimWhitespace := true // Default to true for cleaner matching
	if options, ok := step.Config["options"].(map[string]interface{}); ok {
		if ci, ok := options["caseInsensitive"].(bool); ok {
			caseInsensitive = ci
		}
		if tw, ok := options["trimWhitespace"].(bool); ok {
			trimWhitespace = tw
		}
	}

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	fieldValue := getNestedValue(outputData, field)
	fieldStr := fmt.Sprintf("%v", fieldValue)

	// Apply options to field value
	if trimWhitespace {
		fieldStr = strings.TrimSpace(fieldStr)
	}
	if caseInsensitive {
		fieldStr = strings.ToLower(fieldStr)
	}

	fmt.Printf("🔀 [SwitchCase] Evaluating %s = '%s' (caseInsensitive=%v, trimWhitespace=%v)\n", field, fieldStr, caseInsensitive, trimWhitespace)

	// Find matching case
	var matchedActions []interface{}
	matched := false
	matchedCaseValue := "" // Track which case was matched for branch resolution

	fmt.Printf("🔀 [SwitchCase] Checking %d cases for match...\n", len(cases))
	for i, caseItem := range cases {
		caseMap := caseItem.(map[string]interface{})
		caseValue := fmt.Sprintf("%v", caseMap["value"])
		originalCaseValue := caseValue // Keep original for branch tracking

		fmt.Printf("   [Case %d] Raw value from config: '%s' (type: %T)\n", i+1, caseValue, caseMap["value"])

		// Apply same options to case value for fair comparison
		if trimWhitespace {
			caseValue = strings.TrimSpace(caseValue)
		}
		if caseInsensitive {
			caseValue = strings.ToLower(caseValue)
		}

		fmt.Printf("   [Case %d] Comparing fieldStr='%s' vs caseValue='%s'\n", i+1, fieldStr, caseValue)

		if fieldStr == caseValue {
			matchedActions = caseMap["actions"].([]interface{})
			matched = true
			matchedCaseValue = originalCaseValue // Use original (not normalized) for routing
			fmt.Printf("   ✅ Matched case %d: '%s'\n", i+1, originalCaseValue)
			break
		} else {
			fmt.Printf("   ❌ No match for case %d\n", i+1)
		}
	}

	// Use default if no match
	if !matched {
		matchedActions = defaultActions
		matchedCaseValue = "default"
		log.Printf("   No match, using default actions")
	}

	// Track variables set during execution for step_output
	variablesSet := make(map[string]interface{})

	// Track source paths for each variable (for UI tooltip display)
	// Maps variable key to its source field path (e.g., "PID.13.4" -> "enhancedSegments.PID.fields.12.value")
	variablePathMap := make(map[string]string)

	// Execute actions
	for i, action := range matchedActions {
		actionMap := action.(map[string]interface{})
		actionType := getStringValue(actionMap, "action")

		log.Printf("   [%d] Action: %s", i+1, actionType)

		switch actionType {
		case "continue":
			// No-op, just continue to next step
			log.Printf("      → Continue processing")

		case "set_value":
			// Support both "field" and "targetField" (UI uses targetField)
			targetField := getStringValue(actionMap, "targetField")
			if targetField == "" {
				targetField = getStringValue(actionMap, "field") // Fallback
			}
			value := actionMap["value"]
			setNestedValue(outputData, targetField, value)
			variablesSet[targetField] = value // Track for step_output
			log.Printf("      Set %s = %v", targetField, value)

		case "copy_field":
			// Support both UI naming (sourceField/targetField) and shorthand (source/target)
			sourceField := getStringValue(actionMap, "sourceField")
			if sourceField == "" {
				sourceField = getStringValue(actionMap, "source")
			}
			targetField := getStringValue(actionMap, "targetField")
			if targetField == "" {
				targetField = getStringValue(actionMap, "target")
			}
			value := getNestedValue(outputData, sourceField)
			if value != nil {
				setNestedValue(outputData, targetField, value)
				variablesSet[targetField] = value // Track for step_output
				// Track source path for UI tooltip (shows absolute JSON path on hover)
				variablePathMap[targetField] = getAbsoluteJSONPath(sourceField)
				log.Printf("      Copied %s → %s", sourceField, targetField)
			}

		case "transform":
			// Transform action: sourceField → transformation → targetField
			// Uses shared transformation utilities (OOP - DRY principle)
			fmt.Printf("      🔄 Transform action raw config: %+v\n", actionMap)

			sourceField := getStringValue(actionMap, "sourceField")
			targetField := getStringValue(actionMap, "targetField")
			transformation := getStringValue(actionMap, "transformation")

			// Default to uppercase if transformation not specified (backward compatibility)
			if transformation == "" {
				transformation = "uppercase"
				fmt.Printf("      🔄 Transform: defaulting to 'uppercase' transformation\n")
			}

			fmt.Printf("      🔄 Transform action parsed: sourceField='%s', targetField='%s', transformation='%s'\n", sourceField, targetField, transformation)

			// Fallback to old "field" key for backward compatibility
			if sourceField == "" {
				sourceField = getStringValue(actionMap, "field")
			}

			if targetField == "" {
				fmt.Printf("      ⚠️  Transform: targetField is empty!\n")
				continue
			}

			// Get value from outputData (try outputData first, then message)
			value := getNestedValue(outputData, sourceField)
			if value != nil {
				// Use shared transformation utility (supports: upper, lower, trim, capitalize, regex:, substring:, replace:)
				transformedValue := executors.TransformValue(value, transformation)

				// Store in output data for downstream steps
				setNestedValue(outputData, targetField, transformedValue)
				variablesSet[targetField] = transformedValue

				// Update the original message field (format-agnostic)
				// This ensures the transformed value is visible in the message for downstream steps
				// Uses shared field utilities that support HL7, FHIR, and JSON paths
				if executors.UpdateFieldValue(outputData, targetField, transformedValue) {
					fmt.Printf("      ✅ Updated original message field %s = %v\n", targetField, transformedValue)
				}

				// Track source path for UI tooltip (shows absolute JSON path on hover)
				variablePathMap[targetField] = executors.GetAbsolutePath(sourceField)
				fmt.Printf("      ✅ Transformed %s → %s with %s = %v\n", sourceField, targetField, transformation, transformedValue)
			} else {
				fmt.Printf("      ⚠️  Transform: source field %s not found in data\n", sourceField)
			}

		case "route_to_step":
			// Support both single step (stepId/targetStepId) and multiple steps (targetStepIds)
			stepId := getStringValue(actionMap, "stepId")
			if stepId == "" {
				stepId = getStringValue(actionMap, "targetStepId")
			}

			// Check for multiple target steps (array)
			var targetStepIds []string
			if ids, ok := actionMap["targetStepIds"].([]interface{}); ok && len(ids) > 0 {
				for _, id := range ids {
					if sid, ok := id.(string); ok && sid != "" {
						targetStepIds = append(targetStepIds, sid)
					}
				}
			}

			// Validate: need either single stepId or multiple targetStepIds
			if stepId == "" && len(targetStepIds) == 0 {
				log.Printf("      ⚠️  route_to_step missing stepId or targetStepIds")
				continue
			}

			// Ensure _routing exists
			if _, exists := outputData["_routing"]; !exists {
				outputData["_routing"] = make(map[string]interface{})
			}
			routingMap := outputData["_routing"].(map[string]interface{})

			// Set routing: multiple steps take priority over single step
			if len(targetStepIds) > 0 {
				routingMap["nextSteps"] = targetStepIds
				log.Printf("      🔀 Set _routing.nextSteps = %v (multi-step routing)", targetStepIds)
			} else {
				routingMap["nextStep"] = stepId
				log.Printf("      🔀 Set _routing.nextStep = %s", stepId)
			}

			// Support for skipSteps
			if skipSteps, ok := actionMap["skipSteps"].([]interface{}); ok && len(skipSteps) > 0 {
				skipStepIDs := make([]string, 0, len(skipSteps))
				for _, s := range skipSteps {
					if sid, ok := s.(string); ok {
						skipStepIDs = append(skipStepIDs, sid)
					}
				}
				routingMap["skipSteps"] = skipStepIDs
				log.Printf("      🔀 Set _routing.skipSteps = %v", skipStepIDs)
			}

		case "reject":
			errorMessage := getStringValue(actionMap, "errorMessage")
			if errorMessage == "" {
				errorMessage = "Switch/Case rejection"
			}
			log.Printf("      ❌ REJECT: %s", errorMessage)
			err := fmt.Errorf("REJECT: %s", errorMessage)
			e.PostExecute(ctx, step, err, time.Since(start))
			return outputData, err

		case "log_warning":
			message := getStringValue(actionMap, "message")
			log.Printf("      ⚠️  WARNING: %s", message)

		case "log_error":
			message := getStringValue(actionMap, "message")
			log.Printf("      ❌ ERROR: %s", message)

		case "set_metadata":
			metadata, ok := actionMap["metadata"].(map[string]interface{})
			if !ok {
				continue
			}
			// Ensure _metadata exists
			if _, exists := outputData["_metadata"]; !exists {
				outputData["_metadata"] = make(map[string]interface{})
			}
			metadataMap := outputData["_metadata"].(map[string]interface{})
			for key, value := range metadata {
				metadataMap[key] = value
				variablesSet["_metadata."+key] = value // Track for step_output
				log.Printf("      Set _metadata.%s = %v", key, value)
			}

		default:
			log.Printf("      ⚠️  Unsupported action: %s (skipping)", actionType)
		}
	}

	// step_output contains ONLY the variables/data produced by the step
	// Execution details go in step_metadata
	outputData["_stepOutput"] = variablesSet

	// Add execution details to metadata (will be merged by PostExecute)
	executionDetails := map[string]interface{}{
		"field_evaluated":    field,
		"field_value":        fieldStr,
		"case_matched":       matched,
		"case_matched_value": matchedCaseValue,
		"actions_executed":   len(matchedActions),
	}

	// Include variable path mapping for UI tooltip display
	// This allows the UI to show absolute JSON paths when hovering over output keys
	if len(variablePathMap) > 0 {
		executionDetails["variable_source_paths"] = variablePathMap
	}

	outputData["_executionDetails"] = executionDetails

	// CRITICAL: Set case routing for automatic branch exclusion
	// This tells the pipeline service which case was matched so it can skip other cases' steps
	if _, exists := outputData["_routing"]; !exists {
		outputData["_routing"] = make(map[string]interface{})
	}
	routingMap := outputData["_routing"].(map[string]interface{})
	routingMap["caseMatched"] = matchedCaseValue
	log.Printf("🔀 [SwitchCase] Case matched: '%s' (will auto-skip steps from other cases)", matchedCaseValue)

	e.PostExecute(ctx, step, nil, time.Since(start))
	return outputData, nil
}

// Validate validates the step configuration
func (e *SwitchCaseExecutor) Validate(step *models.TransformationStep) error {
	field := getStringValue(step.Config, "field")
	if field == "" {
		return fmt.Errorf("field is required")
	}

	cases, ok := step.Config["cases"].([]interface{})
	if !ok || len(cases) == 0 {
		return fmt.Errorf("at least one case is required")
	}

	return nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// getStringValue safely gets a string value from a map
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// getBoolValue safely gets a boolean value from a map with default
func getBoolValue(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// getNestedValue retrieves a nested value using dot notation
// Supports HL7 field notation (e.g., "MSH.9" resolves to enhancedSegments.MSH.fields[position=9].value)
func getNestedValue(data map[string]interface{}, path string) interface{} {
	// Check for HL7 field notation (SEGMENT.FIELD_NUMBER format)
	// Examples: MSH.9, PID.5, PV1.3
	if isHL7FieldPath(path) {
		value := resolveHL7Field(data, path)
		if value != nil {
			return value
		}
		// Fall through to standard resolution if HL7 resolution fails
	}

	keys := strings.Split(path, ".")
	var current interface{} = data

	for _, key := range keys {
		if currentMap, ok := current.(map[string]interface{}); ok {
			current = currentMap[key]
		} else {
			return nil
		}
	}

	return current
}

// isHL7FieldPath checks if the path looks like HL7 field notation (SEGMENT.FIELD)
// Pattern: 2-3 uppercase letters followed by a dot and a number
func isHL7FieldPath(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return false
	}

	segment := parts[0]
	fieldNum := parts[1]

	// Segment should be 2-3 uppercase letters (MSH, PID, PV1, etc.)
	if len(segment) < 2 || len(segment) > 3 {
		return false
	}
	for _, c := range segment {
		if c < 'A' || c > 'Z' {
			return false
		}
	}

	// Field number should be numeric
	for _, c := range fieldNum {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// resolveHL7Field resolves HL7 field notation to actual value
// Handles paths like "MSH.9", "PID.5.1" (with subfields)
// Supports both typed Go structs (hl7.EnhancedSegment) and map[string]interface{}
func resolveHL7Field(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return nil
	}

	segmentKey := parts[0]
	fieldPosition := 0
	fmt.Sscanf(parts[1], "%d", &fieldPosition)

	log.Printf("   🔍 [HL7 Resolver] Looking for %s field %d in data (data keys: %v)", segmentKey, fieldPosition, getMapKeys(data))

	// First, try to find enhancedSegments in the data
	var enhancedSegments interface{}
	if es, ok := data["enhancedSegments"]; ok {
		enhancedSegments = es
		log.Printf("   🔍 [HL7 Resolver] Found enhancedSegments directly in data, type: %T", es)
	} else if msg, ok := data["message"].(map[string]interface{}); ok {
		enhancedSegments = msg["enhancedSegments"]
		log.Printf("   🔍 [HL7 Resolver] Found enhancedSegments via data.message, type: %T", enhancedSegments)
	}

	if enhancedSegments == nil {
		log.Printf("   ❌ [HL7 Resolver] enhancedSegments not found in data (tried direct and via message)")
		return nil
	}

	// Handle typed Go struct (map[string]hl7.EnhancedSegment)
	if typedSegments, ok := enhancedSegments.(map[string]hl7.EnhancedSegment); ok {
		log.Printf("   🔍 [HL7 Resolver] Using typed EnhancedSegment struct")
		segment, exists := typedSegments[segmentKey]
		if !exists {
			log.Printf("   ❌ [HL7 Resolver] Segment %s not found in typed segments", segmentKey)
			return nil
		}

		// Search through Fields array
		for _, field := range segment.Fields {
			if field.Position == fieldPosition {
				log.Printf("   ✅ [HL7 Resolver] Found %s.%d = %v (typed)", segmentKey, fieldPosition, field.Value)

				// If there are more path parts, resolve subfields
				if len(parts) > 2 {
					return resolveTypedSubfield(field, parts[2:])
				}
				return field.Value
			}
		}
		log.Printf("   ❌ [HL7 Resolver] Field %s.%d not found in typed segment", segmentKey, fieldPosition)
		return nil
	}

	// Handle map[string]interface{} (from JSON deserialization)
	switch es := enhancedSegments.(type) {
	case map[string]interface{}:
		segment := es[segmentKey]
		if segment == nil {
			log.Printf("   ❌ [HL7 Resolver] Segment %s not found", segmentKey)
			return nil
		}

		// Get the fields array from the segment
		var fields interface{}
		switch seg := segment.(type) {
		case map[string]interface{}:
			fields = seg["fields"]
		default:
			log.Printf("   ❌ [HL7 Resolver] Segment %s is not a map, type: %T", segmentKey, segment)
			return nil
		}

		if fields == nil {
			log.Printf("   ❌ [HL7 Resolver] Fields not found in segment %s", segmentKey)
			return nil
		}

		// Search through fields array to find the one with matching position
		switch fieldsList := fields.(type) {
		case []interface{}:
			for _, f := range fieldsList {
				if fieldMap, ok := f.(map[string]interface{}); ok {
					pos, _ := fieldMap["position"].(float64)
					if int(pos) == fieldPosition {
						value := fieldMap["value"]
						log.Printf("   ✅ [HL7 Resolver] Found %s.%d = %v", segmentKey, fieldPosition, value)

						// If there are more path parts, resolve subfields
						if len(parts) > 2 {
							return resolveSubfield(fieldMap, parts[2:])
						}
						return value
					}
				}
			}
		default:
			log.Printf("   ❌ [HL7 Resolver] Fields is not an array, type: %T", fields)
		}
	default:
		log.Printf("   ❌ [HL7 Resolver] enhancedSegments is not a recognized type: %T", enhancedSegments)
	}

	log.Printf("   ❌ [HL7 Resolver] Field %s.%d not found", segmentKey, fieldPosition)
	return nil
}

// resolveTypedSubfield resolves subfield from a typed FieldInfo struct
func resolveTypedSubfield(field hl7.FieldInfo, subParts []string) interface{} {
	if len(subParts) == 0 {
		return field.Value
	}

	if len(field.Subfields) == 0 {
		return field.Value
	}

	subPosition := 0
	fmt.Sscanf(subParts[0], "%d", &subPosition)

	for _, sf := range field.Subfields {
		// Subfield key format is like "MSH.9.1"
		keyParts := strings.Split(sf.Key, ".")
		if len(keyParts) > 0 {
			lastPart := keyParts[len(keyParts)-1]
			if lastPart == subParts[0] {
				return sf.Value
			}
		}
	}

	// Fallback to parent field value
	return field.Value
}

// resolveSubfield resolves subfield from a field map
func resolveSubfield(fieldMap map[string]interface{}, subParts []string) interface{} {
	if len(subParts) == 0 {
		return fieldMap["value"]
	}

	subfields, ok := fieldMap["subfields"].([]interface{})
	if !ok || len(subfields) == 0 {
		return fieldMap["value"]
	}

	subPosition := 0
	fmt.Sscanf(subParts[0], "%d", &subPosition)

	for _, sf := range subfields {
		if subMap, ok := sf.(map[string]interface{}); ok {
			// Subfield position is component index (1-based)
			if key, ok := subMap["key"].(string); ok {
				// Key format is like "MSH.9.1" - check if last part matches
				keyParts := strings.Split(key, ".")
				if len(keyParts) > 0 {
					lastPart := keyParts[len(keyParts)-1]
					if lastPart == subParts[0] {
						return subMap["value"]
					}
				}
			}
		}
	}

	// Fallback to parent field value
	return fieldMap["value"]
}

// setNestedValue sets a nested value using dot notation
func setNestedValue(data map[string]interface{}, path string, value interface{}) {
	keys := strings.Split(path, ".")

	if len(keys) == 1 {
		data[keys[0]] = value
		return
	}

	current := data
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		if _, exists := current[key]; !exists {
			current[key] = make(map[string]interface{})
		}

		if nextMap, ok := current[key].(map[string]interface{}); ok {
			current = nextMap
		} else {
			// Overwrite with a map if the current value isn't a map
			current[key] = make(map[string]interface{})
			current = current[key].(map[string]interface{})
		}
	}

	current[keys[len(keys)-1]] = value
}

// toFloat64 converts a value to float64
func toFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		var f float64
		fmt.Sscanf(v, "%f", &f)
		return f
	default:
		return 0
	}
}

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getAbsoluteJSONPath converts HL7 field notation to absolute JSON path
// This is used for UI tooltip display to show where the data actually comes from
// Examples:
//   - "PID.3" -> "enhancedSegments.PID.fields[key=PID.3].value"
//   - "PID.13.4" -> "enhancedSegments.PID.fields[key=PID.13].subfields[key=PID.13.4].value"
//   - "MSH.9" -> "enhancedSegments.MSH.fields[key=MSH.9].value"
func getAbsoluteJSONPath(hl7Path string) string {
	parts := strings.Split(hl7Path, ".")
	if len(parts) < 2 {
		// Not an HL7 path, return as-is
		return hl7Path
	}

	segment := parts[0]

	// Check if it looks like an HL7 segment (2-4 uppercase letters/numbers)
	isHL7Segment := len(segment) >= 2 && len(segment) <= 4
	for _, c := range segment {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			isHL7Segment = false
			break
		}
	}

	if !isHL7Segment {
		return hl7Path
	}

	// Build the absolute JSON path
	if len(parts) == 2 {
		// Field-level: PID.3 -> enhancedSegments.PID.fields[key=PID.3].value
		return fmt.Sprintf("enhancedSegments.%s.fields[key=%s].value", segment, hl7Path)
	} else if len(parts) == 3 {
		// Subfield-level: PID.13.4 -> enhancedSegments.PID.fields[key=PID.13].subfields[key=PID.13.4].value
		parentField := fmt.Sprintf("%s.%s", segment, parts[1])
		return fmt.Sprintf("enhancedSegments.%s.fields[key=%s].subfields[key=%s].value", segment, parentField, hl7Path)
	}

	// Unknown format, return as-is
	return hl7Path
}

// updateHL7FieldValue updates the value of an HL7 field in the enhanced segments structure
// This modifies the original message data so downstream steps see the updated value
// Handles both field-level (PID.3) and subfield-level (PID.13.4) paths
func updateHL7FieldValue(data map[string]interface{}, path string, newValue interface{}) bool {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return false
	}

	segmentKey := parts[0]
	fieldKey := fmt.Sprintf("%s.%s", parts[0], parts[1]) // e.g., "PID.13"

	// Find enhancedSegments
	var enhancedSegments interface{}
	if es, ok := data["enhancedSegments"]; ok {
		enhancedSegments = es
	} else if msg, ok := data["message"].(map[string]interface{}); ok {
		enhancedSegments = msg["enhancedSegments"]
	}

	if enhancedSegments == nil {
		fmt.Printf("      ⚠️  [updateHL7FieldValue] enhancedSegments not found\n")
		return false
	}

	// Handle typed Go struct (map[string]hl7.EnhancedSegment)
	if typedSegments, ok := enhancedSegments.(map[string]hl7.EnhancedSegment); ok {
		segment, exists := typedSegments[segmentKey]
		if !exists {
			fmt.Printf("      ⚠️  [updateHL7FieldValue] Segment %s not found\n", segmentKey)
			return false
		}

		// Search through Fields array
		for i := range segment.Fields {
			if segment.Fields[i].Key == fieldKey {
				if len(parts) == 2 {
					// Field-level update
					segment.Fields[i].Value = fmt.Sprintf("%v", newValue)
					typedSegments[segmentKey] = segment
					fmt.Printf("      ✅ [updateHL7FieldValue] Updated typed field %s = %v\n", path, newValue)
					return true
				} else if len(parts) == 3 {
					// Subfield-level update
					subfieldKey := path // e.g., "PID.13.4"
					for j := range segment.Fields[i].Subfields {
						if segment.Fields[i].Subfields[j].Key == subfieldKey {
							segment.Fields[i].Subfields[j].Value = fmt.Sprintf("%v", newValue)
							typedSegments[segmentKey] = segment
							fmt.Printf("      ✅ [updateHL7FieldValue] Updated typed subfield %s = %v\n", path, newValue)
							return true
						}
					}
				}
			}
		}
		return false
	}

	// Handle map[string]interface{} (from JSON deserialization)
	if segmentsMap, ok := enhancedSegments.(map[string]interface{}); ok {
		segmentData, exists := segmentsMap[segmentKey]
		if !exists {
			fmt.Printf("      ⚠️  [updateHL7FieldValue] Segment %s not found in map\n", segmentKey)
			return false
		}

		segmentMap, ok := segmentData.(map[string]interface{})
		if !ok {
			return false
		}

		fields, ok := segmentMap["fields"].([]interface{})
		if !ok {
			return false
		}

		// Search through fields array
		for i, f := range fields {
			fieldMap, ok := f.(map[string]interface{})
			if !ok {
				continue
			}

			if fieldMap["key"] == fieldKey {
				if len(parts) == 2 {
					// Field-level update
					fieldMap["value"] = fmt.Sprintf("%v", newValue)
					fields[i] = fieldMap
					fmt.Printf("      ✅ [updateHL7FieldValue] Updated map field %s = %v\n", path, newValue)
					return true
				} else if len(parts) == 3 {
					// Subfield-level update
					subfieldKey := path
					subfields, ok := fieldMap["subfields"].([]interface{})
					if !ok {
						continue
					}

					for j, sf := range subfields {
						subfieldMap, ok := sf.(map[string]interface{})
						if !ok {
							continue
						}
						if subfieldMap["key"] == subfieldKey {
							subfieldMap["value"] = fmt.Sprintf("%v", newValue)
							subfields[j] = subfieldMap
							fmt.Printf("      ✅ [updateHL7FieldValue] Updated map subfield %s = %v\n", path, newValue)
							return true
						}
					}
				}
			}
		}
	}

	fmt.Printf("      ⚠️  [updateHL7FieldValue] Could not update %s - field not found\n", path)
	return false
}
