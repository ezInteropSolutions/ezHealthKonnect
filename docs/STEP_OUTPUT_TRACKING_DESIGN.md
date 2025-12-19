# Step-Specific Output Tracking & Data Flow Design

**Date**: December 15, 2025
**Status**: Design Phase - Fresh Implementation (No Backward Compatibility)
**Problem**: Current pipeline execution doesn't clearly track step-specific outputs or make them accessible to subsequent steps

---

## Problem Analysis

### Current Issues

1. **Output Overwriting** (Line 294 in transformation_pipeline_service.go)
   ```go
   currentData = outputData
   result.OutputData = outputData  // ⚠️ Overwrites previous step outputs
   ```
   - Each step overwrites the entire `OutputData`
   - Previous step outputs are lost
   - Can't reference data from "Enrich EMPI API" step in "Map FHIR" step

2. **Unclear Step Attribution**
   ```json
   {
     "transformation_log": [
       {
         "step_name": "Validate Patient ID",
         "success": true,
         "input_snapshot": {...},   // ⚠️ Full message (5000+ lines)
         "output_snapshot": {...}   // ⚠️ Full message (5000+ lines)
       }
     ]
   }
   ```
   - Input/Output snapshots contain ENTIRE message
   - No clear "what did THIS step produce"
   - Difficult to spot step-specific data points

3. **No Step Output Namespace**
   - Steps can't explicitly store their outputs in a named location
   - No way to reference "Enrich EMPI API step's response" later
   - All modifications happen on the same global data object

---

