# Step Template Recommendations - Updated

**Date:** December 27, 2025
**Status:** 📋 RECOMMENDATIONS
**Based On:** User feedback from consolidation review

---

## Overview

After completing the initial step template consolidation, the user provided valuable feedback on the remaining TODO templates and identified gaps in functionality. This document outlines the updated recommendations.

---

## User Feedback Summary

1. **Field Combine Operations:** "What about combine?" - Split/Combine removal didn't address combining multiple fields
2. **Date/Time Conversion:** "I was thinking to add a separate step, what do you recommend because there are so many variations this step needs to handle"
3. **Cross-Field Validation:** "It is my understanding comparisons are always used in condition and not generally"
4. **Unit Conversion:** "We can build this but maybe more generic like a mathematical operator step, we could add standard conversions to it"

---

## Updated Recommendations

### 1. Field Combine Operations

#### Current State:
- ✅ **Splitting works:** Field Mapping with component paths (`PID.5.1`, `PID.5.2`)
- ❌ **Combining doesn't work:** Field Mapping only maps source → target, no multi-source support

#### Options:

**Option A: Add Concat Transform to Field Mapping** ⭐ RECOMMENDED
- Add new transform: `concat:field1:field2:separator`
- Example: `{ lhs: 'fullName', rhs: 'firstName', transforms: 'concat:lastName: ' }`
- **Pros:** Simple, keeps field operations in one place
- **Cons:** Limited to concatenation only

**Option B: Script Enrichment for Complex Combines**
- Use JavaScript for complex logic
- Example:
  ```javascript
  function transform(input) {
    input.fullAddress = input.street + ', ' + input.city + ', ' + input.state + ' ' + input.zip;
    return input;
  }
  ```
- **Pros:** Full flexibility, already implemented
- **Cons:** Overkill for simple concatenation

**Option C: Dedicated Field Combine Step**
- New step type: `core.field-combine`
- Config: `{ target: 'fullName', sources: ['firstName', 'lastName'], separator: ' ' }`
- **Pros:** Clear purpose, easy to use
- **Cons:** Another step type, might be redundant

**RECOMMENDATION:**
- **Short-term:** Use Script Enrichment for combining (already works)
- **Medium-term:** Add `concat` transform to Field Mapping executor (enhancement)
- **Long-term:** Evaluate need for dedicated combine step based on user feedback

---

### 2. Date/Time Conversion Step

#### User Request:
"I was thinking to add a separate step, what do you recommend because there are so many variations this step needs to handle"

#### Analysis:
**✅ STRONGLY AGREE - Date/Time needs a dedicated step**

**Reasons:**
1. **Healthcare-Specific Formats:**
   - HL7 dates: `YYYYMMDD`, `YYYYMMDDHHMM`, `YYYYMMDDHHMMSS.SSSS`
   - FHIR dates: ISO 8601 (`YYYY-MM-DD`, `YYYY-MM-DDTHH:MM:SS+00:00`)
   - Custom formats: `MM/DD/YYYY`, `DD-MMM-YYYY`

2. **Complex Operations:**
   - Timezone conversions (UTC ↔ local)
   - Date arithmetic (add/subtract days, months, years)
   - Age calculation from birthdate
   - Epoch/timestamp conversions
   - Relative dates ("today", "now", "yesterday")

3. **Validation Requirements:**
   - Date range validation (min/max dates)
   - Future date prevention
   - Format validation

4. **Too Complex for Field Mapping:**
   - Substring transforms can't handle timezones
   - No date arithmetic support
   - No format parsing/validation

#### Recommendation:

**Create:** `Date/Time Transformation` step

**Step Type:** `pre.transformation.datetime` or `core.datetime`

**Configuration Example:**
```json
{
  "step_type": "core.datetime",
  "config": {
    "conversions": [
      {
        "source": "PID.7",
        "target": "patient.birthDate",
        "sourceFormat": "YYYYMMDD",
        "targetFormat": "YYYY-MM-DD",
        "validation": {
          "maxDate": "today",
          "minDate": "1900-01-01"
        }
      },
      {
        "source": "PV1.44",
        "target": "encounter.period.start",
        "sourceFormat": "YYYYMMDDHHMMSS",
        "targetFormat": "ISO8601",
        "sourceTimezone": "America/New_York",
        "targetTimezone": "UTC"
      },
      {
        "source": "patient.birthDate",
        "target": "patient.age",
        "operation": "age_from_date",
        "unit": "years"
      }
    ]
  }
}
```

