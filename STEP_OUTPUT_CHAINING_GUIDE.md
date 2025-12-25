# Step Output Chaining Guide

## Date
December 25, 2025

## Overview
The ezHealthKonnect Pipeline Builder supports **step output chaining** - using the output from previous steps as input to subsequent steps. This enables powerful data transformation workflows.

## How It Works

### Step Output Storage
Every step can store its output data in the pipeline execution context:

```go
// In any executor
func (e *MyExecutor) Execute(ctx context.Context, step *models.TransformationStep, execContext *models.PipelineExecutionContext) error {
    // ... do work ...

    // Store step output for use by later steps
    outputData := map[string]interface{}{
        "enriched_data": enrichedResult,
        "rows_count": len(rows),
        "status": "success",
    }

    e.SetStepOutput(execContext, step, outputData)
    return nil
}
```

### Accessing Previous Step Output

There are **3 ways** to reference previous step outputs:

#### 1. By Step Alias (Recommended)
```javascript
// In configuration or custom scripts
stepOutput.database_lookup.enriched_data
stepOutput.api_call.response.patient_name
```

#### 2. By Namespace
```javascript
// Full namespace format
stepOutput["database_enrichment_step_123"].enriched_data
```

#### 3. Using GetNestedValue (Backend)
```go
// In Go executors
value := GetNestedValue(execContext.CurrentData, "stepOutput.database_lookup.enriched_data")
```

## Current Implementation Status

### ✅ Already Implemented

**1. Database Enrichment Steps**
- Store enriched data in step output
- Access via `stepOutput.{alias}.enriched_data`

**File**: [services/executors/enrichment/database_enrichment_executor.go](services/executors/enrichment/database_enrichment_executor.go)

```go
// Line 150-160 (approximate)
outputData := map[string]interface{}{
    "enriched_data": enrichedResult,
    "rows_count":    len(enrichedResult),
    "query_params":  step.Config["queryParams"],
}

e.SetStepOutput(execContext, step, outputData)
```

**2. API Enrichment Steps**
- Store API responses in step output
- Access via `stepOutput.{alias}.response`

**File**: [services/executors/enrichment/api_enrichment_executor.go](services/executors/enrichment/api_enrichment_executor.go)

```go
outputData := map[string]interface{}{
    "response":     responseData,
    "status_code":  resp.StatusCode,
    "content_type": resp.Header.Get("Content-Type"),
}

e.SetStepOutput(execContext, step, outputData)
```

**3. Field Validation Steps**
- Store validation results
- Access via `stepOutput.{alias}.validation_errors`

**File**: [services/executors/validation/field_validation_executor.go](services/executors/validation/field_validation_executor.go)

```go
outputData := map[string]interface{}{
    "validation_errors": validationErrors,
    "is_valid":         len(validationErrors) == 0,
    "fields_validated": len(step.Config["rules"].([]interface{})),
}

e.SetStepOutput(execContext, step, outputData)
```

### 📋 Step Output Structure

Each step output contains:

```javascript
{
    "stepID": "uuid-123",
    "stepName": "Database Enrichment MySQL",
    "stepAlias": "mysql_patient_lookup",  // User-friendly name
    "stepType": "database_enrichment",
    "namespace": "database_enrichment_mysql_uuid123",
    "sequence": 100,
    "outputData": {
        // Step-specific output data
        "enriched_data": [...],
        "rows_count": 5,
        "status": "success"
    },
    "success": true
}
```

## Using Step Outputs in Query Parameters

### Example: Use Database Step Output in API Call

**Step 1: Database Enrichment (Sequence 10)**
- **Alias**: `patient_lookup`
- **Query**: `SELECT insurance_id FROM patients WHERE mrn = ?`
- **Output**:
  ```javascript
  {
    "enriched_data": [
      { "insurance_id": "INS123456" }
    ],
    "rows_count": 1
  }
  ```

**Step 2: API Enrichment (Sequence 20)**
- **Alias**: `insurance_verification`
- **URL**: `https://api.insurance.com/verify`
- **Query Parameters**:
  - Key: `policy_id`
  - Value: `stepOutput.patient_lookup.enriched_data[0].insurance_id`

