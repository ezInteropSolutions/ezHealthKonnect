-- V201: HL7 Build Demo — ADT^A01 and RDE^O11 pipelines
--
-- HL7_Build_Demo (interface 3da767f0-a60c-43b0-ae84-d206c53b5824) shipped
-- with a single ORU^R01 pipeline (plain PID + OBX fields sourced directly
-- from JSON test-message fields — no file_parser, no GroupBy). This
-- migration adds two sibling pipelines on the SAME interface — one message
-- type per pipeline, per the message-type-centric architecture
-- (uq_pipeline_interface_message: UNIQUE(interface_id, message_type)) —
-- demonstrating the file_parser -> hl7.build GroupBy/ChildSegments nesting
-- capability instead: a real CSV parsed by file_parser feeds a multi-level
-- segment tree in hl7.build.
--
-- Step configs are kept byte-for-byte consistent with
-- tests/playwright/fhir-hl7-build-e2e.spec.js's FHB-E2-007 (ADT) and
-- FHB-E2-008 (RDE), and with the underlying Go unit tests they mirror
-- (TestHL7Build_GroupBy_MultipleSiblingChildren_SingleAndRepeatingMixed for
-- ADT's INSURANCE group, TestHL7Build_ThreeLevelChain_ORCOwnsGroupBy_OBRPassesThroughToOBX
-- for RDE's ORC -> RXE -> RXR chain).
--
-- No-ops safely if the HL7_Build_Demo interface doesn't exist in this
-- environment — same convention V198 already established for
-- FHIR_Build_Demo's additional-resource migrations, and for the same
-- reason: interfaces.user_id is NOT NULL, and Flyway migrations run before
-- the Node app's first boot creates its default admin user, so a migration
-- cannot reliably create a brand-new demo interface from nothing on a
-- genuinely fresh install. This migration only ever ADDS to an
-- already-existing HL7_Build_Demo interface.

-- transformation_pipelines.status was never added by any tracked migration —
-- it only exists on already-migrated environments because of undocumented
-- schema drift (a manual ALTER TABLE at some point, outside Flyway). This
-- INSERT (and V202's) has always assumed the column exists; on a genuinely
-- fresh install (Flyway migrating from V1) it never did, breaking the chain
-- here. IF NOT EXISTS makes this a no-op on every environment that already
-- has the column (which Flyway's repair, always run before migrate in this
-- project — see docker-compose.yml's flyway service — reconciles safely) and
-- a real fix on one that doesn't.
ALTER TABLE transformation_pipelines ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'draft';

-- ── ADT^A01 pipeline ─────────────────────────────────────────────────────
INSERT INTO transformation_pipelines (id, interface_id, message_type, pipeline_name, enabled, status, pipeline_config)
SELECT
    gen_random_uuid(), i.id, 'ADT^A01', 'HL7 Build Demo Pipeline (ADT)', true, 'draft', '{}'::jsonb
FROM interfaces i
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND NOT EXISTS (
      SELECT 1 FROM transformation_pipelines tp
      WHERE tp.interface_id = i.id AND tp.message_type = 'ADT^A01'
  );

-- Step 1: Parse CSV — a real CSV of insurance coverage rows
-- (planId,groupNumber,certNumber), one row per coverage.
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Parse CSV', 'file_parser', 10,
    '{
        "sourceField": "insuranceCsv",
        "fileFormat": "csv",
        "hasHeader": true
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'ADT^A01'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Parse CSV');

-- Step 2: Build HL7 Message — PID/PV1 from the message directly (one
-- patient per admit), then the INSURANCE group (IN1 owns GroupBy by
-- plan_id, IN2 single + IN3 repeating certifications as child segments —
-- both draw from the same bucket) built from the parsed CSV records.
-- NOTE: planId/groupNumber/certNumber become plan_id/group_number/cert_number
-- by the time this step reads steps.parse_csv.step_output.records —
-- NormalizeStepOutput snake_cases multi-word keys in transit between steps,
-- including keys nested inside a record array (see FHB-E2-007's comment).
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build HL7 Message', 'hl7.build', 20,
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
                "fields": [
                    {"fieldKey": "PV1.2", "sourcePath": "message.admitType"}
                ]
            },
            {
                "segment": "IN1", "cardinality": "repeating",
                "rowsPath": "steps.parse_csv.step_output.records",
                "groupBy": ["plan_id"],
                "fields": [
                    {"fieldKey": "IN1.2", "sourcePath": "plan_id"}
                ],
                "childSegments": [
                    {
                        "segment": "IN2", "cardinality": "single",
                        "fields": [{"fieldKey": "IN2.1", "sourcePath": "group_number"}]
                    },
                    {
                        "segment": "IN3", "cardinality": "repeating", "rowsPath": "_rows",
                        "fields": [{"fieldKey": "IN3.1", "sourcePath": "cert_number"}]
                    }
                ]
            }
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'ADT^A01'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build HL7 Message');

-- ── RDE^O11 pipeline (pharmacy/treatment encoded order) ─────────────────
INSERT INTO transformation_pipelines (id, interface_id, message_type, pipeline_name, enabled, status, pipeline_config)
SELECT
    gen_random_uuid(), i.id, 'RDE^O11', 'HL7 Build Demo Pipeline (RDE)', true, 'draft', '{}'::jsonb
FROM interfaces i
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND NOT EXISTS (
      SELECT 1 FROM transformation_pipelines tp
      WHERE tp.interface_id = i.id AND tp.message_type = 'RDE^O11'
  );

-- Step 1: Parse CSV — a real CSV of medication orders
-- (orderId,drugCode,drugName,dose,route), one row per order+route.
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Parse CSV', 'file_parser', 10,
    '{
        "sourceField": "medicationCsv",
        "fileFormat": "csv",
        "hasHeader": true
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'RDE^O11'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Parse CSV');

-- Step 2: Build HL7 Message — a 3-level chain: ORC (schema-first, owns the
-- GroupBy bucketing by order_id) -> RXE (single — the medication itself,
-- transparently passing its bucket through) -> RXR (repeating routes; an
-- order can have more than one route, e.g. inhaled + nebulized).
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build HL7 Message', 'hl7.build', 20,
    '{
        "messageType": "RDE", "triggerEvent": "O11", "version": "2.5.1",
        "outputField": "message.hl7Message",
        "segments": [
            {
                "segment": "ORC", "cardinality": "repeating",
                "rowsPath": "steps.parse_csv.step_output.records",
                "groupBy": ["order_id"],
                "fields": [
                    {"fieldKey": "ORC.1", "literalValue": "NW"},
                    {"fieldKey": "ORC.2", "sourcePath": "order_id"}
                ],
                "childSegments": [
                    {
                        "segment": "RXE", "cardinality": "single",
                        "fields": [
                            {"fieldKey": "RXE.2.1", "sourcePath": "drug_code"},
                            {"fieldKey": "RXE.2.2", "sourcePath": "drug_name"},
                            {"fieldKey": "RXE.3", "sourcePath": "dose"}
                        ],
                        "childSegments": [
                            {
                                "segment": "RXR", "cardinality": "repeating", "rowsPath": "_rows",
                                "fields": [{"fieldKey": "RXR.1", "sourcePath": "route"}]
                            }
                        ]
                    }
                ]
            }
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'RDE^O11'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build HL7 Message');
