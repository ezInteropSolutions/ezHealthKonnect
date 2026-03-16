-- V51__HIPAA_Audit_And_DLQ.sql
-- HIPAA Strengthening (P1):
--   1. pipeline_step_audit — per-step PHI field hash audit trail (HIPAA 164.312)
--   2. dead_letter_queue   — failed message retention + manual re-processing
--
-- SHA-256 hashes are stored instead of raw PHI data (safe for audit purposes).

-- ─── Pipeline Step PHI Audit ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS pipeline_step_audit (
    id                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id        UUID            NOT NULL,       -- maps to transformation_executions.id
    pipeline_id         UUID            NOT NULL,
    interface_id        UUID            NOT NULL,
    step_name           VARCHAR(255)    NOT NULL,
    step_type           VARCHAR(100)    NOT NULL,
    step_namespace      VARCHAR(255),                  -- runtime namespace (e.g. api_enrichment_a1b2c3)
    input_field_hash    TEXT,                          -- SHA-256 of PHI input fields (never raw)
    output_field_hash   TEXT,                          -- SHA-256 of PHI output fields (never raw)
    phi_fields_touched  TEXT[],                        -- list of field paths that contain PHI
    masked_fields       TEXT[],                        -- fields that were masked in this step
    processed_at        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    duration_ms         INTEGER         NOT NULL DEFAULT 0,
    success             BOOLEAN         NOT NULL DEFAULT TRUE,
    error_message       TEXT,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_psa_execution_id ON pipeline_step_audit (execution_id);
CREATE INDEX IF NOT EXISTS idx_psa_pipeline_id  ON pipeline_step_audit (pipeline_id);
CREATE INDEX IF NOT EXISTS idx_psa_interface_id ON pipeline_step_audit (interface_id);
CREATE INDEX IF NOT EXISTS idx_psa_processed_at ON pipeline_step_audit (processed_at DESC);

COMMENT ON TABLE  pipeline_step_audit                  IS 'HIPAA 164.312 — Per-step PHI access audit trail. Stores hashes only, never raw PHI.';
COMMENT ON COLUMN pipeline_step_audit.input_field_hash  IS 'SHA-256(sorted PHI input field values) — proves data was seen but cannot be reconstructed.';
COMMENT ON COLUMN pipeline_step_audit.output_field_hash IS 'SHA-256(sorted PHI output field values) — proves data was transformed.';

-- ─── Dead Letter Queue ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id    UUID            NOT NULL,
    pipeline_id     UUID,
    message_id      VARCHAR(255),
    raw_message     TEXT            NOT NULL,           -- original message content
    error_message   TEXT            NOT NULL,
    error_step      VARCHAR(255),                      -- step name where failure occurred
    error_type      VARCHAR(100),                      -- e.g. EXECUTOR_ERROR, TIMEOUT, VALIDATION
    failed_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    retry_count     INTEGER         NOT NULL DEFAULT 0,
    max_retries     INTEGER         NOT NULL DEFAULT 3,
    next_retry_at   TIMESTAMPTZ,
    resolved        BOOLEAN         NOT NULL DEFAULT FALSE,
    resolved_at     TIMESTAMPTZ,
    resolved_by     VARCHAR(255),                      -- user or system that resolved it
    resolution_note TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dlq_interface_id ON dead_letter_queue (interface_id);
CREATE INDEX IF NOT EXISTS idx_dlq_resolved     ON dead_letter_queue (resolved) WHERE resolved = FALSE;
CREATE INDEX IF NOT EXISTS idx_dlq_failed_at    ON dead_letter_queue (failed_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlq_next_retry   ON dead_letter_queue (next_retry_at) WHERE resolved = FALSE AND next_retry_at IS NOT NULL;

COMMENT ON TABLE dead_letter_queue IS 'Failed messages awaiting manual review or automatic retry. Raw message stored for re-processing.';