**Features:**
- **Preset Formats:** HL7 (YYYYMMDD, etc.), FHIR (ISO8601), Common (MM/DD/YYYY)
- **Timezone Support:** Convert between timezones with IANA timezone names
- **Date Arithmetic:** Add/subtract days, months, years
- **Age Calculation:** Calculate age from birthdate
- **Validation:** Min/max dates, future date prevention
- **Epoch Support:** Unix timestamp ↔ date conversions

**Implementation:** `services/executors/transformation/datetime_executor.go`

**Priority:** 🔴 HIGH - Very common use case in healthcare

---

### 3. Cross-Field Validation → Conditional Logic

#### User Feedback (Round 1):
"It is my understanding comparisons are always used in condition and not generally"

#### User Challenge (Round 2):
"When ever you do comparison, there is an action right after that, so if else should take care of it, give a real world use case if I am wrong"

#### Analysis After Deep Thought:

**✅ USER IS CORRECT** - Cross-field validation IS just conditional logic with a "reject" action!

**Reality Check - All "Validations" Are Actually Conditionals:**

| Scenario | Looks Like Validation | Actually Is |
|----------|----------------------|-------------|
| Discharge > Admit | Validate dates | `if (discharge <= admit) { reject() } else { continue() }` |
| Patient ID match | Validate consistency | `if (PID.3 != PV1.5) { log_warning() } else { continue() }` |
| Age in range | Validate age | `if (age < 0 \|\| age > 150) { set_null() } else { continue() }` |

**User is Right:** Every comparison has an action:
- ✅ True → Continue / Set metadata / Route
- ❌ False → Reject / Log warning / Set flag / Route to error queue

**Conclusion:** Cross-field validation is just a **special case of conditional logic** where:
- Condition: field1 operator field2
- True action: continue
- False action: reject message

#### Updated Recommendation:

**Remove:** Cross-Field Validation as a separate step ❌

**Replace With:** Conditional Logic step (more flexible, covers all use cases) ✅

**New Configuration (Conditional Logic Step):**
```json
{
  "step_type": "core.conditional",
  "config": {
    "conditions": [
      {
        "name": "Validate Discharge Date",
        "condition": {
          "type": "comparison",
          "field1": "PV1.45",
          "operator": "greater_than",
          "field2": "PV1.44"
        },
        "onTrue": {
          "action": "continue"
        },
        "onFalse": {
          "action": "reject",
          "errorMessage": "Discharge date must be after admit date",
          "severity": "error"
        }
      },
      {
        "name": "Patient ID Consistency Check",
        "condition": {
          "type": "comparison",
          "field1": "PID.3.1",
          "operator": "not_equals",
          "field2": "PV1.5.1"
        },
        "onTrue": {
          "action": "log_warning",
          "message": "Patient ID mismatch between PID and PV1",
          "continue": true
        },
        "onFalse": {
          "action": "continue"
        }
      },
      {
        "name": "Route High Priority Patients",
        "condition": {
          "type": "comparison",
          "field1": "patient.age",
          "operator": "greater_than",
          "value": 65
        },
        "onTrue": {
          "action": "set_metadata",
          "metadata": {
            "priority": "high",
            "routing": "geriatrics"
          }
        },
        "onFalse": {
          "action": "continue"
        }
      }
    ]
  }
}
```

**Operators:**
- `equals`, `not_equals`
- `greater_than`, `greater_than_or_equal`
- `less_than`, `less_than_or_equal`
- `between`, `not_between`
- `contains`, `not_contains`
- `matches_regex`, `not_matches_regex`

**Actions:**
- `continue` - Continue to next step
- `reject` - Stop processing, return error
- `log_warning` - Log warning, optionally continue
- `log_error` - Log error, optionally continue
- `set_metadata` - Add metadata to message
- `set_field` - Set/update field value
- `route_to` - Route to specific destination/queue

