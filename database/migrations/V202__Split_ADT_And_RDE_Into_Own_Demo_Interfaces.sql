-- V202: Split ADT^A01 and RDE^O11 out of HL7_Build_Demo into their own interfaces
--
-- V201 added ADT^A01 and RDE^O11 as two more message-type-scoped pipelines
-- on the EXISTING HL7_Build_Demo interface (the message-type-centric model:
-- one interface, one pipeline per message type). User feedback: that's not
-- what was wanted here — each message type should be its own interface, not
-- share one. This migration undoes that sharing:
--   1. Creates two new interfaces, ADT_Build_Demo and RDE_Build_Demo, each
--      cloning HL7_Build_Demo's connectivity/deployment settings (same
--      pattern, different port so a real activation of all three would
--      never collide) but with exactly one pipeline each.
--   2. Removes the ADT^A01 and RDE^O11 pipelines (and their steps, via
--      transformation_steps' ON DELETE CASCADE) from HL7_Build_Demo, so it
--      reverts to ORU^R01-only — its original shape before V201.
--
-- Step configs are unchanged from V201 (byte-for-byte identical to
-- FHB-E2-007/FHB-E2-008 in tests/playwright/fhir-hl7-build-e2e.spec.js) —
-- only WHICH interface they live under has changed.
--
-- No-ops safely if HL7_Build_Demo doesn't exist in this environment — same
-- convention V198/V201 already established (interfaces.user_id is NOT NULL,
-- and Flyway migrations run before the Node app's first boot creates its
-- default admin user, so a migration can't reliably create a demo interface
-- from nothing on a genuinely fresh install; this one clones an existing
-- interface's user_id, so it can only run where one already exists).

-- ── ADT_Build_Demo interface + pipeline ──────────────────────────────────
INSERT INTO interfaces (
    id, user_id, name, description, source_type, target_type, message_type,
    source_config, target_config, status, interface_status, deployment_mode,
    auto_start, log_level, debug_logging, fhir_validation_policy
)
SELECT
    gen_random_uuid(), src.user_id, 'ADT_Build_Demo',
    'Generates an HL7 v2 ADT^A01 message via hl7.build (PID/PV1 + a CSV-driven INSURANCE group)',
    src.source_type, src.target_type, 'hl7v2',
    jsonb_set(src.source_config, '{port}', '8892'::jsonb),
    src.target_config, src.status, src.interface_status, src.deployment_mode,
    src.auto_start, src.log_level, src.debug_logging, src.fhir_validation_policy
FROM interfaces src
WHERE src.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND NOT EXISTS (SELECT 1 FROM interfaces WHERE name = 'ADT_Build_Demo');

INSERT INTO transformation_pipelines (id, interface_id, message_type, pipeline_name, enabled, status, pipeline_config)
SELECT gen_random_uuid(), i.id, 'ADT^A01', 'ADT Build Demo Pipeline', true, 'draft', '{}'::jsonb
FROM interfaces i
WHERE i.name = 'ADT_Build_Demo'
  AND NOT EXISTS (SELECT 1 FROM transformation_pipelines tp WHERE tp.interface_id = i.id AND tp.message_type = 'ADT^A01');

INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT gen_random_uuid(), tp.id, 'Parse CSV', 'file_parser', 10,
    '{"sourceField": "insuranceCsv", "fileFormat": "csv", "hasHeader": true}'::jsonb, true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.name = 'ADT_Build_Demo' AND tp.message_type = 'ADT^A01'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Parse CSV');

INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT gen_random_uuid(), tp.id, 'Build HL7 Message', 'hl7.build', 20,
    '{
        "messageType": "ADT", "triggerEvent": "A01", "version": "2.5.1",
        "outputField": "message.hl7Message",
        "segments": [
            {
                "segment": "PID", "cardinality": "single",
                "fields": [
                    {"fieldKey": "PID.3", "sourcePath": "message.mrn"},
                    {"fieldKey": "PID.5.1", "sourcePath": "message.lastName"},
                    {"fieldKey": "PID.5.2", "sourcePath": "message.firstName"}
                ]
            },
            {
                "segment": "PV1", "cardinality": "single",
                "fields": [{"fieldKey": "PV1.2", "sourcePath": "message.admitType"}]
            },
            {
                "segment": "IN1", "cardinality": "repeating",
                "rowsPath": "steps.parse_csv.step_output.records",
                "groupBy": ["plan_id"],
                "fields": [{"fieldKey": "IN1.2", "sourcePath": "plan_id"}],
                "childSegments": [
                    {"segment": "IN2", "cardinality": "single",
                     "fields": [{"fieldKey": "IN2.1", "sourcePath": "group_number"}]},
                    {"segment": "IN3", "cardinality": "repeating", "rowsPath": "_rows",
                     "fields": [{"fieldKey": "IN3.1", "sourcePath": "cert_number"}]}
                ]
            }
        ]
    }'::jsonb, true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.name = 'ADT_Build_Demo' AND tp.message_type = 'ADT^A01'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build HL7 Message');

