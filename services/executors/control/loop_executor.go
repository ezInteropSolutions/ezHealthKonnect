package control

import (
	"context"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"
)

// ===============================================================
// LOOP EXECUTOR - Container Step for Iterative Execution
// ===============================================================

// ChildStepExecutor is a callback function that executes a child step
// Returns the output data from the step and any error
type ChildStepExecutor func(
	ctx context.Context,
	stepID string,
	inputData map[string]interface{},
) (outputData map[string]interface{}, stepName string, err error)

// LoopExecutor implements loop logic (For Each, For, While)
// This is a container step that executes nested child steps in a loop
type LoopExecutor struct {
	*executors.BaseExecutor
	childExecutor       ChildStepExecutor          // Callback to execute child steps
	collectionResolvers *CollectionResolverRegistry // OOP multi-format collection resolvers
}

// SetChildExecutor sets the callback function for executing child steps
// This must be called before Execute() to enable actual child step execution
func (e *LoopExecutor) SetChildExecutor(executor ChildStepExecutor) {
	e.childExecutor = executor
}

// NewLoopExecutor creates a new loop executor
func NewLoopExecutor() *LoopExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Loop Container",
		Description: "Container step that executes nested steps in a loop. Supports For Each (iterate collection), For (iterate N times), and While (condition-based) loop types.",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "control",
	}

	base := executors.NewBaseExecutor("control.loop", metadata)

	return &LoopExecutor{
		BaseExecutor:        base,
		collectionResolvers: NewCollectionResolverRegistry(),
	}
}

// Validate validates loop step configuration
func (e *LoopExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("loop configuration is required")
	}

	loopType := getStringValue(step.Config, "loopType")
	if loopType == "" {
		loopType = "foreach" // Default
	}

	switch loopType {
	case "foreach":
		collection := getStringValue(step.Config, "collection")
		if collection == "" {
			return fmt.Errorf("collection field path is required for For Each loop")
		}
	case "for":
		iterations := getIntValue(step.Config, "iterations")
		if iterations < 1 {
			return fmt.Errorf("iterations must be at least 1 for For loop")
		}
	case "while":
		condition, ok := step.Config["condition"].(map[string]interface{})
		if !ok || getStringValue(condition, "field") == "" {
			return fmt.Errorf("condition field is required for While loop")
		}
	default:
		return fmt.Errorf("invalid loop type: %s (expected: foreach, for, while)", loopType)
	}

	return nil
}

