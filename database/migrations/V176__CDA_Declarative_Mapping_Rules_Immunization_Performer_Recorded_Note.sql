-- V176__CDA_Declarative_Mapping_Rules_Immunization_Performer_Recorded_Note.sql
--
-- Three fixes to ImmunizationMappingRules(), found by auditing the
-- Immunizations section of a 99397 CCD sample against HL7's C-CDA on FHIR
-- IG (CF-immunizations.md):
--
-- 1. Immunization.performer -- was a display-only string sourced only from
--    an assignedPerson name, so 11 of the 99397 sample's 13 Immunization
--    Activity entries (organization-only performer, no person name) lost
--    their performer info entirely. Upgraded to a tiered
--    assignedEntityRoleRow (rich PractitionerRole, degrading gracefully to
--    an organization-only PractitionerRole per buildEmittedSubResource's
--    len<=1 gate when no person name exists) with a barePractitionerRow
--    fallback tier (person exists, no organization -- not hit by the real
--    99397 data, but needed by a pre-existing test fixture). Also adds
--    performer[0].function="AP" (Administering Provider), the IG-fixed
--    constant for every C-CDA Immunization Activity /performer.
--
-- 2. Immunization.recorded -- IG specifies "/author/time -> .recorded" but
--    this was never mapped at all, despite every one of the 99397 sample's
--    13 entries carrying a real <author><time .../></author>.
--
-- 3. Immunization.note -- IG-specified source (Comment Activity, templateId
--    2.16.840.1.113883.10.20.22.4.64, code 48767-8, attached via
--    entryRelationship typeCode=COMP). Added for IG correctness; UNLIKE
--    Condition.note/Medication.note, this row has NO real-data evidence in
--    the 99397 sample (zero entryRelationships on any Immunization Activity
--    entry there) -- kept on the strength of the IG citation alone.
--
-- Supersedes V156 entirely for Fields content -- see
-- declarative_oob_rules_migration_v176_test.go.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'immunizations', 'Immunization', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "targetPath": "status",
    "transform": "immunization_status_to_fhir",
    "condition": {
      "whenPath": "negationInd",
      "equals": "true",
      "thenLiteralValue": "not-done",
      "thenTransform": "string_direct"
    }
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "targetPath": "statusReason",
    "transform": "cda_value_or_code_to_codeable_concept"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "targetPath": "vaccineCode",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "occurrenceDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "sourcePath": "routeCode",
    "targetPath": "route",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "doseQuantity",
    "targetPath": "doseQuantity",
    "transform": "cda_quantity_to_fhir"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.lotNumberText",
    "targetPath": "lotNumber"
  },
  {
    "fields": [
      {
        "scope": "performers[0].assignedEntity",
        "fields": [
          {
            "scope": "assignedPerson.names[*]",
            "collectAll": true,
            "targetPath": "name",
            "transform": "cda_name_to_fhir"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "sourcePath": "code",
            "targetPath": "qualification[0].code",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "performers[0].assignedEntity.representedOrganization",
        "fields": [
          {
            "sourcePath": "names[0]",
            "targetPath": "name"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {
        "sourcePath": "performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "performer[0].actor"
  },
  {
    "scope": "performers[0].assignedEntity",
    "fields": [
      {
        "scope": "assignedPerson.names[*]",
        "collectAll": true,
        "targetPath": "name",
        "transform": "cda_name_to_fhir"
      },
      {
        "scope": "ids[*]",
        "collectAll": true,
        "targetPath": "identifier",
        "transform": "cda_ii_to_identifier",
        "embedCDAIdentity": true
      },
      {
        "sourcePath": "code",
        "targetPath": "qualification[0].code",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      },
      {
        "scope": "addresses[*]",
        "collectAll": true,
        "targetPath": "address",
        "transform": "cda_address_to_fhir"
      }
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "performer[0].actor",
    "skipIfResourceHasAnyOf": [
      "performer"
    ]
  },
  {
    "scope": "performers[0].assignedEntity",
    "literalValue": "AP",
    "targetPath": "performer[0].function",
    "transform": "immunization_performer_function_to_fhir"
  },
  {
    "sourcePath": "authors[0].time",
    "targetPath": "recorded",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  },
  {
    "scope": "id[*]",
    "collectAll": true,
    "targetPath": "identifier",
    "transform": "cda_ii_to_identifier"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturerOrganization.names[0]",
    "targetPath": "manufacturer",
    "transform": "cda_name_or_literal_to_display_ref"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
