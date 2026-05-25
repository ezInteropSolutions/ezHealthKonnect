-- V77__Enterprise_Settings.sql
-- Seeds the 7 new enterprise settings keys into system_settings.
-- All rows use ON CONFLICT DO NOTHING so existing customisations are preserved.

-- ── Email / SMTP ──────────────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'smtp',
    jsonb_build_object(
        'host',         '',
        'port',         587,
        'use_tls',      true,
        'require_auth', true,
        'username',     '',
        'password',     '',
        'from_address', '',
        'from_name',    'ezHealthKonnect'
    ),
    'SMTP server configuration for alert notification emails. Credentials are AES-256-GCM encrypted.'
) ON CONFLICT (key) DO NOTHING;

-- ── Security & Access ─────────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'security',
    jsonb_build_object(
        'session_timeout_minutes',  1440,
        'jwt_expiry_hours',         24,
        'max_login_attempts',       5,
        'lockout_duration_minutes', 30,
        'password_min_length',      8
    ),
    'Authentication and access-control policies. Session timeout and JWT expiry are applied immediately on next login.'
) ON CONFLICT (key) DO NOTHING;

-- ── HL7 / FHIR Processing ─────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'hl7_fhir',
    jsonb_build_object(
        'default_hl7_version',    '2.5',
        'default_fhir_version',   'R4',
        'hl7_validation_mode',    'lenient',
        'fhir_validation_mode',   'lenient',
        'default_encoding',       'UTF-8',
        'segment_terminator',     'CR',
        'schema_cache_ttl_minutes', 60
    ),
    'Default HL7 and FHIR processing parameters. Overrides are cached for schema_cache_ttl_minutes.'
) ON CONFLICT (key) DO NOTHING;

-- ── Message Queue & Processing ────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'message_queue',
    jsonb_build_object(
        'max_queue_depth',         10000,
        'dlq_retention_days',      30,
        'message_retention_days',  90,
        'processing_concurrency',  5,
        'batch_size',              100,
        'overflow_action',         'dlq',
        'processing_timeout_ms',   30000
    ),
    'Message queue capacity, retention, and processing concurrency defaults.'
) ON CONFLICT (key) DO NOTHING;

-- ── Connector Defaults ────────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'connector_defaults',
    jsonb_build_object(
        'connection_timeout_ms',   5000,
        'read_timeout_ms',         30000,
        'write_timeout_ms',        30000,
        'pool_size',               10,
        'keep_alive_seconds',      30,
        'max_reconnect_attempts',  5,
        'tls_validation',          'strict'
    ),
    'Global connector timeout and connection-pool defaults applied when a connector has no per-instance override.'
) ON CONFLICT (key) DO NOTHING;

-- ── Alert Defaults ────────────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'alert_defaults',
    jsonb_build_object(
        'error_rate_threshold_pct',  5.0,
        'latency_threshold_ms',      5000,
        'queue_depth_threshold_pct', 80.0,
        'consecutive_failures',      3,
        'cooldown_minutes',          15,
        'history_retention_days',    90,
        'metrics_retention_days',    30,
        'auto_resolve_minutes',      60
    ),
    'Default thresholds pre-filled when creating a new alert rule. Retention policies drive scheduled cleanup.'
) ON CONFLICT (key) DO NOTHING;

-- ── Performance & Tuning ──────────────────────────────────────────────────────
INSERT INTO system_settings (key, value, description) VALUES (
    'performance',
    jsonb_build_object(
        'log_level',                 'info',
        'db_max_open_connections',   25,
        'db_max_idle_connections',   5,
        'db_query_timeout_ms',       5000
    ),
    'Go backend performance tuning. Changes take effect on next Go backend restart.'
) ON CONFLICT (key) DO NOTHING;
