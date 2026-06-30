-- V180__CDA_Declarative_Mapping_Rules_MentalStatus_AssessmentScale_COMP.sql
--
-- Assessment Scale Observation COMP-children fan-out for
-- MentalStatusMappingRules() -- same fix as V179 (functionalStatus),
-- re-parameterized with category="cognitive-status" instead of
-- "functional-status". UNLIKE V179, this is NOT reproducing a real corpus
-- finding -- no Mental Status sample available this session (99397 plus 10
-- other real CCDs) carries an Assessment Scale Observation at all; every
-- real Mental Status section seen is either empty or a single plain Mental
-- Status Observation (.4.74).
--
-- Applied on IG evidence rather than corpus evidence, after explicit user
-- sign-off: HL7 CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1_2015AUG_Vol2 (Dec 2018
-- errata), Table 146 "Mental Status Section (V2) Contexts" lists
-- "Assessment Scale Observation (optional)" as a direct entry sibling of
-- Mental Status Organizer/Observation, confirmed by templateId
-- 2.16.840.1.113883.10.20.22.4.69 in that section's own constraint #7
-- (CONF:1198-28313/28314). Table 232 "Assessment Scale Observation
-- Contexts" confirms the reverse direction too -- "Mental Status Section
-- (V2) (optional)" and "Mental Status Observation (V3) (optional)" both
-- listed as valid containers, right alongside "Functional Status Section
-- (V2)"/"Functional Status Observation (V2)" -- and names "Mini-Mental
-- Status Exam (assesses cognitive function)" as a worked example of this
-- exact template. Constraint #146 on that same page confirms the
-- entryRelationship is typeCode=COMP to Assessment Scale Supporting
-- Observation (.4.86) -- identical nesting to Functional Status's
-- already-fixed case, so the same primitive (assessmentScaleSupporting
-- ObservationRow) applies unchanged. See MentalStatusMappingRules' own doc
-- comment in declarative_oob_rules.go for the full detail this SQL is
-- hand-synced to; declarative_oob_rules_migration_v180_test.go is the
-- drift guard.
--
-- Supersedes V178 for mentalStatus/Observation's Fields content ONLY --
-- vitalSigns/results/labResults/functionalStatus/socialHistory (the other 5
-- rules sharing observationRule()) are UNCHANGED and remain owned by
-- V178/V179.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'mentalStatus', 'Observation', '', 0,
    $rules$
[
  {
    "sourcePath": "code",
    "targetPath": "code",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "text",
    "targetPath": "code.text"
  },
  {
    "sourcePath": "statusCode",
    "targetPath": "status",
    "transform": "observation_status_to_fhir"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "effectiveDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "literalValue": "cognitive-status",
    "targetPath": "category[0]",
    "transform": "observation_category_to_fhir"
  },
  {
    "fields": [
      {
        "scope": "authors[0].assignedAuthor",
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
        "scope": "authors[0].assignedAuthor.representedOrganization",
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
        "sourcePath": "authors[0].assignedAuthor.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "authors[0].assignedAuthor.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "performer[0]"
  },
  {
    "scope": "authors[0].assignedAuthor",
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
    "targetPath": "performer[0]",
    "skipIfResourceHasAnyOf": [
      "performer"
    ]
  },
  {
    "scope": "value[type=PQ]",
    "sourcePath": "quantity",
    "targetPath": "valueQuantity",
    "transform": "cda_quantity_to_fhir"
  },
  {
    "scope": "value[type=CD]",
    "scopeFallbacks": [
      "value[type=CE]",
      "value[type=CS]"
    ],
    "sourcePath": "code",
    "targetPath": "valueCodeableConcept",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "scope": "value[type=ST]",
    "scopeFallbacks": [
      "value[type=ED]"
    ],
    "sourcePath": "text",
    "targetPath": "valueString"
  },
  {
    "scope": "value[type=BL]",
    "sourcePath": "boolean",
    "targetPath": "valueBoolean"
  },
  {
    "scope": "value[type=INT]",
    "sourcePath": "integer",
    "targetPath": "valueInteger"
  },
  {
    "scope": "value[type=REAL]",
    "sourcePath": "real",
    "targetPath": "valueQuantity",
    "transform": "cda_real_to_bare_quantity"
  },
  {
    "scope": "value[type=IVL_TS]",
    "sourcePath": "timeRange",
    "targetPath": "valuePeriod",
    "transform": "cda_timerange_to_period"
  },
  {
    "sourcePath": "interpretationCode",
    "targetPath": "interpretation[0]",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "literalValue": "unknown",
    "targetPath": "dataAbsentReason",
    "transform": "observation_data_absent_reason_to_fhir",
    "skipIfResourceHasAnyOf": [
      "valueQuantity",
      "valueCodeableConcept",
      "valueString",
      "valueBoolean",
      "valueInteger",
      "valuePeriod"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.86]",
    "collectAll": true,
    "fields": [
      {
        "sourcePath": "code",
        "targetPath": "code",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "sourcePath": "text",
        "targetPath": "code.text"
      },
      {
        "sourcePath": "statusCode",
        "targetPath": "status",
        "transform": "observation_status_to_fhir"
      },
      {
        "literalValue": "cognitive-status",
        "targetPath": "category[0]",
        "transform": "observation_category_to_fhir"
      },
      {
        "fields": [
          {
            "scope": "authors[0].assignedAuthor",
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
            "scope": "authors[0].assignedAuthor.representedOrganization",
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
            "sourcePath": "authors[0].assignedAuthor.code",
            "targetPath": "specialty[0]",
            "transform": "cda_code_to_codeable_concept"
          },
          {
            "scope": "authors[0].assignedAuthor.telecoms[*]",
            "collectAll": true,
            "targetPath": "telecom",
            "transform": "cda_telecom_to_fhir"
          }
        ],
        "emitAsResource": "PractitionerRole",
        "emitAsResourceRequiredPaths": [
          "organization"
        ],
        "targetPath": "performer[0]"
      },
      {
        "scope": "authors[0].assignedAuthor",
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
        "targetPath": "performer[0]",
        "skipIfResourceHasAnyOf": [
          "performer"
        ]
      },
      {
        "scope": "value[type=PQ]",
        "sourcePath": "quantity",
        "targetPath": "valueQuantity",
        "transform": "cda_quantity_to_fhir"
      },
      {
        "scope": "value[type=CD]",
        "scopeFallbacks": [
          "value[type=CE]",
          "value[type=CS]"
        ],
        "sourcePath": "code",
        "targetPath": "valueCodeableConcept",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "value[type=ST]",
        "scopeFallbacks": [
          "value[type=ED]"
        ],
        "sourcePath": "text",
        "targetPath": "valueString"
      },
      {
        "scope": "value[type=BL]",
        "sourcePath": "boolean",
        "targetPath": "valueBoolean"
      },
      {
        "scope": "value[type=INT]",
        "sourcePath": "integer",
        "targetPath": "valueInteger"
      },
      {
        "scope": "value[type=REAL]",
        "sourcePath": "real",
        "targetPath": "valueQuantity",
        "transform": "cda_real_to_bare_quantity"
      },
      {
        "scope": "value[type=IVL_TS]",
        "sourcePath": "timeRange",
        "targetPath": "valuePeriod",
        "transform": "cda_timerange_to_period"
      }
    ],
    "emitAsResource": "Observation",
    "emitAsResourcePatientRefPath": [
      "subject"
    ],
    "targetPath": "hasMember"
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
