-- V177__CDA_Declarative_Mapping_Rules_SocialHistory_SDOH_AssessmentScale.sql
--
-- SDOH Assessment Scale fan-out for SocialHistoryMappingRules(): found
-- auditing the Social History section of a 99397 CCD sample, 14 of its 24
-- entries are SDOH screening instruments (2x HARK domestic-violence
-- questionnaire = 8 questions, Social Connection/Isolation panel = 6
-- questions) using C-CDA's spec-conformant 3-level nesting:
--
--   Social History Observation (shell, templateId .4.38, generic code
--   "8689-2 History of Social function", no value of its own)
--     -SPRT-> Assessment Scale Observation (templateId .4.69, e.g. "HARK
--             questionnaire", carries interpretationCode/risk level)
--       -COMP-> Assessment Scale Supporting Observation (templateId .4.86,
--               the actual question/answer pair + author) x N
--
-- Verified against the local CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1 spec PDF
-- (the base Social History Observation (V3) extension 2015-08-01 has NO
-- such child at all) plus the current hl7.org/cda/us/ccda pages for the
-- 2022-06-01 SDOH-extension version, which confirms SPRT (not COMP) attaches
-- the Assessment Scale Observation, and COMP attaches its own Supporting
-- Observations. Before this fix, observationRule()'s generic per-entry
-- mapping only ever read the OUTER shell, producing 14 identical,
-- clinically useless Observations (no value, no question, no answer).
--
-- Required a new engine primitive (applyCollectAllEmitAsResource in
-- declarative_engine.go) since CollectAll+EmitAsResource together (one
-- independent Observation resource PER COMP sibling, not a single
-- multi-field subObj) had no prior code path -- purely additive, confirmed
-- no pre-existing row combined them.
--
-- Each tier now emits as its own independent Observation resource, linked
-- via hasMember[] (shell.hasMember[0] -> Assessment Scale Observation;
-- Assessment Scale Observation.hasMember[] -> N Supporting Observations).
--
-- Supersedes V169 for socialHistory/Observation's Fields content ONLY --
-- vitalSigns/results/labResults/functionalStatus/mentalStatus (the other 5
-- rules sharing observationRule()) are UNCHANGED and remain owned by V169.
-- See declarative_oob_rules_migration_v177_test.go.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'socialHistory', 'Observation', '', 0,
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
    "literalValue": "social-history",
    "targetPath": "category[0]",
    "transform": "observation_category_to_fhir"
  },
  {
    "scope": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "scopeFallbacks": [
      "authors[0].assignedAuthor.representedOrganization.names[0]"
    ],
    "targetPath": "performer[0]",
    "transform": "cda_name_or_literal_to_display_ref"
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
    "scope": "entryRelationships[typeCode=SPRT].entry[templateId=2.16.840.1.113883.10.20.22.4.69]",
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
        "sourcePath": "effectiveTime",
        "targetPath": "effectiveDateTime",
        "transform": "cda_timerange_to_onset"
      },
      {
        "literalValue": "social-history",
        "targetPath": "category[0]",
        "transform": "observation_category_to_fhir"
      },
      {
        "sourcePath": "interpretationCode",
        "targetPath": "interpretation[0]",
        "transform": "cda_code_to_codeable_concept"
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
            "literalValue": "social-history",
            "targetPath": "category[0]",
            "transform": "observation_category_to_fhir"
          },
          {
            "scope": "authors[0].assignedAuthor.assignedPerson.names[0]",
            "scopeFallbacks": [
              "authors[0].assignedAuthor.representedOrganization.names[0]"
            ],
            "targetPath": "performer[0]",
            "transform": "cda_name_or_literal_to_display_ref"
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
    ],
    "emitAsResource": "Observation",
    "emitAsResourcePatientRefPath": [
      "subject"
    ],
    "targetPath": "hasMember[0]"
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();
