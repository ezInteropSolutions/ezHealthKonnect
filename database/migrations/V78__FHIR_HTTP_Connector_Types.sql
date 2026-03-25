-- V78: FHIR HTTP Connector Types (Phase A)
-- ─────────────────────────────────────────────────────────────────────────────
-- Adds http_fhir_inbound and http_fhir_outbound as first-class connector types.
--
-- These are the canonical Phase A implementations:
--   http_fhir_inbound  → HTTPFHIRInboundConnector
--                        Push-mode FHIR R4 HTTP receiver. Accepts single
--                        resources and Bundles. bundleMode config controls
--                        whether a Bundle is routed as one message or unwrapped
--                        into individual per-resource messages.
--                        Exposes GET /metadata CapabilityStatement.
--
--   http_fhir_outbound → HTTPFHIROutboundConnector
--                        FHIR-aware HTTP delivery. Auto-builds URL from
--                        resourceType + id, sets FHIR headers, parses
--                        OperationOutcome error bodies, follows Location header.
--
-- Also updates fhir_r4_inbound / fhir_r4_outbound descriptions and marks them
-- non-beta now that they route to the real implementations.
-- ─────────────────────────────────────────────────────────────────────────────

-- ── 1. http_fhir_inbound ─────────────────────────────────────────────────────

INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups,
     supports_cron, requires_auth, is_bidirectional, is_beta,
     ui_category, ui_sort_order)
SELECT
    gen_random_uuid(),
    'http_fhir_inbound',
    'HTTP FHIR Receiver',
    'inbound',
    'push',
    'Listens for FHIR R4 resources and Bundles via HTTP POST. Supports bundle_as_unit (whole Bundle = 1 message) and bundle_unwrap (each entry = 1 message). Returns FHIR CapabilityStatement on GET /metadata. Supports Basic, Bearer, and API Key authentication.',
    '🏥',
    '{
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
                "description": "How incoming Bundles are processed",
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
            "maxBodySizeMB": {
                "type": "number",
                "title": "Max Body Size (MB)",
                "description": "Maximum accepted request body size",
                "default": 10,
                "minimum": 1,
                "maximum": 500
            }
        },
        "required": ["port"]
    }',
    '{
        "basic":    ["port", "basePath", "fhirVersion", "bundleMode"],
        "security": ["authType", "username", "password", "bearerToken", "apiKey"],
        "advanced": ["maxBodySizeMB"]
    }',
    false, false, false, false,
    'Healthcare Protocols', 15
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'http_fhir_inbound');

-- ── 2. http_fhir_outbound ────────────────────────────────────────────────────

INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups,
     supports_cron, requires_auth, is_bidirectional, is_beta,
     ui_category, ui_sort_order)
