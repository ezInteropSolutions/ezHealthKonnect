-- V292: drop the step copies some saves left in transformation_pipelines.pipeline_config.
--
-- pipeline_config holds pipeline-level settings (default retry, default
-- error handling). The "Use template" flow used to submit the whole template
-- scaffold as pipeline_config, so every step's config, credentials
-- included, was stored there unencrypted next to the real (encrypted)
-- transformation_steps rows. Nothing reads steps from pipeline_config: the
-- engine and Pipeline Builder read only the settings, and steps come from
-- transformation_steps. The save path now strips them (pipelineController
-- pipelineLevelConfig); this removes the copies already stored.

UPDATE transformation_pipelines
SET pipeline_config = pipeline_config - 'execution_groups' - 'steps' - 'layers',
    updated_at = NOW()
WHERE pipeline_config ?| ARRAY['execution_groups', 'steps', 'layers'];
