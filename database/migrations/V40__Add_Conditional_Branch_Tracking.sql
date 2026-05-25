-- V40__Add_Conditional_Branch_Tracking.sql
-- Adds columns to track conditional branch relationships for visual flowchart persistence
-- This allows the UI to restore which branch (TRUE/FALSE) a step belongs to after page refresh

-- Add parent conditional step tracking to transformation_steps
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS parent_conditional_step_id UUID REFERENCES transformation_steps(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS branch_type VARCHAR(10) CHECK (branch_type IS NULL OR branch_type IN ('true', 'false'));

-- Index for efficient lookup of steps by their parent conditional step
CREATE INDEX IF NOT EXISTS idx_transformation_steps_parent_conditional
ON transformation_steps(parent_conditional_step_id)
WHERE parent_conditional_step_id IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN transformation_steps.parent_conditional_step_id IS 'References the conditional step (If-Then-Else) that this step is connected to via branch routing';
COMMENT ON COLUMN transformation_steps.branch_type IS 'Which branch of the conditional step this step belongs to: true or false';
