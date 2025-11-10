# PostgreSQL JSONB Cast Fix - November 3, 2025

## Issue
Backend returned error when saving interface with sink target:
```
invalid input syntax for type json
```

## Root Cause
PostgreSQL JSONB columns require explicit casting when using parameterized queries. Without the `::jsonb` cast, PostgreSQL treats the parameter as plain text instead of JSON.

## Solution
Added explicit JSONB casts to all JSON columns in the UPDATE query:

**Before**:
```sql
source_config = :sourceConfig,
target_config = :targetConfig,
processing_rules = :processingRules,
transformation_mapping = :transformationMapping
```

**After**:
```sql
source_config = :sourceConfig::jsonb,
target_config = :targetConfig::jsonb,
processing_rules = :processingRules::jsonb,
transformation_mapping = :transformationMapping::jsonb
```

## Files Modified
- controllers/interfacesController.js (lines 734-737)

## Testing
1. Edit any interface
2. Change target to "Sink"
3. Save → Should succeed without JSON errors

