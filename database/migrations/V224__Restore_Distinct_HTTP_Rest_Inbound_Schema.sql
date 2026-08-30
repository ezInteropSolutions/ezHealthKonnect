-- V224: Restore http_rest/http_rest_inbound as a genuinely distinct connector
-- Applied: 2026-08-30
--
-- V222 (this session, "Fix Inbound Connector Config Schema Mismatches")
-- overwrote http_rest's config_schema to be identical to http_fhir_inbound's,
-- reasoning that since both names were registered to the same Go connector
-- (HTTPFHIRInboundConnector) they should share a schema. This was wrong: the
-- user confirmed HTTP FHIR Receiver and HTTP/REST API were always meant to be
-- two separate features with two separate settings screens -- FHIR-specific
-- pre-configuration (resource routing, Bundle handling, FHIR versioning) vs a
-- plain generic HTTP intake. The real bug was that http_rest_inbound never
-- had its own connector implementation at all -- it was silently aliased to
-- the FHIR one. services/connectors/http_rest_inbound.go (this session) is
-- the real, separate implementation; connector_factory.go now registers it
-- under http_rest_inbound/http_rest instead of the FHIR connector.
--
-- This migration restores http_rest's own schema -- close to what it was
-- BEFORE V222's mistaken overwrite (endpoint_path, http_methods, content_type,
-- authentication_type, api_key_header), but fixes a real, separate,
-- pre-existing gap found while restoring it: the original schema never had
-- fields to actually HOLD the credential values for the auth modes it
-- offered (api_key, username/password, bearer_token) -- only the header NAME
-- for api_key, nothing else. Selecting "basic_auth" or "bearer_token" in the
-- old schema gave the user nowhere to type the actual secret. Also added
-- "port" (the connector runs its own dedicated listener, same pattern as
-- every other inbound HTTP-based connector in this system) and
-- "max_body_size_mb", both real fields the new Go connector reads.
--
-- Confirmed zero real usage of http_rest_inbound/http_rest before this fix
-- (same check as V222); the one real interface using the generic legacy
-- "http" label is independently confirmed (via its own nested
-- connectorType field) to actually be a FHIR receiver, unaffected by this
-- change since "http" itself still resolves to the FHIR connector.

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["port","endpoint_path"],"properties":{"port":{"type":"integer","title":"Listen Port","minimum":1,"maximum":65535},"endpoint_path":{"type":"string","title":"Endpoint Path","default":"/api/hl7/receive"},"http_methods":{"type":"array","items":{"enum":["POST","PUT"],"type":"string"},"title":"HTTP Methods","default":["POST"]},"content_type":{"type":"string","title":"Fallback Content-Type","default":"text/plain","description":"Used only when the incoming request has no Content-Type header"},"max_body_size_mb":{"type":"integer","title":"Max Body Size (MB)","default":10},"authentication_type":{"enum":["none","api_key","basic_auth","bearer_token"],"type":"string","title":"Authentication","default":"api_key"},"api_key_header":{"type":"string","title":"API Key Header","default":"X-API-Key"},"api_key":{"type":"string","title":"API Key","format":"password","description":"Required when Authentication is api_key"},"username":{"type":"string","title":"Username","description":"Required when Authentication is basic_auth"},"password":{"type":"string","title":"Password","format":"password","description":"Required when Authentication is basic_auth"},"bearer_token":{"type":"string","title":"Bearer Token","format":"password","description":"Required when Authentication is bearer_token"}}}'::jsonb,
    parameter_groups = '{"basic":["port","endpoint_path","http_methods"],"advanced":["content_type","max_body_size_mb"],"security":["authentication_type","api_key_header","api_key","username","password","bearer_token"]}'::jsonb,
    display_name = 'HTTP/REST API Receiver',
    description = 'Generic HTTP intake -- accepts requests at a configured path/method(s) with simple authentication, no FHIR-specific parsing',
    updated_at = NOW()
WHERE type_name = 'http_rest';

UPDATE connectivity_types SET
    config_schema = '{"type":"object","required":["port","endpoint_path"],"properties":{"port":{"type":"integer","title":"Listen Port","minimum":1,"maximum":65535},"endpoint_path":{"type":"string","title":"Endpoint Path","default":"/api/hl7/receive"},"http_methods":{"type":"array","items":{"enum":["POST","PUT"],"type":"string"},"title":"HTTP Methods","default":["POST"]},"content_type":{"type":"string","title":"Fallback Content-Type","default":"text/plain","description":"Used only when the incoming request has no Content-Type header"},"max_body_size_mb":{"type":"integer","title":"Max Body Size (MB)","default":10},"authentication_type":{"enum":["none","api_key","basic_auth","bearer_token"],"type":"string","title":"Authentication","default":"api_key"},"api_key_header":{"type":"string","title":"API Key Header","default":"X-API-Key"},"api_key":{"type":"string","title":"API Key","format":"password","description":"Required when Authentication is api_key"},"username":{"type":"string","title":"Username","description":"Required when Authentication is basic_auth"},"password":{"type":"string","title":"Password","format":"password","description":"Required when Authentication is basic_auth"},"bearer_token":{"type":"string","title":"Bearer Token","format":"password","description":"Required when Authentication is bearer_token"}}}'::jsonb,
    parameter_groups = '{"basic":["port","endpoint_path","http_methods"],"advanced":["content_type","max_body_size_mb"],"security":["authentication_type","api_key_header","api_key","username","password","bearer_token"]}'::jsonb,
    display_name = 'HTTP/REST API Receiver',
    description = 'Generic HTTP intake -- accepts requests at a configured path/method(s) with simple authentication, no FHIR-specific parsing'
WHERE type_name = 'http_rest_inbound';
