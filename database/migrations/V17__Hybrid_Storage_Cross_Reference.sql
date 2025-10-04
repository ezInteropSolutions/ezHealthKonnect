-- V17: Hybrid Storage Cross-Database Reference Support
-- Purpose: Add MongoDB cross-reference columns to support hybrid storage architecture
-- Created: 2024-10-01

-- ============================================================================
-- PART 1: Add Cross-Reference Columns to Interface Table Metadata
-- ============================================================================

-- Add MongoDB reference tracking to interface_table_metadata
ALTER TABLE interface_table_metadata
ADD COLUMN IF NOT EXISTS mongo_collection_name VARCHAR(255),
ADD COLUMN IF NOT EXISTS hybrid_storage_enabled BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS last_sync_check TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS sync_status VARCHAR(50) DEFAULT 'pending';

COMMENT ON COLUMN interface_table_metadata.mongo_collection_name IS 'MongoDB collection name for raw messages (e.g., raw_messages_{interface_id})';
COMMENT ON COLUMN interface_table_metadata.hybrid_storage_enabled IS 'Whether this interface uses hybrid PostgreSQL + MongoDB storage';
COMMENT ON COLUMN interface_table_metadata.last_sync_check IS 'Last time cross-database sync was verified';
COMMENT ON COLUMN interface_table_metadata.sync_status IS 'Sync status: pending, synced, out_of_sync, error';

-- ============================================================================
-- PART 2: Create Function to Add Cross-Reference Columns to Message Tables
-- ============================================================================

-- This function adds MongoDB cross-reference columns to any message table
CREATE OR REPLACE FUNCTION add_hybrid_storage_columns(table_name TEXT)
RETURNS VOID AS $$
DECLARE
    column_exists BOOLEAN;
BEGIN
    -- Check if mongo_document_id column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = $1
        AND column_name = 'mongo_document_id'
    ) INTO column_exists;

    -- Add mongo_document_id if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_document_id VARCHAR(255)', table_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_mongo_doc_id ON %I(mongo_document_id)', table_name, table_name);
        RAISE NOTICE 'Added mongo_document_id column to %', table_name;
    END IF;

    -- Check if mongo_collection column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = $1
        AND column_name = 'mongo_collection'
    ) INTO column_exists;

    -- Add mongo_collection if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_collection VARCHAR(255)', table_name);
        RAISE NOTICE 'Added mongo_collection column to %', table_name;
    END IF;

    -- Check if mongo_synced column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = $1
        AND column_name = 'mongo_synced'
    ) INTO column_exists;

    -- Add mongo_synced if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_synced BOOLEAN DEFAULT false', table_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_mongo_synced ON %I(mongo_synced) WHERE mongo_synced = false', table_name, table_name);
        RAISE NOTICE 'Added mongo_synced column to %', table_name;
    END IF;

    -- Check if mongo_synced_at column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = $1
        AND column_name = 'mongo_synced_at'
    ) INTO column_exists;

    -- Add mongo_synced_at if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_synced_at TIMESTAMP WITH TIME ZONE', table_name);
        RAISE NOTICE 'Added mongo_synced_at column to %', table_name;
    END IF;

END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION add_hybrid_storage_columns(TEXT) IS 'Adds MongoDB cross-reference columns to a message table for hybrid storage';

-- ============================================================================
-- PART 3: Apply Hybrid Storage Columns to Existing Message Tables
-- ============================================================================

-- Apply to all existing message tables
DO $$
DECLARE
    table_record RECORD;
    full_table_name TEXT;
BEGIN
    FOR table_record IN
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND (table_name LIKE 'messages_intf_%' OR table_name LIKE 'output_intf_%')
    LOOP
        full_table_name := table_record.table_name;

        BEGIN
            PERFORM add_hybrid_storage_columns(full_table_name);
            RAISE NOTICE 'Applied hybrid storage columns to: %', full_table_name;
        EXCEPTION WHEN OTHERS THEN
            RAISE WARNING 'Failed to add columns to %: %', full_table_name, SQLERRM;
        END;
    END LOOP;
END $$;

-- ============================================================================
-- PART 4: Create Cross-Database Integrity Tracking Table
-- ============================================================================

CREATE TABLE IF NOT EXISTS cross_db_integrity_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL,
    message_id VARCHAR(255) NOT NULL,
    check_type VARCHAR(50) NOT NULL, -- 'verify', 'repair', 'sync'
    status VARCHAR(50) NOT NULL, -- 'success', 'failed', 'warning'
    postgresql_exists BOOLEAN,
    mongodb_exists BOOLEAN,
    is_consistent BOOLEAN,
    action_taken TEXT,
    error_message TEXT,
    checked_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_interface FOREIGN KEY (interface_id) REFERENCES interfaces(id) ON DELETE CASCADE
);

