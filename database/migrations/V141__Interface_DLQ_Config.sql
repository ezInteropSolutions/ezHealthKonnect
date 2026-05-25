-- V141: Per-interface DLQ (Recovery Queue) configuration
--
-- dlq_config drives:
--   max_attempts        — how many poller retries before status → abandoned
--   initial_delay_s     — delay before the FIRST auto-retry (seconds)
--   retry_delay_s       — base delay between subsequent retries (seconds)
--   backoff_multiplier  — multiplied by retry_delay_s each attempt (1.0 = flat)
--   expires_after_hours — rows older than this are not polled (0 = never)
--
-- When dlq_config is NULL the DLQService falls back to compiled-in defaults
-- (max_attempts=10, initial=30s, retry=60s, backoff=2.0, expires=0).
--
-- The engineer role requires no schema change — users.role is VARCHAR(50).

ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS dlq_config JSONB;

-- Apply a sensible default to existing rows so the UI always has something to show.
UPDATE interfaces
SET dlq_config = '{
    "max_attempts": 10,
    "initial_delay_s": 30,
    "retry_delay_s": 60,
    "backoff_multiplier": 2.0,
    "expires_after_hours": 0
}'::jsonb
WHERE dlq_config IS NULL;

-- Audit trail: track who performed DLQ actions and on which row.
-- Uses existing audit_logs table — adds a dlq_row_id column so DLQ events
-- can be correlated back to the delivery_dlq record.
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS dlq_row_id UUID;

CREATE INDEX IF NOT EXISTS idx_audit_dlq_row
    ON audit_logs (dlq_row_id)
    WHERE dlq_row_id IS NOT NULL;
