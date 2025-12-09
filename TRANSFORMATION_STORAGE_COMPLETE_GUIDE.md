# Complete Guide: Where Transformations Are Saved

**Date**: November 27, 2025
**Status**: ✅ Fixed Implementation Ready

---

## 📊 Storage Architecture (Complete Picture)

### **1. Wizard Mappings**
**Master Copy - Reusable**

```
Table: interfaces
Column: transformation_mapping (JSONB)

Content:
{
  "messageType": "ADT^A01",
  "version": 1,
  "atomicMappings": [
    {
      "segmentName": "PID",
      "hl7Field": "PID.5",
      "fhirResourceType": "Patient",
      "fhirElementPath": "name",
      "dataTypeTransform": "HL7_XPN_to_FHIR_HumanName",
      "isRequired": true
    },
    // ... 50-100+ mappings
  ]
}
```

**Purpose**:
- ✅ Created by wizard
- ✅ Single source of truth for interface
- ✅ Can be updated independently
- ✅ Reusable across multiple pipelines

**When Saved**: After wizard completion

---

### **2. Pipeline Definition**
**Pipeline Metadata**

```
Table: transformation_pipelines

Columns:
- id (UUID)
- interface_id (UUID) → References interfaces
- message_type (VARCHAR) → "ADT^A01"
- pipeline_name (VARCHAR) → "ADT^A01 Pipeline"
- enabled (BOOLEAN) → true/false
- version (INTEGER) → Auto-increments on update
```

**Purpose**:
- ✅ Lightweight metadata only
- ✅ Links pipeline to interface
- ✅ One pipeline per (interface_id + message_type)

**When Saved**: When user clicks "💾 Save" in pipeline builder

---

### **3. Pipeline Steps**
**Individual Step Definitions with Embedded Config**

```
Table: transformation_steps

Columns:
- id (UUID)
- pipeline_id (UUID) → References transformation_pipelines
- step_name (VARCHAR) → "HL7 to FHIR Mapping"
- step_type (VARCHAR) → "core.mapping", "pre.validation", etc.
- sequence (INTEGER) → 10, 20, 30... (execution order)
- layer (VARCHAR) → "pre", "core", "post"
- config (JSONB) ← 🔥 THIS IS WHERE EVERYTHING IS STORED
- script_content (TEXT) → JavaScript code for custom steps
- timeout_ms (INTEGER) → 5000
- on_error_strategy (VARCHAR) → "fail", "skip", "default"
```

**Config Structure Example** (HL7→FHIR Step):
```json
{
  "fhir_version": "R4",
  "use_template": true,
  "interface_id": "ad553ba7-69b4-4e24-a76e-9a749a1087a9",

  "embedded_mappings": {
    "messageType": "ADT^A01",
    "version": 1,
    "atomicMappings": [
      {
        "segmentName": "PID",
        "hl7Field": "PID.5",
        "fhirResourceType": "Patient",
        "fhirElementPath": "name",
        "dataTypeTransform": "HL7_XPN_to_FHIR_HumanName"
      }
      // ... FULL wizard mappings embedded here
    ]
  },

  "_embedded_at": "2025-11-27T15:30:00Z",
  "_mapping_version": 1
}
```

**Config Structure Example** (Validation Step):
```json
{
  "rules": [
    {
      "field": "PID.3",
      "type": "string",
      "required": true,
      "min_length": 1
    },
    {
      "field": "PID.7",
      "type": "date",
      "format": "YYYYMMDD"
    }
  ]
}
```

