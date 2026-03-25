-- ============================================================
-- V84: Git Integration
-- Enables users to version-control their interface/pipeline
-- configurations using Git directly from within the app.
--
-- git_repositories: one row per connected remote repo
-- git_sync_log:     append-only audit trail of every git op
-- ============================================================

CREATE TABLE IF NOT EXISTS git_repositories (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name                VARCHAR(255) NOT NULL,
    description         TEXT         NOT NULL DEFAULT '',
    remote_url          TEXT         NOT NULL,
    branch              VARCHAR(255) NOT NULL DEFAULT 'main',
    -- local_path: server-side clone location.
    -- NULL = auto-resolved to /data/git-repos/{id} at runtime.
    local_path          TEXT,
    auth_type           VARCHAR(50)  NOT NULL DEFAULT 'token',
    -- credentials: encrypted by CredentialStore (AES-256-GCM).
    -- Plaintext shape: {"username":"...","token":"..."}
    credentials         JSONB        NOT NULL DEFAULT '{}',
    auto_push           BOOLEAN      NOT NULL DEFAULT false,
    is_active           BOOLEAN      NOT NULL DEFAULT true,
    last_sync_at        TIMESTAMP WITH TIME ZONE,
    last_sync_status    VARCHAR(50)  NOT NULL DEFAULT '',
    last_sync_error     TEXT         NOT NULL DEFAULT '',
    created_by_user_id  UUID         REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS git_sync_log (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id         UUID         NOT NULL REFERENCES git_repositories(id) ON DELETE CASCADE,
    operation       VARCHAR(50)  NOT NULL,
    -- interface_id is nullable: repo-level ops (pull, checkout) have no interface
    interface_id    UUID,
    branch          VARCHAR(255) NOT NULL DEFAULT '',
    commit_sha      VARCHAR(64)  NOT NULL DEFAULT '',
    commit_message  TEXT         NOT NULL DEFAULT '',
    files_changed   JSONB        NOT NULL DEFAULT '[]',
    status          VARCHAR(50)  NOT NULL,
    error_message   TEXT         NOT NULL DEFAULT '',
    duration_ms     INTEGER      NOT NULL DEFAULT 0,
    triggered_by    UUID         REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_git_repositories_active
    ON git_repositories(is_active);

CREATE INDEX IF NOT EXISTS idx_git_sync_log_repo_created
    ON git_sync_log(repo_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_git_sync_log_interface
    ON git_sync_log(interface_id, created_at DESC);

CREATE OR REPLACE TRIGGER trg_git_repositories_updated_at
    BEFORE UPDATE ON git_repositories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
