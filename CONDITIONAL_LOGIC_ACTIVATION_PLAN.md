# Conditional Logic Activation Plan

**Date:** December 27, 2025
**Status:** 📋 READY TO ACTIVATE
**Effort:** Low (2-3 hours)

---

## Overview

The system already has a **complete conditional logic implementation** that's currently disabled. We need to:
1. Enable the existing executors
2. Add cross-field comparison support
3. Register in executor registry
4. Update UI templates

---

## Current State

### ✅ What Exists (Disabled):
File: `services/executor_conditional_logic.go.disabled`

**Three Executors Implemented:**
1. **IfThenElseExecutor** - Basic conditional logic
2. **SwitchCaseExecutor** - Multi-way branching
3. **FilterExecutor** - Filter/block messages

**Supported Operators:**
- `equals`, `not_equals`
- `contains`, `starts_with`, `ends_with`
- `greater_than`, `less_than`
- `exists`, `not_exists`
- `regex_match`
- `in_list`

**Supported Actions:**
- `set_value` - Set field to constant value
- `copy_field` - Copy field to another field
- `delete_field` - Remove field
- `transform` - Apply transformation (uppercase, lowercase, trim)

### ❌ What's Missing:

1. **Cross-Field Comparison:**
   - Current: `field` compared to `value` (constant)
   - Needed: `field1` compared to `field2` (another field)
   - Example: `PV1.45` (discharge) > `PV1.44` (admit)

2. **Additional Actions:**
   - `reject` - Stop processing, return error
   - `log_warning` / `log_error` - Log message, optionally continue
   - `set_metadata` - Add metadata to message
   - `route_to` - Route to specific destination

3. **New Executor Interface:**
   - Current: Old interface without `BaseExecutor`
   - Needed: Update to use `executors.BaseExecutor` pattern

4. **Registration:**
   - Not registered in `executor_registry.go`

---

## Implementation Plan

### Phase 1: Enable Existing Code (30 minutes)

#### Step 1.1: Rename File
```bash
mv services/executor_conditional_logic.go.disabled services/executors/control/conditional_executor.go
```

#### Step 1.2: Update Package and Imports
Change package from `services` to `control`:
```go
package control

import (
    "context"
    "ezhealthkonnect/models"
    "ezhealthkonnect/services/executors"
    "fmt"
    "regexp"
    "strings"
    "time"
)
```

#### Step 1.3: Register in Executor Registry
In `services/executor_registry.go`, add:
```go
import "ezhealthkonnect/services/executors/control"

// In autoRegisterExecutors():
// Control flow executors
er.Register(control.NewIfThenElseExecutor())
er.Register(control.NewSwitchCaseExecutor())
er.Register(control.NewFilterExecutor())
```

### Phase 2: Add Cross-Field Comparison (45 minutes)

#### Step 2.1: Enhanced Condition Structure
Update condition parsing to support both constant and field comparisons:

**Current:**
```json
{
  "field": "patient.age",
  "operator": "greater_than",
  "value": 65
}
```

**Enhanced:**
```json
{
  "field": "patient.age",
  "operator": "greater_than",
  "value": 65,              // Constant comparison
  "compareToField": null     // OR field comparison
}

// Cross-field example:
{
  "field": "PV1.45",         // Discharge date
  "operator": "greater_than",
  "compareToField": "PV1.44", // Admit date
  "value": null
}
```

#### Step 2.2: Update evaluateCondition()
```go
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
    } else {
        // Constant comparison
        compareValue = condition["value"]
    }

    // Rest of comparison logic...
    switch operator {
    case "equals":
        return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", compareValue), nil
    // ... etc
    }
}
```

### Phase 3: Add New Actions (45 minutes)

#### Step 3.1: Reject Action
```go
case "reject":
    errorMessage := getStringValue(actionMap, "errorMessage")
    if errorMessage == "" {
        errorMessage = "Condition validation failed"
    }
    return outputData, fmt.Errorf("REJECT: %s", errorMessage)
```

#### Step 3.2: Log Actions
```go
case "log_warning":
    message := getStringValue(actionMap, "message")
    log.Printf("⚠️  [IfThenElse] WARNING: %s", message)
    continueProcessing := getBoolValue(actionMap, "continue", true)
    if !continueProcessing {
        return outputData, fmt.Errorf("Processing stopped after warning: %s", message)
    }

case "log_error":
    message := getStringValue(actionMap, "message")
    log.Printf("❌ [IfThenElse] ERROR: %s", message)
    continueProcessing := getBoolValue(actionMap, "continue", false)
    if !continueProcessing {
        return outputData, fmt.Errorf("Processing stopped after error: %s", message)
    }
```

