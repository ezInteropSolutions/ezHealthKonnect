-- V80: FHIR Inbound Operations support
-- ─────────────────────────────────────────────────────────────────────────────
-- Updates http_fhir_inbound config_schema and description to reflect the
-- full FHIR RESTful API surface now supported (§3.1 CRUD + §3.3 Operations).
--
-- New capabilities exposed:
--   - All HTTP methods: GET (search/read), POST (create/ops), PUT (update),
--     PATCH (partial update), DELETE — not just POST
--   - FHIR operations on any URL level:
--       server-level:   POST <basePath>/$process-message
--       type-level:     POST <basePath>/Patient/$validate
--       instance-level: POST <basePath>/Patient/123/$everything
--   - URL path parsed to stamp X-FHIR-Operation, X-HTTP-Method, X-Query-String
--     on every InboundMessage so Switch/Case steps can route on any dimension
--   - allowedMethods: restrict which HTTP verbs this receiver accepts
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE connectivity_types
SET
    description = 'Full FHIR REST server receiver. Accepts the complete FHIR RESTful API surface (§3.1 CRUD) and Operations (§3.3). Parses the incoming URL to extract resource type, resource ID, and operation name, then stamps all routing context onto message headers (X-FHIR-ResourceType, X-FHIR-ResourceId, X-FHIR-Operation, X-HTTP-Method, X-Query-String) so pipeline Switch/Case steps can route without re-parsing the body. Supports Basic, Bearer, and API Key authentication. Returns FHIR CapabilityStatement on GET /metadata.',
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
                "description": "URL prefix for all FHIR endpoints (e.g. /fhir/r4)",
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
                "description": "bundle_as_unit: entire Bundle is one message. bundle_unwrap: each Bundle entry becomes its own message routed individually.",
                "enum": ["bundle_as_unit", "bundle_unwrap"],
                "default": "bundle_as_unit"
            },
            "allowedMethods": {
                "type": "array",
                "title": "Allowed HTTP Methods",
                "description": "Restrict which HTTP methods this receiver accepts. Leave empty to allow all FHIR methods.",
                "items": {
                    "type": "string",
                    "enum": ["GET", "POST", "PUT", "PATCH", "DELETE"]
                },
                "default": ["GET", "POST", "PUT", "PATCH", "DELETE"]
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
                "description": "Server read and write timeout per HTTP request",
                "default": 30,
                "minimum": 1,
                "maximum": 300
            }
        },
        "required": ["port"]
    }',
    parameter_groups = '{
        "basic":    ["port", "basePath", "fhirVersion", "bundleMode", "allowedMethods"],
        "security": ["authType", "username", "password", "bearerToken", "apiKey", "apiKeyHeader"],
        "advanced": ["maxBodySizeMB", "enableCORS", "requestTimeoutSeconds"]
    }'
WHERE type_name = 'http_fhir_inbound';
