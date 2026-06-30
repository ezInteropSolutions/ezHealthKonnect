-- V166__CDA_Declarative_Mapping_Rules_Encounter_PractitionerRole.sql
--
-- Re-seeds Encounters (previously seeded by V157) to match
-- declarative_oob_rules.go's current EncounterMappingRules(): the
-- performers[*] row's emitted Practitioner ("individual") is replaced with
-- assignedEntityRoleRow's PractitionerRole -> {Practitioner, Organization},
-- the same gap-closing change CareTeamMappingRules already got in V164/V165.
--
-- This is a like-for-like swap, not an addition with a duplication risk to
-- manage: performers[*] is the ONLY row in this rule that ever emits a
-- Practitioner -- the sibling participants[*] row only ever produces a
-- display-only {display: name} reference, never an emitted resource -- so
-- there's no second Practitioner-for-the-same-person anywhere else in this
-- rule to dedup-correlate against (unlike Author, where the same id is
-- frequently absent and a literal reference was used instead -- see V165's
-- own doc comment). assignedEntity.Addresses/.RepresentedOrganization were
-- parsed but never read here, same root cause as every other occurrence of
-- this gap in this file.
--
-- IMPORTANT for any future migration touching this table: V165 changed
-- cda_declarative_mapping_rules' unique constraint to
-- (document_type, ccda_version, fhir_version, section_key, fhir_resource,
-- header_path) -- six columns, not five. ON CONFLICT targets below (and in
-- any future migration) must list all six or Postgres will reject the
-- statement with "no unique or exclusion constraint matching the ON
-- CONFLICT specification".
--
-- Also resyncs the nested diagnosis Condition's fields (conditionFields
-- ("encounter-diagnosis")) to the function's CURRENT output: V157's own
-- seed predates conditionFields gaining severity, a literal category
-- (instead of re-reading entry.Code), onsetDateTime, and abatementDateTime
-- -- a pre-existing, narrow drift unrelated to the PractitionerRole change
-- above, caught only because this migration's own drift test does a full
-- field-by-field comparison of the whole rule, nested diagnosis block
-- included. No other migration's seed has this gap (every other
-- conditionFields(...) call site's seed is already in sync).
--
-- See services/cda_fhir/declarative_oob_rules.go's EncounterMappingRules()
-- for the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v166_test.go for the drift guard.

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
    "transform": "cda_ii_to_identifier"
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
