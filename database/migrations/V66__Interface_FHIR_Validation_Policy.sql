-- V66__Interface_FHIR_Validation_Policy.sql
-- Adds fhir_validation_policy to the interfaces table so each interface
-- can independently control what happens when required FHIR fields are missing.
--
-- Policy values (match onMissing in mapping templates):
--   "proceed"      - transform anyway, missing field just omitted  (DEFAULT)
--   "warn"         - transform, add OperationOutcome warning to bundle
--   "reject"       - fail the message, route to DLQ
--   "queue_review" - hold message for manual operator correction
--
-- The interface-level policy overrides the template-level onMissingRequired
-- for all required field mappings in that interface's transformation.
-- If not set (NULL), the template default ("proceed") applies.

BEGIN;

ALTER TABLE interfaces
    ADD COLUMN IF NOT EXISTS fhir_validation_policy VARCHAR(20)
        DEFAULT 'proceed'
        CHECK (fhir_validation_policy IN ('proceed', 'warn', 'reject', 'queue_review'));

COMMENT ON COLUMN interfaces.fhir_validation_policy IS
    'Controls what happens when a required FHIR field mapping has no source HL7 value. '
    'proceed=omit field and continue (default); warn=continue with OperationOutcome; '
    'reject=fail message to DLQ; queue_review=hold for manual correction.';

COMMIT;
