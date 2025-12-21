# How to Configure Validation Mode in the UI

## 📍 Step-by-Step Guide

### Method 1: Pipeline Builder UI (Recommended)

#### Step 1: Navigate to Pipeline Builder
1. Go to your ezHealthKonnect dashboard
2. Click on **Interfaces** in the navigation menu
3. Select the interface you want to configure
4. Click **Configure Pipeline** or **Edit Pipeline**
5. You'll see the **Pipeline Builder** page

#### Step 2: Add or Edit Validation Step
1. In the **Toolbox** panel (left side), find the **Pre-Processing** section
2. Locate **"Field Validation"** step
3. Either:
   - **Drag and drop** it into the pipeline canvas (to add new)
   - **Click on existing** validation step in the canvas (to edit)

#### Step 3: Configure Validation Mode
When the **Step Properties Modal** opens, you'll see:

```
┌─────────────────────────────────────────────────────────────┐
│ Step Configuration: Field Validation                    [×] │
├─────────────────────────────────────────────────────────────┤
│ [Form] [JSON] [Documentation]                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Validation Mode: *                                      │ │
│ │ ┌─────────────────────────────────────────────────────┐ │ │
│ │ │ ⚠️ Accept & Flag - Send ACK with warnings...    ▼  │ │ │
│ │ └─────────────────────────────────────────────────────┘ │ │
│ │                                                         │ │
│ │ Controls ACK/NACK response and pipeline behavior when  │ │
│ │ validation fails. Strict Reject = NACK + stop...       │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Validation Rules: *                                     │ │
│ │                                                         │ │
│ │ [Rule 1]                                                │ │
│ │ Field Path: PID.3                                       │ │
│ │ Type: Required                                          │ │
│ │ Error: Patient ID is required                           │ │
│ │                                                   [×]    │ │
│ │                                                         │ │
│ │ [+ Add Rule]                                            │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│                                    [Save] [Cancel]          │
└─────────────────────────────────────────────────────────────┘
```

#### Step 4: Select Validation Mode
Click the **Validation Mode** dropdown and choose:

**Option 1: ❌ Strict Reject - Send NACK on failure, stop processing**
- Description: Critical interfaces: Pipeline fails immediately, NACK sent to sender
- Use for: Patient safety, lab results, medications

**Option 2: ⚠️ Accept & Flag - Send ACK with warnings, continue processing (DEFAULT)**
- Description: Data quality monitoring: Message accepted with warnings, ACK sent
- Use for: Non-critical interfaces, gradual validation rollout

**Option 3: ⏭️ No Validation - Skip all checks**
- Description: Emergency bypass: All validation skipped, ACK sent immediately
- Use for: Debugging or when validation is not needed

#### Step 5: Configure Validation Rules
Below the validation mode, you'll see the **Validation Rules** section:
1. Click **[+ Add Rule]** to add validation rules
2. For each rule, configure:
   - **Field Path**: Select from autocomplete (e.g., PID.3, MSH.9, PID.5.1)
   - **Validation Type**: Required, Format, Length, or Pattern
   - **Error Message**: Auto-populated or custom
   - **Type-specific options** (format preset, regex, min/max length)

#### Step 6: Save Configuration
1. Click **[Save]** button in the modal
2. Click **[Save Pipeline]** in the top-right corner
3. Your validation mode is now configured at the step level!

---

## 🎯 Validation Mode Dropdown Options

### Option 1: Strict Reject (NACK on Failure)
```
Value: strict_reject
Label: ❌ Strict Reject - Send NACK on failure, stop processing
Description: Critical interfaces: Pipeline fails immediately, NACK sent to sender.
             Use for patient safety, lab results, medications.
```

**Behavior when validation fails**:
- ❌ **NACK sent** to sender
- 🛑 **Pipeline STOPS** immediately
- ❌ **No further processing**
- 📊 Message status: `"rejected"`
- 🔴 Error logged to audit trail

**Example Response**:
```
MSH|^~\&|ezHealthKonnect|Integration|SendingApp|...|20231214120000||ACK^A01|ACK123|P|2.5
MSA|AR|MSG00001|Validation failed: Patient ID is required
ERR|||207^Application internal error^HL70357
```

---

### Option 2: Accept & Flag (ACK with Warnings) - DEFAULT
```
Value: accept_and_flag
Label: ⚠️ Accept & Flag - Send ACK with warnings, continue processing (DEFAULT)
Description: Data quality monitoring: Message accepted with warnings, ACK sent.
             Use for non-critical interfaces, gradual validation rollout.
```

**Behavior when validation fails**:
- ✅ **ACK sent** to sender
- ▶️ **Pipeline CONTINUES** processing
- ⚠️ **Warnings logged** but not blocking
- 📊 Message status: `"warning"`
- 🟡 `_requires_review: true` flag added
- 📋 Validation warnings in `_validation_warnings`

**Example Response**:
```
MSH|^~\&|ezHealthKonnect|Integration|SendingApp|...|20231214120000||ACK^A01|ACK123|P|2.5
MSA|AA|MSG00001|Message accepted with validation warnings
ERR|||0^Message accepted^HL70357|||W
NTE|1||Validation Warning: Patient email format invalid
```

---

### Option 3: No Validation (Skip All Checks)
```
Value: no_validation
Label: ⏭️ No Validation - Skip all checks
Description: Emergency bypass: All validation skipped, ACK sent immediately.
             Use for debugging or when validation is not needed.
```

