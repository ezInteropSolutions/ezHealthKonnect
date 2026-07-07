-- V191: Add cda_dedupe_registry table for cross-message CDA deduplication
-- Applied: 2026-07-07

-- ============================================================
-- CDA DEDUPE REGISTRY
-- ============================================================
-- Backs the cda.dedupe pipeline step's "crossMessage" mode: within-document
-- dedup (the step's original behavior) has no memory between messages, so
-- the same allergy/problem/medication restated in every CCD received for a
-- patient over time is never caught. This table is the persistent identity
-- registry that makes that possible — one row per (interface, patient,
-- section, identity key) combination ever seen. A NEW combination is
-- inserted and the entry is kept; an ALREADY-SEEN combination means the
-- entry is a duplicate of one from an earlier message and is dropped.
--
-- patient_key is NOT a foreign key to any patient table — this codebase has
-- no master-patient-index. It is the raw <id> extension value from the CDA
-- document's own header, for whichever identifier root the interface is
-- configured to treat as "the patient identifier" (cda.dedupe's
-- patientIdentifierRoot config) — scoped per interface so two different
-- source systems' overlapping identifier schemes never cross-match.

CREATE TABLE IF NOT EXISTS cda_dedupe_registry (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id           UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    patient_key            VARCHAR(255) NOT NULL,
    section_key            VARCHAR(100) NOT NULL,
    identity_key           TEXT NOT NULL,
    first_seen_message_id  VARCHAR(255),
    last_seen_message_id   VARCHAR(255),
    first_seen_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    seen_count             INTEGER NOT NULL DEFAULT 1,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cda_dedupe_registry_identity
        UNIQUE (interface_id, patient_key, section_key, identity_key)
);

-- Lookup index — every check this table serves is scoped to
-- (interface_id, patient_key, section_key), narrowing to one identity_key
-- comparison per candidate entry. The UNIQUE constraint above already
-- provides an index with this exact column prefix, so no separate index
-- is needed for the lookup path itself.

CREATE INDEX IF NOT EXISTS idx_cda_dedupe_registry_patient
    ON cda_dedupe_registry(interface_id, patient_key);

COMMENT ON TABLE cda_dedupe_registry IS
    'Persistent identity registry backing cda.dedupe step''s crossMessage mode — one row per (interface, patient, section, identity key) combination ever seen, so duplicate clinical entries restated across separate CDA/CCD documents for the same patient can be recognized, not just duplicates within a single document.';
COMMENT ON COLUMN cda_dedupe_registry.patient_key IS
    'Raw CDA <id> extension value for the identifier root configured as patientIdentifierRoot on the cda.dedupe step — not a foreign key, no master-patient-index exists in this system.';
COMMENT ON COLUMN cda_dedupe_registry.identity_key IS
    'Composite identity key string, same format/derivation as cda.dedupe''s within-document matching (OOB rule or override keyPaths, pipe-joined path=value pairs).';
