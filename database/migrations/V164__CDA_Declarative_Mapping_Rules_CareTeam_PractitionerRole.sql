-- V164__CDA_Declarative_Mapping_Rules_CareTeam_PractitionerRole.sql
-- Phase 4 follow-up to V159 (CDA_Declarative_Mapping_Rules_CareTeam.sql).
--
-- V159's "components[*]" wrapper emitted a bare Practitioner from the Care
-- Team performer's <assignedEntity>, silently dropping its
-- <representedOrganization> (real, rich data in the EPIC-sourced corpus:
-- name/address/telecom for the performer's organization) — the same
-- "Deliberately NOT ported" gap this file's other representedOrganization
-- occurrences already document. FHIR's building block for "this person, at
-- this organization, in this specialty" is PractitionerRole, not a new field
-- on Practitioner itself (which stays organization-agnostic by design).
--
-- This migration re-seeds (ON CONFLICT...DO UPDATE, same natural key V159
-- used) both Care Team section-key aliases' "components[*]" wrapper to emit
-- PractitionerRole -> {Practitioner via "practitioner", Organization via
-- "organization"} instead of a bare Practitioner. V159's own file is left
-- untouched (already applied; Flyway checksums an applied migration's file
-- content, so patching it in place would break `flyway validate` on every
-- environment that already ran it) — this is the correct, Flyway-native way
-- to evolve previously-seeded data.
--
-- See services/cda_fhir/declarative_oob_rules.go's CareTeamMappingRules()
-- (via the new, reusable assignedEntityRoleRow() helper) for the single
-- source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v164_test.go for the drift guard (V159's
-- own drift test was retargeted to this file, since V159's seed is no
-- longer the authoritative source for Care Team's PractitionerRole shape).

-- =========================================================
-- SEED: careTeam -> CareTeam (+ PractitionerRole/Practitioner/Organization per performer)
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
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();

-- =========================================================
-- SEED: careTeams (alias) -> CareTeam (+ PractitionerRole/Practitioner/Organization per performer)
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
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();
