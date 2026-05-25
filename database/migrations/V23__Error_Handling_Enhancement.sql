-- V23: Error Handling Enhancement
-- Comprehensive error capture and tracking for message processing
-- Inspired by Mirth Connect's error handling capabilities

-- ============================================================================
-- PART 1: Add error tracking columns to interface message tables
-- ============================================================================

-- Note: These columns will be added to all existing interface-specific tables
-- via the update_interface_tables() function below

-- Template for new columns (applied to messages_intf_* tables):
-- error_stack JSONB - Array of all errors/warnings from all processing stages
-- last_error_timestamp TIMESTAMP - When the most recent error occurred
-- last_error_stage VARCHAR(50) - Which stage produced the last error
-- error_severity VARCHAR(20) - Severity of last error: 'warning', 'error', 'critical'

-- ============================================================================
-- PART 2: Update InterfaceTableManager schema
-- ============================================================================

-- This function will be called by InterfaceTableManager.js when creating new tables
CREATE OR REPLACE FUNCTION get_message_table_schema_v23()
RETURNS TEXT AS $$
BEGIN
    RETURN '
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        message_id VARCHAR(255) UNIQUE NOT NULL,
        correlation_id VARCHAR(255),
        interface_id UUID NOT NULL,
        status VARCHAR(50) DEFAULT ''received'',
        priority INTEGER DEFAULT 5,
        received_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        source_type VARCHAR(50),
        source_endpoint VARCHAR(255),
        source_ip VARCHAR(50),
        message_type VARCHAR(100),
        message_size INTEGER,
        message_encoding VARCHAR(50),
        raw_message TEXT,
        processing_completed_at TIMESTAMP WITH TIME ZONE,
        processing_time_ms INTEGER,
        error_count INTEGER DEFAULT 0,
        last_error_message TEXT,

        -- V19: JSON conversion columns
        parsed_at TIMESTAMP WITH TIME ZONE,
        parsing_status VARCHAR(50),
        parsing_time_ms INTEGER,
        parsing_error TEXT,

        -- V23: Error handling enhancement
        error_stack JSONB DEFAULT ''[]''::jsonb,
        last_error_timestamp TIMESTAMP WITH TIME ZONE,
        last_error_stage VARCHAR(50),
        error_severity VARCHAR(20),

        delivery_status VARCHAR(50),
        delivery_attempts INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
    ';
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- PART 3: Function to add error columns to existing interface tables
-- ============================================================================

CREATE OR REPLACE FUNCTION add_error_columns_to_interface_tables()
RETURNS TABLE(table_name TEXT, status TEXT) AS $$
DECLARE
    v_table_name TEXT;
    v_sql TEXT;
    v_count INTEGER := 0;
BEGIN
    -- Loop through all interface message tables
    FOR v_table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        BEGIN
            -- Add error_stack column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS error_stack JSONB DEFAULT ''[]''::jsonb', v_table_name);
            EXECUTE v_sql;

            -- Add last_error_timestamp column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE', v_table_name);
            EXECUTE v_sql;

            -- Add last_error_stage column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50)', v_table_name);
            EXECUTE v_sql;

            -- Add error_severity column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20)', v_table_name);
            EXECUTE v_sql;

            v_count := v_count + 1;
            table_name := v_table_name;
            status := 'SUCCESS';
            RETURN NEXT;

        EXCEPTION WHEN OTHERS THEN
            table_name := v_table_name;
            status := 'FAILED: ' || SQLERRM;
            RETURN NEXT;
        END;
    END LOOP;

    RAISE NOTICE 'Added error columns to % interface tables', v_count;
END;
$$ LANGUAGE plpgsql;

-- Execute the function to add columns to all existing tables
SELECT * FROM add_error_columns_to_interface_tables();

-- ============================================================================
-- PART 4: Create indexes for error queries
-- ============================================================================

CREATE OR REPLACE FUNCTION create_error_indexes_on_interface_tables()
RETURNS TABLE(table_name TEXT, index_name TEXT, status TEXT) AS $$
DECLARE
    v_table_name TEXT;
    v_index_name TEXT;
    v_sql TEXT;
