-- ========================================
-- V26: Multi-Connectivity Support
-- ========================================
-- Description: Universal connectivity layer with OOB connectors, cron scheduling, and cloud storage support
-- Author: ezHealthKonnect Team
-- Date: 2025-10-25
-- Dependencies: V25 (Performance Optimization Indexes)

-- ========================================
-- 1. Connectivity Type Definitions (OOB Connectors)
-- ========================================

CREATE TABLE connectivity_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Type identification
    type_name VARCHAR(50) UNIQUE NOT NULL,  -- 'tcp_mllp', 'http_rest', 'file_listener', 'aws_s3_inbound', etc.
    category VARCHAR(20) NOT NULL CHECK (category IN ('inbound', 'outbound')),
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    icon VARCHAR(50),                       -- Emoji or icon identifier

    -- Behavior
    mode VARCHAR(20) NOT NULL CHECK (mode IN ('push', 'pull', 'stream')),
    supports_cron BOOLEAN DEFAULT false,
    requires_auth BOOLEAN DEFAULT false,
    is_bidirectional BOOLEAN DEFAULT false,

    -- Implementation
    implementation_class VARCHAR(255),      -- Go struct or JavaScript class name
    config_schema JSONB NOT NULL,          -- JSON Schema for parameters
    default_config JSONB,                  -- Default parameter values

    -- UI hints
    wizard_template VARCHAR(50),           -- Which wizard template to use
    parameter_groups JSONB,                -- Group parameters for UI display
    validation_rules JSONB,                -- Additional validation beyond schema

    -- Status
    is_active BOOLEAN DEFAULT true,
    is_beta BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 100,          -- Display order in UI

    -- Metadata
    version VARCHAR(20),
    documentation_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE connectivity_types IS 'OOB connector definitions with configuration schemas and metadata';
COMMENT ON COLUMN connectivity_types.type_name IS 'Unique identifier for connector type';
COMMENT ON COLUMN connectivity_types.config_schema IS 'JSON Schema defining required/optional parameters';
COMMENT ON COLUMN connectivity_types.parameter_groups IS 'Groups like {basic: [...], security: [...], advanced: [...]}';

-- ========================================
-- 2. Interface Connectivity Configuration
-- ========================================

CREATE TABLE interface_connectivity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,

    -- Source (Inbound) Configuration
    source_connectivity_type_id UUID REFERENCES connectivity_types(id),
    source_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_enabled BOOLEAN DEFAULT true,

    -- Cron scheduling (for pull-based sources)
    cron_enabled BOOLEAN DEFAULT false,
    cron_expression VARCHAR(100),          -- e.g., "*/5 * * * *" (every 5 minutes)
    cron_timezone VARCHAR(50) DEFAULT 'UTC',
    next_run_at TIMESTAMP WITH TIME ZONE,
    last_run_at TIMESTAMP WITH TIME ZONE,
    last_run_status VARCHAR(50),

    -- Target (Outbound) Configuration
    target_connectivity_type_id UUID REFERENCES connectivity_types(id),
    target_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    target_enabled BOOLEAN DEFAULT true,

    -- Multi-target support (future)
    additional_targets JSONB,              -- Array of {type_id, config, enabled}

    -- Connection state
    connection_status VARCHAR(50) DEFAULT 'disconnected' CHECK (connection_status IN ('connected', 'disconnected', 'error', 'testing')),
    last_connection_test TIMESTAMP WITH TIME ZONE,
    last_connection_error TEXT,

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT one_connectivity_per_interface UNIQUE(interface_id)
);

COMMENT ON TABLE interface_connectivity IS 'Per-interface source and target connectivity configuration';
COMMENT ON COLUMN interface_connectivity.source_config IS 'Type-specific configuration matching connector schema';
COMMENT ON COLUMN interface_connectivity.cron_expression IS 'Standard cron syntax: minute hour day month weekday';

-- ========================================
-- 3. Cron Job Scheduler State
-- ========================================

CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_connectivity_id UUID REFERENCES interface_connectivity(id) ON DELETE CASCADE,

    -- Schedule
    cron_expression VARCHAR(100) NOT NULL,
    timezone VARCHAR(50) DEFAULT 'UTC',

    -- State
    is_enabled BOOLEAN DEFAULT true,
    next_execution TIMESTAMP WITH TIME ZONE,
    last_execution TIMESTAMP WITH TIME ZONE,
    execution_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    consecutive_failures INTEGER DEFAULT 0,

    -- Error handling & circuit breaker
    max_retries INTEGER DEFAULT 3,
    retry_delay_seconds INTEGER DEFAULT 60,
    circuit_breaker_threshold INTEGER DEFAULT 5,  -- Disable after N consecutive failures
    is_circuit_broken BOOLEAN DEFAULT false,
    circuit_broken_at TIMESTAMP WITH TIME ZONE,

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE cron_jobs IS 'Cron scheduler state with circuit breaker for pull-based connectors';
COMMENT ON COLUMN cron_jobs.circuit_breaker_threshold IS 'Auto-disable job after this many consecutive failures';

