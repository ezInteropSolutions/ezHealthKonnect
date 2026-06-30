-- V178__CDA_Declarative_Mapping_Rules_Observation_Performer_PractitionerRole.sql
--
-- Re-seeds all 6 observationRule()-based sections (vitalSigns, results,
-- labResults, functionalStatus, mentalStatus, socialHistory) to upgrade
-- Observation.performer from a bare display string to the same
-- PractitionerRole/Practitioner tiered pattern already applied to
-- Medication.requester, Condition.recorder, and Immunization.performer
-- earlier this session (see V170/V174/V176). socialHistory also upgrades
-- the nested Assessment Scale Supporting Observation's own performer row
-- (sdohAssessmentScaleSupportingObservationRow(), added in V177), for
-- consistency within that same section.
--
-- Real gap found auditing Vital Signs (99397 sample, LOINC 8716-3
-- section): every author there carries a representedOrganization with a
-- real name + full address + id ("mumbai Community Health and
-- Affiliates") and NO assignedPerson, but the old row
-- (cda_name_or_literal_to_display_ref) only ever produced a bare
-- {display: "..."} string, discarding the organization's
-- identifier/address data entirely. HL7's C-CDA on FHIR IG (CF-vitals.md)
-- maps /author to .performer and defers the resource shape to its shared
-- provenance guidance, which recommends "ideally...a PractitionerRole,
-- which can then support both Practitioner (name) and Organization" --
-- the same citation already used for the other three sections.
-- assignedEntityRoleRow degrades gracefully to an organization-only
-- PractitionerRole when no person exists (the common case here), via
-- buildEmittedSubResource's len<=1 gate. A second tier (barePractitionerRow,
-- gated on SkipIfResourceHasAnyOf=["performer"]) keeps a bare Practitioner
-- when there's a person name but no organization at all.
--
-- A companion fix (services/cda_fhir/assembly/rules/bp_panel_rule.go, not a
-- declarative rule and so not part of this migration) stops the BP panel
-- synthesis rule from silently dropping performer when it merges Systolic
-- + Diastolic into one 85354-9 panel Observation.
--
-- This row is shared by all 6 observationRule() callers, so all 6 drift
-- simultaneously from this one Go-side change -- same multi-section
-- re-seed shape as V169. See services/cda_fhir/declarative_oob_rules.go's
-- observationRule() and sdohAssessmentScaleSupportingObservationRow() for
-- the single source of truth this JSON is hand-synced to, and
-- declarative_oob_rules_migration_v178_test.go for the drift guard.

-- =========================================================
-- vitalSigns -> Observation (vital-signs category)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'vitalSigns', 'Observation', '', 0,
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
    "literalValue": "vital-signs",
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
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- results -> Observation (results category)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'results', 'Observation', '', 0,
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
    "literalValue": "laboratory",
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
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- labResults -> Observation (labResults category)
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'labResults', 'Observation', '', 0,
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
    "literalValue": "laboratory",
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
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- functionalStatus -> Observation (functionalStatus category)
-- =========================================================

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
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- mentalStatus -> Observation (mentalStatus category)
-- =========================================================

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
  }
]
    $rules$::jsonb,
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- socialHistory -> Observation (socialHistory category (with SDOH nesting))
-- =========================================================

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

