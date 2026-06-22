-- V155__CDA_Declarative_Mapping_Rules_Observations.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- Adds the Vital Signs / Results (Laboratory) / Social History slice on top
-- of V154's cda_declarative_mapping_rules table (NOT a new table -- same
-- shape, same drift-guard discipline). See
-- services/cda_fhir/declarative_oob_rules.go's observationRule() for the
-- single source of truth this JSON is hand-synced to, and that file's own
-- top doc comment for what this slice deliberately does NOT port (BP-panel
-- combination, shell-entry substitution, interpretationCode) and why.
--
-- New column: flatten_organizers. MappingRule-level fields get dedicated
-- columns (see entry_match in V154), not JSONB, so MappingRule.FlattenOrganizers
-- (declarative_schema.go) needs one too -- defaults to false, so V154's
-- existing 5 rows are unaffected.

ALTER TABLE cda_declarative_mapping_rules
    ADD COLUMN IF NOT EXISTS flatten_organizers BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN cda_declarative_mapping_rules.flatten_organizers IS
    'MappingRule.FlattenOrganizers (declarative_schema.go) -- expand organizer entries into their Components instead of treating the organizer as one resource. See observationRule() in declarative_oob_rules.go.';

-- =========================================================
-- SEED: vitalSigns -> Observation (category=vital-signs)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'vitalSigns', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "vital-signs", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: results -> Observation (category=laboratory)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'results', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "laboratory", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: socialHistory -> Observation (category=social-history)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'socialHistory', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "social-history", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
