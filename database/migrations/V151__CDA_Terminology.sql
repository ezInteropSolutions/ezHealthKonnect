-- V151__CDA_Terminology.sql
-- Sprint D: CDA terminology infrastructure.
--
-- Creates:
--   cda_code_systems       — registry of known healthcare code systems with validation metadata
--   cda_code_translations  — per-interface (and global) code translation lookup table
--
-- Mirrors the pattern of cda_fhir_templates: OOB system rows are seeded here;
-- interface-specific translations are inserted at runtime via the TerminologyService.

-- =========================================================
-- TABLE: cda_code_systems
-- =========================================================

CREATE TABLE IF NOT EXISTS cda_code_systems (
    id               SERIAL       PRIMARY KEY,
    system_uri       VARCHAR(255) NOT NULL UNIQUE,
    short_name       VARCHAR(50)  NOT NULL,
    oid              VARCHAR(100),
    display_name     VARCHAR(200),
    validation_mode  VARCHAR(20)  NOT NULL DEFAULT 'format',
    -- 'format'      → offline regex/pattern check (no external service)
    -- 'passthrough' → unknown system; all codes accepted as-is
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cda_code_systems_oid
    ON cda_code_systems (oid)
    WHERE oid IS NOT NULL;

-- =========================================================
-- SEED: 8 OOB code systems
-- =========================================================

INSERT INTO cda_code_systems (system_uri, short_name, oid, display_name, validation_mode) VALUES
-- SNOMED CT
('http://snomed.info/sct',
 'SNOMED',
 '2.16.840.1.113883.6.96',
 'SNOMED Clinical Terms',
 'format'),

-- RxNorm
('http://www.nlm.nih.gov/research/umls/rxnorm',
 'RxNorm',
 '2.16.840.1.113883.6.88',
 'RxNorm',
 'format'),

-- LOINC
('http://loinc.org',
 'LOINC',
 '2.16.840.1.113883.6.1',
 'Logical Observation Identifiers Names and Codes',
 'format'),

-- CVX (CDC Vaccine Codes)
('http://hl7.org/fhir/sid/cvx',
 'CVX',
 '2.16.840.1.113883.12.292',
 'CDC Vaccine Administered Codes',
 'format'),

-- NDC
('http://hl7.org/fhir/sid/ndc',
 'NDC',
 '2.16.840.1.113883.6.69',
 'National Drug Code',
 'format'),

-- ICD-10-CM
('http://hl7.org/fhir/sid/icd-10-cm',
 'ICD-10-CM',
 '2.16.840.1.113883.6.90',
 'ICD-10 Clinical Modification',
 'format'),

-- CPT
('http://www.ama-assn.org/go/cpt',
 'CPT',
 '2.16.840.1.113883.6.12',
 'Current Procedural Terminology',
 'format'),

-- NCI Thesaurus
('http://ncithesaurus.nci.nih.gov',
 'NCIT',
 '2.16.840.1.113883.3.26.1.1',
 'NCI Thesaurus',
 'format')

ON CONFLICT (system_uri) DO UPDATE SET
    short_name      = EXCLUDED.short_name,
    oid             = EXCLUDED.oid,
    display_name    = EXCLUDED.display_name,
    validation_mode = EXCLUDED.validation_mode;

-- =========================================================
-- TABLE: cda_code_translations
-- =========================================================

CREATE TABLE IF NOT EXISTS cda_code_translations (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id    UUID         REFERENCES interfaces(id) ON DELETE CASCADE,
    -- NULL interface_id = global translation (applies to all interfaces)
    source_system   VARCHAR(255) NOT NULL,
    source_code     VARCHAR(100) NOT NULL,
    target_system   VARCHAR(255) NOT NULL,
    target_code     VARCHAR(100) NOT NULL,
    target_display  VARCHAR(500),
    created_by_user_id UUID      REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE (interface_id, source_system, source_code, target_system)
);

CREATE INDEX IF NOT EXISTS idx_cda_code_translations_lookup
    ON cda_code_translations (source_system, source_code, target_system);

CREATE INDEX IF NOT EXISTS idx_cda_code_translations_interface
    ON cda_code_translations (interface_id)
    WHERE interface_id IS NOT NULL;
