-- V163__CDA_Declarative_Mapping_Rules_FunctionalMentalStatus_LabResults.sql
-- Phase 4 Slice D (Cutover) of the CDA→FHIR Declarative Mapping Engine.
--
-- Closes 2 real section-coverage gaps found while preparing to delete the
-- hardcoded Go mapper path: "functionalStatus" and "mentalStatus" had no
-- declarative rule at all (declarative_transform_registry.go's own doc
-- comment flagged this explicitly), and "labResults" -- the alias
-- document_mapper.go's typedSectionDispatchers mapped to the same mapper/
-- category as "results" -- had no declarative rule either. Without these,
-- cutting the executor over to the declarative engine would silently drop
-- any real document carrying one of these 3 sections. See
-- services/cda_fhir/declarative_oob_rules.go's FunctionalStatusMappingRules()/
-- MentalStatusMappingRules()/ResultsMappingRules() for the single source of
-- truth this JSON is hand-synced to.
--
-- Net-new section coverage, not an edit to an already-seeded row -- follows
-- the original Phase 3 pattern of one migration per new section (unlike
-- Slice B's V155/V157/V161 in-place edits to rows that already existed).
-- No new columns: flatten_organizers/skip_if_code_null_flavor already exist
-- from V155.

-- =========================================================
-- SEED: labResults -> Observation (category=laboratory, alias of "results")
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'labResults', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "laboratory", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "scopeFallbacks": ["authors[0].assignedAuthor.representedOrganization.names[0]"], "transform": "cda_name_or_literal_to_display_ref", "targetPath": "performer[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {"sourcePath": "interpretationCode", "transform": "cda_code_to_codeable_concept", "targetPath": "interpretation[0]"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- SEED: functionalStatus -> Observation (category=functional-status)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'functionalStatus', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "functional-status", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "scopeFallbacks": ["authors[0].assignedAuthor.representedOrganization.names[0]"], "transform": "cda_name_or_literal_to_display_ref", "targetPath": "performer[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {"sourcePath": "interpretationCode", "transform": "cda_code_to_codeable_concept", "targetPath": "interpretation[0]"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- SEED: mentalStatus -> Observation (category=cognitive-status)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'mentalStatus', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "cognitive-status", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "scopeFallbacks": ["authors[0].assignedAuthor.representedOrganization.names[0]"], "transform": "cda_name_or_literal_to_display_ref", "targetPath": "performer[0]"},
  {"scope": "value[type=PQ]", "sourcePath": "quantity", "transform": "cda_quantity_to_fhir", "targetPath": "valueQuantity"},
  {"scope": "value[type=CD]", "scopeFallbacks": ["value[type=CE]", "value[type=CS]"], "sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "valueCodeableConcept"},
  {"scope": "value[type=ST]", "scopeFallbacks": ["value[type=ED]"], "sourcePath": "text", "targetPath": "valueString"},
  {"scope": "value[type=BL]", "sourcePath": "boolean", "targetPath": "valueBoolean"},
  {"scope": "value[type=INT]", "sourcePath": "integer", "targetPath": "valueInteger"},
  {"scope": "value[type=REAL]", "sourcePath": "real", "transform": "cda_real_to_bare_quantity", "targetPath": "valueQuantity"},
  {"scope": "value[type=IVL_TS]", "sourcePath": "timeRange", "transform": "cda_timerange_to_period", "targetPath": "valuePeriod"},
  {"sourcePath": "interpretationCode", "transform": "cda_code_to_codeable_concept", "targetPath": "interpretation[0]"},
  {
    "literalValue": "unknown",
    "transform": "observation_data_absent_reason_to_fhir",
    "targetPath": "dataAbsentReason",
    "skipIfResourceHasAnyOf": ["valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod"]
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
