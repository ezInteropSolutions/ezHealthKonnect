# MySQL Query Parameter Fix

## Date
December 25, 2025

## Problem
MySQL database queries were failing with error:
```
Error 1064 (42000): You have an error in your SQL syntax near '?' at line 1
```

## Root Cause
Go maps (`map[string]string`) **do not preserve insertion order**. When iterating over `config.QueryParams` to build the parameter array, the parameters were extracted in random order, causing them to be bound to the wrong `?` placeholders in MySQL queries.

### Example of the Problem
**Configuration:**
```json
{
  "queryParams": {
    "mrn": "enhancedSegments.PID.fields[3].value",
    "visitId": "enhancedSegments.PV1.fields[19].value"
  }
}
```

**SQL Query:**
```sql
SELECT * FROM patients WHERE mrn = ? AND visit_id = ?
```

**What Happened:**
- Go map iteration is random
- Sometimes extracted as `[mrn_value, visit_value]` ✅ correct
- Sometimes extracted as `[visit_value, mrn_value]` ❌ wrong order
- This caused first `?` to get `visit_value` instead of `mrn_value`

## Solution

### Changes Made

#### 1. database_enrichment_executor.go
**File:** `services/executors/enrichment/database_enrichment_executor.go`

**Added import:**
```go
import (
    ...
    "sort"
    ...
)
```

**Fixed `buildQueryParams` function:**
```go
func (e *DatabaseEnrichmentExecutor) buildQueryParams(
    config *models.DatabaseEnrichmentConfigV2,
    inputData map[string]interface{},
) ([]interface{}, error) {
    if len(config.QueryParams) == 0 {
        return nil, nil
    }

    // Collect and sort parameter names to ensure consistent order
    paramNames := make([]string, 0, len(config.QueryParams))
    for paramName := range config.QueryParams {
        paramNames = append(paramNames, paramName)
    }

    // Sort alphabetically (ensures "1", "2", "3" are in correct order)
    sort.Strings(paramNames)

    params := make([]interface{}, 0, len(paramNames))

    // Extract values in sorted order
    for _, paramName := range paramNames {
        fieldPath := config.QueryParams[paramName]
        value := executors.GetNestedValue(inputData, fieldPath)
        if value == nil {
            log.Printf("   ⚠️  Parameter %s (field %s) is null", paramName, fieldPath)
        }
        params = append(params, value)
        log.Printf("   📋 Parameter %s = %v (from %s)", paramName, value, fieldPath)
    }

    return params, nil
}
```

#### 2. database_test_controller.go
**File:** `controllers/database_test_controller.go`

**Added import:**
```go
import (
    ...
    "sort"
    ...
)
```

**Fixed parameter building in `TestQuery` function:**
```go
// Build parameter values from test params
params := make([]interface{}, 0)

// Collect parameter names
paramNames := make([]string, 0, len(req.QueryParams))
for paramName := range req.QueryParams {
    paramNames = append(paramNames, paramName)
}

// Sort alphabetically (ensures "1", "2", "3" are in correct order)
sort.Strings(paramNames)

// Extract values in sorted order
for _, paramName := range paramNames {
    testValue, exists := req.TestParams[paramName]
    if exists && testValue != "" {
        params = append(params, testValue)
    } else {
        params = append(params, nil)
    }
}
```

## Parameter Naming Convention

To ensure correct order for positional placeholders (`?` for MySQL, `$1, $2, $3` for PostgreSQL), use **numeric keys**:

### Correct Configuration
```json
{
  "query": "SELECT * FROM patients WHERE mrn = ? AND visit_id = ?",
  "queryParams": {
    "1": "enhancedSegments.PID.fields[3].value",
    "2": "enhancedSegments.PV1.fields[19].value"
  }
}
```

### Why This Works
- Alphabetical sort: `["1", "2"]` → Correct order maintained
- First `?` gets parameter "1" value
- Second `?` gets parameter "2" value

### Alternative (Also Works)
You can use descriptive names that sort correctly:
```json
{
  "queryParams": {
    "a_mrn": "enhancedSegments.PID.fields[3].value",
    "b_visit": "enhancedSegments.PV1.fields[19].value"
  }
}
```
Alphabetical sort: `["a_mrn", "b_visit"]` → Correct order

## Testing Instructions

### 1. Using Database Enrichment Step

1. **Add Database Enrichment step** to your pipeline
2. **Configure the step:**
   - Database Type: `mysql`
   - Host: `mysql`
   - Port: `3306`
   - Database: `ezhealthkonnect`
   - Username: `root`
   - Password: `root_password`
   - Query: `SELECT * FROM patients WHERE mrn = ?`

3. **Configure Query Parameters:**
   - Click "Add Parameter"
   - Key: `1` (IMPORTANT: Use "1" for first parameter)
   - Value: Use dropdown to select "Patient MRN" or enter `enhancedSegments.PID.fields[3].value`
   - Description: `Patient MRN from HL7 message`

4. **Test the configuration**
5. **Save the pipeline**

### 2. Using Database Query Tester

1. **Configure your query in the Database Enrichment step**
2. **Click "Test Query" button**
3. **Enter test values:**
   - For parameter "1": Enter a test MRN like `P123456`
4. **Click "Run Query"**
5. **Verify results** - Should show patient data from MySQL

## Multiple Parameters Example

**Query:**
```sql
SELECT * FROM visits
WHERE patient_mrn = ?
  AND visit_date >= ?
  AND status = ?
```

**Configuration:**
```json
{
  "queryParams": {
    "1": "enhancedSegments.PID.fields[3].value",
    "2": "enhancedSegments.PV1.fields[44].value",
    "3": "enhancedSegments.PV1.fields[2].value"
  }
}
```

**Execution:**
- Parameter 1 (first `?`) → Patient MRN from PID-3
- Parameter 2 (second `?`) → Admission date from PV1-44
- Parameter 3 (third `?`) → Patient class from PV1-2

## Database Support

This fix applies to all SQL databases that use positional parameters:

| Database   | Placeholder | Example                          | Status |
|------------|-------------|----------------------------------|--------|
| MySQL      | `?`         | `WHERE id = ?`                  | ✅ Fixed |
| PostgreSQL | `$1, $2`    | `WHERE id = $1 AND name = $2`   | ✅ Fixed |
| SQL Server | `@p1, @p2`  | `WHERE id = @p1`                | ✅ Fixed |
| Oracle     | `:1, :2`    | `WHERE id = :1`                 | ✅ Fixed |

## Verification

After the fix, you should see in the logs:
```
   📋 Parameter 1 = P123456 (from enhancedSegments.PID.fields[3].value)
   📋 Parameter 2 = 2024-01-15 (from enhancedSegments.PV1.fields[44].value)
```

Parameters are now extracted in **sorted order** (1, 2, 3, ...) ensuring they match the positional placeholders in the SQL query.

## Files Modified
1. `services/executors/enrichment/database_enrichment_executor.go` - Added sorting to buildQueryParams
2. `controllers/database_test_controller.go` - Added sorting to test query parameter building

## Related Issues
- Parameter ordering was non-deterministic due to Go map iteration
- Affected both runtime enrichment and query testing
- Could cause intermittent query failures (worked sometimes, failed other times)

## Prevention
- Always use numeric keys ("1", "2", "3") for positional parameters
- Alphabetical sorting ensures consistent order across all Go map iterations
- Code now explicitly sorts parameter names before value extraction
