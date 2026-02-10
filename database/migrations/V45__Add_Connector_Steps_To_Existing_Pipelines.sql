-- V45: Add Connector Steps to Existing Pipelines
-- Adds inbound and outbound connector steps to existing pipelines based on
-- the interface's source_connectivity and target_connectivity JSONB fields.
-- Format: {"type": "tcp", "config": {"host": "localhost", "port": 6667}}

-- Add inbound connector steps for existing pipelines that have source connectivity
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(),
    tp.id,
    CASE i.source_connectivity->>'type'
        WHEN 'tcp' THEN 'TCP/MLLP Inbound'
        WHEN 'http' THEN 'HTTP REST Inbound'
        WHEN 'file' THEN 'File Listener'
        WHEN 'database' THEN 'Database Inbound'
        ELSE 'Inbound Connector'
    END,
    'connector.inbound',
    5,
    jsonb_build_object(
        'connectorType',
        CASE i.source_connectivity->>'type'
            WHEN 'tcp' THEN 'tcp_mllp'
            WHEN 'http' THEN 'http_rest'
            WHEN 'file' THEN 'file_listener'
            WHEN 'database' THEN 'postgresql_inbound'
            ELSE COALESCE(i.source_connectivity->>'type', 'unknown') || '_inbound'
        END,
        'config', COALESCE(i.source_connectivity->'config', '{}'::jsonb),
        'outputField', 'enriched.connector_result',
        'timeoutMs', 30000
    ),
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.source_connectivity IS NOT NULL
  AND i.source_connectivity != '{}'::jsonb
  AND i.source_connectivity->>'type' IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM transformation_steps ts
    WHERE ts.pipeline_id = tp.id AND ts.step_type = 'connector.inbound'
  );

-- Add outbound connector steps for existing pipelines that have target connectivity
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(),
    tp.id,
    CASE i.target_connectivity->>'type'
        WHEN 'http' THEN 'HTTP Outbound'
        WHEN 'fhir' THEN 'FHIR Outbound'
        WHEN 'tcp' THEN 'TCP/MLLP Outbound'
        WHEN 'file' THEN 'File Writer'
        WHEN 'database' THEN 'Database Outbound'
        ELSE 'Outbound Connector'
    END,
    'connector.outbound',
    295,
    jsonb_build_object(
        'connectorType',
        CASE i.target_connectivity->>'type'
            WHEN 'http' THEN 'http_outbound'
            WHEN 'fhir' THEN 'http_outbound'
            WHEN 'tcp' THEN 'tcp_mllp_outbound'
            WHEN 'file' THEN 'file_writer'
            WHEN 'database' THEN 'postgresql_outbound'
            ELSE COALESCE(i.target_connectivity->>'type', 'unknown') || '_outbound'
        END,
        'config', COALESCE(i.target_connectivity->'config', '{}'::jsonb),
        'contentField', 'transformed',
        'contentType', 'application/json'
    ),
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.target_connectivity IS NOT NULL
  AND i.target_connectivity != '{}'::jsonb
  AND i.target_connectivity->>'type' IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM transformation_steps ts
    WHERE ts.pipeline_id = tp.id AND ts.step_type = 'connector.outbound'
  );

-- Also handle interfaces that have interface_connectivity records (Phase 2 connector setup)
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(),
    tp.id,
    'Inbound: ' || ct.display_name,
    'connector.inbound',
    5,
    jsonb_build_object(
        'connectorType', ct.type_name,
        'config', COALESCE(ic.source_config, '{}'::jsonb),
        'outputField', 'enriched.connector_result',
        'timeoutMs', 30000
    ),
    ic.source_enabled
FROM transformation_pipelines tp
JOIN interface_connectivity ic ON ic.interface_id = tp.interface_id
JOIN connectivity_types ct ON ct.id = ic.source_connectivity_type_id
WHERE ic.source_enabled = true
  AND NOT EXISTS (
    SELECT 1 FROM transformation_steps ts
    WHERE ts.pipeline_id = tp.id AND ts.step_type = 'connector.inbound'
  );

INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(),
    tp.id,
    'Outbound: ' || ct.display_name,
    'connector.outbound',
    295,
    jsonb_build_object(
        'connectorType', ct.type_name,
        'config', COALESCE(ic.target_config, '{}'::jsonb),
        'contentField', 'transformed',
        'contentType', 'application/json'
    ),
    ic.target_enabled
FROM transformation_pipelines tp
JOIN interface_connectivity ic ON ic.interface_id = tp.interface_id
JOIN connectivity_types ct ON ct.id = ic.target_connectivity_type_id
WHERE ic.target_enabled = true
  AND NOT EXISTS (
    SELECT 1 FROM transformation_steps ts
    WHERE ts.pipeline_id = tp.id AND ts.step_type = 'connector.outbound'
  );
