-- V188__CDA_Declarative_Mapping_Rules_Condition_Allergy_Asserter_OnsetAge.sql
--
-- Adds three new fields to conditionFields() (shared by problems, healthConcerns,
-- and encounters' nested diagnosis Condition) and five new fields to
-- AllergyMappingRules() (allergiesAndIntolerances), found by auditing the
-- PracticeFusion and Kareo CCDs against HL7's C-CDA on FHIR IG:
--
-- Condition (problems / healthConcerns / encounters nested diagnosis):
--   1. Condition.asserter (PractitionerRole tier) -- the Problem Observation's
--      own <performer><assignedEntity>. IG: CF-problems.html "/performer ->
--      .asserter". PracticeFusion corpus: performer on Active Problems entries.
--
--   2. Condition.asserter (bare Practitioner fallback) -- fires when no
--      representedOrganization is present (same SkipIfResourceHasAnyOf gate
--      as the recorder fallback introduced in V175).
--
--   3. Condition.onsetAge -- Age At Onset Observation (C-CDA templateId
--      2.16.840.1.113883.10.20.22.4.31, SNOMED 445518008) nested inside the
--      Problem Observation via entryRelationship typeCode=SUBJ,inversionInd=true.
--      PracticeFusion corpus: Asthma entry carries PQ value "5 a" (5 years old).
--
-- AllergyIntolerance (allergiesAndIntolerances):
--   4. AllergyIntolerance.recorder (PractitionerRole tier) -- allergy observation
--      own <author><assignedAuthor>. IG: CF-allergies.html "/author -> .recorder".
--      PracticeFusion corpus: both allergy entries have <author> with NPI + name.
--
--   5. AllergyIntolerance.recorder (bare Practitioner fallback) -- fires when
--      no representedOrganization is present.
--
--   6. AllergyIntolerance.recordedDate -- allergy observation own <author><time>.
--      Same IG source as recorder. Mirrors Condition.recordedDate (V175).
--
--   7. AllergyIntolerance.asserter (PractitionerRole tier) -- allergy observation
--      own <performer><assignedEntity>. IG: CF-allergies.html "/performer ->
--      .asserter". PracticeFusion corpus: "Samir Khan" with NPI, address, telecom.
--
--   8. AllergyIntolerance.asserter (bare Practitioner fallback).
--
-- Supersedes V175 (problems/Condition, healthConcerns/Condition) for Fields content.
-- Supersedes V183 (encounters/Encounter) for Fields content.
-- Supersedes V154 (allergiesAndIntolerances/AllergyIntolerance) for Fields content.
-- Applied: 2026-07-01

-- ============================================================
-- problems -> Condition
-- ============================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'problems', 'Condition', '', 0,
    $rules$
[
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "statusCode",
    "targetPath": "clinicalStatus",
    "transform": "condition_status_to_fhir",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "literalValue": "confirmed",
    "targetPath": "verificationStatus",
    "transform": "condition_verification_status_to_fhir",
    "condition": {
      "whenPath": "negationInd",
      "whenPaths": ["entryRelationships[typeCode=SUBJ].entry.negationInd", "entryRelationships[typeCode=REFR].entry.negationInd"],
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]", "entryRelationships[typeCode=SUBJ].entry[code=SEV]"],
    "sourcePath": "value.code",
    "targetPath": "severity",
    "transform": "cda_code_to_codeable_concept"
  },
  {"literalValue": "problem-list-item", "targetPath": "category[0]", "transform": "condition_category_to_fhir"},
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "recorder"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": ["recorder"]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "authors[0].time",
    "targetPath": "recordedDate",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.202]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "asserter"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": ["asserter"]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]"],
    "sourcePath": "value",
    "transform": "cda_value_to_fhir",
    "targetPath": "onsetAge"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- ============================================================
-- healthConcerns -> Condition
-- ============================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'healthConcerns', 'Condition', '', 0,
    $rules$
[
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "statusCode",
    "targetPath": "clinicalStatus",
    "transform": "condition_status_to_fhir",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "literalValue": "confirmed",
    "targetPath": "verificationStatus",
    "transform": "condition_verification_status_to_fhir",
    "condition": {
      "whenPath": "negationInd",
      "whenPaths": ["entryRelationships[typeCode=SUBJ].entry.negationInd", "entryRelationships[typeCode=REFR].entry.negationInd"],
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]", "entryRelationships[typeCode=SUBJ].entry[code=SEV]"],
    "sourcePath": "value.code",
    "targetPath": "severity",
    "transform": "cda_code_to_codeable_concept"
  },
  {"literalValue": "health-concern", "targetPath": "category[0]", "transform": "condition_category_to_fhir"},
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "recorder"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": ["recorder"]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "authors[0].time",
    "targetPath": "recordedDate",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.202]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "asserter"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": ["asserter"]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]"],
    "sourcePath": "value",
    "transform": "cda_value_to_fhir",
    "targetPath": "onsetAge"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- ============================================================