-- ========================================
-- 4. Connectivity Execution Logs
-- ========================================

CREATE TABLE connectivity_execution_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_connectivity_id UUID REFERENCES interface_connectivity(id) ON DELETE CASCADE,
    interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,

    -- Execution details
    execution_type VARCHAR(50) NOT NULL CHECK (execution_type IN ('cron', 'manual', 'startup', 'test')),
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) CHECK (status IN ('running', 'success', 'failed', 'partial')),

    -- Results
    messages_retrieved INTEGER DEFAULT 0,
    messages_processed INTEGER DEFAULT 0,
    messages_failed INTEGER DEFAULT 0,
    error_details JSONB,

    -- Performance
    duration_ms INTEGER,

    -- Metadata
    triggered_by VARCHAR(100),             -- 'cron', 'user:email', 'system', 'test'
    execution_metadata JSONB
);

COMMENT ON TABLE connectivity_execution_log IS 'Execution history for scheduled and manual connector runs';
COMMENT ON COLUMN connectivity_execution_log.execution_type IS 'How execution was triggered';

-- ========================================
-- 5. Indexes for Performance
-- ========================================

-- Interface connectivity lookups
CREATE INDEX idx_interface_connectivity_interface ON interface_connectivity(interface_id);
CREATE INDEX idx_interface_connectivity_source_type ON interface_connectivity(source_connectivity_type_id);
CREATE INDEX idx_interface_connectivity_target_type ON interface_connectivity(target_connectivity_type_id);
CREATE INDEX idx_interface_connectivity_status ON interface_connectivity(connection_status) WHERE connection_status != 'disconnected';

-- Cron job scheduling
CREATE INDEX idx_cron_jobs_next_execution ON cron_jobs(next_execution)
    WHERE is_enabled = true AND is_circuit_broken = false;
CREATE INDEX idx_cron_jobs_interface_conn ON cron_jobs(interface_connectivity_id);
CREATE INDEX idx_cron_jobs_circuit_broken ON cron_jobs(is_circuit_broken) WHERE is_circuit_broken = true;

-- Execution log queries
CREATE INDEX idx_connectivity_execution_log_interface ON connectivity_execution_log(interface_id);
CREATE INDEX idx_connectivity_execution_log_started ON connectivity_execution_log(started_at DESC);
CREATE INDEX idx_connectivity_execution_log_status ON connectivity_execution_log(status);
CREATE INDEX idx_connectivity_execution_log_interface_started ON connectivity_execution_log(interface_id, started_at DESC);

-- Connectivity types (active connectors)
CREATE INDEX idx_connectivity_types_category ON connectivity_types(category) WHERE is_active = true;
CREATE INDEX idx_connectivity_types_active ON connectivity_types(is_active, priority);

-- ========================================
-- 6. Triggers for Automatic Updates
-- ========================================

-- Update timestamps on modification
CREATE OR REPLACE FUNCTION update_connectivity_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_connectivity_types_timestamp
    BEFORE UPDATE ON connectivity_types
    FOR EACH ROW
    EXECUTE FUNCTION update_connectivity_timestamp();

CREATE TRIGGER update_interface_connectivity_timestamp
    BEFORE UPDATE ON interface_connectivity
    FOR EACH ROW
    EXECUTE FUNCTION update_connectivity_timestamp();

CREATE TRIGGER update_cron_jobs_timestamp
    BEFORE UPDATE ON cron_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_connectivity_timestamp();

-- ========================================
-- 7. Seed Data - Core OOB Connectors
-- ========================================

