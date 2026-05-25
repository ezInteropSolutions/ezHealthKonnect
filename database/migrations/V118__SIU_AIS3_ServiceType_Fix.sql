-- V118: Fix AIS.3 mapping in SIU templates
--
-- AIS.3 is the "Universal Service Identifier" (CWE) — the service being
-- scheduled.  It must map to Appointment.serviceType[0], NOT to
-- Appointment.participant[0].actor.display.
--
-- The wrong mapping caused AIS.3's CWE CodeableConcept to overwrite
-- AIP.3's XCN person name at actor.display, leaving the participant actor
-- absent after sanitization.
--
-- This migration patches all existing SIU template rows in-place so that
-- a full OOB rebuild is not required immediately after deployment.
-- The anchor change in semantic_matcher.go ensures all future rebuilds
-- emit the correct path.

BEGIN;

-- Update AIS.3 fhirPath in the Appointment resource block of every SIU template.
-- template_config structure: {"resources": {"Appointment": {"mappings": [...]}}}
-- We replace any mapping with hl7Path = "AIS.3" that still points to the old path.
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Appointment,mappings}',
    (
        SELECT jsonb_agg(
            CASE
                WHEN (m->>'hl7Path') = 'AIS.3'
                THEN m
                    || '{"fhirPath": "Appointment.serviceType[0]",
                        "fhirDataType": "CodeableConcept",
                        "transform": "ce_to_codeableconcept"}'::jsonb
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
      WHERE (m->>'hl7Path') = 'AIS.3'
        AND (m->>'fhirPath') LIKE '%actor%'
  );

DO $$
DECLARE v_count INTEGER;
BEGIN
    GET DIAGNOSTICS v_count = ROW_COUNT;
    RAISE NOTICE 'V118 complete: % SIU template(s) updated (AIS.3 → serviceType[0])', v_count;
END $$;

COMMIT;
