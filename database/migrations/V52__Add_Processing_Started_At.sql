-- V52__Add_Processing_Started_At.sql
-- Phase 2 Durability: adds processing_started_at to all interface message tables.
-- Required for status lifecycle (received → processing → delivered/failed)
-- and startup recovery (find messages stuck in 'processing' state).
--
-- Uses IF NOT EXISTS guard so it is safe to re-run.

DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOR tbl IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename LIKE 'messages_intf_%'
        ORDER BY tablename
    LOOP
        EXECUTE format(
            'ALTER TABLE %I ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ',
            tbl
        );
    END LOOP;
END;
$$;

-- Also update getMessageTableSchema standard so new tables include the column.
-- (InterfaceTableManager reads the schema at runtime — no SQL needed here,
--  the Go code adds the column to the CREATE TABLE template.)
