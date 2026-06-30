-- V165__CDA_Declarative_Mapping_Rules_Header_Constraint_Fix_And_PractitionerRole.sql
--
-- Part 1: fixes a real data-integrity bug found while preparing this
-- migration. cda_declarative_mapping_rules' unique constraint was
-- (document_type, ccda_version, fhir_version, section_key, fhir_resource) --
-- it never included header_path. Author (header_path='authors') and
-- LegalAuthenticator (header_path='legalAuthenticator') both have
-- section_key='' and fhir_resource='Practitioner', so they collided on that
-- constraint: V162's INSERT...ON CONFLICT DO UPDATE for LegalAuthenticator
-- silently overwrote V161's Author row instead of inserting a new one.
-- Confirmed live: header_path='legalAuthenticator' was the only row at
-- section_key=''/fhir_resource='Practitioner', and it carried Author's
-- skip_empty_check=true (a column V162's own UPDATE SET clause never
-- touches, so it survived from the row V162 overwrote instead of resetting
-- to LegalAuthenticator's own default of false).
--
-- This does NOT affect real CDA-to-FHIR conversions today --
-- declarative_document_mapper.go calls AuthorMappingRules()/
-- LegalAuthenticatorMappingRules()/CareTeamMappingRules() as direct Go
-- functions; it never reads this table for these rules. Only this mirror/
-- reference copy was corrupted.
--
-- Part 2: re-seeds Author, LegalAuthenticator, and CareTeam (both
-- section-key aliases) to match declarative_oob_rules.go's current Go
-- literal: Author/LegalAuthenticator gained an organizationLinkRow
-- (PractitionerRole linking to the SAME already-built Practitioner via a
-- literal reference, plus a representedOrganization-derived Organization);
-- CareTeam's existing PractitionerRole row gained
-- "emitAsResourceRequiredPaths": ["organization"] (declarative_schema.go's
-- MappingRow.EmitAsResourceRequiredPaths, added this slice) so it's
-- discarded -- along with the Practitioner/Organization nested inside it --
-- whenever there's no representedOrganization to link, instead of leaving a
-- clutter-only PractitionerRole+duplicate-Practitioner pair in the bundle.
-- Author specifically uses organizationLinkRow rather than CareTeam's
-- assignedEntityRoleRow (which dedup-correlates a SECOND emitted
-- Practitioner by id): real corpus evidence (cerner_sample.xml's,
-- practicefusion_sample.xml's authors) showed that id is frequently absent
-- on this specific element, which left an un-mergeable duplicate
-- Practitioner in the bundle. organizationLinkRow instead references
-- Author's own well-known id ("Practitioner/author-1", hardcoded by
-- declarative_document_mapper.go's Author call site) directly -- no dedup
-- gamble, no duplicate, ever.
--
-- See services/cda_fhir/declarative_oob_rules.go's AuthorMappingRules()/
-- LegalAuthenticatorMappingRules()/CareTeamMappingRules() for the single
-- source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v165_test.go for the drift guard (V161's/
-- V162's/V164's own drift tests were retargeted to this file).

-- =========================================================
-- PART 1: fix the unique constraint to include header_path
-- =========================================================

ALTER TABLE cda_declarative_mapping_rules
    DROP CONSTRAINT cda_declarative_mapping_rules_document_type_ccda_version_fh_key;

ALTER TABLE cda_declarative_mapping_rules
    ADD CONSTRAINT cda_declarative_mapping_rules_doc_ccda_fhir_section_header_key
    UNIQUE (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path);

COMMENT ON CONSTRAINT cda_declarative_mapping_rules_doc_ccda_fhir_section_header_key
    ON cda_declarative_mapping_rules IS
    'Includes header_path (unlike the constraint this replaces) so distinct header-level rules sharing the same section_key=''''/fhir_resource (e.g. Author and LegalAuthenticator, both ''''/Practitioner) no longer collide on ON CONFLICT and silently overwrite each other.';

-- =========================================================
-- PART 2a: restore Author (lost to the V162 collision) + organization link
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, header_path, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, skip_empty_check, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', '', 'authors', 'Practitioner', '', 0,
    $rules$
[
  {"scope": "assignedAuthor.assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
  {"scope": "assignedAuthor.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
  {"scope": "assignedAuthor.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
  {"scope": "assignedAuthor.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"},
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "_emitOnly",
    "fields": [
      {"literalValue": {"reference": "Practitioner/author-1"}, "targetPath": "practitioner"},
      {
        "scope": "assignedAuthor.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
          {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
        ]
      },
      {"sourcePath": "assignedAuthor.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "assignedAuthor.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
    ]
  }
]
    $rules$::jsonb,
    false, ARRAY[]::text[], true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order,
    fields = EXCLUDED.fields, flatten_organizers = EXCLUDED.flatten_organizers,
    required_paths = EXCLUDED.required_paths, skip_empty_check = EXCLUDED.skip_empty_check, updated_at = NOW();

-- =========================================================
-- PART 2b: LegalAuthenticator + organization link (also resets
-- skip_empty_check to its correct default, leaked from the row V162
-- collided with)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, header_path, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, skip_empty_check, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', '', 'legalAuthenticator', 'Practitioner', '', 0,
    $rules$
[
  {"scope": "assignedEntity.assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
  {"scope": "assignedEntity.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier"},
  {"scope": "assignedEntity.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
  {"scope": "assignedEntity.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"},
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "_emitOnly",
    "fields": [
      {"literalValue": {"reference": "Practitioner/legalauthenticator-1"}, "targetPath": "practitioner"},
      {
        "scope": "assignedEntity.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
          {"scope": "addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
        ]
      },
      {"sourcePath": "assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "assignedEntity.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
    ]
  }
]
    $rules$::jsonb,
    false, ARRAY['name'], false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order,
    fields = EXCLUDED.fields, flatten_organizers = EXCLUDED.flatten_organizers,
    required_paths = EXCLUDED.required_paths, skip_empty_check = EXCLUDED.skip_empty_check, updated_at = NOW();

-- =========================================================
-- PART 2c: CareTeam (both section-key aliases) -- add
-- emitAsResourceRequiredPaths to the existing PractitionerRole row
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
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();

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
  }
]
    $rules$::jsonb,
    false, ARRAY['participant'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, required_paths = EXCLUDED.required_paths, updated_at = NOW();
