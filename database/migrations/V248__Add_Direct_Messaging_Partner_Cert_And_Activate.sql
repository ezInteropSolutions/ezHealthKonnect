-- V248: Add partner_cert_pem to direct_messaging_inbound/outbound and activate both.
-- Applied: 2026-09-13

-- ============================================================
-- BACKGROUND
-- ============================================================
-- direct_messaging_inbound/outbound have existed as inactive stubs since
-- V69/V221 — their own config_schema already correctly committed to the
-- right shape (inbound: mode "pull"/supports_cron true, an IMAP poller, not
-- an inbound SMTP server; outbound: mode "push", an SMTP sender) but NEITHER
-- schema had a field for the OTHER PARTY's own certificate: inbound needs
-- the sender's cert to verify their signature, outbound needs the
-- recipient's cert to encrypt to them. This is the same real, pre-existing
-- gap AS2's own as2_inbound/as2_outbound schemas (V246) already solved with
-- their own "partner_cert_pem" field — this migration adds the identical
-- field name here for consistency, since services/connectors/
-- direct_messaging_inbound.go/outbound.go reuse the exact same S/MIME
-- sign+encrypt/decrypt+verify primitives AS2 does (smime_shared.go, renamed
-- from as2_smime.go once this second connector started depending on it).
--
-- Trust model: manually-configured partner cert only, same as AS2 — no DNS
-- CERT-record (RFC 4398) or LDAP auto-discovery exists anywhere in this
-- codebase, and building either is a genuinely separate subsystem, not
-- attempted in this phase.
--
-- Scope: MDN-over-email (DirectTrust delivery/read receipts) is explicitly
-- deferred — this phase builds the core clinical-document exchange only.
--
-- Config field names match services/connectors/direct_messaging_inbound.go/
-- direct_messaging_outbound.go exactly (imap_host/imap_port/smtp_host/
-- smtp_port/username/password/cert_content/private_key/partner_cert_pem/
-- polling_interval_seconds/recipient_address) — confirmed by reading those
-- files directly, not guessed. implementation_class filled in (previously
-- blank on both rows) matching AS2's own precedent.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'direct_messaging_inbound',
    'inbound',
    'Direct Messaging Inbound',
    'Receive clinical documents via Direct Trust (DirectTrust) protocol — S/MIME-encrypted email polled over IMAP, decrypted and signature-verified against the configured partner certificate.',
    '📧',
    'pull',
    true,
    true,
    false,
    'DirectMessagingInboundConnector',
    '{
        "type": "object",
        "required": ["imap_host", "username", "password", "cert_content", "private_key", "partner_cert_pem"],
        "properties": {
            "imap_host": {"type": "string", "title": "IMAP Host"},
            "imap_port": {"type": "number", "title": "IMAP Port", "default": 993},
            "use_tls": {"type": "boolean", "title": "Use Implicit TLS (IMAPS)", "default": true, "description": "Disable only for a plain-text test mailbox — real Direct Trust traffic always uses TLS"},
            "username": {"type": "string", "title": "Direct Address", "description": "e.g. provider@direct.hospital.org"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "cert_content": {"type": "string", "title": "Own S/MIME Certificate (PEM)", "format": "password", "description": "Used as the encryption recipient for incoming messages"},
            "private_key": {"type": "string", "title": "Own Private Key (PEM)", "format": "password", "description": "Decrypts incoming messages"},
            "partner_cert_pem": {"type": "string", "title": "Partner Certificate (PEM)", "description": "Verifies the sender signature on incoming messages — direct-trust model, same as AS2"},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60}
        }
    }'::jsonb,
    '{
        "basic": ["imap_host", "imap_port", "use_tls", "username", "polling_interval_seconds"],
        "security": ["password", "cert_content", "private_key", "partner_cert_pem"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Protocols',
    25
)
ON CONFLICT (type_name) DO UPDATE SET
    implementation_class = EXCLUDED.implementation_class,
    config_schema         = EXCLUDED.config_schema,
    parameter_groups      = EXCLUDED.parameter_groups,
    is_active             = EXCLUDED.is_active,
    updated_at            = NOW();

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'direct_messaging_outbound',
    'outbound',
    'Direct Messaging Outbound',
    'Send clinical documents via Direct Trust (DirectTrust) protocol — S/MIME sign+encrypt then deliver over SMTP, encrypted for the configured partner certificate.',
    '📧',
    'push',
    false,
    true,
    false,
    'DirectMessagingOutboundConnector',
    '{
        "type": "object",
        "required": ["smtp_host", "username", "password", "cert_content", "private_key", "partner_cert_pem", "recipient_address"],
        "properties": {
            "smtp_host": {"type": "string", "title": "SMTP Host"},
            "smtp_port": {"type": "number", "title": "SMTP Port", "default": 587},
            "use_tls": {"type": "boolean", "title": "Use STARTTLS", "default": true, "description": "Disable only for a plain-text test server — real Direct Trust traffic always uses TLS"},
            "username": {"type": "string", "title": "Sender Direct Address"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "cert_content": {"type": "string", "title": "Own S/MIME Certificate (PEM)", "format": "password", "description": "Used to sign outgoing messages"},
            "private_key": {"type": "string", "title": "Own Private Key (PEM)", "format": "password", "description": "Signs outgoing messages"},
            "partner_cert_pem": {"type": "string", "title": "Partner Certificate (PEM)", "description": "Encrypts outgoing messages to the recipient — direct-trust model, same as AS2"},
            "recipient_address": {"type": "string", "title": "Default Recipient Direct Address"}
        }
    }'::jsonb,
    '{
        "basic": ["smtp_host", "smtp_port", "use_tls", "username", "recipient_address"],
        "security": ["password", "cert_content", "private_key", "partner_cert_pem"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Protocols',
    26
)
ON CONFLICT (type_name) DO UPDATE SET
    implementation_class = EXCLUDED.implementation_class,
    config_schema         = EXCLUDED.config_schema,
    parameter_groups      = EXCLUDED.parameter_groups,
    is_active             = EXCLUDED.is_active,
    updated_at            = NOW();
