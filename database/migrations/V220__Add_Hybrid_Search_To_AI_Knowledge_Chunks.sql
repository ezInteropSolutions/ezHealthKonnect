-- V220: Add hybrid (lexical) full-text search support to ai_knowledge_chunks
-- Applied: 2026-08-30

-- ============================================================
-- WHY
-- ============================================================
-- ezCompanion's RAG retrieval (services/ai/rag_service.go) is pure pgvector
-- cosine-similarity search today. That works well for conceptual questions
-- but underperforms on exact-token questions — the kind IT/integration
-- engineers actually ask (an HL7 segment path like "PID.3", an exact error
-- string, a connector type name) — where a semantically-similar-but-wrong
-- chunk can outrank the chunk that literally contains the match.
--
-- This migration adds a generated tsvector column + GIN index so a lexical
-- (keyword) search can run alongside the existing dense vector search and
-- be fused with it (Reciprocal Rank Fusion, implemented in Go). Purely
-- additive: new column + new index only, no existing column, data, or
-- index touched. No new Postgres extension required — tsvector/GIN are
-- core Postgres, and the same to_tsvector/GIN pattern is already used
-- elsewhere in this schema (see V38__Add_Response_Mapping_Templates.sql).
-- ============================================================

ALTER TABLE ai_knowledge_chunks
    ADD COLUMN IF NOT EXISTS content_tsv tsvector
        GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX IF NOT EXISTS ai_knowledge_chunks_content_tsv_idx
    ON ai_knowledge_chunks USING gin(content_tsv);

COMMENT ON COLUMN ai_knowledge_chunks.content_tsv IS
    'Generated tsvector for lexical/full-text search, fused with dense vector search via RRF (see services/ai/rag_service.go). Off by default — gated by system_settings key "ai".hybrid_retrieval_enabled.';