-- TCP/MLLP Connector (Enhanced with TLS)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'tcp_mllp',
    'inbound',
    'TCP/MLLP (HL7 v2.x)',
    'Receive HL7 v2.x messages over MLLP protocol with TLS and authentication support',
    '🔌',
    'push',
    false,
    true,
    'TCPMLLPConnector',
    10,
    '2.0',
    '{
      "type": "object",
      "required": ["port"],
      "properties": {
        "port": {"type": "integer", "minimum": 1024, "maximum": 65535, "default": 2575, "title": "Listen Port"},
        "host": {"type": "string", "default": "0.0.0.0", "title": "Bind Address"},
        "max_connections": {"type": "integer", "default": 10, "title": "Max Connections"},
        "timeout_seconds": {"type": "integer", "default": 300, "title": "Timeout (seconds)"},
        "enable_tls": {"type": "boolean", "default": false, "title": "Enable TLS/SSL"},
        "tls_cert_path": {"type": "string", "title": "TLS Certificate Path"},
        "tls_key_path": {"type": "string", "title": "TLS Private Key Path"},
        "enable_authentication": {"type": "boolean", "default": false, "title": "Enable Authentication"},
        "authentication_method": {"type": "string", "enum": ["basic", "token", "ip_whitelist"], "title": "Auth Method"}
      }
    }'::jsonb,
    '{"basic": ["port", "host"], "security": ["enable_tls", "tls_cert_path", "tls_key_path", "enable_authentication", "authentication_method"], "advanced": ["max_connections", "timeout_seconds"]}'::jsonb
);

-- HTTP/REST Inbound
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'http_rest',
    'inbound',
    'HTTP/REST API',
    'Receive messages via HTTP POST endpoint',
    '🌐',
    'push',
    false,
    true,
    'HTTPRESTConnector',
    20,
    '1.0',
    '{
      "type": "object",
      "required": ["endpoint_path"],
      "properties": {
        "endpoint_path": {"type": "string", "default": "/api/hl7/receive", "title": "Endpoint Path"},
        "http_methods": {"type": "array", "items": {"type": "string", "enum": ["POST", "PUT"]}, "default": ["POST"], "title": "HTTP Methods"},
        "authentication_type": {"type": "string", "enum": ["none", "api_key", "basic_auth", "bearer_token"], "default": "api_key", "title": "Authentication"},
        "api_key_header": {"type": "string", "default": "X-API-Key", "title": "API Key Header"},
        "content_type": {"type": "string", "default": "text/plain", "title": "Content-Type"}
      }
    }'::jsonb,
    '{"basic": ["endpoint_path", "http_methods"], "security": ["authentication_type", "api_key_header"], "advanced": ["content_type"]}'::jsonb
);

-- File Listener (Cron-based)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'file_listener',
    'inbound',
    'File System Listener',
    'Monitor directory for new files (scheduled polling)',
    '📁',
    'pull',
    true,
    false,
    'FileListenerConnector',
    30,
    '1.0',
    '{
      "type": "object",
      "required": ["directory_path", "file_pattern"],
      "properties": {
        "directory_path": {"type": "string", "title": "Monitor Directory"},
        "file_pattern": {"type": "string", "default": "*.hl7", "title": "File Pattern"},
        "after_processing": {"type": "string", "enum": ["delete", "move", "archive", "nothing"], "default": "move", "title": "After Processing"},
        "archive_directory": {"type": "string", "title": "Archive Directory"},
        "file_encoding": {"type": "string", "default": "UTF-8", "title": "File Encoding"}
      }
    }'::jsonb,
    '{"basic": ["directory_path", "file_pattern"], "processing": ["after_processing", "archive_directory"], "advanced": ["file_encoding"]}'::jsonb
);

-- AWS S3 Inbound (Cron-based)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'aws_s3_inbound',
    'inbound',
    'AWS S3 Bucket Reader',
    'Poll AWS S3 bucket for new objects (scheduled)',
    '☁️',
    'pull',
    true,
    true,
    'AWSS3InboundConnector',
    40,
    '1.0',
    '{
      "type": "object",
      "required": ["bucket_name", "region"],
      "properties": {
        "bucket_name": {"type": "string", "title": "S3 Bucket Name"},
        "region": {"type": "string", "title": "AWS Region", "default": "us-east-1"},
        "prefix": {"type": "string", "title": "Object Prefix/Folder"},
        "file_pattern": {"type": "string", "default": "*.hl7", "title": "File Pattern"},
        "authentication_method": {"type": "string", "enum": ["access_keys", "iam_role", "assumed_role"], "default": "access_keys", "title": "Auth Method"},
        "access_key_id": {"type": "string", "title": "Access Key ID"},
        "secret_access_key": {"type": "string", "format": "password", "title": "Secret Access Key"},
        "after_processing": {"type": "string", "enum": ["delete", "move", "tag", "nothing"], "default": "move", "title": "After Processing"},
        "max_objects_per_poll": {"type": "integer", "default": 100, "title": "Max Objects Per Poll"}
      }
    }'::jsonb,
    '{"basic": ["bucket_name", "region", "prefix"], "authentication": ["authentication_method", "access_key_id", "secret_access_key"], "processing": ["after_processing", "max_objects_per_poll"]}'::jsonb
);

