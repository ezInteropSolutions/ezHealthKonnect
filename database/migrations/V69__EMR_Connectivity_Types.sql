-- V69: EMR Connectivity Types
-- 1. Adds ui_category + ui_sort_order to connectivity_types for grouped dropdown
-- 2. Categorizes all existing connector types
-- 3. Inserts new EMR/healthcare protocol connector stubs

-- ── Step 1: Add UI grouping columns ─────────────────────────────────────────
ALTER TABLE connectivity_types
    ADD COLUMN IF NOT EXISTS ui_category VARCHAR(100),
    ADD COLUMN IF NOT EXISTS ui_sort_order INTEGER NOT NULL DEFAULT 100;

CREATE INDEX IF NOT EXISTS idx_conn_types_ui_category ON connectivity_types(ui_category);

-- ── Step 2: Categorize existing connector types ──────────────────────────────
UPDATE connectivity_types SET ui_category = 'Healthcare Network', ui_sort_order = 10
WHERE type_name IN ('tcp_mllp_inbound','tcp_mllp_outbound','http_rest_inbound','http_outbound',
                    'tcp_mllp','http_rest');

UPDATE connectivity_types SET ui_category = 'Databases', ui_sort_order = 30
WHERE type_name IN ('postgresql_inbound','postgresql_outbound',
                    'mysql_inbound','mysql_outbound',
                    'sqlserver_inbound','sqlserver_outbound',
                    'oracle_inbound','oracle_outbound');

UPDATE connectivity_types SET ui_category = 'Data Warehouses', ui_sort_order = 40
WHERE type_name IN ('snowflake_inbound','snowflake_outbound',
                    'databricks_inbound','databricks_outbound',
                    'bigquery_inbound','bigquery_outbound',
                    'redshift_inbound','redshift_outbound',
                    'synapse_inbound','synapse_outbound',
                    'clickhouse_inbound','clickhouse_outbound',
                    'timescaledb_inbound','timescaledb_outbound');

UPDATE connectivity_types SET ui_category = 'Message Queues', ui_sort_order = 50
WHERE type_name IN ('kafka_inbound','kafka_outbound',
                    'rabbitmq_inbound','rabbitmq_outbound',
                    'redis_inbound','redis_outbound');

UPDATE connectivity_types SET ui_category = 'Cloud Storage', ui_sort_order = 60
WHERE type_name IN ('aws_s3_inbound','aws_s3_outbound',
                    'azure_blob_inbound','azure_blob_outbound',
                    'gcs_inbound','gcs_outbound');

UPDATE connectivity_types SET ui_category = 'File Transfer', ui_sort_order = 70
WHERE type_name IN ('sftp_inbound','sftp_outbound',
                    'ftp_inbound','ftp_outbound',
                    'file_listener','file_writer');

UPDATE connectivity_types SET ui_category = 'NoSQL Databases', ui_sort_order = 35
WHERE type_name IN ('mongodb_inbound','mongodb_outbound');

-- Any remaining uncategorized connectors fall into 'Other'
UPDATE connectivity_types SET ui_category = 'Other', ui_sort_order = 200
WHERE ui_category IS NULL;

-- ── Step 3: Insert new EMR / Healthcare Protocol connector stubs ─────────────
-- These are DB-registered stubs. Go implementations added in later sprints.

