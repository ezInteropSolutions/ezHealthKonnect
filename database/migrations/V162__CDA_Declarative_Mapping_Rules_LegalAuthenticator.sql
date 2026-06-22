-- V162__CDA_Declarative_Mapping_Rules_LegalAuthenticator.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping
-- Engine. Seeds the document-header rule for legalAuthenticator (0..1) ->
-- Practitioner, using the header_path column V161 added.
--
-- This is genuinely NEW functionality, not a port of an existing Go
-- mapper: header.legalAuthenticator wasn't even parsed before this slice
-- (cda/document/header_parser.go's parseLegalAuthenticator, cda/document/
-- types.go's CDALegalAuthenticator). No Go mapper has ever produced a
-- resource from it. See services/cda_fhir/declarative_oob_rules.go's
-- LegalAuthenticatorMappingRules() for the single source of truth this
-- JSON is hand-synced to, including why Composition.attester wiring is
-- explicitly deferred to Phase 4.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, header_path, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', '', 'legalAuthenticator', 'Practitioner', '', 0,
    $rules$
[
  {"scope": "assignedEntity.assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
  {"scope": "assignedEntity.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier"},
  {"scope": "assignedEntity.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"},
  {"scope": "assignedEntity.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
]
    $rules$::jsonb,
    false, ARRAY['name'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    header_path = EXCLUDED.header_path, entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order,
    fields = EXCLUDED.fields, flatten_organizers = EXCLUDED.flatten_organizers,
    required_paths = EXCLUDED.required_paths, updated_at = NOW();
