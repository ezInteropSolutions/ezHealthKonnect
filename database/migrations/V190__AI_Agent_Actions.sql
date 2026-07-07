-- V190: AI Agent Actions
-- Applied: 2026-07-05
--
-- Agent Mode audit/approval table for ezCompanion. When the local LLM proposes
-- a tool call (retry a message, activate/deactivate an interface, etc.), a row
-- is written here with status='proposed' BEFORE anything executes. A human
-- must approve or reject it; only approval triggers the real service call.
-- Follows the before/after pattern established by audit_logs (V1) and the
-- session/user/metadata pattern established by ai_feedback (V86).

-- ============================================================
-- AI AGENT ACTIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS ai_agent_actions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id     VARCHAR(100) NOT NULL,
    user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    tool_name      VARCHAR(100) NOT NULL,
    arguments      JSONB NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'proposed'
                   CHECK (status IN ('proposed', 'rejected', 'executed', 'failed')),
    result         JSONB,
    error_message  TEXT,
    entity_type    VARCHAR(100),   -- e.g. 'interface', 'dlq_entry', 'pipeline_step'
    entity_id      VARCHAR(255),
    decided_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_at     TIMESTAMP WITH TIME ZONE,
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS ai_agent_actions_session_idx
    ON ai_agent_actions (session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS ai_agent_actions_status_idx
    ON ai_agent_actions (status)
    WHERE status = 'proposed';

CREATE INDEX IF NOT EXISTS ai_agent_actions_user_idx
    ON ai_agent_actions (user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

-- Comments / documentation
COMMENT ON TABLE ai_agent_actions IS 'Agent Mode tool-call proposals from ezCompanion and their approve/reject/execute outcome. No tool executes before a human approves the row.';
COMMENT ON COLUMN ai_agent_actions.arguments IS 'Raw JSON arguments the model supplied for the tool call, as proposed.';
COMMENT ON COLUMN ai_agent_actions.result IS 'Raw JSON result returned by the tool, populated only when status = executed.';
COMMENT ON COLUMN ai_agent_actions.decided_by IS 'User who approved or rejected the proposal.';