BEGIN
    FOR v_table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        BEGIN
            -- Index for error severity (for filtering failed messages)
            v_index_name := format('idx_%s_error_severity', v_table_name);
            v_sql := format('CREATE INDEX IF NOT EXISTS %I ON %I(error_severity) WHERE error_severity IS NOT NULL',
                          v_index_name, v_table_name);
            EXECUTE v_sql;
            table_name := v_table_name;
            index_name := v_index_name;
            status := 'SUCCESS';
            RETURN NEXT;

            -- Index for error timestamp (for time-based queries)
            v_index_name := format('idx_%s_error_timestamp', v_table_name);
            v_sql := format('CREATE INDEX IF NOT EXISTS %I ON %I(last_error_timestamp) WHERE last_error_timestamp IS NOT NULL',
                          v_index_name, v_table_name);
            EXECUTE v_sql;
            table_name := v_table_name;
            index_name := v_index_name;
            status := 'SUCCESS';
            RETURN NEXT;

            -- Index for error stage (for stage-specific error analysis)
            v_index_name := format('idx_%s_error_stage', v_table_name);
            v_sql := format('CREATE INDEX IF NOT EXISTS %I ON %I(last_error_stage) WHERE last_error_stage IS NOT NULL',
                          v_index_name, v_table_name);
            EXECUTE v_sql;
            table_name := v_table_name;
            index_name := v_index_name;
            status := 'SUCCESS';
            RETURN NEXT;

        EXCEPTION WHEN OTHERS THEN
            table_name := v_table_name;
            index_name := v_index_name;
            status := 'FAILED: ' || SQLERRM;
            RETURN NEXT;
        END;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Execute the function to create indexes
SELECT * FROM create_error_indexes_on_interface_tables();

-- ============================================================================
-- PART 5: Function to add error to message error stack
-- ============================================================================

CREATE OR REPLACE FUNCTION add_error_to_message_stack(
    p_table_name TEXT,
    p_message_id TEXT,
    p_error_context JSONB
) RETURNS VOID AS $$
DECLARE
    v_sql TEXT;
    v_error_severity TEXT;
