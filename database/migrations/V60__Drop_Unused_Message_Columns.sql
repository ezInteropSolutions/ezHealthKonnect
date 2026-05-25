-- V60__Drop_Unused_Message_Columns.sql
-- Remove columns from every messages_intf_* table that are never written
-- by the active codebase and add unnecessary noise to the schema.
--
-- Dropped columns and rationale:
--   correlation_id  — never populated (always NULL); request tracing is
--                     handled by message_id itself
--   priority        — hardcoded to 1 in INSERT, never used for routing or
--                     queue ordering; adds nothing meaningful
--   raw_message     — moved to object storage (raw_content_uri); INSERT no
--                     longer writes this column (removed in engine fix)
--   retry_count     — not driven by any active retry loop; redundant with
--                     error_count and the pipeline error-handling config
--   max_retries     — companion to retry_count; same reasoning
--
-- Preserved columns (even if currently NULL):
--   raw_content_uri, parsed_content_uri, transformed_content_uri, log_uri
--   — these populate automatically once object storage is configured

CREATE OR REPLACE FUNCTION drop_unused_message_columns(p_table TEXT)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
    col TEXT;
    cols TEXT[] := ARRAY['correlation_id','priority','raw_message','retry_count','max_retries'];
BEGIN
    FOREACH col IN ARRAY cols LOOP
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = p_table AND column_name = col
        ) THEN
            EXECUTE format('ALTER TABLE %I DROP COLUMN %I', p_table, col);
            RAISE NOTICE 'Dropped column % from %', col, p_table;
        END IF;
    END LOOP;
END;
$$;

COMMENT ON FUNCTION drop_unused_message_columns(TEXT)
    IS 'Idempotently drops never-written columns from a messages_intf_* table';

-- Apply to every registered interface table
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT table_name
        FROM   interface_table_metadata
        WHERE  table_name IS NOT NULL
    LOOP
        IF EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = r.table_name
        ) THEN
            PERFORM drop_unused_message_columns(r.table_name);
        END IF;
    END LOOP;
END;
$$;
