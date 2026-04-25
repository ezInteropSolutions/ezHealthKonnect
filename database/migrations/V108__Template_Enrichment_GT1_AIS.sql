-- V108: Template enrichment — GT1 (Guarantor) and AIS (Appointment Information)
--
-- Gaps addressed:
--   GT1  (Guarantor)               → RelatedPerson  — ADT, DFT, BAR families
--   AIS  (Appointment Information) → Appointment.participant slot — SIU family
--
-- TXA and SCH are already mapped in V62.
-- NK1, IN1/IN2, ROL are covered by V106.

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. RelatedPerson (GT1 — Guarantor)
--    Added to ADT^A01/A04/A05/A08/A28/A31, DFT^P03, BAR^P01/P02.
--    GT1 carries the financial guarantor's demographics — maps to RelatedPerson
--    with relationship code "GUAR" (guarantor, HL7 Table 0131).
--    Note: NK1 (V106) also maps to RelatedPerson; both coexist using different
--    indexes since GT1 = financial guarantor, NK1 = next of kin/emergency contact.
-- ─────────────────────────────────────────────────────────────────────────────

WITH gt1_section AS (
    SELECT '{
        "optional": true,
        "segment": "GT1",
        "mappings": [
            {
                "hl7Path": "GT1.2.1",
                "fhirPath": "RelatedPerson.identifier[0].value",
                "hl7DataType": "CX",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.3.1",
                "fhirPath": "RelatedPerson.name[0].family",
                "hl7DataType": "FN",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.95
            },
            {
                "hl7Path": "GT1.3.2",
                "fhirPath": "RelatedPerson.name[0].given[0]",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.3.3",
                "fhirPath": "RelatedPerson.name[0].given[1]",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "name_component",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "GT1.5.1",
                "fhirPath": "RelatedPerson.address[0].line[0]",
                "hl7DataType": "SAD",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.5.3",
                "fhirPath": "RelatedPerson.address[0].city",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.5.4",
                "fhirPath": "RelatedPerson.address[0].state",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.5.5",
                "fhirPath": "RelatedPerson.address[0].postalCode",
                "hl7DataType": "ST",
                "fhirDataType": "string",
                "transform": "address_component",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.6.4",
                "fhirPath": "RelatedPerson.telecom[0].value",
                "hl7DataType": "XTN",
                "fhirDataType": "string",
                "transform": "telecom_value",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "GT1.6.3",
                "fhirPath": "RelatedPerson.telecom[0].system",
                "hl7DataType": "ID",
                "fhirDataType": "code",
                "transform": "telecom_system_mapping",
                "required": false,
                "confidence": 0.8,
                "valueMap": {"PH":"phone","CP":"phone","FX":"fax","Internet":"email"}
            },
            {
                "hl7Path": "GT1.8",
                "fhirPath": "RelatedPerson.birthDate",
                "hl7DataType": "TS",
                "fhirDataType": "date",
                "transform": "hl7_timestamp_to_fhir_date",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "GT1.9",
                "fhirPath": "RelatedPerson.gender",
                "hl7DataType": "IS",
                "fhirDataType": "code",
                "transform": "hl7_table_0001_gender",
                "required": false,
                "confidence": 0.9,
                "valueMap": {"M":"male","F":"female","O":"other","U":"unknown"}
            },
            {
                "hl7Path": "GT1.11.1",
                "fhirPath": "RelatedPerson.relationship[0].coding[0].code",
                "hl7DataType": "CE",
                "fhirDataType": "code",
                "transform": "string_direct",
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
        '{resources,RelatedPerson_GT1}',
        gt1_section.section,
        true
    ),
    fhir_resources = (
        SELECT jsonb_agg(DISTINCT x)
        FROM jsonb_array_elements(
            COALESCE(t.fhir_resources, '[]'::jsonb) || '["RelatedPerson"]'::jsonb
        ) x
    ),
    updated_at = NOW()
FROM gt1_section
WHERE t.template_name LIKE 'OOB_%'
  AND t.message_type IN (
      'ADT^A01','ADT^A04','ADT^A05','ADT^A08','ADT^A28','ADT^A31',
      'DFT^P03','BAR^P01','BAR^P02'
  );

-- contextLinks: RelatedPerson_GT1.patient → patient role
UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,RelatedPerson_GT1,contextLinks}'::text[],
    '{"patient":"patient"}'::jsonb
)
WHERE template_name LIKE 'OOB_%'
  AND message_type IN (
      'ADT^A01','ADT^A04','ADT^A05','ADT^A08','ADT^A28','ADT^A31',
      'DFT^P03','BAR^P01','BAR^P02'
  )
  AND template_config->'resources'->'RelatedPerson_GT1' IS NOT NULL;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Appointment.participant slot (AIS — Appointment Information Service/Resource)
--    Added to all SIU templates (S12–S15, S17).
--    AIS carries the booked service resource (room, equipment, care team member).
--    In FHIR R4 these become additional entries in Appointment.participant[].
--    We map into a dedicated "AppointmentParticipant" pseudo-resource so the
--    engine's repeating-segment logic can produce multiple participants; the
--    normalizer merges them into the Appointment.participant array.
-- ─────────────────────────────────────────────────────────────────────────────

WITH ais_section AS (
    SELECT '{
        "optional": true,
        "segment": "AIS",
        "repeating": true,
        "mappings": [
            {
                "hl7Path": "AIS.3.1",
                "fhirPath": "Appointment.participant[0].actor.identifier[0].value",
                "hl7DataType": "CE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "AIS.3.2",
                "fhirPath": "Appointment.participant[0].actor.display",
                "hl7DataType": "CE",
                "fhirDataType": "string",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.9
            },
            {
                "hl7Path": "AIS.4",
                "fhirPath": "Appointment.participant[0].period.start",
                "hl7DataType": "TS",
                "fhirDataType": "dateTime",
                "transform": "hl7_timestamp_to_fhir_datetime",
                "required": false,
                "confidence": 0.85
            },
            {
                "hl7Path": "AIS.10.1",
                "fhirPath": "Appointment.participant[0].status",
                "hl7DataType": "CE",
                "fhirDataType": "code",
                "transform": "string_direct",
                "required": false,
                "confidence": 0.8,
                "valueMap": {
                    "A": "accepted",
                    "D": "declined",
                    "T": "tentative",
                    "N": "needs-action",
                    "P": "accepted"
                }
            }
        ]
    }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        t.template_config,
        '{resources,AppointmentParticipant}',
        ais_section.section,
        true
    ),
    updated_at = NOW()
FROM ais_section
WHERE t.template_name LIKE 'OOB_%'
  AND t.message_type LIKE 'SIU^%';

-- ─────────────────────────────────────────────────────────────────────────────
-- Verify
-- ─────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    v_gt1 INTEGER;
    v_ais INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_gt1 FROM hl7_fhir_templates
    WHERE template_config->'resources'->'RelatedPerson_GT1' IS NOT NULL;

    SELECT COUNT(*) INTO v_ais FROM hl7_fhir_templates
    WHERE template_config->'resources'->'AppointmentParticipant' IS NOT NULL;

    RAISE NOTICE 'V108 enrichment complete:';
    RAISE NOTICE '  RelatedPerson (GT1) added to % templates', v_gt1;
    RAISE NOTICE '  AppointmentParticipant (AIS) added to % SIU templates', v_ais;
END $$;

COMMIT;
