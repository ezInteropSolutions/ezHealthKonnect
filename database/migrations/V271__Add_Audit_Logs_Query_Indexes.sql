-- V271: Add missing indexes on audit_logs for real, confirmed query patterns
-- Applied: 2026-09-20

-- ============================================================
-- CONTEXT
-- ============================================================
-- V1 (schema_only) only ever indexed audit_logs(user_id) and audit_logs(created_at).
-- The 4-phase audit-logging work (this same session) turned audit_logs into a real,
-- steadily-growing hot table (every interface/pipeline/user/message/DLQ mutation now
-- writes a row), and confirmed 3 real, currently-unindexed query patterns against it:
--
--   1. controllers/auditController.js (GET /api/audit-logs, the admin audit-log
--      viewer) filters on risk_level (exact match) and action (Sequelize
--      `Op.iLike: '%value%'` — a case-insensitive, LEADING-wildcard substring
--      search, confirmed via public/js/user-management.js's loadAuditLogs()).
--      A plain btree index cannot accelerate a leading-wildcard LIKE/ILIKE at all —
--      only risk_level gets a plain btree here; action gets a pg_trgm GIN index,
--      the correct tool for this specific query shape.
--   2. services/ai/context_builder.go's fetchRecentErrors (called on every AI
--      Copilot context build) filters on entity_id + result='FAILURE' with a
--      created_at range. services/ai/operational_ingestion.go's error-pattern
--      ingestion filters on result='FAILURE' with a created_at range, no entity_id.
--   3. routes/users.js's GET /:id/activity filters on user_id, ordered by
--      created_at DESC — already served by the existing standalone user_id index
--      plus a separate sort; a composite index removes the extra sort step.
--
-- Purely additive — no existing index is dropped or altered. Table is currently
-- small (~14k rows) so this build runs instantly; done without CONCURRENTLY to
-- match this repo's existing migration convention (see V141's idx_audit_dlq_row),
-- since Flyway migrations here already run inside a transaction.

-- ============================================================
-- EXACT-MATCH FILTER INDEXES
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_audit_logs_risk_level
    ON audit_logs (risk_level);

CREATE INDEX IF NOT EXISTS idx_audit_logs_entity_id
    ON audit_logs (entity_id);

-- ============================================================
-- COMPOSITE INDEXES FOR CONFIRMED RANGE/SORT QUERY SHAPES
-- ============================================================

-- Serves context_builder.go's and operational_ingestion.go's
-- `WHERE result = 'FAILURE' AND created_at > NOW() - INTERVAL '...'` scans.
CREATE INDEX IF NOT EXISTS idx_audit_logs_result_created_at
    ON audit_logs (result, created_at);

-- Serves routes/users.js's `WHERE user_id = :id ORDER BY created_at DESC`
-- without a separate sort step. Additive alongside the existing standalone
-- idx_audit_logs_user_id (V1) — not dropped here; that index is now largely
-- redundant given this composite's leading column, but removing it is a
-- separate decision left for a dedicated cleanup pass, not bundled into this
-- purely-additive migration.
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id_created_at
    ON audit_logs (user_id, created_at DESC);

-- ============================================================
-- TRIGRAM INDEX FOR THE REAL action SUBSTRING-SEARCH QUERY
-- ============================================================
-- auditController.js's `action ILIKE '%value%'` is a leading-wildcard substring
-- search — no plain btree index can serve it. pg_trgm + a GIN trigram index is
-- the standard, correct Postgres mechanism for accelerating this exact query
-- shape (confirmed available in this Postgres image: pg_trgm 1.6, not yet
-- installed, before this migration).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_audit_logs_action_trgm
    ON audit_logs USING GIN (action gin_trgm_ops);

COMMENT ON INDEX idx_audit_logs_action_trgm IS
    'Trigram GIN index accelerating auditController.js''s action ILIKE ''%value%'' substring search. A plain btree index cannot serve a leading-wildcard pattern.';
