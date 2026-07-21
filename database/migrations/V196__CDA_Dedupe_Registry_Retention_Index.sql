-- V196: Add last_seen_at index on cda_dedupe_registry for retention purge
-- Applied: 2026-07-19

-- ============================================================
-- CDA DEDUPE REGISTRY — RETENTION INDEX
-- ============================================================
-- Supports RetentionEnforcementService.enforceCDADedupeRegistry, which purges
-- rows whose last_seen_at has aged past the configured retention window
-- (GDPR Art. 5(1)(e) storage limitation). Neither existing index on this
-- table (the UNIQUE identity constraint, or idx_cda_dedupe_registry_patient
-- on (interface_id, patient_key)) covers a WHERE last_seen_at < cutoff scan —
-- without this index, the hourly retention sweep table-scans as the table
-- grows.

CREATE INDEX IF NOT EXISTS idx_cda_dedupe_registry_last_seen
    ON cda_dedupe_registry(last_seen_at);
