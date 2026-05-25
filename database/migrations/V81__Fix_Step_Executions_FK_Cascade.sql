-- V81__Fix_Step_Executions_FK_Cascade.sql
-- Fix: pipeline save fails with FK violation when re-saving a pipeline that has
-- execution history.  The step_executions.step_id FK lacked ON DELETE CASCADE,
-- so DELETE FROM transformation_steps raised:
--   "update or delete on table "transformation_steps" violates foreign key
--    constraint "step_executions_step_id_fkey" on table "step_executions""
--
-- step_executions rows reference the step definition that was current when the
-- execution ran.  When a pipeline is re-saved, all old step rows are replaced
-- with new ones (same logical steps, new UUIDs).  The old execution records
-- become orphans and can be discarded — ON DELETE CASCADE is the correct
-- semantic here.

ALTER TABLE step_executions
    DROP CONSTRAINT IF EXISTS step_executions_step_id_fkey;

ALTER TABLE step_executions
    ADD CONSTRAINT step_executions_step_id_fkey
    FOREIGN KEY (step_id)
    REFERENCES transformation_steps(id)
    ON DELETE CASCADE;
