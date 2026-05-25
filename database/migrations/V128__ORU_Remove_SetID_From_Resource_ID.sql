-- ============================================================================
-- V128: ORU^R01 — Remove SI (Sequence ID) → FHIR resource id mappings
--
-- OBR.1 (Set ID - OBR) and OBX.1 (Set ID - OBX) are positional sequence
-- counters used to order segment repetitions within a message.  They carry
-- no identity information and must not be used as FHIR resource ids.
--
-- The V62 template incorrectly mapped:
--   OBR.1 → DiagnosticReport.id   (hl7DataType: SI)
--   OBX.1 → Observation.id        (hl7DataType: SI)
--
-- Both are removed here.  DiagnosticReport.id and Observation.id continue to
-- be set by the Go transform engine (diagnosticreport-<nanos>,
-- obs-<n> from AssembleORUObservations).
-- ============================================================================

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,DiagnosticReport,mappings}',
    (
        SELECT COALESCE(jsonb_agg(elem ORDER BY (elem->>'hl7Path')), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config->'resources'->'DiagnosticReport'->'mappings'
        ) AS elem
        WHERE NOT (
            elem->>'hl7DataType' = 'SI'
            AND (elem->>'fhirPath' LIKE '%.id' OR elem->>'fhirPath' = 'id'
                 OR elem->>'fhirPath' = 'DiagnosticReport.id')
        )
    )
),
updated_at = NOW()
WHERE message_type LIKE 'ORU%'
  AND is_default = true
  AND template_config->'resources'->'DiagnosticReport'->'mappings' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Observation,mappings}',
    (
        SELECT COALESCE(jsonb_agg(elem ORDER BY (elem->>'hl7Path')), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config->'resources'->'Observation'->'mappings'
        ) AS elem
        WHERE NOT (
            elem->>'hl7DataType' = 'SI'
            AND (elem->>'fhirPath' LIKE '%.id' OR elem->>'fhirPath' = 'id'
                 OR elem->>'fhirPath' = 'Observation.id')
        )
    )
),
updated_at = NOW()
WHERE message_type LIKE 'ORU%'
  AND is_default = true
  AND template_config->'resources'->'Observation'->'mappings' IS NOT NULL;

DO $$
DECLARE v_dr_count INT; v_obs_count INT;
BEGIN
    SELECT COUNT(*) INTO v_dr_count
    FROM hl7_fhir_templates,
         jsonb_array_elements(template_config->'resources'->'DiagnosticReport'->'mappings') AS elem
    WHERE message_type LIKE 'ORU%' AND is_default = true
      AND elem->>'hl7DataType' = 'SI'
      AND (elem->>'fhirPath' LIKE '%.id' OR elem->>'fhirPath' = 'id');

    SELECT COUNT(*) INTO v_obs_count
    FROM hl7_fhir_templates,
         jsonb_array_elements(template_config->'resources'->'Observation'->'mappings') AS elem
    WHERE message_type LIKE 'ORU%' AND is_default = true
      AND elem->>'hl7DataType' = 'SI'
      AND (elem->>'fhirPath' LIKE '%.id' OR elem->>'fhirPath' = 'id');

    RAISE NOTICE 'V128: Remaining SI→id mappings after cleanup: DR=%, Obs=%', v_dr_count, v_obs_count;
    IF v_dr_count > 0 OR v_obs_count > 0 THEN
        RAISE EXCEPTION 'V128: SI→id mappings still present after cleanup';
    END IF;
    RAISE NOTICE '✅ V128: SI (Set ID) fields removed from DiagnosticReport.id and Observation.id in all ORU templates';
END $$;
