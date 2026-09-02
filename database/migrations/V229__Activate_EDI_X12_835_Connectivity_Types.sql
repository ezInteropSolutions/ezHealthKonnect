-- V229: Fix edi_x12_inbound/edi_x12_outbound's config_schema/parameter_groups to
-- match the real Go connectors (services/connectors/edi_x12_inbound.go,
-- edi_x12_outbound.go), and re-activate them now that they have genuine
-- implementations (835 phase 1 — SFTP transport only).
-- Applied: 2026-09-01

-- ============================================================
-- BACKGROUND
-- ============================================================
-- edi_x12_inbound/edi_x12_outbound were hidden (is_active=false) by V221
-- while they were still stubs. They now have real implementations (this
-- session), but their stored config_schema/parameter_groups still reflect a
-- never-implemented design that predates the real connectors — same class
-- of UI/backend field-name mismatch documented in V222/V224/V225 for other
-- inbound connectors:
--
-- edi_x12_inbound:
--   generate_999_ack   -> removed   (Go has no 999-ack generation yet —
--                                     named Phase 2 in CLAUDE.md's EDI
--                                     section; showing this toggle today
--                                     would silently do nothing)
--   Newly added (existed in Go, missing from schema entirely):
--     key_content, auth_type, file_pattern, after_processing, archive_dir,
--     max_files_per_run, connect_timeout, read_timeout
--   transaction_types default ["837P"] -> ["835"] (837 isn't implemented
--     yet; phase 1 only supports 835)
--   transport enum ["sftp","http","as2"] -> ["sftp"] only — http/as2 are
--     named future phases, not implemented; don't offer options in the UI
--     that return a runtime error (Validate() explicitly rejects them)
--
-- edi_x12_outbound:
--   isa_sender_id / isa_receiver_id -> removed from THIS connector's own
--     schema. These are edi.build pipeline-step config (isaSenderId/
--     isaReceiverId), not connector config — the values are embedded IN
--     the built document content by edi.build, not read by this
--     transport-only "dumb byte shipper" connector at all. Leaving them
--     here would mislead a user into thinking configuring them on the
--     connector affects anything, when it doesn't — the same "field the
--     UI shows but the Go code silently ignores" problem V222/V224/V225
--     already fixed elsewhere.
--   Newly added (existed in Go, missing from schema entirely):
--     key_content, auth_type, filename_pattern
--   transport enum narrowed to ["sftp"] only, same rationale as inbound.
--
-- Zero real usage exists for either type (both were inactive stubs until
-- now), so this is a safe full-JSON overwrite, matching V225's own
-- precedent for the same situation.

UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["transport", "host", "username"],
        "properties": {
            "transport": {"type": "string", "title": "Transport", "enum": ["sftp"], "default": "sftp", "description": "Only SFTP is implemented in phase 1 — HTTP/AS2 are planned future transports"},
            "host": {"type": "string", "title": "Host"},
            "port": {"type": "number", "title": "Port", "default": 22},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "auth_type": {"type": "string", "title": "Auth Type", "enum": ["password", "key"], "default": "password"},
            "key_content": {"type": "string", "title": "Private Key (PEM)", "format": "password", "description": "Used when Auth Type is key"},
            "remote_path": {"type": "string", "title": "Remote Path", "default": "/incoming"},
            "file_pattern": {"type": "string", "title": "File Pattern", "default": "*.edi"},
            "transaction_types": {"type": "array", "title": "Transaction Types", "description": "Accepted but not yet enforced at the connector level in phase 1 (only 835 is supported end-to-end)", "default": ["835"]},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 300},
            "after_processing": {"type": "string", "title": "After Processing", "enum": ["archive", "delete", "none"], "default": "archive"},
            "archive_dir": {"type": "string", "title": "Archive Directory", "description": "Default: <remote_path>/processed"},
            "max_files_per_run": {"type": "number", "title": "Max Files Per Poll", "default": 100},
            "connect_timeout": {"type": "number", "title": "Connect Timeout (seconds)", "default": 10},
            "read_timeout": {"type": "number", "title": "Read Timeout (seconds)", "default": 60}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["transport", "remote_path", "file_pattern", "transaction_types", "polling_interval_seconds"],
        "security": ["host", "port", "username", "password", "auth_type", "key_content"],
        "processing": ["after_processing", "archive_dir", "max_files_per_run", "connect_timeout", "read_timeout"]
    }'::jsonb,
    is_active = true,
    updated_at = NOW()
WHERE type_name = 'edi_x12_inbound';

UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["transport", "host", "username"],
        "properties": {
            "transport": {"type": "string", "title": "Transport", "enum": ["sftp"], "default": "sftp", "description": "Only SFTP is implemented in phase 1 — HTTP/AS2 are planned future transports"},
            "host": {"type": "string", "title": "Host"},
            "port": {"type": "number", "title": "Port", "default": 22},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "auth_type": {"type": "string", "title": "Auth Type", "enum": ["password", "key"], "default": "password"},
            "key_content": {"type": "string", "title": "Private Key (PEM)", "format": "password", "description": "Used when Auth Type is key"},
            "remote_path": {"type": "string", "title": "Remote Path", "default": "/outgoing"},
            "filename_pattern": {"type": "string", "title": "Filename Pattern", "description": "Placeholders: {message_id} {interface_id} {timestamp} {date} {time}. Default: a timestamp+message-id name with a .edi extension"},
            "connect_timeout": {"type": "number", "title": "Connect Timeout (seconds)", "default": 10},
            "timeout_seconds": {"type": "number", "title": "Upload Timeout (seconds)", "default": 60}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["transport", "remote_path", "filename_pattern"],
        "security": ["host", "port", "username", "password", "auth_type", "key_content"],
        "advanced": ["connect_timeout", "timeout_seconds"]
    }'::jsonb,
    is_active = true,
    updated_at = NOW()
WHERE type_name = 'edi_x12_outbound';
