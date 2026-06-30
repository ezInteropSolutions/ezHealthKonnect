-- V168__CDA_Declarative_Mapping_Rules_Condition_Recorder.sql
--
-- Re-seeds Problems (V154), Health Concerns (V154), and Encounters (V167)
-- to add a Condition.recorder row -- conditionFields()'s one shared helper
-- (declarative_oob_rules.go) feeds all three, so all three drift
-- simultaneously from this one Go-side change.
--
-- The Problem Observation's own <author> (entry.Authors, parsed generically
-- for every CDA entry type by entry_parser.go, distinct from header.Authors)
-- was parsed but never read by any mapper or rule. Per the HL7 C-CDA on
-- FHIR Implementation Guide's own Problems mapping page
-- (build.fhir.org/ig/HL7/ccda-on-fhir/CF-problems.html, verified via WebFetch
-- 2026-06-27): "/entryRelationship[@typeCode='SUBJ']/observation/author" ->
-- ".recorder ... .recorder should be authoritative (latest) author if
-- there are multiple" -- the nested Problem Observation's author, not the
-- outer Concern Act's. "authors[0]" takes the first (not necessarily
-- latest -- no corpus evidence of multiple authors on one Problem
-- Observation to justify sorting by <time>).
--
-- assignedEntityRoleRow (not organizationLinkRow): there is no pre-existing
-- Practitioner with a known id to link to here -- a fresh
-- PractitionerRole+Practitioner+Organization is emitted, same shape
-- CareTeam's/Encounter's own performer rows already use for the identical
-- "no known id to link to" situation. Condition.recorder accepts
-- PractitionerRole as well as Practitioner (FHIR R4: Reference(Practitioner|
-- PractitionerRole|Patient|RelatedPerson)).
--
-- See services/cda_fhir/declarative_oob_rules.go's conditionFields() for
-- the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v168_test.go for the drift guard (V154's
-- own Problems/HealthConcerns drift coverage and V167's Encounters drift
-- guard are both retargeted to this file).