**Purpose**:
- ✅ Self-contained - has ALL data needed for execution
- ✅ No external dependencies (won't break)
- ✅ Fast execution (no database lookups)
- ✅ Audit trail (know exactly what was used)

**When Saved**: When user clicks "💾 Save" in pipeline builder

---

### **4. Execution History**
**Runtime Audit Trail**

```
Table: transformation_executions

Columns:
- id (UUID)
- message_id (VARCHAR) → Which message was processed
- interface_id (UUID)
- pipeline_id (UUID) → Which pipeline ran
- started_at (TIMESTAMP)
- completed_at (TIMESTAMP)
- total_time_ms (BIGINT)
- status (VARCHAR) → "running", "completed", "failed"
- steps_executed (INTEGER) → How many steps ran
- steps_failed (INTEGER)
- input_data (JSONB) → Parsed HL7 snapshot
- output_data (JSONB) → FHIR bundle snapshot
- execution_log (JSONB) → Step-by-step logs
```

**Purpose**:
- ✅ Complete audit trail
- ✅ Debugging - see exactly what happened
- ✅ Performance monitoring
- ✅ Compliance (HIPAA audit requirements)

**When Saved**: Every message processed

---

### **5. Step-Level Execution Details**
**Granular Audit**

```
Table: step_executions

Columns:
- id (UUID)
- execution_id (UUID) → References transformation_executions
- step_id (UUID) → Which step ran
- step_name (VARCHAR)
- step_type (VARCHAR)
- started_at (TIMESTAMP)
- completed_at (TIMESTAMP)
- duration_ms (BIGINT)
- status (VARCHAR) → "running", "completed", "failed", "skipped"
- error_message (TEXT)
- input_snapshot (JSONB) → Data BEFORE this step
- output_snapshot (JSONB) → Data AFTER this step
```

**Purpose**:
- ✅ See which step failed
- ✅ See how data changed at each step
- ✅ Performance profiling per step
- ✅ Debug transformations

**When Saved**: Every step of every message processed

---

## 🔄 Complete Data Flow

### **Wizard Phase**
```
User completes wizard
    ↓
Creates 50-100 atomic mappings
    ↓
SAVES TO: interfaces.transformation_mapping (JSONB)
    ↓
✅ Master copy stored, reusable
```

### **Pipeline Build Phase**
```
User opens pipeline builder
    ↓
Drags "HL7 to FHIR Mapping" step to canvas
    ↓
Clicks "Save" in step modal
    ↓
Clicks "💾 Save" in pipeline toolbar
    ↓
Controller loads wizard mappings from interfaces table
    ↓
SAVES TO: transformation_pipelines (metadata)
SAVES TO: transformation_steps (each step with embedded config)
    ↓
✅ Self-contained pipeline stored
```

### **Runtime Execution Phase**
```
HL7 message arrives
    ↓
Parse to JSON (Layer 1)
    ↓
Load pipeline from transformation_pipelines
    ↓
Load steps from transformation_steps (ORDER BY sequence)
    ↓
For each step:
    ├─ Get executor by step_type
    ├─ Pass step.config (has embedded_mappings)
    ├─ Executor uses embedded data (no DB lookup!)
    ├─ Transform data
    ├─ LOG TO: step_executions (input/output snapshots)
    └─ Pass to next step
    ↓
Final FHIR output
    ↓
LOG TO: transformation_executions (complete audit)
    ↓
✅ Message processed, full audit trail saved
```

---

## 🎯 Why This Architecture?

### **Problem with OLD Approach**
```
❌ Step config: { "interface_id": "ad553ba7-..." }
❌ Runtime: Load mappings from interfaces table
❌ BREAKS IF:
   - Interface deleted
   - Mappings updated (old pipelines use new mappings!)
   - Database slow/unavailable
   - Connection issues
```

### **NEW Approach Benefits**
```
✅ Step config: { "embedded_mappings": { ... FULL DATA ... } }
✅ Runtime: Use embedded data directly
✅ NEVER BREAKS because:
   - Self-contained (no dependencies)
   - Frozen snapshot (consistent results)
   - No database lookups (fast)
   - Works even if interface deleted
```

---

## 📝 Implementation Status

### ✅ Completed
1. **Database schema** (V20 migration) - Already exists
2. **New save controller** - `controllers/pipelineController_v2.js`
3. **Documentation** - This file

### ⏳ Next Steps
1. **Replace old controller** - Swap `pipelineController.js` with v2
2. **Update executor** - Check for `embedded_mappings` first
3. **Test end-to-end** - Wizard → Pipeline → Execution
4. **Migration script** - Migrate existing pipelines (if any)

---

## 🔍 How to Verify

### After Wizard Completion:
```sql
SELECT id, name, message_type,
       transformation_mapping->>'messageType' as mapping_type,
       jsonb_array_length(transformation_mapping->'atomicMappings') as mapping_count
FROM interfaces
WHERE id = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9';
```

Expected: Shows interface with mappings

### After Pipeline Save:
```sql
-- Check pipeline
SELECT * FROM transformation_pipelines
WHERE interface_id = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9';

-- Check steps
SELECT id, step_name, step_type, sequence, layer,
       config->>'interface_id' as interface_id,
       CASE
           WHEN config->'embedded_mappings' IS NOT NULL THEN 'YES'
           ELSE 'NO'
       END as has_embedded_mappings,
       jsonb_array_length(config->'embedded_mappings'->'atomicMappings') as mapping_count
FROM transformation_steps
WHERE pipeline_id = (
    SELECT id FROM transformation_pipelines
    WHERE interface_id = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9'
);
```

Expected: Shows HL7→FHIR step with `has_embedded_mappings = 'YES'`

### After Message Processing:
```sql
-- Check execution
SELECT * FROM transformation_executions
WHERE interface_id = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9'
ORDER BY started_at DESC
LIMIT 1;

-- Check step details
SELECT s.step_name, s.step_type, s.duration_ms, s.status
FROM step_executions s
WHERE s.execution_id = (
    SELECT id FROM transformation_executions
    WHERE interface_id = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9'
    ORDER BY started_at DESC
    LIMIT 1
)
ORDER BY s.started_at;
```

Expected: Shows execution with all steps logged

---

## 📋 Summary Table

| Data | Table | Purpose | Size | Updated When |
|------|-------|---------|------|--------------|
| **Wizard Mappings** | `interfaces.transformation_mapping` | Master copy | 50-100 KB | Wizard complete |
| **Pipeline Metadata** | `transformation_pipelines` | Pipeline info | <1 KB | Pipeline save |
| **Pipeline Steps** | `transformation_steps` | Steps + embedded config | 50-100 KB per step | Pipeline save |
| **Execution History** | `transformation_executions` | Audit trail | 10-50 KB per message | Message processed |
| **Step Executions** | `step_executions` | Step-level audit | 5-20 KB per step | Each step executes |

**Total Storage** (per interface):
- Wizard mappings: ~100 KB (once)
- Pipeline: ~500 KB (once)
- Executions: ~50 KB per message (grows with volume)

---

## 🚀 Next Action

**Replace the old controller**:
```bash
cd c:\Projects\ezHealthKonnect
mv controllers/pipelineController.js controllers/pipelineController_old.js
mv controllers/pipelineController_v2.js controllers/pipelineController.js
docker compose restart app
```

Then test the complete flow:
1. Refresh pipeline builder
2. Add HL7→FHIR step
3. Save pipeline
4. Check database - should see embedded mappings
5. Send test message
6. Verify execution logs

---

**Status**: ✅ Complete Architecture Documented
**Ready**: Implementation code ready to deploy
