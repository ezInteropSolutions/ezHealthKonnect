-- V280: per-source fingerprints for ezCompanion's static knowledge.
--
-- Before this, Go startup skipped ALL static knowledge ingestion whenever any
-- static chunk existed, so an upgraded install kept the previous release's
-- docs/schemas knowledge until an admin re-ingested by hand. Each static
-- source (services/ai/knowledge_sources.go Sources()) now records the content
-- fingerprint it was last successfully ingested at; startup re-ingests only
-- sources whose fingerprint changed.
--
-- Existing installs start with no rows, so the first startup after this
-- migration re-ingests every static source once (background, non-blocking).
CREATE TABLE IF NOT EXISTS ai_knowledge_sources (
    source_key     VARCHAR(200) PRIMARY KEY,
    fingerprint    VARCHAR(64)  NOT NULL,
    files_scanned  INTEGER      NOT NULL DEFAULT 0,
    chunks_stored  INTEGER      NOT NULL DEFAULT 0,
    error_count    INTEGER      NOT NULL DEFAULT 0,
    ingested_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE ai_knowledge_sources IS
    'Fingerprint of each ezCompanion static knowledge source at its last successful ingestion; startup re-ingests only changed sources.';
