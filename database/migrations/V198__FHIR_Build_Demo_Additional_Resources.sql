-- V198: FHIR Build Demo — Additional Resources
--
-- Round 1 of FHIR build validation (mirroring the CDA_Build_Demo external-
-- validator effort): expands FHIR_Build_Demo from a single Patient-only
-- fhir.build step into 10 resources (Patient, Encounter, Condition,
-- Observation, MedicationRequest, AllergyIntolerance, Immunization,
-- Practitioner, Organization, Location) assembled into one FHIR Bundle via
-- payload.builder's fhir_bundle mode.
--
-- Cross-resource references (Encounter.subject, Condition.subject,
-- Observation.subject/encounter, MedicationRequest.subject,
-- AllergyIntolerance.patient, Immunization.patient) are sourced from the
-- SAME raw fields Patient.id/Encounter.id already use for themselves (via
-- the new string_prefix transform), not a second hardcoded copy — avoids the
-- two values silently drifting apart (fhir_validation only warns, never
-- errors, on a mismatched reference).
--
-- Step configs here are kept byte-for-byte consistent with
-- services/executors/transform/fhir_build_bundle_assembly_test.go and
-- tests/playwright/fhir-hl7-build-e2e.spec.js's FHB-E2-004/005 — all three
-- read the same tests/fixtures/fhir_build_demo_sample_message.json shape.
--
-- No-ops safely if the FHIR_Build_Demo interface/pipeline doesn't exist in
-- this environment (fresh install without demo data).

-- ── Step 1: existing Patient step needs its own distinct output field ──────
-- (was "message.fhirResource" — generic; the other 9 new steps each need
-- their own field so later steps don't overwrite earlier ones' output).
UPDATE transformation_steps ts
SET config = jsonb_set(ts.config, '{outputField}', '"message.fhirPatient"'::jsonb)
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE ts.pipeline_id = tp.id
  AND i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_type = 'fhir.build'
  AND ts.sequence = 10;

