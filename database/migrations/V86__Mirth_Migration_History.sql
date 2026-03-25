-- V86: Mirth Connect Migration History
-- Tracks every channel imported from Mirth Connect for audit and rollback purposes.

CREATE TABLE IF NOT EXISTS mirth_migration_history (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id    UUID REFERENCES interfaces(id) ON DELETE SET NULL,
    channel_id      VARCHAR(255),               -- original Mirth channel UUID
    channel_name    VARCHAR(255) NOT NULL,
    mirth_version   VARCHAR(50),
    steps_created   INTEGER DEFAULT 0,
    steps_skipped   INTEGER DEFAULT 0,
    migrated_by     VARCHAR(255),               -- user ID or "system"
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mirth_migration_history_interface
    ON mirth_migration_history (interface_id);

CREATE INDEX IF NOT EXISTS idx_mirth_migration_history_channel
    ON mirth_migration_history (channel_id);

COMMENT ON TABLE mirth_migration_history IS
    'Audit log of Mirth Connect channel imports. One row per import operation.';