-- =========================================================
-- problems -> Condition
-- =========================================================

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
    "transform": "cda_value_or_code_to_codeable_concept",
    "targetPath": "code",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "statusCode",
    "transform": "condition_status_to_fhir",
    "targetPath": "clinicalStatus",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "literalValue": "confirmed",
    "transform": "condition_verification_status_to_fhir",
    "targetPath": "verificationStatus",
    "condition": {
      "whenPath": "negationInd",
      "whenPaths": [
        "entryRelationships[typeCode=SUBJ].entry.negationInd",
        "entryRelationships[typeCode=REFR].entry.negationInd"
      ],
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
      "entryRelationships[typeCode=SUBJ].entry[code=SEV]"
    ],
    "sourcePath": "value.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "severity"
  },
  {
    "literalValue": "problem-list-item",
    "transform": "condition_category_to_fhir",
    "targetPath": "category[0]"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "onsetDateTime"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime.high",
    "transform": "cda_time_to_fhir_datetime",
    "targetPath": "abatementDateTime"
  },
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "recorder",
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
          {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
    ]
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- =========================================================
-- healthConcerns -> Condition
-- =========================================================

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
    "transform": "cda_value_or_code_to_codeable_concept",
    "targetPath": "code",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "statusCode",
    "transform": "condition_status_to_fhir",
    "targetPath": "clinicalStatus",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "literalValue": "confirmed",
    "transform": "condition_verification_status_to_fhir",
    "targetPath": "verificationStatus",
    "condition": {
      "whenPath": "negationInd",
      "whenPaths": [
        "entryRelationships[typeCode=SUBJ].entry.negationInd",
        "entryRelationships[typeCode=REFR].entry.negationInd"
      ],
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
      "entryRelationships[typeCode=SUBJ].entry[code=SEV]"
    ],
    "sourcePath": "value.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "severity"
  },
  {
    "literalValue": "health-concern",
    "transform": "condition_category_to_fhir",
    "targetPath": "category[0]"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "onsetDateTime"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
    "sourcePath": "effectiveTime.high",
    "transform": "cda_time_to_fhir_datetime",
    "targetPath": "abatementDateTime"
  },
  {
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": ["organization"],
    "targetPath": "recorder",
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner",
        "fields": [
          {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
          {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "emitAsResource": "Organization",
        "targetPath": "organization",
        "fields": [
          {"sourcePath": "names[0]", "targetPath": "name"},
          {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
          {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
          {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
        ]
      },
      {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
      {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
    ]
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- =========================================================
-- encounters -> Encounter (nested diagnosis Condition only)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'encounters', 'Encounter', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "fallbackPaths": ["statusCode"], "literalValue": "", "transform": "encounter_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "encounter_class_coding", "targetPath": "class"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "type[0]"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "period"},
  {
    "scope": "participants[*]",
    "collectAll": true,
    "targetPath": "participant",
    "fields": [
      {"sourcePath": "typeCode", "transform": "encounter_participant_type_coding", "targetPath": "type[0]"},
      {"scope": "participantRole.playingEntity.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "individual"}
    ]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
    "sourcePath": "participantRole.playingEntity.names[0].family",
    "fallbackPaths": ["participantRole.playingEntity.code.displayName"],
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "location[0].location"
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
    "targetPath": "participant",
    "fields": [
      {"sourcePath": "typeCode", "transform": "encounter_participant_type_coding", "targetPath": "type[0]"},
      {
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": ["organization"],
        "targetPath": "individual",
        "fields": [
          {
            "scope": "assignedEntity",
            "emitAsResource": "Practitioner",
            "targetPath": "practitioner",
            "fields": [
              {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
              {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
              {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
              {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
              {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
            ]
          },
          {
            "scope": "assignedEntity.representedOrganization",
            "emitAsResource": "Organization",
            "targetPath": "organization",
            "fields": [
              {"sourcePath": "names[0]", "targetPath": "name"},
              {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
              {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
              {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
            ]
          },
          {"sourcePath": "assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
          {"scope": "assignedEntity.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
        ]
      }
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "collectAll": true,
    "targetPath": "diagnosis",
    "fields": [
      {
        "emitAsResource": "Condition",
        "emitAsResourcePatientRefPath": ["subject"],
        "targetPath": "condition",
        "fields": [
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "transform": "cda_value_or_code_to_codeable_concept",
            "targetPath": "code",
            "required": true,
            "conformance": "SHALL"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "statusCode",
            "transform": "condition_status_to_fhir",
            "targetPath": "clinicalStatus",
            "required": true,
            "conformance": "SHALL"
          },
          {
            "literalValue": "confirmed",
            "transform": "condition_verification_status_to_fhir",
            "targetPath": "verificationStatus",
            "condition": {
              "whenPath": "negationInd",
              "whenPaths": [
                "entryRelationships[typeCode=SUBJ].entry.negationInd",
                "entryRelationships[typeCode=REFR].entry.negationInd"
              ],
              "equals": "true",
              "thenLiteralValue": "refuted"
            }
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ].entry[code=SEV]",
              "entryRelationships[typeCode=SUBJ].entry[code=SEV]"
            ],
            "sourcePath": "value.code",
            "transform": "cda_code_to_codeable_concept",
            "targetPath": "severity"
          },
          {
            "literalValue": "encounter-diagnosis",
            "transform": "condition_category_to_fhir",
            "targetPath": "category[0]"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "effectiveTime",
            "transform": "cda_timerange_to_onset",
            "targetPath": "onsetDateTime"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": ["entryRelationships[typeCode=REFR].entry", ""],
            "sourcePath": "effectiveTime.high",
            "transform": "cda_time_to_fhir_datetime",
            "targetPath": "abatementDateTime"
          },
          {
            "emitAsResource": "PractitionerRole",
            "emitAsResourceRequiredPaths": ["organization"],
            "targetPath": "recorder",
            "fields": [
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
                "emitAsResource": "Practitioner",
                "targetPath": "practitioner",
                "fields": [
                  {"scope": "assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
                  {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
                  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
                  {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
                  {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
                ]
              },
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
                "emitAsResource": "Organization",
                "targetPath": "organization",
                "fields": [
                  {"sourcePath": "names[0]", "targetPath": "name"},
                  {"scope": "ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
                  {"scope": "telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"},
                  {"scope": "addresses[*]", "collectAll": true, "transform": "cda_address_to_fhir", "targetPath": "address"}
                ]
              },
              {"sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code", "transform": "cda_code_to_codeable_concept", "targetPath": "specialty[0]"},
              {"scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]", "collectAll": true, "transform": "cda_telecom_to_fhir", "targetPath": "telecom"}
            ]
          }
        ]
      }
    ]
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
