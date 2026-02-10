# Pipeline Data Isolation Fix - Critical Architecture Issue

**Date**: January 2026
**Status**: 🔴 **CRITICAL BUG** - Data accumulation causing step pollution
**Impact**: High - Affects all transformation pipelines

---

## Problem Statement

### Issue 1: Data Accumulation (Step Pollution)

**Symptom**: Each step receives accumulated output from ALL previous steps instead of isolated data.

**Example**:
```
Step 1 (Validate): Adds { "validation": { "status": "passed" } }
Step 2 (Enrich):   Receives { "message": {...}, "validation": {...} }
Step 3 (Transform): Receives { "message": {...}, "validation": {...}, "enrichment": {...} }
```

**User Report**: "Steps should not contain the message and other step output, only output from its own"

**Root Cause**:
- Line 110-114 in [conditional_executor.go](services/executors/control/conditional_executor.go:110)
- Line 120 in [transformation_pipeline_helpers.go](services/transformation_pipeline_helpers.go:120)

```go
// WRONG: Copy ALL input data to output
outputData := make(map[string]interface{})
for k, v := range inputData {
    outputData[k] = v  // Accumulates everything!
}

// WRONG: Pass accumulated data to next step
currentData = output  // Contains ALL previous step outputs!
```

---

### Issue 2: Config Format Validation Error

**Symptom**: "Error: condition is required and must be an object"

**Root Cause**: If-Then-Else executor checks 3 different config formats but might receive a 4th format from the builder.

**Config Formats Supported**:
1. **NEWEST** (conditions array with onTrue/onFalse objects)
2. **NEW** (ifThenElse.conditions with thenActions/elseActions arrays)
3. **OLDEST** (flat condition/then_actions/else_actions)

**Issue**: Builder might be creating incompatible format.

---

## Correct Architecture (Already Designed)

The system **already has** the correct data model in [transformation_models.go](models/transformation_models.go:164):

```go
type PipelineExecutionContext struct {
    Message     map[string]interface{} `json:"message"`      // Shared message (transformed)
    StepOutputs map[string]StepOutput  `json:"step_outputs"` // Isolated per-step outputs
    Metadata    map[string]interface{} `json:"metadata"`     // Pipeline metadata
}

type StepOutput struct {
    StepID     string                 `json:"step_id"`
    StepName   string                 `json:"step_name"`
    StepAlias  string                 `json:"step_alias"`   // e.g., "empi"
    Namespace  string                 `json:"namespace"`    // e.g., "empi_b4c9f1"
    OutputData map[string]interface{} `json:"output_data"`  // Step-specific ONLY
    Success    bool                   `json:"success"`
    DurationMs int64                  `json:"duration_ms"`
}
```

**Designed Behavior**:
- ✅ **Message** is shared and transformed through the pipeline
- ✅ **StepOutputs** are isolated and namespaced (empi_b4c9f1, validate_a7f2c3, etc.)
- ✅ Steps can reference previous step outputs via namespace: `{{empi.patientId}}`
- ✅ No data pollution - each step output is separate

---

## Current Implementation (BROKEN)

### What's Happening Now

```go
// transformation_pipeline_helpers.go:68
currentData := inputData  // Starts with full message

for i < len(pipeline.Steps) {
    // Execute step
    output, err := tps.executorRegistry.ExecuteStep(ctx, step, currentData)

    // WRONG: Pass accumulated output to next step
    currentData = output  // Contains: message + step1_output + step2_output + ...
}
```

### What Executors Are Doing

```go
// conditional_executor.go:110
// WRONG: Copy ALL input data
outputData := make(map[string]interface{})
for k, v := range inputData {
    outputData[k] = v  // Copies EVERYTHING
}

// Add step-specific output
outputData["condition_result"] = conditionMet  // Now mixed with everything else!
```

**Result**: Massive data blob with no isolation.

---

## Solution Design

### Option 1: Implement PipelineExecutionContext (Correct)

**Change pipeline execution** to use the designed context structure:

