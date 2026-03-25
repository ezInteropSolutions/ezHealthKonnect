-- V63__Enhanced_FHIR_Resources.sql
-- Adds new FHIR resource sections (Organization, Practitioner, PractitionerRole,
-- EncounterNote, Specimen) and additional field mappings to existing OOB templates
-- using jsonb_set incremental updates. Does NOT re-seed entire templates.

BEGIN;

-- ============================================================
-- 1a. Add Organization resource to ALL OOB templates
-- ============================================================

-- Build the Organization mappings JSON block once and reuse it via a CTE.
WITH org_section AS (
    SELECT
        '{
            "optional": false,
            "mappings": [
                {
                    "hl7Path": "MSH.3.1",
                    "fhirPath": "Organization.name",
                    "hl7DataType": "HD",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "MSH.4.1",
                    "fhirPath": "Organization.identifier[0].value",
                    "hl7DataType": "HD",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "MSH.3",
                    "fhirPath": "Organization.identifier[0].system",
                    "hl7DataType": "HD",
                    "fhirDataType": "uri",
                    "transform": "assigning_authority_to_uri",
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "MSH.3.2",
                    "fhirPath": "Organization.type[0].coding[0].code",
                    "hl7DataType": "HD",
                    "fhirDataType": "code",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.7
                }
            ]
        }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    -- Inject Organization into resources object
    template_config = jsonb_set(
        t.template_config,
        '{resources,Organization}',
        org_section.section,
        true
    ),
    -- Append "Organization" to fhir_resources array, deduplicating
    fhir_resources = (
        SELECT jsonb_agg(DISTINCT x)
        FROM jsonb_array_elements(
            COALESCE(t.fhir_resources, '[]'::jsonb) || '["Organization"]'::jsonb
        ) x
    ),
    updated_at = NOW()
FROM org_section
WHERE t.template_name LIKE 'OOB_%';


-- ============================================================
-- 1b. Add Practitioner + PractitionerRole to all OOB_ADT_% templates
-- ============================================================

WITH practitioner_section AS (
    SELECT
        '{
            "optional": false,
            "mappings": [
                {
                    "hl7Path": "PV1.7.1",
                    "fhirPath": "Practitioner.identifier[0].value",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "PV1.7.9",
                    "fhirPath": "Practitioner.identifier[0].system",
                    "hl7DataType": "XCN",
                    "fhirDataType": "uri",
                    "transform": "assigning_authority_to_uri",
                    "required": false,
                    "confidence": 0.8
                },
                {
                    "hl7Path": "PV1.7.3",
                    "fhirPath": "Practitioner.name[0].family",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "name_component",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "PV1.7.4",
                    "fhirPath": "Practitioner.name[0].given[0]",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "name_component",
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "PV1.7.5",
                    "fhirPath": "Practitioner.name[0].given[1]",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "name_component",
                    "required": false,
                    "confidence": 0.75
                },
                {
                    "hl7Path": "PV1.7.6",
                    "fhirPath": "Practitioner.name[0].prefix[0]",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "name_component",
                    "required": false,
                    "confidence": 0.75
                },
                {
                    "hl7Path": "PV1.7.13",
                    "fhirPath": "Practitioner.identifier[0].type.coding[0].code",
                    "hl7DataType": "XCN",
                    "fhirDataType": "code",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.8
                }
            ]
        }'::jsonb AS section
),
practitioner_role_section AS (
    SELECT
        '{
            "optional": false,
            "mappings": [
                {
                    "hl7Path": "PV1.7.1",
                    "fhirPath": "PractitionerRole.practitioner.identifier[0].value",
                    "hl7DataType": "XCN",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "PV1.2",
                    "fhirPath": "PractitionerRole.code[0].coding[0].code",
                    "hl7DataType": "IS",
                    "fhirDataType": "code",
                    "transform": "patient_class_mapping",
                    "valueMap": {"I": "ATND", "O": "REF", "E": "ATND"},
                    "required": false,
                    "confidence": 0.8
                }
            ]
        }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        jsonb_set(
            t.template_config,
            '{resources,Practitioner}',
            practitioner_section.section,
            true
        ),
        '{resources,PractitionerRole}',
        practitioner_role_section.section,
        true
    ),
    fhir_resources = (
        SELECT jsonb_agg(DISTINCT x)
        FROM jsonb_array_elements(
            COALESCE(t.fhir_resources, '[]'::jsonb)
            || '["Practitioner","PractitionerRole"]'::jsonb
        ) x
    ),
    updated_at = NOW()
FROM practitioner_section, practitioner_role_section
WHERE t.template_name LIKE 'OOB_ADT_%';


-- ============================================================
-- 1c. Add EncounterNote (NTE mappings) to all OOB_ADT_% templates
-- ============================================================

-- We append the two NTE→Encounter.note mappings to the existing
-- Encounter mappings array. Use jsonb_set + || concatenation.

UPDATE hl7_fhir_templates
SET
    template_config = jsonb_set(
        template_config,
        '{resources,Encounter,mappings}',
        (
            COALESCE(template_config->'resources'->'Encounter'->'mappings', '[]'::jsonb)
            || '[
                {
                    "hl7Path": "NTE.3",
                    "fhirPath": "Encounter.note[0].text",
                    "hl7DataType": "FT",
                    "fhirDataType": "markdown",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9,
                    "optional": true
                },
                {
                    "hl7Path": "NTE.4.2",
                    "fhirPath": "Encounter.note[0].authorString",
                    "hl7DataType": "CE",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.75,
                    "optional": true
                }
            ]'::jsonb
        ),
        true
    ),
    updated_at = NOW()
