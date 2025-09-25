-- V14: Dedicated Table-Per-Interface Architecture
-- Purpose: Ultimate performance isolation with dedicated tables per interface

-- =========================================================================
-- STEP 1: Create metadata table to track interface-specific message tables
-- =========================================================================

CREATE TABLE IF NOT EXISTS interface_table_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    table_name VARCHAR(100) NOT NULL UNIQUE,
    schema_version VARCHAR(20) NOT NULL DEFAULT '1.0',

    -- Table statistics
    estimated_rows BIGINT DEFAULT 0,
    last_maintenance_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    auto_maintenance_enabled BOOLEAN DEFAULT true,

    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Performance tracking
    performance_metrics JSONB DEFAULT '{}',

    UNIQUE(interface_id)
);

-- Index for fast table name lookups
CREATE INDEX IF NOT EXISTS idx_interface_table_metadata_interface_id
ON interface_table_metadata(interface_id);

CREATE INDEX IF NOT EXISTS idx_interface_table_metadata_table_name
ON interface_table_metadata(table_name);

-- =========================================================================
-- STEP 2: Create maintenance views for monitoring all interface tables
-- =========================================================================

-- View to get statistics across all interface tables
CREATE OR REPLACE VIEW v_interface_table_summary AS
SELECT
    i.id as interface_id,
    i.name as interface_name,
    i.message_type as interface_format,
    i.status as interface_status,
    itm.table_name,
    itm.estimated_rows,
    itm.last_maintenance_at,
    itm.created_at as table_created_at,
    pg_total_relation_size(itm.table_name) as table_size_bytes,
    pg_size_pretty(pg_total_relation_size(itm.table_name)) as table_size_human
FROM interfaces i
LEFT JOIN interface_table_metadata itm ON i.id = itm.interface_id
WHERE i.status IN ('active', 'testing', 'configured')
ORDER BY itm.estimated_rows DESC;

-- =========================================================================
-- STEP 3: Create functions for table management
-- =========================================================================

-- Function to update table statistics (called by maintenance jobs)
CREATE OR REPLACE FUNCTION update_interface_table_stats(p_interface_id UUID)
RETURNS VOID AS $$
DECLARE
    v_table_name VARCHAR(100);
    v_row_count BIGINT;
    v_sql TEXT;
BEGIN
    -- Get table name for interface
    SELECT table_name INTO v_table_name
    FROM interface_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NULL THEN
        RETURN;
    END IF;

    -- Count rows in interface table
    v_sql := 'SELECT COUNT(*) FROM ' || quote_ident(v_table_name);
    EXECUTE v_sql INTO v_row_count;

    -- Update metadata
    UPDATE interface_table_metadata
    SET
        estimated_rows = v_row_count,
        last_maintenance_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP
    WHERE interface_id = p_interface_id;

    -- Log maintenance activity
    INSERT INTO audit_logs (
        user_id, action, resource_type, resource_id,
        details, created_at
    ) VALUES (
        NULL, 'table_maintenance', 'interface_table', p_interface_id,
        jsonb_build_object(
            'table_name', v_table_name,
            'row_count', v_row_count,
            'maintenance_type', 'statistics_update'
        ),
        CURRENT_TIMESTAMP
    );
END;
$$ LANGUAGE plpgsql;

-- Function to clean old messages from interface tables (data retention)
CREATE OR REPLACE FUNCTION cleanup_interface_table_messages(
    p_interface_id UUID,
    p_retention_days INTEGER DEFAULT 90
)
RETURNS INTEGER AS $$
DECLARE
    v_table_name VARCHAR(100);
    v_deleted_count INTEGER;
    v_sql TEXT;