```go
// transformation_pipeline_helpers.go

func (tps *TransformationPipelineService) ExecutePipeline(
    ctx context.Context,
    pipeline *models.TransformationPipeline,
    inputData map[string]interface{},
) (*models.TransformationExecutionResult, error) {
    // Create execution context
    execCtx := &models.PipelineExecutionContext{
        Message:     inputData,
        StepOutputs: make(map[string]models.StepOutput),
        Metadata:    make(map[string]interface{}),
    }

    for i < len(pipeline.Steps) {
        step := pipeline.Steps[i]

        // Execute step with context
        stepOutput, err := tps.executorRegistry.ExecuteStepWithContext(ctx, step, execCtx)

        if err == nil {
            // Store step output in isolated namespace
            namespace := generateNamespace(step)  // e.g., "empi_b4c9f1"
            execCtx.StepOutputs[namespace] = stepOutput

            // Step may have modified execCtx.Message directly
        }
    }

    // Return final message (transformed) + isolated step outputs
    result.Output = execCtx.Message
    result.StepOutputs = execCtx.StepOutputs
}
```

**Change executors** to work with context:

```go
// ExecuteStepWithContext - new signature
func (e *IfThenElseExecutor) ExecuteStepWithContext(
    ctx context.Context,
    step *models.TransformationStep,
    execCtx *models.PipelineExecutionContext,
) (*models.StepOutput, error) {
    // Read from context
    message := execCtx.Message

    // Evaluate condition using message + previous step outputs
    conditionMet, err := e.evaluateCondition(condition, execCtx)

    // Create isolated step output
    stepOutput := &models.StepOutput{
        StepID:     step.ID,
        StepName:   step.StepName,
        StepAlias:  step.Alias,
        Namespace:  step.Namespace,
        OutputData: map[string]interface{}{
            "condition_result": conditionMet,
            "branch_taken":     branchTaken,
        },
        Success:    true,
        DurationMs: duration.Milliseconds(),
    }

    // Optionally modify shared message
    if action == "set_metadata" {
        execCtx.Message["_metadata"] = metadata  // Modify shared message
    }

    return stepOutput, nil
}
```

---

### Option 2: Quick Fix (Temporary)

**Change executors** to ONLY return step-specific data:

```go
// conditional_executor.go

func (e *IfThenElseExecutor) Execute(
    ctx context.Context,
    step *models.TransformationStep,
    inputData map[string]interface{},
) (map[string]interface{}, error) {
    // DON'T copy inputData - create fresh output
    outputData := make(map[string]interface{})

    // ONLY add what THIS step produces
    outputData["condition_result"] = conditionMet
    outputData["branch_taken"] = branchName

    // If step needs to modify message, use special key
    if needToModifyMessage {
        outputData["_message_updates"] = map[string]interface{}{
            "priority": "high",  // Example: set priority
        }
    }

    return outputData, nil
}
```

**Change pipeline** to merge step outputs properly:

```go
// transformation_pipeline_helpers.go

// Execute step
stepOutput, err := tps.executorRegistry.ExecuteStep(ctx, step, currentData)

if err == nil {
    // Store step output separately
    result.StepOutputs[step.Namespace] = stepOutput

    // Merge message updates only (if provided)
    if updates, ok := stepOutput["_message_updates"].(map[string]interface{}); ok {
        for k, v := range updates {
            currentData[k] = v  // Merge into message
        }
        delete(stepOutput, "_message_updates")  // Remove from step output
    }

    // DON'T merge entire stepOutput into currentData!
}
```

---

## Recommended Approach

**Phase 1: Quick Fix (1-2 hours)**
1. Implement Option 2 (temporary fix)
2. Test with current pipelines
3. Verify step outputs are isolated

**Phase 2: Proper Architecture (4-6 hours)**
1. Implement PipelineExecutionContext
2. Update all executors to use context
3. Update step referencing ({{empi.patientId}} syntax)
4. Full regression testing

**Phase 3: Documentation (1 hour)**
1. Update architecture docs
2. Add examples
3. Update builder guides

---

## If-Then-Else Config Fix

**Issue**: Builder creates format, executor expects different format.

**Fix**: Standardize on ONE format.

### Recommended Format (NEWEST)

```json
{
  "conditions": [
    {
      "name": "Condition 1",
      "condition": {
        "field": "patient.age",
        "operator": "greater_than",
        "value": "18"
      },
      "onTrue": {
        "action": "set_metadata",
        "key": "adult",
        "value": true
      },
      "onFalse": {
        "action": "continue"
      }
    }
  ]
}
```

**Change Executor** to ONLY accept this format:

