-- V172__CDA_Declarative_Mapping_Rules_Medication_Requester.sql
--
-- Re-seeds the Medications section's MedicationRequest row to add a 2-tier
-- PractitionerRole-emitting requester (performer, then author) ahead of the
-- existing display-only fallback row, mirroring
-- services/cda_fhir/declarative_oob_rules.go's requesterFromPerformer() /
-- requesterFromAuthor() (medicationRequestFields()). Supersedes V170's
-- MedicationRequest coverage (V170's MedicationStatement row is untouched
-- and still current -- MedicationStatement has no requester field).
--
-- Real gap found in production data: a 99397 CCD sample (Epic source) has,
-- on its substanceAdministration's own <author>, a full assignedAuthor --
-- NPI, NUCC specialty code, work address/phone, and a representedOrganization
-- -- none of which the old display-only requester row captured. Per explicit
-- product decision: build the full PractitionerRole when the source has
-- enough structured data (representedOrganization present) to justify it,
-- fall back to display-only when it doesn't.
--
-- See declarative_oob_rules_migration_v172_test.go for the drift guard.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationRequest', 'moodCode=INT', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {"literalValue": "order", "targetPath": "intent", "required": true, "conformance": "SHALL"},
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "fields": [
      {
        "scope": "performers[0].assignedEntity",
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {
        "scope": "performers[0].assignedEntity.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {"sourcePath": "performers[0].assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "performers[0].assignedEntity.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
    ]
  },
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
    "fields": [
      {
        "scope": "authors[0].assignedAuthor",
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {
        "scope": "authors[0].assignedAuthor.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {"sourcePath": "authors[0].assignedAuthor.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
    ]
  },
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL",
    "skipIfResourceHasAnyOf": ["requester"]
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"},
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "quantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dispenseRequest.quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "repeatNumber",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dispenseRequest.numberOfRepeatsAllowed"
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosageInstruction[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    updated_at = NOW();
