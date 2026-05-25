-- V106: Template enrichment — add NK1, IN1/IN2, and ROL segment mappings
--
-- Gaps addressed:
--   NK1  (Next of Kin)     → RelatedPerson  — ADT family, DFT, BAR
--   IN1  (Insurance)       → Coverage       — ADT^A01/A04/A05/A08/A28/A31, DFT^P03, BAR^P01
--   ROL  (Role)            → PractitionerRole — ADT^A01, ORU^R01
--
-- These segments are already documented in V95 user guides and referenced in
-- real-world sample messages from WebChart (ADT admission, registration, update).
-- They were absent from the V62 seed templates.
--
-- All new resource sections are marked optional:true so the engine does not raise
-- validation errors when the segment is absent from a given message.
--
-- contextLinks for the new resources are added at the end of this migration (V105
-- only wired resources that existed at that point).

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. RelatedPerson  (from NK1 — Next of Kin / Emergency Contact)
--    Added to all ADT templates, DFT^P03, BAR^P01/P02.
-- ─────────────────────────────────────────────────────────────────────────────

WITH nk1_section AS (
    SELECT '{
        "optional": true,
        "segment": "NK1",
        "mappings": [
            {
                "hl7Path": "NK1.2.1",
                "fhirPath": "RelatedPerson.name[0].family",
                "hl7DataType": "FN",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.95
            },
            {
                "hl7Path": "NK1.2.2",
                "fhirPath": "RelatedPerson.name[0].given[0]",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.2.3",
                "fhirPath": "RelatedPerson.name[0].given[1]",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "NK1.3.1",
                "fhirPath": "RelatedPerson.relationship[0].coding[0].code",
                "hl7DataType": "CE",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "NK1.3.2",
                "fhirPath": "RelatedPerson.relationship[0].coding[0].display",
                "hl7DataType": "CE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "NK1.3.3",
                "fhirPath": "RelatedPerson.relationship[0].coding[0].system",
                "hl7DataType": "ID",
                "fhirDataType": "uri",
                "transform": "coding_system_mapping",
                "required": false,
                "confidence": 0.75
            },
            {
                "hl7Path": "NK1.4.1",
                "fhirPath": "RelatedPerson.address[0].line[0]",
                "hl7DataType": "SAD",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.4.3",
                "fhirPath": "RelatedPerson.address[0].city",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.4.4",
                "fhirPath": "RelatedPerson.address[0].state",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.4.5",
                "fhirPath": "RelatedPerson.address[0].postalCode",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.4.6",
                "fhirPath": "RelatedPerson.address[0].country",
                "hl7DataType": "ID",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "NK1.5.4",
                "fhirPath": "RelatedPerson.telecom[0].value",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "telecom_value",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "NK1.5.3",
                "fhirPath": "RelatedPerson.telecom[0].system",
                "hl7DataType": "ID",
                "fhirDataType": "code",
                "transform": "telecom_system_mapping",
                "required": false,
                "confidence": 0.85,
                "valueMap": {"PH": "phone", "CP": "phone", "FX": "fax", "Internet": "email"}
            },
            {
                "hl7Path": "NK1.5.2",
                "fhirPath": "RelatedPerson.telecom[0].use",
                "hl7DataType": "ID",
                "fhirDataType": "code",
                "transform": "telecom_use_mapping",
                "required": false,
                "confidence": 0.8,
                "valueMap": {"PRN": "home", "WPN": "work", "NET": "home", "BPN": "mobile"}
            },
            {
                "hl7Path": "NK1.8",
                "fhirPath": "RelatedPerson.period.start",
                "hl7DataType": "DT",
                "fhirDataType": "date",
                "transform": "hl7_timestamp_to_fhir_date",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "NK1.9",
                "fhirPath": "RelatedPerson.period.end",
                "hl7DataType": "DT",
                "fhirDataType": "date",
                "transform": "hl7_timestamp_to_fhir_date",
                "required": false,
                "confidence": 0.85
            }
        ]
    }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        t.template_config,
        '{resources,RelatedPerson}'::text[],
        nk1_section.section
    ),
    fhir_resources = CASE
        WHEN t.fhir_resources @> '"RelatedPerson"' THEN t.fhir_resources
        ELSE t.fhir_resources || '"RelatedPerson"'::jsonb
    END
