-- V179__CDA_Declarative_Mapping_Rules_FunctionalStatus_AssessmentScale_COMP.sql
--
-- Assessment Scale Observation COMP-children fan-out for
-- FunctionalStatusMappingRules(): found auditing the Functional Status
-- section of the 99397 CCD sample. Two of its 14 entries ("Alcohol Use"
-- AUDIT-C and a PHQ-2 depression screen) are Assessment Scale Observations
-- (templateId .4.69) sitting DIRECTLY as top-level section entries -- unlike
-- Social History, where this same template nests one level deeper under a
-- SPRT-linked shell that always carries a real code (see V177). Both of
-- this section's .4.69 entries carry code.nullFlavor="UNK" (non-conformant
-- per the IG's own AssessmentScaleObservation StructureDefinition, which
-- requires code+value both [1..1]).
--
-- Before this fix, skip_if_code_null_flavor correctly discarded these
-- code-less shells, but the engine's OLD short-circuit (checked BEFORE any
-- Fields row ran) ALSO silently discarded their COMP-nested Assessment
-- Scale Supporting Observation children (templateId .4.86, which DOES
-- mandate code [1..1] LOINC + value [1..*]) along with them: 3 AUDIT-C
-- question/answer pairs and 2-3 PHQ-2 question/answer pairs (8 real
-- Observations total) were lost entirely -- confirmed against the parsed
-- FHIR output, which had zero trace of any of them. Fixing this required
-- moving the skip_if_code_null_flavor check in declarative_engine.go's
-- buildOneResource to AFTER the Fields loop runs (so an EmitAsResource
-- child a Field already wrote survives even though the entry's OWN main
-- resource gets discarded) -- a behavior change shared by every
-- skip_if_code_null_flavor rule, not just this one, but a no-op for the
-- other 5 (vitalSigns/results/labResults/mentalStatus/socialHistory: none
-- of their null-coded shells in the current corpus have COMP children to
-- lose).
--
-- The new Field reuses assessmentScaleSupportingObservationRow (the SAME
-- CollectAll+EmitAsResource primitive Social History's analogous .4.86
-- children already use via V177, parameterized here with
-- category="functional-status" instead of "social-history") appended
-- directly onto this rule's own Fields, not nested under a SPRT row,
-- because there is no SPRT-linked shell here to nest it under -- the .4.69
-- entry already IS the matched root entry.
--
-- Verified against the IG before writing this fix, not guessed: GitHub
-- HL7/CDA-ccda's StructureDefinition-AssessmentScaleObservation.xml and
-- StructureDefinition-AssessmentScaleSupportingObservation.xml (fetched
-- 2026-06-28), confirming COMP (not SPRT) attaches the Supporting
-- Observation, and that Observation.code is SHALL [1..1] LOINC on the
-- Supporting Observation specifically.
--
-- Deliberately NOT fixed in this same pass (a narrow, documented gap, not a
-- silently-dropped one): the PHQ-2 total-score Supporting Observation
-- (LOINC 55758-7) in the 99397 sample carries TWO sibling <value> elements
-- (an INT raw score AND a CO SNOMED interpretation) -- spec-permitted, but
-- cda/document's CDAEntry.Value is single-valued and FHIR R4's
-- Observation.value[x] is itself single-valued, so there is no existing
-- target for a second value without a new design decision this session
-- didn't get explicit sign-off for. See FunctionalStatusMappingRules' own
-- doc comment in declarative_oob_rules.go for the full detail this SQL is
-- hand-synced to; declarative_oob_rules_migration_v179_test.go is the
-- drift guard.
--
-- Supersedes V178 for functionalStatus/Observation's Fields content ONLY --
-- vitalSigns/results/labResults/mentalStatus/socialHistory (the other 5
-- rules sharing observationRule()) are UNCHANGED and remain owned by V178.

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'functionalStatus', 'Observation', '', 0,
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
    "literalValue": "functional-status",
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
        "literalValue": "functional-status",
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
