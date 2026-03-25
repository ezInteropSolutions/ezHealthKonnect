-- ============================================================================
-- V75: Alert Silences (Maintenance Windows)
-- Suppress alert firing during planned maintenance windows.
-- Silences can target all interfaces/rules (NULL) or be scoped to specific ones.
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_silences (
    id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name               VARCHAR(255) NOT NULL,
    reason             TEXT,

    -- NULL = silence applies to all interfaces
    interface_id       UUID         REFERENCES interfaces(id) ON DELETE CASCADE,

    -- NULL = silence applies to all rules
    alert_rule_id      UUID         REFERENCES alert_rules(id) ON DELETE CASCADE,

    starts_at          TIMESTAMPTZ  NOT NULL,
    ends_at            TIMESTAMPTZ  NOT NULL,

    created_by_user_id UUID,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT alert_silences_time_range_check CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_as_interface_id ON alert_silences(interface_id);
CREATE INDEX IF NOT EXISTS idx_as_rule_id      ON alert_silences(alert_rule_id);
CREATE INDEX IF NOT EXISTS idx_as_time_range   ON alert_silences(starts_at, ends_at);

-- Regular index on ends_at for active-silence lookups (partial index with NOW() not allowed in PG)
CREATE INDEX IF NOT EXISTS idx_as_active ON alert_silences(ends_at);
