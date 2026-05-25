-- V139: Rolling 5-minute aggregated transformation metrics per interface.
-- Populated by a background goroutine; drives the monitoring dashboard
-- and alerting thresholds.

CREATE TABLE interface_transformation_metrics (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id              UUID        NOT NULL,
    message_type              VARCHAR(50),            -- NULL = all types combined
    period_start              TIMESTAMPTZ NOT NULL,
    period_end                TIMESTAMPTZ NOT NULL,
    messages_scored           INTEGER     NOT NULL DEFAULT 0,
    messages_flagged          INTEGER     NOT NULL DEFAULT 0,
    messages_delivered        INTEGER     NOT NULL DEFAULT 0,
    messages_dlq              INTEGER     NOT NULL DEFAULT 0,
    avg_quality_score         NUMERIC(5,2),
    min_quality_score         NUMERIC(5,2),
    p50_latency_ms            INTEGER,
    p95_latency_ms            INTEGER,
    p99_latency_ms            INTEGER,
    dlq_depth                 INTEGER     NOT NULL DEFAULT 0,   -- point-in-time snapshot
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Dashboard time-series queries
CREATE INDEX idx_itm_interface_time
    ON interface_transformation_metrics (interface_id, period_start DESC);

-- Alerting: find interfaces with high flag rates in the last hour
CREATE INDEX idx_itm_flagged_rate
    ON interface_transformation_metrics (period_start DESC, messages_flagged)
    WHERE messages_flagged > 0;

-- Alerting thresholds — one row per interface, upserted by the metrics goroutine
CREATE TABLE interface_alert_thresholds (
    interface_id          UUID        PRIMARY KEY,
    max_flag_rate_pct     NUMERIC(5,2) NOT NULL DEFAULT 20.0,  -- alert if >20% flagged
    max_dlq_depth         INTEGER      NOT NULL DEFAULT 10,
    alert_email           TEXT,
    alert_webhook_url     TEXT,
    enabled               BOOLEAN      NOT NULL DEFAULT true,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
