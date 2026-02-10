# Conditional Step Routing - Design Document

**Date:** December 28, 2025
**Status:** 🎯 APPROVED - User Clarification

---

## User Clarification ✅

**User Intent:** "Route To" should route to **different pipeline steps** based on conditions, NOT external endpoints.

**Example:**
```
IF patient.age > 65
  THEN Route To: Step 200 (Geriatric Enrichment)
  ELSE Route To: Step 300 (Standard Processing)
```

This is **Use Case 2: Conditional Flow Control** from the architecture analysis.

---

## Simplified Action List (5 Actions)

### 1. Continue (+ optional logging)
Proceed to next step in sequence

### 2. Stop (+ error message)
Halt pipeline with error

### 3. Set Value (+ value type)
Assign value to any field

### 4. Delete Field
Remove field from data

### 5. Route To Step
**Jump to a different step in the pipeline**

---

## Route To Step - Detailed Design

### Purpose
Allow conditional branching in pipeline execution based on data conditions.

### Use Cases

**1. Age-Based Processing Paths**
```
IF patient.age > 65
  THEN Route To: Step 200 (Geriatric Enrichment)
  ELSE Continue (proceed to next step)
```

**2. Message Type Routing**
```
IF message.type equals "ADT^A01"
  THEN Route To: Step 100 (Admission Flow)
ELSE IF message.type equals "ADT^A03"
  THEN Route To: Step 300 (Discharge Flow)
ELSE
  Route To: Step 500 (Default Flow)
```

**3. Priority-Based Fast Track**
```
IF metadata.priority equals "urgent"
  THEN Route To: Step 900 (Skip validation, go to delivery)
ELSE
  Continue (proceed to next step)
```

**4. Conditional Validation**
```
IF patient.vip equals true
  THEN Route To: Step 250 (Skip validation)
ELSE
  Route To: Step 100 (Full validation)
```

---

## UI Design

### Simple Dropdown (Step Selector)

```
┌──────────────────────────────────────────────────────────────┐
│ Action: [Route To Step ▼]                                    │
├──────────────────────────────────────────────────────────────┤
│ Jump To Step: [Step 200 - Geriatric Enrichment ▼]           │
│                                                              │
│ Available Steps:                                             │
│   Step 100 - Field Validation                               │
│   Step 150 - API Enrichment (Epic)                          │
│   Step 200 - Geriatric Enrichment                           │
│   Step 250 - VIP Processing                                 │
│   Step 300 - Standard Processing                            │
│   Step 900 - Delivery (Skip Validation)                     │
│                                                              │
│ ⚠️ Current Step: 50 - If-Then-Else Logic                    │
│ Note: Can only route to steps after current step            │
└──────────────────────────────────────────────────────────────┘
```

### Smart Step Loading

**Load from current pipeline:**
```javascript
async loadPipelineSteps(currentStepSequence) {
    const pipeline = window.pipelineBuilder.getPipeline();

    return pipeline.steps
        .filter(step => step.sequence > currentStepSequence) // Only forward jumps
        .map(step => ({
            value: step.id,
            label: `Step ${step.sequence} - ${step.stepName}`,
            sequence: step.sequence
        }))
        .sort((a, b) => a.sequence - b.sequence);
}
```

---

## Backend Implementation

### Current Limitation ⚠️

**Problem:** Current executor architecture executes steps sequentially:

```go
// processing/engine.go
func (pe *ProcessingEngine) executePipeline(pipeline Pipeline, data map[string]interface{}) error {
    steps := pipeline.Steps

    for i := 0; i < len(steps); i++ {
        step := steps[i]

        // Execute step
        result, err := pe.executeStep(step, data)
        if err != nil {
            return err
        }

        // Always proceeds to i++
    }
}
```

**Issue:** No way to jump to arbitrary step - always goes to next in sequence.

### Solution: Add Flow Control Support

**Option A: Simple - Use `_routing.nextStep` (Recommended)**

```go
// services/executors/control/conditional_executor.go

case "route_to_step":
    stepId := getStringValue(actionMap, "stepId")

    // Set next step in routing metadata
    if _, exists := outputData["_routing"]; !exists {
        outputData["_routing"] = make(map[string]interface{})
    }

    routingMap := outputData["_routing"].(map[string]interface{})
    routingMap["nextStep"] = stepId

    log.Printf("      Set _routing.nextStep = %s", stepId)
```

**Update execution engine:**

