-- V48: Add container columns for visual container system
-- Loop, Try-Catch, and Retry steps can contain child steps.
-- These columns track which container a step belongs to and which zone within it.

ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS parent_step_id VARCHAR(255) DEFAULT NULL,
ADD COLUMN IF NOT EXISTS container_zone VARCHAR(50) DEFAULT NULL;

-- Index for fast lookup of child steps by parent container
CREATE INDEX IF NOT EXISTS idx_transformation_steps_parent_step
ON transformation_steps(parent_step_id)
WHERE parent_step_id IS NOT NULL;

COMMENT ON COLUMN transformation_steps.parent_step_id IS 'ID of parent container step (Loop, Try-Catch, Retry). NULL = top-level step';
COMMENT ON COLUMN transformation_steps.container_zone IS 'Zone within parent container: body (Loop/Retry), try/catch/finally (Try-Catch). NULL = not in container';
