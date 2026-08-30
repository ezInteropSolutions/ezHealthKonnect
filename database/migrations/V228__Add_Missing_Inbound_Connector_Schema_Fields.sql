-- V228: Expose config fields that real inbound connector Go code already reads
-- but that connectivity_types.config_schema never surfaced -- found during a
-- full audit of every real (non-stub) inbound connector's schema against its
-- Go struct's actual field usage, following the same method that caught the
-- http_rest_inbound / azure_blob_inbound schema mismatches earlier.
-- Applied: 2026-08-30

-- ============================================================
-- BACKGROUND
-- ============================================================
-- Unlike the earlier bugs (wrong field NAMES, breaking the connector
-- outright), these are fields that were simply omitted from the schema
-- entirely -- the connector still works via its other fields, but a real,
-- already-implemented capability is unreachable from the UI. Confirmed each
-- field is genuinely read AND consumed (not dead code) before adding it here:
--   - tcp_mllp: max_message_size_mb bounds MLLP frame size (DoS protection),
--     keep_alive/keep_alive_period_seconds apply real TCP keep-alive
--     (tcpConn.SetKeepAlivePeriod). NOTE: validate_checksum is also parsed by
--     Go but is NEVER actually read/used anywhere else in the codebase today
--     (a genuine no-op) -- deliberately NOT added here; exposing a UI field
--     for it would misrepresent a feature that doesn't work yet.
--   - kafka_inbound: topics ([]string) enables multi-topic subscription --
--     unlike brokers (which already supports comma-separated multi-value via
--     resolveBrokers()'s fallback), topic has no such fallback, so multi-topic
--     was completely unreachable without this field.
--   - redis_inbound: channels ([]string) enables multi-channel pub/sub,
--     additive with the existing single channel field (both are merged, not
--     mutually exclusive -- see pubSubLoop's channel-collection logic).
--   - mongodb_inbound: host/port/username/password are a real alternative to
--     a full connection_string (buildURI() falls back to them when
--     connection_string is empty), but had no UI path at all.

-- ============================================================
-- tcp_mllp (inbound TCP/MLLP listener)
-- ============================================================
UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["port"],
        "properties": {
            "host": {"type": "string", "title": "Bind Address", "default": "0.0.0.0", "description": "Note: the current listener always binds all interfaces regardless of this value"},
            "port": {"type": "integer", "title": "Listen Port", "default": 2575, "maximum": 65535, "minimum": 1024},
            "key_file": {"type": "string", "title": "TLS Private Key Path"},
            "password": {"type": "string", "title": "Password", "format": "password", "description": "Used when Authentication Type is basic"},
            "username": {"type": "string", "title": "Username", "description": "Used when Authentication Type is basic"},
            "enable_tls": {"type": "boolean", "title": "Enable TLS/SSL", "default": false},
            "tls_version": {"enum": ["1.2", "1.3"], "type": "string", "title": "Minimum TLS Version", "default": "1.2"},
            "max_connections": {"type": "integer", "title": "Max Connections", "default": 10},
            "certificate_file": {"type": "string", "title": "TLS Certificate Path"},
            "authentication_type": {"enum": ["none", "basic", "token"], "type": "string", "title": "Authentication Type", "default": "none"},
            "read_timeout_seconds": {"type": "integer", "title": "Read Timeout (seconds)", "default": 300},
            "write_timeout_seconds": {"type": "integer", "title": "Write Timeout (seconds)", "default": 300},
            "max_message_size_mb": {"type": "integer", "title": "Max Message Size (MB)", "default": 10, "maximum": 100, "minimum": 1, "description": "Bounds a single MLLP frame -- protects against unbounded memory growth if a sender never sends the end-of-frame marker"},
            "keep_alive": {"type": "boolean", "title": "Enable TCP Keep-Alive", "default": false},
            "keep_alive_period_seconds": {"type": "integer", "title": "Keep-Alive Period (seconds)", "default": 60, "description": "Used when Enable TCP Keep-Alive is on"}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["port", "host"],
        "advanced": ["max_connections", "read_timeout_seconds", "write_timeout_seconds", "max_message_size_mb", "keep_alive", "keep_alive_period_seconds"],
        "security": ["enable_tls", "tls_version", "certificate_file", "key_file", "authentication_type", "username", "password"]
    }'::jsonb
WHERE type_name = 'tcp_mllp';

-- ============================================================
-- kafka_inbound
-- ============================================================
UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["brokers", "topic", "consumer_group"],
        "properties": {
            "topic": {"type": "string", "title": "Topic Name"},
            "topics": {"type": "array", "items": {"type": "string"}, "title": "Additional Topics", "description": "Subscribe to multiple topics -- if set, takes priority over the single Topic Name field above"},
            "brokers": {"type": "string", "title": "Bootstrap Servers", "default": "localhost:9092", "description": "Comma-separated list (e.g., localhost:9092,localhost:9093)"},
            "max_bytes": {"type": "integer", "title": "Max Bytes Per Fetch", "default": 1048576},
            "tls_enabled": {"type": "boolean", "title": "Enable TLS", "default": false},
            "sasl_enabled": {"type": "boolean", "title": "Enable SASL Authentication", "default": false},
            "sasl_password": {"type": "string", "title": "SASL Password", "format": "password"},
            "sasl_username": {"type": "string", "title": "SASL Username"},
            "consumer_group": {"type": "string", "title": "Consumer Group ID"},
            "sasl_mechanism": {"enum": ["PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"], "type": "string", "title": "SASL Mechanism", "default": "PLAIN"},
            "session_timeout": {"type": "integer", "title": "Session Timeout (seconds)", "default": 30},
            "tls_skip_verify": {"type": "boolean", "title": "Skip TLS Certificate Verification", "default": false},
            "auto_offset_reset": {"enum": ["earliest", "latest", "none"], "type": "string", "title": "Auto Offset Reset", "default": "latest"},
            "heartbeat_interval": {"type": "integer", "title": "Heartbeat Interval (seconds)", "default": 3}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["brokers", "topic", "topics", "consumer_group"],
        "consumer": ["auto_offset_reset", "session_timeout", "heartbeat_interval", "max_bytes"],
        "security": ["sasl_enabled", "sasl_mechanism", "sasl_username", "sasl_password", "tls_enabled", "tls_skip_verify"]
    }'::jsonb
WHERE type_name = 'kafka_inbound';

-- ============================================================
-- redis_inbound
-- ============================================================
UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["host", "port"],
        "properties": {
            "host": {"type": "string", "title": "Redis Host", "default": "localhost"},
            "port": {"type": "integer", "title": "Redis Port", "default": 6379},
            "stream": {"type": "string", "title": "Stream Key", "description": "Set this for Stream mode (takes priority over Pub/Sub channel)"},
            "channel": {"type": "string", "title": "Pub/Sub Channel", "description": "Set this for Pub/Sub mode"},
            "channels": {"type": "array", "items": {"type": "string"}, "title": "Additional Pub/Sub Channels", "description": "Subscribe to additional channels alongside Pub/Sub Channel above (both are combined, not mutually exclusive)"},
            "use_tls": {"type": "boolean", "title": "Enable TLS/SSL", "default": false},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "db_number": {"type": "integer", "title": "Database Number", "default": 0, "maximum": 15, "minimum": 0},
            "read_timeout": {"type": "integer", "title": "Block Timeout (ms)", "default": 2000},
            "consumer_name": {"type": "string", "title": "Consumer Name (for streams)"},
            "consumer_group": {"type": "string", "title": "Consumer Group (for streams)", "default": "ehk-consumer-group"},
            "max_pending_ack": {"type": "integer", "title": "Max Messages Per Read (for streams)", "default": 10},
            "tls_skip_verify": {"type": "boolean", "title": "Skip TLS Certificate Verification", "default": false}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["host", "port", "password", "db_number"],
        "advanced": ["read_timeout", "max_pending_ack", "use_tls", "tls_skip_verify"],
        "consumer": ["channel", "channels", "stream", "consumer_group", "consumer_name"]
    }'::jsonb
WHERE type_name = 'redis_inbound';

-- ============================================================
-- mongodb_inbound
-- ============================================================
UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["database", "collection"],
        "properties": {
            "query": {"type": "string", "title": "Query Filter (JSON)", "default": "{}", "description": "MongoDB query filter in JSON format"},
            "database": {"type": "string", "title": "Database Name"},
            "collection": {"type": "string", "title": "Collection Name"},
            "max_records": {"type": "integer", "title": "Max Documents Per Poll", "default": 100},
            "watch_changes": {"type": "boolean", "title": "Use Change Streams (Real-Time)", "default": false, "description": "Requires a MongoDB replica set or Atlas cluster"},
            "connect_timeout": {"type": "integer", "title": "Connect Timeout (seconds)", "default": 10},
            "after_processing": {"enum": ["nothing", "delete", "update_flag"], "type": "string", "title": "After Processing", "default": "nothing"},
            "polling_interval": {"type": "integer", "title": "Polling Interval (seconds)", "default": 60},
            "connection_string": {"type": "string", "title": "MongoDB Connection URI", "default": "mongodb://localhost:27017", "description": "mongodb://user:password@host:port or mongodb+srv://... -- takes priority over Host/Port/Username/Password below when set"},
            "host": {"type": "string", "title": "Host", "default": "localhost", "description": "Alternative to Connection URI -- used only when Connection URI is empty"},
            "port": {"type": "integer", "title": "Port", "default": 27017, "description": "Alternative to Connection URI"},
            "username": {"type": "string", "title": "Username", "description": "Alternative to Connection URI"},
            "password": {"type": "string", "title": "Password", "format": "password", "description": "Alternative to Connection URI"},
            "incremental_field": {"type": "string", "title": "Incremental Field", "description": "Field to track last polled value (e.g., _id, updatedAt)"},
            "operation_timeout": {"type": "integer", "title": "Operation Timeout (seconds)", "default": 30},
            "processed_flag_field": {"type": "string", "title": "Processed Flag Field", "default": "processed"},
            "processed_flag_value": {"type": "string", "title": "Processed Flag Value", "default": "true"}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["connection_string", "database", "collection"],
        "authentication": ["host", "port", "username", "password"],
        "query": ["query", "incremental_field", "max_records", "watch_changes"],
        "advanced": ["polling_interval", "connect_timeout", "operation_timeout"],
        "processing": ["after_processing", "processed_flag_field", "processed_flag_value"]
    }'::jsonb
WHERE type_name = 'mongodb_inbound';
