# Validation Mode Configuration Guide

## Overview
The validation system supports 3 modes that control ACK/NACK behavior when validation fails.

## 🎯 Three Validation Modes

### 1. **Strict Reject** (`strict_reject`)
**Behavior**: Send NACK, Stop Processing

```
Validation Failed
    ↓
❌ NACK sent to sender
    ↓
Pipeline STOPS (no further processing)
    ↓
Message status: "rejected"
    ↓
Errors logged for review
```

**Use Cases**:
- Critical healthcare interfaces (lab results, medications)
- Regulatory compliance requirements
- Patient safety scenarios
- Production environments with strict data quality requirements

**Example Response**:
```
MSH|^~\&|ReceivingApp|...|SendingApp|...|20231214120000||ACK^A01|ACK123|P|2.5
MSA|AR|MSG00001|Validation failed: Patient ID is required
ERR|||207^Application internal error^HL70357
```

### 2. **Accept and Flag** (`accept_and_flag`) - DEFAULT
**Behavior**: Send ACK, Continue Processing with Warnings

```
Validation Failed
    ↓
✅ ACK sent to sender
    ↓
Pipeline CONTINUES (all steps execute)
    ↓
Message status: "warning"
    ↓
_validation_warnings flag added
    ↓
_requires_review = true
    ↓
Data quality team notified
```

**Use Cases**:
- Non-critical interfaces
- Data quality monitoring
- Gradual rollout of new validation rules
- Development/testing environments
- Historical data migration

**Example Response**:
```
MSH|^~\&|ReceivingApp|...|SendingApp|...|20231214120000||ACK^A01|ACK123|P|2.5
MSA|AA|MSG00001|Message accepted with validation warnings
ERR|||0^Message accepted^HL70357|||W
NTE|1||Validation Warning: Patient email format invalid
NTE|2||Message flagged for manual review
```

### 3. **No Validation** (`no_validation`)
**Behavior**: Skip All Validation

```
Validation Step Skipped
    ↓
✅ ACK sent (no validation performed)
    ↓
Pipeline CONTINUES normally
    ↓
No validation status added
```

**Use Cases**:
- Troubleshooting/debugging
- Emergency bypass scenarios
- Interfaces with external validation
- Legacy system integration

---

## 📝 Configuration Methods

### Method 1: Database Configuration (Current)

The validation mode is stored in the `transformation_steps` table:

```sql
-- View current configuration
SELECT step_name, config->'validation_mode' as mode
FROM transformation_steps
WHERE step_type = 'pre.validation';

-- Update validation mode to strict_reject
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{validation_mode}',
    '"strict_reject"'
)
WHERE step_type = 'pre.validation'
AND pipeline_id = 'YOUR_PIPELINE_ID';

-- Update to accept_and_flag (lenient)
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{validation_mode}',
    '"accept_and_flag"'
)
WHERE step_type = 'pre.validation'
AND pipeline_id = 'YOUR_PIPELINE_ID';
```

### Method 2: Pipeline Builder UI (Recommended)

**Location**: Pipeline Builder → Validation Step Configuration

**Steps**:
1. Navigate to **Pipeline Builder** (`/pipeline-builder.html`)
2. Select or create a pipeline
3. Add/Edit "Field Validation" step
4. Find **Validation Mode** dropdown:

```
┌─────────────────────────────────────────────────┐
│ Validation Mode:                                │
│ ┌─────────────────────────────────────────────┐ │
│ │ Strict Reject (NACK on failure)         ▼  │ │
│ └─────────────────────────────────────────────┘ │
│                                                 │
│ Options:                                        │
│  • Strict Reject (NACK on failure)              │
│  • Accept and Flag (ACK with warnings) [DEFAULT]│
│  • No Validation (Skip all checks)              │
└─────────────────────────────────────────────────┘
```

5. Select desired mode
6. Click **Save Pipeline**

### Method 3: API Configuration (Programmatic)

**Endpoint**: `POST /api/pipelines/:pipelineId/steps`

**Request Body**:
```json
{
  "step_name": "Field Validation",
  "step_type": "pre.validation",
  "sequence": 10,
  "layer": "pre",
  "enabled": true,
  "config": {
    "validation_mode": "strict_reject",
    "rules": [
      {
        "type": "required",
        "field": "PID.3",
        "errorMessage": "Patient ID is required"
      }
    ]
  }
}
```

**Example with cURL**:
```bash
curl -X POST http://localhost:3000/api/pipelines/YOUR_PIPELINE_ID/steps \
  -H "Content-Type: application/json" \
  -d '{
    "step_name": "Field Validation",
    "step_type": "pre.validation",
    "config": {
      "validation_mode": "strict_reject",
      "rules": [...]
    }
  }'
```

---

## 🔄 ACK/NACK Response Format

### NACK Response (strict_reject mode)
```
MSH|^~\&|ezHealthKonnect|Integration|SendingApp|SendingFacility|20231214120000||ACK^A01|ACK456|P|2.5
MSA|AR|MSG00001|Validation failed: Patient ID is required; Date of birth format invalid
ERR|||207^Application internal error^HL70357
```