```go
// processing/engine.go
func (pe *ProcessingEngine) executePipeline(pipeline Pipeline, data map[string]interface{}) error {
    steps := pipeline.Steps

    i := 0
    for i < len(steps) {
        step := steps[i]

        // Execute step
        result, err := pe.executeStep(step, data)
        if err != nil {
            return err
        }

        // Check if step set routing.nextStep
        if routing, ok := data["_routing"].(map[string]interface{}); ok {
            if nextStepId, ok := routing["nextStep"].(string); ok {
                // Find step by ID
                nextIndex := pe.findStepIndexById(steps, nextStepId)
                if nextIndex >= 0 {
                    log.Printf("🔀 Routing to step %d (ID: %s)", nextIndex, nextStepId)
                    i = nextIndex
                    delete(routing, "nextStep") // Clear after use
                    continue
                }
            }
        }

        // Normal sequential execution
        i++
    }

    return nil
}

func (pe *ProcessingEngine) findStepIndexById(steps []TransformationStep, stepId string) int {
    for i, step := range steps {
        if step.ID == stepId {
            return i
        }
    }
    return -1
}
```

**Option B: Complex - Execution Plan (Future Enhancement)**

```go
// More sophisticated, allows complex routing patterns
data["_executionPlan"] = []string{"step-200", "step-210", "step-900"}
```

---

## Complete Action Implementation

### 1. Continue (with optional logging)

**UI:**
```
Action: Continue
Log Level: [None/Info/Warning/Debug ▼]
Message: [optional]
```

**Backend:**
```go
// No action or log_info/log_warning/log_debug
{ action: "continue" }
{ action: "log_info", message: "..." }
```

---

### 2. Stop (with error message)

**UI:**
```
Action: Stop
Error Message: [required]
Severity: [Warning/Error/Fatal ▼]
```

**Backend:**
```go
{ action: "reject", errorMessage: "...", severity: "error" }
```

---

### 3. Set Value (unified assignment)

**UI:**
```
Action: Set Value
Target Field: [patient.priority 🔍]
Value Type: [Constant/Copy From ▼]

If Constant:  Value: [high]
If Copy From: Source Field: [PID.8 🔍]
```

**Backend (auto-detects):**
```go
// If target starts with _metadata.
{ action: "set_metadata", metadata: {...} }

// If value type is Copy From
{ action: "copy_field", source: "...", target: "..." }

// Otherwise
{ action: "set_field", field: "...", value: "..." }
```

---

### 4. Delete Field

**UI:**
```
Action: Delete Field
Field Path: [patient.ssn 🔍]
```

**Backend:**
```go
{ action: "delete_field", field: "patient.ssn" }
```

---

### 5. Route To Step

**UI:**
```
Action: Route To Step
Jump To: [Step 200 - Geriatric Enrichment ▼]
```

**Backend:**
```go
{ action: "route_to_step", stepId: "step-uuid-200" }

// Sets in data:
{
  "_routing": {
    "nextStep": "step-uuid-200"
  }
}
```

---

## UI Implementation

### Update IfThenElseBuilder

**Action Options (5 total):**
```javascript
createActionOptions(selectedAction) {
    const actions = [
        { value: 'continue', label: 'Continue', icon: 'arrow-right', color: '#10b981' },
        { value: 'stop', label: 'Stop', icon: 'ban', color: '#ef4444' },
        { value: 'set_value', label: 'Set Value', icon: 'edit', color: '#3b82f6' },
        { value: 'delete_field', label: 'Delete Field', icon: 'trash', color: '#ef4444' },
        { value: 'route_to_step', label: 'Route To Step', icon: 'directions', color: '#8b5cf6' }
    ];

    return actions.map(a => `
        <option value="${a.value}" ${selectedAction === a.value ? 'selected' : ''}>
            ${a.label}
        </option>
    `).join('');
}
```

**Route To Step Config:**
```javascript
case 'route_to_step':
    container.innerHTML = `<div id="route-step-select-${actionType}-${index}"></div>`;

    setTimeout(async () => {
        const steps = await this.loadPipelineSteps();
        const selectContainer = document.getElementById(`route-step-select-${actionType}-${index}`);

        if (steps.length === 0) {
            selectContainer.innerHTML = `
                <div class="ifthen-warning">
                    <i class="fas fa-exclamation-triangle"></i>
                    No steps available to route to. Add more steps to pipeline first.
                </div>
            `;
        } else {
            selectContainer.innerHTML = `
                <select class="ifthen-select-compact" data-field="stepId" data-action-type="${actionType}" data-index="${index}">
                    <option value="">-- Select Step --</option>
                    ${steps.map(step => `
                        <option value="${step.value}" ${actionData.stepId === step.value ? 'selected' : ''}>
                            ${step.label}
                        </option>
                    `).join('')}
                </select>
            `;
        }
    }, 0);
    break;
```

