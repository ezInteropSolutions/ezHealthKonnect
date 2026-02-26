-- V50: Remove layer column from transformation_steps and transformation_templates
-- The layer concept (pre/core/post) was removed in V43-V44. All values have been
-- normalized to 'core' since V44. This migration physically drops the now-vestigial
-- column and its associated index.

-- Drop the index first
DROP INDEX IF EXISTS idx_transformation_steps_layer;
DROP INDEX IF EXISTS idx_transformation_templates_layer;

-- Drop the column comment (implicit when dropping column)
-- Drop layer column from transformation_steps
ALTER TABLE transformation_steps DROP COLUMN IF EXISTS layer;

-- Drop layer column from transformation_templates
ALTER TABLE transformation_templates DROP COLUMN IF EXISTS layer;
