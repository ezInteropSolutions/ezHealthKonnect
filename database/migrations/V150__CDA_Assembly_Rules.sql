-- V150__CDA_Assembly_Rules.sql
-- Sprint C: Expand CDA→FHIR OOB template coverage.
--
-- Adds:
--   1. 12 additional sections to the existing CCD OOB template (V149 seeded 9 core sections)
--   2. OOB template for "Discharge Summary" (encounter-centric sections)
--   3. OOB template for "Referral Note" (referral-specific sections)
--
-- After this migration the GenericCDAFHIRMapper can process 21+ C-CDA 2.1 sections
-- across three document types without any Go code changes.

-- =========================================================
-- 1.  Expand CCD template with 12 additional sections
-- =========================================================

UPDATE cda_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{sections}',
    (template_config->'sections')::jsonb || $template$
{
  "encounters": {
    "fhirResource": "Encounter",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-encounter",
    "contextLinks": {"patient": "patient"},
    "mappings": [
      {"cdaField": "encounterCode",    "fhirPath": "Encounter.type[0].coding[0].code",    "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.95},
      {"cdaField": "encounterDate",    "fhirPath": "Encounter.period.start",               "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHALL",  "confidence": 0.90},
      {"cdaField": "encounterEndDate", "fhirPath": "Encounter.period.end",                "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD", "confidence": 0.80},
      {"cdaField": "encounterStatus",  "fhirPath": "Encounter.status",                    "cdaDataType": "CS", "fhirDataType": "code",          "transform": "encounter_status_to_fhir",      "conformance": "SHALL",  "confidence": 0.88},
      {"cdaField": "encounterType",    "fhirPath": "Encounter.class.code",                "cdaDataType": "CS", "fhirDataType": "code",          "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.75}
    ]
  },
  "procedures": {
    "fhirResource": "Procedure",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-procedure",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "procedureCode",    "fhirPath": "Procedure.code.coding[0].code",       "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.92},
      {"cdaField": "procedureDate",    "fhirPath": "Procedure.performedDateTime",          "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHALL",  "confidence": 0.90},
      {"cdaField": "procedureStatus",  "fhirPath": "Procedure.status",                    "cdaDataType": "CS", "fhirDataType": "code",          "transform": "procedure_status_to_fhir",      "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "bodySite",         "fhirPath": "Procedure.bodySite[0].coding[0].code","cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "MAY",    "confidence": 0.70}
    ]
  },
  "careTeam": {
    "fhirResource": "Practitioner",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-practitioner",
    "contextLinks": {},
    "mappings": [
      {"cdaField": "providerName",     "fhirPath": "Practitioner.name[0].family",         "cdaDataType": "PN", "fhirDataType": "string",        "transform": "cda_name_to_fhir",              "conformance": "SHALL",  "confidence": 0.88},
      {"cdaField": "providerNPI",      "fhirPath": "Practitioner.identifier[0].value",    "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.85},
      {"cdaField": "providerRole",     "fhirPath": "Practitioner.qualification[0].code.coding[0].code", "cdaDataType": "CD", "fhirDataType": "code", "transform": "cda_code_to_codeable_concept", "conformance": "SHOULD", "confidence": 0.72}
    ]
  },
  "goals": {
    "fhirResource": "Goal",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-goal",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "goalDescription",  "fhirPath": "Goal.description.text",               "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHALL",  "confidence": 0.90},
      {"cdaField": "goalStatus",       "fhirPath": "Goal.lifecycleStatus",                "cdaDataType": "CS", "fhirDataType": "code",          "transform": "goal_status_to_fhir",           "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "goalTargetDate",   "fhirPath": "Goal.target[0].dueDate",              "cdaDataType": "TS", "fhirDataType": "date",          "transform": "cda_timestamp_to_fhir_date",    "conformance": "SHOULD", "confidence": 0.78}
    ]
  },
  "planOfCare": {
    "fhirResource": "CarePlan",
    "repeating": false,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-careplan",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "planTitle",        "fhirPath": "CarePlan.title",                      "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.80},
      {"cdaField": "planStatus",       "fhirPath": "CarePlan.status",                     "cdaDataType": "CS", "fhirDataType": "code",          "transform": "careplan_status_to_fhir",       "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "planIntent",       "fhirPath": "CarePlan.intent",                     "cdaDataType": "CS", "fhirDataType": "code",          "transform": "string_direct",                 "conformance": "SHALL",  "confidence": 0.82},
      {"cdaField": "planDescription",  "fhirPath": "CarePlan.description",                "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.75}
    ]
  },
  "familyHistory": {
    "fhirResource": "FamilyMemberHistory",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-familymemberhistory",
    "contextLinks": {"patient": "patient"},
    "mappings": [
      {"cdaField": "relationship",     "fhirPath": "FamilyMemberHistory.relationship.coding[0].code", "cdaDataType": "CD", "fhirDataType": "code", "transform": "cda_code_to_codeable_concept", "conformance": "SHALL", "confidence": 0.90},
      {"cdaField": "conditionCode",    "fhirPath": "FamilyMemberHistory.condition[0].code.coding[0].code", "cdaDataType": "CD", "fhirDataType": "code", "transform": "cda_code_to_codeable_concept", "conformance": "SHOULD", "confidence": 0.82},
      {"cdaField": "onsetAge",         "fhirPath": "FamilyMemberHistory.condition[0].onsetAge.value",  "cdaDataType": "PQ", "fhirDataType": "decimal", "transform": "cda_quantity_to_fhir",         "conformance": "MAY",    "confidence": 0.65}
    ]
  },
  "socialHistory": {
    "fhirResource": "Observation",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-social-history",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "socialHistoryCode","fhirPath": "Observation.code.coding[0].code",     "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.88},
      {"cdaField": "socialHistoryValue","fhirPath": "Observation.valueCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHOULD", "confidence": 0.80},
      {"cdaField": "socialHistoryStatus","fhirPath": "Observation.status",                "cdaDataType": "CS", "fhirDataType": "code",          "transform": "observation_status_to_fhir",    "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "socialHistoryDate","fhirPath": "Observation.effectiveDateTime",       "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD", "confidence": 0.78}
    ]
  },
  "functionalStatus": {
    "fhirResource": "Observation",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-clinical-result",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "functionCode",     "fhirPath": "Observation.code.coding[0].code",     "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "functionValue",    "fhirPath": "Observation.valueCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHOULD", "confidence": 0.78},
      {"cdaField": "functionStatus",   "fhirPath": "Observation.status",                  "cdaDataType": "CS", "fhirDataType": "code",          "transform": "observation_status_to_fhir",    "conformance": "SHALL",  "confidence": 0.84}
    ]
  },
  "mentalStatus": {
    "fhirResource": "Observation",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-clinical-result",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "mentalStatusCode", "fhirPath": "Observation.code.coding[0].code",     "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "mentalStatusValue","fhirPath": "Observation.valueCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHOULD", "confidence": 0.78},
      {"cdaField": "mentalStatusDate", "fhirPath": "Observation.effectiveDateTime",       "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD", "confidence": 0.75}
    ]
  },
  "healthConcerns": {
    "fhirResource": "Condition",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-problems-health-concerns",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "concernCode",      "fhirPath": "Condition.code.coding[0].code",       "cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.90},
      {"cdaField": "concernStatus",    "fhirPath": "Condition.clinicalStatus.coding[0].code","cdaDataType": "CS","fhirDataType": "code",         "transform": "condition_status_to_fhir",      "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "concernCategory",  "fhirPath": "Condition.category[0].coding[0].code","cdaDataType": "CD", "fhirDataType": "code",          "transform": "cda_code_to_codeable_concept",  "conformance": "SHOULD", "confidence": 0.75}
    ]
  },
  "payors": {
    "fhirResource": "Coverage",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-coverage",
    "contextLinks": {"beneficiary": "patient"},
    "mappings": [
      {"cdaField": "payorName",        "fhirPath": "Coverage.payor[0].display",           "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHALL",  "confidence": 0.88},
      {"cdaField": "memberId",         "fhirPath": "Coverage.identifier[0].value",        "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.85},
      {"cdaField": "groupId",          "fhirPath": "Coverage.class[0].value",             "cdaDataType": "ST", "fhirDataType": "string",        "transform": "string_direct",                 "conformance": "SHOULD", "confidence": 0.80},
      {"cdaField": "coverageStatus",   "fhirPath": "Coverage.status",                     "cdaDataType": "CS", "fhirDataType": "code",          "transform": "string_direct",                 "conformance": "SHALL",  "confidence": 0.82}
    ]
  },
  "medicalEquipment": {
    "fhirResource": "DeviceUseStatement",
    "repeating": true,
    "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-implantable-device",
    "contextLinks": {"subject": "patient"},
    "mappings": [
      {"cdaField": "deviceCode",       "fhirPath": "DeviceUseStatement.device.display",   "cdaDataType": "CD", "fhirDataType": "string",        "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.85},
      {"cdaField": "deviceStatus",     "fhirPath": "DeviceUseStatement.status",           "cdaDataType": "CS", "fhirDataType": "code",          "transform": "string_direct",                 "conformance": "SHALL",  "confidence": 0.80},
      {"cdaField": "implantDate",      "fhirPath": "DeviceUseStatement.timingDateTime",   "cdaDataType": "TS", "fhirDataType": "dateTime",      "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD", "confidence": 0.75}
    ]
  }
}
$template$::jsonb,
    true  -- create_missing = true
)
WHERE document_type = 'CCD'
  AND ccda_version  = '2.1'
  AND is_default    = true;


