-- V160__CDA_Declarative_Mapping_Rules_Coverage_FamilyHistory_Device.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping
-- Engine. Seeds Coverage's two section-key aliases ("payersInsurance",
-- "payors", both wired to mappers.MapCoverage), FamilyMemberHistory
-- ("familyHistory"), and Device ("medicalEquipment", producing
-- DeviceUseStatement resources, not a standalone Device resource).
--
-- See services/cda_fhir/declarative_oob_rules.go's CoverageMappingRules()/
-- FamilyMemberHistoryMappingRules()/DeviceMappingRules() for the single
-- source of truth this JSON is hand-synced to. No new MappingRule/MappingRow
-- fields needed this slice (no FlattenOrganizers/RequiredPaths usage) --
-- same column shape as V156 (Immunizations), so this reuses
-- migrationV155InsertPattern for its drift guard, not a new regex.
--
-- This slice also fixed a confirmed bug in device_mapper.go itself
-- (participant typeCode checked "DEV"/"CSM"; the actual IG fixes "PRD" per
-- CONF:1098-8754, verified 2026-06-22) -- DeviceMappingRules() uses the
-- corrected "PRD" value from the start.

-- =========================================================
-- SEED: payersInsurance -> Coverage
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'payersInsurance', 'Coverage', '', 0,
    $rules$
[
  {"literalValue": "active", "targetPath": "status"},
  {"sourcePath": "code", "transform": "coverage_type_to_codeable_concept", "targetPath": "type"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[typeCode=HLD|PRF].participantRole.scopingEntity.desc",
    "scopeFallbacks": ["entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]"],
    "targetPath": "payor[0].display"
  },
  {"literalValue": "Unknown", "targetPath": "payor[0].display", "skipIfResourceHasAnyOf": ["payor"]},
  {
    "literalValue": {
      "coding": [
        {"system": "http://terminology.hl7.org/CodeSystem/subscriber-relationship", "code": "self", "display": "Self"}
      ]
    },
    "targetPath": "relationship"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry",
    "sourcePath": "id[0].extension",
    "fallbackPaths": ["id[0].root"],
    "targetPath": "subscriberId"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: payors (alias) -> Coverage
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'payors', 'Coverage', '', 0,
    $rules$
[
  {"literalValue": "active", "targetPath": "status"},
  {"sourcePath": "code", "transform": "coverage_type_to_codeable_concept", "targetPath": "type"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[typeCode=HLD|PRF].participantRole.scopingEntity.desc",
    "scopeFallbacks": ["entryRelationships[typeCode=COMP].entry.performers[0].assignedEntity.representedOrganization.names[0]"],
    "targetPath": "payor[0].display"
  },
  {"literalValue": "Unknown", "targetPath": "payor[0].display", "skipIfResourceHasAnyOf": ["payor"]},
  {
    "literalValue": {
      "coding": [
        {"system": "http://terminology.hl7.org/CodeSystem/subscriber-relationship", "code": "self", "display": "Self"}
      ]
    },
    "targetPath": "relationship"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry",
    "sourcePath": "id[0].extension",
    "fallbackPaths": ["id[0].root"],
    "targetPath": "subscriberId"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: familyHistory -> FamilyMemberHistory
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'familyHistory', 'FamilyMemberHistory', '', 0,
    $rules$
[
  {"literalValue": "completed", "targetPath": "status"},
  {"scope": "participants[typeCode=SBJ].participantRole.code", "transform": "cda_code_to_codeable_concept", "targetPath": "relationship"},
  {"scope": "participants[typeCode=SBJ].participantRole.playingEntity.names[0]", "transform": "cda_name_to_family_string", "targetPath": "name"},
  {
    "scope": "components[*]",
    "collectAll": true,
    "targetPath": "condition",
    "fields": [
      {"sourcePath": "value", "transform": "cda_value_to_fhir", "targetPath": "code"},
      {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "onsetDateTime"}
    ]
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: medicalEquipment -> DeviceUseStatement
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medicalEquipment', 'DeviceUseStatement', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "device_use_statement_status_to_fhir", "targetPath": "status"},
  {
    "scope": "participants[typeCode=PRD].participantRole.playingEntity.code",
    "scopeFallbacks": ["code"],
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "device.codeableConcept"
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "timingPeriod"},
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "timingDateTime",
    "skipIfResourceHasAnyOf": ["timingPeriod"]
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
