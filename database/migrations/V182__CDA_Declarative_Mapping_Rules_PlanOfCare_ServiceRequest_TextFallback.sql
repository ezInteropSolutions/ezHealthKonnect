-- V182__CDA_Declarative_Mapping_Rules_PlanOfCare_ServiceRequest_TextFallback.sql
--
-- Adds an entry.text -> code.text fallback row to PlanOfCareMappingRules()'s
-- ServiceRequest dispatch branch (entryType=procedure|observation|act),
-- across all 4 section-key aliases (carePlan/planOfCare/assessmentAndPlan/
-- planOfTreatment).
--
-- Real gap found auditing 2 independently-sourced real CCDs shared this
-- session (Marshfield Clinic, Dignity Health) -- distinct from the 99397
-- Epic sample and a third new sample (Ascension), both already covered by
-- this rule's existing reasonCode/requester rows. A Planned Act (.4.39)
-- entry's own <code> is a bare nullFlavor="UNK" with NO <originalText>
-- child at all (unlike the Health Maintenance Observation .4.44 shape
-- elsewhere in this same section, where the reference lives INSIDE code's
-- own originalText and so already resolves via parseCD/resolveCodeRef
-- before this engine ever sees it). The resolved content instead lives in
-- the entry's own sibling <text><reference value="#id"/></text> -- e.g.
-- "Return to Clinic - Endocrinology 6/17/26" or a full office-visit note --
-- which the existing code row never read, so every one of these fell
-- through to the generic "Unknown" placeholder, discarding real, often
-- substantial narrative content. Mirrors the entry.text-to-code.text
-- override row already proven in observationRule()/ClinicalNoteMappingRules
-- (declarative_oob_rules.go).
--
-- See PlanOfCareMappingRules' own ServiceRequest row doc comment in
-- declarative_oob_rules.go for the single source of truth this SQL is
-- hand-synced to; declarative_oob_rules_migration_v182_test.go is the drift
-- guard.
--
-- Supersedes V158 for these 4 ServiceRequest rows' Fields content ONLY --
-- V158 still owns entry_match/flatten_organizers for them (unchanged by
-- this migration), and remains the owner of every other Plan-of-Care rule
-- (the "" no-op EVN rule, Goal, MedicationRequest -- already superseded by
-- V171 for its own reasons --, Appointment, SupplyRequest) across all 4
-- aliases.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'ServiceRequest', 'entryType=procedure|observation|act', 5,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "service_request_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "moodCode", "transform": "service_request_intent_from_mood", "targetPath": "intent"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry", "transform": "cda_value_or_code_to_codeable_concept", "targetPath": "reasonCode[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "requester"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'ServiceRequest', 'entryType=procedure|observation|act', 5,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "service_request_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "moodCode", "transform": "service_request_intent_from_mood", "targetPath": "intent"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry", "transform": "cda_value_or_code_to_codeable_concept", "targetPath": "reasonCode[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "requester"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'ServiceRequest', 'entryType=procedure|observation|act', 5,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "service_request_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "moodCode", "transform": "service_request_intent_from_mood", "targetPath": "intent"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry", "transform": "cda_value_or_code_to_codeable_concept", "targetPath": "reasonCode[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "requester"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'ServiceRequest', 'entryType=procedure|observation|act', 5,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "service_request_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "moodCode", "transform": "service_request_intent_from_mood", "targetPath": "intent"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "text", "targetPath": "code.text"},
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry", "transform": "cda_value_or_code_to_codeable_concept", "targetPath": "reasonCode[0]"},
  {"scope": "authors[0].assignedAuthor.assignedPerson.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "requester"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
