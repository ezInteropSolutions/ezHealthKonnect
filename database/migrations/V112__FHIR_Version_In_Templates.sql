-- V112: Add fhir_version column to hl7_fhir_templates
-- Enables separate OOB templates for FHIR R4 and R5.
-- Existing rows default to 'R4' (all current templates target R4).

BEGIN;

ALTER TABLE hl7_fhir_templates
    ADD COLUMN IF NOT EXISTS fhir_version VARCHAR(10) NOT NULL DEFAULT 'R4';

-- Update existing rows explicitly (belt-and-suspenders for rows inserted before DEFAULT was set)
UPDATE hl7_fhir_templates SET fhir_version = 'R4' WHERE fhir_version IS NULL OR fhir_version = '';

-- Drop the old unique constraint and replace it with one that includes fhir_version.
-- The old constraint name from V62 is unique_default_template_v2.
ALTER TABLE hl7_fhir_templates
    DROP CONSTRAINT IF EXISTS unique_default_template_v2;

ALTER TABLE hl7_fhir_templates
    DROP CONSTRAINT IF EXISTS unique_default_template;

ALTER TABLE hl7_fhir_templates
    ADD CONSTRAINT unique_default_template_v3
        UNIQUE (message_type, hl7_version, fhir_version, is_default);

COMMENT ON COLUMN hl7_fhir_templates.fhir_version IS
    'FHIR specification version this template targets (R4, R5). Separate OOB templates exist per version.';

COMMIT;
