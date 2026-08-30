-- V225: Fix azure_blob_inbound's config_schema/parameter_groups to match the real
-- Go connector (services/connectors/azure_blob_inbound.go), and re-activate it now
-- that it has a genuine implementation.
-- Applied: 2026-08-30

-- ============================================================
-- BACKGROUND
-- ============================================================
-- azure_blob_inbound was hidden (is_active=false) by V221 while it was still a
-- stub. It now has a real implementation (this session), but its stored
-- config_schema/parameter_groups still reflect a never-implemented design that
-- predates the real connector -- same class of UI/backend field-name mismatch
-- documented in V222/V224 for other inbound connectors:
--   container_name       -> container            (Go: AzureBlobInboundConfig.Container)
--   blob_pattern          -> file_pattern         (Go: FilePattern)
--   max_blobs_per_poll    -> max_blobs            (Go: MaxBlobs)
--   archive_container     -> archive_prefix       (Go archives via a *prefix* inside
--                                                   the SAME container, not a second
--                                                   container -- a semantic mismatch,
--                                                   not just a rename)
--   enable_https          -> removed              (Go has no HTTPS toggle at all --
--                                                   scheme is fixed by the endpoint
--                                                   it builds/is given)
--   after_processing enum "delete|move|tag|nothing" -> "nothing|delete|archive"
--                                                       (Go never implemented "move"/"tag")
--   required: [account_name, container_name]     -> required: [container]
--                                                   (auth is connection_string OR
--                                                   account_name+account_key, same
--                                                   soft-required convention already
--                                                   used by azure_blob_outbound/aws_s3_inbound)
-- Newly added (existed in Go, missing from schema entirely):
--   endpoint, polling_interval, max_file_size_mb
--
-- Zero real usage exists for this type (it was never selectable while inactive),
-- so this is a safe full-JSON overwrite, matching the aws_s3_inbound /
-- azure_blob_outbound sibling schemas' naming and grouping conventions exactly.

UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "required": ["container"],
        "properties": {
            "container": {"type": "string", "title": "Container Name"},
            "prefix": {"type": "string", "title": "Blob Prefix/Folder"},
            "file_pattern": {"type": "string", "title": "File Pattern", "default": "*.hl7"},
            "endpoint": {"type": "string", "title": "Custom Endpoint", "description": "Override the default service URL, e.g. for Azurite or a non-public-cloud endpoint"},
            "max_blobs": {"type": "integer", "title": "Max Blobs Per Poll", "default": 100},
            "connection_string": {"type": "string", "title": "Connection String", "format": "password", "description": "Full Azure Storage connection string -- alternative to account_name + account_key"},
            "account_name": {"type": "string", "title": "Storage Account Name"},
            "account_key": {"type": "string", "title": "Account Key", "format": "password"},
            "after_processing": {"enum": ["nothing", "delete", "archive"], "type": "string", "title": "After Processing", "default": "nothing"},
            "archive_prefix": {"type": "string", "title": "Archive Prefix", "default": "processed/", "description": "Used when After Processing is archive"},
            "polling_interval": {"type": "integer", "title": "Polling Interval (seconds)", "default": 60},
            "max_file_size_mb": {"type": "integer", "title": "Max File Size (MB)", "default": 50}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["container", "prefix", "file_pattern"],
        "advanced": ["endpoint"],
        "processing": ["after_processing", "archive_prefix", "polling_interval", "max_blobs", "max_file_size_mb"],
        "authentication": ["connection_string", "account_name", "account_key"]
    }'::jsonb,
    is_active = true
WHERE type_name = 'azure_blob_inbound';

COMMENT ON COLUMN connectivity_types.config_schema IS 'JSON Schema driving ConnectorConfigBuilder.js. Must match the Go connector''s config struct field names exactly -- see V222/V224/V225 for prior corrections of this drift.';
