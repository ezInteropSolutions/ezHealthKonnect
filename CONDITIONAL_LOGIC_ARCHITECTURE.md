# Conditional Logic Architecture - Two Use Cases

**Date:** December 28, 2025
**Status:** 🔍 ANALYSIS - User Feedback

---

## User Insight: Two Types of Conditional Logic

The user correctly identified that there are **TWO distinct use cases** for conditional logic in the transformation pipeline:

### Use Case 1: **Conditional Variables** ✅ (Current Implementation)
**Purpose:** Set variables, metadata, or field values based on conditions
**Flow:** Always continues through all pipeline steps
**Scope:** Data manipulation within a single step

**Examples:**
```javascript
// Example 1: Set priority based on age
IF patient.age > 65
  THEN set metadata.priority = "high"
  ELSE set metadata.priority = "normal"

// Example 2: Gender code normalization
IF PID.8 == "M"
  THEN set patient.gender = "male"
  ELSE set patient.gender = "female"

// Example 3: VIP flagging
IF patient.notes contains "VIP"
  THEN set metadata.routing = "vip-care"
  ELSE continue
```

**Current Implementation:**
- ✅ Backend: [services/executors/control/conditional_executor.go](services/executors/control/conditional_executor.go)
- ⏳ UI: IfThenElseBuilder (integration in progress)
- ✅ 11 Operators: equals, not_equals, greater_than, less_than, contains, regex, is_empty, etc.
- ✅ 9 Actions: set_metadata, set_field, copy_field, delete_field, log_warning, reject, etc.

---

### Use Case 2: **Conditional Flow Control** ❌ (NOT YET IMPLEMENTED)
**Purpose:** Control which pipeline steps execute next
**Flow:** Branch to different execution paths
**Scope:** Pipeline-level flow control

**Examples:**
```javascript
// Example 1: Message type routing
IF message_type == "ADT^A01"
  THEN execute steps 100-200 (admission flow)
  ELSE IF message_type == "ADT^A03"
    THEN execute steps 300-400 (discharge flow)
  ELSE
    execute steps 500-600 (default flow)

// Example 2: Priority routing
IF metadata.priority == "urgent"
  THEN skip validation, go directly to delivery
  ELSE execute normal validation pipeline

// Example 3: Conditional enrichment
IF patient.age > 65
  THEN execute geriatric enrichment steps (200-250)
  ELSE skip to standard processing (300)
```

**Not Yet Implemented:**
- ❌ No backend executor for flow control
- ❌ No UI for defining flow branches
- ❌ No step dependency/routing system

---

## Architectural Differences

| Aspect | Conditional Variables | Conditional Flow Control |
|--------|----------------------|--------------------------|
| **Execution** | Step executes, data changes | Step routing changes |
| **Flow** | Linear (all steps run) | Branching (some steps skip) |
| **Actions** | set_field, set_metadata, reject | goto, skip, execute_branch |
| **Scope** | Single step | Multiple steps |
| **Complexity** | Low | High |
| **Backend** | Single executor | Requires flow engine |

---

## Implementation Status

### ✅ Use Case 1: Conditional Variables
**Status:** Backend complete, UI integration in progress

**Backend (Complete):**
```go
// services/executors/control/conditional_executor.go
type IfThenElseExecutor struct {
    *executors.BaseExecutor
}

func (e *IfThenElseExecutor) Execute(data map[string]interface{}, config map[string]interface{}) (map[string]interface{}, error) {
    conditions := config["conditions"].([]interface{})

    for _, cond := range conditions {
        condition := cond.(map[string]interface{})
        result, _ := e.evaluateCondition(condition["condition"], data)

        if result {
            e.executeActions(condition["onTrue"], data)
        } else {
            e.executeActions(condition["onFalse"], data)
        }
    }

    return data, nil // Always continue to next step
}
```

