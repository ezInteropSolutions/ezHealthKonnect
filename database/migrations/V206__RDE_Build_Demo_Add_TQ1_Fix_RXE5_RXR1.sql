-- V206: RDE_Build_Demo (RDE^O11) — add missing TQ1 segment, populate RXE.5
--
-- The hl7/validator conformance checks surfaced two real gaps in this
-- demo's hl7.build config, verified against the real compiled v2.5.1
-- RDE_O11 schema:
--   1. TQ1 was never configured at all. builder.RequiredSpine's algorithm
--      (transitive-AND down the full ancestor chain) makes TQ1 required for
--      RDE_O11: it sits under the "TIMING ENCODED" group, which -- unlike
--      the sibling PATIENT/ORDER DETAIL groups (all usage=O, which is why
--      PID/PV1/IN1/RXO/the ORDER-DETAIL-nested RXR are correctly NOT
--      required) -- is itself usage=R directly under the required ORDER
--      group. No field inside TQ1 is individually required, so a minimal
--      TQ1 with just a literal Set ID satisfies both the segment-presence
--      check and looks realistic.
--   2. RXE.5 (Give Units, usage=R) had no source at all -- added a new CSV
--      column "dose_unit" mapped onto RXE.5.1, the same "populate via a
--      subcomponent" pattern RXE.2.1/RXE.2.2 already used successfully for
--      Give Code.
-- RXR.1 (Route) was already mapped from the CSV's "route" column and did
-- not need a config change -- table 0162 code correctness is a test-data
-- concern (see the payload examples in fhir-hl7-build-e2e.spec.js/chat, not
-- schema data), not something this migration needs to touch.
--
-- Test payloads for this pipeline must now include a CSV with columns:
-- orderId, drugCode, drugName, dose, doseUnit, route.
--
-- No-ops safely if RDE_Build_Demo/its RDE^O11 hl7.build step doesn't exist,
-- same convention as V198/V201-V205.

UPDATE transformation_steps ts
SET config = '{
    "messageType": "RDE", "triggerEvent": "O11", "version": "2.5.1",
    "outputField": "message.hl7Message",
    "segments": [{
        "segment": "ORC", "cardinality": "repeating",
        "rowsPath": "steps.parse_csv.step_output.records",
        "groupBy": ["order_id"],
        "fields": [
            {"fieldKey": "ORC.1", "sourcePath": "", "literalValue": "NW"},
            {"fieldKey": "ORC.2", "sourcePath": "order_id"}
        ],
        "childSegments": [
            {
                "segment": "RXE", "cardinality": "single",
                "fields": [
                    {"fieldKey": "RXE.2.1", "sourcePath": "drug_code"},
                    {"fieldKey": "RXE.2.2", "sourcePath": "drug_name"},
                    {"fieldKey": "RXE.3", "sourcePath": "dose"},
                    {"fieldKey": "RXE.5.1", "sourcePath": "dose_unit"}
                ],
                "childSegments": [{
                    "segment": "RXR", "cardinality": "repeating", "rowsPath": "_rows",
                    "fields": [{"fieldKey": "RXR.1", "sourcePath": "route"}]
                }]
            },
            {
                "segment": "TQ1", "cardinality": "single",
                "fields": [{"fieldKey": "TQ1.1", "sourcePath": "", "literalValue": "1"}]
            }
        ]
    }]
}'::jsonb
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.name = 'RDE_Build_Demo'
  AND tp.message_type = 'RDE^O11'
  AND ts.step_type = 'hl7.build';