-- ── Step 2: Encounter ───────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Encounter', 'fhir.build', 20,
    '{
        "resourceType": "Encounter", "profile": "base", "version": "R4",
        "outputField": "message.fhirEncounter",
        "fields": [
            {"targetPath": "id", "sourcePath": "message.encounterId"},
            {"targetPath": "status", "literalValue": "finished"},
            {"targetPath": "class.system", "literalValue": "http://terminology.hl7.org/CodeSystem/v3-ActCode"},
            {"targetPath": "class.code", "literalValue": "AMB"},
            {"targetPath": "subject.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Encounter');

-- ── Step 3: Condition ────────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Condition', 'fhir.build', 30,
    '{
        "resourceType": "Condition", "profile": "base", "version": "R4",
        "outputField": "message.fhirCondition",
        "fields": [
            {"targetPath": "clinicalStatus.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/condition-clinical"},
            {"targetPath": "clinicalStatus.coding[0].code", "literalValue": "active"},
            {"targetPath": "code.coding[0].system", "sourcePath": "message.conditionCodeSystem"},
            {"targetPath": "code.coding[0].code", "sourcePath": "message.conditionCode"},
            {"targetPath": "subject.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Condition');

-- ── Step 4: Observation ──────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Observation', 'fhir.build', 40,
    '{
        "resourceType": "Observation", "profile": "base", "version": "R4",
        "outputField": "message.fhirObservation",
        "fields": [
            {"targetPath": "status", "literalValue": "final"},
            {"targetPath": "code.coding[0].system", "literalValue": "http://loinc.org"},
            {"targetPath": "code.coding[0].code", "sourcePath": "message.observationCode"},
            {"targetPath": "valueQuantity.value", "sourcePath": "message.observationValue", "transform": "cda_decimal_string_to_number"},
            {"targetPath": "valueQuantity.unit", "sourcePath": "message.observationUnit"},
            {"targetPath": "subject.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}},
            {"targetPath": "encounter.reference", "sourcePath": "message.encounterId", "transform": "string_prefix", "valueMap": {"prefix": "Encounter/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Observation');

-- ── Step 5: MedicationRequest ────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR MedicationRequest', 'fhir.build', 50,
    '{
        "resourceType": "MedicationRequest", "profile": "base", "version": "R4",
        "outputField": "message.fhirMedicationRequest",
        "fields": [
            {"targetPath": "status", "literalValue": "active"},
            {"targetPath": "intent", "literalValue": "order"},
            {"targetPath": "medicationCodeableConcept.coding[0].system", "literalValue": "http://www.nlm.nih.gov/research/umls/rxnorm"},
            {"targetPath": "medicationCodeableConcept.coding[0].code", "sourcePath": "message.drugCode"},
            {"targetPath": "subject.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR MedicationRequest');

-- ── Step 6: AllergyIntolerance ───────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR AllergyIntolerance', 'fhir.build', 60,
    '{
        "resourceType": "AllergyIntolerance", "profile": "base", "version": "R4",
        "outputField": "message.fhirAllergyIntolerance",
        "fields": [
            {"targetPath": "clinicalStatus.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"},
            {"targetPath": "clinicalStatus.coding[0].code", "literalValue": "active"},
            {"targetPath": "code.coding[0].system", "literalValue": "http://snomed.info/sct"},
            {"targetPath": "code.coding[0].code", "sourcePath": "message.allergenCode"},
            {"targetPath": "patient.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR AllergyIntolerance');

-- ── Step 7: Immunization ─────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Immunization', 'fhir.build', 70,
    '{
        "resourceType": "Immunization", "profile": "base", "version": "R4",
        "outputField": "message.fhirImmunization",
        "fields": [
            {"targetPath": "status", "literalValue": "completed"},
            {"targetPath": "vaccineCode.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/cvx"},
            {"targetPath": "vaccineCode.coding[0].code", "sourcePath": "message.vaccineCode"},
            {"targetPath": "occurrenceDateTime", "sourcePath": "message.immunizationDate"},
            {"targetPath": "patient.reference", "sourcePath": "message.patientIdentifiers[0].idValue", "transform": "string_prefix", "valueMap": {"prefix": "Patient/"}}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Immunization');

-- ── Step 8: Practitioner ─────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Practitioner', 'fhir.build', 80,
    '{
        "resourceType": "Practitioner", "profile": "base", "version": "R4",
        "outputField": "message.fhirPractitioner",
        "fields": [
            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
            {"targetPath": "identifier[0].value", "sourcePath": "message.practitionerNPI"},
            {"targetPath": "name[0].family", "sourcePath": "message.practitionerFamily"},
            {"targetPath": "name[0].given[0]", "sourcePath": "message.practitionerGiven"}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Practitioner');

-- ── Step 9: Organization ─────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Organization', 'fhir.build', 90,
    '{
        "resourceType": "Organization", "profile": "base", "version": "R4",
        "outputField": "message.fhirOrganization",
        "fields": [
            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
            {"targetPath": "identifier[0].value", "sourcePath": "message.orgNPI"},
            {"targetPath": "name", "sourcePath": "message.orgName"}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Organization');

-- ── Step 10: Location ────────────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Build FHIR Location', 'fhir.build', 100,
    '{
        "resourceType": "Location", "profile": "base", "version": "R4",
        "outputField": "message.fhirLocation",
        "fields": [
            {"targetPath": "status", "literalValue": "active"},
            {"targetPath": "name", "sourcePath": "message.locationName"}
        ]
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Build FHIR Location');

-- ── Step 11: Bundle assembly ─────────────────────────────────────────────────
INSERT INTO transformation_steps (id, pipeline_id, step_name, step_type, sequence, config, enabled)
SELECT
    gen_random_uuid(), tp.id, 'Assemble Bundle', 'payload.builder', 110,
    '{
        "mode": "fhir_bundle",
        "fhirBundle": {
            "bundleType": "collection",
            "resourcePaths": [
                "message.fhirPatient", "message.fhirEncounter", "message.fhirCondition",
                "message.fhirObservation", "message.fhirMedicationRequest", "message.fhirAllergyIntolerance",
                "message.fhirImmunization", "message.fhirPractitioner", "message.fhirOrganization", "message.fhirLocation"
            ]
        }
    }'::jsonb,
    true
FROM transformation_pipelines tp
JOIN interfaces i ON i.id = tp.interface_id
WHERE i.id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND NOT EXISTS (SELECT 1 FROM transformation_steps ts WHERE ts.pipeline_id = tp.id AND ts.step_name = 'Assemble Bundle');
