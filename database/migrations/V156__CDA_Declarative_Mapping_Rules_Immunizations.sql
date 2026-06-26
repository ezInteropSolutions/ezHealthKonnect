-- V156__CDA_Declarative_Mapping_Rules_Immunizations.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- Adds the Immunizations slice on top of V154/V155's
-- cda_declarative_mapping_rules table (same table, no new columns needed --
-- this rule doesn't need flatten_organizers, included explicitly as false
-- for clarity rather than relying on the column default). See
-- services/cda_fhir/declarative_oob_rules.go's ImmunizationMappingRules()
-- for the single source of truth this JSON is hand-synced to, and that
-- function's own top doc comment for:
--   - the negationInd-takes-priority status logic (already fixed in Go
--     before this session; the inventory's "confirmed bug" text was stale)
--   - the performer field fix this session made (entry.Performers, not
--     entry.Participants -- a real, confirmed bug, fixed in
--     immunization_mapper.go alongside this port)
--   - the deliberate, documented, evidence-free divergence on RSON gating
--     (statusReason isn't conditioned on negationInd here, unlike Go)

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'immunizations', 'Immunization', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "immunization_status_to_fhir",
    "targetPath": "status",
    "condition": {
      "whenPath": "negationInd",
      "equals": "true",
      "thenLiteralValue": "not-done",
      "thenTransform": "string_direct"
    }
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "targetPath": "statusReason"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "vaccineCode"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "occurrenceDateTime"
  },
  {
    "sourcePath": "routeCode",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "route"
  },
  {
    "sourcePath": "doseQuantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "doseQuantity"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.lotNumberText",
    "targetPath": "lotNumber"
  },
  {
    "scope": "performers[0].assignedEntity.assignedPerson.names[0]",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "performer[0].actor"
  },
  {
    "scope": "id[*]",
    "collectAll": true,
    "transform": "cda_ii_to_identifier",
    "targetPath": "identifier"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturerOrganization.names[0]",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "manufacturer"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
