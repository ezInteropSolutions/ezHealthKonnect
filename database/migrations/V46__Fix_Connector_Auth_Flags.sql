-- V46: Fix requires_auth flags on connectivity_types
-- Only HTTP-based connectors should require OAuth2 authentication.
-- Other connectors (TCP, file, database, MQ, cloud) have their own auth
-- mechanisms defined in their config_schema (e.g., enable_authentication).

UPDATE connectivity_types SET requires_auth = false
WHERE type_name NOT LIKE '%http%'
  AND type_name NOT LIKE '%fhir%';
