-- V270: Fix broken audit_logs writes in the two V14 interface-table-maintenance functions
-- Applied: 2026-09-20
--
-- update_interface_table_stats() and cleanup_interface_table_messages() (both
-- defined in V14__Mirth_Style_Interface_Tables.sql, called from
-- services/InterfaceTableMaintenanceService.js) have always INSERTed into
-- audit_logs using columns that don't exist on that table (resource_type,
-- resource_id, details — the real schema, per V1__schema_only.sql, has
-- entity_type, entity_id, metadata). Every real invocation of either
-- function has silently failed at the INSERT and thrown, which PL/pgSQL
-- propagates as an exception out of the function itself — not merely a
-- lost audit row, but the whole stats-update/cleanup call failing.
--
-- This repo's own rule is to never edit a shipped migration in place, so V14
-- is left untouched; CREATE OR REPLACE FUNCTION here redefines both with the
-- corrected columns. Function bodies are otherwise byte-for-byte identical
-- to V14's originals — this migration touches only the two broken INSERTs.
--
-- Action names ('table_maintenance', 'table_cleanup') are unchanged from
-- V14 so any historical reasoning about them (there is none live in
-- audit_logs today, since every prior call errored before the INSERT could
-- commit) still applies; matching taxonomy entries are added to
-- services/audit/audit_events.json in the same commit as this migration so
-- these actions carry the same ATNA-shaped defaults as everything else in
-- audit_logs, whether written from Go or from Node's services/auditService.js.

-- ============================================================
-- update_interface_table_stats — corrected audit_logs INSERT
-- ============================================================
CREATE OR REPLACE FUNCTION update_interface_table_stats(p_interface_id UUID)
RETURNS VOID AS $$
DECLARE
    v_table_name VARCHAR(100);
    v_row_count BIGINT;
    v_sql TEXT;
BEGIN
    -- Get table name for interface
    SELECT table_name INTO v_table_name
    FROM interface_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NULL THEN
        RETURN;
    END IF;

    -- Count rows in interface table
    v_sql := 'SELECT COUNT(*) FROM ' || quote_ident(v_table_name);
    EXECUTE v_sql INTO v_row_count;

    -- Update metadata
    UPDATE interface_table_metadata
    SET
        estimated_rows = v_row_count,
        last_maintenance_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP
    WHERE interface_id = p_interface_id;

    -- Log maintenance activity (corrected columns: entity_type/entity_id/metadata,
    -- not the nonexistent resource_type/resource_id/details)
    INSERT INTO audit_logs (
        user_id, action, entity_type, entity_id,
        metadata, result, risk_level, created_at
    ) VALUES (
        NULL, 'table_maintenance', 'interface_table', p_interface_id::text,
        jsonb_build_object(
            'table_name', v_table_name,
            'row_count', v_row_count,
            'maintenance_type', 'statistics_update'
        ),
        'success', 'low',
        CURRENT_TIMESTAMP
    );
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- cleanup_interface_table_messages — corrected audit_logs INSERT
-- ============================================================
CREATE OR REPLACE FUNCTION cleanup_interface_table_messages(
    p_interface_id UUID,
    p_retention_days INTEGER DEFAULT 90
)
RETURNS INTEGER AS $$
DECLARE
    v_table_name VARCHAR(100);
    v_deleted_count INTEGER;
    v_sql TEXT;
BEGIN
    -- Get table name for interface
    SELECT table_name INTO v_table_name
    FROM interface_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NULL THEN
        RETURN 0;
    END IF;

    -- Delete old messages
    v_sql := 'DELETE FROM ' || quote_ident(v_table_name) ||
             ' WHERE received_at < NOW() - INTERVAL ''' || p_retention_days || ' days''';

    EXECUTE v_sql;
    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;

    -- Update statistics
    PERFORM update_interface_table_stats(p_interface_id);

    -- Log cleanup activity (corrected columns)
    INSERT INTO audit_logs (
        user_id, action, entity_type, entity_id,
        metadata, result, risk_level, created_at
    ) VALUES (
        NULL, 'table_cleanup', 'interface_table', p_interface_id::text,
        jsonb_build_object(
            'table_name', v_table_name,
            'deleted_count', v_deleted_count,
            'retention_days', p_retention_days
        ),
        'success', 'medium',
        CURRENT_TIMESTAMP
    );

    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;
