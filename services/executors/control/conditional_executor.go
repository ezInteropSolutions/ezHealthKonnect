package control

import (
	"context"
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

	base := executors.NewBaseExecutor("pre.logic", metadata)

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

	// Parse configuration
	condition, ok := step.Config["condition"].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("condition is required and must be an object")
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	thenActions, _ := step.Config["then_actions"].([]interface{})
	elseActions, _ := step.Config["else_actions"].([]interface{})

	// Copy input data to output
	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Evaluate condition
	conditionMet, err := e.evaluateCondition(condition, outputData)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return outputData, fmt.Errorf("condition evaluation failed: %v", err)
	}

	log.Printf("🔀 [IfThenElse] Condition evaluated: %v", conditionMet)

	// Execute appropriate actions
	var actionsToExecute []interface{}
	if conditionMet {
		actionsToExecute = thenActions
		log.Printf("   Executing %d THEN actions", len(thenActions))
	} else {
		actionsToExecute = elseActions
		log.Printf("   Executing %d ELSE actions", len(elseActions))
	}

	// Execute actions
	for i, action := range actionsToExecute {
		actionMap, ok := action.(map[string]interface{})
		if !ok {
			continue
		}

		actionType := getStringValue(actionMap, "action")
		log.Printf("   [%d] Action: %s", i+1, actionType)

		if err := e.executeAction(actionMap, outputData); err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return outputData, err
		}
	}

	// Post-execution tracking
	e.PostExecute(ctx, step, nil, time.Since(start))
	return outputData, nil
}

// evaluateCondition evaluates a condition against data
func (e *IfThenElseExecutor) evaluateCondition(condition map[string]interface{}, data map[string]interface{}) (bool, error) {
	field := getStringValue(condition, "field")
	operator := getStringValue(condition, "operator")

	// Get first field value
	fieldValue := getNestedValue(data, field)

	// Determine comparison value (constant or field)
	var compareValue interface{}
	compareToField := getStringValue(condition, "compareToField")
	if compareToField != "" {
		// Cross-field comparison
		compareValue = getNestedValue(data, compareToField)
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

	base := executors.NewBaseExecutor("pre.logic.switch", metadata)

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

	field := getStringValue(step.Config, "field")
	cases, ok := step.Config["cases"].([]interface{})
	if !ok {
		err := fmt.Errorf("invalid cases configuration")
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}
	defaultActions, _ := step.Config["default"].([]interface{})

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	fieldValue := getNestedValue(outputData, field)
	fieldStr := fmt.Sprintf("%v", fieldValue)

	log.Printf("🔀 [SwitchCase] Evaluating %s = %s", field, fieldStr)

	// Find matching case
	var matchedActions []interface{}
	matched := false

	for i, caseItem := range cases {
		caseMap := caseItem.(map[string]interface{})
		caseValue := fmt.Sprintf("%v", caseMap["value"])

		if fieldStr == caseValue {
			matchedActions = caseMap["actions"].([]interface{})
			matched = true
			log.Printf("   Matched case %d: %s", i+1, caseValue)
			break
		}
	}

	// Use default if no match
	if !matched {
		matchedActions = defaultActions
		log.Printf("   No match, using default actions")
	}

	// Execute actions
	for i, action := range matchedActions {
		actionMap := action.(map[string]interface{})
		actionType := getStringValue(actionMap, "action")

		log.Printf("   [%d] Action: %s", i+1, actionType)

		switch actionType {
		case "set_value":
			targetField := getStringValue(actionMap, "field")
			value := actionMap["value"]
			setNestedValue(outputData, targetField, value)

		case "copy_field":
			sourceField := getStringValue(actionMap, "source")
			targetField := getStringValue(actionMap, "target")
			value := getNestedValue(outputData, sourceField)
			if value != nil {
				setNestedValue(outputData, targetField, value)
			}

		case "transform":
			targetField := getStringValue(actionMap, "field")
			transformation := getStringValue(actionMap, "transformation")
			value := getNestedValue(outputData, targetField)
			if value != nil {
				transformedValue := e.applyTransformation(value, transformation)
				setNestedValue(outputData, targetField, transformedValue)
			}

		default:
			err := fmt.Errorf("unsupported action: %s", actionType)
			e.PostExecute(ctx, step, err, time.Since(start))
			return outputData, err
		}
	}

	e.PostExecute(ctx, step, nil, time.Since(start))
	return outputData, nil
}

// applyTransformation applies a string transformation
func (e *SwitchCaseExecutor) applyTransformation(value interface{}, transformation string) interface{} {
	valueStr := fmt.Sprintf("%v", value)

	switch transformation {
	case "uppercase":
		return strings.ToUpper(valueStr)
	case "lowercase":
		return strings.ToLower(valueStr)
	case "trim":
		return strings.TrimSpace(valueStr)
	default:
		return value
	}
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
func getNestedValue(data map[string]interface{}, path string) interface{} {
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
