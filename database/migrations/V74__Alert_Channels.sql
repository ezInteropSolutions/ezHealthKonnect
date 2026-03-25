-- ============================================================================
-- V74: Alert Channels
-- Notification destinations for alert rules: email, webhook, Slack.
-- alert_rule_channels links rules to channels with optional severity filters.
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_channels (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name             VARCHAR(255) NOT NULL,

    -- email | webhook | slack
    channel_type     VARCHAR(50)  NOT NULL,

    -- channel_type = email:
    --   { "to": ["ops@example.com"], "smtp_host": "smtp.example.com",
    --     "smtp_port": 587, "smtp_user": "user", "smtp_password_enc": "...",
    --     "from": "alerts@example.com" }
    -- channel_type = webhook:
    --   { "url": "https://...", "method": "POST",
    --     "headers": {"Authorization": "Bearer ..."}, "timeout_seconds": 10 }
    -- channel_type = slack:
    --   { "webhook_url": "https://hooks.slack.com/services/...",
    --     "channel": "#alerts", "username": "ezHealthKonnect",
    --     "icon_emoji": ":hospital:" }
    config           JSONB        NOT NULL DEFAULT '{}',

    is_enabled       BOOLEAN      NOT NULL DEFAULT TRUE,

    -- Populated by the TestChannel endpoint
    test_status      VARCHAR(50),        -- success | failure | untested
    test_error       TEXT,
    last_tested_at   TIMESTAMPTZ,

    created_by_user_id UUID,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ac_channel_type ON alert_channels(channel_type);
CREATE INDEX IF NOT EXISTS idx_ac_enabled      ON alert_channels(is_enabled) WHERE is_enabled = TRUE;

-- ── Rule ↔ Channel assignment ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_rule_channels (
    rule_id          UUID    NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    channel_id       UUID    NOT NULL REFERENCES alert_channels(id) ON DELETE CASCADE,

    -- Empty array {} = notify on all severities.
    -- Non-empty = only notify when fired alert severity is in this list.
    -- e.g. '{"critical"}' means only page on critical, not warning.
    severity_filter  TEXT[]  NOT NULL DEFAULT '{}',

    PRIMARY KEY (rule_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_arc_channel_id ON alert_rule_channels(channel_id);
CREATE INDEX IF NOT EXISTS idx_arc_rule_id    ON alert_rule_channels(rule_id);