## Proposed Solution: Step Output Namespacing

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Pipeline Execution Context                                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  message: {                                                  │
│    raw: "MSH|^~\&|...",        ← Original raw message      │
│    parsed: {...},               ← Parsed HL7 structure      │
│    enhanced: {...}              ← Enhanced schema           │
│  }                                                           │
│                                                              │
│  stepOutputs: {                  ← NEW: Step-specific data  │
│    "Validate_Patient_ID_a3f8e2": {                          │
│      status: "valid",                                        │
│      validatedFields: ["PID.3", "PID.5"],                   │
│      duration_ms: 45                                         │
│    },                                                        │
│    "Enrich_EMPI_API_b4c9f1": {                              │
│      apiResponse: {...},        ← Full API response         │
│      enrichedFields: 74,                                     │
│      cacheHit: false,                                        │
│      responseStatus: 200                                     │
│    },                                                        │
│    "Map_HL7_to_FHIR_d5e2a3": {                              │
│      fhirBundle: {...},         ← FHIR transformation       │
│      resourceCount: 5,                                       │
│      bundleType: "transaction"                               │
│    }                                                         │
│  }                                                           │
│                                                              │
│  metadata: {                                                 │
│    pipeline_id: "...",                                       │
│    execution_id: "...",                                      │
│    started_at: "..."                                         │
│  }                                                           │
└─────────────────────────────────────────────────────────────┘
```

### Key Concepts

1. **Execution Context Structure**
   ```go
   type PipelineExecutionContext struct {
       Message      map[string]interface{} // Original + parsed message
       StepOutputs  map[string]StepOutput  // Namespaced step outputs
       Metadata     map[string]interface{} // Pipeline metadata
   }

   type StepOutput struct {
       StepID       string                 // Unique step ID (UUID from DB)
       StepName     string                 // Human-readable name
       StepType     string                 // Executor type (pre.enrichment.api, etc.)
       OutputData   map[string]interface{} // Step-specific output
       Success      bool                   // Execution success
       Error        string                 // Error message if failed
       DurationMs   int64                  // Execution time
   }
   ```

2. **Step Output Namespace Pattern (User-Defined Alias + Smart Defaults)**
   - Each step has a **user-defined alias** for easy referencing
   - Namespace format: `{alias}_{shortID}` (internal) but users reference by alias only
   - **Smart Default Aliases**:
     - Auto-generated from step name: "Enrich EMPI API" → `empi`
     - Auto-increment for duplicates: `empi`, `empi2`, `empi3`
     - User can override with custom alias

   **Examples**:
   - Step: "Enrich EMPI API", Alias: `empi` (auto) → Namespace: `empi_b4c9f1`
   - Step: "Validate Patient ID", Alias: `validate_pid` (auto) → Namespace: `validate_pid_a3f8e2`
   - Step: "Map to FHIR", Alias: `fhir` (user-defined) → Namespace: `fhir_d5e2a3`

   **User Referencing** (simple, no GUID needed):
   - `{{empi.full_api_response.insurance}}`
   - `{{validate_pid.status}}`
   - `{{fhir.bundle}}`

3. **Namespace Generation**
   ```go
   func GenerateStepNamespace(stepName string, stepID string, alias *string) string {
       var aliasStr string

       if alias != nil && *alias != "" {
           // User provided custom alias
           aliasStr = *alias
       } else {
           // Generate smart default from step name
           aliasStr = GenerateDefaultAlias(stepName)
       }

       // Sanitize alias (lowercase, replace spaces with underscores)
       sanitized := strings.ToLower(strings.ReplaceAll(aliasStr, " ", "_"))

       // Use first 6 chars of step ID for uniqueness
       shortID := stepID[:6]

       return fmt.Sprintf("%s_%s", sanitized, shortID)
   }

   func GenerateDefaultAlias(stepName string) string {
       // Extract meaningful words from step name
       // "Enrich EMPI API" → "empi"
       // "Validate Patient ID" → "validate_pid"
       // "Map HL7 to FHIR" → "fhir"

       words := strings.Fields(stepName)

       // Simple heuristic: Use last significant word for enrichment/mapping
       if strings.Contains(strings.ToLower(stepName), "enrich") && len(words) >= 2 {
           return strings.ToLower(words[1]) // "Enrich EMPI" → "empi"
       }
       if strings.Contains(strings.ToLower(stepName), "map") && len(words) >= 1 {
           return strings.ToLower(words[len(words)-1]) // "Map to FHIR" → "fhir"
       }

       // Default: Use first word + last word
       if len(words) == 1 {
           return strings.ToLower(words[0])
       }
       return strings.ToLower(words[0]) + "_" + strings.ToLower(words[len(words)-1])
   }
   ```

4. **Message vs Step Output Separation**
   - **`context.Message`**: The actual message being transformed (modified in-place by steps)
   - **`context.StepOutputs`**: Step-specific outputs (what each step produced/discovered)

   **Example**:
   - Step modifies `context.Message` by adding `enriched.empi.insurance` data
   - Step ALSO stores in `context.StepOutputs["Enrich_EMPI_API_b4c9f1"]`:
     - Full API response
     - API metadata (status, response time, cache hit)
     - Number of fields enriched

---

## Implementation Plan

### Phase 1: Data Structure Updates (Week 1)

#### 1.1 Update Models (models/transformation_models.go)

```go
// Add to transformation_models.go

// PipelineExecutionContext represents the full execution context
type PipelineExecutionContext struct {
    Message      map[string]interface{} `json:"message"`
    StepOutputs  map[string]StepOutput  `json:"step_outputs"` // Key: "StepName_shortID"
    Metadata     map[string]interface{} `json:"metadata"`
}

// StepOutput represents output from a single step
type StepOutput struct {
    StepID       string                 `json:"step_id"`       // Full UUID
    StepName     string                 `json:"step_name"`     // Human-readable
    StepAlias    string                 `json:"step_alias"`    // User-friendly alias (e.g., "empi")
    StepType     string                 `json:"step_type"`     // Executor type
    Namespace    string                 `json:"namespace"`     // "alias_shortID" (e.g., "empi_b4c9f1")
    Sequence     int                    `json:"sequence"`      // Step execution order
    OutputData   map[string]interface{} `json:"output_data"`   // Step-specific output
    Success      bool                   `json:"success"`
    Error        string                 `json:"error,omitempty"`
    DurationMs   int64                  `json:"duration_ms"`
}

