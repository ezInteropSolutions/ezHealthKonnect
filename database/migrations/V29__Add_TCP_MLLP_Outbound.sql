-- V29__Add_TCP_MLLP_Outbound.sql
-- Add TCP/MLLP Outbound connector for middleware scenarios
-- ezHealthKonnect as middleware can send HL7 to downstream systems (Epic, Cerner, Meditech, etc.)

-- TCP/MLLP Connector (Outbound - Client)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'tcp_mllp_outbound',
    'outbound',
    'TCP/MLLP (HL7 v2.x) Client',
    'Send HL7 v2.x messages to downstream MLLP endpoint (Epic, Cerner, Meditech, etc.)',
    '🔌',
    'push',
    false,
    true,
    false,
    'TCPMLLPOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Destination Host",
                "description": "Hostname or IP of downstream HL7 system"
            },
            "port": {
                "type": "integer",
                "title": "Destination Port",
                "default": 2575,
                "minimum": 1024,
                "maximum": 65535
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false,
                "description": "Use TLS for encrypted MLLP connection"
            },
            "tls_version": {
                "type": "string",
                "title": "TLS Version",
                "enum": ["TLS_1_2", "TLS_1_3"],
                "default": "TLS_1_2"
            },
            "tls_cert_path": {
                "type": "string",
                "title": "TLS Certificate Path",
                "description": "Client certificate for mutual TLS authentication"
            },
            "tls_key_path": {
                "type": "string",
                "title": "TLS Private Key Path"
            },
            "verify_server_cert": {
                "type": "boolean",
                "title": "Verify Server Certificate",
                "default": true
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30,
                "minimum": 5,
                "maximum": 300
            },
            "read_timeout": {
                "type": "integer",
                "title": "Read Timeout (seconds)",
                "description": "Timeout waiting for acknowledgment (ACK/NACK)",
                "default": 30,
                "minimum": 5,
                "maximum": 300
            },
            "retry_attempts": {
                "type": "integer",
                "title": "Retry Attempts",
                "description": "Number of retries on connection failure",
                "default": 3,
                "minimum": 0,
                "maximum": 10
            },
            "retry_delay_seconds": {
                "type": "integer",
                "title": "Retry Delay (seconds)",
                "default": 5,
                "minimum": 1,
                "maximum": 60
            },
            "keep_alive": {
                "type": "boolean",
                "title": "Keep Connection Alive",
                "description": "Maintain persistent connection for multiple messages",
                "default": true
            },
            "keep_alive_timeout": {
                "type": "integer",
                "title": "Keep-Alive Timeout (seconds)",
                "description": "Close idle connection after timeout",
                "default": 300
            },
            "validate_ack": {
                "type": "boolean",
                "title": "Validate ACK/NACK Response",
                "description": "Parse and validate acknowledgment messages",
                "default": true
            },
            "authentication_type": {
                "type": "string",
                "title": "Authentication Type",
                "enum": ["none", "basic", "token", "mtls"],
                "default": "none",
                "description": "Authentication method (basic=MSH-8, token=custom, mtls=certificate)"
            },
            "username": {
                "type": "string",
                "title": "Username",
                "description": "For basic authentication (placed in MSH-8)"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password",
                "description": "For basic authentication"
            },
            "auth_token": {
                "type": "string",
                "title": "Authentication Token",
                "format": "password",
                "description": "Custom authentication token"
            },
            "message_encoding": {
                "type": "string",
                "title": "Message Encoding",
                "enum": ["UTF-8", "ASCII", "ISO-8859-1"],
                "default": "UTF-8"
            },
            "use_mllp_framing": {
                "type": "boolean",
                "title": "Use MLLP Framing",
                "description": "Wrap messages with MLLP start/end bytes (0x0B/0x1C/0x0D)",
                "default": true
            },
            "mllp_start_byte": {
                "type": "string",
                "title": "MLLP Start Byte (hex)",
                "default": "0x0B",
                "description": "Vertical Tab (VT)"
            },
            "mllp_end_byte_1": {
                "type": "string",
                "title": "MLLP End Byte 1 (hex)",
                "default": "0x1C",
                "description": "File Separator (FS)"
            },
            "mllp_end_byte_2": {
                "type": "string",
                "title": "MLLP End Byte 2 (hex)",
                "default": "0x0D",
                "description": "Carriage Return (CR)"
            },
            "log_hl7_messages": {
                "type": "boolean",
                "title": "Log HL7 Messages",
                "description": "Log full HL7 message content for debugging",
                "default": false
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "description": "Number of concurrent connections to maintain",
                "default": 1,
                "minimum": 1,
                "maximum": 10
            },
            "source_ip": {
                "type": "string",
                "title": "Source IP (optional)",
                "description": "Bind to specific local IP address"
            }
        }
    }',
    '{
        "basic": ["host", "port", "connection_timeout", "read_timeout"],
        "reliability": ["retry_attempts", "retry_delay_seconds", "validate_ack"],
        "connection": ["keep_alive", "keep_alive_timeout", "connection_pool_size"],
        "security": ["enable_tls", "tls_version", "tls_cert_path", "tls_key_path", "verify_server_cert"],
        "authentication": ["authentication_type", "username", "password", "auth_token"],
        "protocol": ["use_mllp_framing", "mllp_start_byte", "mllp_end_byte_1", "mllp_end_byte_2", "message_encoding"],
        "advanced": ["log_hl7_messages", "source_ip"]
    }',
    true,
    false,
    10,
    '1.0'
);

-- Update documentation
COMMENT ON TABLE connectivity_types IS 'Out-of-Box connector definitions with complete configuration schemas. Total: 32 connectors (16 inbound, 16 outbound)';

-- Summary
-- Added TCP/MLLP Outbound connector for middleware scenarios
-- Now supports: ezHealthKonnect → TCP/MLLP → Downstream HL7 Systems
-- Complete bidirectional MLLP support:
--   - tcp_mllp (inbound): Listen for incoming HL7 connections
--   - tcp_mllp_outbound (outbound): Send HL7 to downstream systems
-- Total connectors: 31 → 32 (16 inbound, 16 outbound, perfectly symmetric!)
