-- V138: Dead-letter queue for failed outbound deliveries.
-- Messages that exhaust all retry attempts land here so ops can
-- inspect, re-route, or abandon them without losing the payload.

CREATE TABLE delivery_dlq (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id       VARCHAR     NOT NULL,
    interface_id     UUID        NOT NULL,
    connector_type   VARCHAR(50),
    payload          TEXT,                   -- exact bytes that failed delivery
    content_type     VARCHAR(100),
    error_message    TEXT,
    attempt_count    INTEGER     NOT NULL DEFAULT 1,
    last_attempt_at  TIMESTAMPTZ,
    next_retry_at    TIMESTAMPTZ,
    status           VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- pending   : waiting for next retry
    -- retrying  : retry in flight
    -- resolved  : delivered successfully on retry
    -- abandoned : operator marked as no longer needed
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Retry poller: find pending items due for retry
CREATE INDEX idx_dlq_pending_retry
    ON delivery_dlq (next_retry_at)
    WHERE status = 'pending';

-- Ops dashboard: unresolved items newest-first
CREATE INDEX idx_dlq_unresolved
    ON delivery_dlq (created_at DESC)
    WHERE status IN ('pending', 'retrying');

-- Per-interface view
CREATE INDEX idx_dlq_interface
    ON delivery_dlq (interface_id, created_at DESC);
