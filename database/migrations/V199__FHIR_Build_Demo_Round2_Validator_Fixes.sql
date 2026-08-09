-- V199: FHIR Build Demo — Round 2 external-validator fixes
--
-- The user ran round 1's assembled Bundle through the official HL7 FHIR
-- validator. Findings, cross-checked against the actual generated JSON:
--
-- 1. REAL CODE BUG (fixed in services/executors/payload/payload_builder_executor.go,
--    not this migration): no Bundle entry ever got a fullUrl, which both
--    violates "each entry SHALL have a fullUrl" directly and makes every
--    relative "ResourceType/id" reference elsewhere in the Bundle
--    unresolvable per FHIR's Bundle reference-resolution rules.
-- 2. REAL CONFIG GAP (fixed here): the Observation's LOINC code (8310-5,
--    Body Temperature) auto-selects FHIR's built-in vital-signs "bodytemp"
--    profile, which requires a category (VSCat slice), effective[x], and
--    valueQuantity.system/code — none of which round 1's step config set.
-- 3. TEST-DATA ISSUE (fixed in tests/fixtures/fhir_build_demo_sample_message.json,
--    not this migration): Patient.identifier[0].system used
--    "http://hospital.example.org/mrn" — an IANA documentation-reserved
--    domain external validators explicitly reject. Swapped to a urn:oid
--    identifier system, a more realistic MRN-system representation anyway.
-- 4. EXTERNAL-TOOL-LIMITED, not fixable from our side: java.net.SocketTimeoutException
--    on every coded field — the validator's own terminology server timed out
--    looking up our (correct) SNOMED/LOINC/RxNorm/CVX codes. Same category as
--    the CDA rounds' "DYNAMIC valueset" residuals — named, not silently
--    ignored, but nothing to change here.
--
-- This migration only covers #2 — the Observation step's config, sourced
-- from the new message.observationDate fixture field (added alongside this
-- round's fixture fix).

UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "category[0].coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/observation-category"},
        {"targetPath": "category[0].coding[0].code", "literalValue": "vital-signs"},
        {"targetPath": "category[0].coding[0].display", "literalValue": "Vital Signs"},
        {"targetPath": "effectiveDateTime", "sourcePath": "message.observationDate"},
        {"targetPath": "valueQuantity.system", "literalValue": "http://unitsofmeasure.org"},
        {"targetPath": "valueQuantity.code", "sourcePath": "message.observationUnit"}
    ]'::jsonb
)
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Observation'
  AND ts.step_type = 'fhir.build'
  -- Idempotent: skip if this round's fields were already appended (re-running
  -- the migration, or it already ran as part of V198 on a fresh install).
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'effectiveDateTime'
  );
