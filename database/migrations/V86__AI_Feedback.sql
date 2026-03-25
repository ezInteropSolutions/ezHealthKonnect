-- V86: AI response feedback table
-- Stores user thumbs-up/down on every AI response for quality tracking
-- and KB improvement loop (positive responses auto-ingest into knowledge base).

CREATE TABLE IF NOT EXISTS ai_feedback (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id       TEXT,
    user_id          UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Which AI feature produced this response
    endpoint         TEXT NOT NULL CHECK (endpoint IN (
                         'ask', 'generate-script', 'trace-message',
                         'explain-step', 'suggest-mappings', 'explain-error'
                     )),

    -- Sentiment (required) and optional numeric rating
    sentiment        TEXT NOT NULL CHECK (sentiment IN ('positive', 'negative')),
    rating           SMALLINT CHECK (rating BETWEEN 1 AND 5),

    -- Truncated previews (200 chars max) — no full message content stored
    prompt_preview   TEXT,
    response_preview TEXT,

    -- Optional free-text comment from the user
    comment          TEXT,

    -- Context (for trend analysis and targeted KB ingestion)
    interface_id     UUID,
    pipeline_id      UUID,
    step_id          TEXT,

    -- Whether this feedback triggered a KB ingestion
    kb_ingested      BOOLEAN DEFAULT FALSE,

    metadata         JSONB DEFAULT '{}',
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_feedback_endpoint   ON ai_feedback (endpoint);
CREATE INDEX IF NOT EXISTS idx_ai_feedback_sentiment  ON ai_feedback (sentiment);
CREATE INDEX IF NOT EXISTS idx_ai_feedback_created_at ON ai_feedback (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_feedback_user_id    ON ai_feedback (user_id) WHERE user_id IS NOT NULL;
