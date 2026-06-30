-- V171__CDA_Declarative_Mapping_Rules_PlanOfCare_Medication_Identifier.sql
--
-- Re-seeds the MedicationRequest row in all 4 Plan-of-Care section aliases
-- (carePlan, planOfCare, assessmentAndPlan, planOfTreatment -- all
-- previously seeded by V158) to add the SAME identifier row V170 added to
-- the Medications section's MedicationRequest/MedicationStatement --
-- PlanOfCareMappingRules()'s substanceAdministration branch reuses
-- medicationRequestFields()/medicationCommonRows() verbatim (see that
-- function's own doc comment), so all 4 of these rows drift simultaneously
-- from the same Go-side change V170 made.
--
-- See services/cda_fhir/declarative_oob_rules.go's medicationCommonRows()
-- for the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v171_test.go for the drift guard (V158's
-- own MedicationRequest coverage for these 4 sections is narrowed in
-- declarative_oob_rules_migration_v158_test.go).

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'carePlan', 'MedicationRequest', 'entryType=substanceAdministration', 2,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "medication_request_status_to_fhir", "targetPath": "status", "required": true, "conformance": "SHALL"},
  {"literalValue": "order", "targetPath": "intent", "required": true, "conformance": "SHALL"},
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
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "quantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dispenseRequest.quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "repeatNumber",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dispenseRequest.numberOfRepeatsAllowed"
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
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
    "transform": "cda_decimal_string_to_number",
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
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
  {"literalValue": "order", "targetPath": "intent", "required": true, "conformance": "SHALL"},
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
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "quantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dispenseRequest.quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "repeatNumber",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dispenseRequest.numberOfRepeatsAllowed"
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
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
    "transform": "cda_decimal_string_to_number",
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
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
  {"literalValue": "order", "targetPath": "intent", "required": true, "conformance": "SHALL"},
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
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "quantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dispenseRequest.quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "repeatNumber",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dispenseRequest.numberOfRepeatsAllowed"
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
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
    "transform": "cda_decimal_string_to_number",
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
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
  {"literalValue": "order", "targetPath": "intent", "required": true, "conformance": "SHALL"},
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
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "quantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dispenseRequest.quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry[entryType=supply]",
    "sourcePath": "repeatNumber",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dispenseRequest.numberOfRepeatsAllowed"
  },
  {"scope": "id[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier"},
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
    "transform": "cda_decimal_string_to_number",
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
