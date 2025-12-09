# Pipeline Storage Fix - Single Source of Truth

## Problem

Current implementation has fragmented storage causing confusion and breaks:

1. **Wizard mappings** → `interfaces.transformation_mapping` (JSONB)
2. **Pipeline definition** → `transformation_pipelines.pipeline_config` (JSONB blob)
3. **Steps NOT saved separately** → Should be in `transformation_steps` table

**Result**: Steps reference external mappings → breaks easily

---

## Solution: Hybrid Self-Contained Approach

### Architecture

```
Wizard Completion (Step 1):
├─ Save to: interfaces.transformation_mapping
└─ Purpose: Master copy, reusable

Pipeline Save (Step 2):
├─ Save pipeline: transformation_pipelines
├─ For each step: transformation_steps
│   ├─ If step is HL7→FHIR mapping:
│   │   ├─ Load wizard mappings from interfaces.transformation_mapping
│   │   ├─ Embed full mappings into step.config
│   │   └─ Step becomes self-contained
│   └─ Save step with embedded config
└─ Purpose: Self-contained, no external dependencies

Runtime Execution:
├─ Load pipeline steps from transformation_steps
├─ Each step has ALL data it needs in step.config
└─ No database lookups for mappings
```

---

## Implementation Plan

### 1. Fix Pipeline Save Controller

**File**: `controllers/pipelineController.js`

**Current** (line 133-149):
```javascript
// WRONG: Saves entire pipeline as one JSON blob
INSERT INTO transformation_pipelines (pipeline_config, ...)
VALUES ($5, ...)  // $5 = JSON.stringify(pipelineData)
```

**Should Be**:
```javascript
// 1. Save pipeline metadata
INSERT INTO transformation_pipelines (id, interface_id, message_type, pipeline_name, ...)
VALUES ($1, $2, $3, $4, ...)

// 2. Delete old steps
DELETE FROM transformation_steps WHERE pipeline_id = $1

// 3. For each step in pipeline:
FOR EACH step IN pipelineData.layers.pre/core/post:

    // Special handling for HL7→FHIR mapping
    IF step.type === 'core.mapping':
        // Load wizard mappings
        mappings = SELECT transformation_mapping
                   FROM interfaces
                   WHERE id = interfaceId

        // Embed into step config
        step.config.embedded_mappings = mappings

    // Save step
    INSERT INTO transformation_steps (
        pipeline_id, step_name, step_type, sequence, layer,
        config, required, timeout_ms, on_error_strategy
    ) VALUES (...)
```

---

### 2. Update Step Schema

**File**: `database/migrations/V20__Add_Transformation_Pipeline.sql`

Already has correct schema (line 28-49):
```sql
CREATE TABLE transformation_steps (
    id UUID PRIMARY KEY,
    pipeline_id UUID REFERENCES transformation_pipelines,
    step_name VARCHAR(255),
    step_type VARCHAR(100),  -- 'core.mapping', 'pre.validation', etc.
    sequence INTEGER,         -- 10, 20, 30...
    layer VARCHAR(20),        -- 'pre', 'core', 'post'
    config JSONB,            -- ✅ This holds embedded mappings
    timeout_ms INTEGER,
    on_error_strategy VARCHAR(20),
    ...
)
```

**Config Structure for HL7→FHIR Step**:
```json
{
  "fhir_version": "R4",
  "use_template": true,
  "interface_id": "ad553ba7-...",

  "embedded_mappings": {
    "atomicMappings": [
      {
        "segmentName": "PID",
        "hl7Field": "PID.5",
        "fhirResourceType": "Patient",
        "fhirElementPath": "name",
        ...
      }
    ],
    "version": 1,
    "messageType": "ADT^A01"
  }
}
```

---

### 3. Update Runtime Executor

**File**: `services/executor_registry.go` (line 292-340)