-- HTTP/HTTPS Outbound
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'http_outbound',
    'outbound',
    'HTTP/HTTPS Endpoint',
    'POST to REST endpoint',
    '🌐',
    'push',
    false,
    true,
    'HTTPOutboundConnector',
    10,
    '1.0',
    '{
      "type": "object",
      "required": ["url"],
      "properties": {
        "url": {"type": "string", "format": "uri", "title": "Destination URL"},
        "method": {"type": "string", "enum": ["POST", "PUT"], "default": "POST", "title": "HTTP Method"},
        "authentication_type": {"type": "string", "enum": ["none", "basic_auth", "bearer_token", "api_key"], "default": "none", "title": "Authentication"},
        "content_type": {"type": "string", "default": "application/json", "title": "Content-Type"},
        "timeout_seconds": {"type": "integer", "default": 30, "title": "Timeout (seconds)"}
      }
    }'::jsonb,
    '{"basic": ["url", "method", "content_type"], "security": ["authentication_type"], "advanced": ["timeout_seconds"]}'::jsonb
);

-- File Writer Outbound
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'file_writer',
    'outbound',
    'File System Writer',
    'Write to file system',
    '📁',
    'push',
    false,
    false,
    'FileWriterConnector',
    20,
    '1.0',
    '{
      "type": "object",
      "required": ["directory_path"],
      "properties": {
        "directory_path": {"type": "string", "title": "Output Directory"},
        "filename_pattern": {"type": "string", "default": "{message_id}_{timestamp}.hl7", "title": "Filename Pattern"},
        "file_encoding": {"type": "string", "default": "UTF-8", "title": "File Encoding"},
        "create_subdirectories": {"type": "boolean", "default": true, "title": "Create Subdirectories"},
        "overwrite_existing": {"type": "boolean", "default": false, "title": "Overwrite Existing Files"}
      }
    }'::jsonb,
    '{"basic": ["directory_path", "filename_pattern"], "advanced": ["file_encoding", "create_subdirectories", "overwrite_existing"]}'::jsonb
);

-- AWS S3 Outbound
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon,
    mode, supports_cron, requires_auth,
    implementation_class, priority, version,
    config_schema, parameter_groups
) VALUES (
    'aws_s3_outbound',
    'outbound',
    'AWS S3 Bucket Writer',
    'Upload to AWS S3 bucket',
    '☁️',
    'push',
    false,
    true,
    'AWSS3OutboundConnector',
    30,
    '1.0',
    '{
      "type": "object",
      "required": ["bucket_name", "region"],
      "properties": {
        "bucket_name": {"type": "string", "title": "S3 Bucket Name"},
        "region": {"type": "string", "title": "AWS Region", "default": "us-east-1"},
        "prefix": {"type": "string", "title": "Object Prefix/Folder"},
        "filename_pattern": {"type": "string", "default": "{message_id}_{timestamp}.json", "title": "Filename Pattern"},
        "authentication_method": {"type": "string", "enum": ["access_keys", "iam_role"], "default": "access_keys", "title": "Auth Method"},
        "access_key_id": {"type": "string", "title": "Access Key ID"},
        "secret_access_key": {"type": "string", "format": "password", "title": "Secret Access Key"},
        "storage_class": {"type": "string", "enum": ["STANDARD", "INTELLIGENT_TIERING", "STANDARD_IA"], "default": "STANDARD", "title": "Storage Class"},
        "enable_server_side_encryption": {"type": "boolean", "default": true, "title": "Enable SSE"}
      }
    }'::jsonb,
    '{"basic": ["bucket_name", "region", "prefix", "filename_pattern"], "authentication": ["authentication_method", "access_key_id", "secret_access_key"], "advanced": ["storage_class", "enable_server_side_encryption"]}'::jsonb
);

-- ========================================
-- 8. Grant Permissions
-- ========================================

-- Grant permissions to application user
GRANT SELECT, INSERT, UPDATE, DELETE ON connectivity_types TO ezhealth_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON interface_connectivity TO ezhealth_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON cron_jobs TO ezhealth_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON connectivity_execution_log TO ezhealth_user;

-- ========================================
-- Migration Complete
-- ========================================

COMMENT ON SCHEMA public IS 'V26: Multi-Connectivity Support - OOB connectors with cloud storage and cron scheduling';