**Benefits Over Cross-Field Validation:**
- ✅ More flexible actions (not just reject)
- ✅ Supports routing based on conditions
- ✅ Can set metadata/flags for downstream processing
- ✅ Logging with optional continue
- ✅ Single step type for all conditional logic

**Implementation:** `services/executors/control/conditional_executor.go`

**Priority:** 🔴 HIGH - Replaces Cross-Field Validation + enables routing logic

---

### 4. Mathematical Operations (formerly Unit Conversion)

#### User Feedback:
"We can build this but maybe more generic like a mathematical operator step, we could add standard conversions to it"

#### Analysis:

**✅ STRONGLY AGREE - Make it generic with preset conversions**

Unit conversion is too narrow. A mathematical operations step can:
1. Handle unit conversions as presets
2. Support custom formulas
3. Calculate derived values (BMI, age, etc.)
4. Perform arithmetic on fields

#### Recommendation:

**Create:** `Mathematical Operations` step

**Step Type:** `pre.transformation.math` or `core.math`

**Configuration Example:**
```json
{
  "step_type": "core.math",
  "config": {
    "operations": [
      {
        "name": "Convert weight to kg",
        "target": "patient.weight_kg",
        "operation": "convert",
        "source": "OBX.5",
        "from_unit": "lb",
        "to_unit": "kg"
      },
      {
        "name": "Calculate BMI",
        "target": "patient.bmi",
        "operation": "formula",
        "formula": "weight_kg / (height_m * height_m)",
        "variables": {
          "weight_kg": "patient.weight_kg",
          "height_m": "patient.height_m"
        },
        "round": 2
      },
      {
        "name": "Calculate age",
        "target": "patient.age",
        "operation": "date_diff",
        "field1": "today",
        "field2": "patient.birthDate",
        "unit": "years",
        "round": 0
      },
      {
        "name": "Average of readings",
        "target": "observation.average",
        "operation": "average",
        "sources": ["OBX[0].5", "OBX[1].5", "OBX[2].5"]
      }
    ]
  }
}
```

**Standard Unit Conversions (Presets):**

**Weight:**
- lb ↔ kg (factor: 0.453592)
- oz ↔ g (factor: 28.3495)
- stone ↔ kg (factor: 6.35029)

**Height/Length:**
- in ↔ cm (factor: 2.54)
- ft ↔ m (factor: 0.3048)
- mi ↔ km (factor: 1.60934)

**Temperature:**
- F ↔ C (formula: `(F - 32) * 5/9`)
- C ↔ K (formula: `C + 273.15`)
- F ↔ K (formula: `(F - 32) * 5/9 + 273.15`)

**Volume:**
- oz ↔ ml (factor: 29.5735)
- gal ↔ L (factor: 3.78541)
- cup ↔ ml (factor: 236.588)

**Custom Operations:**
- `formula` - Custom formula with variables
- `sum` - Sum of multiple fields
- `average` - Average of multiple fields
- `min` / `max` - Min/max of multiple fields
- `round` / `ceil` / `floor` - Rounding operations
- `abs` - Absolute value
- `sqrt` / `pow` - Square root / power

**Mathematical Functions:**
- Arithmetic: `+`, `-`, `*`, `/`, `%`, `^`
- Functions: `round(x, decimals)`, `ceil(x)`, `floor(x)`, `abs(x)`, `sqrt(x)`, `pow(x, y)`
- Conditionals: `if(condition, true_value, false_value)`
- Aggregations: `sum(...values)`, `avg(...values)`, `min(...values)`, `max(...values)`

**Implementation:** `services/executors/transformation/math_executor.go`

**Priority:** 🟡 MEDIUM - Nice to have, but Script Enrichment can handle this for now

---

## Summary of Updated Recommendations

### Immediate Actions (Already Done):
1. ✅ Remove 6 redundant templates
2. ✅ Update Field Validation to modern format
3. ✅ Enhance Field Mapping description

### Short-Term (Next 1-2 weeks):

