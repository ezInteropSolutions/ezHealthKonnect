-- V204: HL7_Build_Demo (ORU^R01) — populate OBR.4 (Universal Service ID)
--
-- V203 added the OBR segment itself, but left OBR.4 empty. The existing
-- required-field check (hl7.validateRequiredFields, always active regardless
-- of validationLevel) correctly flags OBR.4 as a required-but-empty field
-- once OBR is actually present. This demo has no per-panel source data, so a
-- literal value is used — same "code^display^system" shape as the OBR.4
-- sample already shown in hl7-reader.html's own built-in ORU^R01 example
-- ("85025^CBC^LN").
--
-- No-ops safely if HL7_Build_Demo/its ORU^R01 hl7.build step doesn't exist,
-- same convention as V198/V201/V202/V203.

UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{segments,1,fields}',
    (ts.config #> '{segments,1,fields}') || '[{"fieldKey": "OBR.4", "sourcePath": "", "literalValue": "85025^CBC PANEL^LN"}]'::jsonb
)
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.id = '3da767f0-a60c-43b0-ae84-d206c53b5824'
  AND tp.message_type = 'ORU^R01'
  AND ts.step_type = 'hl7.build'
  AND ts.config #>> '{segments,1,segment}' = 'OBR';