#### Step 3.3: Set Metadata Action
```go
case "set_metadata":
    metadata, ok := actionMap["metadata"].(map[string]interface{})
    if !ok {
        return outputData, fmt.Errorf("metadata must be an object")
    }

    // Ensure _metadata exists
    if _, exists := outputData["_metadata"]; !exists {
        outputData["_metadata"] = make(map[string]interface{})
    }

    metadataMap := outputData["_metadata"].(map[string]interface{})
    for key, value := range metadata {
        metadataMap[key] = value
    }
```

#### Step 3.4: Route To Action
```go
case "route_to":
    destination := getStringValue(actionMap, "destination")
    queue := getStringValue(actionMap, "queue")

    // Ensure _routing exists
    if _, exists := outputData["_routing"]; !exists {
        outputData["_routing"] = make(map[string]interface{})
    }

    routingMap := outputData["_routing"].(map[string]interface{})
    if destination != "" {
        routingMap["destination"] = destination
    }
    if queue != "" {
        routingMap["queue"] = queue
    }
```

### Phase 4: Modernize to BaseExecutor Pattern (30 minutes)

#### Step 4.1: Add Constructor with BaseExecutor
```go
func NewIfThenElseExecutor() *IfThenElseExecutor {
    metadata := models.ExecutorMetadata{
        Name:        "If-Then-Else",
        Description: "Conditional execution with if/then/else logic",
        Version:     "2.0.0",
        Author:      "ezHealthKonnect",
        Category:    "control",
    }

    base := executors.NewBaseExecutor("pre.logic", metadata)

    return &IfThenElseExecutor{
        BaseExecutor: base,
    }
}

type IfThenElseExecutor struct {
    *executors.BaseExecutor
}
```

#### Step 4.2: Add Pre/Post Execute Hooks
```go
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

    // ... existing logic ...

    // Post-execution tracking
    e.PostExecute(ctx, step, nil, time.Since(start))
    return outputData, nil
}
```

### Phase 5: Update UI Templates (15 minutes)

#### Step 5.1: Update If-Then-Else Template
In `ToolboxManager.js`:
```javascript
new StepTemplate({
    id: 'if-then-else',
    name: 'If-Then-Else',
    type: 'pre.logic',
    description: 'Conditional execution based on field comparisons. Supports cross-field validation, routing, and metadata.',
    layer: 'pre',
    icon: this.getIconForType('pre.logic'),
    isSystem: true,
    defaultConfig: {
        condition: {
            field: 'patient.age',
            operator: 'greater_than',
            value: 65,
            compareToField: null  // Or use field name for cross-field comparison
        },
        then_actions: [
            { action: 'set_metadata', metadata: { priority: 'high', routing: 'geriatrics' } },
            { action: 'log_warning', message: 'Elderly patient detected', continue: true }
        ],
        else_actions: [
            { action: 'set_metadata', metadata: { priority: 'normal' } }
        ]
    }
}),
```

#### Step 5.2: Add Cross-Field Validation Example
Add a new example config:
```javascript
// Example: Cross-field date validation
{
    condition: {
        field: 'PV1.45',           // Discharge date
        operator: 'greater_than',
        compareToField: 'PV1.44'   // Admit date
    },
    then_actions: [
        { action: 'continue' }
    ],
    else_actions: [
        {
            action: 'reject',
            errorMessage: 'Discharge date must be after admit date',
            severity: 'error'
        }
    ]
}
```

---

## Testing Plan

### Test 1: Basic If-Then-Else (Constant Comparison)
```json
{
  "step_type": "pre.logic",
  "config": {
    "condition": {
      "field": "patient.age",
      "operator": "greater_than",
      "value": 65
    },
    "then_actions": [
      { "action": "set_field", "field": "priority", "value": "high" }
    ],
    "else_actions": [
      { "action": "set_field", "field": "priority", "value": "normal" }
    ]
  }
}
```

**Expected:**
- If age > 65 → priority = "high"
- If age ≤ 65 → priority = "normal"

### Test 2: Cross-Field Validation (Reject Action)
```json
{
  "step_type": "pre.logic",
  "config": {
    "condition": {
      "field": "PV1.45",
      "operator": "greater_than",
      "compareToField": "PV1.44"
    },
    "then_actions": [
      { "action": "continue" }
    ],
    "else_actions": [
      { "action": "reject", "errorMessage": "Discharge must be after admit" }
    ]
  }
}
```

**Expected:**
- If discharge > admit → continue processing
- If discharge ≤ admit → reject message with error

### Test 3: Routing Based on Condition
```json
{
  "step_type": "pre.logic",
  "config": {
    "condition": {
      "field": "patient.age",
      "operator": "greater_than",
      "value": 65
    },
    "then_actions": [
      {
        "action": "set_metadata",
        "metadata": {
          "priority": "high",
          "routing": "geriatrics"
        }
      }
    ],
    "else_actions": [
      { "action": "set_metadata", "metadata": { "priority": "normal" } }
    ]
  }
}
```