// Execute performs loop execution
func (e *LoopExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	log.Printf("🔄 [Loop] EXECUTING - Step: %s", step.StepName)

	// Get loop configuration
	loopType := getStringValue(step.Config, "loopType")
	if loopType == "" {
		loopType = "foreach"
	}

	itemVariable := getStringValue(step.Config, "itemVariable")
	if itemVariable == "" {
		itemVariable = "item"
	}

	indexVariable := getStringValue(step.Config, "indexVariable")
	if indexVariable == "" {
		indexVariable = "index"
	}

	maxIterations := getIntValue(step.Config, "maxIterations")
	if maxIterations <= 0 {
		maxIterations = 1000 // Safety limit
	}

	breakOnError := getBoolValue(step.Config, "breakOnError", false)
	continueOnEmpty := getBoolValue(step.Config, "continueOnEmpty", true) // Default true

	childStepIds := getStringArrayValue(step.Config, "childStepIds")

	log.Printf("🔄 [Loop] Type=%s, Collection=%s, ChildSteps=%d, MaxIterations=%d",
		loopType, getStringValue(step.Config, "collection"), len(childStepIds), maxIterations)

	// Initialize output data
	outputData := make(map[string]interface{})

	// Preserve original message
	if message, ok := inputData["message"]; ok {
		outputData["message"] = message
	}

	// Preserve routing directives
	if routing, ok := inputData["_routing"]; ok {
		outputData["_routing"] = routing
	}

	// Preserve metadata
	if metadata, ok := inputData["_metadata"]; ok {
		outputData["_metadata"] = metadata
	}

	// Loop results storage
	loopResults := make([]map[string]interface{}, 0)
	var loopErrors []string

	// Execute based on loop type
	var iterationCount int
	var err error

	switch loopType {
	case "foreach":
		iterationCount, err = e.executeForEach(ctx, step, inputData, outputData, &loopResults, &loopErrors,
			itemVariable, indexVariable, maxIterations, breakOnError, continueOnEmpty, childStepIds)

	case "for":
		iterations := getIntValue(step.Config, "iterations")
		if iterations <= 0 {
			iterations = 10
		}
		iterationCount, err = e.executeFor(ctx, step, inputData, outputData, &loopResults, &loopErrors,
			indexVariable, iterations, maxIterations, breakOnError, childStepIds)

	case "while":
		condition, _ := step.Config["condition"].(map[string]interface{})
		iterationCount, err = e.executeWhile(ctx, step, inputData, outputData, &loopResults, &loopErrors,
			indexVariable, condition, maxIterations, breakOnError, childStepIds)

	default:
		err = fmt.Errorf("unsupported loop type: %s", loopType)
	}

	duration := time.Since(start)

	// ✅ AGGREGATION: Collect child step outputs into all_{stepName} lists
	// This creates a simple, no-code friendly output structure
	aggregatedOutputs := e.aggregateChildOutputs(loopResults)

	// Build step output - simple flat structure:
	// { iterations, errors, child_steps: { stepName: [iter0, iter1, ...] } }
	stepOutput := map[string]interface{}{
		"iterations":  iterationCount,
		"child_steps": aggregatedOutputs,
	}

	// Only include errors if there are any
	if len(loopErrors) > 0 {
		stepOutput["errors"] = loopErrors
	}

	// Set loop summary for downstream access (lightweight, no circular refs)
	outputData["_loop"] = map[string]interface{}{
		"completed":      err == nil,
		"iterations":     iterationCount,
		"childStepCount": len(childStepIds),
	}

	// Store step output
	e.BaseExecutor.SetOutputVariables(outputData, stepOutput)

	if err != nil {
		stepOutput["_error"] = err.Error()
		log.Printf("❌ [Loop] Failed after %d iterations: %v", iterationCount, err)
	} else {
		log.Printf("✅ [Loop] Completed %d iterations in %v", iterationCount, duration)
	}

	return outputData, err
}

// executeForEach executes a For Each loop over a collection
func (e *LoopExecutor) executeForEach(
	ctx context.Context,
	step *models.TransformationStep,
	inputData, outputData map[string]interface{},
	loopResults *[]map[string]interface{},
	loopErrors *[]string,
	itemVariable, indexVariable string,
	maxIterations int,
	breakOnError, continueOnEmpty bool,
	childStepIds []string,
) (int, error) {
	collectionPath := getStringValue(step.Config, "collection")
	if collectionPath == "" {
		return 0, fmt.Errorf("collection field path is required for For Each loop")
	}

	// Get the collection from input data
	collection := e.getCollectionFromPath(inputData, collectionPath)
	if collection == nil {
		if continueOnEmpty {
			log.Printf("⚠️  [Loop:ForEach] Collection '%s' is empty/nil, continuing pipeline", collectionPath)
			return 0, nil
		}
		return 0, fmt.Errorf("collection at path '%s' is empty or not found", collectionPath)
	}

	collectionLen := len(collection)
	log.Printf("🔄 [Loop:ForEach] Found %d items in collection", collectionLen)

	if collectionLen == 0 {
		if continueOnEmpty {
			log.Printf("⚠️  [Loop:ForEach] Collection is empty, continuing pipeline")
			return 0, nil
		}
		return 0, fmt.Errorf("collection is empty")
	}

	// Iterate over collection
	iterationCount := 0
	for i, item := range collection {
		// Check max iterations
		if iterationCount >= maxIterations {
			log.Printf("⚠️  [Loop:ForEach] Reached max iterations limit (%d)", maxIterations)
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return iterationCount, fmt.Errorf("loop cancelled: %w", ctx.Err())
		}

		// Create loop context for this iteration
		loopContext := map[string]interface{}{
			itemVariable:  item,
			indexVariable: i,
			"iteration":   i + 1,
			"isFirst":     i == 0,
			"isLast":      i == collectionLen-1,
			"length":      collectionLen,
		}

		log.Printf("   [Iteration %d/%d] Processing item", i+1, collectionLen)

		// Execute child steps for this iteration
		iterationResult, iterationErr := e.executeChildSteps(ctx, step, inputData, loopContext, childStepIds)

		if iterationErr != nil {
			errMsg := fmt.Sprintf("Iteration %d failed: %v", i+1, iterationErr)
			*loopErrors = append(*loopErrors, errMsg)
			log.Printf("   ❌ %s", errMsg)

			if breakOnError {
				return iterationCount, fmt.Errorf("loop stopped at iteration %d: %v", i+1, iterationErr)
			}
		}

		*loopResults = append(*loopResults, map[string]interface{}{
			"index":   i,
			"result":  iterationResult,
			"success": iterationErr == nil,
		})

		iterationCount++
	}

	return iterationCount, nil
}