-- =========================================================
-- 2.  OOB template: Discharge Summary (C-CDA 2.1 → FHIR R4)
-- =========================================================

INSERT INTO cda_fhir_templates (
    template_name, document_type, ccda_version, fhir_version, us_core_version,
    template_type, template_description, fhir_resources, template_config,
    is_default, is_system, is_public
) VALUES (
    'OOB_DISCHARGE_SUMMARY_21',
    'Discharge Summary',
    '2.1',
    'R4',
    '6.1.0',
    'field_mapping',
    'OOB Discharge Summary C-CDA 2.1 → FHIR R4 US Core 6.1.0. Covers encounter, discharge diagnosis, procedures, medications, results, discharge instructions, and care plan.',
    '["Patient","Encounter","Condition","Procedure","MedicationStatement","Observation","CarePlan"]'::jsonb,
    $template$
{
  "sections": {
    "allergiesAndIntolerances": {
      "fhirResource": "AllergyIntolerance",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-allergyintolerance",
      "contextLinks": {"patient": "patient"},
      "mappings": [
        {"cdaField": "medicationAllergyCode",    "fhirPath": "AllergyIntolerance.code.coding[0].code",          "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.95},
        {"cdaField": "allergyStatus",            "fhirPath": "AllergyIntolerance.clinicalStatus.coding[0].code","cdaDataType": "CS", "fhirDataType": "code",     "transform": "allergy_status_to_fhir",       "conformance": "SHALL",  "confidence": 0.88},
        {"cdaField": "allergyOnset",             "fhirPath": "AllergyIntolerance.onsetDateTime",                "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD","confidence": 0.80}
      ]
    },
    "dischargeDiagnosis": {
      "fhirResource": "Condition",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-encounter-diagnosis",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "diagnosisCode",    "fhirPath": "Condition.code.coding[0].code",          "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.95},
        {"cdaField": "diagnosisStatus",  "fhirPath": "Condition.clinicalStatus.coding[0].code","cdaDataType": "CS", "fhirDataType": "code",     "transform": "condition_status_to_fhir",      "conformance": "SHALL",  "confidence": 0.88},
        {"cdaField": "diagnosisCategory","fhirPath": "Condition.category[0].coding[0].code",  "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.85}
      ]
    },
    "hospitalCourse": {
      "fhirResource": "Encounter",
      "repeating": false,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-encounter",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "encounterDate",    "fhirPath": "Encounter.period.start",    "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime", "conformance": "SHALL",  "confidence": 0.92},
        {"cdaField": "encounterEndDate", "fhirPath": "Encounter.period.end",      "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime", "conformance": "SHALL",  "confidence": 0.90},
        {"cdaField": "encounterStatus",  "fhirPath": "Encounter.status",          "cdaDataType": "CS", "fhirDataType": "code",     "transform": "encounter_status_to_fhir",       "conformance": "SHALL",  "confidence": 0.88}
      ]
    },
    "procedures": {
      "fhirResource": "Procedure",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-procedure",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "procedureCode",   "fhirPath": "Procedure.code.coding[0].code", "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.92},
        {"cdaField": "procedureDate",   "fhirPath": "Procedure.performedDateTime",   "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHALL",  "confidence": 0.90},
        {"cdaField": "procedureStatus", "fhirPath": "Procedure.status",              "cdaDataType": "CS", "fhirDataType": "code",     "transform": "procedure_status_to_fhir",     "conformance": "SHALL",  "confidence": 0.85}
      ]
    },
    "medications": {
      "fhirResource": "MedicationStatement",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-medicationstatement",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "medicationCode",    "fhirPath": "MedicationStatement.medicationCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHALL","confidence": 0.95},
        {"cdaField": "medicationStatus",  "fhirPath": "MedicationStatement.status",  "cdaDataType": "CS", "fhirDataType": "code", "transform": "medication_status_to_fhir", "conformance": "SHALL", "confidence": 0.88},
        {"cdaField": "dosage",            "fhirPath": "MedicationStatement.dosage[0].text", "cdaDataType": "ST", "fhirDataType": "string", "transform": "string_direct", "conformance": "SHOULD", "confidence": 0.75}
      ]
    },
    "dischargeInstructions": {
      "fhirResource": "CarePlan",
      "repeating": false,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-careplan",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "instructionText",  "fhirPath": "CarePlan.description",  "cdaDataType": "ST", "fhirDataType": "string", "transform": "string_direct", "conformance": "SHOULD", "confidence": 0.80},
        {"cdaField": "planStatus",       "fhirPath": "CarePlan.status",       "cdaDataType": "CS", "fhirDataType": "code",   "transform": "careplan_status_to_fhir", "conformance": "SHALL", "confidence": 0.85},
        {"cdaField": "planIntent",       "fhirPath": "CarePlan.intent",       "cdaDataType": "CS", "fhirDataType": "code",   "transform": "string_direct", "conformance": "SHALL", "confidence": 0.82}
      ]
    },
    "results": {
      "fhirResource": "Observation",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-lab",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "testCode",         "fhirPath": "Observation.code.coding[0].code",  "cdaDataType": "CD", "fhirDataType": "code",    "transform": "cda_code_to_codeable_concept",  "conformance": "SHALL",  "confidence": 0.95},
        {"cdaField": "resultValue",      "fhirPath": "Observation.valueQuantity.value",  "cdaDataType": "PQ", "fhirDataType": "decimal",  "transform": "cda_quantity_to_fhir",          "conformance": "SHOULD", "confidence": 0.88},
        {"cdaField": "resultStatus",     "fhirPath": "Observation.status",               "cdaDataType": "CS", "fhirDataType": "code",    "transform": "observation_status_to_fhir",    "conformance": "SHALL",  "confidence": 0.90},
        {"cdaField": "resultDate",       "fhirPath": "Observation.effectiveDateTime",    "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD", "confidence": 0.85}
      ]
    }
  }
}$template$::jsonb,
    true, true, true
) ON CONFLICT DO NOTHING;