**Result**: API call uses `INS123456` extracted from database step

### Example: Conditional Execution Based on Previous Step

**Step 1: Validation (Sequence 10)**
- **Alias**: `patient_validation`
- **Output**:
  ```javascript
  {
    "validation_errors": [],
    "is_valid": true
  }
  ```

**Step 2: Custom JavaScript (Sequence 20)**
```javascript
function transform(input, stepOutput) {
    // Check if validation passed
    if (!stepOutput.patient_validation.is_valid) {
        throw new Error("Patient validation failed");
    }

    // Continue with transformation
    return input;
}
```

## Field Path Resolution

The system supports multiple path formats for accessing data:

### 1. HL7 Field Paths (Simplified)
```javascript
PID.3          // Patient MRN
PID.5.1        // Patient Last Name
PV1.19         // Visit Number
```

### 2. Step Output Paths
```javascript
stepOutput.database_lookup.enriched_data[0].patient_name
stepOutput.api_call.response.insurance_status
```

### 3. Current Message Data
```javascript
enhancedSegments.PID.fields[3].value  // Legacy format
message.patient.name                   // Custom format
```

### 4. Nested Paths
```javascript
// Supports deep nesting
stepOutput.step1.data.nested.deeply.value
stepOutput.api_call.response.patient.insurance[0].policy_number
```

## UI Integration (Planned Enhancement)

### Field Path Search Component Enhancement

Currently, `FieldPathSearchComponent` shows HL7 fields. We should add **Step Output** category:

```javascript
{
    name: "Database Lookup - Enriched Data",
    path: "stepOutput.database_lookup.enriched_data",
    description: "Data from Database Enrichment step",
    category: "Step Outputs"
},
{
    name: "API Call - Response",
    path: "stepOutput.api_verification.response",
    description: "Response from API Enrichment step",
    category: "Step Outputs"
}
```

### Visual Pipeline Flow

Show data flow between steps in the visual builder:

```
[Database Enrichment] → enriched_data
         ↓
[API Enrichment] ← Uses: stepOutput.database_lookup.enriched_data
         ↓
[Custom Transform] ← Uses: stepOutput.api_enrichment.response
```

## Backend Implementation Details

### PipelineExecutionContext Structure

**File**: [models/pipeline_models.go](models/pipeline_models.go)

```go
type PipelineExecutionContext struct {
    PipelineID    string
    InterfaceID   string
    MessageID     string
    CurrentData   map[string]interface{}  // Current message data
    StepOutputs   map[string]StepOutput   // All step outputs
    Metadata      map[string]interface{}
    StartTime     time.Time
    LastStepTime  time.Time
}

type StepOutput struct {
    StepID     string
    StepName   string
    StepAlias  string
    StepType   string
    Namespace  string
    Sequence   int
    OutputData map[string]interface{}
    Success    bool
}
```

### Step Namespace Generation

```go
// Format: stepType_stepName_stepID
func GenerateStepNamespace(stepName string, stepID string, stepAlias *string) string {
    safeName := strings.ToLower(strings.ReplaceAll(stepName, " ", "_"))
    return fmt.Sprintf("%s_%s", safeName, stepID)
}

// Default alias from step name
func GenerateDefaultAlias(stepName string) string {
    return strings.ToLower(strings.ReplaceAll(stepName, " ", "_"))
}
```

### Accessing Step Output in Executors

```go
// By alias (user-friendly)
data, err := e.GetStepOutputByAlias(execContext, "database_lookup")
if err != nil {
    return fmt.Errorf("previous step not found: %w", err)
}

enrichedData := data["enriched_data"]

// By namespace (full path)
data, exists := e.GetStepOutput(execContext, "database_enrichment_mysql_uuid123")
if !exists {
    return fmt.Errorf("step output not found")
}
```

## Practical Use Cases

### Use Case 1: Multi-Database Enrichment

```
Step 1 (Seq 10): MySQL - Get patient insurance ID
  Output: { insurance_id: "INS123" }

Step 2 (Seq 20): PostgreSQL - Get insurance details using ID from Step 1
  Query Parameter: stepOutput.step1.enriched_data[0].insurance_id
  Output: { insurance_details: {...} }

Step 3 (Seq 30): MongoDB - Log combined data
  Input: Both stepOutput.step1 and stepOutput.step2
```