WHERE template_name LIKE 'OOB_ADT_%';


-- Add NTE→Observation.note to OOB_ORU_R01
UPDATE hl7_fhir_templates
SET
    template_config = jsonb_set(
        template_config,
        '{resources,Observation,mappings}',
        (
            COALESCE(template_config->'resources'->'Observation'->'mappings', '[]'::jsonb)
            || '[
                {
                    "hl7Path": "NTE.3",
                    "fhirPath": "Observation.note[0].text",
                    "hl7DataType": "FT",
                    "fhirDataType": "markdown",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9,
                    "optional": true
                },
                {
                    "hl7Path": "NTE.4.2",
                    "fhirPath": "Observation.note[0].authorString",
                    "hl7DataType": "CE",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.75,
                    "optional": true
                }
            ]'::jsonb
        ),
        true
    ),
    updated_at = NOW()
WHERE template_name = 'OOB_ORU_R01';


-- ============================================================
-- 1d. Add OBX.2 value-type dispatch mapping to OOB_ORU_R01
-- ============================================================

UPDATE hl7_fhir_templates
SET
    template_config = jsonb_set(
        template_config,
        '{resources,Observation,mappings}',
        (
            COALESCE(template_config->'resources'->'Observation'->'mappings', '[]'::jsonb)
            || '[
                {
                    "hl7Path": "OBX.2",
                    "fhirPath": "Observation.meta.tag[0].code",
                    "hl7DataType": "ID",
                    "fhirDataType": "code",
                    "transform": "observation_value_type",
                    "valueMap": {
                        "NM": "valueQuantity",
                        "SN": "valueQuantity",
                        "CWE": "valueCodeableConcept",
                        "CE": "valueCodeableConcept",
                        "ST": "valueString",
                        "TX": "valueString",
                        "FT": "valueString",
                        "TS": "valueDateTime",
                        "DT": "valueDate",
                        "TM": "valueTime",
                        "RP": "valueAttachment",
                        "ED": "valueAttachment"
                    },
                    "required": false,
                    "confidence": 0.95
                }
            ]'::jsonb
        ),
        true
    ),
    updated_at = NOW()
WHERE template_name = 'OOB_ORU_R01';


-- ============================================================
-- 1e. Add Specimen resource to OOB_ORU_R01 and OOB_OML_O33
-- ============================================================

WITH specimen_section AS (
    SELECT
        '{
            "optional": true,
            "mappings": [
                {
                    "hl7Path": "SPM.1",
                    "fhirPath": "Specimen.id",
                    "hl7DataType": "SI",
                    "fhirDataType": "id",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "SPM.2.1",
                    "fhirPath": "Specimen.identifier[0].value",
                    "hl7DataType": "EIP",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "SPM.4.1",
                    "fhirPath": "Specimen.type.coding[0].code",
                    "hl7DataType": "CWE",
                    "fhirDataType": "code",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "SPM.4.2",
                    "fhirPath": "Specimen.type.coding[0].display",
                    "hl7DataType": "CWE",
                    "fhirDataType": "string",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "SPM.4.3",
                    "fhirPath": "Specimen.type.coding[0].system",
                    "hl7DataType": "CWE",
                    "fhirDataType": "uri",
                    "transform": "coding_system_mapping",
                    "valueMap": {
                        "SCT": "http://snomed.info/sct",
                        "HL70487": "http://terminology.hl7.org/CodeSystem/v2-0487"
                    },
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "SPM.8.1",
                    "fhirPath": "Specimen.collection.bodySite.coding[0].code",
                    "hl7DataType": "CWE",
                    "fhirDataType": "code",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.8
                },
                {
                    "hl7Path": "SPM.11.1",
                    "fhirPath": "Specimen.collection.collectedDateTime",
                    "hl7DataType": "DR",
                    "fhirDataType": "dateTime",
                    "transform": "hl7_timestamp_to_fhir_datetime",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "SPM.12",
                    "fhirPath": "Specimen.collection.quantity.value",
                    "hl7DataType": "CQ",
                    "fhirDataType": "decimal",
                    "transform": "numeric_mapping",
                    "required": false,
                    "confidence": 0.85
                },
                {
                    "hl7Path": "SPM.17",
                    "fhirPath": "Specimen.receivedTime",
                    "hl7DataType": "TS",
                    "fhirDataType": "dateTime",
                    "transform": "hl7_timestamp_to_fhir_datetime",
                    "required": false,
                    "confidence": 0.9
                },
                {
                    "hl7Path": "SPM.20",
                    "fhirPath": "Specimen.note[0].text",
                    "hl7DataType": "ST",
                    "fhirDataType": "markdown",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.75
                },
                {
                    "hl7Path": "SPM.27.1",
                    "fhirPath": "Specimen.condition[0].coding[0].code",
                    "hl7DataType": "CWE",
                    "fhirDataType": "code",
                    "transform": "string_direct",
                    "required": false,
                    "confidence": 0.8
                }
            ]
        }'::jsonb AS section
)
UPDATE hl7_fhir_templates t
SET
    template_config = jsonb_set(
        t.template_config,
        '{resources,Specimen}',
        specimen_section.section,
        true
    ),
    fhir_resources = (
        SELECT jsonb_agg(DISTINCT x)
        FROM jsonb_array_elements(
            COALESCE(t.fhir_resources, '[]'::jsonb) || '["Specimen"]'::jsonb
        ) x
    ),
    updated_at = NOW()
FROM specimen_section
WHERE t.template_name IN ('OOB_ORU_R01', 'OOB_OML_O33');

COMMIT;
