-- ============================================================================
-- Rollback: V141__Interface_DLQ_Config
-- ============================================================================

DROP INDEX IF EXISTS idx_audit_dlq_row;

ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS dlq_row_id;

ALTER TABLE interfaces
    DROP COLUMN IF EXISTS dlq_config;

-- Flyway history cleanup:
-- DELETE FROM flyway_schema_history WHERE version = '141';
