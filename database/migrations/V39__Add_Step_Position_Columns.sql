-- V39__Add_Step_Position_Columns.sql
-- Add position_x and position_y columns to transformation_steps table
-- These columns store the visual position of steps in the flowchart UI
-- Allows users to arrange steps and have their layout persisted

-- Add position columns to transformation_steps
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS position_x DOUBLE PRECISION DEFAULT NULL,
ADD COLUMN IF NOT EXISTS position_y DOUBLE PRECISION DEFAULT NULL;

-- Add comment for documentation
COMMENT ON COLUMN transformation_steps.position_x IS 'X coordinate of step in flowchart UI (pixels from left)';
COMMENT ON COLUMN transformation_steps.position_y IS 'Y coordinate of step in flowchart UI (pixels from top)';

-- Create partial index for steps that have positions set
-- This optimizes queries that filter by positioned steps
CREATE INDEX IF NOT EXISTS idx_transformation_steps_has_position
ON transformation_steps(pipeline_id)
WHERE position_x IS NOT NULL AND position_y IS NOT NULL;
