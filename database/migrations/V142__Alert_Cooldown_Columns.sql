-- V142: Add alert cooldown columns to interface_alert_thresholds
-- Applied: 2026-05-24
--
-- Prevents alert flooding by tracking when the last alert was sent per
-- interface and per alert type, and adding a configurable cooldown period.
-- The MetricsCollector reads these columns before deciding whether to
-- re-fire a notification for the same threshold breach.

-- ============================================================
-- COOLDOWN TRACKING COLUMNS
-- ============================================================

ALTER TABLE interface_alert_thresholds
    ADD COLUMN IF NOT EXISTS cooldown_minutes   INTEGER      NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS last_flag_alert_at TIMESTAMPTZ,   -- last time a flag-rate alert was sent
    ADD COLUMN IF NOT EXISTS last_dlq_alert_at  TIMESTAMPTZ;   -- last time a DLQ-depth alert was sent

COMMENT ON COLUMN interface_alert_thresholds.cooldown_minutes IS
    'Minimum minutes between repeated alerts of the same type for this interface. Default 30.';
COMMENT ON COLUMN interface_alert_thresholds.last_flag_alert_at IS
    'Timestamp of the most recent flag-rate alert dispatch. NULL = never fired.';
COMMENT ON COLUMN interface_alert_thresholds.last_dlq_alert_at IS
    'Timestamp of the most recent DLQ-depth alert dispatch. NULL = never fired.';
