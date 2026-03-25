-- ============================================================================
-- V73: Alert Rules Engine
-- Dynamic, configurable threshold rules replacing hardcoded checkAndWriteAlerts()
-- Supports global rules (interface_id IS NULL) and per-interface overrides.
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_rules (
    id                        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name                      VARCHAR(255) NOT NULL,
    description               TEXT,

    -- NULL = global rule applied to all interfaces
    -- non-NULL = per-interface override (takes precedence over global for same rule_type)
    interface_id              UUID         REFERENCES interfaces(id) ON DELETE CASCADE,

    -- ERROR_RATE | PROCESSING_TIME | DLQ_DEPTH | INTERFACE_INACTIVE
    -- CONNECTOR_FAILURE | THROUGHPUT_LOW | VALIDATION_FAILURE
    rule_type                 VARCHAR(100) NOT NULL,

    -- gt | lt | gte | lte
    condition                 VARCHAR(20)  NOT NULL DEFAULT 'gt',

    -- NULL = that severity level disabled
    threshold_warning         NUMERIC,
    threshold_critical        NUMERIC,

    -- Rolling window for metric evaluation (minutes)
    evaluation_window_minutes INT          NOT NULL DEFAULT 60,

    -- Suppress re-fire of same alert type for this many minutes after first fire
    cooldown_minutes          INT          NOT NULL DEFAULT 60,

    is_enabled                BOOLEAN      NOT NULL DEFAULT TRUE,

    -- OOB system rules: users may disable but cannot delete
    is_system                 BOOLEAN      NOT NULL DEFAULT FALSE,

    created_by_user_id        UUID,
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ar_interface_id ON alert_rules(interface_id);
CREATE INDEX IF NOT EXISTS idx_ar_rule_type    ON alert_rules(rule_type);
CREATE INDEX IF NOT EXISTS idx_ar_enabled      ON alert_rules(is_enabled) WHERE is_enabled = TRUE;

-- ── OOB System Rules ──────────────────────────────────────────────────────────
-- These mirror the hardcoded thresholds in analytics_service.go::checkAndWriteAlerts()
-- exactly. Behaviour is identical at first deployment; users can tune from the UI.

INSERT INTO alert_rules
    (name, description, rule_type, condition,
     threshold_warning, threshold_critical,
     evaluation_window_minutes, cooldown_minutes,
     is_enabled, is_system)
VALUES
(
    'System Error Rate',
    'Fires when the system-wide pipeline error rate exceeds threshold over the evaluation window.',
    'ERROR_RATE', 'gt',
    5.0, 10.0,
    60, 60,
    TRUE, TRUE
),
(
    'Slow Processing',
    'Fires when the average pipeline processing time exceeds threshold (milliseconds).',
    'PROCESSING_TIME', 'gt',
    2000, NULL,
    60, 60,
    TRUE, TRUE
),
(
    'DLQ Depth High',
    'Fires when the dead letter queue depth exceeds threshold.',
    'DLQ_DEPTH', 'gt',
    10, 50,
    60, 30,
    TRUE, TRUE
),
(
    'Interface Inactive',
    'Fires when an active interface has received no messages within the evaluation window.',
    'INTERFACE_INACTIVE', 'lt',
    1, NULL,
    120, 120,
    TRUE, TRUE
),
(
    'Throughput Low',
    'Fires when message throughput drops below the warning threshold (messages per window).',
    'THROUGHPUT_LOW', 'lt',
    10, NULL,
    60, 60,
    TRUE, TRUE
),
(
    'Validation Failures',
    'Fires when the count of critical validation-stage errors exceeds threshold.',
    'VALIDATION_FAILURE', 'gt',
    5, 20,
    60, 60,
    TRUE, TRUE
)
ON CONFLICT DO NOTHING;
