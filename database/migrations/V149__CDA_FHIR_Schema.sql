-- V149__CDA_FHIR_Schema.sql
-- Sprint B: CDA→FHIR template infrastructure.
--
-- Creates:
--   cda_fhir_templates      — OOB and user-authored CDA→FHIR mapping templates
--   interface_cda_mappings  — per-interface delta overrides on top of an OOB template
--
-- Mirrors the HL7→FHIR pattern (hl7_fhir_templates / interface_message_mappings) exactly.
-- All field-level mappings live in template_config JSONB — zero Go code changes to add
-- a new section or document type.
--
-- Seed: one OOB CCD row covering 9 core C-CDA 2.1 sections with full US Core R4 field
-- mappings including entry-level author/performer fields added in Sprint B.

-- =========================================================
-- TABLE: cda_fhir_templates
-- =========================================================

CREATE TABLE IF NOT EXISTS cda_fhir_templates (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_name    VARCHAR(200)  NOT NULL,
    document_type    VARCHAR(100)  NOT NULL,   -- 'CCD', 'Discharge Summary', 'Referral Note', …
    ccda_version     VARCHAR(20)   NOT NULL DEFAULT '2.1',
    fhir_version     VARCHAR(10)   NOT NULL DEFAULT 'R4',
    us_core_version  VARCHAR(20)   NOT NULL DEFAULT '6.1.0',
    template_type    VARCHAR(50)   NOT NULL DEFAULT 'field_mapping',
    template_description TEXT,
    fhir_resources   JSONB,                    -- ["Patient","AllergyIntolerance",…]
    template_config  JSONB         NOT NULL,   -- full section→field→FHIR mapping tree
    is_default       BOOLEAN       NOT NULL DEFAULT true,
    is_system        BOOLEAN       NOT NULL DEFAULT true,
    is_public        BOOLEAN       NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (document_type, ccda_version, fhir_version, is_default)
);

CREATE INDEX IF NOT EXISTS idx_cda_fhir_templates_doc_type
    ON cda_fhir_templates (document_type, ccda_version, fhir_version);

-- =========================================================
-- TABLE: interface_cda_mappings
-- =========================================================

CREATE TABLE IF NOT EXISTS interface_cda_mappings (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id            UUID         NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    document_type           VARCHAR(100) NOT NULL,
    uses_standard_template  BOOLEAN      NOT NULL DEFAULT true,
    standard_template_id    UUID         REFERENCES cda_fhir_templates(id),
    custom_mapping_config   JSONB,   -- full replacement: { sections: { … }, enabledSections: […] }
    mapping_overrides       JSONB,   -- delta additions/removals only: [{action, cdaField, fhirPath, transform, …}]
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (interface_id, document_type)
);

CREATE INDEX IF NOT EXISTS idx_interface_cda_mappings_interface
    ON interface_cda_mappings (interface_id, document_type);

-- =========================================================
-- updated_at triggers
-- =========================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
        CREATE FUNCTION set_updated_at()
        RETURNS TRIGGER LANGUAGE plpgsql AS $func$
        BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
        $func$;
    END IF;
END;
$$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_cda_fhir_templates_updated_at'
    ) THEN
        CREATE TRIGGER trg_cda_fhir_templates_updated_at
            BEFORE UPDATE ON cda_fhir_templates
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END; $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_interface_cda_mappings_updated_at'
    ) THEN
        CREATE TRIGGER trg_interface_cda_mappings_updated_at
            BEFORE UPDATE ON interface_cda_mappings
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END; $$;

-- =========================================================
-- OOB SEED: CCD C-CDA 2.1 → FHIR R4 (US Core 6.1.0)
-- Covers 9 core sections with author/performer fields
-- =========================================================