// Update TransformationStep to include alias
type TransformationStep struct {
    ID              string                 `json:"id" db:"id"`
    PipelineID      string                 `json:"pipeline_id" db:"pipeline_id"`
    StepName        string                 `json:"step_name" db:"step_name"`
    StepAlias       *string                `json:"step_alias,omitempty" db:"step_alias"` // NEW: User-defined alias
    StepType        string                 `json:"step_type" db:"step_type"`
    Sequence        int                    `json:"sequence" db:"sequence"`
    // ... rest of fields
}

// Update StepExecutionLog to include alias
type StepExecutionLog struct {
    StepID          string                 `json:"step_id"`
    StepName        string                 `json:"step_name"`
    StepAlias       string                 `json:"step_alias"`    // "empi", "validate_pid", "fhir"
    StepType        string                 `json:"step_type"`
    Namespace       string                 `json:"namespace"`     // "empi_b4c9f1"
    StartedAt       time.Time              `json:"started_at"`
    CompletedAt     time.Time              `json:"completed_at"`
    DurationMs      int64                  `json:"duration_ms"`
    Success         bool                   `json:"success"`
    Error           string                 `json:"error,omitempty"`
    StepOutput      *StepOutput            `json:"step_output,omitempty"` // Step-specific data
}

// Helper: Generate namespace from alias + ID
func GenerateStepNamespace(stepName string, stepID string, alias *string) string {
    var aliasStr string

    if alias != nil && *alias != "" {
        aliasStr = *alias
    } else {
        aliasStr = GenerateDefaultAlias(stepName)
    }

    sanitized := strings.ToLower(strings.ReplaceAll(aliasStr, " ", "_"))
    shortID := stepID[:6]
    return fmt.Sprintf("%s_%s", sanitized, shortID)
}

func GenerateDefaultAlias(stepName string) string {
    words := strings.Fields(stepName)

    if strings.Contains(strings.ToLower(stepName), "enrich") && len(words) >= 2 {
        return strings.ToLower(words[1]) // "Enrich EMPI API" → "empi"
    }
    if strings.Contains(strings.ToLower(stepName), "map") && len(words) >= 1 {
        return strings.ToLower(words[len(words)-1]) // "Map to FHIR" → "fhir"
    }

    if len(words) == 1 {
        return strings.ToLower(words[0])
    }
    return strings.ToLower(words[0]) + "_" + strings.ToLower(words[len(words)-1])
}
```

#### 1.2 Update Executor Interface (services/executors/base_executor.go)

```go
// New executor interface (clean implementation - no backward compatibility)
type Executor interface {
    Execute(ctx context.Context, step *models.TransformationStep, execContext *models.PipelineExecutionContext) error
    Validate(step *models.TransformationStep) error
    GetMetadata() models.ExecutorMetadata
}

// Helper methods for step output
func (e *BaseExecutor) SetStepOutput(
    execContext *models.PipelineExecutionContext,
    step *models.TransformationStep,
    outputData map[string]interface{},
) {
    namespace := models.GenerateStepNamespace(step.StepName, step.ID)

    execContext.StepOutputs[namespace] = models.StepOutput{
        StepID:     step.ID,
        StepName:   step.StepName,
        StepType:   step.StepType,
        Namespace:  namespace,
        OutputData: outputData,
        Success:    true,
    }
}

func (e *BaseExecutor) GetStepOutput(
    execContext *models.PipelineExecutionContext,
    namespace string,
) (map[string]interface{}, bool) {
    if output, exists := execContext.StepOutputs[namespace]; exists {
        return output.OutputData, true
    }
    return nil, false
}