**Current**:
```go
func (hme *HL7FHIRMappingExecutor) Execute(...) {
    interfaceID, _ := step.Config["interface_id"].(string)
    // ❌ Loads from database at runtime
    mappings = service.loadFromDatabase(interfaceID)
}
```

**Should Be**:
```go
func (hme *HL7FHIRMappingExecutor) Execute(...) {
    // ✅ Check for embedded mappings first
    if embeddedMappings, ok := step.Config["embedded_mappings"].(map[string]interface{}); ok {
        log.Printf("✅ Using embedded mappings (self-contained)")
        return hme.transformWithEmbeddedMappings(embeddedMappings, inputData)
    }

    // Fallback: Load from database (backward compatibility)
    interfaceID, _ := step.Config["interface_id"].(string)
    if interfaceID != "" {
        log.Printf("⚠️ Loading mappings from database (fallback)")
        mappings = service.loadFromDatabase(interfaceID)
    }
}
```

---

## Benefits of This Approach

### ✅ Resilience
- Step has ALL data it needs
- No external dependencies to break
- Works even if interface is deleted

### ✅ Performance
- No database lookup at runtime
- Faster execution

### ✅ Traceability
- Can see EXACTLY what mappings were used
- Audit trail shows complete snapshot
- Execution logs have full context

### ✅ Versioning
- Can have different mapping versions per pipeline
- Update wizard → old pipelines unaffected
- Explicit version control

### ✅ Portability
- Export pipeline → includes all mappings
- Import pipeline → works immediately
- No orphaned references

---

## Migration Path

### For Existing Pipelines (if any):

```sql
-- Find pipelines with HL7→FHIR steps that only have interface_id reference
SELECT p.id, p.pipeline_name, s.id as step_id, s.config
FROM transformation_pipelines p
JOIN transformation_steps s ON s.pipeline_id = p.id
WHERE s.step_type = 'core.mapping'
  AND s.config->>'embedded_mappings' IS NULL
  AND s.config->>'interface_id' IS NOT NULL;

-- For each one, update to embed mappings
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{embedded_mappings}',
    (SELECT transformation_mapping FROM interfaces WHERE id = (config->>'interface_id')::uuid)
)
WHERE step_type = 'core.mapping'
  AND config->>'embedded_mappings' IS NULL;
```

---

## Testing Plan

### Test 1: Wizard → Pipeline Flow
1. Complete wizard → Verify saved to `interfaces.transformation_mapping`
2. Create pipeline with HL7→FHIR step
3. Save pipeline → Verify mappings embedded in `transformation_steps.config`
4. Check step config has `embedded_mappings` key

### Test 2: Runtime Execution
1. Send HL7 message
2. Pipeline executes
3. HL7→FHIR step uses embedded mappings
4. No database lookup for mappings
5. Transformation succeeds

### Test 3: Mapping Updates
1. Update wizard mappings
2. Old pipeline still works (has embedded snapshot)
3. Create NEW pipeline → gets updated mappings
4. Both pipelines work independently

---

## Files to Modify

1. ✅ **controllers/pipelineController.js** (lines 94-167)
   - Rewrite `savePipeline()` to save steps separately
   - Add logic to embed wizard mappings

2. ✅ **services/executor_registry.go** (lines 292-340)
   - Update `HL7FHIRMappingExecutor.Execute()` to check for embedded mappings first
   - Keep fallback for backward compatibility

3. ✅ **Create migration script** (optional)
   - Migrate existing pipelines to embed mappings

---

## Next Steps

1. **Implement new savePipeline()** - Properly save to V20 schema
2. **Update executor** - Check embedded_mappings first
3. **Test end-to-end** - Wizard → Pipeline → Execution
4. **Document** - Update CLAUDE.md with new architecture

---

**Status**: 📋 Design Complete - Ready for Implementation
**Priority**: 🔴 High - Fixes recurring breaks
**Complexity**: 🟡 Medium - ~2-3 hours
