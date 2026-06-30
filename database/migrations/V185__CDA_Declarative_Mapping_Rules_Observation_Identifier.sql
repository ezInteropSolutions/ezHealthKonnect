-- V185__CDA_Declarative_Mapping_Rules_Observation_Identifier.sql
--
-- Adds Observation.identifier row to all 6 observationRule()-based sections
-- (vitalSigns, results, labResults, functionalStatus, mentalStatus, socialHistory).
-- Confirmed present in 3/4 corpus files' Results sections: Ascension (47
-- observations with real <id>), Epic 99397 (2), Marshfield (1); Dignity
-- Health's Results section is genuinely empty. Same CollectAll+Scope idiom
-- as MedicationMappingRules' identifier row (line ~491). Supersedes V184
-- for all 6 sections' Fields content.

-- =========================================================
-- vitalSigns -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'vitalSigns', 'Observation', '', 0,
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
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- results -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'results', 'Observation', '', 0,
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
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- labResults -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'labResults', 'Observation', '', 0,
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
    true, true, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource, header_path) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, skip_if_code_null_flavor = EXCLUDED.skip_if_code_null_flavor, updated_at = NOW();

-- =========================================================
-- socialHistory -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'socialHistory', 'Observation', '', 0,
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

-- =========================================================
-- functionalStatus -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'functionalStatus', 'Observation', '', 0,
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

-- =========================================================
-- mentalStatus -> Observation
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, skip_if_code_null_flavor, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'mentalStatus', 'Observation', '', 0,
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