-- =========================================================
-- 3.  OOB template: Referral Note (C-CDA 2.1 → FHIR R4)
-- =========================================================

INSERT INTO cda_fhir_templates (
    template_name, document_type, ccda_version, fhir_version, us_core_version,
    template_type, template_description, fhir_resources, template_config,
    is_default, is_system, is_public
) VALUES (
    'OOB_REFERRAL_NOTE_21',
    'Referral Note',
    '2.1',
    'R4',
    '6.1.0',
    'field_mapping',
    'OOB Referral Note C-CDA 2.1 → FHIR R4 US Core 6.1.0. Covers reason for referral, problems, medications, assessment, plan, and social history.',
    '["Patient","Condition","MedicationStatement","Observation","CarePlan","ServiceRequest"]'::jsonb,
    $template$
{
  "sections": {
    "reasonForReferral": {
      "fhirResource": "ServiceRequest",
      "repeating": false,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-servicerequest",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "referralCode",     "fhirPath": "ServiceRequest.code.coding[0].code", "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.88},
        {"cdaField": "referralStatus",   "fhirPath": "ServiceRequest.status",              "cdaDataType": "CS", "fhirDataType": "code",     "transform": "string_direct",               "conformance": "SHALL",  "confidence": 0.85},
        {"cdaField": "referralIntent",   "fhirPath": "ServiceRequest.intent",              "cdaDataType": "CS", "fhirDataType": "code",     "transform": "string_direct",               "conformance": "SHALL",  "confidence": 0.82},
        {"cdaField": "referralDate",     "fhirPath": "ServiceRequest.occurrenceDateTime",  "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD","confidence": 0.80},
        {"cdaField": "referralNote",     "fhirPath": "ServiceRequest.note[0].text",        "cdaDataType": "ST", "fhirDataType": "string",   "transform": "string_direct",               "conformance": "MAY",    "confidence": 0.72}
      ]
    },
    "problems": {
      "fhirResource": "Condition",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-problems-health-concerns",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "problemCode",      "fhirPath": "Condition.code.coding[0].code",           "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.95},
        {"cdaField": "problemStatus",    "fhirPath": "Condition.clinicalStatus.coding[0].code", "cdaDataType": "CS", "fhirDataType": "code",     "transform": "condition_status_to_fhir",     "conformance": "SHALL",  "confidence": 0.88},
        {"cdaField": "problemOnsetDate", "fhirPath": "Condition.onsetDateTime",                 "cdaDataType": "TS", "fhirDataType": "dateTime", "transform": "cda_timestamp_to_fhir_datetime","conformance": "SHOULD","confidence": 0.82}
      ]
    },
    "medications": {
      "fhirResource": "MedicationStatement",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-medicationstatement",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "medicationCode",   "fhirPath": "MedicationStatement.medicationCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHALL","confidence": 0.95},
        {"cdaField": "medicationStatus", "fhirPath": "MedicationStatement.status", "cdaDataType": "CS", "fhirDataType": "code", "transform": "medication_status_to_fhir", "conformance": "SHALL", "confidence": 0.88}
      ]
    },
    "assessmentAndPlan": {
      "fhirResource": "CarePlan",
      "repeating": false,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-careplan",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "assessmentText",   "fhirPath": "CarePlan.description",  "cdaDataType": "ST", "fhirDataType": "string", "transform": "string_direct", "conformance": "SHOULD", "confidence": 0.80},
        {"cdaField": "planStatus",       "fhirPath": "CarePlan.status",       "cdaDataType": "CS", "fhirDataType": "code",   "transform": "careplan_status_to_fhir", "conformance": "SHALL", "confidence": 0.85},
        {"cdaField": "planIntent",       "fhirPath": "CarePlan.intent",       "cdaDataType": "CS", "fhirDataType": "code",   "transform": "string_direct", "conformance": "SHALL",  "confidence": 0.82}
      ]
    },
    "allergiesAndIntolerances": {
      "fhirResource": "AllergyIntolerance",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-allergyintolerance",
      "contextLinks": {"patient": "patient"},
      "mappings": [
        {"cdaField": "medicationAllergyCode", "fhirPath": "AllergyIntolerance.code.coding[0].code",           "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.95},
        {"cdaField": "allergyStatus",         "fhirPath": "AllergyIntolerance.clinicalStatus.coding[0].code", "cdaDataType": "CS", "fhirDataType": "code",     "transform": "allergy_status_to_fhir",       "conformance": "SHALL",  "confidence": 0.88}
      ]
    },
    "socialHistory": {
      "fhirResource": "Observation",
      "repeating": true,
      "useCoreProfile": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-social-history",
      "contextLinks": {"subject": "patient"},
      "mappings": [
        {"cdaField": "socialHistoryCode",  "fhirPath": "Observation.code.coding[0].code",  "cdaDataType": "CD", "fhirDataType": "code",     "transform": "cda_code_to_codeable_concept", "conformance": "SHALL",  "confidence": 0.88},
        {"cdaField": "socialHistoryValue", "fhirPath": "Observation.valueCodeableConcept.coding[0].code","cdaDataType": "CD","fhirDataType": "code","transform": "cda_code_to_codeable_concept","conformance": "SHOULD","confidence": 0.80},
        {"cdaField": "socialHistoryStatus","fhirPath": "Observation.status", "cdaDataType": "CS", "fhirDataType": "code", "transform": "observation_status_to_fhir", "conformance": "SHALL", "confidence": 0.85}
      ]
    }
  }
}$template$::jsonb,
    true, true, true
) ON CONFLICT DO NOTHING;
