-- V41: Add case_value column for Switch/Case branch tracking
-- This extends the conditional branch tracking to support multiple cases (not just true/false)

-- Add case_value column to track which case a step belongs to in Switch/Case scenarios
-- For IfThenElse: branch_type is "true" or "false"
-- For Switch/Case: case_value is the actual case value (e.g., "M", "F", "default")
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS case_value VARCHAR(255);

-- Add index for efficient lookups
CREATE INDEX IF NOT EXISTS idx_transformation_steps_case_value
ON transformation_steps(parent_conditional_step_id, case_value)
WHERE case_value IS NOT NULL;

-- Add comment explaining usage
COMMENT ON COLUMN transformation_steps.case_value IS
'For Switch/Case steps: the case value this step belongs to (e.g., "M", "F", "default"). Used for automatic branch exclusion when a different case is matched.';