FROM nk1_section
WHERE t.is_system = true
  AND t.message_type IN (
      'ADT^A01','ADT^A02','ADT^A03','ADT^A04','ADT^A05',
      'ADT^A06','ADT^A07','ADT^A08','ADT^A11','ADT^A12',
      'ADT^A13','ADT^A14','ADT^A15','ADT^A16','ADT^A17',
      'ADT^A20','ADT^A21','ADT^A22','ADT^A23','ADT^A24',
      'ADT^A28','ADT^A29','ADT^A31','ADT^A34','ADT^A35',
      'ADT^A36','ADT^A39','ADT^A40',
      'DFT^P03','BAR^P01','BAR^P02'
  );

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Coverage  (from IN1 — Insurance)
--    Added to admission/registration/update/financial templates.
--    IN1.36 = policy number (subscriber ID in FHIR terminology)
--    IN1.18/19 = eligibility dates → Coverage.period
-- ─────────────────────────────────────────────────────────────────────────────

WITH in1_section AS (
    SELECT '{
        "optional": true,
        "segment": "IN1",
        "mappings": [
            {
                "hl7Path": "IN1.1",
                "fhirPath": "Coverage.order",
                "hl7DataType": "SI",
                "fhirDataType": "positiveInt",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "IN1.2.1",
                "fhirPath": "Coverage.identifier[0].value",
                "hl7DataType": "CWE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.3.1",
                "fhirPath": "Coverage.payor[0].identifier[0].value",
                "hl7DataType": "CX",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.4.1",
                "fhirPath": "Coverage.payor[0].display",
                "hl7DataType": "XON",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.11",
                "fhirPath": "Coverage.class[0].value",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "IN1.12",
                "fhirPath": "Coverage.class[0].name",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "IN1.15.1",
                "fhirPath": "Coverage.type.coding[0].code",
                "hl7DataType": "IS",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "IN1.15.2",
                "fhirPath": "Coverage.type.coding[0].display",
                "hl7DataType": "IS",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.75
            },
            {
                "hl7Path": "IN1.16.1",
                "fhirPath": "Coverage.subscriber.display",
                "hl7DataType": "XPN",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "IN1.18",
                "fhirPath": "Coverage.period.start",
                "hl7DataType": "DT",
                "fhirDataType": "date",
                "transform": "hl7_timestamp_to_fhir_date",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.19",
                "fhirPath": "Coverage.period.end",
                "hl7DataType": "DT",
                "fhirDataType": "date",
                "transform": "hl7_timestamp_to_fhir_date",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.36",
                "fhirPath": "Coverage.subscriberId",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "IN1.46.1",
                "fhirPath": "Coverage.relationship.coding[0].code",
                "hl7DataType": "CWE",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "IN1.46.2",
                "fhirPath": "Coverage.relationship.coding[0].display",
                "hl7DataType": "CWE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.75
            }
        ]
    }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        t.template_config,
        '{resources,Coverage}'::text[],
        in1_section.section
    ),
    fhir_resources = CASE
        WHEN t.fhir_resources @> '"Coverage"' THEN t.fhir_resources
        ELSE t.fhir_resources || '"Coverage"'::jsonb
    END
FROM in1_section
WHERE t.is_system = true
  AND t.message_type IN (
      'ADT^A01','ADT^A04','ADT^A05','ADT^A08',
      'ADT^A28','ADT^A31',
      'DFT^P03','BAR^P01','BAR^P02'
  );

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. PractitionerRole  (from ROL — Role)
--    Added to ADT^A01 (primary admission with full role set) and ORU^R01
--    (clinical results carry ordering/performing physician roles).
--    ROL.3 = role code (consulting, attending, referring, etc.)
--    ROL.4 = person in role (XCN — name + ID)
-- ─────────────────────────────────────────────────────────────────────────────

