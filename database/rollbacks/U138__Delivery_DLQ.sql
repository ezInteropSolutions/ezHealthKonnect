-- ============================================================================
-- Rollback: V138__Delivery_DLQ
-- ============================================================================
-- Drops the dead-letter queue table.
-- All pending/retrying DLQ rows will be permanently lost.
-- Resolve or archive DLQ rows before running this rollback.
-- ============================================================================

-- Abort if any active DLQ rows exist (pending/retrying) — they would be lost.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM delivery_dlq WHERE status IN ('pending', 'retrying') LIMIT 1) THEN
        RAISE EXCEPTION 'Active DLQ rows exist (pending/retrying). Resolve them before rolling back V138.';
    END IF;
END $$;

DROP TABLE IF EXISTS delivery_dlq;

-- Flyway history cleanup:
-- DELETE FROM flyway_schema_history WHERE version = '138';
