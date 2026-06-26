-- V157__CDA_Declarative_Mapping_Rules_Encounters_Procedures.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- Adds Encounters and Procedures on top of V154/V155/V156's
-- cda_declarative_mapping_rules table (same table, no schema change).
-- See services/cda_fhir/declarative_oob_rules.go's EncounterMappingRules()/
-- ProcedureMappingRules() for the single source of truth this JSON is
-- hand-synced to, and those functions' own top doc comments for what's
-- deliberately out of scope (document-header-level fields for Encounters;
-- the Procedure-Activity-template-uniform treatment matching Go's own).
--
-- Phase 4 Slice B update: building the document-level orchestrator
-- (DeclarativeMapDocument) and comparing it against MapDocument() on real
-- corpus data found a real divergence (kareo_sample.xml/
-- practicefusion_sample.xml both have a nullFlavor="NI" "no information"
-- encounter) -- encounter_mapper.go:37 calls EncounterStatusToFHIR
-- UNCONDITIONALLY, defaulting to "unknown" rather than omitting status, so
-- the status row below now carries fallbackPaths+literalValue to reach that
-- same default when statusCode is absent.

-- =========================================================
-- SEED: encounters -> Encounter
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
    "transform": "cda_ii_to_identifier"
  },
  {
    "scope": "performers[*]",
    "collectAll": true,
    "targetPath": "participant",
    "fields": [
      {"sourcePath": "typeCode", "transform": "encounter_participant_type_coding", "targetPath": "type[0]"},
      {
        "emitAsResource": "Practitioner",
        "targetPath": "individual",
        "fields": [
          {"scope": "assignedEntity.assignedPerson.names[*]", "collectAll": true, "transform": "cda_name_to_fhir", "targetPath": "name"},
          {"scope": "assignedEntity.ids[*]", "collectAll": true, "transform": "cda_ii_to_identifier", "embedCDAIdentity": true, "targetPath": "identifier"},
          {"sourcePath": "assignedEntity.code", "transform": "cda_code_to_codeable_concept", "targetPath": "qualification[0].code"},
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
          {"literalValue": "encounter-diagnosis", "transform": "condition_category_to_fhir", "targetPath": "category[0]"},
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
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();

-- =========================================================
-- SEED: procedures -> Procedure
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, flatten_organizers, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'procedures', 'Procedure', '', 0,
    $rules$
[
  {"sourcePath": "statusCode", "transform": "procedure_status_to_fhir", "targetPath": "status"},
  {"sourcePath": "code", "transform": "cda_code_to_codeable_concept", "targetPath": "code"},
  {"sourcePath": "effectiveTime", "transform": "cda_timerange_to_period", "targetPath": "performedPeriod"},
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "performedDateTime",
    "skipIfResourceHasAnyOf": ["performedPeriod"]
  },
  {
    "scope": "participants[typeCode=PRF|SPRF]",
    "collectAll": true,
    "targetPath": "performer",
    "fields": [
      {"scope": "participantRole.playingEntity.names[0]", "transform": "cda_name_or_literal_to_display_ref", "targetPath": "actor"}
    ]
  },
  {"sourcePath": "targetSiteCode", "transform": "cda_code_to_codeable_concept", "targetPath": "bodySite[0]"}
]
    $rules$::jsonb,
    false, true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields,
    flatten_organizers = EXCLUDED.flatten_organizers, updated_at = NOW();
