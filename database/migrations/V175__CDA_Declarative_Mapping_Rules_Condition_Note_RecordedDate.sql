-- V175__CDA_Declarative_Mapping_Rules_Condition_Note_RecordedDate.sql
--
-- Three fixes to conditionFields() (shared by Problems, Health Concerns,
-- and Encounters'' own nested-diagnosis Condition), found by auditing the
-- Active Problems section of a 99397 CCD sample against HL7''s C-CDA on
-- FHIR IG (CF-problems.html):
--
-- 1. Condition.recorder fallback tier (barePractitionerRow) -- all 5 of
--    the sample''s Active Problems have a Problem Observation author with
--    an NPI but NO representedOrganization, so assignedEntityRoleRow''s own
--    EmitAsResourceRequiredPaths=["organization"] gate discarded the
--    WHOLE recorder reference for every one of them. The fallback keeps a
--    bare Practitioner (with the NPI) when the rich PractitionerRole tier
--    can''t fire.
--
-- 2. Condition.recordedDate -- IG specifies "/author/time -> .recordedDate"
--    but this was never mapped at all.
--
-- 3. Condition.note -- C-CDA''s Note Activity (templateId
--    2.16.840.1.113883.10.20.22.4.202, code 34109-9 "Note"), attached to
--    the Concern Act via entryRelationship typeCode=COMP, was never mapped
--    -- 4 of the sample''s 5 Active Problems carry a substantive provider
--    overview note that was silently dropped.
--
-- Supersedes V168 (problems/Condition, healthConcerns/Condition,
-- encounters/Encounter) entirely for Fields content -- see
-- declarative_oob_rules_migration_v175_test.go.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'problems', 'Condition', '', 0,
    $rules$