-- ── RDE_Build_Demo interface + pipeline ──────────────────────────────────
INSERT INTO interfaces (
    id, user_id, name, description, source_type, target_type, message_type,
    source_config, target_config, status, interface_status, deployment_mode,
    auto_start, log_level, debug_logging, fhir_validation_policy
)
SELECT
    gen_random_uuid(), src.user_id, 'RDE_Build_Demo',
    'Generates an HL7 v2 RDE^O11 medication order via hl7.build (ORC -> RXE -> RXR 3-level chain)',
    src.source_type, src.target_type, 'hl7v2',
    jsonb_set(src.source_config, '{port}', '8893'::jsonb),
    src.target_config, src.status, src.interface_status, src.deployment_mode,
    src.auto_start, src.log_level, src.debug_logging, src.fhir_validation_policy
FROM interfaces src
WHERE src.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND NOT EXISTS (SELECT 1 FROM interfaces WHERE name = 'RDE_Build_Demo');

INSERT INTO transformation_pipelines (id, interface_id, message_type, pipeline_name, enabled, status, pipeline_config)
SELECT gen_random_uuid(), i.id, 'RDE^O11', 'RDE Build Demo Pipeline', true, 'draft', '{}'::jsonb
FROM interfaces i
WHERE i.name = 'RDE_Build_Demo'
  AND NOT EXISTS (SELECT 1 FROM transformation_pipelines tp WHERE tp.interface_id = i.id AND tp.message_type = 'RDE^O11');

INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT gen_random_uuid(), tp.id, 'Parse CSV', 'file_parser', 10,
    '{"sourceField": "medicationCsv", "fileFormat": "csv", "hasHeader": true}'::jsonb, true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.name = 'RDE_Build_Demo' AND tp.message_type = 'RDE^O11'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Parse CSV');

INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT gen_random_uuid(), tp.id, 'Build HL7 Message', 'hl7.build', 20,
    '{
        "messageType": "RDE", "triggerEvent": "O11", "version": "2.5.1",
        "outputField": "message.hl7Message",
        "segments": [{
            "segment": "ORC", "cardinality": "repeating",
            "rowsPath": "steps.parse_csv.step_output.records",
            "groupBy": ["order_id"],
            "fields": [
                {"fieldKey": "ORC.1", "literalValue": "NW"},
                {"fieldKey": "ORC.2", "sourcePath": "order_id"}
            ],
            "childSegments": [{
                "segment": "RXE", "cardinality": "single",
                "fields": [
                    {"fieldKey": "RXE.2.1", "sourcePath": "drug_code"},
                    {"fieldKey": "RXE.2.2", "sourcePath": "drug_name"},
                    {"fieldKey": "RXE.3", "sourcePath": "dose"}
                ],
                "childSegments": [{
                    "segment": "RXR", "cardinality": "repeating", "rowsPath": "_rows",
                    "fields": [{"fieldKey": "RXR.1", "sourcePath": "route"}]
                }]
            }]
        }]
    }'::jsonb, true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.name = 'RDE_Build_Demo' AND tp.message_type = 'RDE^O11'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build HL7 Message');

-- ── Revert HL7_Build_Demo to ORU^R01-only (its original shape before V201) ──
-- transformation_steps.pipeline_id has ON DELETE CASCADE, so each pipeline's
-- steps are removed automatically. No transformation_executions rows exist
-- for these pipelines (only ever run through the ad-hoc Test Pipeline API,
-- which doesn't persist execution history), so this can't hit the NO ACTION
-- FK on transformation_executions.pipeline_id.
DELETE FROM transformation_pipelines
WHERE interface_id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND message_type IN ('ADT^A01', 'RDE^O11');
