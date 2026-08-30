-- V219: Fix azure_blob_outbound config_schema/parameter_groups mismatch
-- Applied: 2026-08-29
--
-- azure_blob_outbound's connectivity_types row predates this session's real
-- Go implementation (services/connectors/azure_blob_outbound.go,
-- AzureBlobOutboundConfig struct) -- it was written back when the connector
-- was still a stub, so its field names never got checked against real Go
-- code. Same class of bug V216 fixed for 8 other connectors, just not caught
-- there because azure_blob_outbound had no real implementation to compare
-- against at the time. Confirmed zero rows use azure_blob_outbound in
-- transformation_steps (SELECT count(*) FROM transformation_steps WHERE
-- config->>'connectorType' = 'azure_blob_outbound' -> 0), so this is safe to
-- fix directly.
--
-- Fixes:
--   container_name      -> container       (Go: AzureBlobOutboundConfig.Container)
--   blob_name_pattern    -> key_pattern      (Go: KeyPattern)
--   access_tier enum     Hot/Cool/Archive -> hot/cool/archive (Go lowercases
--                         before comparing, so casing wasn't actually broken,
--                         but matching the real accepted values avoids
--                         confusion)
--   prefix                REMOVED -- not read by the Go connector at all; the
--                         key_pattern field already covers the same use case
--                         (a caller can just prepend a path segment), so this
--                         was a silent no-op field, not a real gap
--   enable_https          REMOVED -- not read by the Go connector; HTTP vs
--                         HTTPS is implied by connection_string/endpoint
--   endpoint              ADDED -- Go reads config.Endpoint to override the
--                         service URL (e.g. Azurite or a non-public-cloud
--                         endpoint) -- this field genuinely didn't exist
--                         before and had no way to be set via the UI
--
-- required is just ["container"] -- the connection_string vs
-- account_name+account_key OR-requirement isn't expressible in this flat
-- schema style; it's already enforced by the real Go Initialize()/Validate()
-- at dry-run time, same precedent as the Databricks (V217) and Snowflake
-- (V218) schemas added this session.

UPDATE connectivity_types
SET
    config_schema = '{"type":"object","required":["container"],"properties":{"container":{"type":"string","title":"Container Name"},"connection_string":{"type":"string","title":"Connection String","format":"password","description":"Full Azure Storage connection string -- alternative to account_name + account_key"},"account_name":{"type":"string","title":"Storage Account Name"},"account_key":{"type":"string","title":"Account Key","format":"password"},"endpoint":{"type":"string","title":"Custom Endpoint","description":"Override the default service URL, e.g. for Azurite or a non-public-cloud endpoint"},"key_pattern":{"type":"string","title":"Blob Name Pattern","default":"{message_id}.json","description":"Placeholders: {message_id} {interface_id} {timestamp} {date} {time}"},"content_type":{"type":"string","title":"Content Type Override"},"access_tier":{"enum":["hot","cool","archive"],"type":"string","title":"Access Tier"}}}'::jsonb,
    parameter_groups = '{"basic":["container","key_pattern","content_type"],"authentication":["connection_string","account_name","account_key","endpoint"],"storage":["access_tier"]}'::jsonb,
    updated_at = NOW()
WHERE type_name = 'azure_blob_outbound';
