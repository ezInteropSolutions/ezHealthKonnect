-- V213: Bulk Reprocess Jobs
-- Applied: 2026-08-22
--
-- Tracks async bulk message-reprocess jobs triggered from the Messages page.
-- A job is EITHER a filter (status/message type/date range — for "select all
-- N matching", potentially millions of rows) OR an explicit list of message
-- IDs (for a small checkbox-based selection; application layer caps this list
-- size, not enforced here). The row created by the API returns instantly —
-- all actual work happens in a background worker (services/bulk_reprocess_service.go)
-- that polls this table, claims batches via keyset pagination on last_processed_id,
-- and checkpoints progress after each batch so a job survives an app restart.

-- ============================================================
-- BULK REPROCESS JOBS
-- ============================================================

CREATE TABLE IF NOT EXISTS bulk_reprocess_jobs (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id         UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    table_name           VARCHAR(255) NOT NULL,
    -- Resolved messages_intf_* table name at creation time, so the worker
    -- never has to re-resolve it via interface_table_metadata on every poll.

    selection_mode       VARCHAR(20) NOT NULL DEFAULT 'filter',
    -- 'filter' = filter JSONB below defines the match set (any size)
    -- 'ids'    = explicit_ids below is the exact set (small, checkbox-driven)
    filter               JSONB,
    -- e.g. {"status":"failed","messageType":"ADT^A01","dateFrom":"...","dateTo":"..."}
    -- Same shape as messages.html's existing filter bar — NULL when selection_mode='ids'.
    explicit_ids         TEXT[],
    -- NULL when selection_mode='filter'. Application layer caps this array's
    -- size (small, precise selections only) — not enforced at the DB layer.

    status                VARCHAR(20) NOT NULL DEFAULT 'queued',
    -- 'queued' | 'running' | 'completed' | 'cancelled' | 'failed'
    cancel_requested      BOOLEAN NOT NULL DEFAULT false,
    -- Set instantly by the cancel API; the worker only flips status to
    -- 'cancelled' between batches, avoiding a race with its own status writes.

    total_matched         BIGINT NOT NULL DEFAULT 0,
    -- Computed once at creation (COUNT(*) for 'filter', array length for 'ids').
    processed_count        BIGINT NOT NULL DEFAULT 0,
    succeeded_count         BIGINT NOT NULL DEFAULT 0,
    failed_count            BIGINT NOT NULL DEFAULT 0,
    last_processed_id       VARCHAR(255),
    -- Keyset checkpoint (message_id of the last row processed) — the worker's
    -- next batch query is "WHERE id > last_processed_id ORDER BY id LIMIT batch_size",
    -- never OFFSET, so resuming a huge job is O(batch_size), not O(rows already done).

    batch_size             INTEGER NOT NULL DEFAULT 500,
    concurrency            INTEGER NOT NULL DEFAULT 10,
    throttle_per_sec        INTEGER,
    -- NULL = uncapped. When set, the worker paces batches so it never exceeds
    -- roughly this many message reprocess calls per second (protects a real
    -- downstream system on the other end of the interface's outbound connector).

    error_summary          JSONB NOT NULL DEFAULT '[]',
    -- Most recent failures only ({"messageId":"...","error":"..."}), capped by
    -- the worker to the last 100 — full failure detail is never dropped from
    -- the source message's own last_error_message column, only summarized here.

    created_by_user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at              TIMESTAMPTZ,
    completed_at            TIMESTAMPTZ
);

-- Worker's own poll query: WHERE status IN ('queued','running')
CREATE INDEX IF NOT EXISTS idx_bulk_reprocess_jobs_status
    ON bulk_reprocess_jobs(status);

-- "List my recent/active jobs for this interface" (Messages page job panel)
CREATE INDEX IF NOT EXISTS idx_bulk_reprocess_jobs_interface_created
    ON bulk_reprocess_jobs(interface_id, created_at DESC);

CREATE TRIGGER trg_bulk_reprocess_jobs_updated_at
    BEFORE UPDATE ON bulk_reprocess_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE bulk_reprocess_jobs IS
    'Async bulk message-reprocess jobs (filter-based or explicit-ID-based). Created instantly by the API; processed by a background worker in batches, checkpointed for resumability, cancellable mid-run.';
