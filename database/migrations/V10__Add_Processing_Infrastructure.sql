-- Migration V10: Add Processing Infrastructure for Runtime Engine
-- Purpose: Support active interface processing with monitoring and lineage

-- =========================================================================
-- STEP 1: Update interfaces table for runtime status
-- =========================================================================

-- Add processing status and monitoring columns
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS interface_status VARCHAR(20) DEFAULT 'draft'
    CHECK (interface_status IN ('draft', 'configured', 'testing', 'active', 'paused', 'error', 'retired')),
ADD COLUMN IF NOT EXISTS last_status_update TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN IF NOT EXISTS processing_stats JSONB DEFAULT '{}',
ADD COLUMN IF NOT EXISTS error_threshold INTEGER DEFAULT 10,
ADD COLUMN IF NOT EXISTS retry_config JSONB DEFAULT '{"maxAttempts": 3, "backoffStrategy": "exponential"}';

-- =========================================================================
-- STEP 2: Create message audit log table for data lineage
-- =========================================================================

CREATE TABLE IF NOT EXISTS message_audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID NOT NULL,
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL, -- INGESTION, TRANSFORMATION, DELIVERY, ERROR
    event_data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Performance indexes
    INDEX (message_id, created_at),
    INDEX (interface_id, event_type, created_at),
    INDEX (created_at) -- For time-based queries
);

-- =========================================================================
-- STEP 3: Create processing jobs table for job management
-- =========================================================================

CREATE TABLE IF NOT EXISTS processing_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    job_type VARCHAR(50) NOT NULL, -- 'message_processing', 'batch_processing', 'scheduled_run'
    job_status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (job_status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),

    -- Job configuration and metadata
    job_config JSONB NOT NULL DEFAULT '{}',
    input_source JSONB, -- Details about input source
    processing_stats JSONB DEFAULT '{}', -- Messages processed, errors, timing

    -- Timing information
    scheduled_at TIMESTAMP WITH TIME ZONE,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms BIGINT,

    -- Error handling
    error_details JSONB,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,

    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id)
);

-- =========================================================================
-- STEP 4: Create interface processing metrics table
-- =========================================================================

CREATE TABLE IF NOT EXISTS interface_processing_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    metric_date DATE NOT NULL DEFAULT CURRENT_DATE,

    -- Volume metrics
    messages_received INTEGER DEFAULT 0,
    messages_processed INTEGER DEFAULT 0,
    messages_failed INTEGER DEFAULT 0,
    messages_retried INTEGER DEFAULT 0,

    -- Performance metrics
    avg_processing_time_ms FLOAT,
    min_processing_time_ms FLOAT,
    max_processing_time_ms FLOAT,
    total_processing_time_ms BIGINT DEFAULT 0,

    -- Error metrics by type
    error_breakdown JSONB DEFAULT '{}', -- {"timeout": 5, "validation": 2, "connection": 1}

    -- Data volume metrics
    total_data_processed_bytes BIGINT DEFAULT 0,
    avg_message_size_bytes FLOAT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_interface_metrics_date UNIQUE(interface_id, metric_date)
);

-- =========================================================================
-- STEP 5: Create processing queues management table
-- =========================================================================

CREATE TABLE IF NOT EXISTS processing_queue_status (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    queue_name VARCHAR(100) NOT NULL,

    -- Queue metrics
    messages_pending INTEGER DEFAULT 0,
    messages_processing INTEGER DEFAULT 0,
    messages_completed_today INTEGER DEFAULT 0,
    messages_failed_today INTEGER DEFAULT 0,

    -- Queue health
    last_message_at TIMESTAMP WITH TIME ZONE,
    last_success_at TIMESTAMP WITH TIME ZONE,
    last_failure_at TIMESTAMP WITH TIME ZONE,

    -- Queue configuration
    max_concurrent_jobs INTEGER DEFAULT 5,
    is_active BOOLEAN DEFAULT true,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_interface_queue UNIQUE(interface_id, queue_name)
);

-- =========================================================================
-- STEP 6: Create indexes for performance
-- =========================================================================

-- Message audit log indexes
CREATE INDEX IF NOT EXISTS idx_message_audit_message_id ON message_audit_log(message_id);
CREATE INDEX IF NOT EXISTS idx_message_audit_interface_event ON message_audit_log(interface_id, event_type);
CREATE INDEX IF NOT EXISTS idx_message_audit_created_at ON message_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_audit_lineage ON message_audit_log(message_id, created_at);

