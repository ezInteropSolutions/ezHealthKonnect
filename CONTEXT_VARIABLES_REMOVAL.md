# Context Variables Removal Summary

## Date: December 26, 2025

## Changes Made

### ✅ Deprecated Context Variables in Script Enrichment

Context variables have been **deprecated** (not removed) because they are redundant with the step chaining pattern.

---

## Files Modified

### 1. **models/enrichment_models.go**
**Lines 329-331:** Added deprecation notice
```go
// DEPRECATED: Context variables - use Metadata Enrichment or Database Enrichment steps instead
// Kept for backward compatibility only
Context map[string]interface{} `json:"context,omitempty"`
```

### 2. **services/executors/enrichment/script_enrichment_executor.go**

**Lines 159-168:** Added deprecation warning when context is used
```go
// Set context variables if configured (DEPRECATED)
if config.Context != nil && len(config.Context) > 0 {
    log.Printf("   ⚠️  [DEPRECATED] Context variables used in script. Use Metadata/Database Enrichment steps instead.")
    for key, value := range config.Context {
        // ... still works for backward compatibility
    }
}
```

**Lines 297-324:** Removed context from schema (UI won't show it)
```go
"properties": map[string]interface{}{
    "script": ...,
    // REMOVED: "context" field
    "targetPath": ...,
    "timeoutMs": ...,
    "failOnError": ...,
}
```

**Lines 330-367:** Updated example to show step chaining pattern
```javascript
// Get configuration from previous enrichment step (if needed)
var config = getNestedValue(input, "enriched.metadata.config");

// Get patient data from database enrichment
var patientData = getNestedValue(input, "enriched.database.patient");
```

### 3. **public/js/pipeline/managers/ToolboxManager.js**
**Version:** 8.7 → 8.8

**Lines 156-161:** Removed `context: {}` from default config
```javascript
defaultConfig: {
    script: `...`,
    // REMOVED: context: {},
    targetPath: 'enriched.script',
    timeoutMs: 5000,
    failOnError: false
}
```

### 4. **public/pipeline-builder.html**
**Line 303:** Updated script version
```html
<script src="/js/pipeline/managers/ToolboxManager.js?v=8.8"></script>
```

---

## What Changed

### Before (Context Variables)
```javascript
// Step Config:
{
  "script": "if (accountBalance > vipThreshold) ...",
  "context": {
    "vipThreshold": 100000,  // ❌ Static, hardcoded
    "hospitalId": "HOSP_001"
  }
}

// JavaScript can use:
console.log(vipThreshold);  // From context
console.log(hospitalId);    // From context
```

**Problems:**
- Static values baked into pipeline config
- Can't see where values come from
- Hard to debug (not in step output)
- Can't vary by interface without duplicating pipeline

---

### After (Step Chaining)
```javascript
// Step 1: Database Enrichment
{
  "query": "SELECT vip_threshold, hospital_id FROM config",
  "targetPath": "enriched.database.config"
}

// Step 2: Script Enrichment
{
  "script": `
    var config = getNestedValue(input, "enriched.database.config");
    var vipThreshold = config.vip_threshold;  // ✅ From previous step
    if (accountBalance > vipThreshold) ...
  `
}
```

**Benefits:**
- ✅ Dynamic values from database/API
- ✅ Traceable (visible in step output)
- ✅ Reusable (same config query for multiple scripts)
- ✅ Debuggable (can inspect each step)
- ✅ Multi-tenant (different values per interface)

---

## Backward Compatibility

**Existing pipelines with context variables will still work:**
- Context field is kept in data model
- Backend still injects context variables into JavaScript VM
- Deprecation warning appears in logs when context is used

**New pipelines:**
- UI no longer shows context field
- Default config has no context
- Example code shows step chaining pattern

---

## How to Get Configuration Now

### Option 1: Metadata Enrichment (for constants)
```
Step 1: Add Metadata
  Config:
    PI: 3.14159
    COMPANY_NAME: "ezHealthKonnect"
  Target Path: enriched.metadata.config

Step 2: Script Enrichment
  Script:
    var config = getNestedValue(input, "enriched.metadata.config");
    var pi = config.PI;
```

### Option 2: Database Enrichment (for dynamic config)
```
Step 1: Database Enrichment
  Query: SELECT * FROM hospital_config WHERE id = 'HOSP_001'
  Target Path: enriched.database.hospital_config

Step 2: Script Enrichment
  Script:
    var config = getNestedValue(input, "enriched.database.hospital_config");
    var vipThreshold = config.vip_threshold;
```

### Option 3: API Enrichment (for external config)
```
Step 1: API Enrichment
  URL: https://config-api.com/settings
  Target Path: enriched.api.settings

Step 2: Script Enrichment
  Script:
    var settings = getNestedValue(input, "enriched.api.settings");
    var enabled = settings.featureEnabled;
```

---

## Migration Guide

### If you have existing pipelines using context:

**Before:**
```json
{
  "stepType": "pre.enrichment.script",
  "config": {
    "script": "if (age > seniorAge) ...",
    "context": {
      "seniorAge": 65,
      "vipThreshold": 100000
    }
  }
}
```

**After:**
```json
// Step 1: Add Metadata
{
  "stepType": "pre.enrichment.metadata",
  "stepAlias": "config",
  "config": {
    "metadata": {
      "seniorAge": 65,
      "vipThreshold": 100000
    },
    "targetPath": "enriched.metadata.config"
  }
}

// Step 2: Script (updated)
{
  "stepType": "pre.enrichment.script",
  "config": {
    "script": "var config = getNestedValue(input, 'enriched.metadata.config'); if (age > config.seniorAge) ..."
  }
}
```

---

## Why This Change?

### User Feedback
> "I am trying to understand how that would be an input, this cannot be a static data"

**User was right:** Context variables suggested static configuration, which doesn't make sense for dynamic healthcare data.

### Better Architecture
1. **Single Pattern:** Everything flows through enrichment steps
2. **Traceable:** All data visible in step output
3. **Reusable:** Same enrichment can feed multiple scripts
4. **Testable:** Each step independently verifiable
5. **Multi-tenant:** Different values per interface

### Aligned with System Design
- Metadata Enrichment → for constants/literals
- Database Enrichment → for dynamic config
- API Enrichment → for external config
- Script Enrichment → for calculations using enriched data

**No need for a separate context injection mechanism.**

---

## Testing

### Existing Pipelines (with context)
```bash
# Should still work, but log warning
docker-compose logs app | grep "DEPRECATED"
# Output: "⚠️  [DEPRECATED] Context variables used in script. Use Metadata/Database Enrichment steps instead."
```

### New Pipelines (without context)
```bash
# UI should not show context field
# Default config has no context property
# Example shows step chaining pattern
```

---

## Documentation

### New Guide Created
- **[SCRIPT_ENRICHMENT_STEP_CHAINING_GUIDE.md](SCRIPT_ENRICHMENT_STEP_CHAINING_GUIDE.md)** - Complete guide with 3 detailed examples

### Key Examples
1. **Calculate VIP Status** - Database → Script
2. **Calculate Readmission Risk** - Database + API → Script
3. **Distance Calculator** - Metadata (constants) → Script

---

## Summary

**What:** Deprecated context variables in Script Enrichment
**Why:** Redundant with step chaining pattern; user feedback confirmed confusion
**How:** Use Metadata/Database/API Enrichment steps to provide data to scripts
**Impact:** Existing pipelines still work (backward compatible); new pipelines follow cleaner pattern

**Key Principle:** Get all configuration and external data through enrichment steps, not context variables.