INSERT INTO cda_fhir_templates (
    template_name,
    document_type,
    ccda_version,
    fhir_version,
    us_core_version,
    template_type,
    template_description,
    fhir_resources,
    template_config,
    is_default,
    is_system,
    is_public
) VALUES (
    'OOB_CCD_21',
    'CCD',
    '2.1',
    'R4',
    '6.1.0',
    'field_mapping',
    'Out-of-box CDA/CCD C-CDA 2.1 to FHIR R4 field mapping template. Covers 9 core USCDI sections with US Core R4 6.1.0 profiles including entry-level author and performer fields.',
    '["AllergyIntolerance","MedicationStatement","Condition","Observation","Encounter","Procedure","Immunization"]'::jsonb,
    $template$
{
  "version": "1.0",
  "fhirVersion": "R4",
  "usCoreVersion": "6.1.0",
  "documentType": "CCD",
  "ccdaVersion": "2.1",
  "context": {
    "patient": "header",
    "encounter": "encounters"
  },
  "sections": {
    "allergiesAndIntolerances": {
      "fhirResource": "AllergyIntolerance",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-allergyintolerance",
      "contextLinks": { "patient": "patient" },
      "mappings": [
        {
          "cdaField": "medicationAllergyCode",
          "fhirPath": "AllergyIntolerance.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "medicationAllergyCodeDisplay",
          "fhirPath": "AllergyIntolerance.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "medicationAllergyCodeSystem",
          "fhirPath": "AllergyIntolerance.code.coding[0].system",
          "cdaDataType": "UID", "fhirDataType": "uri",
          "transform": "oid_to_fhir_system_uri",
          "conformance": "SHALL", "required": false, "confidence": 0.95,
          "valueMap": {
            "2.16.840.1.113883.6.88": "http://www.nlm.nih.gov/research/umls/rxnorm",
            "2.16.840.1.113883.6.96": "http://snomed.info/sct",
            "2.16.840.1.113883.6.69": "http://hl7.org/fhir/sid/ndc"
          }
        },
        {
          "cdaField": "severity",
          "fhirPath": "AllergyIntolerance.reaction[0].severity",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "cda_allergy_severity_to_fhir",
          "conformance": "SHOULD", "required": false, "confidence": 0.9,
          "valueMap": {
            "371924009": "mild",
            "6736007": "moderate",
            "24484000": "severe"
          }
        },
        {
          "cdaField": "status",
          "fhirPath": "AllergyIntolerance.clinicalStatus.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "cda_allergy_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "55561003": "active",
            "73425007": "inactive",
            "413322009": "resolved"
          }
        },
        {
          "cdaField": "onsetDate",
          "fhirPath": "AllergyIntolerance.onsetDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHOULD", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "authorGiven",
          "fhirPath": "AllergyIntolerance.recorder.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "authorNPI",
          "fhirPath": "AllergyIntolerance.recorder.identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    },
    "medications": {
      "fhirResource": "MedicationStatement",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-medicationstatement",
      "contextLinks": { "subject": "patient" },
      "mappings": [
        {
          "cdaField": "drugCode",
          "fhirPath": "MedicationStatement.medicationCodeableConcept.coding[0].code",
          "cdaDataType": "CE", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "drugCodeDisplay",
          "fhirPath": "MedicationStatement.medicationCodeableConcept.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "status",
          "fhirPath": "MedicationStatement.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "cda_medication_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "active": "active",
            "completed": "completed",
            "aborted": "stopped",
            "suspended": "on-hold"
          }
        },
        {
          "cdaField": "startDate",
          "fhirPath": "MedicationStatement.effectivePeriod.start",
          "cdaDataType": "IVL_TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHOULD", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "stopDate",
          "fhirPath": "MedicationStatement.effectivePeriod.end",
          "cdaDataType": "IVL_TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "MAY", "required": false, "confidence": 0.8
        },
        {
          "cdaField": "routeCode",
          "fhirPath": "MedicationStatement.dosage[0].route.coding[0].code",
          "cdaDataType": "CE", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHOULD", "required": false, "confidence": 0.85
        },
        {
          "cdaField": "doseQuantity",
          "fhirPath": "MedicationStatement.dosage[0].doseAndRate[0].doseQuantity.value",
          "cdaDataType": "PQ", "fhirDataType": "decimal",
          "transform": "string_direct",
          "conformance": "SHOULD", "required": false, "confidence": 0.85
        },
        {
          "cdaField": "doseQuantityUnit",
          "fhirPath": "MedicationStatement.dosage[0].doseAndRate[0].doseQuantity.unit",
          "cdaDataType": "PQ", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHOULD", "required": false, "confidence": 0.85
        },
        {
          "cdaField": "authorGiven",
          "fhirPath": "MedicationStatement.informationSource.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    },
    "problems": {
      "fhirResource": "Condition",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-problems-health-concerns",
      "contextLinks": { "subject": "patient", "encounter": "encounter" },
      "mappings": [
        {
          "cdaField": "conditionCode",
          "fhirPath": "Condition.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "conditionCodeDisplay",
          "fhirPath": "Condition.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "status",
          "fhirPath": "Condition.clinicalStatus.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "cda_problem_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "55561003": "active",
            "73425007": "inactive",
            "413322009": "resolved"
          }
        },
        {
          "cdaField": "onsetDate",
          "fhirPath": "Condition.onsetDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHOULD", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "authorGiven",
          "fhirPath": "Condition.recorder.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "authorNPI",
          "fhirPath": "Condition.recorder.identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    },
    "vitalSigns": {
      "fhirResource": "Observation",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-vital-signs",
      "contextLinks": { "subject": "patient", "encounter": "encounter" },
      "mappings": [
        {
          "cdaField": "vitalCode",
          "fhirPath": "Observation.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "vitalCodeDisplay",
          "fhirPath": "Observation.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "value",
          "fhirPath": "Observation.valueQuantity.value",
          "cdaDataType": "PQ", "fhirDataType": "decimal",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "valueUnit",
          "fhirPath": "Observation.valueQuantity.unit",
          "cdaDataType": "PQ", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "effectiveDate",
          "fhirPath": "Observation.effectiveDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "status",
          "fhirPath": "Observation.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "cda_result_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "completed": "final",
            "active": "preliminary",
            "aborted": "cancelled",
            "nullified": "entered-in-error"
          }
        }
      ]
    },
    "results": {
      "fhirResource": "Observation",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-lab",
      "contextLinks": { "subject": "patient", "encounter": "encounter" },
      "mappings": [
        {
          "cdaField": "testCode",
          "fhirPath": "Observation.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "testCodeDisplay",
          "fhirPath": "Observation.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "resultValue",
          "fhirPath": "Observation.valueQuantity.value",
          "cdaDataType": "PQ", "fhirDataType": "decimal",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "resultValueUnit",
          "fhirPath": "Observation.valueQuantity.unit",
          "cdaDataType": "PQ", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "status",
          "fhirPath": "Observation.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "cda_result_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "completed": "final",
            "active": "preliminary",
            "aborted": "cancelled",
            "nullified": "entered-in-error"
          }
        },
        {
          "cdaField": "effectiveDate",
          "fhirPath": "Observation.effectiveDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHALL", "required": false, "confidence": 0.95
        },
        {
          "cdaField": "performerGiven",
          "fhirPath": "Observation.performer[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "performerNPI",
          "fhirPath": "Observation.performer[0].identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    },
    "encounters": {
      "fhirResource": "Encounter",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-encounter",
      "contextLinks": { "subject": "patient" },
      "mappings": [
        {
          "cdaField": "encounterCode",
          "fhirPath": "Encounter.type[0].coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "encounterCodeDisplay",
          "fhirPath": "Encounter.type[0].coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "effectiveDate",
          "fhirPath": "Encounter.period.start",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHALL", "required": false, "confidence": 0.95
        },
        {
          "cdaField": "status",
          "fhirPath": "Encounter.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "performerGiven",
          "fhirPath": "Encounter.participant[0].individual.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "performerNPI",
          "fhirPath": "Encounter.participant[0].individual.identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "performerOrgName",
          "fhirPath": "Encounter.serviceProvider.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.65
        }
      ]
    },
    "procedures": {
      "fhirResource": "Procedure",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-procedure",
      "contextLinks": { "subject": "patient", "encounter": "encounter" },
      "mappings": [
        {
          "cdaField": "procedureCode",
          "fhirPath": "Procedure.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "procedureCodeDisplay",
          "fhirPath": "Procedure.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "status",
          "fhirPath": "Procedure.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "cda_procedure_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "completed": "completed",
            "active": "in-progress",
            "aborted": "stopped",
            "cancelled": "not-done"
          }
        },
        {
          "cdaField": "effectiveDate",
          "fhirPath": "Procedure.performedDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHOULD", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "performerGiven",
          "fhirPath": "Procedure.performer[0].actor.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "performerNPI",
          "fhirPath": "Procedure.performer[0].actor.identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    },
    "immunizations": {
      "fhirResource": "Immunization",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-immunization",
      "contextLinks": { "patient": "patient" },
      "mappings": [
        {
          "cdaField": "vaccineCode",
          "fhirPath": "Immunization.vaccineCode.coding[0].code",
          "cdaDataType": "CE", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "vaccineCodeDisplay",
          "fhirPath": "Immunization.vaccineCode.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "administrationDate",
          "fhirPath": "Immunization.occurrenceDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "status",
          "fhirPath": "Immunization.status",
          "cdaDataType": "CS", "fhirDataType": "code",
          "transform": "cda_immunization_status_to_fhir",
          "conformance": "SHALL", "required": true, "confidence": 1.0,
          "valueMap": {
            "completed": "completed",
            "active": "completed",
            "nullified": "entered-in-error",
            "aborted": "not-done"
          }
        },
        {
          "cdaField": "lotNumber",
          "fhirPath": "Immunization.lotNumber",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHOULD", "required": false, "confidence": 0.85
        },
        {
          "cdaField": "performerGiven",
          "fhirPath": "Immunization.performer[0].actor.display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        },
        {
          "cdaField": "performerOrg",
          "fhirPath": "Immunization.performer[0].actor.identifier.value",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "MAY", "required": false, "confidence": 0.65
        }
      ]
    },
    "socialHistory": {
      "fhirResource": "Observation",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-social-history",
      "contextLinks": { "subject": "patient" },
      "mappings": [
        {
          "cdaField": "observationCode",
          "fhirPath": "Observation.code.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": true, "confidence": 1.0
        },
        {
          "cdaField": "observationCodeDisplay",
          "fhirPath": "Observation.code.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 1.0
        },
        {
          "cdaField": "value",
          "fhirPath": "Observation.valueCodeableConcept.coding[0].code",
          "cdaDataType": "CD", "fhirDataType": "code",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "valueDisplay",
          "fhirPath": "Observation.valueCodeableConcept.coding[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "string_direct",
          "conformance": "SHALL", "required": false, "confidence": 0.9
        },
        {
          "cdaField": "effectiveDate",
          "fhirPath": "Observation.effectiveDateTime",
          "cdaDataType": "TS", "fhirDataType": "dateTime",
          "transform": "cda_timestamp_to_fhir_datetime",
          "conformance": "SHOULD", "required": false, "confidence": 0.85
        },
        {
          "cdaField": "authorGiven",
          "fhirPath": "Observation.performer[0].display",
          "cdaDataType": "ST", "fhirDataType": "string",
          "transform": "cda_name_to_fhir",
          "conformance": "MAY", "required": false, "confidence": 0.7
        }
      ]
    }
  }
}
$template$::jsonb,
    true,
    true,
    true
) ON CONFLICT (document_type, ccda_version, fhir_version, is_default) DO UPDATE SET
    template_name        = EXCLUDED.template_name,
    template_description = EXCLUDED.template_description,
    fhir_resources       = EXCLUDED.fhir_resources,
    template_config      = EXCLUDED.template_config,
    updated_at           = NOW();
