-- V113: Fix MFN^M02 FHIR template
--
-- Addresses two categories of logic gaps found during universal interface testing:
--
-- 1. PractitionerRole.language received raw HL7 composite key (PRA.1) because the
--    OOB schema generator had no explicit anchor for PRA.1 and fell through to
--    DomainResource.language (a code field). The fix adds explicit PRA mappings
--    and removes the incorrect language entry if present.
--
-- 2. Practitioner.name was missing because the whole-field STF.3→HumanName anchor
--    did not expand XPN subfields. Explicit STF.3.1/3.2/3.5 mappings are added.
--
-- These changes update the live template so the fix is effective immediately
-- without requiring an OOB rebuild.

BEGIN;

UPDATE hl7_fhir_templates
SET
    version         = '1.2',
    template_config = jsonb_set(
        jsonb_set(
            template_config,
            '{version}',
            '"1.2"'
        ),
        '{resources}',
        '{
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
              {"hl7Path":"STF.3.1","fhirPath":"Practitioner.name[0].family","hl7DataType":"FN","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.97},
              {"hl7Path":"STF.3.2","fhirPath":"Practitioner.name[0].given[0]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.95},
              {"hl7Path":"STF.3.3","fhirPath":"Practitioner.name[0].given[1]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.88},
              {"hl7Path":"STF.3.5","fhirPath":"Practitioner.name[0].prefix[0]","hl7DataType":"ST","fhirDataType":"string","transform":"name_component","required":false,"confidence":0.90},
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
          },
          "PractitionerRole": {
            "segment": "PRA",
            "mappings": [
              {"hl7Path":"PRA.3","fhirPath":"PractitionerRole.specialty[0].coding[0].code","hl7DataType":"IS","fhirDataType":"code","transform":"string_direct","required":false,"confidence":0.92},
              {"hl7Path":"PRA.3","fhirPath":"PractitionerRole.specialty[0].coding[0].system","hl7DataType":"IS","fhirDataType":"uri","transform":"string_literal","literalValue":"http://snomed.info/sct","required":false,"confidence":0.75},
              {"hl7Path":"PRA.6.1","fhirPath":"PractitionerRole.identifier[0].value","hl7DataType":"CM_PLN","fhirDataType":"string","transform":"string_direct","required":false,"confidence":0.88},
              {"hl7Path":"PRA.6.3","fhirPath":"PractitionerRole.identifier[0].system","hl7DataType":"ID","fhirDataType":"uri","transform":"assigning_authority_to_uri","required":false,"confidence":0.82}
            ]
          }
        }'::jsonb
    ),
    fhir_resources  = '["MessageHeader","Practitioner","PractitionerRole"]'::jsonb
WHERE message_type = 'MFN^M02'
  AND is_default = true
  AND (is_system = true OR is_system IS NULL OR is_system = false);

DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M02'
      AND template_config->'resources'->'PractitionerRole' IS NOT NULL
      AND template_config->>'version' = '1.2';

    RAISE NOTICE 'V113 complete: MFN^M02 template updated (% row(s) at v1.2 with PractitionerRole block)', v_count;
END $$;

COMMIT;
