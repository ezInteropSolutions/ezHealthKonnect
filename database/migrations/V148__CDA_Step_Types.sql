-- V148: Register CDA pipeline step types in transformation_templates.
-- Adds three OOB system templates for the Sprint B CDA executors:
--   cda.to_fhir   — converts parsed CDA document to FHIR R4 Bundle
--   fhir.to_cda   — converts FHIR R4 Bundle to C-CDA 2.1 XML for legacy delivery
--   cda.normalize — upgrades C32/HITSP template OIDs to C-CDA 2.1 equivalents

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'CDA to FHIR R4',
    'cda.to_fhir',
    'Converts a parsed CDA/CCD document to a FHIR R4 Bundle. '
    'Reads the parsed CDA structure produced by the CDA parser and outputs '
    'a Bundle containing Patient, AllergyIntolerance, MedicationStatement, '
    'Condition, Observation, and Immunization resources.',
    '{}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'cda.to_fhir'
);

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'FHIR R4 to CDA',
    'fhir.to_cda',
    'Converts a FHIR R4 Bundle to a C-CDA 2.1 XML document for delivery to '
    'legacy systems. Produces Patient demographics header and four clinical '
    'sections: Allergies, Medications, Problems, and Immunizations.',
    '{"sourceField":"fhirBundle","outputField":"cdaXML","profile":"C-CDA 2.1"}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'fhir.to_cda'
);

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'CDA Normalizer (C32 to C-CDA 2.1)',
    'cda.normalize',
    'Upgrades C32 and HITSP CDA template OIDs to their C-CDA 2.1 equivalents. '
    'Use as a pre-parse step when ingesting legacy C32 or HITSP documents. '
    'Documents already in C-CDA 2.1 format pass through unchanged.',
    '{"sourceField":"raw","outputField":"raw"}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'cda.normalize'
);
