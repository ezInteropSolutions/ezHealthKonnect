-- V157__CDA_Declarative_Mapping_Rules_Encounters_Procedures.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- Adds Encounters and Procedures on top of V154/V155/V156's
-- cda_declarative_mapping_rules table (same table, no schema change).
-- See services/cda_fhir/declarative_oob_rules.go's EncounterMappingRules()/
-- ProcedureMappingRules() for the single source of truth this JSON is
-- hand-synced to, and those functions' own top doc comments for what's
-- deliberately out of scope (document-header-level fields for Encounters;
-- the Procedure-Activity-template-uniform treatment matching Go's own).

-- =========================================================
-- SEED: encounters -> Encounter
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'encounters', 'Encounter', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "encounter_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "encounter_class_coding", "targetPath": "class"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "type[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[*]",
    "collectAll": true,
    "targetPath": "participant",
    "fields": [
      {"sourcePath": "typeCode", "transform": "encounter_participant_type_coding", "targetPath": "type[0]"},
      {"scope": "participantRole.playingEntity.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "individual"}
    ]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
    "sourcePath": "participantRole.playingEntity.names[0].family",
    "fallbackPaths": ["participantRole.playingEntity.code.displayName"],
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "location[0].location"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: procedures -> Procedure
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'procedures', 'Procedure', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "procedure_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "performedPeriod"},
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "performedDateTime",
    "skipIfResourceHasAnyOf": ["performedPeriod"]
  },
  {
    "scope": "participants[typeCode=PRF|SPRF]",
    "collectAll": true,
    "targetPath": "performer",
    "fields": [
      {"scope": "participantRole.playingEntity.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "actor"}
    ]
  },
  {"sourcePath": "targetSiteCode", "transform": "cda_code_to_codeable_concept", "targetPath": "bodySite[0]"}
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