SELECT
    gen_random_uuid(),
    'http_fhir_outbound',
    'HTTP FHIR Sender',
    'outbound',
    'push',
    'Delivers FHIR R4 resources and Bundles to a FHIR REST server. Auto-constructs the URL from resourceType and id (e.g. POST /Patient, PUT /Patient/123, POST /$transaction for transaction Bundles). Parses OperationOutcome error bodies from 4xx/5xx responses. Supports Basic, Bearer (SMART on FHIR), and API Key authentication.',
    '🚀',
    '{
        "type": "object",
        "properties": {
            "baseUrl": {
                "type": "string",
                "title": "FHIR Base URL",
                "description": "e.g. https://fhir.hospital.com/r4"
            },
            "resourceType": {
                "type": "string",
                "title": "Resource Type",
                "description": "Target FHIR resource type (e.g. Patient, Observation). Leave blank to auto-detect from the message body at runtime."
            },
            "resourceId": {
                "type": "string",
                "title": "Resource ID",
                "description": "Logical resource id. When set, produces PUT /<ResourceType>/<id> (update). Leave blank for POST (create)."
            },
            "operation": {
                "type": "string",
                "title": "FHIR Operation",
                "description": "Optional FHIR operation to invoke, with or without the $ prefix. Applied at the appropriate level: server ($process-message), type (Patient/$validate), or instance (Patient/id/$everything)."
            },
            "queryParams": {
                "type": "string",
                "title": "Query Parameters",
                "description": "Raw URL query string appended to the endpoint (no leading ?). e.g. _format=json&_summary=count or family=Smith&gender=male for FHIR search."
            },
            "method": {
                "type": "string",
                "title": "HTTP Method",
                "enum": ["POST", "PUT", "PATCH", "GET", "DELETE"],
                "default": "POST",
                "description": "Defaults to POST for create and operations, PUT when Resource ID is set. Override here for PATCH (partial update), GET (read / search), or DELETE."
            },
            "fhirVersion": {
                "type": "string",
                "title": "FHIR Version",
                "enum": ["R4", "R5", "STU3"],
                "default": "R4"
            },
            "authType": {
                "type": "string",
                "title": "Authentication Type",
                "enum": ["none", "basic", "bearer", "smart", "api_key"],
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
                "title": "Bearer / SMART Token",
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
                "default": "X-API-Key"
            },
            "timeout": {
                "type": "number",
                "title": "Timeout (seconds)",
                "default": 30,
                "minimum": 1,
                "maximum": 300
            },
            "retryAttempts": {
                "type": "number",
                "title": "Retry Attempts",
                "default": 3,
                "minimum": 0,
                "maximum": 10
            },
            "retryDelay": {
                "type": "number",
                "title": "Retry Delay (seconds)",
                "default": 1,
                "minimum": 0
            }
        },
        "required": ["baseUrl"]
    }',
    '{
        "basic":    ["baseUrl", "resourceType", "resourceId", "operation", "method", "fhirVersion"],
        "security": ["authType", "username", "password", "bearerToken", "apiKey", "apiKeyHeader"],
        "advanced": ["queryParams", "timeout", "retryAttempts", "retryDelay"]
    }',
    false, false, false, false,
    'Healthcare Protocols', 16
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'http_fhir_outbound');

-- ── 3. Upgrade fhir_r4_inbound / fhir_r4_outbound ───────────────────────────
-- These now route to the real implementations (no longer stubs).
-- Mark is_beta = false, strip "(Beta)" from display_name, standardize config_schema
-- to match http_fhir_inbound / http_fhir_outbound so the UI auth dynamics work uniformly.

UPDATE connectivity_types
SET
    is_beta          = false,
    display_name     = 'FHIR R4 Receiver',
    description      = 'FHIR R4 HTTP receiver — accepts FHIR resources and Bundles via HTTP POST. Routes to the same engine as http_fhir_inbound. Use http_fhir_inbound for new interfaces; fhir_r4_inbound is kept for backward compatibility.',
    config_schema    = (SELECT config_schema    FROM connectivity_types WHERE type_name = 'http_fhir_inbound'),
    parameter_groups = (SELECT parameter_groups FROM connectivity_types WHERE type_name = 'http_fhir_inbound'),
    ui_category      = 'Healthcare Protocols',
    ui_sort_order    = 17
WHERE type_name = 'fhir_r4_inbound';

UPDATE connectivity_types
SET
    is_beta          = false,
    display_name     = 'FHIR R4 Sender',
    description      = 'FHIR R4 HTTP sender — delivers FHIR resources and Bundles to a FHIR REST server. Routes to the same engine as http_fhir_outbound. Use http_fhir_outbound for new interfaces; fhir_r4_outbound is kept for backward compatibility.',
    config_schema    = (SELECT config_schema    FROM connectivity_types WHERE type_name = 'http_fhir_outbound'),
    parameter_groups = (SELECT parameter_groups FROM connectivity_types WHERE type_name = 'http_fhir_outbound'),
    ui_category      = 'Healthcare Protocols',
    ui_sort_order    = 18
WHERE type_name = 'fhir_r4_outbound';

-- ── 4. Ensure Healthcare Network category covers http_rest_inbound / http_outbound ─
-- http_rest_inbound is the legacy name for http_fhir_inbound — keep it categorised.

UPDATE connectivity_types
SET ui_category = 'Healthcare Network', ui_sort_order = 10
WHERE type_name IN ('tcp_mllp_inbound', 'tcp_mllp_outbound',
                    'http_rest_inbound', 'http_outbound',
                    'tcp_mllp', 'http_rest', 'http')
  AND (ui_category IS NULL OR ui_category = 'Other');
