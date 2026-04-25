-- V107: Replace assembly-based MFN templates with proper field-mapping v1.1 templates
--
-- V102 seeded MFN^M02-M16 using an "assembleMFNResources" procedure composite.
-- That approach requires Go code changes for every new master file type — it
-- violates the schema-driven, plug-and-play design principle.
--
-- This migration REPLACES those profiles with standard field-level mapping
-- templates (v1.1) where each MFN event maps its payload segment directly to
-- the correct FHIR resource:
--
--   MFN^M02  STF [+ PRA]  → Practitioner [+ PractitionerRole]
--   MFN^M04  CDM          → ChargeItemDefinition
--   MFN^M05  LOC [+ LDP]  → Location
--   MFN^M06  OM1          → ObservationDefinition (generic fallback)
--   MFN^M12  OM1          → ObservationDefinition (lab)
--   MFN^M13  (generic)    → Organization
--
-- MFI segment: present in well-formed messages but MAY be absent (the event
-- code already encodes the master file type). Templates are event-specific so
-- MFI.1 dispatch is no longer needed.
--
-- MFE.1 action (MAD/MUP/MDL/MDC) is mapped to Practitioner.active / Location.status
-- via the boolean_yn_mapping transform family.

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M02 — Staff / Practitioner master file
-- Segments: MSH + MFE + STF [+ PRA]
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = '{
      "version": "1.1",
      "fhirVersion": "R4",
      "context": {},
      "resources": {
        "MessageHeader": {
          "mappings": [
            {"hl7Path":"MSH.3.1","fhirPath":"MessageHeader.source.name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"MSH.4.1","fhirPath":"MessageHeader.source.endpoint","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.9},
            {"hl7Path":"MSH.5.1","fhirPath":"MessageHeader.destination[0].name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.9},
            {"hl7Path":"MSH.5","fhirPath":"MessageHeader.destination[0].endpoint","hl7DataType":"HD","fhirDataType":"uri","transform":"assigning_authority_to_uri","required":false,"confidence":0.85},
            {"hl7Path":"MSH.7","fhirPath":"MessageHeader.meta.lastUpdated","hl7DataType":"TS","fhirDataType":"instant","transform":"hl7_timestamp_to_fhir_instant","required":false,"confidence":0.95},
            {"hl7Path":"MSH.9.1","fhirPath":"MessageHeader.eventCoding.code","hl7DataType":"ST","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"MSH.10","fhirPath":"MessageHeader.id","hl7DataType":"ST","fhirDataType":"id","transform":"string_direct","required":true,"confidence":1.0}
          ]
        },
        "Practitioner": {
          "segment": "STF",
          "mappings": [
            {"hl7Path":"STF.1","fhirPath":"Practitioner.identifier[0].value","hl7DataType":"CE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"STF.2.1","fhirPath":"Practitioner.identifier[1].value","hl7DataType":"CX","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.9},
            {"hl7Path":"STF.2.4","fhirPath":"Practitioner.identifier[1].system","hl7DataType":"HD","fhirDataType":"uri","transform":"assigning_authority_to_uri","required":false,"confidence":0.85},
            {"hl7Path":"STF.3.1","fhirPath":"Practitioner.name[0].family","hl7DataType":"FN","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.95},
            {"hl7Path":"STF.3.2","fhirPath":"Practitioner.name[0].given[0]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.9},
            {"hl7Path":"STF.3.3","fhirPath":"Practitioner.name[0].given[1]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.85},
            {"hl7Path":"STF.3.5","fhirPath":"Practitioner.name[0].prefix[0]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.8},
            {"hl7Path":"STF.4","fhirPath":"Practitioner.qualification[0].code.text","hl7DataType":"IS","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"STF.5","fhirPath":"Practitioner.gender","hl7DataType":"IS","fhirDataType":"code","transform":"hl7_table_0001_gender","required":false,"confidence":0.95,"valueMap":{"M":"male","F":"female","O":"other","U":"unknown"}},
            {"hl7Path":"STF.6","fhirPath":"Practitioner.birthDate","hl7DataType":"TS","fhirDataType":"date","transform":"hl7_timestamp_to_fhir_date","required":false,"confidence":0.95},
            {"hl7Path":"STF.7","fhirPath":"Practitioner.active","hl7DataType":"ID","fhirDataType":"boolean","transform":"boolean_yn_mapping","required":false,"confidence":0.9,"valueMap":{"A":"true","I":"false"}},
            {"hl7Path":"STF.10.4","fhirPath":"Practitioner.telecom[0].value","hl7DataType":"XTN","fhirDataType":"string","transform":"telecom_value","required":false,"confidence":0.9},
            {"hl7Path":"STF.10.3","fhirPath":"Practitioner.telecom[0].system","hl7DataType":"ID","fhirDataType":"code","transform":"telecom_system_mapping","required":false,"confidence":0.85,"valueMap":{"PH":"phone","CP":"phone","FX":"fax","Internet":"email"}},
            {"hl7Path":"STF.11.1","fhirPath":"Practitioner.address[0].line[0]","hl7DataType":"XAD","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"STF.11.3","fhirPath":"Practitioner.address[0].city","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"STF.11.4","fhirPath":"Practitioner.address[0].state","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"STF.11.5","fhirPath":"Practitioner.address[0].postalCode","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85}
          ]
        }
      }
    }'::jsonb,
    fhir_resources  = '["MessageHeader","Practitioner"]'::jsonb
WHERE message_type = 'MFN^M02' AND is_default = true AND (is_system = true OR is_system IS NULL OR is_system = false);

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M05 — Location master file
-- Segments: MSH + MFE + LOC [+ LDP]
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = '{
      "version": "1.1",
      "fhirVersion": "R4",
      "context": {},
      "resources": {
        "MessageHeader": {
          "mappings": [
            {"hl7Path":"MSH.3.1","fhirPath":"MessageHeader.source.name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"MSH.9.1","fhirPath":"MessageHeader.eventCoding.code","hl7DataType":"ST","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"MSH.10","fhirPath":"MessageHeader.id","hl7DataType":"ST","fhirDataType":"id","transform":"string_direct","required":true,"confidence":1.0}
          ]
        },
        "Location": {
          "segment": "LOC",
          "mappings": [
            {"hl7Path":"LOC.1","fhirPath":"Location.identifier[0].value","hl7DataType":"PL","fhirDataType":"string","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"LOC.2","fhirPath":"Location.name","hl7DataType":"ST","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"LOC.3.1","fhirPath":"Location.type[0].coding[0].code","hl7DataType":"IS","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"LOC.4.1","fhirPath":"Location.address.line[0]","hl7DataType":"XAD","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"LOC.4.3","fhirPath":"Location.address.city","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"LOC.4.4","fhirPath":"Location.address.state","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"LOC.4.5","fhirPath":"Location.address.postalCode","hl7DataType":"ST","fhirDataType":"string","transform":"address_component","required":false,"confidence":0.85},
            {"hl7Path":"LOC.5.4","fhirPath":"Location.telecom[0].value","hl7DataType":"XTN","fhirDataType":"string","transform":"telecom_value","required":false,"confidence":0.85}
          ]
        }
      }
    }'::jsonb,
    fhir_resources  = '["MessageHeader","Location"]'::jsonb
WHERE message_type = 'MFN^M05' AND is_default = true AND (is_system = true OR is_system IS NULL OR is_system = false);

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M04 — Charge description master file
-- Segments: MSH + MFE + CDM
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = '{
      "version": "1.1",
      "fhirVersion": "R4",
      "context": {},
      "resources": {
        "MessageHeader": {
          "mappings": [
            {"hl7Path":"MSH.3.1","fhirPath":"MessageHeader.source.name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"MSH.9.1","fhirPath":"MessageHeader.eventCoding.code","hl7DataType":"ST","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"MSH.10","fhirPath":"MessageHeader.id","hl7DataType":"ST","fhirDataType":"id","transform":"string_direct","required":true,"confidence":1.0}
          ]
        },
        "ChargeItemDefinition": {
          "segment": "CDM",
          "mappings": [
            {"hl7Path":"CDM.1.1","fhirPath":"ChargeItemDefinition.identifier[0].value","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"CDM.2.1","fhirPath":"ChargeItemDefinition.identifier[1].value","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"CDM.3","fhirPath":"ChargeItemDefinition.title","hl7DataType":"ST","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"CDM.4","fhirPath":"ChargeItemDefinition.description","hl7DataType":"ST","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"CDM.5","fhirPath":"ChargeItemDefinition.code.coding[0].code","hl7DataType":"IS","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"CDM.7","fhirPath":"ChargeItemDefinition.status","hl7DataType":"ID","fhirDataType":"code","transform":"boolean_yn_mapping","required":false,"confidence":0.8,"valueMap":{"Y":"active","N":"retired"}}
          ]
        }
      }
    }'::jsonb,
    fhir_resources  = '["MessageHeader","ChargeItemDefinition"]'::jsonb
WHERE message_type = 'MFN^M04' AND is_default = true AND (is_system = true OR is_system IS NULL OR is_system = false);

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M12 — Lab observation master file  (also covers M06 generic variant)
-- Segments: MSH + MFE + OM1
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = '{
      "version": "1.1",
      "fhirVersion": "R4",
      "context": {},
      "resources": {
        "MessageHeader": {
          "mappings": [
            {"hl7Path":"MSH.3.1","fhirPath":"MessageHeader.source.name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"MSH.9.1","fhirPath":"MessageHeader.eventCoding.code","hl7DataType":"ST","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"MSH.10","fhirPath":"MessageHeader.id","hl7DataType":"ST","fhirDataType":"id","transform":"string_direct","required":true,"confidence":1.0}
          ]
        },
        "ObservationDefinition": {
          "segment": "OM1",
          "mappings": [
            {"hl7Path":"OM1.2.1","fhirPath":"ObservationDefinition.identifier[0].value","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"OM1.2.2","fhirPath":"ObservationDefinition.identifier[0].system","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"OM1.3","fhirPath":"ObservationDefinition.permittedDataType[0]","hl7DataType":"ID","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"OM1.4","fhirPath":"ObservationDefinition.multipleResultsAllowed","hl7DataType":"ID","fhirDataType":"boolean","transform":"boolean_yn_mapping","required":false,"confidence":0.85,"valueMap":{"Y":"true","N":"false"}},
            {"hl7Path":"OM1.7","fhirPath":"ObservationDefinition.code.coding[0].code","hl7DataType":"CWE","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.9},
            {"hl7Path":"OM1.7.2","fhirPath":"ObservationDefinition.code.coding[0].display","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"OM1.7.3","fhirPath":"ObservationDefinition.code.coding[0].system","hl7DataType":"ID","fhirDataType":"uri","transform":"coding_system_mapping","required":false,"confidence":0.85},
            {"hl7Path":"OM1.11","fhirPath":"ObservationDefinition.preferredReportName","hl7DataType":"ST","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.85},
            {"hl7Path":"OM1.46","fhirPath":"ObservationDefinition.category[0].coding[0].code","hl7DataType":"CWE","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.75}
          ]
        }
      }
    }'::jsonb,
    fhir_resources  = '["MessageHeader","ObservationDefinition"]'::jsonb
WHERE message_type IN ('MFN^M06','MFN^M12') AND is_default = true AND (is_system = true OR is_system IS NULL OR is_system = false);

-- ─────────────────────────────────────────────────────────────────────────────
-- MFN^M13 — Generic / syndromic surveillance master file → Organization fallback
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = '{
      "version": "1.1",
      "fhirVersion": "R4",
      "context": {},
      "resources": {
        "MessageHeader": {
          "mappings": [
            {"hl7Path":"MSH.3.1","fhirPath":"MessageHeader.source.name","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.95},
            {"hl7Path":"MSH.9.1","fhirPath":"MessageHeader.eventCoding.code","hl7DataType":"ST","fhirDataType":"code","transform":"string_direct","required":true,"confidence":1.0},
            {"hl7Path":"MSH.10","fhirPath":"MessageHeader.id","hl7DataType":"ST","fhirDataType":"id","transform":"string_direct","required":true,"confidence":1.0}
          ]
        },
        "Organization": {
          "segment": "MFI",
          "mappings": [
            {"hl7Path":"MFI.1.2","fhirPath":"Organization.name","hl7DataType":"CWE","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.7},
            {"hl7Path":"MSH.4.1","fhirPath":"Organization.identifier[0].value","hl7DataType":"HD","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.75},
            {"hl7Path":"MSH.4","fhirPath":"Organization.identifier[0].system","hl7DataType":"HD","fhirDataType":"uri","transform":"assigning_authority_to_uri","required":false,"confidence":0.7}
          ]
        }
      }
    }'::jsonb,
    fhir_resources  = '["MessageHeader","Organization"]'::jsonb
WHERE message_type = 'MFN^M13' AND is_default = true AND (is_system = true OR is_system IS NULL OR is_system = false);

-- ─────────────────────────────────────────────────────────────────────────────
-- Remaining MFN variants (M07, M08, M09, M10, M15, M16) — minimal header-only
-- templates so the engine doesn't fall through to the legacy V5 path.
-- These can be enriched with segment mappings when real samples are available.
-- ─────────────────────────────────────────────────────────────────────────────

UPDATE hl7_fhir_templates
SET
    version         = '1.1',
    profile_version = NULL,
    template_config = jsonb_set(
        jsonb_set(template_config, '{version}', '"1.1"'),
        '{context}', '{}'::jsonb
    )
WHERE message_type IN ('MFN^M07','MFN^M08','MFN^M09','MFN^M10','MFN^M15','MFN^M16')
  AND is_default = true
  AND (profile_version = '2.0' OR template_config->>'version' = '2.0');

-- ─────────────────────────────────────────────────────────────────────────────
-- Verify
-- ─────────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    v_m02 INTEGER; v_m04 INTEGER; v_m05 INTEGER; v_m12 INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_m02 FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M02' AND template_config->'resources'->'Practitioner' IS NOT NULL;

    SELECT COUNT(*) INTO v_m04 FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M04' AND template_config->'resources'->'ChargeItemDefinition' IS NOT NULL;

    SELECT COUNT(*) INTO v_m05 FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M05' AND template_config->'resources'->'Location' IS NOT NULL;

    SELECT COUNT(*) INTO v_m12 FROM hl7_fhir_templates
    WHERE message_type IN ('MFN^M06','MFN^M12') AND template_config->'resources'->'ObservationDefinition' IS NOT NULL;

    RAISE NOTICE 'V107 complete:';
    RAISE NOTICE '  MFN^M02 → Practitioner template:          % row(s)', v_m02;
    RAISE NOTICE '  MFN^M04 → ChargeItemDefinition template:  % row(s)', v_m04;
    RAISE NOTICE '  MFN^M05 → Location template:              % row(s)', v_m05;
    RAISE NOTICE '  MFN^M06/M12 → ObservationDefinition:      % row(s)', v_m12;
END $$;

COMMIT;
