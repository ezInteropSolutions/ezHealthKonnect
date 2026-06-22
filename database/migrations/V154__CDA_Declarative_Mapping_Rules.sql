-- V154__CDA_Declarative_Mapping_Rules.sql
-- Phase 3 (OOB Template Migration) of the CDA→FHIR Declarative Mapping Engine.
--
-- This is a NEW table for the NEW MappingRule/MappingRow shape introduced in
-- Phase 2 (services/cda_fhir/declarative_schema.go) — deliberately NOT a
-- patch to V149__CDA_FHIR_Schema.sql's cda_fhir_templates table, which seeds
-- the OLD flat {cdaField, fhirPath, transform, valueMap} row shape that
-- Phase 0's inventory found can't express most of what the Go mappers
-- actually do (see architecture/CDA_FHIR_MAPPING_INVENTORY.md's
-- cross-cutting finding #1). cda_fhir_templates is left exactly as-is; it
-- keeps serving the dormant generic_mapper.go path until Phase 4 deletes
-- both the dormant engine and (eventually) this seed's predecessor together.
--
-- Seeds 5 OOB rule rows for the three sections this Phase 3 session ported
-- (see architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md, Phase 3):
--   allergiesAndIntolerances -> AllergyIntolerance            (1 rule)
--   medications              -> MedicationRequest/Statement   (2 rules, moodCode dispatch)
--   problems                 -> Condition (category=problem-list-item)
--   healthConcerns           -> Condition (category=health-concern)
--
-- The JSON in each "fields" column is hand-synced to match
-- services/cda_fhir/declarative_oob_rules.go's Go literals EXACTLY.
-- declarative_oob_rules_test.go's round-trip test reads this file back,
-- extracts each dollar-quoted JSON block, unmarshals it into []MappingRow,
-- and deep-compares against that Go file's output -- any drift between the
-- two fails a test instead of silently shipping, since this migration is
-- not (yet) read by any running code path (Phase 4's cutover is what wires
-- a repository to this table; Phase 3's job is proving the rules are
-- correct and seedable, not switching production over to reading them).

-- =========================================================
-- TABLE: cda_declarative_mapping_rules
-- =========================================================

CREATE TABLE IF NOT EXISTS cda_declarative_mapping_rules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_type    VARCHAR(100)  NOT NULL DEFAULT 'CCD',
    ccda_version     VARCHAR(20)   NOT NULL DEFAULT '2.1',
    fhir_version     VARCHAR(10)   NOT NULL DEFAULT 'R4',
    section_key      VARCHAR(100)  NOT NULL,   -- "allergiesAndIntolerances", "medications", …
    fhir_resource    VARCHAR(100)  NOT NULL,   -- "AllergyIntolerance", "MedicationRequest", …
    entry_match      VARCHAR(200)  NOT NULL DEFAULT '', -- Phase 1 predicate clause, no brackets; "" = every entry
    rule_order       INTEGER       NOT NULL DEFAULT 0,  -- evaluation order within (section_key) for BuildResourcesForRules' first-match-wins dispatch
    fields           JSONB         NOT NULL,   -- []MappingRow — see declarative_schema.go
    is_system        BOOLEAN       NOT NULL DEFAULT true,
    is_public        BOOLEAN       NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (document_type, ccda_version, fhir_version, section_key, fhir_resource)
);

CREATE INDEX IF NOT EXISTS idx_cda_declarative_mapping_rules_section
    ON cda_declarative_mapping_rules (document_type, ccda_version, fhir_version, section_key, rule_order);

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_cda_declarative_mapping_rules_updated_at'
    ) THEN
        CREATE TRIGGER trg_cda_declarative_mapping_rules_updated_at
            BEFORE UPDATE ON cda_declarative_mapping_rules
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END; $$;

