-- V64__Validation_Queue_And_Referencing_Mode.sql
-- Adds referencing_mode column to hl7_fhir_templates,
-- creates fhir_validation_queue table, and applies onMissing
-- validation policies to critical fields in ADT templates.

BEGIN;

-- ============================================================
-- 2a. Add referencing_mode column to hl7_fhir_templates
-- ============================================================
-- Controls whether secondary resources are placed as contained
-- resources inside their parent or as separate bundle entries.
--
-- "contained"    - strip from bundle entries, embed under parent.contained[]
--                  with id="#originalId"; update references to "#id"
-- "bundle_entry" - keep as independent bundle entries (default behaviour)
--
-- Parents: Practitioner → Encounter, Location → Encounter,
--          Specimen → DiagnosticReport or Observation

ALTER TABLE hl7_fhir_templates
    ADD COLUMN IF NOT EXISTS referencing_mode JSONB
        DEFAULT '{
            "Practitioner":   "contained",
            "Organization":   "bundle_entry",
            "Location":       "contained",
            "Specimen":       "bundle_entry"
        }'::jsonb;

-- ============================================================
-- 2b. Create fhir_validation_queue table
-- ============================================================
-- Holds messages that failed required-field validation with
-- onMissing = "queue_review". Operators review, resolve, and
-- resubmit or reject each queued item via the UI.
--
-- status values:
--   pending_review      - arrived, not yet claimed
--   in_review           - claimed by a reviewer (assigned_to)
--   resolved_override   - reviewer approved with override values
--   resolved_rejected   - reviewer rejected (message will not deliver)
--   resolved_provided   - reviewer supplied missing values; requeued

CREATE TABLE IF NOT EXISTS fhir_validation_queue (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id     UUID         NOT NULL,
    message_id       VARCHAR(255),
    message_type     VARCHAR(50),
    raw_hl7          TEXT,
    partial_fhir     JSONB,

    -- Array of failure objects, e.g.:
    -- [{"field":"Patient.identifier","hl7Path":"PID.3.1",
    --   "missingCode":"MISSING_MRN","severity":"error"}]
    validation_failures JSONB     NOT NULL DEFAULT '[]'::jsonb,

    -- Lifecycle status (see values above)
    status           VARCHAR(30)  NOT NULL DEFAULT 'pending_review',

    -- Resolution payload, e.g.:
    -- {"action":"provide_value",
    --  "overrides":{"PID.3.1":"MRN12345"},
    --  "resolved_by":"user@example.com",
    --  "resolved_at":"2026-03-16T12:00:00Z"}
    resolution       JSONB,

    assigned_to      VARCHAR(255),
    notes            TEXT,

    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fhir_vq_interface
    ON fhir_validation_queue(interface_id);

CREATE INDEX IF NOT EXISTS idx_fhir_vq_status
    ON fhir_validation_queue(status);

CREATE INDEX IF NOT EXISTS idx_fhir_vq_created
    ON fhir_validation_queue(created_at DESC);

-- Auto-update updated_at on row changes
-- (reuse the generic trigger function if it already exists in the schema;
--  otherwise create a minimal one here)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at'
    ) THEN
        CREATE FUNCTION set_updated_at()
        RETURNS TRIGGER LANGUAGE plpgsql AS $fn$
        BEGIN
            NEW.updated_at = NOW();
            RETURN NEW;
        END;
        $fn$;
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_fhir_vq_updated_at ON fhir_validation_queue;
CREATE TRIGGER trg_fhir_vq_updated_at
    BEFORE UPDATE ON fhir_validation_queue
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 2c. onMissing policy documentation and ADT template updates
-- ============================================================
--
-- onMissing values supported in mapping entries:
--   "proceed"      - log warning, continue transformation (default)
--   "warn"         - add OperationOutcome issue to bundle, continue
--   "queue_review" - hold message in fhir_validation_queue, do not deliver
--   "reject"       - send NACK / HTTP 422, route to DLQ
--   "default"      - substitute the mapping entry's defaultValue field
--
-- validationPolicy object on a resource section:
--   onMissingRequired  - policy applied when any required mapping has no HL7 value
--   criticalFields     - list of FHIR paths whose absence triggers onMissingRequired
--
-- Example mapping entry with onMissing:
--   {"hl7Path":"PID.3.1","fhirPath":"Patient.identifier[0].value",
--    "required":true,"onMissing":"queue_review","defaultValue":"UNKNOWN"}

-- Apply validationPolicy to ALL OOB templates with safe default: "proceed"
-- Operators opt in to stricter policies (warn/reject/queue_review) per interface
-- via the fhir_validation_policy column on the interfaces table (see V66).
UPDATE hl7_fhir_templates
SET
    template_config = jsonb_set(
        template_config,
        '{resources,Patient,validationPolicy}',
        '{
            "onMissingRequired": "proceed",
            "criticalFields": [
                "Patient.identifier[0].value",
                "Patient.name[0].family"
            ]
        }'::jsonb,
        true
    ),
    updated_at = NOW()
WHERE template_name LIKE 'OOB_ADT_%'
   OR template_name LIKE 'OOB_ORU_%'
   OR template_name LIKE 'OOB_ORM_%'
   OR template_name LIKE 'OOB_SIU_%'
   OR template_name LIKE 'OOB_MDM_%';

COMMIT;