BEGIN
    -- Extract severity from error context
    v_error_severity := p_error_context->>'severity';

    -- Build dynamic SQL to update the specific interface table
    v_sql := format('
        UPDATE %I
        SET
            error_stack = error_stack || $1::jsonb,
            error_count = error_count + 1,
            last_error_message = $1->>''message'',
            last_error_timestamp = ($1->>''timestamp'')::timestamp with time zone,
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

    -- Log the error addition
    RAISE NOTICE 'Error added to message % in table %: % [%]',
        p_message_id, p_table_name, p_error_context->>'message', v_error_severity;

EXCEPTION WHEN OTHERS THEN
    RAISE WARNING 'Failed to add error to message stack: %', SQLERRM;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- PART 6: Function to retrieve all errors for a message
-- ============================================================================

CREATE OR REPLACE FUNCTION get_message_errors(
    p_table_name TEXT,
    p_message_id TEXT
) RETURNS TABLE(
    error_timestamp TIMESTAMP WITH TIME ZONE,
    stage TEXT,
    severity TEXT,
    error_type TEXT,
    message TEXT,
    details TEXT,
    stack_trace TEXT,
    recovery_action TEXT
) AS $$
DECLARE
    v_sql TEXT;
BEGIN
    v_sql := format('
        SELECT
            (error->>''timestamp'')::timestamp with time zone as error_timestamp,
            error->>''stage'' as stage,
            error->>''severity'' as severity,
            error->>''error_type'' as error_type,
            error->>''message'' as message,
            error->>''details'' as details,
            error->>''stack_trace'' as stack_trace,
            error->>''recovery_action'' as recovery_action
        FROM %I,
        LATERAL jsonb_array_elements(error_stack) AS error
        WHERE message_id = $1
        ORDER BY (error->>''timestamp'')::timestamp with time zone DESC
    ', p_table_name);

    RETURN QUERY EXECUTE v_sql USING p_message_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- PART 7: Error statistics function
-- ============================================================================

CREATE OR REPLACE FUNCTION get_error_statistics(
    p_table_name TEXT,
    p_hours INTEGER DEFAULT 24
) RETURNS TABLE(
    total_errors BIGINT,
    critical_errors BIGINT,
    regular_errors BIGINT,
    warnings BIGINT,
    most_common_error_type TEXT,
    most_common_error_stage TEXT,
    error_rate_percent NUMERIC
) AS $$
DECLARE
    v_sql TEXT;
BEGIN
    v_sql := format('
        WITH error_stats AS (
            SELECT
                COUNT(*) FILTER (WHERE error_severity IS NOT NULL) as total_errors,
                COUNT(*) FILTER (WHERE error_severity = ''critical'') as critical_errors,
                COUNT(*) FILTER (WHERE error_severity = ''error'') as regular_errors,
                COUNT(*) FILTER (WHERE error_severity = ''warning'') as warnings,
                COUNT(*) as total_messages
            FROM %I
            WHERE received_at >= NOW() - INTERVAL ''%s hours''
        ),
        error_types AS (
            SELECT
                error->>''error_type'' as error_type,
                COUNT(*) as count
            FROM %I,
            LATERAL jsonb_array_elements(error_stack) AS error
            WHERE received_at >= NOW() - INTERVAL ''%s hours''
            GROUP BY error->>''error_type''
            ORDER BY count DESC
            LIMIT 1
        ),
        error_stages AS (
            SELECT
                error->>''stage'' as stage,
                COUNT(*) as count
            FROM %I,
            LATERAL jsonb_array_elements(error_stack) AS error
            WHERE received_at >= NOW() - INTERVAL ''%s hours''
            GROUP BY error->>''stage''
            ORDER BY count DESC
            LIMIT 1
        )
        SELECT
            es.total_errors,
            es.critical_errors,
            es.regular_errors,
            es.warnings,
            et.error_type as most_common_error_type,
            est.stage as most_common_error_stage,
            CASE
                WHEN es.total_messages > 0
                THEN ROUND((es.total_errors::NUMERIC / es.total_messages::NUMERIC) * 100, 2)
                ELSE 0
            END as error_rate_percent
        FROM error_stats es
        CROSS JOIN error_types et
        CROSS JOIN error_stages est
    ', p_table_name, p_hours, p_table_name, p_hours, p_table_name, p_hours);

    RETURN QUERY EXECUTE v_sql;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- PART 8: Add error columns to output tables as well
-- ============================================================================

CREATE OR REPLACE FUNCTION add_error_columns_to_output_tables()
RETURNS TABLE(table_name TEXT, status TEXT) AS $$
DECLARE
    v_table_name TEXT;
    v_sql TEXT;
    v_count INTEGER := 0;
BEGIN
    FOR v_table_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'output_intf_%'
    LOOP
        BEGIN
            -- Add error_stack column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS error_stack JSONB DEFAULT ''[]''::jsonb', v_table_name);
            EXECUTE v_sql;

            -- Add last_error_timestamp column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE', v_table_name);
            EXECUTE v_sql;

            -- Add last_error_stage column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50)', v_table_name);
            EXECUTE v_sql;

            -- Add error_severity column
            v_sql := format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20)', v_table_name);
            EXECUTE v_sql;

            v_count := v_count + 1;
            table_name := v_table_name;
            status := 'SUCCESS';
            RETURN NEXT;

        EXCEPTION WHEN OTHERS THEN
            table_name := v_table_name;
            status := 'FAILED: ' || SQLERRM;
            RETURN NEXT;
        END;
    END LOOP;

    RAISE NOTICE 'Added error columns to % output tables', v_count;
END;
$$ LANGUAGE plpgsql;

-- Execute the function to add columns to output tables
SELECT * FROM add_error_columns_to_output_tables();

-- ============================================================================
-- Migration Complete
-- ============================================================================

-- Summary of changes:
-- ✓ Added error_stack, last_error_timestamp, last_error_stage, error_severity to all messages_intf_* tables
-- ✓ Added error_stack, last_error_timestamp, last_error_stage, error_severity to all output_intf_* tables
-- ✓ Created indexes for efficient error queries
-- ✓ Created add_error_to_message_stack() function for error capture
-- ✓ Created get_message_errors() function for error retrieval
-- ✓ Created get_error_statistics() function for error analytics
-- ✓ Updated get_message_table_schema_v23() for new interface tables

COMMENT ON FUNCTION add_error_to_message_stack IS 'Captures an error/warning and adds it to the message error stack';
COMMENT ON FUNCTION get_message_errors IS 'Retrieves all errors for a specific message ordered by timestamp';
COMMENT ON FUNCTION get_error_statistics IS 'Calculates error statistics for a given time period';
