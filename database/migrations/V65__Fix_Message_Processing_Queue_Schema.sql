-- V65__Fix_Message_Processing_Queue_Schema.sql
-- Ensures message_processing_queue has all expected columns.
-- V12 defined message_data but the column may be absent if the table
-- was created via a different path or if V12 had a partial failure.
-- All ALTER statements use IF NOT EXISTS so this is idempotent.

BEGIN;

ALTER TABLE message_processing_queue
    ADD COLUMN IF NOT EXISTS message_data     JSONB DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS message_metadata JSONB DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS message_type     VARCHAR(50),
    ADD COLUMN IF NOT EXISTS queue_name       VARCHAR(100) DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS priority         INTEGER DEFAULT 5,
    ADD COLUMN IF NOT EXISTS scheduled_for    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS started_processing_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS completed_at     TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS attempts         INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_attempts     INTEGER DEFAULT 3,
    ADD COLUMN IF NOT EXISTS retry_delay_ms   INTEGER DEFAULT 1000,
    ADD COLUMN IF NOT EXISTS error_message    TEXT,
    ADD COLUMN IF NOT EXISTS error_details    JSONB;

-- Re-create indexes in case they were also missing
CREATE INDEX IF NOT EXISTS idx_message_queue_status_priority
    ON message_processing_queue(status, priority DESC, scheduled_for ASC);
CREATE INDEX IF NOT EXISTS idx_message_queue_interface
    ON message_processing_queue(interface_id, status);
CREATE INDEX IF NOT EXISTS idx_message_queue_scheduled
    ON message_processing_queue(scheduled_for) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_message_queue_message_id
    ON message_processing_queue(message_id);

COMMIT;
