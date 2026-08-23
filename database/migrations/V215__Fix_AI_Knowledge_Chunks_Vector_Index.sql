-- V215: Replace ai_knowledge_chunks' ivfflat vector index with HNSW
-- Applied: 2026-08-23

-- ============================================================
-- BUG
-- ============================================================
-- V82__AI_Knowledge_Base.sql created ai_knowledge_chunks_embedding_idx as an
-- ivfflat index with lists=100. ivfflat's own tuning guidance is
-- lists ~= rows/1000 (minimum 1) for up to ~1M rows — with only a few
-- hundred rows in this table, 100 lists means most clusters hold a
-- handful of rows or none at all. Combined with Postgres's default
-- ivfflat.probes=1 (search only 1 of the 100 clusters per query), a RAG
-- query can silently land on an empty/wrong cluster and return ZERO
-- results even when a highly relevant chunk exists in the table —
-- confirmed live: the same query returned 0 rows at probes=1 and the
-- correct top match (77.6% cosine similarity) at probes=100.
--
-- Since RAGService.Retrieve (services/ai/rag_service.go) never sets
-- ivfflat.probes and always runs at the default of 1, this was a
-- silent, unpredictable correctness bug affecting any RAG-backed
-- ezCompanion answer, not just newly-ingested content.
--
-- FIX: switch to an HNSW index. HNSW is graph-based rather than
-- cluster-based, doesn't have ivfflat's "wrong/empty cluster" failure
-- mode, and its recall stays high without needing a lists parameter
-- re-tuned as the table grows — the same index definition works whether
-- ai_knowledge_chunks has hundreds or hundreds of thousands of rows.
-- ============================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        EXECUTE $sql$
            DROP INDEX IF EXISTS ai_knowledge_chunks_embedding_idx
        $sql$;
        EXECUTE $sql$
            CREATE INDEX IF NOT EXISTS ai_knowledge_chunks_embedding_idx
                ON ai_knowledge_chunks
                USING hnsw (embedding vector_cosine_ops)
        $sql$;
    END IF;
END $$;
