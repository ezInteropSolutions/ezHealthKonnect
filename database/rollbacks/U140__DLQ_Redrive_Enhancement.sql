-- ============================================================================
-- Rollback: V140__DLQ_Redrive_Enhancement
-- ============================================================================
-- Removes the redrive columns added to delivery_dlq and their indexes.
-- Run U138 first if you also need to drop the entire delivery_dlq table.
-- ============================================================================

DROP INDEX IF EXISTS idx_dlq_unique_active;
DROP INDEX IF EXISTS idx_dlq_expires_at;
DROP INDEX IF EXISTS idx_dlq_pipeline_id;

ALTER TABLE delivery_dlq
    DROP COLUMN IF EXISTS pipeline_id,
    DROP COLUMN IF EXISTS failed_step_id,
    DROP COLUMN IF EXISTS step_name,
    DROP COLUMN IF EXISTS pipeline_input_snapshot,
    DROP COLUMN IF EXISTS pipeline_data_snapshot,
    DROP COLUMN IF EXISTS redrive_mode,
    DROP COLUMN IF EXISTS expires_at;

-- Flyway history cleanup:
-- DELETE FROM flyway_schema_history WHERE version = '140';