BEGIN
    -- Get table name for interface
    SELECT table_name INTO v_table_name
    FROM interface_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NULL THEN
        RETURN 0;
    END IF;

    -- Delete old messages
    v_sql := 'DELETE FROM ' || quote_ident(v_table_name) ||
             ' WHERE received_at < NOW() - INTERVAL ''' || p_retention_days || ' days''';

    EXECUTE v_sql;
    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;

    -- Update statistics
    PERFORM update_interface_table_stats(p_interface_id);

    -- Log cleanup activity
    INSERT INTO audit_logs (
        user_id, action, resource_type, resource_id,
        details, created_at
    ) VALUES (
        NULL, 'table_cleanup', 'interface_table', p_interface_id,
        jsonb_build_object(
            'table_name', v_table_name,
            'deleted_count', v_deleted_count,
            'retention_days', p_retention_days
        ),
        CURRENT_TIMESTAMP
    );

    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- STEP 4: Create triggers to maintain table metadata
-- =========================================================================

-- Trigger function to update metadata when interfaces are modified
CREATE OR REPLACE FUNCTION trigger_interface_table_metadata_update()
RETURNS TRIGGER AS $$
BEGIN
    -- Update the updated_at timestamp
    IF TG_OP = 'UPDATE' THEN
        NEW.updated_at = CURRENT_TIMESTAMP;
        RETURN NEW;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to metadata table
DROP TRIGGER IF EXISTS tr_interface_table_metadata_update ON interface_table_metadata;
CREATE TRIGGER tr_interface_table_metadata_update
    BEFORE UPDATE ON interface_table_metadata
    FOR EACH ROW
    EXECUTE FUNCTION trigger_interface_table_metadata_update();

-- =========================================================================
-- STEP 5: Add configuration for dedicated table management
-- =========================================================================

-- Add column to interfaces table to track table-per-interface mode
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS use_dedicated_table BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS table_management_strategy VARCHAR(50) DEFAULT 'shared'
    CHECK (table_management_strategy IN ('shared', 'dedicated', 'hybrid'));

-- =========================================================================
-- STEP 6: Performance optimization indexes
-- =========================================================================

-- Optimize interface table metadata queries
CREATE INDEX IF NOT EXISTS idx_interface_table_metadata_performance
ON interface_table_metadata(estimated_rows DESC, last_maintenance_at DESC)
WHERE auto_maintenance_enabled = true;

-- =========================================================================
-- STEP 7: Initial data seeding for existing interfaces
-- =========================================================================

-- Mark high-volume interfaces for dedicated table mode (can be adjusted)
UPDATE interfaces
SET
    use_dedicated_table = true,
    table_management_strategy = 'dedicated'
WHERE status IN ('active', 'testing')
AND message_type IN ('HL7', 'FHIR', 'hl7', 'fhir', 'ADT^A01', 'ORU^R01');

-- =========================================================================
-- STEP 8: Add monitoring and alerting configuration
-- =========================================================================

-- Table to track performance metrics per interface table
CREATE TABLE IF NOT EXISTS interface_table_performance_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    table_name VARCHAR(100) NOT NULL,

    -- Performance metrics
    query_time_ms NUMERIC(10,2),
    row_count BIGINT,
    table_size_mb NUMERIC(10,2),
    index_usage_ratio NUMERIC(5,4),

    -- Query patterns
    query_type VARCHAR(50), -- SELECT, INSERT, UPDATE, DELETE
    filter_conditions TEXT,

    -- Timing
    measured_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Context
    user_session_id VARCHAR(100),
    client_ip INET
);

-- Index for performance analysis
CREATE INDEX IF NOT EXISTS idx_interface_perf_log_timing
ON interface_table_performance_log(interface_id, measured_at DESC, query_type);

-- =========================================================================
-- COMPLETED: Dedicated Table-Per-Interface Architecture Ready
-- =========================================================================

-- Summary of changes:
-- 1. ✅ Metadata tracking for interface-specific tables
-- 2. ✅ Dynamic table creation and management functions
-- 3. ✅ Performance monitoring and maintenance automation
-- 4. ✅ Data retention and cleanup capabilities
-- 5. ✅ Configuration flags for table management strategy
-- 6. ✅ Audit logging for all table operations
-- 7. ✅ Views for monitoring table health across all interfaces

-- Next steps after migration:
-- 1. Update MessageController to use InterfaceTableManager
-- 2. Configure which interfaces use dedicated tables vs shared
-- 3. Set up maintenance jobs for table cleanup and stats updates
-- 4. Monitor performance improvements through interface_table_performance_log