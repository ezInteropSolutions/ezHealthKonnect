-- V43: Remove layer CHECK constraint from transformation_steps
-- Layers are now purely informational - any step type can go in any position
-- Execution order is determined solely by sequence number

-- Drop the layer check constraint on transformation_steps
ALTER TABLE transformation_steps DROP CONSTRAINT IF EXISTS chk_layer;

-- Drop the layer check constraint on transformation_templates (if exists)
ALTER TABLE transformation_templates DROP CONSTRAINT IF EXISTS chk_template_layer;

-- Set default layer to 'core' for any future rows
ALTER TABLE transformation_steps ALTER COLUMN layer SET DEFAULT 'core';