-- =========================================================
-- SEED: allergiesAndIntolerances -> AllergyIntolerance
-- Ports allergy_mapper.go's buildAllergyResource. See
-- services/cda_fhir/declarative_oob_rules.go:AllergyMappingRules for the
-- per-row citations to the Go line numbers this transcribes.
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'allergiesAndIntolerances', 'AllergyIntolerance', '', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "allergy_status_to_fhir",
    "targetPath": "clinicalStatus",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "onsetDateTime"
  },
  {
    "literalValue": "confirmed",
    "transform": "allergy_verification_status_to_fhir",
    "targetPath": "verificationStatus",
    "condition": {
      "whenPath": "negationInd",
      "whenPaths": ["entryRelationships[typeCode=SUBJ].entry.negationInd"],
      "equals": "true",
      "thenLiteralValue": "refuted"
    }
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [""],
    "sourcePath": "value.code.code",
    "transform": "allergy_type_to_fhir",
    "targetPath": "type"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry",
    "scopeFallbacks": [""],
    "sourcePath": "participants[typeCode=CSM].participantRole.playingEntity.code",
    "fallbackPaths": ["value.code"],
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "code",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM]",
    "scopeFallbacks": ["participants[typeCode=CSM]"],
    "sourcePath": "participantRole.playingEntity.code.codeSystem",
    "transform": "allergy_category_from_substance_system",
    "targetPath": "category"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ].entry.entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.9]",
    "scopeFallbacks": [
      "entryRelationships[typeCode=MFST,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.9]"
    ],
    "collectAll": true,
    "targetPath": "reaction",
    "fields": [
      {
        "transform": "cda_value_or_code_to_codeable_concept",
        "targetPath": "manifestation[0]"
      },
      {
        "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.8]",
        "sourcePath": "value.code.code",
        "transform": "allergy_reaction_severity_to_fhir",
        "targetPath": "severity"
      }
    ]
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- =========================================================
-- SEED: medications -> MedicationRequest (moodCode=INT) and
-- MedicationStatement (everything else) — moodCode-driven first-match-wins
-- dispatch via rule_order (0 before 1), exactly the order
-- declarative_oob_rules.go:MedicationMappingRules returns.
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationRequest', 'moodCode=INT', 0,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "medication_request_status_to_fhir",
    "targetPath": "status",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "performers[0].assignedEntity.assignedPerson.names[0]",
    "fallbackPaths": ["authors[0].assignedAuthor.assignedPerson.names[0]"],
    "literalValue": "Ordering Provider",
    "transform": "cda_name_or_literal_to_display_ref",
    "targetPath": "requester",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "authoredOn"
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "medicationCodeableConcept"
  },
  {
    "sourcePath": "routeCode",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "dosageInstruction[0].route"
  },
  {
    "sourcePath": "doseQuantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dosageInstruction[0].doseAndRate[0].doseQuantity"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosageInstruction[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosageInstruction[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosageInstruction[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosageInstruction[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'medications', 'MedicationStatement', '', 1,
    $rules$
[
  {
    "sourcePath": "statusCode",
    "transform": "medication_status_to_fhir",
    "targetPath": "status",
    "required": true,
    "conformance": "SHALL"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_period",
    "targetPath": "effectivePeriod"
  },
  {
    "sourcePath": "effectiveTime",
    "transform": "cda_timerange_to_onset",
    "targetPath": "effectiveDateTime",
    "skipIfResourceHasAnyOf": ["effectivePeriod"]
  },
  {
    "sourcePath": "consumable.manufacturedProduct.manufacturedMaterial.code",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "medicationCodeableConcept"
  },
  {
    "sourcePath": "routeCode",
    "transform": "cda_code_to_codeable_concept",
    "targetPath": "dosage[0].route"
  },
  {
    "sourcePath": "doseQuantity",
    "transform": "cda_quantity_to_fhir",
    "targetPath": "dosage[0].doseAndRate[0].doseQuantity"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "literalValue": 1,
    "targetPath": "dosage[0].timing.repeat.frequency"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "value",
    "targetPath": "dosage[0].timing.repeat.period"
  },
  {
    "scope": "effectiveTimes[xsiType=PIVL_TS].period",
    "sourcePath": "unit",
    "targetPath": "dosage[0].timing.repeat.periodUnit"
  },
  {
    "scope": "entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147]",
    "sourcePath": "text",
    "targetPath": "dosage[0].text"
  },
  {
    "scope": "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20]",
    "sourcePath": "text",
    "targetPath": "dosage[0].patientInstruction"
  },
  {
    "scope": "entryRelationships[typeCode=RSON].entry",
    "transform": "cda_value_or_code_to_codeable_concept",
    "collectAll": true,
    "targetPath": "reasonCode"
  }
]
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();

-- =========================================================
-- SEED: problems / healthConcerns -> Condition
-- Two separate rules (not one rule with a runtime category parameter) —
-- see declarative_oob_rules.go:HealthConcernsMappingRules's doc comment for
-- why: each rule's category row carries its own fixed literalValue, so
-- there is no parameter to mismatch the way condition_mapper.go's
-- pre-Phase-0-fix hardcoded category was found to.
-- =========================================================

INSERT INTO cda_declarative_mapping_rules
    (document_type, ccda_version, fhir_version, section_key, fhir_resource, entry_match, rule_order, fields, is_system, is_public)
VALUES
(
    'CCD', '2.1', 'R4', 'problems', 'Condition', '', 0,
    $rules$
[
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
    "literalValue": "problem-list-item",
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
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
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
    "literalValue": "health-concern",
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
    $rules$::jsonb,
    true, true
)
ON CONFLICT (document_type, ccda_version, fhir_version, section_key, fhir_resource) DO UPDATE SET
    entry_match = EXCLUDED.entry_match, rule_order = EXCLUDED.rule_order, fields = EXCLUDED.fields, updated_at = NOW();
