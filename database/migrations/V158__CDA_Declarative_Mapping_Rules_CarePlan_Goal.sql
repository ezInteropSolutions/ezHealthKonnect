-- V158__CDA_Declarative_Mapping_Rules_CarePlan_Goal.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- Adds the standalone "goals" section and all 4 Plan-of-Care section-key
-- aliases on top of V154-V157's cda_declarative_mapping_rules table (same
-- table, no schema change). See services/cda_fhir/declarative_oob_rules.go's
-- GoalMappingRules()/PlanOfCareMappingRules() for the single source of
-- truth this JSON is hand-synced to, and that file's own top doc comment
-- for what's deliberately out of scope (careplan_mapper.go's MapCarePlans
-- is confirmed dead code, not ported; no rule here sets patientRef-derived
-- fields like subject/participant/deliverFor, consistent with every other
-- Phase 3 rule).
--
-- Each Plan-of-Care alias gets the SAME 6-rule dispatch group (EVN sentinel,
-- Goal, MedicationRequest, Appointment, SupplyRequest, ServiceRequest) --
-- a real document only ever populates ONE of these 4 section keys, never
-- more than one simultaneously, so seeding all 4 is real coverage for
-- different vendors' title/templateId resolution paths, not speculative
-- duplication. rule_order distinguishes the 6 rules within one alias's
-- dispatch group (BuildResourcesForRules' first-match-wins claiming
-- requires the moodCode-based rules (0,1) before the EntryType-based ones
-- (2-5), matching plan_of_care_mapper.go's own moodCode-checked-before-
-- EntryType-switch order).