**Behavior**:
- ⏭️ **All validation skipped**
- ✅ **ACK sent** immediately
- ▶️ **Pipeline CONTINUES** normally
- 📊 No validation status added

---

## 📂 Where Configuration is Stored

### Database Storage
The validation mode is stored in the `transformation_steps` table in the `config` JSONB column:

```sql
-- Example stored configuration
{
  "validation_mode": "strict_reject",  -- or "accept_and_flag" or "no_validation"
  "rules": [
    {
      "type": "required",
      "field": "PID.3",
      "errorMessage": "Patient ID is required"
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

### Query Current Configuration
```sql
SELECT
    ts.step_name,
    ts.config->>'validation_mode' as validation_mode,
    jsonb_array_length(ts.config->'rules') as rules_count,
    tp.pipeline_name
FROM transformation_steps ts
JOIN transformation_pipelines tp ON ts.pipeline_id = tp.id
WHERE ts.step_type = 'pre.validation';
```

---

## 🔧 Alternative Configuration Methods

### Method 2: Direct SQL Update
```sql
-- Change to strict_reject
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{validation_mode}',
    '"strict_reject"'
)
WHERE step_type = 'pre.validation'
AND pipeline_id = 'YOUR_PIPELINE_ID';

-- Change to accept_and_flag
UPDATE transformation_steps
SET config = jsonb_set(
    config,
    '{validation_mode}',
    '"accept_and_flag"'
)
WHERE step_type = 'pre.validation'
AND pipeline_id = 'YOUR_PIPELINE_ID';
```

### Method 3: CLI Utility
```bash
# List all pipelines with validation
node change_validation_mode.js --list

# Change validation mode
node change_validation_mode.js <pipeline_id> strict_reject
node change_validation_mode.js <pipeline_id> accept_and_flag
node change_validation_mode.js <pipeline_id> no_validation
```

### Method 4: REST API
```bash
# Get current step configuration
curl http://localhost:3000/api/pipelines/:pipelineId/steps/:stepId

# Update validation mode
curl -X PUT http://localhost:3000/api/pipelines/:pipelineId/steps/:stepId \
  -H "Content-Type: application/json" \
  -d '{
    "config": {
      "validation_mode": "strict_reject",
      "rules": [...]
    }
  }'
```

---

## 🧪 Testing Your Configuration

After configuring validation mode, test it:

### Test 1: Valid Message
```bash
node test_validation_success.js
# Should: ✅ Pass all validations
# Should: ACK sent (all modes)
```

### Test 2: Invalid Message (Strict Reject)
```bash
# First, set mode to strict_reject
node change_validation_mode.js <pipeline_id> strict_reject

# Then test
node test_strict_reject.js
# Should: ❌ Pipeline fails
# Should: NACK sent
# Should: Status = "rejected"
```

### Test 3: Invalid Message (Accept & Flag)
```bash
# First, set mode to accept_and_flag
node change_validation_mode.js <pipeline_id> accept_and_flag

# Then test
node test_accept_and_flag.js
# Should: ✅ Pipeline succeeds
# Should: ACK sent with warnings
# Should: Status = "warning"
```

---

## 📋 Quick Reference

| Where | What | How |
|-------|------|-----|
| **UI** | Pipeline Builder → Validation Step → Validation Mode dropdown | Drag validation step, configure in modal |
| **Database** | `transformation_steps.config->>'validation_mode'` | SQL UPDATE with `jsonb_set()` |
| **CLI** | `change_validation_mode.js` | `node change_validation_mode.js <id> <mode>` |
| **API** | `PUT /api/pipelines/:id/steps/:stepId` | Update `config.validation_mode` |

---

## 💡 Best Practices

1. **Start with `accept_and_flag`** (default)
   - Monitor validation warnings for 1-2 weeks
   - Understand failure patterns
   - Refine rules as needed

2. **Gradually move to `strict_reject`**
   - Once validation rules are stable
   - For critical interfaces only
   - Document the decision

3. **Per-Interface Configuration**
   - Lab results → `strict_reject`
   - Demographic updates → `accept_and_flag`
   - Historical data load → `no_validation`

4. **Document Your Choices**
   - Add comments in pipeline description
   - Record why each mode was chosen
   - Review periodically

---

## ❓ FAQ

**Q: Can I have different validation modes for different interfaces?**
A: Yes! Each pipeline has its own validation step configuration. Configure each interface independently.

**Q: Can I change the mode without restarting the application?**
A: Yes! Changes to validation mode are read from the database at runtime. No restart needed.

**Q: What happens if I don't specify a validation mode?**
A: The default is `accept_and_flag` (lenient mode with ACK + warnings).

**Q: Can I temporarily disable validation?**
A: Yes! Set validation mode to `no_validation` or disable the entire validation step.

**Q: Where can I see validation results?**
A: Check:
- Application logs: `docker compose logs app | grep validation`
- Database: `validation_feedback` table (if implemented)
- Message metadata: `_validation_status`, `_validation_warnings` fields

---

## 📞 Support

For questions:
- Check [VALIDATION_SYSTEM_SUMMARY.md](VALIDATION_SYSTEM_SUMMARY.md) - Complete system documentation
- Check [VALIDATION_MODE_CONFIGURATION_GUIDE.md](VALIDATION_MODE_CONFIGURATION_GUIDE.md) - Detailed mode explanations
- Run test suite: `node test_validation_pipeline.js`
- View logs: `docker compose logs app | grep validation`