**Expected:**
- If age > 65 → _metadata.priority = "high", _metadata.routing = "geriatrics"
- If age ≤ 65 → _metadata.priority = "normal"

---

## Implementation Checklist

### Phase 1: Enable Existing Code
- [ ] Rename file: `executor_conditional_logic.go.disabled` → `executors/control/conditional_executor.go`
- [ ] Update package to `control`
- [ ] Update imports
- [ ] Create directory: `services/executors/control/`
- [ ] Register in `executor_registry.go`
- [ ] Build and test

### Phase 2: Add Cross-Field Comparison
- [ ] Update condition structure to support `compareToField`
- [ ] Modify `evaluateCondition()` to handle both constant and field comparisons
- [ ] Add helper function `resolveComparisonValue()`
- [ ] Test cross-field comparisons

### Phase 3: Add New Actions
- [ ] Implement `reject` action
- [ ] Implement `log_warning` action
- [ ] Implement `log_error` action
- [ ] Implement `set_metadata` action
- [ ] Implement `route_to` action
- [ ] Test all actions

### Phase 4: Modernize to BaseExecutor
- [ ] Add `BaseExecutor` to struct
- [ ] Create `New*` constructors
- [ ] Add `PreExecute` / `PostExecute` calls
- [ ] Update `GetStepType()` to use base
- [ ] Test executor lifecycle

### Phase 5: Update UI
- [ ] Update If-Then-Else template description
- [ ] Add cross-field validation example
- [ ] Update Switch/Case template
- [ ] Add Filter template examples
- [ ] Test UI rendering

### Phase 6: Documentation
- [ ] Update STEP_TEMPLATE_RECOMMENDATIONS.md
- [ ] Create CONDITIONAL_LOGIC_USER_GUIDE.md
- [ ] Update executor count in SYSTEM_DOCUMENTATION.md
- [ ] Add examples to README

---

## Expected Outcome

### Before Activation: 9 executors
1. PassthroughExecutor
2. FieldValidationExecutor
3. APIEnrichmentExecutor
4. DatabaseEnrichmentExecutor
5. ScriptEnrichmentExecutor
6. HL7FHIRMappingExecutor
7. FieldMappingExecutor
8. FHIRValidationExecutor
9. GenericExecutor

### After Activation: 12 executors
1. PassthroughExecutor
2. FieldValidationExecutor
3. APIEnrichmentExecutor
4. DatabaseEnrichmentExecutor
5. ScriptEnrichmentExecutor
6. HL7FHIRMappingExecutor
7. FieldMappingExecutor
8. FHIRValidationExecutor
9. GenericExecutor
10. **IfThenElseExecutor** ✨ NEW
11. **SwitchCaseExecutor** ✨ NEW
12. **FilterExecutor** ✨ NEW

---

## Benefits

✅ **Conditional Logic Enabled** - Full if/then/else support
✅ **Cross-Field Validation** - Compare fields to each other
✅ **Routing Support** - Route messages based on conditions
✅ **Rejection Logic** - Fail messages that don't meet criteria
✅ **Metadata Enrichment** - Add flags/routing info based on conditions
✅ **Flexible Actions** - reject, log, set_metadata, route_to, set_field, copy_field
✅ **Multi-Way Branching** - Switch/case for complex routing
✅ **Filtering** - Block/pass messages based on conditions

---

## Risk Assessment

**Risk Level:** 🟢 LOW

**Reasons:**
- Code already implemented and tested (just disabled)
- No changes to existing executors
- Additive only (no breaking changes)
- UI templates already exist
- Can be disabled if issues arise

---

## Timeline

**Total Effort:** 2-3 hours

- Phase 1: 30 minutes (enable existing code)
- Phase 2: 45 minutes (cross-field comparison)
- Phase 3: 45 minutes (new actions)
- Phase 4: 30 minutes (modernize to BaseExecutor)
- Phase 5: 15 minutes (update UI)
- Testing: 15 minutes

---

## Next Steps

**User Decision Required:**
1. ✅ Approve activation plan
2. ⏭️ Implement Phase 1 (enable existing code)
3. ⏭️ Implement Phase 2 (cross-field comparison)
4. ⏭️ Implement Phase 3 (new actions)
5. ⏭️ Test and verify

**Ready to proceed?** Say "go ahead" and I'll start with Phase 1!

---

**Status:** 📋 AWAITING USER APPROVAL
**Priority:** 🔴 HIGH (replaces cross-field validation, enables routing)
**Effort:** 🟢 LOW (2-3 hours)
