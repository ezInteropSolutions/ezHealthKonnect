-- V203: HL7_Build_Demo (ORU^R01) — nest OBX under a proper OBR segment
--
-- The demo's hl7.build step config predates the OBR-owns-OBX grouping
-- capability (childSegments) added later in the same work — it only ever
-- configured PID + a top-level repeating OBX, with no OBR at all. The new
-- hl7/validator conformance checks (hl7-reader.html) correctly flag this:
-- OBR is the one segment builder.RequiredSpine returns for ORU_R01 (OBX
-- results structurally belong under an order), so a message built without
-- one isn't a conformant ORU^R01.
--
-- Fix: add a single OBR segment (Set ID literal "1" — this demo has no
-- per-order data, just one flat lab-results array) and move the existing
-- OBX mapping under it as a childSegment, unchanged otherwise. Same pattern
-- already proven by the CBC/CMP and INSURANCE childSegments tests in
-- hl7_build_executor_test.go / fhir-hl7-build-e2e.spec.js.
--
-- No-ops safely if HL7_Build_Demo/its ORU^R01 hl7.build step doesn't exist
-- in this environment, same convention as V198/V201/V202.

UPDATE transformation_steps ts
SET config = '{
    "messageType": "ORU", "triggerEvent": "R01", "version": "2.5.1",
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
            "segment": "OBR", "cardinality": "single",
            "fields": [
                {"fieldKey": "OBR.1", "sourcePath": "", "literalValue": "1"}
            ],
            "childSegments": [{
                "segment": "OBX", "cardinality": "repeating",
                "rowsPath": "message.labResults",
                "fields": [
                    {"fieldKey": "OBX.3", "sourcePath": "testCode"},
                    {"fieldKey": "OBX.5", "sourcePath": "value"},
                    {"fieldKey": "OBX.11", "sourcePath": "", "literalValue": "F"}
                ]
            }]
        }
    ]
}'::jsonb
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'ORU^R01'
  AND ts.step_type = 'hl7.build';