**UI (In Progress):**
- ✅ Component created: [IfThenElseBuilder.js](public/js/pipeline/components/IfThenElseBuilder.js)
- ⏳ Integration: PropertiesPanel detection not working (debugging)
- ✅ Visual builder with 11 operators, 9 actions
- ✅ Cross-field comparison support
- ✅ Color theme compliance (navy blue, pastel pink)

**Issue:** User screenshot shows old Configuration form, not visual builder. Likely a detection issue with `stepType` or `templateId`.

---

### ❌ Use Case 2: Conditional Flow Control
**Status:** Not implemented, requires new architecture

**Requirements:**

1. **Step Dependencies:**
   - Define which steps depend on which
   - Example: Step 200 only runs if Step 100 sets `metadata.urgent = true`

2. **Conditional Branching:**
   - Define multiple execution paths
   - Example: IF urgent THEN steps [200, 210, 220] ELSE steps [300, 310, 320]

3. **Flow Control Actions:**
   - `goto_step`: Jump to specific step
   - `skip_steps`: Skip range of steps
   - `execute_branch`: Execute named branch
   - `stop_pipeline`: Halt execution

4. **Execution Engine Changes:**
   - Current: Sequential execution (step 1 → step 2 → step 3)
   - Needed: Conditional execution with routing table

**Proposed Backend Architecture:**

```go
// services/executors/control/flow_control_executor.go
type FlowControlExecutor struct {
    *executors.BaseExecutor
}

type FlowBranch struct {
    Condition  Condition   `json:"condition"`
    StepIDs    []string    `json:"step_ids"`    // Steps to execute if true
}

type FlowControlConfig struct {
    Branches      []FlowBranch `json:"branches"`
    DefaultBranch []string     `json:"default_branch"`
}

func (e *FlowControlExecutor) Execute(data map[string]interface{}, config map[string]interface{}) (map[string]interface{}, error) {
    flowConfig := parseFlowControlConfig(config)

    // Evaluate branches
    for _, branch := range flowConfig.Branches {
        if e.evaluateCondition(branch.Condition, data) {
            // Set execution plan in metadata
            data["_executionPlan"] = branch.StepIDs
            return data, nil
        }
    }

    // Default branch
    data["_executionPlan"] = flowConfig.DefaultBranch
    return data, nil
}
```

