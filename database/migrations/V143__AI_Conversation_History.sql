-- V100: Add user_id index on ai_conversations for efficient history queries by user.
-- The session index (session_id, created_at) already exists from V82.

CREATE INDEX IF NOT EXISTS ai_conversations_user_idx
    ON ai_conversations (user_id, created_at DESC)
    WHERE user_id IS NOT NULL;
