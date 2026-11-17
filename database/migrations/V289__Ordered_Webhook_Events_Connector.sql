-- V289: the Ordered Webhook Events (CloudEvents) inbound connector, the
-- durable connector cursor it keeps, and Standard Webhooks signature
-- checking as an authentication choice on the generic HTTP receiver.
--
-- webhook_events_inbound (services/connectors/webhook_events_inbound.go)
-- receives CloudEvents from a sender that delivers strictly in order and
-- waits for the reply (ezIdentitySense's webhooks, for one). It processes a
-- request's events one at a time, answers only after each has finished, and
-- records the last finished event's sequence in connector_cursors so a
-- retried delivery never processes an event twice. Config field names match
-- that file exactly.
--
-- Port 8095 is inside the Docker-mapped HTTP range (docker-compose.yml:
-- "8081-8099:8081-8099").

-- ─────────────────────────────────────────────────────────────────────────────
-- Durable connector cursors (services/connectors/cursor_store.go)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS connector_cursors (
    interface_id  TEXT        NOT NULL,
    cursor_key    TEXT        NOT NULL,
    cursor_value  TEXT        NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (interface_id, cursor_key)
);

COMMENT ON TABLE connector_cursors IS
    'An inbound connector''s durable position in its source (e.g. the last webhook event sequence fully processed), one per interface and key';

-- ─────────────────────────────────────────────────────────────────────────────
-- Connectivity type
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'webhook_events_inbound',
    'inbound',
    'Ordered Webhook Events (CloudEvents)',
    'Receive CloudEvents webhooks from a sender that delivers in order (for example ezIdentitySense). Each event is processed one at a time and the sender is answered only after its events have finished, so nothing is lost or reordered; retries never process an event twice.',
    '🔔',
    'push',
    false,
    true,
    false,
    'WebhookEventsInboundConnector',
    '{
        "type": "object",
        "required": ["port", "endpoint_path"],
        "properties": {
            "port": {"type": "integer", "title": "Listen Port", "minimum": 1, "maximum": 65535, "default": 8095},
            "endpoint_path": {"type": "string", "title": "Endpoint Path", "default": "/events", "description": "The sender posts to https://<this server>:<port><path>"},
            "authentication_type": {"enum": ["standard_webhooks", "bearer_token", "api_key", "basic_auth"], "type": "string", "title": "Authentication", "default": "standard_webhooks", "description": "standard_webhooks checks the sender''s signature on every request"},
            "webhook_secret": {"type": "string", "title": "Signing Secret", "format": "password", "description": "The whsec_... secret the sender showed when the webhook was created"},
            "webhook_tolerance_seconds": {"type": "integer", "title": "Signature Time Tolerance (s)", "default": 300},
            "bearer_token": {"type": "string", "title": "Bearer Token", "format": "password", "description": "Required when Authentication is bearer_token"},
            "api_key_header": {"type": "string", "title": "API Key Header", "default": "X-API-Key"},
            "api_key": {"type": "string", "title": "API Key", "format": "password", "description": "Required when Authentication is api_key"},
            "username": {"type": "string", "title": "Username", "description": "Required when Authentication is basic_auth"},
            "password": {"type": "string", "title": "Password", "format": "password", "description": "Required when Authentication is basic_auth"},
            "sequence_attribute": {"type": "string", "title": "Sequence Attribute", "default": "sequence", "description": "CloudEvents attribute that numbers the events in order"},
            "reply_within_seconds": {"type": "integer", "title": "Reply Within (s)", "default": 8, "description": "Keep below the sender''s delivery timeout (ezIdentitySense: 10 s). Unfinished work is resumed on the sender''s retry."},
            "event_timeout_seconds": {"type": "integer", "title": "Event Timeout (s)", "default": 300},
            "max_body_size_mb": {"type": "integer", "title": "Max Body Size (MB)", "default": 5},
            "cursor_key": {"type": "string", "title": "Cursor Name", "description": "Defaults to the endpoint path"}
        }
    }'::jsonb,
    '{
        "basic": ["port", "endpoint_path"],
        "security": ["authentication_type", "webhook_secret", "webhook_tolerance_seconds", "bearer_token", "api_key_header", "api_key", "username", "password"],
        "advanced": ["sequence_attribute", "reply_within_seconds", "event_timeout_seconds", "max_body_size_mb", "cursor_key"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Protocols',
    40
)
ON CONFLICT (type_name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    is_active = EXCLUDED.is_active,
    ui_category = EXCLUDED.ui_category,
    ui_sort_order = EXCLUDED.ui_sort_order,
    updated_at = NOW();

-- ─────────────────────────────────────────────────────────────────────────────
-- Generic HTTP receiver: offer Standard Webhooks signatures
-- (services/connectors/inbound_auth.go, standard_webhooks)
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE connectivity_types
SET config_schema = jsonb_set(
        jsonb_set(
            config_schema,
            '{properties,authentication_type,enum}',
            '["none","api_key","basic_auth","bearer_token","standard_webhooks"]'::jsonb
        ),
        '{properties,webhook_secret}',
        '{"type":"string","title":"Signing Secret","format":"password","description":"Required when Authentication is standard_webhooks: the sender''s whsec_... secret"}'::jsonb
    ),
    parameter_groups = jsonb_set(
        parameter_groups,
        '{security}',
        (parameter_groups->'security') || '["webhook_secret"]'::jsonb
    ),
    updated_at = NOW()
WHERE type_name IN ('http_rest_inbound', 'http_rest')
  AND config_schema ? 'properties'
  AND NOT (parameter_groups->'security' ? 'webhook_secret');
