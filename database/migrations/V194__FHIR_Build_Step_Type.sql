-- V194: Register the fhir.build pipeline step type in transformation_templates.
-- The no-code, format-agnostic on-ramp for a single FHIR R4 resource: maps
-- CSV columns, DB query columns, or arbitrary JSON fields directly onto a
-- FHIR resource's own element paths -- the FHIR-side mirror of V193's
-- cda.map_to_canonical, except a FHIR resource already IS the JSON output
-- shape, so there is no separate "build" step for it.

INSERT INTO transformation_templates
    (template_name, template_type, description, default_config, is_system, is_public)
SELECT
    'FHIR Resource Builder',
    'fhir.build',
    'Maps CSV columns, DB query columns, or arbitrary JSON fields directly '
    'onto a single FHIR R4 resource''s element paths, using no-code field '
    'mappings (source path -> target element, with optional value '
    'translation and named transforms) instead of a script. Add one step '
    'per resource type, then feed each output into a payload.builder '
    '(fhir_bundle mode) step to assemble a Bundle from any non-CDA source.',
    '{"resourceType":"Patient","profile":"base","version":"R4","outputField":"fhirResource","fields":[],"repeatingGroups":[]}'::jsonb,
    true,
    true
WHERE NOT EXISTS (
    SELECT 1 FROM transformation_templates WHERE template_type = 'fhir.build'
);
