-- V246: Add connectivity_types rows for as2_inbound/as2_outbound.
-- Applied: 2026-09-12

-- ============================================================
-- BACKGROUND
-- ============================================================
-- Phase 4 (AS2 transport) — a NEW connector type, not a widened `transport`
-- enum on edi_x12_inbound/edi_x12_outbound (those stay SFTP-only
-- permanently; see services/connectors/as2_inbound.go's own header comment
-- for why AS2 needed its own connector shape: a persistent HTTPS listener +
-- S/MIME sign/verify/encrypt/decrypt + synchronous MDN generation, none of
-- which the SFTP-poller connectors do at all).
--
-- Scope of this pass: synchronous MDN only (RFC 4130's mode where the MDN
-- is the same HTTP response) — async MDN (partner posts the MDN back later
-- to a separate URL) is a named, deferred item. CMS-wrapped signing
-- ("application/pkcs7-mime; smime-type=signed-data", content attached) is
-- used rather than RFC 4130's alternative multipart/signed MIME-boundary
-- form — both are valid AS2 message shapes; multipart/signed is a named,
-- not-yet-built alternative if a specific real trading partner requires it.
--
-- Config field names match services/connectors/as2_inbound.go/as2_outbound.go
-- exactly (own_cert_pem/own_key_pem/partner_cert_pem, port/base_path,
-- partner_url, as2_from/as2_to, request_mdn) — confirmed by reading those
-- files directly, not guessed.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'as2_inbound',
    'inbound',
    'AS2 Receiver',
    'Receive signed+encrypted EDI documents over HTTPS (AS2/RFC 4130), returning a signed synchronous MDN receipt',
    '🔐',
    'push',
    false,
    true,
    false,
    'AS2InboundConnector',
    '{
        "type": "object",
        "required": ["port", "as2_to", "own_cert_pem", "own_key_pem", "partner_cert_pem"],
        "properties": {
            "port": {"type": "integer", "title": "Listener Port"},
            "base_path": {"type": "string", "title": "URL Path", "default": "/as2"},
            "as2_from": {"type": "string", "title": "Expected Partner AS2 ID", "description": "Used for logging/MDN fields — the actual trust decision is the certificate, not this header value"},
            "as2_to": {"type": "string", "title": "Own AS2 ID (This Station)"},
            "own_cert_pem": {"type": "string", "title": "Own Certificate (PEM)", "description": "Used to sign outgoing MDNs and as the encryption recipient"},
            "own_key_pem": {"type": "string", "title": "Own Private Key (PEM)", "format": "password", "description": "Decrypts incoming messages"},
            "partner_cert_pem": {"type": "string", "title": "Partner Certificate (PEM)", "description": "Verifies the partner signature on incoming messages"},
            "tls_enabled": {"type": "boolean", "title": "Enable TLS (HTTPS)", "default": false, "description": "Real partner traffic should enable this"},
            "tls_cert_file": {"type": "string", "title": "TLS Certificate File Path"},
            "tls_key_file": {"type": "string", "title": "TLS Key File Path"},
            "request_timeout_seconds": {"type": "integer", "title": "Request Timeout (seconds)", "default": 30}
        }
    }'::jsonb,
    '{
        "basic": ["port", "base_path", "as2_from", "as2_to"],
        "security": ["own_cert_pem", "own_key_pem", "partner_cert_pem"],
        "tls": ["tls_enabled", "tls_cert_file", "tls_key_file"],
        "advanced": ["request_timeout_seconds"]
    }'::jsonb,
    true,
    60,
    '1.0',
    'Payers & Revenue Cycle',
    52
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'as2_outbound',
    'outbound',
    'AS2 Sender',
    'Sign+encrypt an EDI document as S/MIME (AS2/RFC 4130) and deliver it to a trading partner over HTTPS, verifying their signed synchronous MDN receipt',
    '🔐',
    'push',
    false,
    true,
    false,
    'AS2OutboundConnector',
    '{
        "type": "object",
        "required": ["partner_url", "as2_from", "as2_to", "own_cert_pem", "own_key_pem", "partner_cert_pem"],
        "properties": {
            "partner_url": {"type": "string", "title": "Partner AS2 Endpoint URL"},
            "as2_from": {"type": "string", "title": "Own AS2 ID (This Station)"},
            "as2_to": {"type": "string", "title": "Partner AS2 ID"},
            "own_cert_pem": {"type": "string", "title": "Own Certificate (PEM)", "description": "Used to sign outgoing messages"},
            "own_key_pem": {"type": "string", "title": "Own Private Key (PEM)", "format": "password", "description": "Signs outgoing messages"},
            "partner_cert_pem": {"type": "string", "title": "Partner Certificate (PEM)", "description": "Encrypts outgoing messages and verifies the partner MDN"},
            "request_mdn": {"type": "boolean", "title": "Request Signed MDN", "default": true, "description": "Synchronous MDN only in this phase"},
            "timeout_seconds": {"type": "integer", "title": "Request Timeout (seconds)", "default": 60}
        }
    }'::jsonb,
    '{
        "basic": ["partner_url", "as2_from", "as2_to", "request_mdn"],
        "security": ["own_cert_pem", "own_key_pem", "partner_cert_pem"],
        "advanced": ["timeout_seconds"]
    }'::jsonb,
    true,
    60,
    '1.0',
    'Payers & Revenue Cycle',
    53
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();