**Load Pipeline Steps:**
```javascript
async loadPipelineSteps() {
    try {
        // Get current pipeline from PipelineBuilder
        const pipeline = window.pipelineBuilder?.getPipeline();
        if (!pipeline || !pipeline.steps) {
            return [];
        }

        // Get current step's sequence
        const currentStep = this.step;
        const currentSequence = currentStep?.sequence || 0;

        // Filter steps that come AFTER current step
        return pipeline.steps
            .filter(step => step.sequence > currentSequence)
            .map(step => ({
                value: step.id,
                label: `Step ${step.sequence} - ${step.stepName}`,
                sequence: step.sequence
            }))
            .sort((a, b) => a.sequence - b.sequence);
    } catch (err) {
        console.error('Failed to load pipeline steps:', err);
        return [];
    }
}
```

---

## Action Translation Layer

### UI to Backend

```javascript
class IfThenElseActionTranslator {
    static toBackend(uiAction) {
        switch (uiAction.action) {
            case 'continue':
                if (!uiAction.logLevel || uiAction.logLevel === 'none') {
                    return { action: 'continue' };
                }
                return {
                    action: `log_${uiAction.logLevel}`,
                    message: uiAction.message || ''
                };

            case 'stop':
                return {
                    action: 'reject',
                    errorMessage: uiAction.message || 'Processing stopped',
                    severity: uiAction.severity || 'error'
                };

            case 'set_value':
                return this.translateSetValue(uiAction);

            case 'delete_field':
                return {
                    action: 'delete_field',
                    field: uiAction.targetField
                };

            case 'route_to_step':
                return {
                    action: 'route_to_step',
                    stepId: uiAction.stepId
                };

            default:
                return uiAction;
        }
    }

    static translateSetValue(config) {
        const { targetField, valueType, value, sourceField } = config;

        // Copy from another field
        if (valueType === 'copy') {
            return {
                action: 'copy_field',
                source: sourceField,
                target: targetField
            };
        }

        // Set metadata (auto-detect by field prefix)
        if (targetField.startsWith('_metadata.')) {
            const metadataKey = targetField.replace('_metadata.', '');
            return {
                action: 'set_metadata',
                metadata: { [metadataKey]: value }
            };
        }

        // Regular field assignment
        return {
            action: 'set_field',
            field: targetField,
            value: value
        };
    }

    static fromBackend(backendAction) {
        switch (backendAction.action) {
            case 'continue':
                return {
                    action: 'continue',
                    logLevel: 'none',
                    message: ''
                };

            case 'log_info':
            case 'log_warning':
            case 'log_debug':
                return {
                    action: 'continue',
                    logLevel: backendAction.action.replace('log_', ''),
                    message: backendAction.message || ''
                };

            case 'reject':
                return {
                    action: 'stop',
                    message: backendAction.errorMessage,
                    severity: backendAction.severity || 'error'
                };

            case 'set_metadata':
                const metadataKey = Object.keys(backendAction.metadata)[0];
                return {
                    action: 'set_value',
                    targetField: `_metadata.${metadataKey}`,
                    valueType: 'constant',
                    value: backendAction.metadata[metadataKey]
                };

            case 'set_field':
                return {
                    action: 'set_value',
                    targetField: backendAction.field,
                    valueType: 'constant',
                    value: backendAction.value
                };

            case 'copy_field':
                return {
                    action: 'set_value',
                    targetField: backendAction.target,
                    valueType: 'copy',
                    sourceField: backendAction.source
                };

            case 'delete_field':
                return {
                    action: 'delete_field',
                    targetField: backendAction.field
                };

            case 'route_to_step':
                return {
                    action: 'route_to_step',
                    stepId: backendAction.stepId
                };

            default:
                return backendAction;
        }
    }
}
```

---

## Backend Implementation Tasks

### Task 1: Add `route_to_step` Action to Executor

**File:** `services/executors/control/conditional_executor.go`

```go
case "route_to_step":
    stepId := getStringValue(actionMap, "stepId")

    if stepId == "" {
        return fmt.Errorf("stepId is required for route_to_step action")
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

    routingMap["nextStep"] = stepId
    log.Printf("      Set _routing.nextStep = %s (conditional routing)", stepId)
```

### Task 2: Update Execution Engine

**File:** `processing/engine.go`