// executeFor executes a For loop with a fixed number of iterations
func (e *LoopExecutor) executeFor(
	ctx context.Context,
	step *models.TransformationStep,
	inputData, outputData map[string]interface{},
	loopResults *[]map[string]interface{},
	loopErrors *[]string,
	indexVariable string,
	iterations, maxIterations int,
	breakOnError bool,
	childStepIds []string,
) (int, error) {
	log.Printf("🔄 [Loop:For] Iterations: %d", iterations)

	// Limit to max iterations
	if iterations > maxIterations {
		log.Printf("⚠️  [Loop:For] Iterations (%d) exceeds max (%d), limiting", iterations, maxIterations)
		iterations = maxIterations
	}

	iterationCount := 0
	for i := 0; i < iterations; i++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return iterationCount, fmt.Errorf("loop cancelled: %w", ctx.Err())
		}

		// Create loop context for this iteration
		loopContext := map[string]interface{}{
			indexVariable: i,
			"iteration":   i + 1,
			"isFirst":     i == 0,
			"isLast":      i == iterations-1,
			"total":       iterations,
		}

		log.Printf("   [Iteration %d/%d]", i+1, iterations)

		// Execute child steps for this iteration
		iterationResult, iterationErr := e.executeChildSteps(ctx, step, inputData, loopContext, childStepIds)

		if iterationErr != nil {
			errMsg := fmt.Sprintf("Iteration %d failed: %v", i+1, iterationErr)
			*loopErrors = append(*loopErrors, errMsg)
			log.Printf("   ❌ %s", errMsg)

			if breakOnError {
				return iterationCount, fmt.Errorf("loop stopped at iteration %d: %v", i+1, iterationErr)
			}
		}

		*loopResults = append(*loopResults, map[string]interface{}{
			"index":   i,
			"result":  iterationResult,
			"success": iterationErr == nil,
		})

		iterationCount++
	}

	return iterationCount, nil
}

// executeWhile executes a While loop based on a condition
func (e *LoopExecutor) executeWhile(
	ctx context.Context,
	step *models.TransformationStep,
	inputData, outputData map[string]interface{},
	loopResults *[]map[string]interface{},
	loopErrors *[]string,
	indexVariable string,
	condition map[string]interface{},
	maxIterations int,
	breakOnError bool,
	childStepIds []string,
) (int, error) {
	log.Printf("🔄 [Loop:While] Condition: %v", condition)

	iterationCount := 0
	for {
		// Check max iterations
		if iterationCount >= maxIterations {
			log.Printf("⚠️  [Loop:While] Reached max iterations limit (%d)", maxIterations)
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return iterationCount, fmt.Errorf("loop cancelled: %w", ctx.Err())
		}

		// Evaluate condition
		conditionMet := e.evaluateCondition(inputData, condition)
		if !conditionMet {
			log.Printf("   [While] Condition no longer met, exiting loop")
			break
		}

		// Create loop context for this iteration
		loopContext := map[string]interface{}{
			indexVariable: iterationCount,
			"iteration":   iterationCount + 1,
		}

		log.Printf("   [Iteration %d] Condition met, executing", iterationCount+1)

		// Execute child steps for this iteration
		iterationResult, iterationErr := e.executeChildSteps(ctx, step, inputData, loopContext, childStepIds)

		if iterationErr != nil {
			errMsg := fmt.Sprintf("Iteration %d failed: %v", iterationCount+1, iterationErr)
			*loopErrors = append(*loopErrors, errMsg)
			log.Printf("   ❌ %s", errMsg)

			if breakOnError {
				return iterationCount, fmt.Errorf("loop stopped at iteration %d: %v", iterationCount+1, iterationErr)
			}
		}

		*loopResults = append(*loopResults, map[string]interface{}{
			"index":   iterationCount,
			"result":  iterationResult,
			"success": iterationErr == nil,
		})

		iterationCount++
	}

	return iterationCount, nil
}

