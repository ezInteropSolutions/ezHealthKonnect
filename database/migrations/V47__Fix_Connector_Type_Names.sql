-- V47: Fix connector type names from V45 migration
-- V45 stored short names (tcp_mllp, http_rest) but the factory
-- registers full names (tcp_mllp_inbound, http_rest_inbound).
-- Factory aliases handle runtime compatibility, but fix the data too.

UPDATE transformation_steps
SET config = jsonb_set(config, '{connectorType}', '"tcp_mllp_inbound"')
WHERE step_type = 'connector.inbound'
  AND config->>'connectorType' = 'tcp_mllp';

UPDATE transformation_steps
SET config = jsonb_set(config, '{connectorType}', '"http_rest_inbound"')
WHERE step_type = 'connector.inbound'
  AND config->>'connectorType' = 'http_rest';
