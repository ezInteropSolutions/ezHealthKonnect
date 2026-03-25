-- V79: Remove legacy fhir_r4_inbound / fhir_r4_outbound connector types
-- ─────────────────────────────────────────────────────────────────────────────
-- These types are superseded by http_fhir_inbound / http_fhir_outbound (V78).
-- No backward compatibility needed — V1 product, no production instances.
--
-- Also extends http_fhir_inbound config_schema with three new options that
-- complement the outbound connector:
--   apiKeyHeader          — configurable header name for API key auth
--   enableCORS            — toggle CORS headers (default: true)
--   requestTimeoutSeconds — server read/write timeout (default: 30)
-- ─────────────────────────────────────────────────────────────────────────────

-- ── 1. Remove legacy aliases ─────────────────────────────────────────────────

DELETE FROM connectivity_types
WHERE type_name IN ('fhir_r4_inbound', 'fhir_r4_outbound');

-- ── 2. Extend http_fhir_inbound config_schema ────────────────────────────────

UPDATE connectivity_types
SET
    config_schema = '{
        "type": "object",
        "properties": {
            "port": {
                "type": "number",
                "title": "Listen Port",
                "description": "TCP port to listen on (e.g. 7250)",
                "minimum": 1,
                "maximum": 65535
            },
            "basePath": {
                "type": "string",
                "title": "Base Path",
                "description": "URL prefix for FHIR endpoints",
                "default": "/fhir/r4"
            },
            "fhirVersion": {
                "type": "string",
                "title": "FHIR Version",
                "enum": ["R4", "R5", "STU3"],
                "default": "R4"
            },
            "bundleMode": {
                "type": "string",
                "title": "Bundle Mode",
                "description": "bundle_as_unit: entire Bundle is one message. bundle_unwrap: each Bundle entry becomes its own message.",
                "enum": ["bundle_as_unit", "bundle_unwrap"],
                "default": "bundle_as_unit"
            },
            "authType": {
                "type": "string",
                "title": "Authentication Type",
                "enum": ["none", "basic", "bearer", "api_key"],
                "default": "none"
            },
            "username": {
                "type": "string",
                "title": "Username"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "bearerToken": {
                "type": "string",
                "title": "Bearer Token",
                "format": "password"
            },
            "apiKey": {
                "type": "string",
                "title": "API Key",
                "format": "password"
            },
            "apiKeyHeader": {
                "type": "string",
                "title": "API Key Header Name",
                "description": "HTTP header the caller sends the API key in",
                "default": "X-API-Key"
            },
            "maxBodySizeMB": {
                "type": "number",
                "title": "Max Body Size (MB)",
                "description": "Maximum accepted request body size",
                "default": 10,
                "minimum": 1,
                "maximum": 500
            },
            "enableCORS": {
                "type": "boolean",
                "title": "Enable CORS",
                "description": "Add CORS headers to allow browser / cross-origin callers",
                "default": true
            },
            "requestTimeoutSeconds": {
                "type": "number",
                "title": "Request Timeout (seconds)",
                "description": "Server read and write timeout for each HTTP request",
                "default": 30,
                "minimum": 1,
                "maximum": 300
            }
        },
        "required": ["port"]
    }',
    parameter_groups = '{
        "basic":    ["port", "basePath", "fhirVersion", "bundleMode"],
        "security": ["authType", "username", "password", "bearerToken", "apiKey", "apiKeyHeader"],
        "advanced": ["maxBodySizeMB", "enableCORS", "requestTimeoutSeconds"]
    }'
WHERE type_name = 'http_fhir_inbound';
