-- V25: Performance Optimization for Million+ Messages
-- Adds composite indexes and materialized views for high-volume scenarios

-- ============================================
-- Add Composite Indexes for Common Query Patterns
-- ============================================

DO $$
DECLARE
    table_record RECORD;
    index_name TEXT;
BEGIN
    -- Loop through all messages_intf_* tables
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        RAISE NOTICE 'Adding performance indexes to: %', table_record.tablename;

        -- Composite index: status + received_at (for filtered date queries)
        index_name := format('idx_%s_status_received', substring(table_record.tablename from 1 for 50));
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I
            ON %I(status, received_at DESC)
        ', index_name, table_record.tablename);

        -- Composite index: message_type + received_at (for type filtering)
        index_name := format('idx_%s_type_received', substring(table_record.tablename from 1 for 50));
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I
            ON %I(message_type, received_at DESC)
        ', index_name, table_record.tablename);

        -- Composite index: status + message_type (for combined filtering)
        index_name := format('idx_%s_status_type', substring(table_record.tablename from 1 for 50));
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I
            ON %I(status, message_type)
        ', index_name, table_record.tablename);

        -- Composite index: received_at + message_type + status (covering index)
        index_name := format('idx_%s_covering', substring(table_record.tablename from 1 for 50));
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I
            ON %I(received_at DESC, message_type, status)
            INCLUDE (message_id, message_size, error_count, delivery_status)
        ', index_name, table_record.tablename);

        -- Partial index: only failed/error messages (for error dashboards)
        index_name := format('idx_%s_failures', substring(table_record.tablename from 1 for 50));
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I
            ON %I(received_at DESC)
            WHERE status IN (''failed'', ''error'') OR error_count > 0
        ', index_name, table_record.tablename);

        -- Note: Partial index with CURRENT_TIMESTAMP not possible (not immutable)
        -- Use date range queries instead with the covering index above

        RAISE NOTICE 'Completed performance indexes for: %', table_record.tablename;
    END LOOP;
END $$;

-- ============================================
-- Stats Caching Table (Manual refresh via function)
-- ============================================

