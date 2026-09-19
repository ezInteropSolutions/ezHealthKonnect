-- V249: Add connectivity_types rows for websocket_inbound/websocket_outbound.
-- Applied: 2026-09-13

-- ============================================================
-- BACKGROUND
-- ============================================================
-- A user asked whether HTTP/TCP/WebSocket outbound senders let a downstream
-- pipeline step read the response. WebSocket didn't exist as a connector at
-- all before this migration (confirmed via full grep — only a dead
-- ProtocolWebSocket enum value with zero wiring existed). This adds both
-- directions:
--   websocket_inbound  — a real-time server accepting websocket connections,
--                         enqueuing each received frame as a normal message.
--   websocket_outbound — a client that dials a remote websocket server,
--                         sends a frame, and (by default) reads back a
--                         response frame over the same connection, surfaced
--                         to a downstream pipeline step via
--                         steps.<alias>.step_output.response_body (text) or
--                         .response_binary_base64 (binary — never silently
--                         dropped).
--
-- Config field names match services/connectors/websocket_inbound.go and
-- websocket_outbound.go exactly — confirmed by reading those files directly,
-- not guessed.
--
-- Named, deferred scope (see websocket_inbound.go's own file header for the
-- full reasoning): no pipeline-driven synchronous reply frame is sent back to
-- an inbound client after a message is enqueued — that would require
-- blocking the connection's goroutine on full async pipeline completion, a
-- genuinely new architecture no existing connector in this codebase has.
--
-- mode='push' (not the untested-but-legal 'stream' enum value) — matches
-- every other persistent-connection connector shipped so far (tcp_mllp_*,
-- as2_inbound). ui_category='Healthcare Network' matches the real DB value
-- already used by tcp_mllp_*/http_* (V69__EMR_Connectivity_Types.sql) — the
-- true peer group for a generic transport, not AS2's domain-specific
-- "Payers & Revenue Cycle".

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'websocket_inbound',
    'inbound',
    'WebSocket Server',
    'Accept real-time websocket connections from external systems and enqueue each received message for processing',
    '🔌',
    'push',
    false,
    true,
    false,
    'WebSocketInboundConnector',
    '{
        "type": "object",
        "required": ["port"],
        "properties": {
            "port": {"type": "integer", "title": "Listener Port"},
            "base_path": {"type": "string", "title": "URL Path", "default": "/ws"},
            "tls_enabled": {"type": "boolean", "title": "Enable TLS (wss://)", "default": false},
            "tls_cert_file": {"type": "string", "title": "TLS Certificate File Path"},
            "tls_key_file": {"type": "string", "title": "TLS Key File Path"},
            "max_message_size_mb": {"type": "integer", "title": "Max Message Size (MB)", "default": 10, "description": "Hard cap 100 MB"},
            "read_timeout_seconds": {"type": "integer", "title": "Read Timeout (seconds)", "default": 60},
            "write_timeout_seconds": {"type": "integer", "title": "Write Timeout (seconds)", "default": 10},
            "ping_interval_seconds": {"type": "integer", "title": "Ping Interval (seconds)", "default": 30, "description": "Keepalive ping sent to each connected client"},
            "max_connections": {"type": "integer", "title": "Max Concurrent Connections", "default": 100},
            "authentication_type": {"type": "string", "title": "Authentication", "enum": ["none", "bearer", "basic"], "default": "none"},
            "bearer_token": {"type": "string", "title": "Expected Bearer Token", "format": "password"},
            "username": {"type": "string", "title": "Expected Username (basic auth)"},
            "password": {"type": "string", "title": "Expected Password (basic auth)", "format": "password"}
        }
    }'::jsonb,
    '{
        "basic": ["port", "base_path", "max_connections"],
        "tls": ["tls_enabled", "tls_cert_file", "tls_key_file"],
        "security": ["authentication_type", "bearer_token", "username", "password"],
        "advanced": ["max_message_size_mb", "read_timeout_seconds", "write_timeout_seconds", "ping_interval_seconds"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    15
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
    'websocket_outbound',
    'outbound',
    'WebSocket Client',
    'Send a message to a remote websocket server and read back its response over the same connection for use by later pipeline steps',
    '🔌',
    'push',
    false,
    false,
    false,
    'WebSocketOutboundConnector',
    '{
        "type": "object",
        "required": ["url"],
        "properties": {
            "url": {"type": "string", "title": "WebSocket URL", "description": "ws:// or wss://"},
            "sub_protocol": {"type": "string", "title": "Sub-Protocol"},
            "headers": {"type": "object", "title": "Handshake Headers", "description": "Custom headers sent during the websocket handshake (e.g. Authorization)"},
            "connection_mode": {"type": "string", "title": "Connection Mode", "enum": ["persistent", "per-message"], "default": "persistent"},
            "connect_timeout_seconds": {"type": "integer", "title": "Connect Timeout (seconds)", "default": 10},
            "write_timeout_seconds": {"type": "integer", "title": "Write Timeout (seconds)", "default": 10},
            "wait_for_response": {"type": "boolean", "title": "Wait For Response", "default": true, "description": "Read one response frame back over the same connection after sending"},
            "response_timeout_seconds": {"type": "integer", "title": "Response Timeout (seconds)", "default": 10, "description": "A timeout here is NOT treated as a delivery failure — the message was already sent"},
            "tls_skip_verify": {"type": "boolean", "title": "Skip TLS Certificate Verification", "default": false, "description": "For wss:// with self-signed certs in dev/test only"},
            "max_retries": {"type": "integer", "title": "Max Retries", "default": 0},
            "retry_delay_ms": {"type": "integer", "title": "Retry Delay (ms)", "default": 500}
        }
    }'::jsonb,
    '{
        "basic": ["url", "sub_protocol", "connection_mode"],
        "response": ["wait_for_response", "response_timeout_seconds"],
        "security": ["headers", "tls_skip_verify"],
        "advanced": ["connect_timeout_seconds", "write_timeout_seconds", "max_retries", "retry_delay_ms"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Network',
    16
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();
