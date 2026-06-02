-- V144: Product config table for encrypted product-internal configuration
-- Applied: 2026-05-29

-- ============================================================
-- PRODUCT CONFIG TABLE
-- ============================================================
-- Stores product-internal configuration that is:
--   - NOT user-configurable (hidden from Settings UI)
--   - Encrypted at rest using AES-256-GCM via CredentialStore
--   - Seeded by the official installer or by the app on first startup
--   - Separate from system_settings which holds user-visible config
--
-- Examples: telemetry HMAC secret, telemetry endpoint URL,
--           future license keys, support webhook URLs.
--
-- Community / self-built installs: rows seeded with NULL encrypted_value
--   → TelemetryService detects this and silently disables telemetry.
-- Official installer builds: values written encrypted before app starts
--   → TelemetryService decrypts at runtime via APP_CREDENTIAL_KEY.

CREATE TABLE IF NOT EXISTS product_config (
    key             VARCHAR(100)  PRIMARY KEY,
    encrypted_value BYTEA,
    description     TEXT,
    seeded_by       VARCHAR(50)   NOT NULL DEFAULT 'default',
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE product_config IS
    'Product-internal encrypted configuration. NOT user-configurable. '
    'Values are AES-256-GCM encrypted using APP_CREDENTIAL_KEY. '
    'Seeded by the official installer; app seeds defaults on first startup.';

COMMENT ON COLUMN product_config.encrypted_value IS
    'AES-256-GCM ciphertext from CredentialStore.Encrypt(). '
    'NULL or zero-length = no value seeded; feature silently disabled.';

COMMENT ON COLUMN product_config.seeded_by IS
    'Who wrote this row: installer | migration | api | default.';

-- ============================================================
-- SEED PLACEHOLDER ROWS (NULL values — real values from app/installer)
-- ============================================================

INSERT INTO product_config (key, encrypted_value, description, seeded_by)
VALUES
    ('telemetry_secret',
     NULL,
     'HMAC-SHA256 signing secret for telemetry payloads. '
     'Null = telemetry disabled. Seeded encrypted by app on first startup.',
     'migration'),

    ('telemetry_endpoint',
     NULL,
     'HTTPS endpoint that receives install_ping and feedback_submit events. '
     'Null = telemetry disabled. Seeded encrypted by app on first startup.',
     'migration')

ON CONFLICT (key) DO NOTHING;
