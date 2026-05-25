-- V18: Fix Hybrid Storage Column Addition
-- Purpose: Fix the ambiguous table_name reference and properly add MongoDB columns
-- Created: 2024-10-01

-- ============================================================================
-- PART 1: Drop and Recreate the Function with Fixed Table Name Reference
-- ============================================================================

DROP FUNCTION IF EXISTS add_hybrid_storage_columns(TEXT);

CREATE OR REPLACE FUNCTION add_hybrid_storage_columns(p_table_name TEXT)
RETURNS VOID AS $$
DECLARE
    column_exists BOOLEAN;
BEGIN
    -- Check if mongo_document_id column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE information_schema.columns.table_name = p_table_name
        AND information_schema.columns.column_name = 'mongo_document_id'
    ) INTO column_exists;

    -- Add mongo_document_id if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_document_id VARCHAR(255)', p_table_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_mongo_doc_id ON %I(mongo_document_id)',
            substring(p_table_name, 1, 50), p_table_name);
        RAISE NOTICE 'Added mongo_document_id column to %', p_table_name;
    ELSE
        RAISE NOTICE 'Column mongo_document_id already exists in %', p_table_name;
    END IF;

    -- Check if mongo_collection column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE information_schema.columns.table_name = p_table_name
        AND information_schema.columns.column_name = 'mongo_collection'
    ) INTO column_exists;

    -- Add mongo_collection if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_collection VARCHAR(255)', p_table_name);
        RAISE NOTICE 'Added mongo_collection column to %', p_table_name;
    ELSE
        RAISE NOTICE 'Column mongo_collection already exists in %', p_table_name;
    END IF;

    -- Check if mongo_synced column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE information_schema.columns.table_name = p_table_name
        AND information_schema.columns.column_name = 'mongo_synced'
    ) INTO column_exists;

    -- Add mongo_synced if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_synced BOOLEAN DEFAULT false', p_table_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_mongo_synced ON %I(mongo_synced) WHERE mongo_synced = false',
            substring(p_table_name, 1, 50), p_table_name);
        RAISE NOTICE 'Added mongo_synced column to %', p_table_name;
    ELSE
        RAISE NOTICE 'Column mongo_synced already exists in %', p_table_name;
    END IF;

    -- Check if mongo_synced_at column exists
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE information_schema.columns.table_name = p_table_name
        AND information_schema.columns.column_name = 'mongo_synced_at'
    ) INTO column_exists;

    -- Add mongo_synced_at if it doesn't exist
    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN mongo_synced_at TIMESTAMP WITH TIME ZONE', p_table_name);
        RAISE NOTICE 'Added mongo_synced_at column to %', p_table_name;
    ELSE
        RAISE NOTICE 'Column mongo_synced_at already exists in %', p_table_name;
    END IF;

END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION add_hybrid_storage_columns(TEXT) IS 'Adds MongoDB cross-reference columns to a message table (fixed version)';

-- ============================================================================
-- PART 2: Apply Hybrid Storage Columns to All Existing Message Tables
-- ============================================================================

DO $$
DECLARE
    table_record RECORD;
    full_table_name TEXT;
BEGIN
    RAISE NOTICE 'Starting to add hybrid storage columns to existing tables...';

    FOR table_record IN
        SELECT t.table_name
        FROM information_schema.tables t
        WHERE t.table_schema = 'public'
        AND (t.table_name LIKE 'messages_intf_%' OR t.table_name LIKE 'output_intf_%')
        ORDER BY t.table_name
    LOOP
        full_table_name := table_record.table_name;

        BEGIN
            RAISE NOTICE '========================================';
            RAISE NOTICE 'Processing table: %', full_table_name;
            PERFORM add_hybrid_storage_columns(full_table_name);
            RAISE NOTICE '✅ Successfully applied hybrid storage columns to: %', full_table_name;
        EXCEPTION WHEN OTHERS THEN
            RAISE WARNING '❌ Failed to add columns to %: %', full_table_name, SQLERRM;
        END;
    END LOOP;

    RAISE NOTICE '========================================';
    RAISE NOTICE '✅ Completed adding hybrid storage columns to all tables';
