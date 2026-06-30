-- V186__CDA_Declarative_Mapping_Rules_HealthConcern_AssessmentScale.sql
--
-- Seeds the new healthConcerns/Observation rule that maps Health Concern Act
-- entries (templateId .4.132) whose REFR-linked nested observation is an
-- Assessment Scale Observation (templateId .4.69, e.g. PHQ-9 44261-6, PHQ-2
-- 55758-7) to FHIR Observation resources.
--
-- Real gap: Ascension and Epic 99397 both had PHQ-9/PHQ-2 health concern
-- entries producing wrong Condition resources before this rule. The new
-- "refrTemplateId" EntryMatch predicate (cda_path_resolver.go) checks the
-- REFR-linked nested observation's own templateIds, since the outer Health
-- Concern Act always carries .4.132 regardless of what's inside.
--
-- The existing healthConcerns/Condition row (V175) remains unchanged and
-- handles genuine problem-type health concerns. Both rows coexist in the DB
-- (different fhir_resource value = no ON CONFLICT clash).

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'healthConcerns', 'Observation', 'refrTemplateId=2.16.840.1.113883.10.20.22.4.69', 0,
    $rules$
[
  {
    "sourcePath": "entryRelationships[typeCode=REFR].entry.code",
    "targetPath": "code",
    "transform": "cda_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "entryRelationships[typeCode=REFR].entry.statusCode",
    "targetPath": "status",
    "transform": "observation_status_to_fhir"
  },
  {
    "sourcePath": "entryRelationships[typeCode=REFR].entry.effectiveTime",
    "targetPath": "effectiveDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "literalValue": "survey",
    "targetPath": "category[0]",
    "transform": "observation_category_to_fhir"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry.value[type=INT]",
    "sourcePath": "integer",
    "targetPath": "valueInteger"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry.value[type=REAL]",
    "sourcePath": "real",
    "targetPath": "valueQuantity",
    "transform": "cda_real_to_quantity"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry.value[type=CD]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry.value[type=CE]",
      "entryRelationships[typeCode=REFR].entry.value[type=CS]"
    ],
    "sourcePath": "code",
    "targetPath": "valueCodeableConcept",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "scope": "entryRelationships[typeCode=REFR].entry.interpretationCode",
    "targetPath": "interpretation[0]",
    "transform": "cda_code_to_codeable_concept"
  }
]
    $rules$::jsonb,
    false, false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