```go
// conditional_executor.go:59

// Parse configuration - ONLY support NEWEST format
conditions, ok := step.Config["conditions"].([]interface{})
if !ok || len(conditions) == 0 {
    return inputData, fmt.Errorf("conditions array is required")
}

firstCond, ok := conditions[0].(map[string]interface{})
if !ok {
    return inputData, fmt.Errorf("first condition must be an object")
}

condition, ok := firstCond["condition"].(map[string]interface{})
if !ok {
    return inputData, fmt.Errorf("condition is required and must be an object")
}

// Extract onTrue/onFalse
onTrue, _ := firstCond["onTrue"].(map[string]interface{})
onFalse, _ := firstCond["onFalse"].(map[string]interface{})
```

**Verify Builder** creates this format:

```javascript
// IfThenElseBuilder.js:32

getDefaultConfig() {
    return {
        conditions: [  // ✅ Root-level conditions array
            {
                name: 'Condition 1',
                condition: {  // ✅ condition object
                    field: '',
                    operator: 'equals',
                    value: ''
                },
                onTrue: {  // ✅ onTrue object (not array)
                    action: 'continue'
                },
                onFalse: {  // ✅ onFalse object (not array)
                    action: 'continue'
                }
            }
        ]
    };
}
```

---

## Testing Plan

### Test 1: Data Isolation

**Setup**: Create pipeline with 3 steps

```
Step 1 (Validate): Output { "validation": { "status": "passed" } }
Step 2 (Enrich):   Output { "enrichment": { "patient_name": "John Doe" } }
Step 3 (Transform): Output { "fhir_bundle": {...} }
```

**Expected Result**:
```json
{
  "message": { ... },  // Final transformed message
  "step_outputs": {
    "validate_a7f2c3": {
      "output_data": { "validation": { "status": "passed" } }
    },
    "enrich_b4c9f1": {
      "output_data": { "enrichment": { "patient_name": "John Doe" } }
    },
    "transform_c2d8e5": {
      "output_data": { "fhir_bundle": {...} }
    }
  }
}
```

**Current (BROKEN) Result**:
```json
{
  "message": {
    "validation": { "status": "passed" },
    "enrichment": { "patient_name": "John Doe" },
    "fhir_bundle": {...}
  }  // Everything mixed together!
}
```

### Test 2: If-Then-Else Config

**Setup**: Create If-Then-Else step

**Expected**: No "condition is required" error

**Verify**: Executor receives config in correct format

---

## Implementation Priority

1. 🔴 **CRITICAL**: Fix If-Then-Else config validation (30 min)
2. 🔴 **CRITICAL**: Implement Option 2 quick fix for data isolation (2 hours)
3. 🟡 **HIGH**: Test with existing pipelines (1 hour)
4. 🟡 **HIGH**: Update documentation (1 hour)
5. 🟢 **MEDIUM**: Implement full PipelineExecutionContext (Phase 2)

---

## Files to Modify

### Critical (Option 2 Quick Fix)

1. [services/executors/control/conditional_executor.go](services/executors/control/conditional_executor.go)
   - Line 110-114: Don't copy inputData
   - Line 59-108: Simplify to ONLY accept NEWEST format

2. [services/transformation_pipeline_helpers.go](services/transformation_pipeline_helpers.go)
   - Line 120: Don't merge entire stepOutput
   - Add step output isolation logic

### Optional (Full Context Implementation)

3. [services/executor_registry.go](services/executor_registry.go)
   - Add `ExecuteStepWithContext()` method

4. All executor files in `services/executors/`
   - Implement context-aware execution

---

## Conclusion

The architecture is **already designed correctly** (PipelineExecutionContext), but **not implemented**. The current code is doing simple data passing which causes accumulation.

**Immediate Fix**: Option 2 (2-3 hours)
**Proper Fix**: Option 1 (6-8 hours)
**User Impact**: HIGH - Affects all pipelines immediately

This is **NOT** related to the modular architecture refactoring we just completed. The refactoring was about **code organization** (OOP, MVC, modularity) - this is a **runtime data flow bug** in the pipeline execution engine.

---

**Status**: 🔴 **AWAITING FIX**
**Priority**: 🔴 **CRITICAL**
**Estimated Fix Time**: 2-3 hours (Option 2)
