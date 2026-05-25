-- V120: Fix AI*.3 actor.display transforms in SIU templates
--
-- AIG.3, AIL.3, AIP.3 all target Appointment.participant[0].actor.display.
-- Earlier templates stored ce_to_codeableconcept for these CE-typed fields,
-- producing a CodeableConcept map instead of a plain display string.
-- xcn_to_reference correctly handles id^family^given composites (XCN, CE,
-- or PL style) and returns a clean "Family, Given" string.
--
-- This migration also ensures fhirPath is the full .actor.display path
-- (not the abbreviated .actor path) so the sanitizer can find the value.

BEGIN;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Appointment,mappings}',
    (
        SELECT jsonb_agg(
            CASE
                WHEN (m->>'hl7Path') IN ('AIP.3', 'AIG.3', 'AIL.3')
                THEN m
                    || '{"transform": "xcn_to_reference"}'::jsonb
                    || '{"fhirPath": "Appointment.participant[0].actor.display"}'::jsonb
                    || '{"fhirDataType": "string"}'::jsonb
                ELSE m
            END
        )
        FROM jsonb_array_elements(
            template_config->'resources'->'Appointment'->'mappings'
        ) AS m
    )
)
WHERE message_type LIKE 'SIU%'
  AND template_config->'resources'->'Appointment'->'mappings' IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(
          template_config->'resources'->'Appointment'->'mappings'
      ) m
      WHERE (m->>'hl7Path') IN ('AIP.3', 'AIG.3', 'AIL.3')
        AND COALESCE(m->>'transform', '') != 'xcn_to_reference'
  );

DO $$
DECLARE v_count INTEGER;
BEGIN
    GET DIAGNOSTICS v_count = ROW_COUNT;
    RAISE NOTICE 'V120 complete: % SIU template(s) updated (AI*.3 → xcn_to_reference at actor.display)', v_count;
END $$;

COMMIT;
