-- V170__CDA_Declarative_Mapping_Rules_Medication_Identifier.sql
--
-- Re-seeds Medications (MedicationRequest, MedicationStatement -- both
-- previously seeded by V154) to add an identifier row to
-- medicationCommonRows() -- the substanceAdministration's own <id>, parsed
-- generically for every CDA entry (entry_parser.go) but never read by any
-- row in this section. Real gap found in production data: a 99397 CCD
-- sample (Epic source) has a distinct <id root="..." extension="..."/> on
-- every one of its 10 medication entries, and neither MedicationRequest nor
-- MedicationStatement ever carried an identifier in the output.
--
-- No embedCDAIdentity -- unlike Encounter's analogous "id[*]" row, nothing
-- in this engine needs to cross-match a Medication resource against
-- another source by shared id.
--
-- See services/cda_fhir/declarative_oob_rules.go's medicationCommonRows()
-- for the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v170_test.go for the drift guard (V154's
-- own medications coverage is narrowed/removed in
-- declarative_oob_rules_migration_test.go).

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationRequest', 'moodCode=INT', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "medication_request_status_to_fhir",
    "targetPath": "status",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "literalValue": "order",
    "targetPath": "intent",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "authoredOn"
  },
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
  {
    "scope": "id[*]",
    "collectAll": true,
    "transform": "cda_ii_to_identifier",
    "targetPath": "identifier"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "medicationCodeableConcept"
  },
  {
    "sourcePath": "routeCode",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "dosageInstruction[0].route"
  },
  {
    "sourcePath": "doseQuantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"
  },
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
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationStatement', '', 1,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "medication_status_to_fhir",
    "targetPath": "status",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_period",
    "targetPath": "effectivePeriod"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "effectiveDateTime",
    "skipIfResourceHasAnyOf": ["effectivePeriod"]
  },
  {
    "scope": "id[*]",
    "collectAll": true,
    "transform": "cda_ii_to_identifier",
    "targetPath": "identifier"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "medicationCodeableConcept"
  },
  {
    "sourcePath": "routeCode",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "dosage[0].route"
  },
  {
    "sourcePath": "doseQuantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dosage[0].doseAndRate[0].doseQuantity"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosage[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "transform": "cda_decimal_string_to_number",
    "targetPath": "dosage[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosage[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosage[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosage[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();
