# Parallel Execution Design for Transformation Pipelines

**Your Requirement**: "Support executing steps in parallel or in sequence within the same pipeline - meaning 2 steps can execute in parallel and then move to 3rd which is sequential"

**Perfect Example**:
```
Step 1: Validate Required Fields (sequential - alone)
    ↓
Step 2A: Call EMPI API       ⟍
Step 2B: Call Lab System API  ⟼  Run these 3 in PARALLEL
Step 2C: Check Insurance DB   ⟋
    ↓
Step 3: HL7→FHIR Mapping (sequential - waits for all 3 to complete)
```

---

## Current State

### ✅ Database Model SUPPORTS This
```go
// models/transformation_models.go:39
ExecutionMode string `json:"execution_mode" db:"execution_mode"` // sequential, parallel
```

### ❌ Execution Logic DOESN'T USE It
Current code executes everything **sequentially only** - the `ExecutionMode` field is stored but **never read**!

---

## Proposed Design (Simple & Powerful)

### Rule: Steps with Same Sequence Number = Parallel

**Simple**:
```
Sequence 10: Validate (alone)          → Runs alone (sequential)
Sequence 20: API Call A                ⟍
Sequence 20: API Call B                ⟼ All run at same time (parallel)
Sequence 20: Database Lookup           ⟋
Sequence 30: HL7→FHIR Mapping (alone)  → Runs alone (sequential)
```

**No complex configuration needed** - just assign the same sequence number!

---

## Data Flow

### Sequential Steps
```
Input → Step 1 → Output becomes Input to Step 2 → Output becomes Input to Step 3
```

### Parallel Steps
```
        ┌─→ Step 2A (EMPI API) ──→ {empi_data: {...}}
Input ──┼─→ Step 2B (Lab API) ───→ {lab_results: {...}}
        └─→ Step 2C (Insurance) ─→ {insurance: {...}}
                ↓
        Merge all outputs together
                ↓
        Combined output becomes input to Step 3
```

**Example**:
```javascript
// Input to parallel group (sequence 20):
{
  "patient_id": "12345",
  "message": {...}
}

// After Step 2A completes:
{
  "patient_id": "12345",
  "message": {...},
  "empi_data": {...}  // Added by EMPI API
}

// After Step 2B completes:
{
  "patient_id": "12345",
  "message": {...},
  "lab_results": {...}  // Added by Lab API
}

// After Step 2C completes:
{
  "patient_id": "12345",
  "message": {...},
  "insurance": {...}  // Added by Insurance lookup
}

// Merged result (input to sequence 30):
{
  "patient_id": "12345",
  "message": {...},
  "empi_data": {...},     // From 2A
  "lab_results": {...},   // From 2B
  "insurance": {...}      // From 2C
}
```

---

## Implementation (Go Goroutines)

### Algorithm
```go
func ExecutePipeline(steps []Step, inputData map[string]interface{}) {
    // 1. Group steps by sequence number
    stepGroups := groupBySequence(steps)
    // Result: [[step1], [step2A, step2B, step2C], [step3]]

    currentData := inputData

    // 2. Execute each group
    for _, group := range stepGroups {
        if len(group) == 1 {
            // Single step = sequential
            currentData = executeStep(group[0], currentData)
        } else {
            // Multiple steps = parallel
            results := executeParallel(group, currentData)
            currentData = mergeResults(currentData, results)
        }
    }
}

func executeParallel(steps []Step, input map[string]interface{}) []map[string]interface{} {
    results := make(chan map[string]interface{}, len(steps))

    // Launch goroutines for each step
    for _, step := range steps {
        go func(s Step) {
            output := executeStep(s, input) // Each gets same input
            results <- output
        }(step)
    }

    // Wait for all to complete
    outputs := []map[string]interface{}{}
    for i := 0; i < len(steps); i++ {
        outputs = append(outputs, <-results)
    }

    return outputs
}
```

---

## Real-World Use Case

### Patient Registration Pipeline

**Without Parallel** (Sequential):
```
10. Validate Patient ID       → 50ms
20. Call EMPI API              → 300ms
30. Call Lab System            → 500ms
40. Call Insurance DB          → 400ms
50. HL7→FHIR Mapping           → 100ms
───────────────────────────────────────
Total: 1,350ms
```

**With Parallel** (Your Requirement):
```
10. Validate Patient ID                → 50ms
20. Call EMPI API        ⟍
20. Call Lab System       ⟼             → max(300, 500, 400) = 500ms
20. Call Insurance DB    ⟋
30. HL7→FHIR Mapping                   → 100ms
───────────────────────────────────────
Total: 650ms ✅ 2x FASTER!
```

---

## Error Handling

**Question**: If one parallel step fails, what happens?

**Option 1: Fail Fast** (Recommended)
- If ANY parallel step fails → stop everything, fail pipeline

**Option 2: Continue on Error**
- If step fails → log error, continue with empty output
- Controlled by `OnErrorStrategy` field (already in model!)

**Example**:
```javascript
{
  step_name: "Call EMPI API",
  sequence: 20,
  on_error_strategy: "skip"  // Continue even if this fails
}

{
  step_name: "Validate Required Fields",
  sequence: 10,
  on_error_strategy: "fail"  // Stop pipeline if this fails
}
```

---

## UI Implementation

### Creating Parallel Steps (Option 1: Simple)

**User Experience**:
1. Drag "API Enrichment" step onto canvas → Auto-assigned sequence 20
2. Drag another "API Enrichment" step → Auto-assigned sequence 30
3. User edits second step's sequence from 30 to 20 in properties panel
4. **UI automatically shows parallel grouping!**

### Visual Indicator
```
Pipeline Builder:

  [10] Validate Required Fields
    ↓
  ╔═══════════════════════════════╗
  ║ [20] Call EMPI API            ║  ← Parallel bracket
  ║ [20] Call Lab System          ║
  ║ [20] Check Insurance          ║
  ╚═══════════════════════════════╝
    ↓
  [30] HL7→FHIR Mapping
```

---

## Timeline

**Phase 1: Backend Implementation** (1 day)
- Step grouping by sequence
- Parallel execution with goroutines
- Output merging logic
- Error handling

**Phase 2: UI Visual Indicators** (0.5 days)
- Detect parallel groups (same sequence)
- Show visual bracket/grouping
- Add tooltip: "These steps run in parallel"

**Phase 3: Testing** (0.5 days)
- Test sequential execution (current behavior)
- Test parallel execution (2-3 steps)
- Test error handling
- Performance benchmark

**Total**: ~2 days

---

## Questions for You

1. **Error Handling**: If one parallel step fails, should the pipeline:
   - **A)** Stop immediately (fail fast)
   - **B)** Let all parallel steps finish, then fail
   - **C)** Configurable per step via `OnErrorStrategy`

2. **Data Conflicts**: If two parallel steps write to the same field, should:
   - **A)** Last one wins (overwrite)
   - **B)** Error on conflict
   - **C)** Each step writes to namespaced field (e.g., `empi_data`, `lab_data`)

3. **UI**: How should users create parallel steps:
   - **A)** Manually edit sequence numbers (simple)
   - **B)** Drag-and-drop with "run in parallel" checkbox
   - **C)** Both

---

## My Recommendation

**YES - Implement This!**

**Why**:
- ✅ Model already supports it (`ExecutionMode` field exists)
- ✅ Real-world performance gain (2-3x faster for API-heavy pipelines)
- ✅ Matches your exact requirement
- ✅ Industry standard pattern (Apache Camel, AWS Step Functions, etc.)
- ✅ Simple implementation (~2 days)
- ✅ Huge value for users

**Should I proceed with implementation?**