-- Generic FHIR R4 Inbound (polling + SMART on FHIR auth)
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'fhir_r4_inbound', 'FHIR R4 Inbound', 'inbound', 'pull',
    'Poll a FHIR R4 server for resource updates using incremental timestamps. Supports SMART on FHIR OAuth2 (client_credentials), Bearer Token, and Basic auth.',
    '🌐',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "FHIR Base URL", "description": "e.g. https://fhir.hospital.org/R4"},
            "resource_types": {"type": "array", "title": "Resource Types", "description": "e.g. Patient, Observation, Encounter", "default": ["Patient"]},
            "auth_type": {"type": "string", "title": "Auth Type", "enum": ["smart_on_fhir", "bearer_token", "basic", "none"], "default": "bearer_token"},
            "client_id": {"type": "string", "title": "Client ID"},
            "client_secret": {"type": "string", "title": "Client Secret", "format": "password"},
            "token_url": {"type": "string", "title": "Token URL", "description": "OAuth2 token endpoint (auto-discovered for SMART)"},
            "scope": {"type": "string", "title": "OAuth2 Scope", "default": "system/*.read"},
            "bearer_token": {"type": "string", "title": "Bearer Token", "format": "password"},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "fhir_version": {"type": "string", "title": "FHIR Version", "enum": ["R4", "STU3"], "default": "R4"},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60},
            "enable_rate_limiting": {"type": "boolean", "title": "Enable Rate Limiting", "default": true},
            "rate_limit_per_minute": {"type": "number", "title": "Max Requests / Minute", "default": 100},
            "enable_tls_verify": {"type": "boolean", "title": "Verify TLS Certificate", "default": true}
        },
        "required": ["fhir_base_url"]
    }',
    '{"basic": ["fhir_base_url", "resource_types", "fhir_version", "polling_interval_seconds"], "security": ["auth_type", "client_id", "client_secret", "token_url", "scope", "bearer_token", "username", "password", "enable_tls_verify"], "advanced": ["enable_rate_limiting", "rate_limit_per_minute"]}',
    true, true, false, true,
    'Healthcare Protocols', 20
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'fhir_r4_inbound');

-- Generic FHIR R4 Outbound (POST/PUT FHIR resources)
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'fhir_r4_outbound', 'FHIR R4 Outbound', 'outbound', 'push',
    'POST or PUT FHIR R4 resources/bundles to a FHIR server. Supports SMART on FHIR, Bearer Token, and Basic auth.',
    '🌐',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "FHIR Base URL", "description": "e.g. https://fhir.hospital.org/R4"},
            "resource_type": {"type": "string", "title": "Resource Type", "description": "e.g. Bundle, Patient (leave blank to auto-detect)"},
            "auth_type": {"type": "string", "title": "Auth Type", "enum": ["smart_on_fhir", "bearer_token", "basic", "none"], "default": "bearer_token"},
            "client_id": {"type": "string", "title": "Client ID"},
            "client_secret": {"type": "string", "title": "Client Secret", "format": "password"},
            "token_url": {"type": "string", "title": "Token URL"},
            "scope": {"type": "string", "title": "OAuth2 Scope", "default": "system/*.write"},
            "bearer_token": {"type": "string", "title": "Bearer Token", "format": "password"},
            "timeout_seconds": {"type": "number", "title": "Timeout (seconds)", "default": 30},
            "retry_count": {"type": "number", "title": "Retry Count", "default": 3},
            "enable_tls_verify": {"type": "boolean", "title": "Verify TLS Certificate", "default": true}
        },
        "required": ["fhir_base_url"]
    }',
    '{"basic": ["fhir_base_url", "resource_type", "timeout_seconds", "retry_count"], "security": ["auth_type", "client_id", "client_secret", "token_url", "scope", "bearer_token", "enable_tls_verify"]}',
    false, true, false, true,
    'Healthcare Protocols', 21
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'fhir_r4_outbound');

-- Epic FHIR R4 Inbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'epic_fhir_inbound', 'Epic FHIR R4 Inbound', 'inbound', 'pull',
    'Receive FHIR R4 resources from Epic Systems using SMART on FHIR OAuth2. Supports Epic sandbox and production environments.',
    '🏥',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "Epic FHIR Base URL", "description": "e.g. https://fhir.epic.com/interconnect-ambu-oauth/api/FHIR/R4"},
            "epic_environment": {"type": "string", "title": "Environment", "enum": ["production", "sandbox", "non_prod"], "default": "sandbox"},
            "client_id": {"type": "string", "title": "Epic Client ID (Non-Production)"},
            "resource_types": {"type": "array", "title": "Resource Types", "default": ["Patient", "Observation"]},
            "scope": {"type": "string", "title": "SMART Scope", "default": "system/*.read"},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60},
            "rate_limit_per_minute": {"type": "number", "title": "Max Requests / Minute", "default": 100}
        },
        "required": ["fhir_base_url", "client_id"]
    }',
    '{"basic": ["fhir_base_url", "epic_environment", "resource_types", "polling_interval_seconds"], "security": ["client_id", "scope"], "advanced": ["rate_limit_per_minute"]}',
    true, true, false, true,
    'EMR / EHR', 5
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'epic_fhir_inbound');