// executeChildSteps executes the child steps for one iteration
// ✅ ENHANCED: Now actually executes child steps via callback and collects outputs
// When a child step is also a Loop, it can reference the parent loop's item in its collection path
func (e *LoopExecutor) executeChildSteps(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
	loopContext map[string]interface{},
	childStepIds []string,
) (map[string]interface{}, error) {
	// Build enriched input data for child steps with loop context accessible
	childInputData := make(map[string]interface{})

	// Copy original input data
	for k, v := range inputData {
		childInputData[k] = v
	}

	// ✅ Add loop context in multiple accessible formats for nested loops:
	// - "loop" : { item, index, isFirst, isLast, length }  → Primary access
	// - "item" : current item (shorthand)
	// - "index": current index (shorthand)
	// - "_parentLoops": stack of parent loop contexts (for deeply nested)
	childInputData["loop"] = loopContext
	childInputData["item"] = loopContext[getStringValue(step.Config, "itemVariable")]
	childInputData["index"] = loopContext[getStringValue(step.Config, "indexVariable")]

	// Build parent loops stack for deeply nested scenarios
	parentLoops := []map[string]interface{}{}
	if existingParents, ok := inputData["_parentLoops"].([]map[string]interface{}); ok {
		parentLoops = append(parentLoops, existingParents...)
	}
	parentLoops = append(parentLoops, loopContext)
	childInputData["_parentLoops"] = parentLoops

	// ✅ NEW: Actually execute child steps if executor callback is available
	if e.childExecutor != nil && len(childStepIds) > 0 {
		log.Printf("     → Executing %d child steps with callback", len(childStepIds))

		// Track ONLY user-created variables from each child step (not full output maps)
		childOutputs := make(map[string]interface{})
		currentInput := childInputData

		for _, childStepID := range childStepIds {
			// Execute the child step
			fullOutput, stepName, err := e.childExecutor(ctx, childStepID, currentInput)
			if err != nil {
				log.Printf("     ❌ Child step %s failed: %v", childStepID, err)
				return nil, fmt.Errorf("child step %s failed: %w", childStepID, err)
			}

			log.Printf("     ✅ Child step '%s' (%s) completed", stepName, childStepID)

			// Extract ONLY _stepOutput (user variables) to avoid circular references.
			// The full output map contains shared references (message, _routing, etc.)
			// that create cycles when stored in aggregated results.
			if fullOutput != nil {
				if userVars, ok := fullOutput["_stepOutput"].(map[string]interface{}); ok {
					childOutputs[stepName] = userVars
				} else {
					childOutputs[stepName] = map[string]interface{}{}
				}
				// Chain: next child step still gets the full output as input
				currentInput = fullOutput
			}
		}

		// Return child outputs for this iteration (cycle-free)
		result := map[string]interface{}{
			"childOutputs": childOutputs,
			"status":       "completed",
		}

		return result, nil
	}

	// Fallback: If no executor callback, just return routing directives (legacy behavior)
	result := map[string]interface{}{
		"loopContext":  loopContext,
		"childStepIds": childStepIds,
		"status":       "pending_execution",
		"inputData":    childInputData,
	}

	if len(childStepIds) > 0 {
		log.Printf("     → Routing to child steps (no callback): %v", childStepIds)
		result["_routing"] = map[string]interface{}{
			"executeChildSteps": childStepIds,
			"loopContext":       loopContext,
			"childInputData":    childInputData,
		}
	}

	return result, nil
}

