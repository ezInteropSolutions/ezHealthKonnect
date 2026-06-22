-- V159__CDA_Declarative_Mapping_Rules_CareTeam.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping
-- Engine. Adds a "required_paths" column (MappingRule.RequiredPaths, see
-- declarative_schema.go) and seeds both of document_mapper.go's Care Team
-- section-key aliases ("careTeam", "careTeams" — both wired to
-- mappers.MapCareTeam, never more than one populated per real document).
--
-- See services/cda_fhir/declarative_oob_rules.go's CareTeamMappingRules()
-- for the single source of truth this JSON is hand-synced to. This is the
-- first OOB rule to use EmitAsResource (MappingRow.EmitAsResource) — each
-- participants[*] match builds both a CareTeam.participant entry AND an
-- independent, cross-referenced Practitioner resource.

ALTER TABLE cda_declarative_mapping_rules
    ADD COLUMN IF NOT EXISTS required_paths TEXT[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN cda_declarative_mapping_rules.required_paths IS
    'MappingRule.RequiredPaths (declarative_schema.go) -- discard the entire built resource unless every one of these top-level keys ended up present. See CareTeamMappingRules() in declarative_oob_rules.go.';

-- =========================================================
-- SEED: careTeam -> CareTeam (+ Practitioner per participant)
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
          {"scope": "participantRole.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier"},
          {"sourcePath": "participantRole.code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "participantRole.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
        ]
      },
      {"sourcePath": "functionCode", "transform": "cda_code_to_codeable_concept", "targetPath": "role[0]"}
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
-- SEED: careTeams (alias) -> CareTeam (+ Practitioner per participant)
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
          {"scope": "participantRole.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier"},
          {"sourcePath": "participantRole.code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "participantRole.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
        ]
      },
      {"sourcePath": "functionCode", "transform": "cda_code_to_codeable_concept", "targetPath": "role[0]"}
    ]
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();
