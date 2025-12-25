# HL7 Field Path Format Update

## Date
December 25, 2025

## Problem
Database enrichment MySQL query was returning 0 rows even with correct configuration. Backend logs showed:
```
⚠️  Parameter 1 (field enhancedSegments.PID.fields[3].value) is null
📋 Parameter 1 = <nil> (from enhancedSegments.PID.fields[3].value)
✅ [Database Enrichment] Query successful, 0 rows returned
```

## Root Cause
The FieldPathSearchComponent was using the **old legacy format** for HL7 field paths:
- Old format: `enhancedSegments.PID.fields[3].value`
- The backend has a legacy conversion function `convertLegacyPathToHL7Key()` that is **not implemented** (returns empty string)
- Comment in code says: "UI has been updated to generate simple keys directly"
- But the UI was still using the old format

## Solution
Updated all HL7 field paths in FieldPathSearchComponent to use the **new simplified format**.

### Format Comparison

| Old Format (Legacy) | New Format (Simplified) | Description |
|---------------------|-------------------------|-------------|
| `enhancedSegments.PID.fields[3].value` | `PID.3` | Patient MRN |
| `enhancedSegments.PID.fields[5].subfields[1].value` | `PID.5.2` | Patient First Name |
| `enhancedSegments.PV1.fields[19].value` | `PV1.19` | Visit Number |
| `enhancedSegments.MSH.fields[10].value` | `MSH.10` | Message Control ID |

### Benefits of New Format
1. **Shorter**: `PID.3` vs `enhancedSegments.PID.fields[3].value`
2. **Readable**: Directly maps to HL7 field notation (PID-3 → `PID.3`)
3. **Standard**: Matches HL7 specification format
4. **Performant**: Backend has optimized lookup for this format

## Files Modified

### 1. FieldPathSearchComponent.js (v2.0)
**File**: `public/js/pipeline/components/FieldPathSearchComponent.js`

**Changes**: Updated all 47 pre-defined field paths from legacy format to new format

**Example changes**:
```javascript
// OLD FORMAT
{ name: 'Patient MRN', path: 'enhancedSegments.PID.fields[3].value', description: 'Medical Record Number (PID-3)', category: 'Patient' },
{ name: 'Patient First Name', path: 'enhancedSegments.PID.fields[5].subfields[1].value', description: 'Given Name (PID-5.2)', category: 'Patient' },
{ name: 'Visit Number', path: 'enhancedSegments.PV1.fields[19].value', description: 'Visit Number (PV1-19)', category: 'Visit' },

// NEW FORMAT
{ name: 'Patient MRN', path: 'PID.3', description: 'Medical Record Number (PID-3)', category: 'Patient' },
{ name: 'Patient First Name', path: 'PID.5.2', description: 'Given Name (PID-5.2)', category: 'Patient' },
{ name: 'Visit Number', path: 'PV1.19', description: 'Visit Number (PV1-19)', category: 'Visit' },
```

### 2. QueryParamBuilder.js (v1.3)
**File**: `public/js/pipeline/components/QueryParamBuilder.js`

**Changes**: Updated placeholder text to reflect new format
```javascript
// OLD
valueInput.placeholder = 'e.g., enhancedSegments.PID.fields[3].value or search...';

// NEW
valueInput.placeholder = 'e.g., PID.3 or search...';
```

### 3. pipeline-builder.html
**Changes**: Updated version numbers for cache busting
- FieldPathSearchComponent: v1.3 → v2.0
- QueryParamBuilder: v1.2 → v1.3

## Backend Implementation

The backend ([base_executor.go](services/executors/base_executor.go)) has optimized lookup for the new format:

```go
func GetNestedValue(data map[string]interface{}, path string) interface{} {
    // Auto-lookup HL7 field keys directly from enhanced schema
    // Example: "PID.3" looks up enhancedSegments["PID"].Fields for Key="PID.3"
    // Example: "PID.5.1" looks up field PID.5 then searches its Subfields for Key="PID.5.1"
    if isHL7FieldKey(path) {
        return getHL7FieldValue(data, path)
    }

    // LEGACY SUPPORT: Convert old format to new format automatically
    // "enhancedSegments.MSH.fields[2].value" -> "MSH.2"
    // "enhancedSegments.PID.fields[3].subfields[0].value" -> "PID.3.1"
    if strings.HasPrefix(path, "enhancedSegments.") {
        convertedKey := convertLegacyPathToHL7Key(path)
        if convertedKey != "" && isHL7FieldKey(convertedKey) {
            log.Printf("🔄 [LEGACY] Auto-converting path '%s' to '%s'", path, convertedKey)
            return getHL7FieldValue(data, convertedKey)
        }
    }
    // ... fallback to manual parsing
}
```

