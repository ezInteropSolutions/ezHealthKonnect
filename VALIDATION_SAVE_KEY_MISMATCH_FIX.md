# Validation Save Bug - CRITICAL FIX

## Issue

**User Report**: "I saved it and open again and I don't see it"

**Behavior**:
1. User adds validation rules (e.g., 3 rules)
2. Clicks "Save & Add to Pipeline" → Success message shown ✅
3. Step appears in pipeline ✅
4. User clicks step to reopen → **Rules are gone!** ❌

---

## Root Cause

**Critical Bug**: Key name mismatch between save and load operations

### Save Operation (Line 1348)
**File**: [PropertiesPanel.js:1348](public/js/pipeline/managers/PropertiesPanel.js#L1348)

```javascript
// collectFormData() - SAVES to wrong key
step.config.validationRules = JSON.parse(validationRulesInput.value);  // ❌ WRONG KEY!
```

### Load Operation (Lines 1476, 1717)
**File**: [PropertiesPanel.js:1476](public/js/pipeline/managers/PropertiesPanel.js#L1476)

```javascript
// createDynamicFormFields() - READS from different key
const rawValue = step.config?.[field.key] || field.default || '';  // field.key = 'rules'
```

**File**: [PropertiesPanel.js:1717](public/js/pipeline/managers/PropertiesPanel.js#L1717)

```javascript
// getStepConfiguration() - Defines field key
'pre.validation': {
    fields: [
        {
            key: 'rules',  // ← READS FROM HERE
            label: 'Validation Rules',
            type: 'validation-builder',
            required: true
        }
    ]
}
```

### The Mismatch

```
SAVE:  step.config.validationRules = [...]  ❌
LOAD:  step.config.rules = ???              ← Undefined!
```

**Result**: Rules saved successfully to database, but when reopening step, PropertiesPanel looks for `step.config.rules` and finds nothing!

---

## Fix Applied

**File**: [PropertiesPanel.js:1348](public/js/pipeline/managers/PropertiesPanel.js#L1348)

```javascript
// BEFORE (BROKEN):
step.config.validationRules = JSON.parse(validationRulesInput.value);
console.log('[PropertiesPanel] Collected validation rules:', step.config.validationRules);

// AFTER (FIXED):
step.config.rules = JSON.parse(validationRulesInput.value);
console.log('[PropertiesPanel] Collected validation rules:', step.config.rules);
console.log('[PropertiesPanel] ✅ Saved', step.config.rules.length, 'validation rules');
```

**Changes**:
1. ✅ Changed `step.config.validationRules` → `step.config.rules`
2. ✅ Added count logging for debugging
3. ✅ Now matches the key name used by `getStepConfiguration()`

---

## Testing Instructions

### Test: Save and Reopen Validation Rules

1. **Clear browser cache** (Ctrl + Shift + Delete)
2. Open Pipeline Builder with console (F12)
3. Add "Field Validation" step
4. Add 3 validation rules:
   - Rule 1: PID.5.1 (Family Name) - Required
   - Rule 2: PID.5.2 (Given Name) - Required
   - Rule 3: PID.7 (Date of Birth) - Required
5. Click "Save & Add to Pipeline"
6. **Check console**: Should see "✅ Saved 3 validation rules"
7. **Close the properties modal**
8. **Click the step again to reopen**
9. **Expected Result**: All 3 rules appear ✅

---

## Files Modified

| File | Old Version | New Version | Changes |
|------|-------------|-------------|---------|
| PropertiesPanel.js | v9.2 | v9.3 | Changed save key: `step.config.validationRules` → `step.config.rules` |
| pipeline-builder.html | - | - | Updated script version: PropertiesPanel.js?v=9.3 |

---

## Summary

**Critical Bug**: Validation rules saved to `step.config.validationRules` but loaded from `step.config.rules`

**Fix**: Changed save operation to use `step.config.rules` (matching load operation)

**Result**: Validation rules now persist correctly across save/reopen cycles ✅
