-- V209__CDA_Declarative_Mapping_Rules_OperativeNote_ProcedureNote_CarePlan.sql
--
-- Seeds 9 sections (Care Plan, Operative Note, Procedure Note) that already
-- parsed and Coverage-Audited correctly but produced no FHIR resource --
-- the same "Coverage-Audit-visible-but-unmapped" gap header.legalAuthenticator
-- had before V162 gave it its own rule.
--
-- See services/cda_fhir/declarative_oob_rules.go for the single source of
-- truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v209_test.go for the drift guard.
--
-- Target resource summary (see declarative_oob_rules.go's own doc comments
-- for the full reasoning behind each choice):
--   healthStatusEvaluationsOutcomes -> Observation (observationRule reuse,
--                                        entry_match="entryType=observation")
--   complications/procedureFindings  -> Condition (directProblemObsFields --
--                                        NOT conditionRule/conditionFields;
--                                        see below)
--   preoperativeDiagnosis/postprocedureDiagnosis
--                                     -> Condition (conditionRule reuse,
--                                        category "encounter-diagnosis" --
--                                        genuinely act-wrapped, so
--                                        conditionFields' problemObsScope is
--                                        correct here)
--   procedureIndications             -> Condition (new, small field set --
--                                        Indication (V2) .4.19, NOT the
--                                        Problem Observation shape)
--   plannedProcedure                 -> ServiceRequest (serviceRequestFields,
--                                        extracted from planOfCareRulesForSectionKey
--                                        -- moodCode=RQO, requested/not-yet-
--                                        performed)
--   anesthesia/medicationsAdministered -> MedicationAdministration (new
--                                        medicationAdministrationFields --
--                                        user-confirmed: medication already
--                                        given during a procedure, not an
--                                        order or a patient-reported
--                                        statement)
--
-- REVISED after this migration's first version (same day, same session):
-- validated the 9 new rules against 3 real HL7-published sample documents
-- (C-CDA-Examples repo's Operative_Note.xml/Procedure_Note.xml/Care_Plan.xml)
-- and found two real bugs, both fixed here:
--   1. healthStatusEvaluationsOutcomes originally had no entry_match, so a
--      non-observation <act> "External Document Reference" entry the real
--      Care Plan sample carries alongside the genuine Outcome Observation
--      was ALSO claimed and forced through the Observation field list,
--      producing a near-empty junk resource. Fixed with
--      entry_match="entryType=observation".
--   2. complications/procedureFindings originally reused conditionRule's
--      fields verbatim (problemObsScope, an ACT-relative "hop into my one
--      real nested Problem Observation" path) -- WRONG when the matched
--      entry is already the Problem Observation itself, which is what these
--      two sections' entry/observation shape always is. The real Operative
--      Note sample's "complications" entry (Pneumonia) carries its own
--      genuine Age At Onset sub-observation via a SUBJ-typed
--      entryRelationship (inversionInd="true") -- problemObsScope's
--      unfiltered SUBJ grab resolved to THAT instead of falling through to
--      self, so Condition.code came out as a bare Quantity ({code:"a",
--      unit:"a",value:57}) instead of Pneumonia's own CodeableConcept --
--      invalid FHIR, not just a wrong value. Fixed with a dedicated
--      directProblemObsFields() field set that resolves relative to the
--      matched entry directly (no problemObsScope hop at all) and adds a
--      proper one-hop onsetAge extraction that was previously missing
--      entirely for this shape (Age At Onset data was silently dropped, not
--      just misrouted).
-- preoperativeDiagnosis/postprocedureDiagnosis/procedureIndications/
-- plannedProcedure/anesthesia/medicationsAdministered were all re-verified
-- against the same 3 real documents and are unaffected -- unchanged below.

-- =========================================================
-- SEED: healthStatusEvaluationsOutcomes -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'healthStatusEvaluationsOutcomes', 'Observation', 'entryType=observation', 0,
    $rules$
[
  {
    "scope": "id[*]",
    "collectAll": true,
    "targetPath": "identifier",
    "transform": "cda_ii_to_identifier"
  },
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
    "literalValue": "survey",
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
    "sourcePath": "referenceRangeText",
    "targetPath": "referenceRange[0].text"
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
  }
]
    $rules$::jsonb,
    true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: complications -> Condition
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'complications', 'Condition', '', 0,
    $rules$
[
  {
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
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
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry[code=SEV]",
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
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
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
    "targetPath": "recorder"
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
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": [
      "recorder"
    ]
  },
  {
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
        "scope": "performers[0].assignedEntity",
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
        "scope": "performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "asserter"
  },
  {
    "scope": "performers[0].assignedEntity",
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
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": [
      "asserter"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "sourcePath": "value",
    "targetPath": "onsetAge",
    "transform": "cda_value_to_fhir"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: preoperativeDiagnosis -> Condition
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'preoperativeDiagnosis', 'Condition', '', 0,
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
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
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
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "asserter"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
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
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": [
      "asserter"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]"
    ],
    "sourcePath": "value",
    "targetPath": "onsetAge",
    "transform": "cda_value_to_fhir"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: postprocedureDiagnosis -> Condition
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'postprocedureDiagnosis', 'Condition', '', 0,
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
  },
  {
    "fields": [
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
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
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "asserter"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.performers[0].assignedEntity",
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
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": [
      "asserter"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=REFR].entry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]"
    ],
    "sourcePath": "value",
    "targetPath": "onsetAge",
    "transform": "cda_value_to_fhir"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: procedureFindings -> Condition
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'procedureFindings', 'Condition', '', 0,
    $rules$
