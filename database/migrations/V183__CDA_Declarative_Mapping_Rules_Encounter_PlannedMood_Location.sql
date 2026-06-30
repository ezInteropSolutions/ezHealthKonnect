-- V183__CDA_Declarative_Mapping_Rules_Encounter_PlannedMood_Location.sql
--
-- Two fixes to encounterFields() (declarative_oob_rules.go), the Fields
-- slice shared by EncounterMappingRules() (the Encounters section) and, at
-- runtime, PlanOfCareMappingRules()'s entryType=encounter branch when an
-- interface/pipeline resolves its target resource to "Encounter" (see
-- declarative_document_mapper.go's planOfCareEncounterTarget resolution +
-- applyPlanOfCareEncounterTarget swap -- a per-document runtime decision,
-- NOT a change to any OOB rule's own hardcoded definition, which is why
-- there is no parallel "Encounter variant" row seeded anywhere for
-- PlanOfCareMappingRules() itself).
--
-- Both fixes found auditing a real Ascension Wisconsin CCD's Plan-of-Care
-- section: 18 "Encounter Activity"-shaped entries, classCode="ENC"
-- moodCode="APT" (a booked future visit).
--
-- 1. Status: every one of those entries carries statusCode="active" -- the
--    SAME value a completed (moodCode=EVN) encounter would carry.
--    encounter_status_to_fhir alone maps "active" to FHIR "in-progress",
--    wrong for a visit that hasn't happened yet. A new row reads moodCode
--    via the new encounter_status_from_planned_mood transform (APT/INT/
--    PRP/RQO/ARQ -> "planned") and runs FIRST; the existing statusCode row
--    gets skipIfResourceHasAnyOf:["status"] so it only fires for the EVN
--    case, unchanged from today.
-- 2. Location: the existing location row's scope only matches a LOC
--    participant wrapped in a COMP entryRelationship. Ascension's data has
--    a DIRECT, unwrapped participant typeCode=LOC on the entry itself --
--    added as a scopeFallbacks entry, same "wrapped, else direct" idiom as
--    problemObsScopeFallbacks elsewhere in this file.
--
-- See encounterFields()'s own doc comment in declarative_oob_rules.go for
-- the single source of truth this SQL is hand-synced to;
-- declarative_oob_rules_migration_v183_test.go is the drift guard.
--
-- Supersedes V175 for this ONE row's Fields content only (entries_match/
-- flatten_organizers unchanged) -- V175 remains the owner of the
-- problems/Condition and healthConcerns/Condition rows seeded alongside it.

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