However, `convertLegacyPathToHL7Key()` is **not implemented** (returns ""), so legacy paths fail to resolve.

## Testing Instructions

### 1. Update Existing MySQL Step
1. Hard refresh browser (Ctrl+Shift+R)
2. Open your MySQL database enrichment step
3. Click in the Query Parameters VALUE field
4. Delete the old value (`enhancedSegments.PID.fields[3].value`)
5. Type "patient mrn" in the search
6. Select "Patient MRN" from dropdown
7. **Expected**: Value is now `PID.3` (not the old format)
8. Save the step
9. Test Query with MRN: `P123456`
10. **Expected**: Query returns 1 row with patient data

### 2. Verify New Format Works
Check the backend logs:
```bash
docker-compose logs app --tail 50 | grep "Parameter"
```

**Expected output**:
```
📋 Parameter 1 = P123456 (from PID.3)
✅ [Database Enrichment] Query successful, 1 rows returned
```

**NOT** (old broken format):
```
⚠️  Parameter 1 (field enhancedSegments.PID.fields[3].value) is null
📋 Parameter 1 = <nil> (from enhancedSegments.PID.fields[3].value)
```

## Field Path Reference

### Patient Demographics (PID)
| Field | New Path | Old Path (Legacy) |
|-------|----------|-------------------|
| Patient MRN | `PID.3` | `enhancedSegments.PID.fields[3].value` |
| Patient ID | `PID.2` | `enhancedSegments.PID.fields[2].value` |
| First Name | `PID.5.2` | `enhancedSegments.PID.fields[5].subfields[1].value` |
| Last Name | `PID.5.1` | `enhancedSegments.PID.fields[5].subfields[0].value` |
| Date of Birth | `PID.7` | `enhancedSegments.PID.fields[7].value` |
| Gender | `PID.8` | `enhancedSegments.PID.fields[8].value` |

### Visit Information (PV1)
| Field | New Path | Old Path (Legacy) |
|-------|----------|-------------------|
| Visit Number | `PV1.19` | `enhancedSegments.PV1.fields[19].value` |
| Patient Class | `PV1.2` | `enhancedSegments.PV1.fields[2].value` |
| Admission Date | `PV1.44` | `enhancedSegments.PV1.fields[44].value` |
| Discharge Date | `PV1.45` | `enhancedSegments.PV1.fields[45].value` |

### Message Control (MSH)
| Field | New Path | Old Path (Legacy) |
|-------|----------|-------------------|
| Message Control ID | `MSH.10` | `enhancedSegments.MSH.fields[10].value` |
| Message Type | `MSH.9` | `enhancedSegments.MSH.fields[9].value` |
| Sending Application | `MSH.3` | `enhancedSegments.MSH.fields[3].value` |

## Migration Notes

### Existing Pipelines
Existing pipelines with the old format will **continue to work** IF the backend's `convertLegacyPathToHL7Key()` function is implemented. Currently it's not, so:

**Action Required**: Update all existing query parameters to use new format:
1. Open each database enrichment step
2. Delete and re-select field paths using the search component
3. Save the pipeline

### Custom Scripts
If you have custom JavaScript in pipeline steps that use field paths, update them:

```javascript
// OLD - Will not work
const mrn = input.enhancedSegments.PID.fields[3].value;

// NEW - Use simplified path with getHL7FieldValue
const mrn = getHL7FieldValue(input, 'PID.3');

// OR access via enhanced schema directly
const pidSegment = input.enhancedSegments.PID;
const mrnField = pidSegment.fields.find(f => f.key === 'PID.3');
const mrn = mrnField ? mrnField.value : null;
```

## Related Documentation
- [DATABASE_ENRICHMENT_CONNECTION_STRING_FIX.md](DATABASE_ENRICHMENT_CONNECTION_STRING_FIX.md)
- [MYSQL_QUERY_PARAMETER_FIX.md](MYSQL_QUERY_PARAMETER_FIX.md)
- [SESSION_SUMMARY_DEC25_2025.md](SESSION_SUMMARY_DEC25_2025.md)

## Summary

✅ **Fixed**: Updated FieldPathSearchComponent to use new simplified HL7 field path format
✅ **Benefit**: Field paths like `PID.3` now work correctly in database enrichment queries
⚠️ **Action**: Update existing pipelines to use new format (delete old value, re-select from dropdown)
📚 **Reference**: Use format `SEGMENT.FIELD` (e.g., `PID.3`) or `SEGMENT.FIELD.SUBFIELD` (e.g., `PID.5.1`)
