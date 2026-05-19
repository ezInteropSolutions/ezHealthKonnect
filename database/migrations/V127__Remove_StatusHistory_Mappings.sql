-- V127: Remove statusHistory and episodeOfCare field mappings from all mapping tables
-- Applied: 2026-05-07
--
-- Encounter.statusHistory and Encounter.episodeOfCare have no HL7v2 source
-- defined in the V2-to-FHIR IG.  statusHistory requires a period with both
-- start and end, which a single v2 event cannot supply.  episodeOfCare is a
-- FHIR-server-managed concept with no v2 equivalent.
--
-- The semantic matcher is now fixed (fhirPathBlocklist added, EVN.1 anchored
-- to empty), but any mappings already stored in the DB must be cleaned.
--
-- Affected tables and JSONB paths:
--   hl7_fhir_templates.template_config
--     OOB builder format:  .resources.<Resource>.mappings[]
--     Legacy V9 format:    .mappings.<Resource>.mappings[]
--   interface_message_mappings.custom_mapping_config  (same shapes)
--   hl7_fhir_mappings.fhir_path (flat V1 table)

-- ============================================================
-- 1. OOB TEMPLATES — resources.<Resource>.mappings[] format
-- ============================================================

DO $$
DECLARE
    resource_key TEXT;
    resource_keys TEXT[] := ARRAY[
        'Encounter', 'Patient', 'MessageHeader', 'Observation',
        'DiagnosticReport', 'ServiceRequest', 'Condition', 'Procedure',
        'AllergyIntolerance', 'Coverage', 'RelatedPerson', 'Appointment',
        'Practitioner', 'PractitionerRole', 'Location', 'DocumentReference',
        'ChargeItem', 'ChargeItemDefinition', 'ObservationDefinition'
    ];
BEGIN
    FOREACH resource_key IN ARRAY resource_keys LOOP
        UPDATE hl7_fhir_templates
        SET template_config = jsonb_set(
            template_config,
            ARRAY['resources', resource_key, 'mappings'],
            COALESCE(
                (
                    SELECT jsonb_agg(m ORDER BY m)
                    FROM jsonb_array_elements(
                        template_config #> ARRAY['resources', resource_key, 'mappings']
                    ) AS m
                    WHERE (m->>'fhirPath') NOT ILIKE '%statusHistory%'
                      AND (m->>'fhirPath') NOT ILIKE '%episodeOfCare%'
                ),
                '[]'::jsonb
            )
        )
        WHERE template_config #> ARRAY['resources', resource_key, 'mappings'] IS NOT NULL
          AND (template_config::text ILIKE '%statusHistory%'
            OR template_config::text ILIKE '%episodeOfCare%');
    END LOOP;
END;
$$;

-- ============================================================
-- 2. OOB TEMPLATES — legacy V9 mappings.<Resource>.mappings[] format
-- ============================================================

DO $$
DECLARE
    resource_key TEXT;
    resource_keys TEXT[] := ARRAY[
        'Encounter', 'Patient', 'MessageHeader', 'Observation',
        'DiagnosticReport', 'ServiceRequest', 'Condition', 'Procedure',
        'AllergyIntolerance', 'Coverage', 'RelatedPerson', 'Appointment',
        'Practitioner', 'PractitionerRole', 'Location', 'DocumentReference',
        'ChargeItem', 'ChargeItemDefinition', 'ObservationDefinition'
    ];
BEGIN
    FOREACH resource_key IN ARRAY resource_keys LOOP
        UPDATE hl7_fhir_templates
        SET template_config = jsonb_set(
            template_config,
            ARRAY['mappings', resource_key, 'mappings'],
            COALESCE(
                (
                    SELECT jsonb_agg(m ORDER BY m)
                    FROM jsonb_array_elements(
                        template_config #> ARRAY['mappings', resource_key, 'mappings']
                    ) AS m
                    WHERE (m->>'fhirPath') NOT ILIKE '%statusHistory%'
                      AND (m->>'fhirPath') NOT ILIKE '%episodeOfCare%'
                ),
                '[]'::jsonb
            )
        )
        WHERE template_config #> ARRAY['mappings', resource_key, 'mappings'] IS NOT NULL
          AND (template_config::text ILIKE '%statusHistory%'
            OR template_config::text ILIKE '%episodeOfCare%');
    END LOOP;
END;
$$;

-- ============================================================
-- 3. INTERFACE-SPECIFIC CUSTOM MAPPINGS — both JSONB shapes
-- ============================================================

DO $$
DECLARE
    resource_key TEXT;
    resource_keys TEXT[] := ARRAY[
        'Encounter', 'Patient', 'MessageHeader', 'Observation',
        'DiagnosticReport', 'ServiceRequest', 'Condition', 'Procedure',
        'AllergyIntolerance', 'Coverage', 'RelatedPerson', 'Appointment',
        'Practitioner', 'PractitionerRole', 'Location', 'DocumentReference',
        'ChargeItem', 'ChargeItemDefinition', 'ObservationDefinition'
    ];
BEGIN
    FOREACH resource_key IN ARRAY resource_keys LOOP
        -- resources.<Resource>.mappings[] shape (current OOB format)
        UPDATE interface_message_mappings
        SET custom_mapping_config = jsonb_set(
            custom_mapping_config,
            ARRAY['resources', resource_key, 'mappings'],
            COALESCE(
                (
                    SELECT jsonb_agg(m ORDER BY m)
                    FROM jsonb_array_elements(
                        custom_mapping_config #> ARRAY['resources', resource_key, 'mappings']
                    ) AS m
                    WHERE (m->>'fhirPath') NOT ILIKE '%statusHistory%'
                      AND (m->>'fhirPath') NOT ILIKE '%episodeOfCare%'
                ),
                '[]'::jsonb
            )
        )
        WHERE custom_mapping_config #> ARRAY['resources', resource_key, 'mappings'] IS NOT NULL
          AND (custom_mapping_config::text ILIKE '%statusHistory%'
            OR custom_mapping_config::text ILIKE '%episodeOfCare%');

        -- mappings.<Resource>.mappings[] shape (legacy V9 format)
        UPDATE interface_message_mappings
        SET custom_mapping_config = jsonb_set(
            custom_mapping_config,
            ARRAY['mappings', resource_key, 'mappings'],
            COALESCE(
                (
                    SELECT jsonb_agg(m ORDER BY m)
                    FROM jsonb_array_elements(
                        custom_mapping_config #> ARRAY['mappings', resource_key, 'mappings']
                    ) AS m
                    WHERE (m->>'fhirPath') NOT ILIKE '%statusHistory%'
                      AND (m->>'fhirPath') NOT ILIKE '%episodeOfCare%'
                ),
                '[]'::jsonb
            )
        )
        WHERE custom_mapping_config #> ARRAY['mappings', resource_key, 'mappings'] IS NOT NULL
          AND (custom_mapping_config::text ILIKE '%statusHistory%'
            OR custom_mapping_config::text ILIKE '%episodeOfCare%');
    END LOOP;
END;
$$;

-- ============================================================
-- 4. LEGACY hl7_fhir_mappings flat table (V1 schema)
-- ============================================================

DELETE FROM hl7_fhir_mappings
WHERE fhir_path ILIKE '%statusHistory%'
   OR fhir_path ILIKE '%episodeOfCare%';