WITH rol_section AS (
    SELECT '{
        "optional": true,
        "segment": "ROL",
        "mappings": [
            {
                "hl7Path": "ROL.1.1",
                "fhirPath": "PractitionerRole.identifier[0].value",
                "hl7DataType": "EI",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8
            },
            {
                "hl7Path": "ROL.3.1",
                "fhirPath": "PractitionerRole.code[0].coding[0].code",
                "hl7DataType": "CE",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "ROL.3.2",
                "fhirPath": "PractitionerRole.code[0].coding[0].display",
                "hl7DataType": "CE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "ROL.3.3",
                "fhirPath": "PractitionerRole.code[0].coding[0].system",
                "hl7DataType": "ID",
                "fhirDataType": "uri",
                "transform": "coding_system_mapping",
                "required": false,
                "confidence": 0.75
            },
            {
                "hl7Path": "ROL.4.1",
                "fhirPath": "PractitionerRole.practitioner.identifier[0].value",
                "hl7DataType": "XCN",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "ROL.4.3",
                "fhirPath": "PractitionerRole.practitioner.display",
                "hl7DataType": "XCN",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "ROL.4.9",
                "fhirPath": "PractitionerRole.specialty[0].coding[0].code",
                "hl7DataType": "CE",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.75
            },
            {
                "hl7Path": "ROL.5",
                "fhirPath": "PractitionerRole.period.start",
                "hl7DataType": "TS",
                "fhirDataType": "dateTime",
                "transform": "hl7_timestamp_to_fhir_datetime",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "ROL.6",
                "fhirPath": "PractitionerRole.period.end",
                "hl7DataType": "TS",
                "fhirDataType": "dateTime",
                "transform": "hl7_timestamp_to_fhir_datetime",
                "required": false,
                "confidence": 0.85
            }
        ]
    }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        t.template_config,
        '{resources,PractitionerRole}'::text[],
        rol_section.section
    ),
    fhir_resources = CASE
        WHEN t.fhir_resources @> '"PractitionerRole"' THEN t.fhir_resources
        ELSE t.fhir_resources || '"PractitionerRole"'::jsonb
    END
FROM rol_section
WHERE t.is_system = true
  AND t.message_type IN (
      'ADT^A01','ADT^A02','ADT^A03','ADT^A04','ADT^A05',
      'ADT^A06','ADT^A07','ADT^A08','ADT^A28','ADT^A31',
      'ORU^R01'
  );

-- ─────────────────────────────────────────────────────────────────────────────
-- 4. contextLinks for the newly-added resources
--    (V105 ran before these resources existed, so we wire them here.)
-- ─────────────────────────────────────────────────────────────────────────────

-- RelatedPerson.patient → patient
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,RelatedPerson,contextLinks}'::text[],
    '{"patient":"patient"}'::jsonb
)
WHERE is_system = true
  AND template_config->'resources'->'RelatedPerson' IS NOT NULL;

-- Coverage.beneficiary → patient  (the insured person is the patient)
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Coverage,contextLinks}'::text[],
    '{"beneficiary":"patient"}'::jsonb
)
WHERE is_system = true
  AND template_config->'resources'->'Coverage' IS NOT NULL;

-- PractitionerRole: no direct patient link in FHIR — no contextLinks needed
-- (PractitionerRole references the Practitioner and Organization, not the Patient)

-- ─────────────────────────────────────────────────────────────────────────────
-- 5. Verify
-- ─────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    v_nk1  INTEGER;
    v_in1  INTEGER;
    v_rol  INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_nk1 FROM hl7_fhir_templates
    WHERE is_system = true AND template_config->'resources'->'RelatedPerson' IS NOT NULL;

    SELECT COUNT(*) INTO v_in1 FROM hl7_fhir_templates
    WHERE is_system = true AND template_config->'resources'->'Coverage' IS NOT NULL;

    SELECT COUNT(*) INTO v_rol FROM hl7_fhir_templates
    WHERE is_system = true AND template_config->'resources'->'PractitionerRole' IS NOT NULL;

    RAISE NOTICE 'V106 enrichment complete:';
    RAISE NOTICE '  RelatedPerson (NK1) added to % templates', v_nk1;
    RAISE NOTICE '  Coverage (IN1)      added to % templates', v_in1;
    RAISE NOTICE '  PractitionerRole (ROL) added to % templates', v_rol;
END $$;

COMMIT;