**Fields**:
- `MSA-1`: `AR` (Application Reject)
- `MSA-2`: Original message control ID
- `MSA-3`: Detailed validation error message
- `ERR`: Error details

### ACK with Warnings (accept_and_flag mode)
```
MSH|^~\&|ezHealthKonnect|Integration|SendingApp|SendingFacility|20231214120000||ACK^A01|ACK456|P|2.5
MSA|AA|MSG00001|Message accepted with validation warnings
ERR|||0^Message accepted^HL70357|||W
NTE|1||Validation Warning: Patient email format invalid
NTE|2||Validation Warning: MRN length outside expected range
```

**Fields**:
- `MSA-1`: `AA` (Application Accept)
- `MSA-2`: Original message control ID
- `MSA-3`: Acceptance confirmation
- `ERR`: Warning severity (`W`)
- `NTE`: Warning details (one per validation error)

---

## 🎛️ Additional Configuration Options

### Stop on First Error
Continue validation after first error, or stop immediately?

```json
{
  "validation_mode": "strict_reject",
  "stopOnFirstError": true,  // Stop after first validation failure
  "rules": [...]
}
```

**Default**: `false` (validate all rules, return all errors)

### Add Field Names to Errors
Automatically prepend field names to error messages?

```json
{
  "validation_mode": "strict_reject",
  "addFieldNames": true,  // "[Patient ID] Patient ID is required"
  "rules": [...]
}
```

**Default**: `false`

---

## 📊 Configuration Examples

### Example 1: Critical Interface (Strict)
```json
{
  "validation_mode": "strict_reject",
  "stopOnFirstError": false,
  "addFieldNames": true,
  "rules": [
    {
      "type": "required",
      "field": "PID.3",
      "errorMessage": "Patient ID is required"
    },
    {
      "type": "required",
      "field": "PID.5.1",
      "errorMessage": "Patient last name is required"
    },
    {
      "type": "format",
      "field": "PID.7",
      "format": "date",
      "errorMessage": "Date of birth must be YYYYMMDD format"
    }
  ]
}
```

### Example 2: Data Quality Monitoring (Lenient)
```json
{
  "validation_mode": "accept_and_flag",
  "stopOnFirstError": false,
  "addFieldNames": false,
  "rules": [
    {
      "type": "format",
      "field": "PID.13",
      "format": "phone",
      "errorMessage": "Phone number format recommended"
    },
    {
      "type": "pattern",
      "field": "PID.18",
      "regex": "^[A-Z0-9]{6,12}$",
      "errorMessage": "Account number format recommended"
    }
  ]
}
```

---

## 🔍 Monitoring & Debugging

### Check Current Validation Mode
```javascript
// Node.js script
const { Pool } = require('pg');
const pool = new Pool({ /* connection config */ });

pool.query(`
  SELECT
    ts.step_name,
    ts.config->>'validation_mode' as validation_mode,
    tp.pipeline_name,
    tp.interface_id
  FROM transformation_steps ts
  JOIN transformation_pipelines tp ON ts.pipeline_id = tp.id
  WHERE ts.step_type = 'pre.validation'
`).then(r => {
  console.log('Validation Configurations:');
  console.table(r.rows);
  pool.end();
});
```

### View Validation Results in Logs
```bash
# View validation feedback
docker compose logs app | grep -E "validation|NACK|ACK"

# Filter by mode
docker compose logs app | grep "Strict Validation"  # strict_reject
docker compose logs app | grep "Accept & Flag"      # accept_and_flag
```

### Query Validation Feedback
```sql
-- If validation_feedback table exists
SELECT
  message_id,
  validation_mode,
  validation_status,
  error_count,
  errors,
  created_at
FROM validation_feedback
WHERE validation_status = 'rejected'
ORDER BY created_at DESC
LIMIT 10;
```

---

## 🎯 Quick Reference

| Mode | ACK/NACK | Processing | Status | Use Case |
|------|----------|------------|--------|----------|
| `strict_reject` | ❌ NACK | Stops | `rejected` | Critical interfaces |
| `accept_and_flag` | ✅ ACK | Continues | `warning` | Data quality monitoring |
| `no_validation` | ✅ ACK | Continues | - | Emergency bypass |

---

## 🚀 Best Practices

1. **Start Lenient**: Use `accept_and_flag` initially to understand validation failure patterns
2. **Gradual Enforcement**: Move to `strict_reject` once validation rules are refined
3. **Per-Interface Configuration**: Different interfaces may need different modes
4. **Monitor Warnings**: Review `accept_and_flag` warnings regularly
5. **Document Decisions**: Record why each interface uses its validation mode
6. **Test Thoroughly**: Test both modes with sample messages before production

---

## 📞 Support

For questions about validation configuration:
- Check logs: `docker compose logs app | grep validation`
- Review test suite: `node test_validation_pipeline.js`
- Read full documentation: `VALIDATION_SYSTEM_SUMMARY.md`