-- =========================================================
-- SEED: goals -> Goal
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'goals', 'Goal', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "goal_status_to_fhir", "targetPath": "lifecycleStatus"},
  {"sourcePath": "value.code", "fallbackPaths": ["code"], "transform": "cda_code_to_codeable_concept", "targetPath": "description"},
  {"literalValue": {"text": "Goal"}, "targetPath": "description", "skipIfResourceHasAnyOf": ["description"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "target[0].dueDate"}
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: carePlan -> {EVN-sentinel, Goal, MedicationRequest, Appointment,
--                     SupplyRequest, ServiceRequest}
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
('CCD', '2.1', 'R4', 'carePlan', '', 'moodCode=EVN', 0, $rules$[]$rules$::jsonb, true, true, true)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'Goal', 'moodCode=GOL', 1,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "goal_status_to_fhir", "targetPath": "lifecycleStatus"},
  {"sourcePath": "value.code", "fallbackPaths": ["code"], "transform": "cda_code_to_codeable_concept", "targetPath": "description"},
  {"literalValue": {"text": "Goal"}, "targetPath": "description", "skipIfResourceHasAnyOf": ["description"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "target[0].dueDate"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'MedicationRequest', 'entryType=substanceAdministration', 2,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosageInstruction[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'Appointment', 'entryType=encounter', 3,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "appointment_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "serviceType[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_start", "targetPath": "start"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_end", "targetPath": "end"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'SupplyRequest', 'entryType=supply', 4,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "supply_request_status_to_fhir", "targetPath": "status"},
  {"literalValue": {"value": 1}, "targetPath": "quantity"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "itemCodeableConcept"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

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
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry.code", "transform": "cda_code_to_codeable_concept", "targetPath": "reasonCode[0]"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: planOfCare (alias of carePlan) -> same 6-rule dispatch group
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
('CCD', '2.1', 'R4', 'planOfCare', '', 'moodCode=EVN', 0, $rules$[]$rules$::jsonb, true, true, true)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'Goal', 'moodCode=GOL', 1,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "goal_status_to_fhir", "targetPath": "lifecycleStatus"},
  {"sourcePath": "value.code", "fallbackPaths": ["code"], "transform": "cda_code_to_codeable_concept", "targetPath": "description"},
  {"literalValue": {"text": "Goal"}, "targetPath": "description", "skipIfResourceHasAnyOf": ["description"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "target[0].dueDate"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'MedicationRequest', 'entryType=substanceAdministration', 2,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosageInstruction[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'Appointment', 'entryType=encounter', 3,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "appointment_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "serviceType[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_start", "targetPath": "start"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_end", "targetPath": "end"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfCare', 'SupplyRequest', 'entryType=supply', 4,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "supply_request_status_to_fhir", "targetPath": "status"},
  {"literalValue": {"value": 1}, "targetPath": "quantity"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "itemCodeableConcept"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
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
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry.code", "transform": "cda_code_to_codeable_concept", "targetPath": "reasonCode[0]"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: assessmentAndPlan (alias of carePlan) -> same 6-rule dispatch group
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
('CCD', '2.1', 'R4', 'assessmentAndPlan', '', 'moodCode=EVN', 0, $rules$[]$rules$::jsonb, true, true, true)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'Goal', 'moodCode=GOL', 1,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "goal_status_to_fhir", "targetPath": "lifecycleStatus"},
  {"sourcePath": "value.code", "fallbackPaths": ["code"], "transform": "cda_code_to_codeable_concept", "targetPath": "description"},
  {"literalValue": {"text": "Goal"}, "targetPath": "description", "skipIfResourceHasAnyOf": ["description"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "target[0].dueDate"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'MedicationRequest', 'entryType=substanceAdministration', 2,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosageInstruction[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'Appointment', 'entryType=encounter', 3,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "appointment_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "serviceType[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_start", "targetPath": "start"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_end", "targetPath": "end"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'assessmentAndPlan', 'SupplyRequest', 'entryType=supply', 4,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "supply_request_status_to_fhir", "targetPath": "status"},
  {"literalValue": {"value": 1}, "targetPath": "quantity"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "itemCodeableConcept"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
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
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry.code", "transform": "cda_code_to_codeable_concept", "targetPath": "reasonCode[0]"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: planOfTreatment (alias of carePlan) -> same 6-rule dispatch group
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
('CCD', '2.1', 'R4', 'planOfTreatment', '', 'moodCode=EVN', 0, $rules$[]$rules$::jsonb, true, true, true)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'Goal', 'moodCode=GOL', 1,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "goal_status_to_fhir", "targetPath": "lifecycleStatus"},
  {"sourcePath": "value.code", "fallbackPaths": ["code"], "transform": "cda_code_to_codeable_concept", "targetPath": "description"},
  {"literalValue": {"text": "Goal"}, "targetPath": "description", "skipIfResourceHasAnyOf": ["description"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "target[0].dueDate"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'MedicationRequest', 'entryType=substanceAdministration', 2,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"},
  {"sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code", "transform": "cda_code_to_codeable_concept", "targetPath": "medicationCodeableConcept"},
  {"sourcePath": "routeCode", "transform": "cda_code_to_codeable_concept", "targetPath": "dosageInstruction[0].route"},
  {"sourcePath": "doseQuantity", "transform": "cda_quantity_to_fhir", "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"},
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'Appointment', 'entryType=encounter', 3,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "appointment_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "serviceType[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_start", "targetPath": "start"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period_end", "targetPath": "end"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'planOfTreatment', 'SupplyRequest', 'entryType=supply', 4,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "supply_request_status_to_fhir", "targetPath": "status"},
  {"literalValue": {"value": 1}, "targetPath": "quantity"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "itemCodeableConcept"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "authoredOn"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
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
  {"literalValue": {"text": "Unknown"}, "targetPath": "code", "skipIfResourceHasAnyOf": ["code"]},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "occurrencePeriod"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_onset", "targetPath": "occurrenceDateTime", "skipIfResourceHasAnyOf": ["occurrencePeriod"]},
  {"scope": "entryRelationships[typeCode=RSON].entry.code", "transform": "cda_code_to_codeable_concept", "targetPath": "reasonCode[0]"}
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
