# Flyway Migration V21 - Output Table Schema Fix

## Overview
Migration V21 was created to fix schema inconsistencies in existing output tables that were created before the standardized schema was finalized.

## Problem Statement
- **Issue**: Existing output tables (created dynamically per interface) had incomplete schemas
- **Symptoms**:
  - Missing columns like `input_message_id`, `correlation_id`, `message_type`, `transformed_message`
  - Application errors when storing transformation results
  - PostgreSQL errors: "column does not exist"
- **Root Cause**: Tables were created before schema standardization was complete (V16 was later updated but existing tables weren't migrated)

## Solution
Created migration **V21__Fix_Output_Table_Schema.sql** to:
1. Add missing columns to all existing output tables
2. Add missing indexes for performance
3. Update schema version tracking
4. Ensure all future tables use standardized schema v1.1

## Migration Details

### File Location
```
database/migrations/V21__Fix_Output_Table_Schema.sql
```

### Key Changes

#### 1. New Function: `upgrade_output_table_schema()`
Adds missing columns and indexes to existing output tables:

**Columns Added:**
- `input_message_id` - Links to input message
- `correlation_id` - Message correlation tracking
- `message_type` - HL7 message type (ADT^A01, etc.)
- `transformation_pipeline_id` - Pipeline used for transformation
- `source_message_size`, `source_message_type`, `source_encoding` - Source metadata
- `transformed_message` - JSONB with FHIR output
- `transformation_metadata` - JSONB with transformation details
- `target_format`, `target_encoding` - Output format info
- `output_message_size` - Size tracking
- `fhir_resource_type`, `fhir_resource_id` - FHIR resource identification
- `transformation_started_at`, `transformation_completed_at`, `transformation_time_ms` - Timing
- `error_count`, `last_error_message` - Error tracking
- `validation_status`, `validation_errors` - FHIR validation
- `delivery_endpoint`, `last_delivery_attempt`, `delivery_response` - Delivery tracking
- `created_by` - Audit trail

**Indexes Added:**
- B-tree indexes on: `input_message_id`, `correlation_id`, `message_type`, `fhir_resource_type`
- GIN indexes on JSONB columns: `transformed_message`, `transformation_metadata`, `validation_errors`
- Unique constraint: `(interface_id, input_message_id)`

#### 2. Updated Functions

**`create_interface_output_table()`** - Updated to create schema v1.1
- All new tables automatically get complete standardized schema
- No manual column additions needed

**`get_interface_output_table()`** - Updated to register v1.1
- Sets `schema_version = '1.1'` in metadata
- Ensures tracking of schema versions

#### 3. Migration Execution
```sql
DO $$
DECLARE
    table_record RECORD;
BEGIN
    -- Upgrade all existing output tables
    FOR table_record IN
        SELECT output_table_name
        FROM output_table_metadata
        WHERE schema_version = '1.0' OR schema_version IS NULL
    LOOP
        PERFORM upgrade_output_table_schema(table_record.output_table_name);
    END LOOP;

    -- Handle orphaned tables not in metadata
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'output_intf_%'
        AND tablename NOT IN (SELECT output_table_name FROM output_table_metadata)
    LOOP
        PERFORM upgrade_output_table_schema(table_record.tablename);
    END LOOP;
END
$$;
```

## Execution Results

### Migration Status
```
✅ Successfully applied 1 migration to schema "public", now at version v21
```

### Tables Upgraded
```sql
-- Check upgraded tables
SELECT interface_id, output_table_name, schema_version, updated_at
FROM output_table_metadata;

-- Result:
interface_id: 629ac1e8-0c50-447a-b93f-ebfc15830a7d
output_table_name: output_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
schema_version: 1.1
updated_at: 2025-10-18 10:11:00
```

### Warnings (Expected & Safe)
The migration generated warnings for columns/indexes that already existed:
- `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` - PostgreSQL warns but safely skips
- `CREATE INDEX IF NOT EXISTS` - Warnings about existing indexes (safe to ignore)
- Identifier truncation warnings - PostgreSQL 63-character limit (handled automatically)

## Benefits

### Before V21
```
❌ Missing columns → Application errors
❌ Incomplete schema → Cannot store transformation results
❌ Manual ALTER TABLE commands needed for each interface
❌ Schema version 1.0 (incomplete)
```

### After V21
```
✅ Complete standardized schema on all tables
✅ Transformation results stored successfully
✅ Automatic schema for new interfaces
✅ Schema version 1.1 (complete)
✅ Full hybrid storage (PostgreSQL + MongoDB)
```

## Verification

### 1. Check Schema Version
```sql
SELECT schema_version FROM output_table_metadata;
-- Should return: 1.1
```

### 2. Verify Columns Exist
```sql
\d output_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
-- Should show all 30+ columns
```

### 3. Test Transformation Storage
```bash
# Send HL7 message
printf '\x0BMSH|...|ADT^A01|...' | nc localhost 6661

# Check output stored
SELECT COUNT(*) FROM output_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d;
-- Should show records
```

## Impact on System

### Data Integrity
- ✅ **No data loss** - Existing data preserved
- ✅ **Backward compatible** - Old columns remain unchanged
- ✅ **Additive only** - Only adds missing columns, doesn't modify existing

### Performance
- ✅ **Improved querying** - GIN indexes on JSONB columns
- ✅ **Better filtering** - B-tree indexes on key columns
- ✅ **Optimized joins** - Indexes on foreign key relationships

### Maintenance
- ✅ **Self-healing** - Migration detects and upgrades any table
- ✅ **Future-proof** - New tables automatically get v1.1 schema
- ✅ **Tracked** - Schema versions in metadata table

## Related Migrations

### Migration Timeline
- **V16** - Initial output table infrastructure
- **V17** - Hybrid storage cross-reference
- **V18** - Hybrid storage column fixes
- **V19** - Parsing columns for JSON conversion
- **V20** - Transformation pipeline infrastructure
- **V21** - **Output table schema standardization (this migration)**

## Rollback (If Needed)

While not recommended (data would be lost), rollback would involve:
```sql
-- 1. Drop added columns (CAUTION: Data loss!)
ALTER TABLE output_intf_* DROP COLUMN IF EXISTS input_message_id CASCADE;
-- ... (repeat for all added columns)

-- 2. Revert schema version
UPDATE output_table_metadata SET schema_version = '1.0';

-- 3. Delete Flyway history
DELETE FROM flyway_schema_history WHERE version = '21';
```

**Note**: Rollback is NOT recommended. The migration is designed to be safe and additive.

## Maintenance Commands

### Check All Output Tables
```sql
SELECT
    otm.interface_id,
    i.name as interface_name,
    otm.output_table_name,
    otm.schema_version,
    otm.updated_at
FROM output_table_metadata otm
JOIN interfaces i ON otm.interface_id = i.id
ORDER BY otm.updated_at DESC;
```

### Manually Upgrade a Specific Table
```sql
SELECT upgrade_output_table_schema('output_intf_<uuid>');
```

### Verify Complete Schema
```sql
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_name = 'output_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d'
ORDER BY ordinal_position;
```

## Conclusion

Migration V21 successfully standardizes all output table schemas to version 1.1, ensuring:
- Complete hybrid storage capability (PostgreSQL + MongoDB)
- Full transformation result tracking
- Consistent schema across all interfaces
- Future-proof table creation

All output tables now support the complete transformation pipeline with proper metadata, error tracking, delivery status, and FHIR resource identification.

---

**Migration Status**: ✅ **COMPLETE** (Applied: 2025-10-18 10:11:00 UTC)
**Schema Version**: 1.1
**Tables Upgraded**: All existing output tables
**Data Impact**: None (additive only)
**Performance Impact**: Improved (additional indexes)