// Helper: Get step output by alias (user-friendly lookup)
func (ctx *PipelineExecutionContext) GetStepOutputByAlias(alias string) (*StepOutput, error) {
    // Try exact namespace match first
    if output, exists := ctx.StepOutputs[alias]; exists {
        return &output, nil
    }

    // Try alias prefix match (alias without shortID)
    // "empi" matches "empi_b4c9f1"
    for namespace, output := range ctx.StepOutputs {
        parts := strings.Split(namespace, "_")
        if len(parts) >= 2 {
            namespaceAlias := strings.Join(parts[:len(parts)-1], "_")
            if namespaceAlias == alias {
                return &output, nil
            }
        }
    }

    return nil, fmt.Errorf("step output not found for alias: %s", alias)
}
```

### Phase 2: Pipeline Execution Updates (Week 1-2)

#### 2.1 Update ExecutePipeline (services/transformation_pipeline_service.go)

```go
func (tps *TransformationPipelineService) executePipeline(
    ctx context.Context,
    pipeline *models.TransformationPipeline,
    input map[string]interface{},
) (*models.TransformationResult, error) {

    // Create execution context
    execContext := &models.PipelineExecutionContext{
        Message:     input,
        StepOutputs: make(map[string]models.StepOutput),
        Metadata: map[string]interface{}{
            "pipeline_id":   pipeline.ID,
            "pipeline_name": pipeline.PipelineName,
            "started_at":    time.Now(),
        },
    }

    result := &models.TransformationResult{
        Success:           true,
        TransformationLog: []models.StepExecutionLog{},
        TotalTimeMs:       0,
    }

    // Execute steps in order
    for _, step := range pipeline.Steps {
        stepStartTime := time.Now()

        executor := tps.executorRegistry.GetExecutor(step.StepType)
        if executor == nil {
            return nil, fmt.Errorf("no executor for step type: %s", step.StepType)
        }

        // Execute step (modifies execContext.Message AND populates execContext.StepOutputs)
        stepErr := executor.Execute(ctx, &step, execContext)
        stepDuration := time.Now().Sub(stepStartTime)

        // Get step output from namespace
        namespace := models.GenerateStepNamespace(step.StepName, step.ID)
        stepOutput, outputExists := execContext.StepOutputs[namespace]

        // Update step output with duration and success status
        if outputExists {
            stepOutput.DurationMs = stepDuration.Milliseconds()
            if stepErr != nil {
                stepOutput.Success = false
                stepOutput.Error = stepErr.Error()
            }
            execContext.StepOutputs[namespace] = stepOutput
        } else if stepErr != nil {
            // Step didn't set output but failed - create error output
            execContext.StepOutputs[namespace] = models.StepOutput{
                StepID:     step.ID,
                StepName:   step.StepName,
                StepType:   step.StepType,
                Namespace:  namespace,
                OutputData: map[string]interface{}{},
                Success:    false,
                Error:      stepErr.Error(),
                DurationMs: stepDuration.Milliseconds(),
            }
        }

        // Create log entry with step output reference
        stepLog := models.StepExecutionLog{
            StepID:      step.ID,
            StepName:    step.StepName,
            StepType:    step.StepType,
            Namespace:   namespace,
            StartedAt:   stepStartTime,
            CompletedAt: time.Now(),
            DurationMs:  stepDuration.Milliseconds(),
            Success:     stepErr == nil,
        }

        if stepErr != nil {
            stepLog.Error = stepErr.Error()
        }

        // Attach step output to log
        if output, exists := execContext.StepOutputs[namespace]; exists {
            stepLog.StepOutput = &output
        }

        result.TransformationLog = append(result.TransformationLog, stepLog)

        // Handle errors according to strategy
        if stepErr != nil {
            if step.Required && step.OnErrorStrategy == "fail" {
                result.Success = false
                result.Error = fmt.Sprintf("Required step failed: %s", stepErr.Error())
                return result, stepErr
            }
            // Continue with skip/default strategies
        }

        result.TotalTimeMs += stepDuration.Milliseconds()
    }

    // Final output is the transformed message + all step outputs
    result.OutputData = execContext.Message

    return result, nil
}
```

### Phase 3: Step Executor Updates (Week 2)

#### 3.1 Update API Enrichment Executor Example

```go
func (e *APIEnrichmentExecutor) Execute(
    ctx context.Context,
    step *models.TransformationStep,
    execContext *models.PipelineExecutionContext,
) error {
    config := parseAPIEnrichmentConfig(step.Config)

    // Make API call
    startTime := time.Now()
    apiResponse, err := e.httpClient.ExecuteRequest(ctx, config)
    responseTime := time.Now().Sub(startTime).Milliseconds()

    if err != nil {
        return fmt.Errorf("API call failed: %w", err)
    }

    // Store response data in message at target path
    SetNestedValue(execContext.Message, config.TargetPath, apiResponse.Data)

    // Store step-specific output (what THIS step produced)
    stepOutput := map[string]interface{}{
        "api_url":           apiResponse.RequestURL,
        "http_method":       config.Method,
        "response_status":   apiResponse.StatusCode,
        "response_time_ms":  responseTime,
        "cache_hit":         apiResponse.CachedToken,
        "enriched_path":     config.TargetPath,
        "full_api_response": apiResponse.Data, // Complete API response
    }

    e.SetStepOutput(execContext, step, stepOutput)

    return nil
}
```

**What goes in StepOutput for API Enrichment**:
- API request metadata (URL, method, headers)
- Response metadata (status code, response time, cache hit)
- Full API response (can be referenced by later steps)
- Target path where data was stored in message

**What goes in Message**:
- The actual enriched data at the configured path
- Example: `message.enriched.empi = apiResponse.Data`

### Phase 4: UI/Display Improvements (Week 3)

#### 4.1 Step Alias Configuration in Pipeline Builder

**Add Alias Field to Step Properties**:

```javascript
// In PropertiesPanel.js - Basic step properties