CREATE INDEX idx_integrity_log_interface ON cross_db_integrity_log(interface_id, checked_at DESC);
CREATE INDEX idx_integrity_log_message ON cross_db_integrity_log(message_id);
CREATE INDEX idx_integrity_log_status ON cross_db_integrity_log(status) WHERE status != 'success';
CREATE INDEX idx_integrity_log_consistency ON cross_db_integrity_log(is_consistent) WHERE is_consistent = false;

COMMENT ON TABLE cross_db_integrity_log IS 'Tracks cross-database referential integrity checks and repairs';
COMMENT ON COLUMN cross_db_integrity_log.check_type IS 'Type of integrity operation: verify, repair, sync';
COMMENT ON COLUMN cross_db_integrity_log.action_taken IS 'Description of repair action if any was performed';

-- ============================================================================
-- PART 5: Create Integrity Statistics Summary Table
-- ============================================================================

CREATE TABLE IF NOT EXISTS cross_db_integrity_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL UNIQUE,
    postgresql_count BIGINT DEFAULT 0,
    mongodb_count BIGINT DEFAULT 0,
    synced_count BIGINT DEFAULT 0,
    unsynced_count BIGINT DEFAULT 0,
    orphaned_pg_count BIGINT DEFAULT 0,
    orphaned_mongo_count BIGINT DEFAULT 0,
    integrity_score DECIMAL(5,2) DEFAULT 0, -- Percentage (0-100)
    last_check_at TIMESTAMP WITH TIME ZONE,
    last_repair_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_interface_stats FOREIGN KEY (interface_id) REFERENCES interfaces(id) ON DELETE CASCADE
);

CREATE INDEX idx_integrity_stats_interface ON cross_db_integrity_stats(interface_id);
CREATE INDEX idx_integrity_stats_score ON cross_db_integrity_stats(integrity_score);
CREATE INDEX idx_integrity_stats_last_check ON cross_db_integrity_stats(last_check_at DESC);

COMMENT ON TABLE cross_db_integrity_stats IS 'Summary statistics for cross-database integrity per interface';
COMMENT ON COLUMN cross_db_integrity_stats.integrity_score IS 'Integrity score as percentage (100 = perfect sync)';

-- ============================================================================
-- PART 6: Create Trigger to Update Sync Status
-- ============================================================================

-- Function to automatically update mongo_collection when message is inserted
CREATE OR REPLACE FUNCTION set_mongo_collection()
RETURNS TRIGGER AS $$
BEGIN
    -- Set the mongo_collection name based on table name pattern
    IF NEW.mongo_collection IS NULL THEN
        NEW.mongo_collection := 'raw_' || TG_TABLE_NAME;
    END IF;

    -- If mongo_document_id is set, mark as synced
    IF NEW.mongo_document_id IS NOT NULL AND NEW.mongo_synced IS NULL THEN
        NEW.mongo_synced := true;
        NEW.mongo_synced_at := CURRENT_TIMESTAMP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION set_mongo_collection() IS 'Automatically sets mongo_collection name and sync status on insert';

-- Apply trigger to existing message tables
DO $$
DECLARE
    table_record RECORD;
    trigger_name TEXT;
