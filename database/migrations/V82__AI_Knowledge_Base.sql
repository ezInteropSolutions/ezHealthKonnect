-- V82__AI_Knowledge_Base.sql
-- Local AI: pgvector extension + knowledge base tables for RAG-based healthcare AI.
-- Gracefully degrades to JSONB embedding column when pgvector is not available.

-- Try to enable pgvector extension; ignore if not installed on this host
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector extension not available (%), embedding column will use JSONB fallback', SQLERRM;
END $$;

-- ─────────────────────────────────────────────────────────────────────────────
-- ai_knowledge_chunks
-- Chunked, embedded text from healthcare standards (HL7, FHIR, X12, CCD, etc.)
-- Used for RAG: embed a user query → find similar chunks → augment LLM prompt.
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        EXECUTE $sql$
            CREATE TABLE IF NOT EXISTS ai_knowledge_chunks (
                id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
                source_type VARCHAR(50)  NOT NULL,
                source_ref  VARCHAR(255),
                source_file VARCHAR(500),
                content     TEXT         NOT NULL,
                embedding   vector(768),
                metadata    JSONB,
                created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
                updated_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
            )
        $sql$;
    ELSE
        EXECUTE $sql$
            CREATE TABLE IF NOT EXISTS ai_knowledge_chunks (
                id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
                source_type VARCHAR(50)  NOT NULL,
                source_ref  VARCHAR(255),
                source_file VARCHAR(500),
                content     TEXT         NOT NULL,
                embedding   JSONB,
                metadata    JSONB,
                created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
                updated_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
            )
        $sql$;
    END IF;
END $$;

-- IVFFlat index only when pgvector is available
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        EXECUTE $sql$
            CREATE INDEX IF NOT EXISTS ai_knowledge_chunks_embedding_idx
                ON ai_knowledge_chunks
                USING ivfflat (embedding vector_cosine_ops)
                WITH (lists = 100)
        $sql$;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS ai_knowledge_chunks_source_type_idx
    ON ai_knowledge_chunks (source_type);

-- ─────────────────────────────────────────────────────────────────────────────
-- ai_conversations
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ai_conversations (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) NOT NULL,
    user_id    UUID,
    role       VARCHAR(20)  NOT NULL,
    content    TEXT         NOT NULL,
    metadata   JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS ai_conversations_session_idx
    ON ai_conversations (session_id, created_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- ai_ingestion_runs
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ai_ingestion_runs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    triggered_by  UUID,
    status        VARCHAR(20) NOT NULL DEFAULT 'running',
    source_types  TEXT[],
    files_scanned INTEGER     NOT NULL DEFAULT 0,
    chunks_stored INTEGER     NOT NULL DEFAULT 0,
    errors        JSONB,
    started_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at  TIMESTAMP WITH TIME ZONE
);
