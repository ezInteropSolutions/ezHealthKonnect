-- V36: Add step alias and namespace support for step output tracking
-- Date: December 19, 2025
-- Purpose: Enable user-friendly step output referencing with aliases

-- Add step_alias column to transformation_steps table
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS step_alias VARCHAR(100);

-- Create unique index on (pipeline_id, step_alias) for uniqueness validation
-- This ensures aliases are unique within a pipeline
CREATE UNIQUE INDEX IF NOT EXISTS idx_pipeline_step_alias
ON transformation_steps(pipeline_id, step_alias)
WHERE step_alias IS NOT NULL;

-- Add index for faster step_alias lookups
CREATE INDEX IF NOT EXISTS idx_step_alias
ON transformation_steps(step_alias)
WHERE step_alias IS NOT NULL;

-- Add namespace and step_alias columns to step_executions table
ALTER TABLE step_executions
ADD COLUMN IF NOT EXISTS namespace VARCHAR(255),
ADD COLUMN IF NOT EXISTS step_alias VARCHAR(100);

-- Add index for faster namespace lookups in execution logs
CREATE INDEX IF NOT EXISTS idx_step_executions_namespace
ON step_executions(namespace)
WHERE namespace IS NOT NULL;

-- Add step_output JSONB column to store step-specific outputs
ALTER TABLE step_executions
ADD COLUMN IF NOT EXISTS step_output JSONB;

-- Add index for step_output JSONB queries
CREATE INDEX IF NOT EXISTS idx_step_executions_step_output
ON step_executions USING GIN (step_output)
WHERE step_output IS NOT NULL;

-- Add comment to explain the purpose
COMMENT ON COLUMN transformation_steps.step_alias IS 'User-defined alias for referencing step outputs (e.g., "empi", "validate_pid")';
COMMENT ON COLUMN step_executions.namespace IS 'Generated namespace combining alias and short ID (e.g., "empi_b4c9f1")';
COMMENT ON COLUMN step_executions.step_output IS 'Step-specific output data in JSON format';