**Execution Engine Changes:**

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

        // Check if step set execution plan
        if executionPlan, ok := data["_executionPlan"].([]string); ok {
            // Find next step by ID from execution plan
            nextStep := findStepByID(steps, executionPlan[0])
            i = indexOf(steps, nextStep) - 1 // -1 because loop will i++

            // Remove first step from plan
            data["_executionPlan"] = executionPlan[1:]
            delete(data, "_executionPlan") // Clean up if empty
        }
    }

    return nil
}
```

**UI Requirements:**

1. **Flow Diagram:**
   - Visual representation of branches
   - Drag-and-drop step assignment to branches
   - Condition builder for each branch

2. **Branch Configuration:**
   - Name branches (e.g., "Urgent Path", "Standard Path")
   - Assign steps to each branch
   - Define default branch

3. **Visual Indicators:**
   - Show which branch each step belongs to
   - Highlight active execution path during testing
   - Show step dependencies

---

## Recommended Approach

### Short-Term (Fix Current Implementation)
1. **Debug UI Integration:**
   - Add console logging to PropertiesPanel.createDynamicFormFields
   - Check actual `stepType` and `templateId` values
   - Fix condition to match actual values
   - Verify IfThenElseBuilder.js is loading

2. **Complete Conditional Variables:**
   - Get visual builder working
   - Test all 11 operators and 9 actions
   - Document examples for users
   - User testing and feedback

**Timeline:** 1-2 hours

---

### Medium-Term (Add Flow Control)
1. **Design Flow Control Architecture:**
   - Define step dependency model
   - Design execution routing mechanism
   - Plan backward compatibility

2. **Implement Backend:**
   - Create FlowControlExecutor
   - Modify execution engine for routing
   - Add step dependency resolver

3. **Implement UI:**
   - Create FlowBranchBuilder component
   - Visual flow diagram
   - Branch assignment interface

**Timeline:** 1-2 weeks

---

### Long-Term (Advanced Features)
1. **Visual Flow Designer:**
   - Drag-and-drop flowchart interface
   - Automatic step dependency detection
   - Flow validation and simulation

2. **Conditional Execution Patterns:**
   - Pre-built flow templates (urgent path, vip path, etc.)
   - Import/export flow configurations
   - A/B testing support

**Timeline:** 1-2 months

---

## Decision Points

### Question 1: Do we need both types of conditional logic?

**Answer:** YES

**Reason:**
- **Conditional Variables** needed for data manipulation (90% of use cases)
- **Conditional Flow Control** needed for complex routing (10% of use cases)
- Cannot replace one with the other - different purposes

### Question 2: Should we implement flow control now or later?

**Options:**

**Option A: Now (Parallel Development)**
- Pro: Complete solution faster
- Pro: Users can leverage both immediately
- Con: Complexity - two systems at once
- Con: Higher risk of bugs

**Option B: Later (Sequential Development)**
- Pro: Finish conditional variables first (simpler)
- Pro: Learn from user feedback before flow control
- Pro: Lower risk
- Con: Users wait longer for flow control

**Recommendation:** **Option B (Later)** - Finish conditional variables first, then add flow control based on user demand.

### Question 3: How should we name these two features to avoid confusion?

**Current:**
- "If-Then-Else" step (ambiguous - could mean either)
- User confusion: "there are 2 levels i can see using if"

**Proposed Names:**

| Feature | Current Name | Proposed Name | Icon |
|---------|--------------|---------------|------|
| Conditional Variables | If-Then-Else | **Conditional Data** | 🔄 |
| Conditional Flow Control | (not implemented) | **Conditional Routing** | 🔀 |

**Alternative Names:**
- Conditional Variables → **Data Rules**
- Conditional Flow Control → **Flow Branching**

---

## User Feedback Analysis

**User Quote:**
> "there are 2 levels i can see using if, one where we probable want to set conditional variables depending on the flow, other where we want to control the flow of the steps, so if this scenario take this route else this route"

**Analysis:**
1. ✅ User correctly identified two distinct use cases
2. ✅ User understands difference between data manipulation and flow control
3. ❌ Current implementation only handles one use case (conditional variables)
4. ❌ UI not showing correctly (screenshot shows old form)

**Priority Actions:**
1. **Immediate:** Fix UI integration so user can see visual builder
2. **Short-term:** Complete conditional variables implementation
3. **Medium-term:** Design and implement flow control
4. **Long-term:** Unified visual flow designer

---

## Next Steps

### 1. Debug UI Integration (IMMEDIATE)
- Add console logging to createDynamicFormFields
- User opens If-Then-Else step and shares console output
- Identify why condition `(stepType === 'pre.logic' && step.templateId === 'if-then-else')` fails
- Fix detection logic

### 2. Complete Conditional Variables (TODAY)
- Get visual builder appearing
- Test all operators and actions
- User acceptance testing

### 3. Discuss Flow Control (THIS WEEK)
- Review proposed architecture
- Decide: Now or Later?
- Prioritize based on user needs

---

## Questions for User

1. **UI Integration:**
   - Can you open browser console (F12) and share what you see when opening If-Then-Else step?
   - This will show us why the visual builder isn't appearing

2. **Use Cases:**
   - Which use case is more important to you right now?
     - A. Conditional variables (set metadata, normalize values)
     - B. Conditional flow control (route to different steps)

3. **Timeline:**
   - Do you need flow control immediately, or can we finish conditional variables first?

4. **Naming:**
   - Do you like the proposed names?
     - "Conditional Data" vs "Conditional Routing"
   - Or do you prefer different terminology?

---

**Status:** 🔍 AWAITING USER FEEDBACK
**Next Action:** Debug UI integration with console logs

---

**Created By:** Claude Code
**Date:** December 28, 2025
