# If-Then-Else Conditional Logic - User Guide

**Version:** 1.0
**Date:** December 28, 2025
**Status:** ✅ Ready for Use

---

## Overview

The **If-Then-Else** step provides powerful no-code conditional logic for your transformation pipelines. Build complex decision trees without writing any code using our visual builder.

### Key Features:
- ✅ Visual condition builder (no coding required)
- ✅ Cross-field comparison support
- ✅ 9 different action types
- ✅ Multiple conditions per step
- ✅ Real-time validation
- ✅ Color-coded UI (navy blue & pastel pink theme)

---

## When to Use If-Then-Else

Use this step when you need to:

1. **Route Messages Based on Conditions**
   - Send emergency patients to high-priority queue
   - Route different message types to different destinations

2. **Validate Data**
   - Check discharge date > admit date
   - Validate patient ID consistency across segments

3. **Enrich Data Conditionally**
   - Add metadata for geriatric patients
   - Flag VIP patients for special handling

4. **Handle Edge Cases**
   - Log warnings for data quality issues
   - Set default values when fields are missing

---

## UI Components

### Condition Builder

Configure the "IF" part of your logic:

| Field | Description | Example |
|-------|-------------|---------|
| **Field** | HL7 path or JSON path to check | `PID.8`, `patient.age` |
| **Operator** | Comparison type | `equals`, `greater_than`, `contains` |
| **Compare To** | Value OR another field | `'M'` or `PV1.44` |

**Operators Available:**
- `equals` / `not_equals`
- `greater_than` / `greater_than_or_equal`
- `less_than` / `less_than_or_equal`
- `contains` / `not_contains`
- `matches_regex`
- `is_empty` / `is_not_empty`

### Action Builder

Configure the "THEN" and "ELSE" actions:

| Action | Description | Use Case |
|--------|-------------|----------|
| **Continue** | Proceed to next step | Normal flow |
| **Reject** | Stop processing, return error | Validation failure |
| **Log Warning** | Log warning, optionally continue | Data quality issue |
| **Log Error** | Log error, optionally continue | Non-critical error |
| **Set Metadata** | Add metadata to message | Routing, priority flags |
| **Set Field** | Update field value | Data correction |
| **Copy Field** | Copy value from one field to another | Data consolidation |
| **Delete Field** | Remove field from message | PHI removal |
| **Route To** | Route to specific destination/queue | Message routing |

---

## Examples

### Example 1: Age-Based Priority Routing

**Scenario:** Send patients over 65 to geriatrics queue with high priority

**Configuration:**

**IF Condition:**
- Field: `patient.age`
- Operator: `greater_than`
- Value: `65`

**THEN Action (onTrue):**
- Action: `Set Metadata`
- Metadata:
  ```json
  {
    "priority": "high",
    "routing": "geriatrics",
    "age_group": "senior"
  }
  ```

**ELSE Action (onFalse):**
- Action: `Continue`

**Result:**
- Patients over 65 → High priority, routed to geriatrics
- Other patients → Continue normal processing

---

### Example 2: Cross-Field Date Validation

**Scenario:** Validate discharge date is after admit date

**Configuration:**

**IF Condition:**
- Field: `PV1.45` (discharge date)
- Operator: `less_than_or_equal`
- Compare To Field: `PV1.44` (admit date)
- ☑️ **Compare to another field** (checked)

**THEN Action (onTrue):**
- Action: `Reject`
- Error Message: `Discharge date must be after admit date`
- Severity: `error`

**ELSE Action (onFalse):**
- Action: `Continue`

**Result:**
- Invalid dates → Message rejected with error
- Valid dates → Continue processing

---

### Example 3: Patient ID Consistency Check

**Scenario:** Log warning if patient IDs don't match between segments

**Configuration:**

**IF Condition:**
- Field: `PID.3.1`
- Operator: `not_equals`
- Compare To Field: `PV1.5.1`
- ☑️ **Compare to another field** (checked)

**THEN Action (onTrue):**
- Action: `Log Warning`
- Message: `Patient ID mismatch between PID and PV1`
- ☑️ **Continue processing after logging** (checked)

**ELSE Action (onFalse):**
- Action: `Continue`

**Result:**
- Mismatch → Warning logged, processing continues
- Match → Silent continue

---

### Example 4: Message Type Routing

**Scenario:** Route ADT^A04 messages to registration system

**Configuration:**

**IF Condition:**
- Field: `messageType`
- Operator: `equals`
- Value: `ADT^A04`

**THEN Action (onTrue):**
- Action: `Route To`
- Destination: `registration`
- Queue: `patient-registration`

**ELSE Action (onFalse):**
- Action: `Continue`

**Result:**
- ADT^A04 → Routed to registration queue
- Other types → Continue to default destination

---

### Example 5: Gender Code Normalization

**Scenario:** Convert HL7 gender codes to FHIR values

**Configuration:**

**IF Condition:**
- Field: `PID.8`
- Operator: `equals`
- Value: `M`

**THEN Action (onTrue):**
- Action: `Set Field`
- Field: `patient.gender`
- Value: `male`

**ELSE Action (onFalse):**
- Action: `Set Field`
- Field: `patient.gender`
- Value: `female`

**Result:**
- `M` → `male`
- Anything else → `female`

---

### Example 6: Empty Field Handling

**Scenario:** Set default value when field is missing

**Configuration:**

**IF Condition:**
- Field: `PID.13` (phone number)
- Operator: `is_empty`

**THEN Action (onTrue):**
- Action: `Set Field`
- Field: `patient.telecom.phone`
- Value: `000-000-0000`

**ELSE Action (onFalse):**
- Action: `Copy Field`
- Source Field: `PID.13`
- Target Field: `patient.telecom.phone`