<div class="form-group">
    <label>Step Name <span class="required">*</span></label>
    <input type="text" id="stepName" value="${step.stepName}" required>
    <small>Human-readable name (e.g., "Enrich EMPI API")</small>
</div>

<div class="form-group">
    <label>Step Alias (Reference Name)</label>
    <input type="text" id="stepAlias"
           value="${step.stepAlias || generateDefaultAlias(step.stepName)}"
           placeholder="e.g., empi">
    <small>Use this to reference step outputs: <code>{{empi.full_api_response}}</code></small>
    <div class="alias-preview">
        Full namespace: <code>${step.stepAlias || 'empi'}_${step.id.substring(0,6)}</code>
    </div>
</div>

<div class="form-group">
    <label>Step Type</label>
    <select id="stepType">...</select>
</div>
```

**Auto-Update Alias Preview**:
```javascript
// Update preview when user types in step name or alias
document.getElementById('stepName').addEventListener('input', (e) => {
    const aliasInput = document.getElementById('stepAlias');
    if (!aliasInput.dataset.userModified) {
        // Auto-update if user hasn't manually changed it
        aliasInput.value = generateDefaultAlias(e.target.value);
        updateAliasPreview();
    }
});

document.getElementById('stepAlias').addEventListener('input', (e) => {
    e.target.dataset.userModified = 'true';
    updateAliasPreview();
});

function generateDefaultAlias(stepName) {
    const words = stepName.split(' ');
    if (stepName.toLowerCase().includes('enrich') && words.length >= 2) {
        return words[1].toLowerCase(); // "Enrich EMPI API" → "empi"
    }
    if (stepName.toLowerCase().includes('map') && words.length >= 1) {
        return words[words.length - 1].toLowerCase(); // "Map to FHIR" → "fhir"
    }
    if (words.length === 1) {
        return words[0].toLowerCase();
    }
    return (words[0] + '_' + words[words.length - 1]).toLowerCase();
}
```

**Alias Uniqueness Validation**:
```javascript
function validateAliasUniqueness(alias, currentStepId, pipeline) {
    const existingAliases = pipeline.steps
        .filter(s => s.id !== currentStepId)
        .map(s => s.stepAlias || generateDefaultAlias(s.stepName));

    if (existingAliases.includes(alias)) {
        // Auto-append number for uniqueness
        let counter = 2;
        let uniqueAlias = `${alias}${counter}`;
        while (existingAliases.includes(uniqueAlias)) {
            counter++;
            uniqueAlias = `${alias}${counter}`;
        }
        return uniqueAlias;
    }
    return alias;
}
```

**Visual Representation in Pipeline Builder**:
```
┌────────────────────────────────┐
│ 🔍 Validate Patient ID         │
│ Alias: validate_pid            │  ← Shows alias
│ Seq: 10 │ Pre-Processing       │
└────────────────────────────────┘
        ↓
