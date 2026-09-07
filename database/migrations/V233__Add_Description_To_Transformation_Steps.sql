-- V233: transformation_steps was missing a description column entirely,
-- silently discarding every step's authored description on every save.
-- Applied: 2026-09-06
--
-- BACKGROUND
-- ----------
-- Found while investigating user-reported confusion about the "Assemble 835
-- FHIR Bundle" step (V231's EDI-835-to-FHIR template) — that step's own
-- template JSON carries a genuinely helpful, purpose-written description
-- ("resourcePaths accepts both single-resource paths (message.
-- paymentReconciliation) and array-valued ones...") explaining exactly the
-- field the user found opaque. That description IS correctly stored in
-- transformation_pipelines.pipeline_config (confirmed via direct query) and
-- IS correctly read by the frontend's VisualStep.fromJSON/toJSON and
-- rendered into the Basic Properties "Description" textarea (confirmed via
-- direct code read, public/js/pipeline/models/PipelineModels.js and
-- public/js/pipeline/managers/PropertiesPanel.js) -- but it never reaches
-- the browser at all, because:
--   1. transformation_steps (the "single source of truth" table every
--      pipeline actually loads its steps from -- see interfacesController.js's
--      own comment to that effect) never had a description column.
--   2. pipelineController.js's STEPS_SELECT (used by loadPipeline,
--      loadPipelineByInterface, and clonePipeline) never selected it.
--   3. savePipeline's INSERT -- the ONE write path used both by "Use
--      Template" (dashboard.js's useTemplate() POSTs the template's
--      execution_groups, description field intact, straight to
--      POST /api/pipelines) and by every ordinary "Save Pipeline" click from
--      the UI -- never included it either.
-- Net effect: this wasn't EDI-specific or template-specific. ANY step's
-- description, anywhere in the app, was silently discarded the moment it
-- touched the database -- a systemic gap, not a cosmetic one, confirmed by
-- reading every INSERT/UPDATE/SELECT touching this table before writing
-- this migration.

ALTER TABLE transformation_steps ADD COLUMN IF NOT EXISTS description TEXT;

COMMENT ON COLUMN transformation_steps.description IS
    'Author-facing explanation of what this step does and why, shown in the pipeline builder''s Basic Properties panel. Distinct from config (the step''s actual behavior) -- this is documentation only, never read by the execution engine.';
