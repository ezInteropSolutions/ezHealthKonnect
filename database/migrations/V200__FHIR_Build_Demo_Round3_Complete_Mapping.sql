-- V200: FHIR Build Demo — Round 3 complete field mapping + cross-references
--
-- Every step in the demo already satisfied FHIR R4's actual required-field
-- bar (verified via the fixed, cardinality-aware fhir.build completeness
-- banner — see FHIRRequirementsHelper.js). This round goes beyond bare
-- compliance: each of the 10 resources gets a more complete, realistic set
-- of fields, AND Practitioner/Organization/Location — built but never
-- referenced by anything else in the Bundle before this round — now get
-- their own "id" (sourced from the same natural identifier already used
-- elsewhere: practitionerNPI/orgNPI/a new locationId) so Encounter,
-- Condition, Observation, MedicationRequest, AllergyIntolerance, and
-- Immunization can reference them by a resolvable "ResourceType/id".
--
-- Deliberately NOT mapped: Patient.active, Practitioner.active,
-- Organization.active, Immunization.primarySource. fhir.build's
-- literalValue is always a Go string (see fhir_build_executor.go's
-- fhirFieldMappingRow.LiteralValue), so a literal for a boolean field would
-- render as the JSON STRING "true", not the boolean true — a real type
-- defect the current validator doesn't happen to catch, but not one to ship
-- into this demo. Proper fix is a future no-code booleanLiteral-style
-- addition to fhir.build itself, out of scope here.
--
-- New source fields consumed (see tests/fixtures/fhir_build_demo_sample_message.json):
-- patientFamily, patientGiven, patientPhone, patientAddress{Line,City,State,PostalCode},
-- encounterPeriodStart, encounterPeriodEnd, practitionerPhone, practitionerGender,
-- orgPhone, locationId.

-- Patient: name, telecom, address.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "name[0].family", "sourcePath": "message.patientFamily"},
        {"targetPath": "name[0].given[0]", "sourcePath": "message.patientGiven"},
        {"targetPath": "telecom[0].system", "literalValue": "phone"},
        {"targetPath": "telecom[0].value", "sourcePath": "message.patientPhone"},
        {"targetPath": "address[0].line[0]", "sourcePath": "message.patientAddressLine"},
        {"targetPath": "address[0].city", "sourcePath": "message.patientAddressCity"},
        {"targetPath": "address[0].state", "sourcePath": "message.patientAddressState"},
        {"targetPath": "address[0].postalCode", "sourcePath": "message.patientAddressPostalCode"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Patient'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'name[0].family'
  );

-- Encounter: period, reasonCode, participant (Practitioner), serviceProvider
-- (Organization), location (Location).
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "period.start", "sourcePath": "message.encounterPeriodStart"},
        {"targetPath": "period.end", "sourcePath": "message.encounterPeriodEnd"},
        {"targetPath": "reasonCode[0].coding[0].system", "sourcePath": "message.conditionCodeSystem"},
        {"targetPath": "reasonCode[0].coding[0].code", "sourcePath": "message.conditionCode"},
        {"targetPath": "participant[0].individual.reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "serviceProvider.reference", "sourcePath": "message.orgNPI", "transform": "string_prefix", "valueMap": {"prefix": "Organization/"}},
        {"targetPath": "location[0].location.reference", "sourcePath": "message.locationId", "transform": "string_prefix", "valueMap": {"prefix": "Location/"}}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Encounter'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'period.start'
  );

-- Condition: recorder (Practitioner), category, verificationStatus.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "recorder.reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "category[0].coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/condition-category"},
        {"targetPath": "category[0].coding[0].code", "literalValue": "problem-list-item"},
        {"targetPath": "verificationStatus.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/condition-ver-status"},
        {"targetPath": "verificationStatus.coding[0].code", "literalValue": "confirmed"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Condition'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'recorder.reference'
  );

-- Observation: performer (Practitioner), interpretation.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "performer[0].reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "interpretation[0].coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation"},
        {"targetPath": "interpretation[0].coding[0].code", "literalValue": "N"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Observation'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'performer[0].reference'
  );

-- MedicationRequest: requester (Practitioner), authoredOn, dosageInstruction.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "requester.reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "authoredOn", "sourcePath": "message.encounterPeriodStart"},
        {"targetPath": "dosageInstruction[0].text", "literalValue": "Take 1 tablet by mouth once daily"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR MedicationRequest'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'requester.reference'
  );

-- AllergyIntolerance: recorder (Practitioner), type, category, criticality.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "recorder.reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "type", "literalValue": "allergy"},
        {"targetPath": "category[0]", "literalValue": "medication"},
        {"targetPath": "criticality", "literalValue": "low"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR AllergyIntolerance'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'recorder.reference'
  );

-- Immunization: performer (Practitioner), location (Location), lotNumber.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "performer[0].actor.reference", "sourcePath": "message.practitionerNPI", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/"}},
        {"targetPath": "location.reference", "sourcePath": "message.locationId", "transform": "string_prefix", "valueMap": {"prefix": "Location/"}},
        {"targetPath": "lotNumber", "literalValue": "LOT12345"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Immunization'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'performer[0].actor.reference'
  );

-- Practitioner: id (referenceability), gender, telecom.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "id", "sourcePath": "message.practitionerNPI"},
        {"targetPath": "gender", "sourcePath": "message.practitionerGender"},
        {"targetPath": "telecom[0].system", "literalValue": "phone"},
        {"targetPath": "telecom[0].value", "sourcePath": "message.practitionerPhone"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Practitioner'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'id'
  );

-- Organization: id (referenceability), type, telecom.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "id", "sourcePath": "message.orgNPI"},
        {"targetPath": "type[0].coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/organization-type"},
        {"targetPath": "type[0].coding[0].code", "literalValue": "prov"},
        {"targetPath": "telecom[0].system", "literalValue": "phone"},
        {"targetPath": "telecom[0].value", "sourcePath": "message.orgPhone"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Organization'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'id'
  );

-- Location: id (referenceability), managingOrganization (Organization), physicalType.
UPDATE transformation_steps ts
SET config = jsonb_set(
    ts.config,
    '{fields}',
    (ts.config->'fields') || '[
        {"targetPath": "id", "sourcePath": "message.locationId"},
        {"targetPath": "managingOrganization.reference", "sourcePath": "message.orgNPI", "transform": "string_prefix", "valueMap": {"prefix": "Organization/"}},
        {"targetPath": "physicalType.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/location-physical-type"},
        {"targetPath": "physicalType.coding[0].code", "literalValue": "ro"}
    ]'::jsonb
)
FROM transformation_pipelines tp
WHERE ts.pipeline_id = tp.id
  AND tp.interface_id = '8952d74e-9c81-4bae-a1e9-abd83ca094a6'
  AND ts.step_name = 'Build FHIR Location'
  AND ts.step_type = 'fhir.build'
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(ts.config->'fields') f
      WHERE f->>'targetPath' = 'id'
  );
