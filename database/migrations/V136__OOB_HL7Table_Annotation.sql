-- V136: Annotate OOB template field mappings with hl7Table for unambiguous HL7 table lookups
--
-- When a CE/CWE field carries only a bare code with no coding-system component (CE.3 empty),
-- transformCEToCodeableConcept cannot attach a FHIR system URI. For fields where the HL7 table
-- is fixed by the standard (not site-specific), we annotate the OOB template mapping with
-- "hl7Table" so the transformer can call the ValueMapper and inject the correct FHIR system.
--
-- Annotated fields (OOB-controllable — no legitimate per-site variation):
--   PID.16  → HL7 Table 0002 (Marital Status)
--             FHIR system: http://terminology.hl7.org/CodeSystem/v3-MaritalStatus
--   NK1.3   → HL7 Table 0063 (Relationship)
--             FHIR system: http://terminology.hl7.org/CodeSystem/v2-0063
--
-- Fields deliberately NOT annotated here (user-controlled, legitimately varies by site):
--   DG1.3   — diagnosis coding system (ICD-10, SNOMED, local) chosen per interface
--   PR1.3   — procedure coding (CPT, ICD, SNOMED) chosen per interface
--   PV1.2   — patient class (may use local extensions beyond Table 0004)
--   IN1.15  — plan type (payer-specific codes)
--   OBR.4   — universal service ID (LOINC, CPT, local lab codes)

-- ── PID.16 → hl7Table "0002" (Marital Status) across all OOB templates ──────────────────────

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Patient,mappings}',
    (
        SELECT COALESCE(jsonb_agg(
            CASE
                WHEN (elem->>'hl7Path') = 'PID.16'
                     AND (elem->>'hl7Table') IS NULL
                THEN elem || '{"hl7Table": "0002"}'::jsonb
                ELSE elem
            END
            ORDER BY elem->>'hl7Path'
        ), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config -> 'resources' -> 'Patient' -> 'mappings'
        ) AS elem
    )
)
WHERE is_default = true
  AND template_config -> 'resources' -> 'Patient' -> 'mappings' IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(
          template_config -> 'resources' -> 'Patient' -> 'mappings'
      ) AS elem
      WHERE elem->>'hl7Path' = 'PID.16'
        AND (elem->>'hl7Table') IS NULL
  );

-- ── NK1.3 → hl7Table "0063" (Relationship) across all OOB templates ─────────────────────────

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,RelatedPerson,mappings}',
    (
        SELECT COALESCE(jsonb_agg(
            CASE
                WHEN (elem->>'hl7Path') = 'NK1.3'
                     AND (elem->>'hl7Table') IS NULL
                THEN elem || '{"hl7Table": "0063"}'::jsonb
                ELSE elem
            END
            ORDER BY elem->>'hl7Path'
        ), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config -> 'resources' -> 'RelatedPerson' -> 'mappings'
        ) AS elem
    )
)
WHERE is_default = true
  AND template_config -> 'resources' -> 'RelatedPerson' -> 'mappings' IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(
          template_config -> 'resources' -> 'RelatedPerson' -> 'mappings'
      ) AS elem
      WHERE elem->>'hl7Path' = 'NK1.3'
        AND (elem->>'hl7Table') IS NULL
  );
