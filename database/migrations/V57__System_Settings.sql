-- V57__System_Settings.sql
-- System-wide key-value settings table.
-- Each row holds a named configuration blob (JSONB) that the Go backend reads
-- at request time so changes take effect without a server restart.
--
-- Initial seed row: object_storage — mirrors the OBJECT_STORAGE_* env vars so
-- the admin UI can override them at runtime without touching the container.

-- ─── 1. Settings table ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS system_settings (
    key         VARCHAR(100) PRIMARY KEY,
    value       JSONB        NOT NULL DEFAULT '{}',
    description TEXT,
    updated_by  VARCHAR(255),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE system_settings IS 'Runtime-configurable system settings (overrides env vars)';
COMMENT ON COLUMN system_settings.key   IS 'Setting identifier, e.g. "object_storage"';
COMMENT ON COLUMN system_settings.value IS 'Encrypted JSONB config blob (sensitive fields wrapped in ENC:v1:... envelope)';

-- ─── 2. Auto-update updated_at on every change ────────────────────────────────
CREATE OR REPLACE FUNCTION update_system_settings_timestamp()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_system_settings_updated_at ON system_settings;
CREATE TRIGGER trg_system_settings_updated_at
    BEFORE UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION update_system_settings_timestamp();

-- ─── 3. Seed default object_storage row (no-op if already present) ────────────
-- Values are intentionally empty — the app falls back to env vars when blank.
-- The admin UI will populate real values once the admin configures storage.
INSERT INTO system_settings (key, value, description) VALUES (
    'object_storage',
    jsonb_build_object(
        'driver',      '',
        'bucket',      '',
        'region',      '',
        'endpoint',    '',
        'path_style',  false,
        'access_key',  '',
        'secret_key',  '',
        'local_path',  ''
    ),
    'Object storage configuration (S3/MinIO or local filesystem). Overrides OBJECT_STORAGE_* env vars when non-empty.'
) ON CONFLICT (key) DO NOTHING;
