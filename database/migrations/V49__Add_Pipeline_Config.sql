-- V49: Add pipeline-level configuration for default error handling and retry
-- Enables users to set defaults at the pipeline level instead of configuring every step individually.
-- Steps can override or inherit pipeline defaults via resolution chain:
--   Pipeline Default → Step Override → Final Config

ALTER TABLE transformation_pipelines
ADD COLUMN IF NOT EXISTS pipeline_config JSONB DEFAULT '{}';

COMMENT ON COLUMN transformation_pipelines.pipeline_config IS
'Pipeline-level configuration: default error handling, retry, notifications. Steps inherit these unless overridden.';