[
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
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
    "targetPath": "severity",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "literalValue": "problem-list-item",
    "targetPath": "category[0]",
    "transform": "condition_category_to_fhir"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "fields": [
          {
            "scope": "assignedPerson.names[*]",
            "collectAll": true,
            "targetPath": "name",
            "transform": "cda_name_to_fhir"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "sourcePath": "code",
            "targetPath": "qualification[0].code",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "fields": [
          {
            "sourcePath": "names[0]",
            "targetPath": "name"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {
        "sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "recorder"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
    "fields": [
      {
        "scope": "assignedPerson.names[*]",
        "collectAll": true,
        "targetPath": "name",
        "transform": "cda_name_to_fhir"
      },
      {
        "scope": "ids[*]",
        "collectAll": true,
        "targetPath": "identifier",
        "transform": "cda_ii_to_identifier",
        "embedCDAIdentity": true
      },
      {
        "sourcePath": "code",
        "targetPath": "qualification[0].code",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      },
      {
        "scope": "addresses[*]",
        "collectAll": true,
        "targetPath": "address",
        "transform": "cda_address_to_fhir"
      }
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": [
      "recorder"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "authors[0].time",
    "targetPath": "recordedDate",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.202]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
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
    'CCD', '2.1', 'R4', 'healthConcerns', 'Condition', '', 0,
    $rules$
[
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
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
    "targetPath": "severity",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "literalValue": "health-concern",
    "targetPath": "category[0]",
    "transform": "condition_category_to_fhir"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
        "fields": [
          {
            "scope": "assignedPerson.names[*]",
            "collectAll": true,
            "targetPath": "name",
            "transform": "cda_name_to_fhir"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "sourcePath": "code",
            "targetPath": "qualification[0].code",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Practitioner",
        "targetPath": "practitioner"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
        "fields": [
          {
            "sourcePath": "names[0]",
            "targetPath": "name"
          },
          {
            "scope": "ids[*]",
            "collectAll": true,
            "targetPath": "identifier",
            "transform": "cda_ii_to_identifier",
            "embedCDAIdentity": true
          },
          {
            "scope": "telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          },
          {
            "scope": "addresses[*]",
            "collectAll": true,
            "targetPath": "address",
            "transform": "cda_address_to_fhir"
          }
        ],
        "emitAsResource": "Organization",
        "targetPath": "organization"
      },
      {
        "sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "recorder"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
    "fields": [
      {
        "scope": "assignedPerson.names[*]",
        "collectAll": true,
        "targetPath": "name",
        "transform": "cda_name_to_fhir"
      },
      {
        "scope": "ids[*]",
        "collectAll": true,
        "targetPath": "identifier",
        "transform": "cda_ii_to_identifier",
        "embedCDAIdentity": true
      },
      {
        "sourcePath": "code",
        "targetPath": "qualification[0].code",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      },
      {
        "scope": "addresses[*]",
        "collectAll": true,
        "targetPath": "address",
        "transform": "cda_address_to_fhir"
      }
    ],
    "emitAsResource": "Practitioner",
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": [
      "recorder"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry",
      ""
    ],
    "sourcePath": "authors[0].time",
    "targetPath": "recordedDate",
    "transform": "cda_time_to_fhir_datetime"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.202]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'encounters', 'Encounter', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "fallbackPaths": [
      "statusCode"
    ],
    "literalValue": "",
    "targetPath": "status",
    "transform": "encounter_status_to_fhir"
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
      {
        "sourcePath": "typeCode",
        "targetPath": "type[0]",
        "transform": "encounter_participant_type_coding"
      },
      {
        "scope": "participantRole.playingEntity.names[0]",
        "targetPath": "individual",
        "transform": "cda_name_or_literal_to_display_ref"
      }
    ],
    "targetPath": "participant"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]",
    "sourcePath": "participantRole.playingEntity.names[0].family",
    "fallbackPaths": [
      "participantRole.playingEntity.code.displayName"
    ],
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
      {
        "sourcePath": "typeCode",
        "targetPath": "type[0]",
        "transform": "encounter_participant_type_coding"
      },
      {
        "fields": [
          {
            "scope": "assignedEntity",
            "fields": [
              {
                "scope": "assignedPerson.names[*]",
                "collectAll": true,
                "targetPath": "name",
                "transform": "cda_name_to_fhir"
              },
              {
                "scope": "ids[*]",
                "collectAll": true,
                "targetPath": "identifier",
                "transform": "cda_ii_to_identifier",
                "embedCDAIdentity": true
              },
              {
                "sourcePath": "code",
                "targetPath": "qualification[0].code",
                "transform": "cda_code_to_codeable_concept"
              },
              {
                "scope": "telecoms[*]",
                "collectAll": true,
                "targetPath": "telecom",
                "transform": "cda_telecom_to_fhir"
              },
              {
                "scope": "addresses[*]",
                "collectAll": true,
                "targetPath": "address",
                "transform": "cda_address_to_fhir"
              }
            ],
            "emitAsResource": "Practitioner",
            "targetPath": "practitioner"
          },
          {
            "scope": "assignedEntity.representedOrganization",
            "fields": [
              {
                "sourcePath": "names[0]",
                "targetPath": "name"
              },
              {
                "scope": "ids[*]",
                "collectAll": true,
                "targetPath": "identifier",
                "transform": "cda_ii_to_identifier",
                "embedCDAIdentity": true
              },
              {
                "scope": "telecoms[*]",
                "collectAll": true,
                "targetPath": "telecom",
                "transform": "cda_telecom_to_fhir"
              },
              {
                "scope": "addresses[*]",
                "collectAll": true,
                "targetPath": "address",
                "transform": "cda_address_to_fhir"
              }
            ],
            "emitAsResource": "Organization",
            "targetPath": "organization"
          },
          {
            "sourcePath": "assignedEntity.code",
            "targetPath": "specialty[0]",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "scope": "assignedEntity.telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          }
        ],
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": [
          "organization"
        ],
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
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry",
              ""
            ],
            "targetPath": "code",
            "transform": "cda_value_or_code_to_codeable_concept",
            "required": true,
            "conformance": "SHALL"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry",
              ""
            ],
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
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry",
              ""
            ],
            "sourcePath": "effectiveTime",
            "targetPath": "onsetDateTime",
            "transform": "cda_timerange_to_onset"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry",
              ""
            ],
            "sourcePath": "effectiveTime.high",
            "targetPath": "abatementDateTime",
            "transform": "cda_time_to_fhir_datetime"
          },
          {
            "fields": [
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
                "fields": [
                  {
                    "scope": "assignedPerson.names[*]",
                    "collectAll": true,
                    "targetPath": "name",
                    "transform": "cda_name_to_fhir"
                  },
                  {
                    "scope": "ids[*]",
                    "collectAll": true,
                    "targetPath": "identifier",
                    "transform": "cda_ii_to_identifier",
                    "embedCDAIdentity": true
                  },
                  {
                    "sourcePath": "code",
                    "targetPath": "qualification[0].code",
                    "transform": "cda_code_to_codeable_concept"
                  },
                  {
                    "scope": "telecoms[*]",
                    "collectAll": true,
                    "targetPath": "telecom",
                    "transform": "cda_telecom_to_fhir"
                  },
                  {
                    "scope": "addresses[*]",
                    "collectAll": true,
                    "targetPath": "address",
                    "transform": "cda_address_to_fhir"
                  }
                ],
                "emitAsResource": "Practitioner",
                "targetPath": "practitioner"
              },
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.representedOrganization",
                "fields": [
                  {
                    "sourcePath": "names[0]",
                    "targetPath": "name"
                  },
                  {
                    "scope": "ids[*]",
                    "collectAll": true,
                    "targetPath": "identifier",
                    "transform": "cda_ii_to_identifier",
                    "embedCDAIdentity": true
                  },
                  {
                    "scope": "telecoms[*]",
                    "collectAll": true,
                    "targetPath": "telecom",
                    "transform": "cda_telecom_to_fhir"
                  },
                  {
                    "scope": "addresses[*]",
                    "collectAll": true,
                    "targetPath": "address",
                    "transform": "cda_address_to_fhir"
                  }
                ],
                "emitAsResource": "Organization",
                "targetPath": "organization"
              },
              {
                "sourcePath": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.code",
                "targetPath": "specialty[0]",
                "transform": "cda_code_to_codeable_concept"
              },
              {
                "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor.telecoms[*]",
                "collectAll": true,
                "targetPath": "telecom",
                "transform": "cda_telecom_to_fhir"
              }
            ],
            "emitAsResource": "PractitionerRole",
            "emitAsResourceRequiredPaths": [
              "organization"
            ],
            "targetPath": "recorder"
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry.authors[0].assignedAuthor",
            "fields": [
              {
                "scope": "assignedPerson.names[*]",
                "collectAll": true,
                "targetPath": "name",
                "transform": "cda_name_to_fhir"
              },
              {
                "scope": "ids[*]",
                "collectAll": true,
                "targetPath": "identifier",
                "transform": "cda_ii_to_identifier",
                "embedCDAIdentity": true
              },
              {
                "sourcePath": "code",
                "targetPath": "qualification[0].code",
                "transform": "cda_code_to_codeable_concept"
              },
              {
                "scope": "telecoms[*]",
                "collectAll": true,
                "targetPath": "telecom",
                "transform": "cda_telecom_to_fhir"
              },
              {
                "scope": "addresses[*]",
                "collectAll": true,
                "targetPath": "address",
                "transform": "cda_address_to_fhir"
              }
            ],
            "emitAsResource": "Practitioner",
            "targetPath": "recorder",
            "skipIfResourceHasAnyOf": [
              "recorder"
            ]
          },
          {
            "scope": "entryRelationships[typeCode=SUBJ].entry",
            "scopeFallbacks": [
              "entryRelationships[typeCode=REFR].entry",
              ""
            ],
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
        "emitAsResourcePatientRefPath": [
          "subject"
        ],
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
