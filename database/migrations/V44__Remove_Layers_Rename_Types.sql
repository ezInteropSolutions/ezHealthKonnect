-- V44: Remove layer prefixes from step types, normalize layer column
-- Backward compatibility maintained via Go executor registry aliases

-- Rename step types in transformation_steps
UPDATE transformation_steps SET step_type = 'field_validation' WHERE step_type IN ('pre.validation', 'core.validation');
UPDATE transformation_steps SET step_type = 'if_then_else' WHERE step_type = 'pre.logic';
UPDATE transformation_steps SET step_type = 'switch_case' WHERE step_type = 'pre.logic.switch';
UPDATE transformation_steps SET step_type = 'enrichment.api' WHERE step_type = 'pre.enrichment.api';
UPDATE transformation_steps SET step_type = 'enrichment.database' WHERE step_type = 'pre.enrichment.database';
UPDATE transformation_steps SET step_type = 'enrichment.script' WHERE step_type = 'pre.enrichment.script';
UPDATE transformation_steps SET step_type = 'hl7_fhir_transform' WHERE step_type IN ('core.mapping', 'hl7_to_fhir_mapping');
UPDATE transformation_steps SET step_type = 'field_mapping' WHERE step_type = 'core.transformation';
UPDATE transformation_steps SET step_type = 'fhir_validation' WHERE step_type = 'post.validation';
UPDATE transformation_steps SET step_type = 'data_masking' WHERE step_type = 'post.data_masking';
UPDATE transformation_steps SET step_type = 'remove_duplicates' WHERE step_type = 'post.remove_duplicates';
UPDATE transformation_steps SET step_type = 'normalizer' WHERE step_type = 'post.normalizer';

-- Normalize all layers to 'core' (layer concept removed)
UPDATE transformation_steps SET layer = 'core' WHERE layer IN ('pre', 'post');

-- Same renames for templates table (column is template_type)
UPDATE transformation_templates SET template_type = 'field_validation' WHERE template_type IN ('pre.validation', 'core.validation');
UPDATE transformation_templates SET template_type = 'hl7_fhir_transform' WHERE template_type IN ('core.mapping', 'hl7_to_fhir_mapping');
UPDATE transformation_templates SET template_type = 'field_mapping' WHERE template_type = 'core.transformation';
UPDATE transformation_templates SET template_type = 'enrichment.api' WHERE template_type = 'pre.enrichment.api';
UPDATE transformation_templates SET template_type = 'enrichment.database' WHERE template_type = 'pre.enrichment.database';
UPDATE transformation_templates SET template_type = 'enrichment.script' WHERE template_type = 'pre.enrichment.script';
UPDATE transformation_templates SET template_type = 'fhir_validation' WHERE template_type = 'post.validation';
UPDATE transformation_templates SET layer = 'core' WHERE layer IN ('pre', 'post');

-- Set default layer to 'core' for all future inserts
ALTER TABLE transformation_steps ALTER COLUMN layer SET DEFAULT 'core';
