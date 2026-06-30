-- V174__CDA_Declarative_Mapping_Rules_Medication_Requester_Note.sql
--
-- Two IG-driven fixes to medicationRequestFields()/medicationCommonRows(),
-- found by checking HL7's C-CDA on FHIR IG (CF-medications.html) directly
-- after a user asked which CDA source is authoritative for requester and
-- whether a CDA source for .note exists at all:
--
-- 1. MedicationRequest.requester priority reversed from "performer, else
--    author" (inherited from medication_mapper.go) to "author, else
--    performer" -- the IG specifies "/author -> .requester (Provenance)"
--    as THE source; /performer is not listed as a requester source at all.
--
-- 2. MedicationRequest.note / MedicationStatement.note added -- the IG
--    specifies Comment Activity (entryRelationship/act[code/@code=
--    '48767-8']/text -> .note as an Annotation), a field this codebase had
--    no row for. typeCode="COMP" confirmed against this exact 99397 CCD
--    sample's own real (non-medication) Comment Activity usage.
--
-- Supersedes V170 (medications, both resources), V172 (medications/
-- MedicationRequest), V171 and V173 (the 4 Plan-of-Care aliases'
-- MedicationRequest, which reuse medicationRequestFields()/
-- medicationCommonRows() verbatim via PlanOfCareMappingRules()'s
-- substanceAdministration branch) entirely for Fields content.
--
-- See declarative_oob_rules_migration_v174_test.go for the drift guard.

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
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
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
    "sourcePath": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "fallbackPaths": ["performers[0].assignedEntity.assignedPerson.names[0]"],
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
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationStatement', '', 1,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "effectivePeriod"},
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "effectiveDateTime",
    "skipIfResourceHasAnyOf": ["effectivePeriod"]
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosage[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosage[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosage[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dosage[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosage[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosage[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosage[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'MedicationRequest', 'entryType=substanceAdministration', 2,
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
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
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
    "sourcePath": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "fallbackPaths": ["performers[0].assignedEntity.assignedPerson.names[0]"],
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
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'MedicationRequest', 'entryType=substanceAdministration', 2,
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
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
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
    "sourcePath": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "fallbackPaths": ["performers[0].assignedEntity.assignedPerson.names[0]"],
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
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'MedicationRequest', 'entryType=substanceAdministration', 2,
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
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
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
    "sourcePath": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "fallbackPaths": ["performers[0].assignedEntity.assignedPerson.names[0]"],
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
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'MedicationRequest', 'entryType=substanceAdministration', 2,
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
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "requester",
    "skipIfResourceHasAnyOf": ["requester"],
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
    "sourcePath": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "fallbackPaths": ["performers[0].assignedEntity.assignedPerson.names[0]"],
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
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
