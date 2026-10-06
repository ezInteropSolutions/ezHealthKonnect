-- V278: Expose max_instance_size_mb and tls_ca_file on dicom_storage_inbound's
-- config_schema -- both were added to services/connectors/dicom_storage_inbound.go
-- and dicom/receiver.go during a 360 QA/production-readiness pass (October 2026)
-- that found two real, previously-named-but-unaddressed gaps:
--
--   max_instance_size_mb -- no configurable bound existed on one C-STORE's total
--     reassembled dataset size (beyond go-dicom's own hard, non-configurable
--     128/256 MiB PDU ceilings) -- a misbehaving or malicious sender had nothing
--     stopping it from filling storage/overloading the downstream pipeline.
--     Defaults to 100MB when unset (not unlimited), clamped at 1024MB.
--
--   tls_ca_file -- real mutual TLS, previously believed impossible with this
--     library (go-dicom's own TLSConfig.CAFile convenience field is declared but
--     never read) -- found buildable via the library's OWN documented escape
--     hatch (TLSConfig.Config *tls.Config) once actually read directly. Optional;
--     only meaningful alongside tls_enabled.
--
-- Full config_schema/parameter_groups overwrite, matching the real Go struct and
-- V277's own exact field set plus these two additions -- the same safe,
-- documented "full overwrite matching the real Go structs" pattern V222/V224/V225
-- already established in this codebase, confirmed by reading
-- dicom_storage_inbound.go's own Initialize() directly before writing this.

UPDATE connectivity_types
SET config_schema = '{
        "type": "object",
        "required": ["ae_title", "port"],
        "properties": {
            "ae_title": {"type": "string", "title": "This SCP''s AE Title", "description": "The DICOM Application Entity title this server answers to. Enter this exact value into the modality''s own destination/AE configuration."},
            "port": {"type": "integer", "title": "Listener Port", "default": 11112},
            "bind_address": {"type": "string", "title": "Bind Address", "default": "0.0.0.0"},
            "max_associations": {"type": "integer", "title": "Max Concurrent Associations", "default": 0, "description": "0 = unlimited"},
            "allowed_calling_ae_titles": {"type": "array", "items": {"type": "string"}, "title": "Allowed Calling AE Titles", "description": "Leave empty to accept any calling AE title (not recommended for internet-reachable deployments)"},
            "additional_abstract_syntaxes": {"type": "array", "items": {"type": "string"}, "title": "Additional Storage SOP Class UIDs", "description": "Extra SOP Class UIDs to accept beyond the built-in CR/Digital X-Ray/Secondary Capture defaults"},
            "max_instance_size_mb": {"type": "integer", "title": "Max Instance Size (MB)", "default": 100, "description": "Rejects a received image larger than this. 0 = use the default (100MB), not unlimited. Capped at 1024MB. A real CR/DX image is commonly 8-25MB -- this bounds a misbehaving or malicious sender, not normal traffic."},
            "tls_enabled": {"type": "boolean", "title": "Enable TLS", "default": false},
            "tls_cert_file": {"type": "string", "title": "TLS Certificate File Path"},
            "tls_key_file": {"type": "string", "title": "TLS Private Key File Path"},
            "tls_ca_file": {"type": "string", "title": "TLS Client CA File Path (enables mutual TLS)", "description": "Optional. When set alongside Enable TLS, the modality must present a certificate signed by this CA or the connection is refused -- real mutual TLS, not just server-side encryption."}
        }
    }'::jsonb,
    parameter_groups = '{
        "basic": ["ae_title", "port", "bind_address"],
        "advanced": ["max_associations", "allowed_calling_ae_titles", "additional_abstract_syntaxes", "max_instance_size_mb", "tls_enabled", "tls_cert_file", "tls_key_file", "tls_ca_file"]
    }'::jsonb,
    updated_at = NOW()
WHERE type_name = 'dicom_storage_inbound';