// aggregateChildOutputs collects outputs from all iterations and compacts them.
// Compact mode: transposes per-iteration objects into per-variable arrays.
// Example: [{planid:"PPO123"},{planid:"HMO456"}] → {planid:["PPO123","HMO456"]}
func (e *LoopExecutor) aggregateChildOutputs(loopResults []map[string]interface{}) map[string]interface{} {
	// Step 1: Collect iteration outputs per step name
	stepLists := make(map[string][]interface{})

	for _, iterResult := range loopResults {
		childOutputs, ok := iterResult["result"].(map[string]interface{})
		if !ok {
			continue
		}

		outputs, ok := childOutputs["childOutputs"].(map[string]interface{})
		if !ok {
			outputs = childOutputs
		}

		for stepName, output := range outputs {
			// Skip internal keys
			if stepName == "loopContext" || stepName == "status" ||
				stepName == "finalOutput" || stepName == "childStepIds" ||
				stepName == "inputData" {
				continue
			}

			key := strings.ReplaceAll(stepName, " ", "_")

			if _, exists := stepLists[key]; !exists {
				stepLists[key] = make([]interface{}, 0)
			}
			stepLists[key] = append(stepLists[key], output)
		}
	}

	// Step 2: Compact each step's iteration list
	aggregated := make(map[string]interface{})
	for key, list := range stepLists {
		// Unwrap mapped_fields and collect flat maps
		flatMaps := make([]map[string]interface{}, 0, len(list))
		allFlat := true

		for _, item := range list {
			m, ok := item.(map[string]interface{})
			if !ok {
				allFlat = false
				break
			}

			// Unwrap mapped_fields if it's the only key (or primary key)
			if mf, hasMF := m["mapped_fields"].(map[string]interface{}); hasMF {
				flatMaps = append(flatMaps, mf)
				continue
			}

			// Check if this is a nested loop output (has child_steps + iterations)
			if _, hasCS := m["child_steps"]; hasCS {
				allFlat = false
				break
			}

			// Otherwise treat as a flat map
			flatMaps = append(flatMaps, m)
		}

		if allFlat && len(flatMaps) > 0 {
			// Transpose: [{k1:v1,k2:v2},{k1:v3,k2:v4}] → {k1:[v1,v3], k2:[v2,v4]}
			aggregated[key] = compactFlatMaps(flatMaps)
			log.Printf("   📦 Compacted %s: %d iterations → per-variable arrays", key, len(flatMaps))
		} else {
			// Nested loop outputs: recursively compact child_steps inside each iteration
			compacted := make([]interface{}, 0, len(list))
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					compacted = append(compacted, compactNestedLoopOutput(m))
				} else {
					compacted = append(compacted, item)
				}
			}
			aggregated[key] = compacted
			log.Printf("   📦 Aggregated %s: %d iterations (nested/complex)", key, len(list))
		}
	}

	return aggregated
}

// compactFlatMaps transposes a list of flat maps into per-key arrays.
// [{k1:v1,k2:v2},{k1:v3,k2:v4}] → {k1:[v1,v3], k2:[v2,v4]}
func compactFlatMaps(items []map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Collect all keys across all items
	for _, item := range items {
		for key := range item {
			if _, exists := result[key]; !exists {
				result[key] = make([]interface{}, 0, len(items))
			}
		}
	}

	// Fill arrays in order
	for _, item := range items {
		for key := range result {
			arr := result[key].([]interface{})
			if val, exists := item[key]; exists {
				result[key] = append(arr, val)
			} else {
				result[key] = append(arr, nil)
			}
		}
	}

	return result
}

