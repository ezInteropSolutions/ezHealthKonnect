-- V192: Register the cda.build pipeline step type in transformation_templates.
-- Format-agnostic successor to fhir.to_cda: builds a full C-CDA 2.1 document
-- covering all 17 of CCD's real sections (7 SHALL + 2 SHOULD + 8 MAY, per
-- the C-CDA 2.1 IG Table 30, 2018 errata) from canonical USCDI-keyed JSON
-- (the same shape cda.parse produces) or a raw FHIR Bundle
-- (inputFormat="fhir_bundle").

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'CDA/CCD Document Builder',
    'cda.build',
    'Builds a C-CDA 2.1 document from canonical USCDI-keyed JSON or a FHIR '
    'R4 Bundle, covering all 17 of CCD''s real sections per the C-CDA 2.1 '
    'IG (SHALL: Allergies, Medications, Problems, Results, Plan of '
    'Treatment, Social History, Vital Signs; SHOULD: Procedures, Payers; '
    'MAY: Advance Directives, Encounters, Family History, Functional '
    'Status, Immunizations, Medical Equipment, Mental Status, Nutrition) '
    'using the same CDA schema that drives cda.parse, so adding a new '
    'section is a schema change, not new code.',
    '{"sourceField":"parsedCDA","inputFormat":"canonical","outputField":"cdaXML","documentType":"CCD"}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'cda.build'
);