-- Epic FHIR R4 Outbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'epic_fhir_outbound', 'Epic FHIR R4 Outbound', 'outbound', 'push',
    'Write FHIR R4 resources back to Epic Systems via the Epic FHIR API with SMART on FHIR authentication.',
    '🏥',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "Epic FHIR Base URL"},
            "epic_environment": {"type": "string", "title": "Environment", "enum": ["production", "sandbox", "non_prod"], "default": "sandbox"},
            "client_id": {"type": "string", "title": "Epic Client ID"},
            "scope": {"type": "string", "title": "SMART Scope", "default": "system/*.write"},
            "timeout_seconds": {"type": "number", "title": "Timeout (seconds)", "default": 30}
        },
        "required": ["fhir_base_url", "client_id"]
    }',
    '{"basic": ["fhir_base_url", "epic_environment", "timeout_seconds"], "security": ["client_id", "scope"]}',
    false, true, false, true,
    'EMR / EHR', 6
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'epic_fhir_outbound');

-- Cerner FHIR R4 Inbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'cerner_fhir_inbound', 'Cerner FHIR R4 Inbound', 'inbound', 'pull',
    'Receive FHIR R4 resources from Cerner Millennium via the Oracle Health FHIR API with OAuth2 authentication.',
    '🏥',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "Cerner FHIR Base URL", "description": "e.g. https://fhir-ehr.cerner.com/r4/{tenant}"},
            "tenant_id": {"type": "string", "title": "Cerner Tenant ID"},
            "client_id": {"type": "string", "title": "OAuth2 Client ID"},
            "client_secret": {"type": "string", "title": "OAuth2 Client Secret", "format": "password"},
            "resource_types": {"type": "array", "title": "Resource Types", "default": ["Patient", "Observation"]},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60}
        },
        "required": ["fhir_base_url", "client_id", "client_secret"]
    }',
    '{"basic": ["fhir_base_url", "tenant_id", "resource_types", "polling_interval_seconds"], "security": ["client_id", "client_secret"]}',
    true, true, false, true,
    'EMR / EHR', 7
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'cerner_fhir_inbound');

-- Cerner FHIR R4 Outbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'cerner_fhir_outbound', 'Cerner FHIR R4 Outbound', 'outbound', 'push',
    'Write FHIR R4 resources to Cerner Millennium via the Oracle Health FHIR API.',
    '🏥',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "Cerner FHIR Base URL"},
            "tenant_id": {"type": "string", "title": "Cerner Tenant ID"},
            "client_id": {"type": "string", "title": "OAuth2 Client ID"},
            "client_secret": {"type": "string", "title": "OAuth2 Client Secret", "format": "password"},
            "timeout_seconds": {"type": "number", "title": "Timeout (seconds)", "default": 30}
        },
        "required": ["fhir_base_url", "client_id", "client_secret"]
    }',
    '{"basic": ["fhir_base_url", "tenant_id", "timeout_seconds"], "security": ["client_id", "client_secret"]}',
    false, true, false, true,
    'EMR / EHR', 8
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'cerner_fhir_outbound');

-- Meditech FHIR Inbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'meditech_fhir_inbound', 'Meditech FHIR Inbound', 'inbound', 'pull',
    'Receive FHIR R4 resources from Meditech Expanse via FHIR API.',
    '🏥',
    '{
        "properties": {
            "fhir_base_url": {"type": "string", "title": "Meditech FHIR Base URL"},
            "client_id": {"type": "string", "title": "OAuth2 Client ID"},
            "client_secret": {"type": "string", "title": "OAuth2 Client Secret", "format": "password"},
            "resource_types": {"type": "array", "title": "Resource Types", "default": ["Patient"]},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60}
        },
        "required": ["fhir_base_url", "client_id"]
    }',
    '{"basic": ["fhir_base_url", "resource_types", "polling_interval_seconds"], "security": ["client_id", "client_secret"]}',
    true, true, false, true,
    'EMR / EHR', 9
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'meditech_fhir_inbound');

