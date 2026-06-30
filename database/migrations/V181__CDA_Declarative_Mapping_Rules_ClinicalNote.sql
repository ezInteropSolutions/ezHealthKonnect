-- V181__CDA_Declarative_Mapping_Rules_ClinicalNote.sql
--
-- First-time OOB seed for ClinicalNoteMappingRules() (declarative_oob_rules.go)
-- -- a Note Activity entry (templateId 2.16.840.1.113883.10.20.22.4.202,
-- extension 2016-11-01) mapped to one US Core DocumentReference.
--
-- Found auditing the "historyOfPresentIllness" section (LOINC 10164-2) of
-- the 99397 sample: Epic titles this section "Progress Notes" and uses it
-- to carry the entire visit note as a single Note Activity act -- NOT the
-- official C-CDA R2.1 "Progress Note (V3)" document template (.1.9,
-- confirmed absent from every sample this session) or the narrative-only
-- "Assessment Section" (.2.8, no entry at all per the IG). The section's
-- own templateId (1.3.6.1.4.1.19376.1.5.3.1.3.4, the legacy IHE
-- Consolidated CDA "History of Present Illness Section", CONF:9566) is not
-- registered in ccda_2_1.json -- resolveKey falls through to the LOINC
-- match on 10164-2, which IS registered under "historyOfPresentIllness",
-- so SectionKey is correct despite the templateId mismatch.
--
-- User-confirmed this exact entry shape recurs across every Epic site they
-- have seen, not just this one sample.
--
-- See ClinicalNoteMappingRules' own doc comment in declarative_oob_rules.go
-- for full field-by-field provenance; declarative_oob_rules_migration_v181_test.go
-- is the drift guard.
--
-- This is the first OOB rule for the "historyOfPresentIllness" section key
-- and the first to emit a DocumentReference resource.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'historyOfPresentIllness', 'DocumentReference', 'templateId=2.16.840.1.113883.10.20.22.4.202', 0,
    $rules$
[
  {
    "sourcePath": "code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "type"
  },
  {
    "literalValue": {
      "coding": [
        {
          "system": "http://hl7.org/fhir/us/core/CodeSystem/us-core-documentreference-category",
          "code": "clinical-note",
          "display": "Clinical Note"
        }
      ]
    },
    "targetPath": "category[0]"
  },
  {
    "literalValue": "current",
    "targetPath": "status"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "date"
  },
  {
    "sourcePath": "text",
    "transform": "cda_text_to_attachment",
    "targetPath": "content[0].attachment"
  },
  {
    "scope": "id[*]",
    "collectAll": true,
    "transform": "cda_ii_to_identifier",
    "targetPath": "identifier"
  },
  {
    "scope": "authors[0].assignedAuthor",
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
      {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
      {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
      {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "author[0]"
  },
  {
    "scope": "informants[0].assignedEntity.representedOrganization",
    "fields": [
      {"sourcePath": "names[0]", "targetPath": "name"},
      {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "targetPath": "identifier", "embedCDAIdentity": true},
      {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
      {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
    ],
    "emitAsResource": "Organization",
    "targetPath": "custodian"
  }
]
    $rules$::jsonb,
    false, false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
