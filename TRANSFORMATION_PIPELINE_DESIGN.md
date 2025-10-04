# Transformation Pipeline Architecture Design

## Overview

Design for a flexible, configurable transformation pipeline that applies business logic to parsed JSON messages in a sequence defined by the user.

**Date**: October 2, 2025
**Status**: Design Phase

---

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Transformation Pipeline Concept](#transformation-pipeline-concept)
3. [Architecture Design](#architecture-design)
4. [Database Schema](#database-schema)
5. [Transformation Execution Engine](#transformation-execution-engine)
6. [User-Defined Logic](#user-defined-logic)
7. [Sequence Management](#sequence-management)
8. [Implementation Plan](#implementation-plan)

---

## Current State Analysis

### What We Have

```
✅ Raw Message Storage (PostgreSQL + MongoDB)
✅ JSON Conversion (Enhanced Schema in MongoDB)
✅ HL7-FHIR Mapping Templates (hl7_fhir_templates table)
✅ Interface-Message Mappings (interface_message_mappings table)
✅ Message-Type Centric Design (V9 migration)
```

### Existing Database Structure

**hl7_fhir_templates**: Standard mapping templates
```sql
id                   UUID
message_type         VARCHAR(50)      -- "ADT^A01"
template_config      JSONB            -- Standard mapping configuration
is_default           BOOLEAN
```

**interface_message_mappings**: Per-interface, per-message-type configuration
```sql
id                     UUID
interface_id           UUID
message_type           VARCHAR(50)
uses_standard_template BOOLEAN        -- Use standard or custom?
standard_template_id   UUID            -- Reference to hl7_fhir_templates
custom_mapping_config  JSONB           -- Custom mapping if not using standard
```

### What We Need

```
❌ Transformation execution engine
❌ Transformation step sequencing
❌ User-defined business logic support
❌ Pre/Post processing hooks
❌ Transformation result storage
❌ Error handling and rollback
❌ Transformation auditing
```

---

## Transformation Pipeline Concept

### Three-Layer Transformation Model

```
┌────────────────────────────────────────────────────────────┐
│ Layer 1: SYSTEM TRANSFORMATIONS (Built-in)                │
│ - Format-specific conversions (HL7→JSON, XML→JSON)        │
│ - Auto-executed, cannot be disabled                       │
│ - Already implemented (JSON conversion pipeline)          │
└────────────────────────────────────────────────────────────┘
                          ↓
┌────────────────────────────────────────────────────────────┐
│ Layer 2: PRE-PROCESSING TRANSFORMATIONS (Optional)        │
│ - Data enrichment (lookup external data)                  │
│ - Data validation (business rules)                        │
│ - Data cleansing (fix known issues)                       │
│ - User-defined sequence                                   │
└────────────────────────────────────────────────────────────┘
                          ↓
┌────────────────────────────────────────────────────────────┐
│ Layer 3: CORE MAPPING TRANSFORMATION (Required)           │
│ - HL7→FHIR mapping (template-based)                       │
│ - Field-level transformations                             │
│ - Conditional logic                                       │
│ - Uses: hl7_fhir_templates + interface customizations     │
└────────────────────────────────────────────────────────────┘
                          ↓
┌────────────────────────────────────────────────────────────┐
│ Layer 4: POST-PROCESSING TRANSFORMATIONS (Optional)       │
│ - FHIR validation                                         │
│ - Custom business logic                                   │
│ - Data masking/anonymization                             │
│ - User-defined sequence                                   │
└────────────────────────────────────────────────────────────┘
                          ↓
                  Output Message
```

### Message Journey

```
Incoming HL7 Message
    ↓
[ALREADY IMPLEMENTED]
    ↓ JSON Conversion (System Transformation)
    ↓
Parsed JSON (Enhanced Schema)
    ↓
[NEW: TRANSFORMATION PIPELINE]
    ↓ Pre-Processing Steps (User-defined sequence)
    ↓ Core Mapping (HL7→FHIR using templates)
    ↓ Post-Processing Steps (User-defined sequence)
    ↓
FHIR Bundle (Output)
    ↓
[DELIVERY]
    ↓ Send to destination
```

---

## Architecture Design

### Transformation Step Types

```go
// models/transformation_models.go

type TransformationType string

const (
    // System transformations (auto-executed)
    TransformTypeSystemParsing     TransformationType = "system.parsing"

    // Pre-processing transformations
    TransformTypeDataEnrichment    TransformationType = "pre.enrichment"
    TransformTypeDataValidation    TransformationType = "pre.validation"
    TransformTypeDataCleansing     TransformationType = "pre.cleansing"
    TransformTypeCustomPreLogic    TransformationType = "pre.custom"

    // Core mapping transformations
    TransformTypeCoreMapping       TransformationType = "core.mapping"
    TransformTypeFieldMapping      TransformationType = "core.field_mapping"
    TransformTypeConditionalLogic  TransformationType = "core.conditional"

    // Post-processing transformations
    TransformTypeFHIRValidation    TransformationType = "post.validation"
    TransformTypeDataMasking       TransformationType = "post.masking"
    TransformTypeCustomPostLogic   TransformationType = "post.custom"
)

type TransformationStep struct {
    ID              string             // Step identifier
    Name            string             // Human-readable name
    Type            TransformationType // Step type
    Sequence        int                // Execution order
    Enabled         bool               // Can be disabled
    Required        bool               // Cannot skip if true
    Configuration   map[string]interface{} // Step-specific config
    Script          string             // JavaScript/Lua script for custom logic
    OnError         ErrorHandling      // What to do on error
    Timeout         int                // Max execution time (ms)
}

type ErrorHandling struct {
    Strategy    string // "skip", "fail", "retry", "default"
    RetryCount  int
    RetryDelay  int    // milliseconds
    DefaultValue interface{}
}

type TransformationPipeline struct {
    ID                  string
    InterfaceID         string
    MessageType         string
    Steps               []TransformationStep
    Created             time.Time
    Updated             time.Time
    Version             int
}
```

---

## Database Schema

### New Tables

#### 1. `transformation_pipelines` - Pipeline Configuration

```sql
CREATE TABLE transformation_pipelines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    message_type VARCHAR(50) NOT NULL,

    -- Pipeline metadata
    pipeline_name VARCHAR(100) NOT NULL,
    description TEXT,

    -- Execution settings
    enabled BOOLEAN DEFAULT true,
    timeout_ms INTEGER DEFAULT 30000,
    max_retries INTEGER DEFAULT 3,

    -- Error handling
    on_error_strategy VARCHAR(20) DEFAULT 'fail', -- 'fail', 'skip', 'use_default'
    default_output JSONB, -- Default output if all steps fail

    -- Versioning
    version INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),

    -- Performance tracking
    avg_execution_time_ms INTEGER,
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    last_executed_at TIMESTAMP WITH TIME ZONE,

    UNIQUE(interface_id, message_type)
);

CREATE INDEX idx_pipelines_interface ON transformation_pipelines(interface_id);
CREATE INDEX idx_pipelines_message_type ON transformation_pipelines(message_type);
CREATE INDEX idx_pipelines_enabled ON transformation_pipelines(enabled) WHERE enabled = true;
```

#### 2. `transformation_steps` - Individual Steps

```sql
CREATE TABLE transformation_steps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pipeline_id UUID NOT NULL REFERENCES transformation_pipelines(id) ON DELETE CASCADE,

    -- Step identification
    step_name VARCHAR(100) NOT NULL,
    step_type VARCHAR(50) NOT NULL, -- 'pre.enrichment', 'core.mapping', etc.
    description TEXT,

    -- Execution order
    sequence INTEGER NOT NULL, -- Lower = earlier execution
    layer VARCHAR(20) NOT NULL, -- 'pre', 'core', 'post'

    -- Execution settings
    enabled BOOLEAN DEFAULT true,
    required BOOLEAN DEFAULT false, -- Cannot skip if true
    timeout_ms INTEGER DEFAULT 5000,

    -- Configuration
    config JSONB, -- Step-specific configuration

    -- Custom logic (for user-defined steps)
    script_type VARCHAR(20), -- 'javascript', 'lua', 'python', null
    script_content TEXT, -- User-defined script

    -- Error handling
    on_error_strategy VARCHAR(20) DEFAULT 'fail',
    retry_count INTEGER DEFAULT 0,
    retry_delay_ms INTEGER DEFAULT 1000,
    default_output JSONB,

    -- Dependencies
    depends_on_steps UUID[], -- Array of step IDs that must succeed first

    -- Performance tracking
    avg_execution_time_ms INTEGER,
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(pipeline_id, sequence)
);

CREATE INDEX idx_steps_pipeline ON transformation_steps(pipeline_id);
CREATE INDEX idx_steps_sequence ON transformation_steps(pipeline_id, sequence);
CREATE INDEX idx_steps_type ON transformation_steps(step_type);
CREATE INDEX idx_steps_enabled ON transformation_steps(pipeline_id, enabled) WHERE enabled = true;
```

#### 3. `transformation_executions` - Execution History/Audit

```sql
CREATE TABLE transformation_executions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pipeline_id UUID NOT NULL REFERENCES transformation_pipelines(id),
    message_id VARCHAR(255) NOT NULL, -- Reference to original message

    -- Execution metadata
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL, -- 'running', 'success', 'failed', 'partial'

    -- Input/Output
    input_data JSONB, -- Parsed JSON input
    output_data JSONB, -- Final transformed output

    -- Performance
    total_execution_time_ms INTEGER,
    steps_executed INTEGER DEFAULT 0,
    steps_succeeded INTEGER DEFAULT 0,
    steps_failed INTEGER DEFAULT 0,
    steps_skipped INTEGER DEFAULT 0,

    -- Error tracking
    error_message TEXT,
    failed_step_id UUID REFERENCES transformation_steps(id),

    -- Metadata
    executed_by VARCHAR(50) DEFAULT 'system', -- 'system', 'manual', 'retry'
    retry_count INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_executions_pipeline ON transformation_executions(pipeline_id);
CREATE INDEX idx_executions_message ON transformation_executions(message_id);
CREATE INDEX idx_executions_status ON transformation_executions(status);
CREATE INDEX idx_executions_started ON transformation_executions(started_at DESC);
```

#### 4. `transformation_step_executions` - Detailed Step Tracking

```sql
CREATE TABLE transformation_step_executions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    execution_id UUID NOT NULL REFERENCES transformation_executions(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES transformation_steps(id),

    -- Execution details
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL, -- 'success', 'failed', 'skipped'

    -- Input/Output for this step
    input_data JSONB,
    output_data JSONB,

    -- Performance
    execution_time_ms INTEGER,

    -- Error details
    error_message TEXT,
    error_stack TEXT,

    -- Retry tracking
    retry_attempt INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_step_executions_execution ON transformation_step_executions(execution_id);
CREATE INDEX idx_step_executions_step ON transformation_step_executions(step_id);
CREATE INDEX idx_step_executions_status ON transformation_step_executions(status);
```

#### 5. `transformation_templates` - Reusable Step Templates

```sql
CREATE TABLE transformation_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Template identification
    template_name VARCHAR(100) NOT NULL UNIQUE,
    template_type VARCHAR(50) NOT NULL, -- 'enrichment', 'validation', etc.
    description TEXT,

    -- Template configuration
    default_config JSONB NOT NULL,
    script_template TEXT, -- Template for user-defined logic

    -- Parameters that can be customized
    configurable_params JSONB, -- Schema for what users can customize

    -- Metadata
    category VARCHAR(50), -- 'data_quality', 'business_logic', etc.
    tags TEXT[],

    -- Usage tracking
    usage_count INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id)
);

CREATE INDEX idx_templates_type ON transformation_templates(template_type);
CREATE INDEX idx_templates_category ON transformation_templates(category);
```

---

## Transformation Execution Engine

### Service Architecture

```go
// services/transformation_pipeline_service.go

type TransformationPipelineService struct {
    db                *sql.DB
    mongoService      *MongoDBMessageService
    postgresService   *InterfaceMessageService
    executors         map[TransformationType]TransformationExecutor
}

// Main execution method
func (tps *TransformationPipelineService) ExecuteTransformation(
    ctx context.Context,
    messageID string,
    interfaceID string,
    messageType string,
    parsedJSON map[string]interface{},
) (*TransformationResult, error) {

    // 1. Get transformation pipeline for this interface + message type
    pipeline, err := tps.GetPipeline(interfaceID, messageType)
    if err != nil {
        return nil, fmt.Errorf("failed to get pipeline: %w", err)
    }

    // 2. Create execution record
    execution := tps.StartExecution(pipeline.ID, messageID, parsedJSON)

    // 3. Execute steps in sequence
    currentData := parsedJSON
    for _, step := range pipeline.Steps {
        if !step.Enabled {
            tps.RecordStepSkipped(execution.ID, step.ID, "disabled")
            continue
        }

        // Check dependencies
        if !tps.CheckDependencies(execution.ID, step.DependsOnSteps) {
            tps.RecordStepSkipped(execution.ID, step.ID, "dependencies_not_met")
            continue
        }

        // Execute step
        stepResult, err := tps.ExecuteStep(ctx, step, currentData)

        if err != nil {
            // Handle error based on strategy
            if step.Required || step.OnError.Strategy == "fail" {
                tps.RecordExecutionFailed(execution.ID, step.ID, err)
                return nil, fmt.Errorf("required step failed: %w", err)
            }

            // Use default or skip
            if step.OnError.Strategy == "default" && step.OnError.DefaultValue != nil {
                currentData = step.OnError.DefaultValue.(map[string]interface{})
            }
            // If "skip", continue with current data

            tps.RecordStepFailed(execution.ID, step.ID, err)
            continue
        }

        // Update current data with step output
        currentData = stepResult.OutputData
        tps.RecordStepSuccess(execution.ID, step.ID, stepResult)
    }

    // 4. Store final output
    tps.StoreTransformedOutput(messageID, interfaceID, currentData)

    // 5. Mark execution complete
    tps.RecordExecutionSuccess(execution.ID, currentData)

    return &TransformationResult{
        Success:    true,
        OutputData: currentData,
        Execution:  execution,
    }, nil
}
```

### Step Executors

```go
// services/transformation_executors.go

type TransformationExecutor interface {
    Execute(ctx context.Context, step TransformationStep, inputData map[string]interface{}) (*StepResult, error)
    Validate(step TransformationStep) error
    GetType() TransformationType
}

// Core Mapping Executor (HL7 → FHIR)
type CoreMappingExecutor struct {
    templateService *HL7FHIRTemplateService
}

func (cme *CoreMappingExecutor) Execute(
    ctx context.Context,
    step TransformationStep,
    inputData map[string]interface{},
) (*StepResult, error) {

    startTime := time.Now()

    // Get mapping configuration
    mappingConfig := step.Configuration["mapping_config"]

    // Apply HL7 → FHIR transformation
    fhirBundle, err := cme.ApplyMapping(inputData, mappingConfig)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Success:       true,
        OutputData:    fhirBundle,
        ExecutionTime: time.Since(startTime),
        Metadata: map[string]interface{}{
            "resources_created": len(fhirBundle["entry"].([]interface{})),
        },
    }, nil
}

// Custom Script Executor (User-defined logic)
type CustomScriptExecutor struct {
    jsRuntime     *goja.Runtime // JavaScript engine
    luaRuntime    *lua.LState   // Lua engine
}

func (cse *CustomScriptExecutor) Execute(
    ctx context.Context,
    step TransformationStep,
    inputData map[string]interface{},
) (*StepResult, error) {

    switch step.ScriptType {
    case "javascript":
        return cse.executeJavaScript(ctx, step.Script, inputData)
    case "lua":
        return cse.executeLua(ctx, step.Script, inputData)
    default:
        return nil, fmt.Errorf("unsupported script type: %s", step.ScriptType)
    }
}

// Data Enrichment Executor
type DataEnrichmentExecutor struct {
    externalAPIs map[string]ExternalAPIClient
    cacheService *CacheService
}

func (dee *DataEnrichmentExecutor) Execute(
    ctx context.Context,
    step TransformationStep,
    inputData map[string]interface{},
) (*StepResult, error) {

    // Example: Lookup patient data from external system
    enrichmentConfig := step.Configuration["enrichment"]

    // Extract identifier from input
    patientID := inputData["enhancedSegments"].(map[string]interface{})["PID"].(map[string]interface{})["fields"]...

    // Check cache first
    cachedData, found := dee.cacheService.Get(patientID)
    if found {
        return dee.mergeEnrichment(inputData, cachedData)
    }

    // Call external API
    externalData, err := dee.fetchExternalData(ctx, patientID, enrichmentConfig)
    if err != nil {
        return nil, err
    }

    // Cache result
    dee.cacheService.Set(patientID, externalData, 3600)

    // Merge with input data
    return dee.mergeEnrichment(inputData, externalData)
}

// Data Validation Executor
type DataValidationExecutor struct {
    validationRules map[string]ValidationRule
}

func (dve *DataValidationExecutor) Execute(
    ctx context.Context,
    step TransformationStep,
    inputData map[string]interface{},
) (*StepResult, error) {

    validationConfig := step.Configuration["validation"]

    errors := []string{}
    warnings := []string{}

    // Run validation rules
    for _, rule := range validationConfig["rules"].([]interface{}) {
        result := dve.validateRule(inputData, rule)
        if !result.IsValid {
            if result.Severity == "error" {
                errors = append(errors, result.Message)
            } else {
                warnings = append(warnings, result.Message)
            }
        }
    }

    if len(errors) > 0 && step.Required {
        return nil, fmt.Errorf("validation failed: %v", errors)
    }

    // Add validation results to metadata
    inputData["_validation"] = map[string]interface{}{
        "errors":   errors,
        "warnings": warnings,
    }

    return &StepResult{
        Success:    len(errors) == 0,
        OutputData: inputData,
        Metadata: map[string]interface{}{
            "errors":   errors,
            "warnings": warnings,
        },
    }, nil
}
```

---

## User-Defined Logic

### JavaScript/Lua Script Support

**User writes custom transformation logic**:

```javascript
// Example: Custom pre-processing step
function transform(input) {
    // Access parsed HL7 data
    var pid = input.enhancedSegments.PID;
    var patientName = pid.fields.find(f => f.key === "PID.5");

    // Custom business logic
    if (patientName.value.includes("TEST")) {
        input._metadata = input._metadata || {};
        input._metadata.isTestPatient = true;
        input._metadata.skipFHIRValidation = true;
    }

    // Enrich data
    input.enrichedData = {
        fullName: formatName(patientName.value),
        timestamp: new Date().toISOString()
    };

    return input;
}

function formatName(rawName) {
    var parts = rawName.split("^");
    return parts[1] + " " + parts[0]; // "John Doe"
}
```

**Store in database**:

```sql
INSERT INTO transformation_steps (
    pipeline_id,
    step_name,
    step_type,
    sequence,
    layer,
    script_type,
    script_content,
    config
) VALUES (
    'pipeline-uuid',
    'Mark Test Patients',
    'pre.custom',
    100,
    'pre',
    'javascript',
    '...script above...',
    '{
        "description": "Identify test patients and set metadata",
        "timeout_ms": 1000
    }'::jsonb
);
```

### Template-Based Custom Logic

**Reusable template with parameters**:

```sql
-- Create template
INSERT INTO transformation_templates (
    template_name,
    template_type,
    description,
    script_template,
    configurable_params
) VALUES (
    'Patient Data Enrichment',
    'enrichment',
    'Lookup patient demographics from external API',
    'function transform(input, config) {
        var pid = extractPID(input);
        var externalData = callAPI(config.apiEndpoint, pid, config.apiKey);
        return mergeData(input, externalData, config.mergeStrategy);
     }',
    '{
        "apiEndpoint": {"type": "string", "required": true},
        "apiKey": {"type": "string", "required": true},
        "mergeStrategy": {"type": "enum", "values": ["overwrite", "merge", "append"], "default": "merge"}
    }'::jsonb
);

-- User customizes template for their interface
INSERT INTO transformation_steps (
    pipeline_id,
    step_name,
    step_type,
    sequence,
    config
) VALUES (
    'pipeline-uuid',
    'Enrich from Epic',
    'pre.enrichment',
    50,
    '{
        "template_id": "template-uuid",
        "apiEndpoint": "https://epic.hospital.com/api/patients",
        "apiKey": "secret-key",
        "mergeStrategy": "overwrite"
    }'::jsonb
);
```

---

## Sequence Management

### Defining Execution Order

**Sequence Rules**:
1. **Lower sequence number = earlier execution**
2. **Steps in same layer execute in sequence order**
3. **Layers execute in fixed order**: pre → core → post
4. **Dependencies override sequence** (dependent steps wait)

### UI Design (Future)

**Drag-and-Drop Pipeline Builder**:

```
┌─────────────────────────────────────────────────────────┐
│ Pipeline: ADT^A01 → FHIR (Interface: Hospital A)       │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  PRE-PROCESSING                                         │
│  ┌──────────────────────────────────────────┐          │
│  │ [↕] 10. Validate Patient ID (Required)   │ [Edit]   │
│  ├──────────────────────────────────────────┤          │
│  │ [↕] 20. Enrich from Epic API             │ [Edit]   │
│  ├──────────────────────────────────────────┤          │
│  │ [↕] 30. Mark Test Patients (Custom JS)   │ [Edit]   │
│  └──────────────────────────────────────────┘          │
│                                                         │
│  CORE MAPPING                                           │
│  ┌──────────────────────────────────────────┐          │
│  │ [↕] 100. HL7 → FHIR Mapping (Template)   │ [Edit]   │
│  └──────────────────────────────────────────┘          │
│                                                         │
│  POST-PROCESSING                                        │
│  ┌──────────────────────────────────────────┐          │
│  │ [↕] 200. Validate FHIR Bundle            │ [Edit]   │
│  ├──────────────────────────────────────────┤          │
│  │ [↕] 210. Anonymize PHI (Custom)          │ [Edit]   │
│  └──────────────────────────────────────────┘          │
│                                                         │
│  [+ Add Step]  [Save Pipeline]  [Test]                 │
└─────────────────────────────────────────────────────────┘
```

### Managing Sequence Conflicts

**Auto-Resequencing**:
```sql
-- When user moves step from sequence 30 to 15
-- Auto-update affected steps:
UPDATE transformation_steps
SET sequence = sequence + 1
WHERE pipeline_id = 'pipeline-uuid'
  AND sequence >= 15
  AND sequence < 30;

UPDATE transformation_steps
SET sequence = 15
WHERE id = 'step-uuid';
```

**Dependency Validation**:
```sql
-- Before saving, check dependencies are earlier in sequence
SELECT COUNT(*) FROM transformation_steps s1
JOIN unnest(s1.depends_on_steps) dep_id ON true
JOIN transformation_steps s2 ON s2.id = dep_id::uuid
WHERE s1.pipeline_id = 'pipeline-uuid'
  AND s2.sequence >= s1.sequence; -- ERROR: dependency must be earlier

-- Should return 0 (no conflicts)
```

---

## Implementation Plan

### Phase 1: Database Schema (Week 1)

**Tasks**:
1. ✅ Create `transformation_pipelines` table
2. ✅ Create `transformation_steps` table
3. ✅ Create `transformation_executions` table
4. ✅ Create `transformation_step_executions` table
5. ✅ Create `transformation_templates` table
6. ✅ Create Flyway migration V20
7. ✅ Seed with default templates

### Phase 2: Core Services (Week 2-3)

**Tasks**:
1. Create `TransformationPipelineService`
2. Create `CoreMappingExecutor` (HL7 → FHIR)
3. Create `CustomScriptExecutor` (JavaScript support)
4. Create pipeline execution engine
5. Integrate with existing message processing
6. Add audit logging

### Phase 3: User-Defined Logic (Week 4)

**Tasks**:
1. Implement JavaScript runtime (goja)
2. Create script validation
3. Add script templates
4. Implement error handling
5. Add timeout protection

### Phase 4: Management API (Week 5)

**Tasks**:
1. Pipeline CRUD endpoints
2. Step CRUD endpoints
3. Template management endpoints
4. Execution history endpoints
5. Testing endpoints

### Phase 5: UI (Week 6-8)

**Tasks**:
1. Pipeline builder (drag-and-drop)
2. Step configuration forms
3. Script editor with syntax highlighting
4. Test runner
5. Execution history viewer

---

## Integration Points

### How HL7-FHIR Mapping Gets Applied

```go
// processing/engine.go - After JSON conversion completes

func (pe *ProcessingEngine) convertToJSON(...) {
    // ... JSON conversion code ...

    if result.Success {
        // Trigger transformation pipeline
        go pe.executeTransformationPipeline(
            messageID,
            interfaceID,
            result.ParsedJSON,
            result.Metadata.MessageType,
        )
    }
}

func (pe *ProcessingEngine) executeTransformationPipeline(
    messageID string,
    interfaceID string,
    parsedJSON map[string]interface{},
    messageType string,
) {
    ctx := context.Background()

    fmt.Printf("🔄 Starting transformation pipeline for message: %s\n", messageID)

    // Execute transformation pipeline
    result, err := pe.transformationService.ExecuteTransformation(
        ctx,
        messageID,
        interfaceID,
        messageType,
        parsedJSON,
    )

    if err != nil {
        fmt.Printf("❌ Transformation failed for %s: %v\n", messageID, err)
        return
    }

    fmt.Printf("✅ Transformation completed for %s\n", messageID)

    // Store transformed output (FHIR bundle)
    pe.storeTransformedOutput(messageID, interfaceID, result.OutputData)
}
```

### Data Flow with Transformation Pipeline

```
Message Arrives
    ↓
Store Raw (PostgreSQL + MongoDB)
    ↓
JSON Conversion (Already Implemented)
    ↓
parsed_content in MongoDB
    ↓
[NEW] Transformation Pipeline Triggered
    ↓
Get Pipeline for (interface_id, message_type)
    ↓
Execute Steps in Sequence:
    ├─ Pre-Processing Steps
    │  ├─ Data Validation
    │  ├─ Data Enrichment
    │  └─ Custom Logic
    ↓
    ├─ Core Mapping Step
    │  └─ Apply HL7→FHIR Template
    ↓
    └─ Post-Processing Steps
       ├─ FHIR Validation
       └─ Custom Logic
    ↓
Store Transformed Output
    ├─ MongoDB: transformed_content field
    └─ PostgreSQL: transformation_status
    ↓
Deliver to Destination
```

---

## Next Steps

1. **Review this design** with team
2. **Create V20 migration** for transformation tables
3. **Implement TransformationPipelineService** (core engine)
4. **Create CoreMappingExecutor** (reuse existing FHIR mapping)
5. **Add JavaScript runtime** for custom logic
6. **Build management API** endpoints
7. **Design UI mockups** for pipeline builder

---

## Questions to Resolve

1. **Script Language**: JavaScript only, or also Lua/Python?
2. **External API Calls**: Allow in pre-processing? Security concerns?
3. **Caching Strategy**: Redis for enrichment cache?
4. **Execution Timeout**: Global default? Per-step override?
5. **Testing**: Automated testing of pipelines before deployment?
6. **Rollback**: Ability to revert to previous pipeline version?

---

**Status**: Ready for Implementation
**Estimated Timeline**: 6-8 weeks for full implementation
**Dependencies**: JSON Conversion Pipeline (✅ Complete)