-- EDI X12 Inbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'edi_x12_inbound', 'EDI X12 Inbound', 'inbound', 'pull',
    'Receive EDI X12 transactions (837 claims, 835 remittance, 270/271 eligibility) via SFTP or HTTP. Parses ISA/GS envelopes and individual transaction sets.',
    '💰',
    '{
        "properties": {
            "transport": {"type": "string", "title": "Transport", "enum": ["sftp", "http", "as2"], "default": "sftp"},
            "host": {"type": "string", "title": "Host"},
            "port": {"type": "number", "title": "Port", "default": 22},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "remote_path": {"type": "string", "title": "Remote Path", "default": "/incoming"},
            "transaction_types": {"type": "array", "title": "Transaction Types", "description": "e.g. 837P, 835, 270", "default": ["837P"]},
            "generate_999_ack": {"type": "boolean", "title": "Generate 999 Acknowledgment", "default": true},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 300}
        },
        "required": ["transport"]
    }',
    '{"basic": ["transport", "transaction_types", "polling_interval_seconds", "generate_999_ack"], "security": ["host", "port", "username", "password", "remote_path"]}',
    true, true, false, true,
    'Payers & Revenue Cycle', 50
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'edi_x12_inbound');

-- EDI X12 Outbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'edi_x12_outbound', 'EDI X12 Outbound', 'outbound', 'push',
    'Send EDI X12 transactions to trading partners or clearinghouses via SFTP or HTTP. Generates ISA/GS envelopes and proper transaction set wrapping.',
    '💰',
    '{
        "properties": {
            "transport": {"type": "string", "title": "Transport", "enum": ["sftp", "http", "as2"], "default": "sftp"},
            "host": {"type": "string", "title": "Host"},
            "port": {"type": "number", "title": "Port", "default": 22},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "remote_path": {"type": "string", "title": "Remote Path", "default": "/outgoing"},
            "isa_sender_id": {"type": "string", "title": "ISA Sender ID"},
            "isa_receiver_id": {"type": "string", "title": "ISA Receiver ID"},
            "timeout_seconds": {"type": "number", "title": "Timeout (seconds)", "default": 60}
        },
        "required": ["transport"]
    }',
    '{"basic": ["transport", "timeout_seconds"], "security": ["host", "port", "username", "password", "remote_path"], "advanced": ["isa_sender_id", "isa_receiver_id"]}',
    false, true, false, true,
    'Payers & Revenue Cycle', 51
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'edi_x12_outbound');

-- Direct Messaging Inbound (DirectTrust SMTP+SMIME)
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'direct_messaging_inbound', 'Direct Messaging Inbound', 'inbound', 'pull',
    'Receive clinical documents via Direct Trust (DirectTrust) protocol — S/MIME-encrypted SMTP messages carrying C-CDA documents.',
    '📧',
    '{
        "properties": {
            "imap_host": {"type": "string", "title": "IMAP Host"},
            "imap_port": {"type": "number", "title": "IMAP Port", "default": 993},
            "username": {"type": "string", "title": "Direct Address", "description": "e.g. provider@direct.hospital.org"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "cert_content": {"type": "string", "title": "S/MIME Certificate (PEM)", "format": "password"},
            "private_key": {"type": "string", "title": "Private Key (PEM)", "format": "password"},
            "polling_interval_seconds": {"type": "number", "title": "Polling Interval (seconds)", "default": 60}
        },
        "required": ["imap_host", "username", "password"]
    }',
    '{"basic": ["imap_host", "imap_port", "username", "polling_interval_seconds"], "security": ["password", "cert_content", "private_key"]}',
    true, true, false, true,
    'Healthcare Protocols', 25
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'direct_messaging_inbound');

-- Direct Messaging Outbound
INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'direct_messaging_outbound', 'Direct Messaging Outbound', 'outbound', 'push',
    'Send clinical documents via Direct Trust (DirectTrust) protocol — S/MIME-encrypted SMTP to any Direct-enabled provider.',
    '📧',
    '{
        "properties": {
            "smtp_host": {"type": "string", "title": "SMTP Host"},
            "smtp_port": {"type": "number", "title": "SMTP Port", "default": 587},
            "username": {"type": "string", "title": "Sender Direct Address"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "cert_content": {"type": "string", "title": "S/MIME Certificate (PEM)", "format": "password"},
            "private_key": {"type": "string", "title": "Private Key (PEM)", "format": "password"},
            "recipient_address": {"type": "string", "title": "Default Recipient Direct Address"}
        },
        "required": ["smtp_host", "username", "password"]
    }',
    '{"basic": ["smtp_host", "smtp_port", "username", "recipient_address"], "security": ["password", "cert_content", "private_key"]}',
    false, true, false, true,
    'Healthcare Protocols', 26
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'direct_messaging_outbound');
