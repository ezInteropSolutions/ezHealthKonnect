-- V193: Register the cda.map_to_canonical pipeline step type in transformation_templates.
-- The no-code, format-agnostic on-ramp for CCD construction: maps CSV
-- columns, DB query columns, or arbitrary JSON fields onto the same
-- canonical USCDI-keyed JSON shape cda.parse and cda.build already share
-- (see V192), so a CSV or DB source can feed cda.build with zero new Go
-- code -- only step configuration.

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'Map to Canonical CDA JSON',
    'cda.map_to_canonical',
    'Maps CSV columns, DB query columns, or arbitrary JSON fields onto the '
    'canonical USCDI-keyed JSON shape cda.build consumes, using no-code '
    'field mappings (source path -> canonical field key, with optional '
    'value translation) instead of a script. Pair with cda.build to '
    'construct a CCD from any non-FHIR, non-CDA source system.',
    '{"outputField":"parsedCDA","header":[],"sections":[]}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'cda.map_to_canonical'
);
