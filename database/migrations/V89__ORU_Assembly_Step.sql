-- V89: (Superseded) ORU Assembly step was moved into the core.mapping step tab.
-- The hl7.assemble_observations step type is no longer needed as a separate pipeline step.
-- Assembly rules are now configured directly in the Assembly tab of the HL7→FHIR Transform step.
-- This migration is intentionally a no-op.
SELECT 1;