┌────────────────────────────────┐
│ ☁️  Enrich EMPI API            │
│ Alias: empi                    │  ← User references as {{empi}}
│ Seq: 20 │ Pre-Processing       │
└────────────────────────────────┘
        ↓
┌────────────────────────────────┐
│ 🔄 Map to FHIR                 │
│ Alias: fhir                    │  ← User references as {{fhir}}
│ Seq: 100 │ Core Mapping        │
└────────────────────────────────┘
```

#### 4.2 Enhanced Step Execution Log Display

```javascript
// Frontend: Display step-specific outputs clearly

function renderStepExecutionLog(stepLog) {
    const statusIcon = stepLog.success ? '✅' : '❌';
    const statusClass = stepLog.success ? 'success' : 'error';

    return `
        <div class="step-execution-card ${statusClass}">
            <div class="step-header">
                <span class="step-status">${statusIcon}</span>
                <span class="step-name">${stepLog.step_name}</span>
                <span class="step-namespace">(${stepLog.namespace})</span>
                <span class="step-duration">${stepLog.duration_ms}ms</span>
            </div>

            ${stepLog.error ? `
                <div class="step-error">
                    <strong>Error:</strong> ${stepLog.error}
                </div>
            ` : ''}

            <!-- Step-Specific Output -->
            ${stepLog.step_output ? `
                <div class="step-output">
                    <h4>Step Output</h4>
                    <pre>${JSON.stringify(stepLog.step_output.output_data, null, 2)}</pre>
                </div>
            ` : ''}
        </div>
    `;
}

// Example rendering for API Enrichment step:
/*
┌─────────────────────────────────────────────────────┐
│ ✅ Enrich EMPI API (Enrich_EMPI_API_b4c9f1)  234ms │
├─────────────────────────────────────────────────────┤
│ Step Output:                                        │
│ {                                                   │
│   "api_url": "https://empi.../patients/12345",     │
│   "http_method": "GET",                             │
│   "response_status": 200,                           │
│   "response_time_ms": 234,                          │
│   "cache_hit": false,                               │
│   "enriched_path": "enriched.empi",                 │
│   "full_api_response": {                            │
│     "insurance": [...],                             │
│     "emergency_contact": {...}                      │
│   }                                                 │
│ }                                                   │
└─────────────────────────────────────────────────────┘
*/
```

#### 4.2 Step Output Browser/Inspector

```javascript
// Allow users to browse all step outputs

function renderStepOutputBrowser(executionLog) {
    return `
        <div class="step-output-browser">
            <h3>Step Outputs</h3>
            <div class="step-output-list">
                ${executionLog.map(stepLog => `
                    <div class="step-output-item">
                        <button onclick="showStepOutput('${stepLog.namespace}')">
                            ${stepLog.step_name}
                        </button>
                        <span class="namespace">${stepLog.namespace}</span>
                    </div>
                `).join('')}
            </div>
        </div>
    `;
}

// Example: Allow step 5 to reference step 2's output
// User can click "Enrich_EMPI_API_b4c9f1" to view full output
```

---

## Benefits

### 1. Clear Step Attribution (Readable Namespaces)
```json
{
  "transformation_log": [
    {
      "step_name": "Enrich EMPI API",
      "namespace": "Enrich_EMPI_API_b4c9f1",
      "success": true,
      "duration_ms": 234,
      "step_output": {
        "api_url": "https://empi.hospital.org/api/patients/12345",
        "response_status": 200,
        "response_time_ms": 234,
        "cache_hit": false,
        "enriched_path": "enriched.empi",
        "full_api_response": {
          "insurance": [...],
          "emergency_contact": {...}
        }
      }
    }
  ]
}
```
**Instant clarity**: "Enrich EMPI API step called EMPI in 234ms, got 200 response, stored at enriched.empi"

### 2. Step Output Referencing (Clean Alias-Based)
```javascript
// User references by simple alias (no GUID needed!)
const empiResponse = context.stepOutputs.get("empi");
const patientInsurance = empiResponse.full_api_response.insurance;