#### 1. Date/Time Transformation Step 🔴 HIGH PRIORITY
- **Why:** Very common in healthcare, too complex for Field Mapping
- **Effort:** Medium (3-5 days)
- **Impact:** High - Every interface needs date handling
- **Action:** Create `DateTimeExecutor` with preset formats + timezone support

#### 2. Field Combine Support 🟡 MEDIUM PRIORITY
- **Why:** Users need to combine fields (fullName, fullAddress, etc.)
- **Effort:** Low (1-2 days)
- **Impact:** Medium - Common but can use Script Enrichment for now
- **Action:** Add `concat:field1:field2:separator` transform to Field Mapping

### Medium-Term (Next 1-2 months):

#### 3. Mathematical Operations Step 🟡 MEDIUM PRIORITY
- **Why:** Unit conversions, BMI calculation, age calculation
- **Effort:** Medium (3-5 days)
- **Impact:** Medium - Nice to have, Script Enrichment works for now
- **Action:** Create `MathExecutor` with preset conversions + formula support

#### 4. Cross-Field Validation Step 🟢 LOW PRIORITY
- **Why:** Data quality gates for multi-field rules
- **Effort:** Low (2-3 days)
- **Impact:** Low - Most validation is single-field
- **Action:** Create `CrossFieldValidator` with comparison operators

---

## Template Priority Order

**Phase 1 (Completed):** ✅ UI Consolidation
- Remove redundant templates
- Update Field Validation
- Enhance Field Mapping description

**Phase 2 (Next):** 🔴 Date/Time Transformation
- Most requested feature
- Healthcare-critical
- Too complex for workarounds

**Phase 3 (Soon):** 🟡 Field Combine + Mathematical Operations
- Both enhance existing capabilities
- Nice-to-have features
- Can be deferred if needed

**Phase 4 (Later):** 🟢 Cross-Field Validation
- Specific use case
- Low priority
- Can use Script Enrichment for complex rules

---

## Implementation Checklist

### Date/Time Transformation (Priority 1):
- [ ] Create `services/executors/transformation/datetime_executor.go`
- [ ] Implement preset format support (HL7, FHIR, ISO8601, custom)
- [ ] Add timezone conversion (IANA timezone database)
- [ ] Add date arithmetic (add/subtract days, months, years)
- [ ] Add age calculation
- [ ] Add validation (min/max dates, future prevention)
- [ ] Create UI template in `ToolboxManager.js`
- [ ] Write tests
- [ ] Update documentation

### Field Combine (Priority 2):
- [ ] Add `concat` transform to `field_mapping_executor.go`
- [ ] Support variable number of fields: `concat:field1:field2:field3:separator`
- [ ] Update Field Mapping UI template with example
- [ ] Write tests
- [ ] Update documentation

### Mathematical Operations (Priority 3):
- [ ] Create `services/executors/transformation/math_executor.go`
- [ ] Implement preset unit conversions (weight, height, temperature, volume)
- [ ] Add formula parser (support variables, operators, functions)
- [ ] Add aggregation functions (sum, avg, min, max)
- [ ] Add rounding/precision control
- [ ] Create UI template in `ToolboxManager.js`
- [ ] Write tests
- [ ] Update documentation

### Cross-Field Validation (Priority 4):
- [ ] Create `services/executors/validation/cross_field_validator.go`
- [ ] Implement comparison operators (equals, greater_than, less_than, between, etc.)
- [ ] Add severity levels (error, warning, info)
- [ ] Support multiple data types (string, number, date)
- [ ] Create UI template in `ToolboxManager.js`
- [ ] Write tests
- [ ] Update documentation

---

## Conclusion

Based on user feedback, the updated recommendations are:

1. **✅ Keep:** Cross-Field Validation (separate from conditionals)
2. **✅ Upgrade:** Unit Conversion → Mathematical Operations (more generic)
3. **✅ Add:** Date/Time Transformation (dedicated step for complex date handling)
4. **✅ Enhance:** Field Mapping with concat transform (simple field combining)

**Next Action:** Implement Date/Time Transformation step (highest priority, most common use case)

---

**Author:** Claude Code
**Status:** 📋 Recommendations Ready
**Next Step:** User approval to proceed with Date/Time Transformation implementation