-- allergiesAndIntolerances -> AllergyIntolerance
-- ============================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'allergiesAndIntolerances', 'AllergyIntolerance', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "allergy_status_to_fhir",
    "targetPath": "clinicalStatus",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "onsetDateTime"
  },
  {
    "literalValue": "confirmed",
    "transform": "allergy_verification_status_to_fhir",
    "targetPath": "verificationStatus"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [""],
    "sourcePath": "value.code.code",
    "transform": "allergy_type_to_fhir",
    "targetPath": "type"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [""],
    "transform": "allergy_substance_or_no_known_allergy_to_fhir",
    "targetPath": "code",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM]",
    "scopeFallbacks": ["participants[typeCode=CSM]"],
    "sourcePath": "participantRole.playingEntity.code.codeSystem",
    "transform": "allergy_category_from_substance_system",
    "targetPath": "category"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=82606-5]",
    "scopeFallbacks": ["entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=82606-5]"],
    "sourcePath": "value.code.code",
    "transform": "allergy_criticality_to_fhir",
    "targetPath": "criticality"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.9]",
    "scopeFallbacks": ["entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.9]"],
    "collectAll": true,
    "targetPath": "reaction",
    "fields": [
      {
        "transform": "cda_value_or_code_to_codeable_concept",
        "targetPath": "manifestation[0]"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.8]",
        "sourcePath": "value.code.code",
        "transform": "allergy_reaction_severity_to_fhir",
        "targetPath": "severity"
      }
    ]
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "recorder"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
    "scopeFallbacks": ["authors[0].assignedAuthor"],
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": ["recorder"]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [""],
    "sourcePath": "authors[0].time",
    "transform": "cda_time_to_fhir_datetime",
    "targetPath": "recordedDate"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
          {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
          {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "asserter"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
    "scopeFallbacks": ["performers[0].assignedEntity"],
    "fields": [
      {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
      {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
      {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
      {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
      {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": ["asserter"]
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- ============================================================
-- encounters -> Encounter  (supersedes V183 for Fields content)
-- Adds asserter (PractitionerRole + bare Practitioner) and
-- onsetAge to the nested Condition's conditionFields block.
-- All other rows are identical to V183.
-- ============================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'encounters', 'Encounter', '', 0,
    $rules$
[
  {
    "sourcePath": "moodCode",
    "targetPath": "status",
    "transform": "encounter_status_from_planned_mood"
  },
  {
    "sourcePath": "statusCode",
    "fallbackPaths": ["statusCode"],
    "literalValue": "",
    "targetPath": "status",
    "transform": "encounter_status_to_fhir",
    "skipIfResourceHasAnyOf": ["status"]
  },
  {
    "sourcePath": "code",
    "targetPath": "class",
    "transform": "encounter_class_coding"
  },
  {
    "sourcePath": "code",
    "targetPath": "type[0]",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "period",
    "transform": "cda_timerange_to_period"
  },
  {
    "scope": "participants[*]",
    "collectAll": true,
    "fields": [
      {"sourcePath": "typeCode", "targetPath": "type[0]", "transform": "encounter_participant_type_coding"},
      {"scope": "participantRole.playingEntity.names[0]", "targetPath": "individual", "transform": "cda_name_or_literal_to_display_ref"}
    ],
    "targetPath": "participant"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
    "scopeFallbacks": ["participants[typeCode=LOC]"],
    "sourcePath": "participantRole.playingEntity.names[0].family",
    "fallbackPaths": ["participantRole.playingEntity.code.displayName"],
    "targetPath": "location[0].location",
    "transform": "cda_name_or_literal_to_display_ref"
  },
  {
    "scope": "id[*]",
    "collectAll": true,
    "targetPath": "identifier",
    "transform": "cda_ii_to_identifier",
    "embedCDAIdentity": true
  },
  {
    "scope": "performers[*]",
    "collectAll": true,
    "fields": [
      {"sourcePath": "typeCode", "targetPath": "type[0]", "transform": "encounter_participant_type_coding"},
      {
        "fields": [
          {
            "scope": "assignedEntity",
            "fields": [
              {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
              {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
              {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
              {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
              {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
            ],
            "emitAsResource": "Practitioner",
            "targetPath": "practitioner"
          },
          {
            "scope": "assignedEntity.representedOrganization",
            "fields": [
              {"sourcePath": "names[0]", "targetPath": "name"},
              {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
              {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
              {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
            ],
            "emitAsResource": "Organization",
            "targetPath": "organization"
          },
          {"sourcePath": "assignedEntity.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
          {"scope": "assignedEntity.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
        ],
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": ["organization"],
        "targetPath": "individual"
      }
    ],
    "targetPath": "participant"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "collectAll": true,
    "fields": [
      {
        "fields": [
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "targetPath": "code",
            "transform": "cda_value_or_code_to_codeable_concept",
            "required": true,
            "conformance": "SHALL"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "statusCode",
            "targetPath": "clinicalStatus",
            "transform": "condition_status_to_fhir",
            "required": true,
            "conformance": "SHALL"
          },
          {
            "literalValue": "confirmed",
            "targetPath": "verificationStatus",
            "transform": "condition_verification_status_to_fhir",
            "condition": {
              "whenPath": "negationInd",
              "whenPaths": ["entryRelationships[typeCode=SUBJ].entry.negationInd", "entryRelationships[typeCode=REFR].entry.negationInd"],
              "equals": "true",
              "thenLiteralValue": "refuted"
            }
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]", "entryRelationships[typeCode=SUBJ].entry[code=SEV]"],
            "sourcePath": "value.code",
            "targetPath": "severity",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "literalValue": "encounter-diagnosis",
            "targetPath": "category[0]",
            "transform": "condition_category_to_fhir"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "effectiveTime",
            "targetPath": "onsetDateTime",
            "transform": "cda_timerange_to_onset"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "effectiveTime.high",
            "targetPath": "abatementDateTime",
            "transform": "cda_time_to_fhir_datetime"
          },
          {
            "fields": [
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
                "fields": [
                  {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
                  {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
                  {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
                  {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
                  {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
                ],
                "emitAsResource": "Practitioner",
                "targetPath": "practitioner"
              },
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
                "fields": [
                  {"sourcePath": "names[0]", "targetPath": "name"},
                  {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
                  {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
                  {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
                ],
                "emitAsResource": "Organization",
                "targetPath": "organization"
              },
              {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
              {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
            ],
            "emitAsResource": "PractitionerRole",
            "emitAsResourceRequiredPaths": ["organization"],
            "targetPath": "recorder"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
            "fields": [
              {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
              {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
              {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
              {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
              {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
            ],
            "emitAsResource": "Practitioner",
            "targetPath": "recorder",
            "skipIfResourceHasAnyOf": ["recorder"]
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "authors[0].time",
            "targetPath": "recordedDate",
            "transform": "cda_time_to_fhir_datetime"
          },
          {
            "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.202]",
            "sourcePath": "text",
            "targetPath": "note[0].text"
          },
          {
            "fields": [
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
                "fields": [
                  {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
                  {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
                  {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
                  {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
                  {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
                ],
                "emitAsResource": "Practitioner",
                "targetPath": "practitioner"
              },
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
                "fields": [
                  {"sourcePath": "names[0]", "targetPath": "name"},
                  {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
                  {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
                  {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
                ],
                "emitAsResource": "Organization",
                "targetPath": "organization"
              },
              {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code", "targetPath": "specialty[0]", "transform": "cda_code_to_codeable_concept"},
              {"scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"}
            ],
            "emitAsResource": "PractitionerRole",
            "emitAsResourceRequiredPaths": ["organization"],
            "targetPath": "asserter"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
            "fields": [
              {"scope": "assignedPerson.names[*]", "collectAll": true, "targetPath": "name", "transform": "cda_name_to_fhir"},
              {"scope": "ids[*]", "collectAll": true, "targetPath": "identifier", "transform": "cda_ii_to_identifier", "embedCDAIdentity": true},
              {"sourcePath": "code", "targetPath": "qualification[0].code", "transform": "cda_code_to_codeable_concept"},
              {"scope": "telecoms[*]", "collectAll": true, "targetPath": "telecom", "transform": "cda_telecom_to_fhir"},
              {"scope": "addresses[*]", "collectAll": true, "targetPath": "address", "transform": "cda_address_to_fhir"}
            ],
            "emitAsResource": "Practitioner",
            "targetPath": "asserter",
            "skipIfResourceHasAnyOf": ["asserter"]
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]"],
            "sourcePath": "value",
            "transform": "cda_value_to_fhir",
            "targetPath": "onsetAge"
          }
        ],
        "emitAsResource": "Condition",
        "emitAsResourcePatientRefPath": ["subject"],
        "targetPath": "condition"
      }
    ],
    "targetPath": "diagnosis"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