BEGIN
    FOR table_record IN
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_name LIKE 'messages_intf_%'
    LOOP
        trigger_name := 'trg_set_mongo_collection_' || table_record.table_name;

        -- Drop trigger if exists
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, table_record.table_name);

        -- Create trigger
        EXECUTE format('
            CREATE TRIGGER %I
            BEFORE INSERT ON %I
            FOR EACH ROW
            EXECUTE FUNCTION set_mongo_collection()
        ', trigger_name, table_record.table_name);

        RAISE NOTICE 'Created trigger % on %', trigger_name, table_record.table_name;
    END LOOP;
END $$;

-- ============================================================================
-- PART 7: Update interface_table_metadata with MongoDB Collection Names
-- ============================================================================

-- Automatically populate mongo_collection_name for existing interfaces
UPDATE interface_table_metadata
SET mongo_collection_name = CONCAT('raw_', table_name),
    hybrid_storage_enabled = true,
    sync_status = 'pending'
WHERE mongo_collection_name IS NULL;

-- ============================================================================
-- PART 8: Create Helper Views for Monitoring
-- ============================================================================

-- View to see sync status across all interfaces
CREATE OR REPLACE VIEW v_hybrid_storage_sync_status AS
SELECT
    i.id as interface_id,
    i.name as interface_name,
    itm.table_name,
    itm.mongo_collection_name,
    itm.hybrid_storage_enabled,
    itm.sync_status,
    itm.last_sync_check,
    stats.postgresql_count,
    stats.mongodb_count,
    stats.synced_count,
    stats.unsynced_count,
    stats.integrity_score,
    stats.last_check_at,
    CASE
        WHEN stats.integrity_score >= 99 THEN 'excellent'
        WHEN stats.integrity_score >= 95 THEN 'good'
        WHEN stats.integrity_score >= 90 THEN 'warning'
        ELSE 'critical'
    END as health_status
FROM interfaces i
LEFT JOIN interface_table_metadata itm ON i.id = itm.interface_id
LEFT JOIN cross_db_integrity_stats stats ON i.id = stats.interface_id
WHERE itm.hybrid_storage_enabled = true
ORDER BY stats.integrity_score ASC NULLS LAST;

COMMENT ON VIEW v_hybrid_storage_sync_status IS 'Overview of hybrid storage sync status across all interfaces';

-- View to see recent integrity issues
CREATE OR REPLACE VIEW v_recent_integrity_issues AS
SELECT
    log.interface_id,
    i.name as interface_name,
    log.message_id,
    log.check_type,
    log.status,
    log.postgresql_exists,
    log.mongodb_exists,
    log.is_consistent,
    log.action_taken,
    log.error_message,
    log.checked_at
FROM cross_db_integrity_log log
JOIN interfaces i ON log.interface_id = i.id
WHERE log.is_consistent = false OR log.status != 'success'
ORDER BY log.checked_at DESC
LIMIT 100;

COMMENT ON VIEW v_recent_integrity_issues IS 'Recent cross-database integrity issues requiring attention';

-- ============================================================================
-- PART 9: Create Stored Procedure for Integrity Check
-- ============================================================================

CREATE OR REPLACE FUNCTION check_interface_integrity(p_interface_id UUID)
RETURNS TABLE(
    check_type TEXT,
    count BIGINT,
    status TEXT
) AS $$
DECLARE
    v_table_name TEXT;
    v_total_count BIGINT;
    v_synced_count BIGINT;
    v_unsynced_count BIGINT;
BEGIN
    -- Get table name for interface
    SELECT table_name INTO v_table_name
    FROM interface_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NULL THEN
        RAISE EXCEPTION 'Interface % not found', p_interface_id;
    END IF;

    -- Count total messages
    EXECUTE format('SELECT COUNT(*) FROM %I', v_table_name) INTO v_total_count;

    -- Count synced messages
    EXECUTE format('SELECT COUNT(*) FROM %I WHERE mongo_synced = true', v_table_name) INTO v_synced_count;

    -- Count unsynced messages
    v_unsynced_count := v_total_count - v_synced_count;

    -- Return results
    RETURN QUERY VALUES
        ('total_messages', v_total_count, 'info'),
        ('synced_messages', v_synced_count, 'success'),
        ('unsynced_messages', v_unsynced_count, CASE WHEN v_unsynced_count > 0 THEN 'warning' ELSE 'success' END);
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION check_interface_integrity(UUID) IS 'Quick integrity check for a specific interface';

-- ============================================================================
-- PART 10: Create Sample Data and Validation
-- ============================================================================

-- Insert initial stats for all interfaces with message tables
INSERT INTO cross_db_integrity_stats (interface_id, last_check_at)
SELECT DISTINCT interface_id, CURRENT_TIMESTAMP
FROM interface_table_metadata
WHERE table_name IS NOT NULL
ON CONFLICT (interface_id) DO NOTHING;

-- Log the migration completion
DO $$
BEGIN
    RAISE NOTICE '✅ V17 Migration Complete: Hybrid Storage Cross-Reference Support';
    RAISE NOTICE '   - Added MongoDB cross-reference columns to all message tables';
    RAISE NOTICE '   - Created integrity tracking tables';
    RAISE NOTICE '   - Created monitoring views and stored procedures';
    RAISE NOTICE '   - Applied automatic sync status triggers';
    RAISE NOTICE '';
    RAISE NOTICE '📊 To check integrity status, run:';
    RAISE NOTICE '   SELECT * FROM v_hybrid_storage_sync_status;';
    RAISE NOTICE '';
    RAISE NOTICE '🔧 To check specific interface:';
    RAISE NOTICE '   SELECT * FROM check_interface_integrity(''<interface_id>'');';
END $$;