// Or in configuration fields using template syntax:
{
  "fieldMappings": {
    "insurance": "{{empi.full_api_response.insurance}}",
    "validationStatus": "{{validate_pid.status}}"
  }
}

// System internally resolves:
// "empi" → "empi_b4c9f1" → StepOutput
// "validate_pid" → "validate_pid_a3f8e2" → StepOutput
```

### 3. No Output Confusion
**Before**: All steps overwrite `outputData` - outputs lost
**After**: Each step has its own named output - nothing lost

### 4. Better Debugging
- See exactly what each step produced
- Identify which step's API call failed
- Audit trail of all step operations
- Performance profiling per step with real names

### 5. Easier Testing
- Verify step-specific outputs in unit tests
- Mock previous step outputs by namespace
- Test individual steps in isolation

---

## Database Migration

```sql
-- V30: Add step alias and namespace columns

-- Add step_alias to transformation_steps table
ALTER TABLE transformation_steps
ADD COLUMN step_alias VARCHAR(100);

-- Create unique index on (pipeline_id, step_alias) for uniqueness validation
CREATE UNIQUE INDEX idx_pipeline_step_alias
ON transformation_steps(pipeline_id, step_alias)
WHERE step_alias IS NOT NULL;

-- Add namespace column to transformation execution logs
ALTER TABLE transformation_step_executions
ADD COLUMN namespace VARCHAR(255),
ADD COLUMN step_alias VARCHAR(100);

-- Add index for faster namespace lookups
CREATE INDEX idx_step_executions_namespace
ON transformation_step_executions(namespace);
```

**Note**: No backward compatibility needed - fresh implementation!

---

## Example Execution Log (Before vs After)

### Before (Current)
```json
{
  "transformation_log": [
    {
      "step_name": "Enrich EMPI",
      "success": true,
      "duration_ms": 234,
      "output_snapshot": {
        "raw": "MSH|^~\\&|...",
        "parsed": {...},
        "enhancedSegments": {...5000 lines...},
        "enriched": {
          "empi": {
            "insurance": [...],
            "emergency_contact": {...}
          }
        }
      }
    }
  ]
}
```
⚠️ **Problems**:
- Where's the enriched data? Buried in 5000+ line output snapshot!
- What did THIS step do? Can't tell from full message dump
- Output gets overwritten by next step
- Can't reference this step's API response later

### After (Proposed)
```json
{
  "transformation_log": [
    {
      "step_name": "Enrich EMPI API",
      "namespace": "Enrich_EMPI_API_b4c9f1",
      "success": true,
      "duration_ms": 234,
      "step_output": {
        "api_url": "https://empi.hospital.org/api/patients/12345",
        "http_method": "GET",
        "response_status": 200,
        "response_time_ms": 234,
        "cache_hit": false,
        "enriched_path": "enriched.empi",
        "full_api_response": {
          "insurance": [
            {"provider": "BCBS", "group": "G12345"},
            {"provider": "Medicare", "id": "M98765"}
          ],
          "emergency_contact": {
            "name": "Jane Doe",
            "phone": "555-1234"
          },
          "allergies": ["Penicillin", "Shellfish", "Latex"]
        }
      }
    }
  ]
}
```
✅ **Solutions**:
- **Clear**: See exactly what THIS step produced
- **Concise**: Only step-specific data, not full message
- **Accessible**: Reference `Enrich_EMPI_API_b4c9f1` in later steps
- **Readable**: Namespace includes step name for easy identification

---

## Timeline

- **Week 1**: Data structure updates, namespace generation, base executor changes
- **Week 2**: Pipeline execution updates, API enrichment executor migration
- **Week 3**: UI improvements, step output browser, testing
- **Week 4**: Documentation, production deployment

**Total Estimated Time**: 4 weeks

**First Implementation Target**: API Enrichment Executor (most critical for visibility)