// compactNestedLoopOutput recursively compacts child_steps inside a nested loop output.
// If the output has "child_steps" map, each child step's iteration list gets compacted.
func compactNestedLoopOutput(output map[string]interface{}) map[string]interface{} {
	childSteps, ok := output["child_steps"].(map[string]interface{})
	if !ok {
		return output
	}

	compactedChildren := make(map[string]interface{})
	for stepName, stepData := range childSteps {
		iterList, ok := stepData.([]interface{})
		if !ok {
			compactedChildren[stepName] = stepData
			continue
		}

		// Try to compact this step's iterations (same logic as main aggregation)
		flatMaps := make([]map[string]interface{}, 0, len(iterList))
		allFlat := true

		for _, item := range iterList {
			m, ok := item.(map[string]interface{})
			if !ok {
				allFlat = false
				break
			}
			if mf, hasMF := m["mapped_fields"].(map[string]interface{}); hasMF {
				flatMaps = append(flatMaps, mf)
				continue
			}
			if _, hasCS := m["child_steps"]; hasCS {
				allFlat = false
				break
			}
			flatMaps = append(flatMaps, m)
		}

		if allFlat && len(flatMaps) > 0 {
			compactedChildren[stepName] = compactFlatMaps(flatMaps)
		} else {
			// Recursively compact nested loop outputs
			compacted := make([]interface{}, 0, len(iterList))
			for _, item := range iterList {
				if m, ok := item.(map[string]interface{}); ok {
					compacted = append(compacted, compactNestedLoopOutput(m))
				} else {
					compacted = append(compacted, item)
				}
			}
			compactedChildren[stepName] = compacted
		}
	}

	// Return a new map with compacted child_steps
	result := make(map[string]interface{})
	for k, v := range output {
		if k == "child_steps" {
			result[k] = compactedChildren
		} else {
			result[k] = v
		}
	}
	return result
}

// getCollectionFromPath retrieves a collection using the OOP CollectionResolverRegistry.
// Delegates to format-specific resolvers (HL7, FHIR, JSON) that auto-detect the data format.
func (e *LoopExecutor) getCollectionFromPath(data map[string]interface{}, path string) []interface{} {
	// Delegate to the OOP collection resolver registry
	collection, resolverName := e.collectionResolvers.ResolveCollection(data, path)
	if collection != nil {
		log.Printf("✅ [Loop] Resolved %d items via %s resolver for path: %s", len(collection), resolverName, path)
		return collection
	}

	log.Printf("⚠️  [Loop] No resolver could find collection for path: %s", path)
	return nil
}

// evaluateCondition evaluates a while loop condition
func (e *LoopExecutor) evaluateCondition(data map[string]interface{}, condition map[string]interface{}) bool {
	field := getStringValue(condition, "field")
	operator := getStringValue(condition, "operator")
	value := getStringValue(condition, "value")

	if field == "" {
		return false
	}

	// Get field value
	fieldValue := e.getFieldValue(data, field)
	fieldStr := fmt.Sprintf("%v", fieldValue)

	switch operator {
	case "equals":
		return fieldStr == value
	case "not_equals":
		return fieldStr != value
	case "contains":
		return strings.Contains(fieldStr, value)
	case "not_empty":
		return fieldValue != nil && fieldStr != ""
	case "is_empty":
		return fieldValue == nil || fieldStr == ""
	case "greater_than":
		// Simple numeric comparison
		return fieldStr > value
	case "less_than":
		return fieldStr < value
	case "true":
		return fieldStr == "true" || fieldStr == "1"
	case "false":
		return fieldStr == "false" || fieldStr == "0"
	default:
		return fieldValue != nil && fieldStr != ""
	}
}

// getFieldValue retrieves a field value from data using dot notation
func (e *LoopExecutor) getFieldValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")

	var current interface{} = data
	for _, part := range parts {
		if current == nil {
			return nil
		}

		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// ===============================================================
// HELPER FUNCTIONS - Use functions from conditional_executor.go
// ===============================================================
// getStringValue, getBoolValue, getNestedValue are defined in conditional_executor.go

// getIntValue safely extracts an int value from a map
func getIntValue(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		case string:
			// Try to parse string as int
			var i int
			fmt.Sscanf(val, "%d", &i)
			return i
		}
	}
	return 0
}

// getStringArrayValue safely extracts a string array from a map
func getStringArrayValue(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case []string:
			return val
		case []interface{}:
			result := make([]string, 0, len(val))
			for _, item := range val {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}