**Result:**
- Empty → Default phone
- Has value → Copy actual phone

---

## Multiple Conditions

You can add **multiple conditions** to handle complex scenarios:

### Example: VIP Patient Handling

**Condition 1:**
- IF `patient.vipFlag` equals `true`
- THEN Set Metadata: `{"priority": "vip", "routing": "vip-care"}`
- ELSE Continue

**Condition 2:**
- IF `patient.age` greater_than `65`
- THEN Set Metadata: `{"priority": "high", "routing": "geriatrics"}`
- ELSE Continue

**Condition 3:**
- IF `visit.type` equals `Emergency`
- THEN Set Metadata: `{"priority": "urgent", "routing": "emergency"}`
- ELSE Continue

**Result:** All conditions execute sequentially, allowing layered logic.

---

## Best Practices

### 1. Use Descriptive Condition Names
```
❌ "Condition 1"
✅ "Validate Discharge Date"
✅ "Check Age for Geriatric Routing"
```

### 2. Order Conditions Logically
- Validation checks first (reject early if invalid)
- Routing logic second
- Enrichment last

### 3. Use Continue for Default Flow
- ELSE: Continue → Safe default
- Only use complex ELSE actions when necessary

### 4. Leverage Cross-Field Comparison
- Compare dates: discharge > admit
- Compare IDs: PID.3 == PV1.5
- Compare values: observed > expected

### 5. Log Before Rejecting
```
Condition 1: Check validity
  THEN: Log Error "Invalid discharge date"
  ELSE: Continue

Condition 2: Enforce validation
  IF: same_check_as_condition_1
  THEN: Reject
  ELSE: Continue
```

### 6. Use Set Metadata for Routing
```json
{
  "priority": "high",
  "routing": "destination-name",
  "reason": "age > 65"
}
```

---

## Testing Your Conditions

### Test Strategy:

1. **Test True Path:**
   - Create test message that matches condition
   - Verify THEN action executes

2. **Test False Path:**
   - Create test message that doesn't match
   - Verify ELSE action executes

3. **Test Edge Cases:**
   - Empty fields
   - Null values
   - Missing segments

### Example Test Cases:

```javascript
// Test Case 1: Age > 65
{
  "patient": { "age": 70 },
  "expected": {
    "metadata": { "priority": "high", "routing": "geriatrics" }
  }
}

// Test Case 2: Age <= 65
{
  "patient": { "age": 45 },
  "expected": {
    "metadata": {}  // No changes
  }
}

// Test Case 3: Age missing
{
  "patient": {},
  "expected": {
    "metadata": {}  // Handles gracefully
  }
}
```

---

## Troubleshooting

### Condition Not Triggering

**Problem:** THEN action never executes

**Solutions:**
1. Check field path is correct (case-sensitive)
2. Verify operator matches data type (e.g., numeric > for numbers)
3. Check value matches exactly (quotes matter for strings)
4. Use browser console to inspect field value

### Cross-Field Comparison Not Working

**Problem:** Field comparison always returns false

**Solutions:**
1. Ensure both fields exist
2. Check both fields have same data type
3. Verify field paths are correct
4. ☑️ Ensure "Compare to another field" checkbox is checked

### Metadata Not Setting

**Problem:** Metadata action doesn't add metadata

**Solutions:**
1. Verify JSON syntax is valid
2. Check for trailing commas in JSON
3. Ensure metadata is object, not string
4. Use JSON validator to check syntax

---

## Performance Tips

1. **Order Matters:**
   - Put most common conditions first
   - Early rejection saves processing time

2. **Combine Related Conditions:**
   - Use multiple conditions in one step for related logic
   - Use separate steps for unrelated logic

3. **Avoid Redundant Checks:**
   - Don't repeat same condition across steps
   - Use metadata to cache validation results

---

## Advanced Patterns

### Pattern 1: Tiered Priority

```
Condition 1: IF vipFlag == true → priority: "vip"
Condition 2: IF age > 75 → priority: "high"
Condition 3: IF age > 65 → priority: "medium"
```

### Pattern 2: Data Quality Scoring

```
Condition 1: IF missing_fields > 5 → quality: "poor"
Condition 2: IF missing_fields > 2 → quality: "medium"
Condition 3: IF missing_fields > 0 → quality: "good"
```

### Pattern 3: Multi-Destination Routing

```
Condition 1: IF messageType == "ADT^A01" → route: "admission"
Condition 2: IF messageType == "ADT^A03" → route: "discharge"
Condition 3: IF messageType == "ADT^A04" → route: "registration"
```

---

## Integration with Other Steps

### Before If-Then-Else:
- **Field Validation:** Ensure data exists before checking
- **Script Enrichment:** Calculate derived fields (age, BMI)
- **API Enrichment:** Fetch external data for comparison

### After If-Then-Else:
- **Field Mapping:** Use metadata set by conditions
- **Database Enrichment:** Route determines database query
- **FHIR Validation:** Apply different rules based on routing

---

## Color Theme Reference

Our If-Then-Else UI uses the standard pipeline builder theme:

- **Primary:** Navy Blue (#1e3a8a) - Headers, buttons
- **Accent:** Pastel Pink (#f8bbd9) - Borders, highlights
- **Background:** White (#ffffff) - Main background
- **Success:** Green (#10b981) - THEN actions
- **Error:** Red (#ef4444) - ELSE actions, rejects

---

## Support & Documentation

- **Executor Code:** `services/executors/control/conditional_executor.go`
- **UI Component:** `public/js/pipeline/components/IfThenElseBuilder.js`
- **Examples:** See above sections

**Questions?** Check the help button (?) in the UI for quick examples!

---

**Status:** ✅ Complete and Ready for Use
**Version:** 1.0
**Last Updated:** December 28, 2025