### Use Case 2: API-Database Hybrid

```
Step 1 (Seq 10): API - Verify patient exists in EHR
  Output: { patient_id: "P123", verified: true }

Step 2 (Seq 20): Database - Only if verified, fetch detailed records
  Condition: stepOutput.api_verification.verified === true
  Query Parameter: stepOutput.api_verification.patient_id
  Output: { patient_details: {...} }
```

### Use Case 3: Validation-Enrichment Chain

```
Step 1 (Seq 10): Validation - Check required fields
  Output: { is_valid: true, missing_fields: [] }

Step 2 (Seq 20): Database Enrichment - Only if valid
  Precondition: stepOutput.validation.is_valid
  Output: { enriched_data: [...] }

Step 3 (Seq 30): Custom JavaScript - Combine validation + enrichment
  Uses: stepOutput.validation + stepOutput.database_enrichment
```

## Testing Step Output Chaining

### Manual Testing Steps

1. **Create Pipeline with 2+ Steps**:
   - Step 1: Database Enrichment (Alias: `patient_lookup`)
   - Step 2: API Enrichment

2. **Configure Step 2 to Use Step 1 Output**:
   - In Step 2 query parameters
   - Value: `stepOutput.patient_lookup.enriched_data[0].insurance_id`

3. **Test with Sample Message**:
   - Send HL7 message through pipeline
   - Check logs for step output storage
   - Verify Step 2 receives correct value

### Expected Log Output

```
✅ [database_enrichment_mysql] Completed in 50ms
📦 Step Output Stored: database_enrichment_mysql_uuid123
   - enriched_data: [{"insurance_id": "INS123"}]
   - rows_count: 1

🔄 [api_enrichment_insurance] Starting execution
📥 Resolving parameter: stepOutput.patient_lookup.enriched_data[0].insurance_id
✅ Resolved to: INS123
🌐 API Call: GET https://api.insurance.com/verify?policy_id=INS123
```

## Future Enhancements

### 1. Visual Step Output Inspector
Add UI panel showing available step outputs in real-time:

```
Step Outputs Available:
├─ patient_lookup (database_enrichment)
│  ├─ enriched_data: Array(1)
│  └─ rows_count: 1
│
├─ insurance_verification (api_enrichment)
│  ├─ response: Object
│  └─ status_code: 200
```

### 2. Smart Autocomplete
When configuring step parameters, show available step outputs:

```
Query Parameter Value:
┌─────────────────────────────────────┐
│ stepOutput.                         │
│ ├─ patient_lookup                   │ ← Clickable
│ │  ├─ enriched_data[0]             │
│ │  │  └─ insurance_id              │ ← Select this
│ │  └─ rows_count                   │
│ └─ api_verification                 │
│    └─ response                      │
└─────────────────────────────────────┘
```

### 3. Step Dependency Graph
Visual representation of step dependencies:

```
     ┌──────────────┐
     │   PID.3      │ (HL7 Field)
     └───────┬──────┘
             ↓
     ┌──────────────┐
     │ DB Lookup    │ Step 1
     └───────┬──────┘
             ↓ enriched_data
     ┌──────────────┐
     │ API Call     │ Step 2
     └───────┬──────┘
             ↓ response
     ┌──────────────┐
     │ Transform    │ Step 3
     └──────────────┘
```

## Documentation References

- [STEP_OUTPUT_TRACKING.md](STEP_OUTPUT_TRACKING.md) - Original step output implementation
- [PIPELINE_EXECUTION_CONTEXT.md](PIPELINE_EXECUTION_CONTEXT.md) - Execution context details
- [FIELD_PATH_RESOLUTION.md](FIELD_PATH_RESOLUTION.md) - Path resolution algorithms

## Summary

✅ **Implemented**: Step output storage and retrieval
✅ **Supported**: All executor types can store/access outputs
✅ **Accessible**: Via alias, namespace, or GetNestedValue
✅ **Tested**: Database, API, and Validation executors
📋 **Next**: UI enhancements for visual step output chaining
🎯 **Goal**: No-code data flow orchestration between pipeline steps