-- Processing jobs indexes
CREATE INDEX IF NOT EXISTS idx_processing_jobs_interface ON processing_jobs(interface_id, job_status);
CREATE INDEX IF NOT EXISTS idx_processing_jobs_status ON processing_jobs(job_status, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_processing_jobs_created ON processing_jobs(created_at DESC);

-- Interface metrics indexes
CREATE INDEX IF NOT EXISTS idx_interface_metrics_date ON interface_processing_metrics(interface_id, metric_date DESC);
CREATE INDEX IF NOT EXISTS idx_interface_metrics_performance ON interface_processing_metrics(metric_date, avg_processing_time_ms);

-- Interface status indexes
CREATE INDEX IF NOT EXISTS idx_interfaces_status ON interfaces(interface_status) WHERE interface_status = 'active';
CREATE INDEX IF NOT EXISTS idx_interfaces_last_update ON interfaces(last_status_update DESC);

-- =========================================================================
-- STEP 7: Create functions for processing engine
-- =========================================================================

-- Function to update interface processing stats
CREATE OR REPLACE FUNCTION update_interface_processing_stats(
    p_interface_id UUID,
    p_processing_time_ms INTEGER,
    p_success BOOLEAN,
    p_error_type VARCHAR DEFAULT NULL
)
RETURNS VOID AS $$
DECLARE
    today_date DATE := CURRENT_DATE;
BEGIN
    -- Update or insert daily metrics
    INSERT INTO interface_processing_metrics (
        interface_id,
        metric_date,
        messages_received,
        messages_processed,
        messages_failed,
        avg_processing_time_ms,
        min_processing_time_ms,
        max_processing_time_ms,
        total_processing_time_ms
    )
    VALUES (
        p_interface_id,
        today_date,
        1,
        CASE WHEN p_success THEN 1 ELSE 0 END,
        CASE WHEN NOT p_success THEN 1 ELSE 0 END,
        p_processing_time_ms,
        p_processing_time_ms,
        p_processing_time_ms,
        p_processing_time_ms
    )
    ON CONFLICT (interface_id, metric_date)
    DO UPDATE SET
        messages_received = interface_processing_metrics.messages_received + 1,
        messages_processed = interface_processing_metrics.messages_processed +
            CASE WHEN p_success THEN 1 ELSE 0 END,
        messages_failed = interface_processing_metrics.messages_failed +
            CASE WHEN NOT p_success THEN 1 ELSE 0 END,
        total_processing_time_ms = interface_processing_metrics.total_processing_time_ms + p_processing_time_ms,
        avg_processing_time_ms = (interface_processing_metrics.total_processing_time_ms + p_processing_time_ms) /
            GREATEST(interface_processing_metrics.messages_received + 1, 1),
        min_processing_time_ms = LEAST(interface_processing_metrics.min_processing_time_ms, p_processing_time_ms),
        max_processing_time_ms = GREATEST(interface_processing_metrics.max_processing_time_ms, p_processing_time_ms),
        updated_at = CURRENT_TIMESTAMP;

    -- Update error breakdown if it's an error
    IF NOT p_success AND p_error_type IS NOT NULL THEN
        UPDATE interface_processing_metrics
        SET error_breakdown = jsonb_set(
            COALESCE(error_breakdown, '{}'),
            ARRAY[p_error_type],
            COALESCE((error_breakdown->>p_error_type)::integer, 0) + 1
        )
        WHERE interface_id = p_interface_id AND metric_date = today_date;
    END IF;

    -- Update interface-level stats
    UPDATE interfaces
    SET
        processing_stats = jsonb_set(
            jsonb_set(
                COALESCE(processing_stats, '{}'),
                '{lastProcessedAt}',
                to_jsonb(CURRENT_TIMESTAMP)
            ),
            '{totalProcessed}',
            COALESCE((processing_stats->>'totalProcessed')::integer, 0) + 1
        ),
        last_status_update = CURRENT_TIMESTAMP
    WHERE id = p_interface_id;
END;
$$ LANGUAGE plpgsql;

-- Function to get interface processing health
CREATE OR REPLACE FUNCTION get_interface_processing_health(p_interface_id UUID)
RETURNS JSONB AS $$
DECLARE
    result JSONB;
    last_24h_metrics RECORD;
    current_status RECORD;
BEGIN
    -- Get current interface status
    SELECT interface_status, last_status_update, processing_stats
    INTO current_status
    FROM interfaces
    WHERE id = p_interface_id;

    -- Get last 24 hours metrics
    SELECT
        COALESCE(SUM(messages_processed), 0) as processed_24h,
        COALESCE(SUM(messages_failed), 0) as failed_24h,
        COALESCE(AVG(avg_processing_time_ms), 0) as avg_time_24h
    INTO last_24h_metrics
    FROM interface_processing_metrics
    WHERE interface_id = p_interface_id
      AND metric_date >= CURRENT_DATE - INTERVAL '1 day';

    result := jsonb_build_object(
        'interfaceId', p_interface_id,
        'currentStatus', current_status.interface_status,
        'lastUpdate', current_status.last_status_update,
        'metrics24h', jsonb_build_object(
            'processed', last_24h_metrics.processed_24h,
            'failed', last_24h_metrics.failed_24h,
            'avgProcessingTime', last_24h_metrics.avg_time_24h,
            'successRate', CASE
                WHEN (last_24h_metrics.processed_24h + last_24h_metrics.failed_24h) > 0
                THEN ROUND((last_24h_metrics.processed_24h::float /
                    (last_24h_metrics.processed_24h + last_24h_metrics.failed_24h)) * 100, 2)
                ELSE 0
            END
        ),
        'overallStats', current_status.processing_stats
    );

    RETURN result;
END;
$$ LANGUAGE plpgsql STABLE;

-- =========================================================================
-- STEP 8: Create triggers for automatic updates
-- =========================================================================

-- Trigger to update interface last_status_update when status changes
CREATE OR REPLACE FUNCTION update_interface_status_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.interface_status IS DISTINCT FROM NEW.interface_status THEN
        NEW.last_status_update = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_interface_status_update ON interfaces;
CREATE TRIGGER tr_interface_status_update
    BEFORE UPDATE ON interfaces
    FOR EACH ROW
    EXECUTE FUNCTION update_interface_status_timestamp();

-- =========================================================================
-- STEP 9: Create views for monitoring and reporting
-- =========================================================================

-- View for real-time processing overview
CREATE OR REPLACE VIEW v_processing_overview AS
SELECT
    i.id as interface_id,
    i.name as interface_name,
    i.interface_status,
    i.last_status_update,
    COALESCE(m.messages_processed, 0) as messages_today,
    COALESCE(m.messages_failed, 0) as failures_today,
    COALESCE(m.avg_processing_time_ms, 0) as avg_processing_time,
    CASE
        WHEN COALESCE(m.messages_processed, 0) + COALESCE(m.messages_failed, 0) > 0
        THEN ROUND((COALESCE(m.messages_processed, 0)::float /
            (COALESCE(m.messages_processed, 0) + COALESCE(m.messages_failed, 0))) * 100, 2)
        ELSE 100
    END as success_rate_today
FROM interfaces i
LEFT JOIN interface_processing_metrics m ON i.id = m.interface_id
    AND m.metric_date = CURRENT_DATE
WHERE i.interface_status IN ('active', 'paused', 'error')
ORDER BY i.name;

-- View for data lineage tracking
CREATE OR REPLACE VIEW v_message_lineage AS
SELECT
    mal.message_id,
    i.name as interface_name,
    mal.event_type,
    mal.event_data,
    mal.created_at,
    ROW_NUMBER() OVER (PARTITION BY mal.message_id ORDER BY mal.created_at) as sequence_number
FROM message_audit_log mal
JOIN interfaces i ON mal.interface_id = i.id
ORDER BY mal.message_id, mal.created_at;

-- =========================================================================
-- STEP 10: Grant permissions and add comments
-- =========================================================================

-- Grant permissions to application roles
GRANT SELECT, INSERT, UPDATE ON message_audit_log TO PUBLIC;
GRANT SELECT, INSERT, UPDATE ON processing_jobs TO PUBLIC;
GRANT SELECT, INSERT, UPDATE ON interface_processing_metrics TO PUBLIC;
GRANT SELECT ON v_processing_overview TO PUBLIC;
GRANT SELECT ON v_message_lineage TO PUBLIC;

-- Add table comments
COMMENT ON TABLE message_audit_log IS 'Complete audit trail for message processing with data lineage';
COMMENT ON TABLE processing_jobs IS 'Job management for batch and scheduled processing tasks';
COMMENT ON TABLE interface_processing_metrics IS 'Daily aggregated metrics for interface performance monitoring';
COMMENT ON TABLE processing_queue_status IS 'Real-time status of processing queues for load management';

COMMENT ON COLUMN interfaces.interface_status IS 'Current operational status of the interface';
COMMENT ON COLUMN interfaces.processing_stats IS 'Real-time processing statistics and metadata';

-- =========================================================================
-- STEP 11: Migration completion summary
-- =========================================================================

DO $$
DECLARE
    active_interfaces INTEGER;
    total_metrics INTEGER;
BEGIN
    SELECT COUNT(*) INTO active_interfaces FROM interfaces WHERE interface_status = 'active';
    SELECT COUNT(*) INTO total_metrics FROM interface_processing_metrics;

    RAISE NOTICE '=== V10 Processing Infrastructure Migration Completed ===';
    RAISE NOTICE 'Enhanced interfaces table with status tracking';
    RAISE NOTICE 'Created message_audit_log for complete data lineage';
    RAISE NOTICE 'Created processing_jobs for job management';
    RAISE NOTICE 'Created interface_processing_metrics for monitoring';
    RAISE NOTICE 'Added processing functions and triggers';
    RAISE NOTICE 'Current active interfaces: %', active_interfaces;
    RAISE NOTICE '✅ Runtime Processing Engine infrastructure ready!';
END $$;