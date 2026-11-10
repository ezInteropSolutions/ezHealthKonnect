-- V284: error-capture columns on every per-interface message table, and a capture function that fails loudly.
--
-- Why: V23 added error_stack / last_error_timestamp / last_error_stage / error_severity to the message tables that
-- existed at the time, and add_error_to_message_stack() (called by the Go ErrorCaptureService from ~10 places in the
-- processing engine), get_message_errors() (the message viewer's Errors tab) and the Go error summary all depend on
-- them. The later standardised table schema (InterfaceTableManager.getMessageTableSchema) never included them, so on
-- every newer interface:
--   * add_error_to_message_stack()'s single UPDATE failed ("column error_stack does not exist"), and its
--     EXCEPTION WHEN OTHERS ... RAISE WARNING swallowed it, so the Go service believed the capture had succeeded;
--     the error details were dropped and the message was never marked failed by this path;
--   * GET /api/messages/:id/errors answered 500.
-- Found by the v26.10 UAT pass (2,195 of 2,199 tables lacked the column).
--
-- What this does:
--   1. adds the four new columns, plus ensures last_error_at and updated_at (older standard columns a small number of
--      pre-InterfaceTableManager-1.3 tables still lack — one real table in this project's own history had neither),
--      on every messages_intf_* table (InterfaceTableManager 1.3 now creates all six for new tables; this migration
--      lazily upgrades any existing table it did not reach);
--   2. backfills severity/timestamp from the summary columns older code did record (error_count, last_error_message,
--      last_error_at) — it does NOT invent error_stack entries: no detail was captured, so none is fabricated;
--   3. redefines add_error_to_message_stack(): null-safe appends, keeps the standard last_error_at in step with
--      last_error_timestamp, and no longer swallows failures — an error in the error handler must be loud.

DO $$
DECLARE
    t RECORD;
    n INTEGER := 0;
BEGIN
    FOR t IN
        SELECT tablename FROM pg_tables
        WHERE schemaname = 'public' AND tablename LIKE 'messages\_intf\_%'
    LOOP
        EXECUTE format($f$
            ALTER TABLE %I
                ADD COLUMN IF NOT EXISTS error_stack JSONB NOT NULL DEFAULT '[]'::jsonb,
                ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE,
                ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50),
                ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20),
                ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMP WITH TIME ZONE,
                ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        $f$, t.tablename);

        EXECUTE format($f$
            UPDATE %I
            SET last_error_timestamp = COALESCE(last_error_timestamp, last_error_at, updated_at),
                error_severity       = COALESCE(error_severity, 'error')
            WHERE COALESCE(error_count, 0) > 0
              AND last_error_message IS NOT NULL
              AND error_severity IS NULL
        $f$, t.tablename);

        n := n + 1;
    END LOOP;
    RAISE NOTICE 'V284: error-capture columns ensured on % message tables', n;
END $$;

CREATE OR REPLACE FUNCTION add_error_to_message_stack(
    p_table_name TEXT,
    p_message_id TEXT,
    p_error_context JSONB
) RETURNS VOID AS $$
DECLARE
    v_sql  TEXT;
    v_rows BIGINT;
BEGIN
    v_sql := format('
        UPDATE %I
        SET
            error_stack = COALESCE(error_stack, ''[]''::jsonb) || $1::jsonb,
            error_count = COALESCE(error_count, 0) + 1,
            last_error_message = $1->>''message'',
            last_error_timestamp = ($1->>''timestamp'')::timestamp with time zone,
            last_error_at = ($1->>''timestamp'')::timestamp with time zone,
            last_error_stage = $1->>''stage'',
            error_severity = CASE
                WHEN $1->>''severity'' = ''critical'' THEN ''critical''
                WHEN error_severity = ''critical'' THEN ''critical''
                WHEN $1->>''severity'' = ''error'' THEN ''error''
                WHEN error_severity = ''error'' THEN ''error''
                ELSE $1->>''severity''
            END,
            status = CASE
                WHEN $1->>''severity'' IN (''error'', ''critical'') THEN ''failed''
                ELSE status
            END,
            updated_at = NOW()
        WHERE message_id = $2
    ', p_table_name);

    EXECUTE v_sql USING p_error_context, p_message_id;
    GET DIAGNOSTICS v_rows = ROW_COUNT;

    IF v_rows = 0 THEN
        -- Not an exception (a capture can legitimately race ahead of the row), but never silent.
        RAISE WARNING 'add_error_to_message_stack: message % not found in %', p_message_id, p_table_name;
    END IF;
    -- No EXCEPTION block: any other failure propagates to the caller (the Go ErrorCaptureService logs it) instead of
    -- being reported as success.
END;
$$ LANGUAGE plpgsql;
