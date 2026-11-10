-- V285: one failure is ONE error, and the summary keeps the REASON.
--
-- V284 made add_error_to_message_stack() work on every message table. That exposed that every failure path in the
-- processing engine records a failure TWICE: processing/engine_message_processor.go first calls updateMessageStatus(
-- "failed", {last_error_message: <full reason>, error_count: 1}) (an absolute 1), and then ErrorCaptureService.CaptureError()
-- -> this function, which added another +1 and overwrote last_error_message with the capture's short title
-- ("Pipeline execution failed"). The message viewer therefore showed "2 errors" for one failure, with a banner that no
-- longer said why.
--
-- Fix, in the function (the two writers stay independent):
--   * error_count = the number of recorded errors — never fewer than what the engine's own update recorded, never more
--     than the stack holds: GREATEST(existing count, existing stack length + 1);
--   * last_error_message = "<title>: <details>" when the capture carries details that differ from the title (details
--     truncated to 500 chars), otherwise the title — so the summary and the failed-message banner say why it failed.
-- Everything else is unchanged from V284.

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
            error_count = GREATEST(
                COALESCE(error_count, 0),
                jsonb_array_length(COALESCE(error_stack, ''[]''::jsonb)) + 1
            ),
            error_stack = COALESCE(error_stack, ''[]''::jsonb) || $1::jsonb,
            last_error_message = CASE
                WHEN COALESCE($1->>''details'', '''') <> '''' AND $1->>''details'' <> COALESCE($1->>''message'', '''')
                    THEN COALESCE($1->>''message'', ''Error'') || '': '' || left($1->>''details'', 500)
                ELSE $1->>''message''
            END,
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
    -- No handler for other failures: they propagate to the caller (the Go ErrorCaptureService logs them).
END;
$$ LANGUAGE plpgsql;
