-- V189: Add CareTeam.managingOrganization from organizer author's representedOrganization
-- Applied: 2026-07-01
--
-- Supersedes V165 (CDA_Declarative_Mapping_Rules_Header_Constraint_Fix_And_PractitionerRole)
-- for the careTeam / careTeams rows only. V165's Author/LegalAuthenticator rows are unchanged.
--
-- C-CDA on FHIR IG alignment: the Care Team Organizer's own
-- <author><assignedAuthor><representedOrganization> names the organization
-- responsible for the care team. This maps to CareTeam.managingOrganization.
-- Real corpus evidence: Epic 99397 organizer has representedOrganization
-- "Boulder Community Health and Affiliates" (no id on the assignedAuthor).
--
-- Drift guard: services/cda_fhir/declarative_oob_rules_migration_v189_test.go

-- =========================================================
-- SEED: careTeam -> CareTeam (+ managingOrganization from organizer author)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'careTeam', 'CareTeam', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "care_team_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "category[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[*]",
    "collectAll": true,
    "targetPath": "participant",
    "fields": [
      {
        "emitAsResource": "Practitioner",
        "targetPath": "member",
        "fields": [
          {"scope": "participantRole.playingEntity.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
          {"scope": "participantRole.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
          {"sourcePath": "participantRole.code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "participantRole.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
          {"scope": "participantRole.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
        ]
      },
      {"sourcePath": "functionCode", "transform": "cda_code_to_codeable_concept", "targetPath": "role[0]"}
    ]
  },
  {
    "scope": "components[*]",
    "collectAll": true,
    "targetPath": "_emitOnly",
    "fields": [
      {
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": ["organization"],
        "targetPath": "ref",
        "fields": [
          {
            "scope": "performers[0].assignedEntity",
            "emitAsResource": "Practitioner",
            "targetPath": "practitioner",
            "fields": [
              {"scope": "assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
              {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
              {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
              {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
              {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
            ]
          },
          {
            "scope": "performers[0].assignedEntity.representedOrganization",
            "emitAsResource": "Organization",
            "targetPath": "organization",
            "fields": [
              {"sourcePath": "names[0]", "targetPath": "name"},
              {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
              {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
              {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
            ]
          },
          {"sourcePath": "performers[0].assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
          {"scope": "performers[0].assignedEntity.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
        ]
      }
    ]
  },
  {
    "scope": "authors[0].assignedAuthor.representedOrganization",
    "emitAsResource": "Organization",
    "targetPath": "managingOrganization",
    "fields": [
      {"sourcePath": "names[0]", "targetPath": "name"},
      {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
      {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
      {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
    ]
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();

-- =========================================================
-- SEED: careTeams (alias) -> CareTeam (+ managingOrganization from organizer author)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'careTeams', 'CareTeam', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "care_team_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "category[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[*]",
    "collectAll": true,
    "targetPath": "participant",
    "fields": [
      {
        "emitAsResource": "Practitioner",
        "targetPath": "member",
        "fields": [
          {"scope": "participantRole.playingEntity.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
          {"scope": "participantRole.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
          {"sourcePath": "participantRole.code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "participantRole.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
          {"scope": "participantRole.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
        ]
      },
      {"sourcePath": "functionCode", "transform": "cda_code_to_codeable_concept", "targetPath": "role[0]"}
    ]
  },
  {
    "scope": "components[*]",
    "collectAll": true,
    "targetPath": "_emitOnly",
    "fields": [
      {
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": ["organization"],
        "targetPath": "ref",
        "fields": [
          {
            "scope": "performers[0].assignedEntity",
            "emitAsResource": "Practitioner",
            "targetPath": "practitioner",
            "fields": [
              {"scope": "assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
              {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
              {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
              {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
              {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
            ]
          },
          {
            "scope": "performers[0].assignedEntity.representedOrganization",
            "emitAsResource": "Organization",
            "targetPath": "organization",
            "fields": [
              {"sourcePath": "names[0]", "targetPath": "name"},
              {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
              {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
              {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
            ]
          },
          {"sourcePath": "performers[0].assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
          {"scope": "performers[0].assignedEntity.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
        ]
      }
    ]
  },
  {
    "scope": "authors[0].assignedAuthor.representedOrganization",
    "emitAsResource": "Organization",
    "targetPath": "managingOrganization",
    "fields": [
      {"sourcePath": "names[0]", "targetPath": "name"},
      {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
      {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
      {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
    ]
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();