[
  {
    "targetPath": "code",
    "transform": "cda_value_or_code_to_codeable_concept",
    "required": true,
    "conformance": "SHALL"
  },
  {
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
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry[code=SEV]",
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
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "sourcePath": "effectiveTime.high",
    "targetPath": "abatementDateTime",
    "transform": "cda_time_to_fhir_datetime"
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
    "targetPath": "recorder"
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
    "targetPath": "recorder",
    "skipIfResourceHasAnyOf": [
      "recorder"
    ]
  },
  {
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
        "scope": "performers[0].assignedEntity",
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
        "scope": "performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "asserter"
  },
  {
    "scope": "performers[0].assignedEntity",
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
    "targetPath": "asserter",
    "skipIfResourceHasAnyOf": [
      "asserter"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[code=445518008]",
    "sourcePath": "value",
    "targetPath": "onsetAge",
    "transform": "cda_value_to_fhir"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: procedureIndications -> Condition
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'procedureIndications', 'Condition', '', 0,
    $rules$
[
  {
    "sourcePath": "value",
    "targetPath": "code",
    "transform": "cda_value_to_fhir",
    "conformance": "SHOULD"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "onsetDateTime",
    "transform": "cda_timerange_to_onset"
  },
  {
    "literalValue": "active",
    "targetPath": "clinicalStatus",
    "transform": "condition_status_to_fhir"
  },
  {
    "literalValue": "confirmed",
    "targetPath": "verificationStatus",
    "transform": "condition_verification_status_to_fhir"
  },
  {
    "literalValue": "encounter-diagnosis",
    "targetPath": "category[0]",
    "transform": "condition_category_to_fhir"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: plannedProcedure -> ServiceRequest
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'plannedProcedure', 'ServiceRequest', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "targetPath": "status",
    "transform": "service_request_status_to_fhir"
  },
  {
    "sourcePath": "moodCode",
    "targetPath": "intent",
    "transform": "service_request_intent_from_mood"
  },
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
    "literalValue": {
      "text": "Unknown"
    },
    "targetPath": "code",
    "skipIfResourceHasAnyOf": [
      "code"
    ]
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "occurrencePeriod",
    "transform": "cda_timerange_to_period"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "occurrenceDateTime",
    "transform": "cda_timerange_to_onset",
    "skipIfResourceHasAnyOf": [
      "occurrencePeriod"
    ]
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "targetPath": "reasonCode[0]",
    "transform": "cda_value_or_code_to_codeable_concept"
  },
  {
    "scope": "authors[0].assignedAuthor.assignedPerson.names[0]",
    "targetPath": "requester",
    "transform": "cda_name_or_literal_to_display_ref"
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: anesthesia -> MedicationAdministration
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'anesthesia', 'MedicationAdministration', '', 0,
    $rules$
[
  {
    "scope": "id[*]",
    "collectAll": true,
    "targetPath": "identifier",
    "transform": "cda_ii_to_identifier"
  },
  {
    "sourcePath": "statusCode",
    "targetPath": "status",
    "transform": "medication_administration_status_to_fhir",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "targetPath": "medicationCodeableConcept",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "effectivePeriod",
    "transform": "cda_timerange_to_period"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "effectiveDateTime",
    "transform": "cda_timerange_to_onset",
    "skipIfResourceHasAnyOf": [
      "effectivePeriod"
    ]
  },
  {
    "sourcePath": "routeCode",
    "targetPath": "dosage.route",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "doseQuantity",
    "targetPath": "dosage.dose",
    "transform": "cda_quantity_to_fhir"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosage.text"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "collectAll": true,
    "targetPath": "reasonCode",
    "transform": "cda_value_or_code_to_codeable_concept"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  },
  {
    "fields": [
      {
        "scope": "performers[0].assignedEntity",
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
        "scope": "performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "performer[0].actor"
  },
  {
    "scope": "performers[0].assignedEntity",
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
    "targetPath": "performer[0].actor",
    "skipIfResourceHasAnyOf": [
      "performer"
    ]
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: medicationsAdministered -> MedicationAdministration
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medicationsAdministered', 'MedicationAdministration', '', 0,
    $rules$
[
  {
    "scope": "id[*]",
    "collectAll": true,
    "targetPath": "identifier",
    "transform": "cda_ii_to_identifier"
  },
  {
    "sourcePath": "statusCode",
    "targetPath": "status",
    "transform": "medication_administration_status_to_fhir",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "targetPath": "medicationCodeableConcept",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "effectivePeriod",
    "transform": "cda_timerange_to_period"
  },
  {
    "sourcePath": "effectiveTime",
    "targetPath": "effectiveDateTime",
    "transform": "cda_timerange_to_onset",
    "skipIfResourceHasAnyOf": [
      "effectivePeriod"
    ]
  },
  {
    "sourcePath": "routeCode",
    "targetPath": "dosage.route",
    "transform": "cda_code_to_codeable_concept"
  },
  {
    "sourcePath": "doseQuantity",
    "targetPath": "dosage.dose",
    "transform": "cda_quantity_to_fhir"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosage.text"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "collectAll": true,
    "targetPath": "reasonCode",
    "transform": "cda_value_or_code_to_codeable_concept"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.64]",
    "sourcePath": "text",
    "targetPath": "note[0].text"
  },
  {
    "fields": [
      {
        "scope": "performers[0].assignedEntity",
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
        "scope": "performers[0].assignedEntity.representedOrganization",
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
        "sourcePath": "performers[0].assignedEntity.code",
        "targetPath": "specialty[0]",
        "transform": "cda_code_to_codeable_concept"
      },
      {
        "scope": "performers[0].assignedEntity.telecoms[*]",
        "collectAll": true,
        "targetPath": "telecom",
        "transform": "cda_telecom_to_fhir"
      }
    ],
    "emitAsResource": "PractitionerRole",
    "emitAsResourceRequiredPaths": [
      "organization"
    ],
    "targetPath": "performer[0].actor"
  },
  {
    "scope": "performers[0].assignedEntity",
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
    "targetPath": "performer[0].actor",
    "skipIfResourceHasAnyOf": [
      "performer"
    ]
  }
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