CREATE TABLE IF NOT EXISTS interface_stats_cache (
    interface_id UUID PRIMARY KEY,
    interface_name VARCHAR(255),
    user_id UUID,
    total_messages BIGINT DEFAULT 0,
    successful_messages BIGINT DEFAULT 0,
    failed_messages BIGINT DEFAULT 0,
    processing_messages BIGINT DEFAULT 0,
    last_24h_messages BIGINT DEFAULT 0,
    avg_processing_time NUMERIC(10,2) DEFAULT 0,
    last_message_at TIMESTAMP WITH TIME ZONE,
    cache_updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_interface_stats_cache_user_id
ON interface_stats_cache(user_id);

CREATE INDEX IF NOT EXISTS idx_interface_stats_cache_updated
ON interface_stats_cache(cache_updated_at);

-- ============================================
-- Function to Refresh Stats Cache
-- ============================================

CREATE OR REPLACE FUNCTION refresh_interface_stats_cache()
RETURNS void AS $$
DECLARE
    interface_record RECORD;
    v_table_name TEXT;
    v_stats RECORD;
BEGIN
    FOR interface_record IN
        SELECT id, name, user_id FROM interfaces WHERE status = 'active'
    LOOP
        v_table_name := 'messages_intf_' || REPLACE(interface_record.id::text, '-', '_');

        -- Check if table exists
        IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = v_table_name) THEN
            -- Get stats from interface table
            EXECUTE format('
                SELECT
                    COUNT(*) as total,
                    COUNT(*) FILTER (WHERE status IN (''sent'', ''delivered'')) as successful,
                    COUNT(*) FILTER (WHERE status IN (''failed'', ''error'')) as failed,
                    COUNT(*) FILTER (WHERE status IN (''received'', ''processing'')) as processing,
                    COUNT(*) FILTER (WHERE received_at > NOW() - INTERVAL ''24 hours'') as last_24h,
                    AVG(processing_time_ms)::NUMERIC(10,2) as avg_time,
                    MAX(received_at) as last_msg
                FROM %I
            ', v_table_name) INTO v_stats;

            -- Upsert into cache
            INSERT INTO interface_stats_cache (
                interface_id, interface_name, user_id, total_messages,
                successful_messages, failed_messages, processing_messages,
                last_24h_messages, avg_processing_time, last_message_at, cache_updated_at
            ) VALUES (
                interface_record.id, interface_record.name, interface_record.user_id,
                v_stats.total, v_stats.successful, v_stats.failed, v_stats.processing,
                v_stats.last_24h, v_stats.avg_time, v_stats.last_msg, NOW()
            )
            ON CONFLICT (interface_id) DO UPDATE SET
                interface_name = EXCLUDED.interface_name,
                total_messages = EXCLUDED.total_messages,
                successful_messages = EXCLUDED.successful_messages,
                failed_messages = EXCLUDED.failed_messages,
                processing_messages = EXCLUDED.processing_messages,
                last_24h_messages = EXCLUDED.last_24h_messages,
                avg_processing_time = EXCLUDED.avg_processing_time,
                last_message_at = EXCLUDED.last_message_at,
                cache_updated_at = EXCLUDED.cache_updated_at;
        END IF;
    END LOOP;

    RAISE NOTICE 'Interface stats cache refreshed at %', NOW();
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION refresh_interface_stats_cache() IS 'Refreshes interface statistics cache table. Call this periodically (e.g., every 5 minutes) via cron or pg_cron extension.';

-- ============================================
-- Optimized Stats Function for Single Interface
-- (Uses window functions for better performance)
-- ============================================

CREATE OR REPLACE FUNCTION get_interface_stats_fast(
    p_interface_id UUID,
    p_hours INTEGER DEFAULT 24
)
RETURNS TABLE(
    total_messages BIGINT,
    successful_messages BIGINT,
    failed_messages BIGINT,
    processing_messages BIGINT,
    recent_messages BIGINT,
    avg_processing_time NUMERIC,
    last_message_at TIMESTAMP WITH TIME ZONE,
    success_rate NUMERIC
) AS $$
DECLARE
    v_table_name TEXT;
BEGIN
    -- Get table name
    v_table_name := 'messages_intf_' || REPLACE(p_interface_id::text, '-', '_');

    -- Check if table exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename = v_table_name
    ) THEN
        -- Return zeros if table doesn't exist
        RETURN QUERY SELECT 0::BIGINT, 0::BIGINT, 0::BIGINT, 0::BIGINT, 0::BIGINT,
                            0::NUMERIC, NULL::TIMESTAMP WITH TIME ZONE, 0::NUMERIC;
        RETURN;
    END IF;

    -- Use single scan with window functions for all stats
    RETURN QUERY EXECUTE format('
        SELECT
            COUNT(*) as total_messages,
            COUNT(*) FILTER (WHERE status IN (''sent'', ''delivered'')) as successful_messages,
            COUNT(*) FILTER (WHERE status IN (''failed'', ''error'')) as failed_messages,
            COUNT(*) FILTER (WHERE status IN (''received'', ''processing'')) as processing_messages,
            COUNT(*) FILTER (WHERE received_at > NOW() - INTERVAL ''%s hours'') as recent_messages,
            AVG(processing_time_ms)::NUMERIC(10,2) as avg_processing_time,
            MAX(received_at) as last_message_at,
            CASE
                WHEN COUNT(*) > 0 THEN
                    (COUNT(*) FILTER (WHERE status IN (''sent'', ''delivered'')) * 100.0 / COUNT(*))::NUMERIC(5,2)
                ELSE 0
            END as success_rate
        FROM %I
    ', p_hours, v_table_name);
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_interface_stats_fast(UUID, INTEGER) IS 'Optimized single-scan stats calculation for an interface. Much faster than multiple COUNT queries.';

-- ============================================
-- Create Partition-Ready Structure (Future)
-- ============================================

COMMENT ON SCHEMA public IS 'V25: Added performance indexes for million+ messages. Composite indexes on common query patterns. Materialized view for stats caching. Optimized stats functions.';

-- ============================================
-- Performance Analysis Query (for monitoring)
-- ============================================

-- This query helps identify slow queries and missing indexes
CREATE OR REPLACE VIEW query_performance_stats AS
SELECT
    schemaname,
    relname as tablename,
    indexrelname as indexname,
    idx_scan as times_used,
    idx_tup_read as tuples_read,
    idx_tup_fetch as tuples_fetched,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
AND relname LIKE 'messages_intf_%'
ORDER BY idx_scan DESC;

COMMENT ON VIEW query_performance_stats IS 'Monitor index usage and performance. Low idx_scan values indicate unused indexes.';

-- ============================================
-- Vacuum and Analyze Recommendations
-- ============================================

-- Run this periodically for optimal performance
CREATE OR REPLACE FUNCTION maintain_message_tables()
RETURNS void AS $$
DECLARE
    table_record RECORD;
BEGIN
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        EXECUTE format('VACUUM ANALYZE %I', table_record.tablename);
        RAISE NOTICE 'Maintained table: %', table_record.tablename;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION maintain_message_tables() IS 'Vacuum and analyze all interface message tables. Run this weekly or after bulk inserts.';
