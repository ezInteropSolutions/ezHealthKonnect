-- V293: the FHIR Subscription (topic-based) inbound connector.
--
-- fhir_subscription_inbound (services/connectors/fhir_subscription_inbound.go)
-- subscribes to topics on a FHIR server implementing HL7 Subscriptions R5
-- Backport (topic-based subscriptions on FHIR R4; ezIdentitySense, for one)
-- and processes its numbered notifications in order, one event at a time,
-- answering only once each has finished and filling any gap from the
-- server's $events. It manages its subscriptions itself. Config field names
-- match that file exactly.
--
-- Port 8089 is inside the Docker-mapped HTTP range (docker-compose.yml:
-- "8081-8099:8081-8099") and outside the ports the connector forms reserve
-- (8081, 8090-8097).

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'fhir_subscription_inbound',
    'inbound',
    'FHIR Subscription (topic-based)',
    'Subscribe to topics on a FHIR server (HL7 topic-based Subscriptions, for example ezIdentitySense) and process its notifications in order. The subscriptions are made and renewed for you; each event is processed one at a time, nothing is lost or repeated, and missed events are fetched again.',
    '📡',
    'push',
    false,
    true,
    false,
    'FHIRSubscriptionInboundConnector',
    '{
        "type": "object",
        "required": ["port", "callback_url", "fhir_base_url", "topics"],
        "properties": {
            "port": {"type": "integer", "title": "Listen Port", "minimum": 1, "maximum": 65535, "default": 8089},
            "endpoint_path": {"type": "string", "title": "Endpoint Path", "default": "/fhir-notifications"},
            "callback_url": {"type": "string", "title": "Address the Server Reaches This At", "description": "The full https address the FHIR server sends notifications to, e.g. https://ezhealthkonnect.example.org:8089/fhir-notifications"},
            "fhir_base_url": {"type": "string", "title": "FHIR Server Base", "description": "e.g. https://empi.example.org/fhir/r4"},
            "topics": {"type": "array", "items": {"type": "string"}, "title": "Topics", "description": "The topic URLs to subscribe to"},
            "filters": {"type": "array", "items": {"type": "string"}, "title": "Filters", "description": "Filter criteria for every subscription, as name=value (e.g. source-system=EHR)"},
            "content": {"enum": ["full-resource", "id-only", "empty"], "type": "string", "title": "Notification Content", "default": "full-resource"},
            "heartbeat_seconds": {"type": "integer", "title": "Heartbeat (s)", "default": 0, "description": "Ask for a heartbeat after this long without events; 0 for none"},
            "auth_type": {"enum": ["oauth2", "bearer_token"], "type": "string", "title": "Authentication", "default": "oauth2"},
            "oauth2_token_url": {"type": "string", "title": "Token URL"},
            "oauth2_client_id": {"type": "string", "title": "Client ID"},
            "oauth2_client_secret": {"type": "string", "title": "Client Secret", "format": "password"},
            "oauth2_scope": {"type": "string", "title": "Scope", "default": "identities.subscribe identities.read identities.read.demographics"},
            "oauth2_audience": {"type": "string", "title": "Audience"},
            "bearer_token": {"type": "string", "title": "Bearer Token", "format": "password", "description": "Required when Authentication is bearer_token"},
            "renew_hours": {"type": "integer", "title": "Renew Every (hours)", "default": 12},
            "reply_within_seconds": {"type": "integer", "title": "Reply Within (s)", "default": 8, "description": "Keep below the server''s delivery timeout (ezIdentitySense: 10 s). Unfinished work is resumed on its retry."},
            "event_timeout_seconds": {"type": "integer", "title": "Event Timeout (s)", "default": 300},
            "max_body_size_mb": {"type": "integer", "title": "Max Body Size (MB)", "default": 10}
        }
    }'::jsonb,
    '{
        "basic": ["port", "endpoint_path", "callback_url", "fhir_base_url", "topics", "filters", "content"],
        "security": ["auth_type", "oauth2_token_url", "oauth2_client_id", "oauth2_client_secret", "oauth2_scope", "oauth2_audience", "bearer_token"],
        "advanced": ["heartbeat_seconds", "renew_hours", "reply_within_seconds", "event_timeout_seconds", "max_body_size_mb"]
    }'::jsonb,
    true,
    100,
    '1.0',
    'Healthcare Protocols',
    41
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
