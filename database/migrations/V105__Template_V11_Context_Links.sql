-- V105: Upgrade OOB templates from v1.0 → v1.1 by adding "context" and "contextLinks"
--
-- Philosophy:
--   v1.0 templates carry no context declarations — the Go engine uses hardcoded wiring.
--   v1.1 templates declare context explicitly so the engine wires cross-resource
--   FHIR references (e.g. Encounter.subject → Patient) without any Go code change.
--
-- Approach:
--   1. Per message family, add a "context" block mapping role names to HL7 segments.
--   2. Per resource type, add "contextLinks" mapping FHIR fields to role names.
--      Each resource-level update is guarded: only applied when that resource already
--      exists in the template (prevents empty skeleton resources being created).
--   3. Version field bumped to "1.1" in both the JSONB and the version column.
--
-- Backward compatibility: templates without a "context" block continue to use
-- the hardcoded Go wiring path — this migration does NOT touch that path.

BEGIN;

-- ────────────────────────────────────────────────────────────────────────────
-- ADT family: A01-A08, A11-A17, A20-A24, A28-A29, A31, A34-A36, A39-A40
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type LIKE 'ADT^%'
  AND is_system = true;

-- Encounter.subject → patient (all ADT templates that have an Encounter resource)
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'ADT^%'
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

-- AllergyIntolerance.patient + .encounter → patient / encounter
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,AllergyIntolerance,contextLinks}'::text[],
    '{"patient":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type LIKE 'ADT^%'
  AND is_system = true
  AND template_config->'resources'->'AllergyIntolerance' IS NOT NULL;

-- Condition.subject + .encounter → patient / encounter
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Condition,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type LIKE 'ADT^%'
  AND is_system = true
  AND template_config->'resources'->'Condition' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- ORU family: R01, R30
-- Context: patient=PID, encounter=PV1, order=ORC
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1","order":"ORC"}'::jsonb
    )
WHERE message_type LIKE 'ORU^%'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,DiagnosticReport,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type LIKE 'ORU^%'
  AND is_system = true
  AND template_config->'resources'->'DiagnosticReport' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Observation,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type LIKE 'ORU^%'
  AND is_system = true
  AND template_config->'resources'->'Observation' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- ORM / OMG / OML families
-- Context: patient=PID, encounter=PV1, order=ORC
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1","order":"ORC"}'::jsonb
    )
WHERE message_type IN ('ORM^O01', 'OMG^O19', 'OML^O21')
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,ServiceRequest,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type IN ('ORM^O01', 'OMG^O19', 'OML^O21')
  AND is_system = true
  AND template_config->'resources'->'ServiceRequest' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type IN ('ORM^O01', 'OMG^O19', 'OML^O21')
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- MDM family: T01-T03, T05, T07, T11
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type LIKE 'MDM^%'
  AND is_system = true;

-- DocumentReference.subject → patient (encounter is nested inside context.encounter; skip for now)
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,DocumentReference,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'MDM^%'
  AND is_system = true
  AND template_config->'resources'->'DocumentReference' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'MDM^%'
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- SIU family: S12-S15, S17
-- Context: patient=PID, encounter=PV1
-- Note: FHIR Appointment uses participants[], not a direct subject reference;
--       contextLinks are added for Encounter only.
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type LIKE 'SIU^%'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'SIU^%'
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- VXU^V04 — Immunization
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type = 'VXU^V04'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Immunization,contextLinks}'::text[],
    '{"patient":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type = 'VXU^V04'
  AND is_system = true
  AND template_config->'resources'->'Immunization' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- VXQ^V01 — Immunisation query (read-only; patient reference in response only)
-- Context: patient=PID (no encounter in a query message)
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID"}'::jsonb
    )
WHERE message_type = 'VXQ^V01'
  AND is_system = true;

-- ────────────────────────────────────────────────────────────────────────────
-- PPR family: PC1-PC3 — Problem-oriented record
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type LIKE 'PPR^%'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Condition,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type LIKE 'PPR^%'
  AND is_system = true
  AND template_config->'resources'->'Condition' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- REF family: I12, I14 — Referral
-- Context: patient=PID (referral source; no encounter guaranteed)
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID"}'::jsonb
    )
WHERE message_type LIKE 'REF^%'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,ServiceRequest,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'REF^%'
  AND is_system = true
  AND template_config->'resources'->'ServiceRequest' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- BAR family: P01, P02
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type LIKE 'BAR^%'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'BAR^%'
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Account,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type LIKE 'BAR^%'
  AND is_system = true
  AND template_config->'resources'->'Account' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- DFT^P03 — Detail financial transaction
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type = 'DFT^P03'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,ChargeItem,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type = 'DFT^P03'
  AND is_system = true
  AND template_config->'resources'->'ChargeItem' IS NOT NULL;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Encounter,contextLinks}'::text[],
    '{"subject":"patient"}'::jsonb
)
WHERE message_type = 'DFT^P03'
  AND is_system = true
  AND template_config->'resources'->'Encounter' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- RDE^O11 — Pharmacy/treatment encoded order
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type = 'RDE^O11'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,MedicationRequest,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type = 'RDE^O11'
  AND is_system = true
  AND template_config->'resources'->'MedicationRequest' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- RAS^O17 — Pharmacy/treatment administration
-- Context: patient=PID, encounter=PV1
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{"patient":"PID","encounter":"PV1"}'::jsonb
    )
WHERE message_type = 'RAS^O17'
  AND is_system = true;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,MedicationAdministration,contextLinks}'::text[],
    '{"subject":"patient","encounter":"encounter"}'::jsonb
)
WHERE message_type = 'RAS^O17'
  AND is_system = true
  AND template_config->'resources'->'MedicationAdministration' IS NOT NULL;

-- ────────────────────────────────────────────────────────────────────────────
-- MFN family: M02-M16 — Master file notification
-- No patient context — these messages carry staff / location / charge master data.
-- Adding an empty context block still bumps the version so the engine knows
-- this is a v1.1 template even without cross-resource links.
-- ────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}',  '"1.1"'),
        '{context}',
        '{}'::jsonb
    )
WHERE message_type LIKE 'MFN^%'
  AND is_system = true;

-- ────────────────────────────────────────────────────────────────────────────
-- Verify: report the updated count per family for the migration log
-- ────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    v_total INTEGER;
    v_v11   INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_total FROM hl7_fhir_templates WHERE is_system = true;
    SELECT COUNT(*) INTO v_v11   FROM hl7_fhir_templates
    WHERE is_system = true
      AND version = '1.1'
      AND template_config->>'version' = '1.1';

    RAISE NOTICE 'V105 complete: % / % system templates upgraded to v1.1', v_v11, v_total;
END $$;

COMMIT;
