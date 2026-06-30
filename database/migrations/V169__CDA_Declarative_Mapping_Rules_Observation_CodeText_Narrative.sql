-- V169__CDA_Declarative_Mapping_Rules_Observation_CodeText_Narrative.sql
--
-- Re-seeds all 6 observationRule()-based sections (vitalSigns, results,
-- socialHistory from V155; labResults, functionalStatus, mentalStatus from
-- V163) to add a code.text-from-narrative row -- observationRule() is one
-- shared Go helper feeding all 6, so all 6 drift simultaneously from this
-- one Go-side change.
--
-- Real gap found in production data (a 99397 CCD, MAGNUSSON sample, Epic
-- source): the Functional Status section repeats one generic LOINC code
-- (54522-8 "Functional status") across multiple unrelated entries -- a
-- Height value of 64, a Weight value of 2668.8, an Exercise-Days value of
-- "6 days", a Patient-Position value of "Sitting" all carried the SAME
-- code.text ("Functional status"), making it impossible to tell the values
-- apart from the Observation alone. The actual distinguishing label
-- ("Height", "Weight", etc.) lived only in the section's narrative <text>
-- block, linked back via each entry's own <text><reference value="#..."/>
-- (entry.Text, resolved by cda/document/section_parser.go's
-- resolveEntryRefs/walkForIDs against the narrative index) -- walkForIDs
-- itself had a companion bug fixed in the same change: it indexed an ID'd
-- narrative element (e.g. <tr ID="functionalStatus.11">) by ITS OWN inner
-- text (value+date+author, not the label), instead of preferring the
-- ancestor <item>'s <caption> ("Height") when one exists -- see that
-- function's own doc comment in section_parser.go.
--
-- This row runs immediately after the existing code row and overwrites
-- code.text (never coding[].display, which stays LOINC-correct) whenever a
-- narrative label resolved; declarative_engine.go's applyRow skips writing
-- anything when SourcePath resolves to nothing, so entries with no <text>
-- element (most Vitals/Results entries) are completely unaffected.
--
-- See services/cda_fhir/declarative_oob_rules.go's observationRule() for
-- the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v169_test.go for the drift guard (V155's
-- own vitalSigns/results/socialHistory coverage and V163's own
-- labResults/functionalStatus/mentalStatus coverage are both retargeted to
-- this file).

-- =========================================================
-- vitalSigns -> Observation (category=vital-signs)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'vitalSigns', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "vital-signs", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- results -> Observation (category=laboratory)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'results', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- socialHistory -> Observation (category=social-history)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'socialHistory', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"sourcePath": "statusCode", "transform": "observation_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "effectiveDateTime"},
  {"literalValue": "social-history", "transform": "observation_category_to_fhir", "targetPath": "category[0]"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- labResults -> Observation (category=laboratory, alias of "results")
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'labResults', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- functionalStatus -> Observation (category=functional-status)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'functionalStatus', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- mentalStatus -> Observation (category=cognitive-status)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'mentalStatus', 'Observation', '', 0,
    $rules$
[
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
