-- V205: ADT_Build_Demo (ADT^A01) — add missing EVN segment, fix IN1/IN3 field mappings
--
-- The hl7/validator conformance checks (hl7-reader.html) surfaced three real
-- gaps in this demo's hl7.build config, verified against the real compiled
-- v2.5.1 ADT_A01 schema:
--   1. EVN segment was never configured at all (builder.RequiredSpine
--      returns EVN/PID/PV1 for ADT_A01 -- EVN was silently missing).
--      EVN.2 (Recorded Date/Time) is the segment's one truly required field
--      (usage=R); EVN.1 (Event Type Code) is usage=B (backward-compatible,
--      not required) but is populated too for a realistic message, using a
--      real code from its bound table 0003.
--   2. IN1.3 (Insurance Company ID) is required (usage=R) but had no source
--      -- added a new CSV column "company_id".
--   3. IN3.1 ("Set ID - IN3", data type SI/numeric) was wrongly mapped to
--      cert_number (a string like "CERT1001"), tripping the new data-type
--      format check. The certificate number actually belongs on IN3.2
--      ("Certification Number", CX). Added two new CSV columns -- "set_id"
--      (IN1's own required Set ID, one value per plan_id group) and
--      "in3_seq" (IN3's required Set ID, sequential within its parent IN1) --
--      since HL7 Set ID fields must come from real source data; there is no
--      auto-increment mechanism in hl7.build's row/bucket resolution.
--
-- Test payloads for this pipeline must now include a CSV with columns:
-- set_id, plan_id, company_id, group_number, in3_seq, cert_number.
--
-- No-ops safely if ADT_Build_Demo/its ADT^A01 hl7.build step doesn't exist,
-- same convention as V198/V201-V204.

UPDATE transformation_steps ts
SET config = '{
    "messageType": "ADT", "triggerEvent": "A01", "version": "2.5.1",
    "outputField": "message.hl7Message",
    "segments": [
        {
            "segment": "EVN", "cardinality": "single",
            "fields": [
                {"fieldKey": "EVN.1", "sourcePath": "", "literalValue": "A01"},
                {"fieldKey": "EVN.2", "sourcePath": "", "literalValue": "20260101120000"}
            ]
        },
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
            "fields": [
                {"fieldKey": "IN1.1", "sourcePath": "set_id"},
                {"fieldKey": "IN1.2", "sourcePath": "plan_id"},
                {"fieldKey": "IN1.3", "sourcePath": "company_id"}
            ],
            "childSegments": [
                {"segment": "IN2", "cardinality": "single",
                 "fields": [{"fieldKey": "IN2.1", "sourcePath": "group_number"}]},
                {"segment": "IN3", "cardinality": "repeating", "rowsPath": "_rows",
                 "fields": [
                    {"fieldKey": "IN3.1", "sourcePath": "in3_seq"},
                    {"fieldKey": "IN3.2", "sourcePath": "cert_number"}
                 ]}
            ]
        }
    ]
}'::jsonb
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.name = 'ADT_Build_Demo'
  AND tp.message_type = 'ADT^A01'
  AND ts.step_type = 'hl7.build';
