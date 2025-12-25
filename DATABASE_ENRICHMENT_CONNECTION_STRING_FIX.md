# Database Enrichment Connection String Auto-Clear Fix

## Date
December 25, 2025

## Problem
Database enrichment steps were failing to connect even though individual connection fields (Host, Port, Database, Username, Password) were correctly configured. The issue affected both MySQL and PostgreSQL enrichment steps.

## Root Cause
When database enrichment steps were created from the toolbox, they had a hardcoded default connection string in their configuration:
```javascript
connectionString: 'postgresql://user:pass@localhost:5432/dbname'
```

This default connection string was saved with the step configuration and persisted across saves/reloads. When the step was executed, the backend executor prioritized this saved `connectionString` field over the individual connection fields (dbHost, dbPort, dbName, dbUser, dbPassword), causing connections to fail with:
- MySQL: `Error 1045: Access denied for user 'user'@'localhost'`
- PostgreSQL: Similar authentication errors

## Solution

### Part 1: Fixed Default Configuration (Already Applied)
Changed the default `connectionString` in ToolboxManager from hardcoded PostgreSQL string to empty string:

**File**: `public/js/pipeline/managers/ToolboxManager.js`
```javascript
defaultConfig: {
    databaseType: 'postgresql',
    connectionString: '', // Empty - will be built from individual fields
    query: 'SELECT * FROM patients WHERE patient_id = $1',
    // ...
}
```

### Part 2: Auto-Clear on Save (NEW FIX)
Added automatic detection and clearing of `connectionString` when saving a step that has individual connection fields configured.

**File**: `public/js/pipeline/managers/PropertiesPanel.js` (v20.5)

**Changes** (lines 2172-2179):
```javascript
// CRITICAL FIX: For database enrichment steps, if individual connection fields are provided,
// force connectionString to be empty so executor builds it from individual fields
if (step.config.databaseType &&
    (step.config.dbHost || step.config.dbName || step.config.dbUser)) {
    console.log('[PropertiesPanel] 🔧 Database enrichment with individual fields detected');
    console.log('[PropertiesPanel] 🔧 Setting connectionString to empty to force field-based connection');
    step.config.connectionString = '';
}
```

**Logic**:
1. Check if this is a database enrichment step (`step.config.databaseType` exists)
2. Check if individual connection fields are provided (at least one of: `dbHost`, `dbName`, `dbUser`)
3. If both conditions are true, force `connectionString` to empty string
4. Backend executor will then build connection string from individual fields instead of using the (invalid) saved connectionString

## How It Works

### Backend Behavior
The backend executor ([database_enrichment_executor.go](services/executors/enrichment/database_enrichment_executor.go)) already has logic to build connection strings from individual fields:

```go
// If connectionString is empty, build from individual fields
if config.ConnectionString == "" {
    switch config.DatabaseType {
    case "mysql":
        config.ConnectionString = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
            config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
    case "postgresql":
        config.ConnectionString = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
            config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName)
    // ... other database types
    }
}
```

### Frontend Behavior (After Fix)
When user saves a database enrichment step:
1. PropertiesPanel collects all `config_*` fields (including any saved `connectionString`)
2. NEW: Detects if individual connection fields are present
3. NEW: If present, overwrites `connectionString` to empty string
4. Step is saved with `connectionString: ""`
5. Backend builds correct connection string from individual fields at runtime

## User Impact

### Before Fix
- User configures individual fields correctly (Host, Port, Database, Username, Password)
- Step saves successfully
- Query test FAILS with authentication errors
- Pipeline execution FAILS with "No database enrichment data found"
- User has to manually find and clear the "Connection String" field (which is in Advanced section and easy to miss)

### After Fix
- User configures individual fields correctly (Host, Port, Database, Username, Password)
- Step saves successfully
- **System automatically clears any saved connectionString**
- Query test SUCCEEDS
- Pipeline execution SUCCEEDS
- No manual intervention required

## Testing Instructions

### Test Existing Steps (Steps Created Before Fix)
1. Open an existing database enrichment step (MySQL or PostgreSQL)
2. Verify individual connection fields are correctly filled:
   - Host: `mysql` or `postgres`
   - Port: `3306` or `5432`
   - Database: `ezhealthkonnect`
   - Username: `ezhealth_user`
   - Password: `secure_password_change_me`
3. Click "Save" (don't change anything)
4. Check browser console - should see:
   ```
   [PropertiesPanel] 🔧 Database enrichment with individual fields detected
   [PropertiesPanel] 🔧 Setting connectionString to empty to force field-based connection
   ```
5. Click "Test Query" with a test value
6. **Expected**: Query executes successfully, returns data

### Test New Steps (Steps Created After Fix)
1. Add new Database Enrichment step from toolbox
2. Configure individual connection fields
3. Configure query and query parameters
4. Click "Save"
5. Click "Test Query"
6. **Expected**: Query executes successfully

## Files Modified

1. **`public/js/pipeline/managers/PropertiesPanel.js`** (v20.5)
   - Added auto-clear logic in `collectFormData()` method
   - Lines 2172-2179

2. **`public/pipeline-builder.html`**
   - Updated PropertiesPanel version to v20.5 for cache busting

## Related Documentation

- [MYSQL_QUERY_PARAMETER_FIX.md](MYSQL_QUERY_PARAMETER_FIX.md) - Parameter ordering fix
- [SESSION_SUMMARY_DEC25_2025.md](SESSION_SUMMARY_DEC25_2025.md) - Complete session summary
- [QUERY_PARAMS_FIELD_SEARCH_INTEGRATION.md](QUERY_PARAMS_FIELD_SEARCH_INTEGRATION.md) - Field search integration

## Prevention

This issue will not occur for new steps because:
1. Default config in ToolboxManager has empty `connectionString`
2. Auto-clear logic ensures any manually-entered connectionString is cleared if individual fields are present

For existing steps (created before fix):
1. Simply re-saving the step will trigger the auto-clear logic
2. No need to manually clear the Connection String field anymore
