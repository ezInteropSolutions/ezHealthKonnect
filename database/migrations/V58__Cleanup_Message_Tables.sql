-- V58__Cleanup_Message_Tables.sql
-- Clean up interface message tables after the object-storage migration (V56).
--
-- Changes applied to EVERY messages_intf_* table:
--
--   1. Clear raw_message for rows whose content is already in object storage
--      (raw_content_uri IS NOT NULL).  Keeps the column nullable but avoids
--      storing the same bytes twice.  Column will be dropped in a future
--      migration once all environments have confirmed object-storage parity.
--
--   2. Drop columns that are never written by the active codebase:
--        transformation_applied   — legacy placeholder, never populated
--        transformation_type      — legacy placeholder, never populated
--        last_error_at            — legacy, superseded by last_error_message
--        target_endpoint          — legacy, never written
--        last_delivery_attempt_at — legacy, superseded by delivery_attempts
--        mongo_document_id        — MongoDB era reference, no longer used
--
--   3. Add delivery_status value 'not_required' to the set of accepted states.
--      (No check constraint exists today, so this is informational only.)
--
-- NOTE: parsing_status, parsed_at, parsing_time_ms, parsing_error (added by
--       V19-era migrations) are actively written by the processing engine and
--       are deliberately preserved.
-- ─────────────────────────────────────────────────────────────────────────────

-- Helper function: clean up one interface table (idempotent — safe to re-run)
CREATE OR REPLACE FUNCTION cleanup_message_table(p_table TEXT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    -- 1. Null-out raw_message where we already have the URI in object storage
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'raw_message'
    ) AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'raw_content_uri'
    ) THEN
        EXECUTE format(
            'UPDATE %I SET raw_message = NULL WHERE raw_content_uri IS NOT NULL AND raw_message IS NOT NULL',
            p_table
        );
    END IF;

    -- 2a. Drop transformation_applied
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'transformation_applied'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN transformation_applied', p_table);
    END IF;

    -- 2b. Drop transformation_type
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'transformation_type'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN transformation_type', p_table);
    END IF;

    -- 2c. Drop last_error_at (superseded by last_error_message + updated_at)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'last_error_at'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN last_error_at', p_table);
    END IF;

    -- 2d. Drop target_endpoint (delivery destination is on the interface config)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'target_endpoint'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN target_endpoint', p_table);
    END IF;

    -- 2e. Drop last_delivery_attempt_at (superseded by delivery_attempts counter)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'last_delivery_attempt_at'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN last_delivery_attempt_at', p_table);
    END IF;

    -- 2f. Drop mongo_document_id (MongoDB era, object storage replaces this)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table AND column_name = 'mongo_document_id'
    ) THEN
        EXECUTE format('ALTER TABLE %I DROP COLUMN mongo_document_id', p_table);
    END IF;
END;
$$;

COMMENT ON FUNCTION cleanup_message_table(TEXT)
    IS 'Idempotently clears stale raw_message bytes and drops legacy columns from a messages_intf_* table';

-- Apply cleanup to every registered interface table
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
            PERFORM cleanup_message_table(r.table_name);
        END IF;
    END LOOP;
END;
$$;