END $$;

-- ============================================================================
-- PART 3: Verify Column Addition
-- ============================================================================

-- Show summary of columns added
DO $$
DECLARE
    total_tables INTEGER;
    tables_with_mongo_cols INTEGER;
BEGIN
    -- Count total message tables
    SELECT COUNT(*)
    INTO total_tables
    FROM information_schema.tables
    WHERE table_schema = 'public'
    AND (table_name LIKE 'messages_intf_%' OR table_name LIKE 'output_intf_%');

    -- Count tables with mongo columns
    SELECT COUNT(DISTINCT table_name)
    INTO tables_with_mongo_cols
    FROM information_schema.columns
    WHERE table_schema = 'public'
    AND column_name IN ('mongo_document_id', 'mongo_collection', 'mongo_synced', 'mongo_synced_at')
    AND (table_name LIKE 'messages_intf_%' OR table_name LIKE 'output_intf_%');

    RAISE NOTICE '========================================';
    RAISE NOTICE '📊 Migration V18 Summary:';
    RAISE NOTICE '   Total message/output tables: %', total_tables;
    RAISE NOTICE '   Tables with MongoDB columns: %', tables_with_mongo_cols;

    IF tables_with_mongo_cols = total_tables THEN
        RAISE NOTICE '   Status: ✅ ALL TABLES UPDATED SUCCESSFULLY';
    ELSE
        RAISE WARNING '   Status: ⚠️ % tables still need updating', (total_tables - tables_with_mongo_cols);
    END IF;
    RAISE NOTICE '========================================';
END $$;

-- ============================================================================
-- PART 4: Create a View to Show Hybrid Storage Status
-- ============================================================================

CREATE OR REPLACE VIEW v_message_tables_hybrid_status AS
SELECT
    t.table_name,
    CASE WHEN c_doc_id.column_name IS NOT NULL THEN '✅' ELSE '❌' END as has_mongo_document_id,
    CASE WHEN c_collection.column_name IS NOT NULL THEN '✅' ELSE '❌' END as has_mongo_collection,
    CASE WHEN c_synced.column_name IS NOT NULL THEN '✅' ELSE '❌' END as has_mongo_synced,
    CASE WHEN c_synced_at.column_name IS NOT NULL THEN '✅' ELSE '❌' END as has_mongo_synced_at,
    CASE
        WHEN c_doc_id.column_name IS NOT NULL
         AND c_collection.column_name IS NOT NULL
         AND c_synced.column_name IS NOT NULL
         AND c_synced_at.column_name IS NOT NULL
        THEN '✅ READY'
        ELSE '❌ INCOMPLETE'
    END as hybrid_storage_status
FROM information_schema.tables t
LEFT JOIN information_schema.columns c_doc_id
    ON t.table_name = c_doc_id.table_name
    AND c_doc_id.column_name = 'mongo_document_id'
LEFT JOIN information_schema.columns c_collection
    ON t.table_name = c_collection.table_name
    AND c_collection.column_name = 'mongo_collection'
LEFT JOIN information_schema.columns c_synced
    ON t.table_name = c_synced.table_name
    AND c_synced.column_name = 'mongo_synced'
LEFT JOIN information_schema.columns c_synced_at
    ON t.table_name = c_synced_at.table_name
    AND c_synced_at.column_name = 'mongo_synced_at'
WHERE t.table_schema = 'public'
AND (t.table_name LIKE 'messages_intf_%' OR t.table_name LIKE 'output_intf_%')
ORDER BY t.table_name;

COMMENT ON VIEW v_message_tables_hybrid_status IS 'Shows which message tables have hybrid storage columns';

-- Show the status
DO $$
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '🔍 To verify hybrid storage status, run:';
    RAISE NOTICE '   SELECT * FROM v_message_tables_hybrid_status;';
    RAISE NOTICE '';
END $$;