```go
func (pe *ProcessingEngine) ExecutePipeline(
    ctx context.Context,
    pipeline *models.TransformationPipeline,
    data map[string]interface{},
) (map[string]interface{}, error) {

    steps := pipeline.Steps

    // Execute steps with conditional routing support
    i := 0
    for i < len(steps) {
        step := steps[i]

        log.Printf("📍 Executing Step %d: %s (Type: %s)", step.Sequence, step.StepName, step.StepType)

        // Execute step
        result, err := pe.executeStep(ctx, step, data)
        if err != nil {
            return data, fmt.Errorf("step %d failed: %v", step.Sequence, err)
        }

        data = result

        // Check for conditional routing
        if routing, ok := data["_routing"].(map[string]interface{}); ok {
            if nextStepId, ok := routing["nextStep"].(string); ok {
                // Find step by ID
                nextIndex := pe.findStepIndexById(steps, nextStepId)
                if nextIndex >= 0 {
                    log.Printf("🔀 Conditional routing: Jumping to Step %d (ID: %s)", steps[nextIndex].Sequence, nextStepId)
                    i = nextIndex

                    // Clear routing directive after use
                    delete(routing, "nextStep")
                    continue
                } else {
                    log.Printf("⚠️  Warning: Step ID %s not found, continuing sequentially", nextStepId)
                }
            }
        }

        // Normal sequential execution
        i++
    }

    return data, nil
}

func (pe *ProcessingEngine) findStepIndexById(steps []*models.TransformationStep, stepId string) int {
    for i, step := range steps {
        if step.ID == stepId {
            return i
        }
    }
    return -1
}
```

---

## Example Execution Flow

### Pipeline Configuration

```
Step 10  - Receive Message
Step 20  - Parse HL7
Step 50  - If-Then-Else: Age Check
Step 100 - Standard Validation
Step 150 - Standard Enrichment
Step 200 - Geriatric Enrichment
Step 250 - Geriatric Validation
Step 900 - FHIR Transformation
```

### Condition Configuration (Step 50)

```json
{
  "conditions": [
    {
      "name": "Age-Based Routing",
      "condition": {
        "field": "patient.age",
        "operator": "greater_than",
        "value": "65"
      },
      "onTrue": {
        "action": "route_to_step",
        "stepId": "step-uuid-200"
      },
      "onFalse": {
        "action": "continue"
      }
    }
  ]
}
```

### Execution Log (Patient Age = 70)

```
📍 Executing Step 10: Receive Message
📍 Executing Step 20: Parse HL7
📍 Executing Step 50: If-Then-Else: Age Check
   🔀 [IfThenElse] Condition evaluated: true
   Executing 1 THEN actions
   [1] Action: route_to_step
      Set _routing.nextStep = step-uuid-200 (conditional routing)
🔀 Conditional routing: Jumping to Step 200 (ID: step-uuid-200)
📍 Executing Step 200: Geriatric Enrichment
📍 Executing Step 250: Geriatric Validation
📍 Executing Step 900: FHIR Transformation
```

**Steps 100 and 150 were skipped!**

---

## Implementation Timeline

| Task | Time | Priority |
|------|------|----------|
| 1. Add `route_to_step` action to backend | 30 min | HIGH |
| 2. Update execution engine with routing | 1 hour | HIGH |
| 3. Create action translator | 1 hour | HIGH |
| 4. Update IfThenElseBuilder UI | 2 hours | HIGH |
| 5. Test step routing | 1 hour | HIGH |
| **Total** | **5.5 hours** | |

---

## Questions for User

### 1. Routing Direction
**Should we allow routing to ANY step or only FORWARD steps?**

- **Forward Only (Recommended):** Prevents infinite loops, safer
- **Any Step:** More flexible, but can cause loops

### 2. Multiple Conditions
**If multiple conditions route to different steps, which wins?**

```
Condition 1: IF age > 65 THEN Route To Step 200
Condition 2: IF vip = true THEN Route To Step 250
```

If patient is 70 AND vip, which step?

**Options:**
- A. First condition wins
- B. Last condition wins
- C. Error if multiple routes

### 3. Default Behavior
**If ELSE action is "Continue", should it proceed to next step in sequence?**

- **Yes (Recommended):** Natural flow
- **No:** Must explicitly route

---

## Next Steps

1. ✅ User approves step routing design
2. ⏳ Implement `route_to_step` backend action
3. ⏳ Update execution engine with routing logic
4. ⏳ Create action translator
5. ⏳ Update UI with 5 consolidated actions
6. ⏳ Test conditional routing

---

**Status:** 📋 READY TO IMPLEMENT
**User Approved:** Step-level routing focus
**Timeline:** 5.5 hours

---

**Created By:** Claude Code
**Date:** December 28, 2025
