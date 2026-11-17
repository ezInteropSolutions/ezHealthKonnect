-- V291: pipeline_state, the expiring key-value store behind the state.store
-- pipeline step: something one message's pipeline leaves for a later
-- message's, e.g. where the next page of a query's results starts (the HL7
-- continuation protocol). Entries are scoped to a pipeline and namespace,
-- and expire; expired entries are cleared on each write.

CREATE TABLE IF NOT EXISTS pipeline_state (
    scope      TEXT        NOT NULL,
    key        TEXT        NOT NULL,
    value      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key)
);

CREATE INDEX IF NOT EXISTS idx_pipeline_state_expires_at ON pipeline_state (expires_at);

COMMENT ON TABLE pipeline_state IS
    'Expiring values kept by state.store pipeline steps (scope = pipeline id/namespace).';
