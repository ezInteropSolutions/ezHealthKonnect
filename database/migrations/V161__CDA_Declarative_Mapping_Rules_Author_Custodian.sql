-- V161__CDA_Declarative_Mapping_Rules_Author_Custodian.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping
-- Engine. Adds a "header_path" column (MappingRule.HeaderPath, see
-- declarative_schema.go) and seeds the document-header rules for Author
-- (-> Practitioner) and Custodian (-> Organization) — the first rules in
-- this table that don't target a section at all. patient_mapper.go's
-- MapAuthor/MapCustodian are called directly from document_mapper.go
-- before the section-key dispatch loop even starts (doc.Header.Authors /
-- doc.Header.Custodian), so section_key is '' for both rows here;
-- header_path carries the equivalent selector instead. BuildHeaderResource
-- (not BuildResources/BuildResourcesForRules) is what reads header_path.
--
-- See services/cda_fhir/declarative_oob_rules.go's AuthorMappingRules()/
-- CustodianMappingRules() for the single source of truth this JSON is
-- hand-synced to.
--
-- Phase 4 Slice A update: both identifier rows now carry
-- "embedCDAIdentity": true, matching MappingRow.EmbedCDAIdentity added to
-- the Go literal (mirrors patient_mapper.go's embedCDAIds calls for Author/
-- Custodian).
--
-- Phase 4 Slice B update: building the document-level orchestrator found
-- kareo_sample.xml's one document author has a structurally-present but
-- textually-empty name (no given/family/text) and a nullFlavor identifier --
-- patient_mapper.go's MapAuthor never checks whether any field beyond
-- resourceType+id actually got set, so it still emits a placeholder
-- Practitioner. The Author row now sets the new MappingRule.SkipEmptyCheck
-- (its own dedicated column, same convention as flatten_organizers/
-- header_path) to match; Custodian's row is unaffected (MapCustodian's own
-- `len(org.Names) == 0` guard is content-based already, not vacuous).

ALTER TABLE cda_declarative_mapping_rules
    ADD COLUMN IF NOT EXISTS header_path VARCHAR(100) NOT NULL DEFAULT '';

COMMENT ON COLUMN cda_declarative_mapping_rules.header_path IS
    'MappingRule.HeaderPath (declarative_schema.go) -- when non-empty, BuildHeaderResource resolves this against documentMap["header"] instead of section_key''s entries. Exactly one of section_key/header_path is set per row.';

ALTER TABLE cda_declarative_mapping_rules
    ADD COLUMN IF NOT EXISTS skip_empty_check BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN cda_declarative_mapping_rules.skip_empty_check IS
    'MappingRule.SkipEmptyCheck (declarative_schema.go) -- keep the built resource even when no Fields row set anything beyond resourceType. See AuthorMappingRules() in declarative_oob_rules.go.';

-- =========================================================
-- SEED: header.authors -> Practitioner
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, header_path, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, skip_empty_check, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', '', 'authors', 'Practitioner', '', 0,
    $rules$
[
  {"scope": "assignedAuthor.assignedPerson.names[*]", "transform": "cda_name_to_fhir", "collectAll": true, "targetPath": "name"},
  {"scope": "assignedAuthor.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
  {"scope": "assignedAuthor.telecoms[*]", "transform": "cda_telecom_to_fhir", "collectAll": true, "targetPath": "telecom"}
]
    $rules$::jsonb,
    false, ARRAY[]::text[], true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    header_path = EXCLUDED.header_path, entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order,
    fields = EXCLUDED.fields, flatten_organizers = EXCLUDED.flatten_organizers,
    required_paths = EXCLUDED.required_paths, skip_empty_check = EXCLUDED.skip_empty_check, updated_at = NOW();

-- =========================================================
-- SEED: header.custodian -> Organization
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, header_path, fhir_resource, entry_match, rule_order, fields, flatten_organizers, required_paths, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', '', 'custodian', 'Organization', '', 0,
    $rules$
[
  {"sourcePath": "assignedCustodian.representedCustodianOrganization.names[0]", "targetPath": "name"},
  {"literalValue": true, "targetPath": "active"},
  {"scope": "assignedCustodian.representedCustodianOrganization.ids[*]", "transform": "cda_ii_to_identifier", "collectAll": true, "targetPath": "identifier", "embedCDAIdentity": true},
  {"scope": "assignedCustodian.representedCustodianOrganization.addresses[*]", "transform": "cda_address_to_fhir", "collectAll": true, "targetPath": "address"}
]
    $rules$::jsonb,
    false, ARRAY['name'], true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    header_path = EXCLUDED.header_path, entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order,
    fields = EXCLUDED.fields, flatten_organizers = EXCLUDED.flatten_organizers,
    required_paths = EXCLUDED.required_paths, updated_at = NOW();
