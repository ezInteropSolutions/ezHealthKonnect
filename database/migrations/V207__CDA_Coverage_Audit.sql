-- V207: CDA Coverage Audit — per-interface opt-in tracking of CDA/CCD source
-- elements that were never read by any pipeline step, reported per delivery
-- destination after the message is delivered or errored.
-- Applied: 2026-08-04

-- ============================================================
-- INTERFACE-LEVEL OPT-IN (off by default)
-- ============================================================
-- NULL = disabled. Deliberately NOT backfilled with a default config object
-- (unlike dlq_config in V141) — this feature adds per-message overhead when
-- enabled, so every existing interface should stay off until a user
-- explicitly opts in. Shape when enabled: {"enabled": true}.
ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS cda_coverage_audit_config JSONB;

-- ============================================================
-- CDA COVERAGE AUDITS
-- ============================================================
-- One row per (message, outbound destination) — a pipeline can fan out to
-- multiple connector.outbound steps, and coverage is reported separately for
-- each since different destinations may be reached via different branches.
--
-- category_stats intentionally stores only structural location (section,
-- entry index, field label) for anything missed, never the clinical value
-- itself — this table is not a PHI store. To see the actual value behind a
-- gap, follow message_id back into the existing, already access-controlled
-- message detail view.
CREATE TABLE IF NOT EXISTS cda_coverage_audits (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id           UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    message_id             VARCHAR(255) NOT NULL,
    step_id                UUID,
    step_name              VARCHAR(255),
    connector_type         VARCHAR(100),
    destination            VARCHAR(500),
    delivery_outcome       VARCHAR(20) NOT NULL CHECK (delivery_outcome IN ('delivered', 'failed')),
    overall_coverage_pct   NUMERIC(5,2),
    category_stats         JSONB NOT NULL DEFAULT '{}',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cda_coverage_audits_message
    ON cda_coverage_audits(message_id);

CREATE INDEX IF NOT EXISTS idx_cda_coverage_audits_interface_created
    ON cda_coverage_audits(interface_id, created_at);

COMMENT ON TABLE cda_coverage_audits IS
    'Per-destination report of CDA/CCD source elements never read by any pipeline step. Structural location only (no clinical values) — see V207 migration header.';
COMMENT ON COLUMN interfaces.cda_coverage_audit_config IS
    'NULL = coverage audit disabled (default). Non-null opts this interface into per-message coverage tracking.';
