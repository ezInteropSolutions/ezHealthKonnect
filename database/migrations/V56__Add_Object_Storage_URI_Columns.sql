-- V56__Add_Object_Storage_URI_Columns.sql
-- Adds object storage URI columns to the interface_table_metadata and system tables.
-- Per-interface tables (messages_intf_*) are handled dynamically at runtime by
-- InterfaceTableManager.  This migration adds the columns to the metadata registry
-- and creates a helper function that the app can call to migrate individual interface
-- tables lazily on first use.
--
-- Storage URI columns:
--   raw_content_uri         s3://bucket/raw/{interfaceId}/{messageId}.raw
--   parsed_content_uri      s3://bucket/parsed/{interfaceId}/{messageId}.json
--   transformed_content_uri s3://bucket/transformed/{interfaceId}/{messageId}.json
--   log_uri                 s3://bucket/logs/{interfaceId}/{messageId}.ndjson

-- ─── 1. Track object-storage migration state per interface table ──────────────
ALTER TABLE interface_table_metadata
    ADD COLUMN IF NOT EXISTS storage_migrated_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS storage_driver       VARCHAR(20) DEFAULT 'none';

COMMENT ON COLUMN interface_table_metadata.storage_migrated_at
    IS 'When this interface table had its raw_message column replaced with URI columns';
COMMENT ON COLUMN interface_table_metadata.storage_driver
    IS 'Object storage driver in use: none | s3 | local';

-- ─── 2. Helper function: add URI columns to a named messages_intf_* table ─────
CREATE OR REPLACE FUNCTION add_storage_uri_columns(p_table_name TEXT)
RETURNS VOID
LANGUAGE plpgsql AS $$
BEGIN
    -- raw content URI (replaces raw_message TEXT for new rows)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'raw_content_uri'
    ) THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN raw_content_uri VARCHAR(500)', p_table_name);
    END IF;

    -- parsed content URI (JSON conversion output)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'parsed_content_uri'
    ) THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN parsed_content_uri VARCHAR(500)', p_table_name);
    END IF;

    -- transformed content URI (post-pipeline FHIR/output)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'transformed_content_uri'
    ) THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN transformed_content_uri VARCHAR(500)', p_table_name);
    END IF;

    -- log URI (NDJSON processing log file)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'log_uri'
    ) THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN log_uri VARCHAR(500)', p_table_name);
    END IF;
END;
$$;

COMMENT ON FUNCTION add_storage_uri_columns(TEXT)
    IS 'Idempotently adds 4 object-storage URI columns to a messages_intf_* table';

-- ─── 3. Apply URI columns to all existing interface tables immediately ─────────
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT table_name
        FROM   interface_table_metadata
        WHERE  table_name IS NOT NULL
    LOOP
        -- Only process tables that actually exist
        IF EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = r.table_name
        ) THEN
            PERFORM add_storage_uri_columns(r.table_name);
            UPDATE interface_table_metadata
               SET storage_migrated_at = NOW(),
                   storage_driver      = 'none'   -- will be updated to 's3' or 'local' at runtime
             WHERE table_name = r.table_name;
        END IF;
    END LOOP;
END;
$$;

-- ─── 4. Index URIs for fast lookup by storage path ────────────────────────────
-- Note: these are created per-table at runtime by InterfaceTableManager when a
-- new interface table is provisioned.  Existing tables get indexes via the DO block
-- above only if we know the table name; for safety we skip index creation here
-- since the tables are named dynamically (messages_intf_<uuid>).
