-- V208: CDA Coverage Audit notifications — per-interface cooldown state for
-- external alert dispatch (email/webhook/Slack via the existing alert_channels
-- system) when a coverage audit finds gaps.
-- Applied: 2026-08-04

-- ============================================================
-- COVERAGE-GAP NOTIFICATION COOLDOWN
-- ============================================================
-- A plain column on interfaces, not a separate table — interface_alert_thresholds
-- (V139/V142) shows what happens to cooldown state kept in a separate table:
-- nothing ever seeds a row for it, so it silently never fires. Read/written
-- atomically (UPDATE ... WHERE last_coverage_gap_notified_at <= NOW() - interval
-- ... RETURNING id) by services/cda_coverage/worker_pool.go, since multiple
-- worker goroutines can process gap-containing jobs for the same interface
-- concurrently.
--
-- Notification destination/throttle configuration itself lives in the existing
-- interfaces.cda_coverage_audit_config JSONB (V207) as
-- {"enabled": true, "notify": {"channel_ids": [...], "cooldown_minutes": 60}} —
-- no schema change needed there, it's already an opaque JSONB blob.
ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS last_coverage_gap_notified_at TIMESTAMPTZ;

COMMENT ON COLUMN interfaces.last_coverage_gap_notified_at IS
    'Last time a CDA Coverage Audit gap notification was dispatched for this interface — gates the cooldown configured in cda_coverage_audit_config.notify.cooldown_minutes. NULL = never notified.';
