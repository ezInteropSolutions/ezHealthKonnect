-- V140: DLQ Redrive Enhancement
-- Adds redrive support columns to delivery_dlq: pipeline context snapshots,
-- failed step tracking, expiry, and partial unique index to prevent duplicate pending rows.

ALTER TABLE delivery_dlq
    ADD COLUMN IF NOT EXISTS pipeline_id            UUID,
    ADD COLUMN IF NOT EXISTS failed_step_id         VARCHAR(255),
    ADD COLUMN IF NOT EXISTS step_name              VARCHAR(255),
    ADD COLUMN IF NOT EXISTS pipeline_input_snapshot JSONB,
    ADD COLUMN IF NOT EXISTS pipeline_data_snapshot  JSONB,
    ADD COLUMN IF NOT EXISTS redrive_mode           VARCHAR(20) DEFAULT 'from_failed_step',
    ADD COLUMN IF NOT EXISTS expires_at             TIMESTAMPTZ;

-- Prevent two active rows for the same message+interface combination.
-- Resolved and abandoned rows are not affected (status not in the index predicate).
-- Note: Flyway runs migrations in a transaction; CONCURRENTLY is not allowed inside
-- a transaction, so plain CREATE UNIQUE INDEX is used here.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dlq_unique_active
    ON delivery_dlq (message_id, interface_id)
    WHERE status IN ('pending', 'retrying');

CREATE INDEX IF NOT EXISTS idx_dlq_expires_at
    ON delivery_dlq (expires_at)
    WHERE expires_at IS NOT NULL AND status IN ('pending', 'retrying');

CREATE INDEX IF NOT EXISTS idx_dlq_pipeline_id
    ON delivery_dlq (pipeline_id)
    WHERE pipeline_id IS NOT NULL;
